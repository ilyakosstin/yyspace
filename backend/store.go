package main

import (
	"context"
	"errors"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type store struct {
	tasks, timers, counters *mongo.Collection
	binLimit                int64
}

func openStore(ctx context.Context, db *mongo.Database, binLimit int64) (*store, error) {
	s := &store{db.Collection("tasks"), db.Collection("timers"), db.Collection("counters"), binLimit}
	_, err := s.tasks.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "deadline", Value: 1}, {Key: "created_at", Value: 1}, {Key: "_id", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "timeout_at", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "finished_at", Value: -1}, {Key: "_id", Value: -1}}},
		{Keys: bson.D{{Key: "timer.expires_at", Value: 1}}, Options: options.Index().SetSparse(true)},
		{Keys: bson.D{{Key: "timer._id", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true)},
	})
	if err != nil {
		return nil, err
	}
	_, err = s.timers.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "expires_at", Value: 1}},
	})
	return s, err
}

func (s *store) nextID(ctx context.Context, name string) (int64, error) {
	var counter struct{ Value int64 }
	err := s.counters.FindOneAndUpdate(ctx, bson.M{"_id": name}, bson.M{"$inc": bson.M{"value": 1}},
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)).Decode(&counter)
	return counter.Value, err
}

func (s *store) createTask(ctx context.Context, t *task) error {
	id, err := s.nextID(ctx, "tasks")
	if err != nil {
		return err
	}
	t.ID = id
	if t.Timer != nil {
		t.Timer.ID, err = s.nextID(ctx, "timers")
		if err != nil {
			return err
		}
		t.Timer.TaskID = &t.ID
	}
	_, err = s.tasks.InsertOne(ctx, t)
	return err
}

func (s *store) createTimer(ctx context.Context, t *timer) error {
	id, err := s.nextID(ctx, "timers")
	if err != nil {
		return err
	}
	t.ID = id
	_, err = s.timers.InsertOne(ctx, t)
	return err
}

// Compare revisions so concurrent requests can safely update the same task or timer.
// A task and its timer are replaced together using MongoDB's single-document atomicity.
func change[T any](ctx context.Context, coll *mongo.Collection, filter bson.M,
	identity func(*T) (int64, *int64), edit func(*T) (bool, error)) (T, error) {
	for range 16 {
		var doc T
		if err := coll.FindOne(ctx, filter).Decode(&doc); err != nil {
			return doc, err
		}
		id, revision := identity(&doc)
		current := bson.M{"_id": id, "revision": *revision}
		remove, err := edit(&doc)
		switch {
		case err != nil:
			return doc, err
		case remove:
			result, err := coll.DeleteOne(ctx, current)
			if err != nil || result.DeletedCount == 1 {
				return doc, err
			}
		default:
			*revision++
			result, err := coll.ReplaceOne(ctx, current, doc)
			if err != nil || result.MatchedCount == 1 {
				return doc, err
			}
		}
	}
	var zero T
	return zero, conflict("This entry changed concurrently; try again")
}

func taskIdentity(t *task) (int64, *int64)   { return t.ID, &t.Revision }
func timerIdentity(t *timer) (int64, *int64) { return t.ID, &t.Revision }

var errAlreadySettled = errors.New("entry is no longer due")

func settleDue[T any](ctx context.Context, coll *mongo.Collection, filter bson.M,
	id func(*T) (int64, *int64), settle func(*T) bool) error {
	cur, err := coll.Find(ctx, filter, options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var entry struct {
			ID int64 `bson:"_id"`
		}
		if err := cur.Decode(&entry); err != nil {
			return err
		}
		_, err := change(ctx, coll, bson.M{"_id": entry.ID}, id, func(doc *T) (bool, error) {
			if !settle(doc) {
				return false, errAlreadySettled
			}
			return false, nil
		})
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) && !errors.Is(err, errAlreadySettled) {
			return err
		}
	}
	return cur.Err()
}

func (s *store) sync(ctx context.Context, now time.Time) error {
	due := bson.M{"$lte": now}
	filter := bson.M{"$or": []bson.M{
		{"status": todo, "type": classic, "timeout_at": due},
		{"timer.expires_at": due},
	}}
	if err := settleDue(ctx, s.tasks, filter, taskIdentity, func(t *task) bool { return t.settle(now) }); err != nil {
		return err
	}
	return settleDue(ctx, s.timers, bson.M{"expires_at": due}, timerIdentity,
		func(t *timer) bool { return t.settle(nil, now) })
}

type state struct {
	ServerTime time.Time `json:"server_time"`
	Tasks      []task    `json:"tasks"`
	Timers     []*timer  `json:"timers"`
}

func readMany[T any](ctx context.Context, coll *mongo.Collection, filter bson.M, opts *options.FindOptionsBuilder) ([]T, error) {
	items := make([]T, 0)
	cur, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	err = cur.All(ctx, &items)
	return items, err
}

func (s *store) state(ctx context.Context, now time.Time) (state, error) {
	order := bson.D{{Key: "deadline", Value: 1}, {Key: "created_at", Value: 1}, {Key: "_id", Value: 1}}
	active, err := readMany[task](ctx, s.tasks, bson.M{"status": todo}, options.Find().SetSort(order))
	if err != nil {
		return state{}, err
	}
	// MongoDB sorts null deadlines first. Move them to the end while retaining index order.
	firstDeadline := 0
	for firstDeadline < len(active) && active[firstDeadline].Deadline == nil {
		firstDeadline++
	}
	tasks := make([]task, 0, len(active))
	tasks = append(tasks, active[firstDeadline:]...)
	tasks = append(tasks, active[:firstDeadline]...)
	history, err := readMany[task](ctx, s.tasks, bson.M{"status": bson.M{"$in": []string{completed, cancelled}}},
		options.Find().SetSort(bson.D{{Key: "finished_at", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(s.binLimit*2))
	if err != nil {
		return state{}, err
	}
	for _, t := range history {
		tasks = append(tasks, t)
	}
	standalone, err := readMany[timer](ctx, s.timers, bson.M{}, options.Find())
	if err != nil {
		return state{}, err
	}
	timers := make([]*timer, 0, len(standalone))
	for _, t := range standalone {
		timers = append(timers, t.response(now))
	}
	for i := range tasks {
		tasks[i] = tasks[i].response(now)
		if tasks[i].Status == todo && tasks[i].Timer != nil {
			timers = append(timers, tasks[i].Timer)
		}
	}
	slices.SortFunc(timers, func(a, b *timer) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return state{now, tasks, timers}, nil
}
