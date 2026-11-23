package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestTaskInput(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	for _, body := range []string{
		`{}`, `{"title":" "}`, `{"title":"x","type":"TIMED"}`,
		`{"title":"x","type":"TIMED","duration_seconds":60,"timeout_seconds":5}`,
		`{"title":"x","duration_seconds":60}`, `{"title":"x","deadline":"2030-01-01T00:00:00"}`,
		`{"title":"x","timeout_seconds":604801}`,
	} {
		var input taskInput
		err := json.Unmarshal([]byte(body), &input)
		if err == nil {
			_, err = input.build(now)
		}
		if err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

func TestTimerChangesTaskAtExpiry(t *testing.T) {
	start := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	timer := newTimer(1, nil, 60)
	task := &task{Status: todo, Type: timed, Timer: timer}
	if err := timer.action(task, "start", start); err != nil {
		t.Fatal(err)
	}
	timer.pause(start.Add(10 * time.Second))
	if got := timer.response(start).RemainingMS; got != 50000 {
		t.Fatalf("remaining after pause = %d", got)
	}
	resumed := start.Add(time.Minute)
	if err := timer.action(task, "start", resumed); err != nil {
		t.Fatal(err)
	}
	end := resumed.Add(50 * time.Second)
	if task.settle(end.Add(-time.Millisecond)) || !task.settle(end.Add(time.Minute)) {
		t.Fatal("settled at the wrong time")
	}
	if task.Status != completed || timer.State != finished || !task.FinishedAt.Equal(end) {
		t.Fatalf("unexpected expiry: %+v / %+v", task, timer)
	}
	timer.reset(task)
	if task.Status != todo || task.FinishedAt != nil || timer.State != idle {
		t.Fatal("reset did not reopen the task")
	}
}

func TestMongoAPI(t *testing.T) {
	uri := os.Getenv("MONGODB_TEST_URI")
	if uri == "" {
		t.Skip("set MONGODB_TEST_URI to run against MongoDB")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	db := client.Database(fmt.Sprintf("focus_test_%d", time.Now().UnixNano()))
	defer db.Drop(context.Background())
	s, err := openStore(ctx, db, 200)
	if err != nil {
		t.Fatal(err)
	}
	handler := s.routes()
	request := func(method, path, body string, want int, result any) {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(method, "/api"+path, strings.NewReader(body)))
		if w.Code != want {
			t.Fatalf("%s %s = %d, want %d: %s", method, path, w.Code, want, w.Body)
		}
		if result != nil {
			if err := json.Unmarshal(w.Body.Bytes(), result); err != nil {
				t.Fatal(err)
			}
		}
	}
	var initial state
	request("GET", "/state", "", 200, &initial)
	if initial.Tasks == nil || initial.Timers == nil {
		t.Fatal("empty arrays must not be null")
	}
	var a, b, c task
	deadline := time.Now().UTC().Add(time.Hour).Truncate(time.Millisecond)
	request("POST", "/tasks", `{"title":" no deadline ","timeout_seconds":60}`, 201, &a)
	request("POST", "/tasks", fmt.Sprintf(`{"title":"timed","type":"TIMED","duration_seconds":60,"deadline":%q}`, deadline.Format(time.RFC3339Nano)), 201, &b)
	var standalone timer
	request("POST", "/timers", `{"duration_seconds":60}`, 201, &standalone)
	var snapshot state
	request("GET", "/state", "", 200, &snapshot)
	if a.Title != "no deadline" || len(snapshot.Tasks) != 2 || snapshot.Tasks[0].ID != b.ID || len(snapshot.Timers) != 2 {
		t.Fatal("state has unexpected tasks or timers")
	}
	expired := time.Now().UTC().Truncate(time.Millisecond).Add(-time.Second)
	if _, err := s.tasks.UpdateByID(ctx, a.ID, bson.M{"$set": bson.M{"timeout_at": expired}}); err != nil {
		t.Fatal(err)
	}
	taskPath := fmt.Sprintf("/tasks/%d", b.ID)
	timerPath := fmt.Sprintf("/timers/%d", b.Timer.ID)
	request("POST", timerPath+"/start", "", 200, nil)
	if _, err := s.tasks.UpdateByID(ctx, b.ID, bson.M{"$set": bson.M{"timer.expires_at": expired}}); err != nil {
		t.Fatal(err)
	}
	request("GET", "/state", "", 200, &snapshot)
	for _, entry := range snapshot.Tasks {
		if entry.FinishedAt == nil || !entry.FinishedAt.Equal(expired) {
			t.Fatalf("expiry timestamp missing for %d", entry.ID)
		}
	}
	request("POST", timerPath+"/reset", "", 200, nil)
	request("DELETE", timerPath, "", 409, nil)

	// Patches to the task and its embedded timer must both survive concurrent writes.
	var workers sync.WaitGroup
	for _, patch := range []struct{ path, body string }{
		{taskPath, `{"description":"**note**"}`},
		{timerPath, `{"remaining_seconds":30}`},
	} {
		workers.Go(func() {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest("PATCH", "/api"+patch.path, strings.NewReader(patch.body)))
			if w.Code != 200 {
				t.Errorf("concurrent patch returned %d: %s", w.Code, w.Body)
			}
		})
	}
	workers.Wait()
	request("PATCH", taskPath, `{}`, 200, &c)
	if c.Status != todo || c.Description == nil || *c.Description != "**note**" || c.Timer.RemainingMS != 30000 {
		t.Fatalf("concurrent edits lost or reset failed: %+v / %+v", c, c.Timer)
	}
	request("DELETE", taskPath, "", 204, nil)
	request("POST", timerPath+"/start", "", 404, nil)
}
