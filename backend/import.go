package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func putIfAbsent(ctx context.Context, collection *mongo.Collection, id int64, doc any) (bool, error) {
	result, err := collection.UpdateByID(ctx, id, bson.M{"$setOnInsert": doc}, options.UpdateOne().SetUpsert(true))
	return err == nil && result.UpsertedCount == 1, err
}

func (s *store) importLegacy(ctx context.Context, source io.Reader) error {
	var snapshot struct {
		Tasks  []task  `json:"tasks"`
		Timers []timer `json:"timers"`
	}
	if err := json.NewDecoder(source).Decode(&snapshot); err != nil {
		return fmt.Errorf("read SQLite export: %w", err)
	}
	tasks := make(map[int64]*task, len(snapshot.Tasks))
	high := map[string]int64{"tasks": 0, "timers": 0}
	for i := range snapshot.Tasks {
		t := &snapshot.Tasks[i]
		tasks[t.ID] = t
		high["tasks"] = max(high["tasks"], t.ID)
	}
	standalone := make([]*timer, 0, len(snapshot.Timers))
	for i := range snapshot.Timers {
		t := &snapshot.Timers[i]
		t.setExpiry()
		high["timers"] = max(high["timers"], t.ID)
		if t.TaskID == nil {
			standalone = append(standalone, t)
			continue
		}
		parent := tasks[*t.TaskID]
		if parent == nil || parent.Timer != nil {
			return fmt.Errorf("invalid task reference for timer %d", t.ID)
		}
		parent.Timer = t
	}
	for name, highest := range high {
		_, err := s.counters.UpdateByID(ctx, name, bson.M{"$max": bson.M{"value": highest}}, options.UpdateOne().SetUpsert(true))
		if err != nil {
			return err
		}
	}
	addedTasks, addedTimers := 0, 0
	for i := range snapshot.Tasks {
		t := &snapshot.Tasks[i]
		inserted, err := putIfAbsent(ctx, s.tasks, t.ID, t)
		if err != nil {
			return err
		}
		if inserted {
			addedTasks++
			if t.Timer != nil {
				addedTimers++
			}
		}
	}
	for _, t := range standalone {
		inserted, err := putIfAbsent(ctx, s.timers, t.ID, t)
		if err != nil {
			return err
		}
		if inserted {
			addedTimers++
		}
	}
	log.Printf("Imported %d tasks and %d timers. Source had %d tasks and %d timers.", addedTasks, addedTimers, len(snapshot.Tasks), len(snapshot.Timers))
	return nil
}
