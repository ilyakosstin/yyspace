package main

import (
	"fmt"
	"time"
)

const (
	classic   = "CLASSIC"
	timed     = "TIMED"
	todo      = "TODO"
	completed = "COMPLETED"
	cancelled = "CANCELLED"
	idle      = "IDLE"
	running   = "RUNNING"
	paused    = "PAUSED"
	finished  = "FINISHED"

	maxSeconds = 7 * 24 * 3600
)

type task struct {
	ID          int64      `bson:"_id" json:"id"`
	Title       string     `bson:"title" json:"title"`
	Description *string    `bson:"description" json:"description"`
	Type        string     `bson:"type" json:"type"`
	Status      string     `bson:"status" json:"status"`
	Deadline    *time.Time `bson:"deadline" json:"deadline"`
	TimeoutAt   *time.Time `bson:"timeout_at" json:"timeout_at"`
	CreatedAt   time.Time  `bson:"created_at" json:"created_at"`
	FinishedAt  *time.Time `bson:"finished_at" json:"finished_at"`
	Timer       *timer     `bson:"timer,omitempty" json:"timer"`
	Revision    int64      `bson:"revision" json:"-"`
}

type timer struct {
	ID          int64      `bson:"_id" json:"id"`
	TaskID      *int64     `bson:"task_id" json:"task_id"`
	Name        *string    `bson:"name" json:"name"`
	State       string     `bson:"state" json:"state"`
	DurationMS  int64      `bson:"duration_ms" json:"duration_ms"`
	RemainingMS int64      `bson:"-" json:"remaining_ms"`
	ElapsedMS   int64      `bson:"elapsed_ms" json:"elapsed_ms,omitempty"`
	StartedAt   *time.Time `bson:"started_at" json:"started_at,omitempty"`
	ExpiresAt   *time.Time `bson:"expires_at" json:"-"`
	Revision    int64      `bson:"revision" json:"-"`
}

func newTimer(id int64, name *string, seconds int64) *timer {
	return &timer{ID: id, Name: name, DurationMS: seconds * 1000, State: idle}
}

func (t timer) elapsed(now time.Time) int64 {
	if t.State == running && t.StartedAt != nil {
		return t.ElapsedMS + max(now.Sub(*t.StartedAt).Milliseconds(), 0)
	}
	return t.ElapsedMS
}

func (t timer) response(now time.Time) *timer {
	t.RemainingMS = max(t.DurationMS-t.elapsed(now), 0)
	if t.State == finished {
		t.RemainingMS = 0
	}
	t.ElapsedMS, t.StartedAt = 0, nil
	return &t
}

func (t task) response(now time.Time) task {
	if t.Timer != nil {
		t.Timer = t.Timer.response(now)
	}
	return t
}

func (t *timer) setStart(now time.Time) {
	t.StartedAt = &now
	t.setExpiry()
}

func (t *timer) setExpiry() {
	t.ExpiresAt = nil
	if t.State == running && t.StartedAt != nil {
		end := t.StartedAt.Add(time.Duration(t.DurationMS-t.ElapsedMS) * time.Millisecond)
		t.ExpiresAt = &end
	}
}

func (t *timer) settle(parent *task, now time.Time) bool {
	if t.State != running || t.ExpiresAt == nil || t.ExpiresAt.After(now) {
		return false
	}
	if parent != nil && parent.Status == todo {
		parent.Status = completed
		parent.FinishedAt = t.ExpiresAt
	}
	t.State, t.ElapsedMS = finished, t.DurationMS
	t.StartedAt, t.ExpiresAt = nil, nil
	return true
}

func (t *timer) pause(now time.Time) {
	if t.State == running {
		t.ElapsedMS = t.elapsed(now)
		t.State, t.StartedAt, t.ExpiresAt = paused, nil, nil
	}
}

func (t *timer) reset(parent *task) {
	t.State, t.ElapsedMS, t.StartedAt, t.ExpiresAt = idle, 0, nil, nil
	if parent != nil && parent.Status == completed {
		parent.Status, parent.FinishedAt = todo, nil
	}
}

func (t *timer) action(parent *task, action string, now time.Time) error {
	switch action {
	case "start":
		if parent != nil && parent.Status != todo {
			return conflict("The task of this timer is no longer active")
		}
		if t.State == finished {
			return conflict("Timer is finished; reset it first")
		}
		if t.State != running {
			t.State = running
			t.setStart(now)
		}
	case "pause":
		t.pause(now)
	case "reset":
		t.reset(parent)
	}
	return nil
}

func (t *timer) setRemaining(parent *task, seconds int64, now time.Time) {
	if t.State == finished {
		t.reset(parent)
	}
	t.ElapsedMS = t.elapsed(now)
	t.DurationMS = t.ElapsedMS + seconds*1000
	if t.State == running {
		t.setStart(now)
	}
}

func (t *task) settle(now time.Time) bool {
	if t.Timer != nil {
		return t.Timer.settle(t, now)
	}
	if t.Type == classic && t.Status == todo && t.TimeoutAt != nil && !t.TimeoutAt.After(now) {
		t.Status, t.FinishedAt = cancelled, t.TimeoutAt
		return true
	}
	return false
}

func (t *task) action(action string, now time.Time) error {
	switch action {
	case "complete", "cancel":
		if t.Status != todo {
			return conflict("Task is not active")
		}
		t.Status, t.FinishedAt = completed, &now
		if action == "cancel" {
			t.Status = cancelled
		}
		if t.Timer != nil {
			t.Timer.pause(now)
		}
	case "reopen":
		if t.Status == todo {
			return conflict("Task is already active")
		}
		t.Status, t.FinishedAt, t.TimeoutAt = todo, nil, nil
		if t.Timer != nil {
			t.Timer.reset(t)
		}
	default:
		return fmt.Errorf("unknown task action %q", action)
	}
	return nil
}
