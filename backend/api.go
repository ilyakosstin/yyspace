package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type failure struct {
	code    int
	message string
}

func (e failure) Error() string     { return e.message }
func invalid(message string) error  { return failure{http.StatusUnprocessableEntity, message} }
func conflict(message string) error { return failure{http.StatusConflict, message} }
func notFound(kind string) error    { return failure{http.StatusNotFound, kind + " not found"} }

func writeJSON(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, err error) {
	var e failure
	if !errors.As(err, &e) {
		log.Printf("request: %v", err)
		e = failure{http.StatusInternalServerError, "Internal server error"}
	}
	writeJSON(w, e.code, map[string]string{"detail": e.message})
}

type field[T any] struct {
	present bool
	value   *T
}

func (f *field[T]) UnmarshalJSON(data []byte) error {
	f.present = true
	return json.Unmarshal(data, &f.value)
}

type taskInput struct {
	Title       field[string]    `json:"title"`
	Description field[string]    `json:"description"`
	Type        field[string]    `json:"type"`
	Deadline    field[time.Time] `json:"deadline"`
	Timeout     field[int64]     `json:"timeout_seconds"`
	Duration    field[int64]     `json:"duration_seconds"`
}

type timerInput struct {
	Name      field[string] `json:"name"`
	Duration  field[int64]  `json:"duration_seconds"`
	Remaining field[int64]  `json:"remaining_seconds"`
}

func decode[T any](w http.ResponseWriter, r *http.Request) (*T, error) {
	body := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
	var value *T
	if err := body.Decode(&value); err != nil || value == nil {
		return nil, invalid("Invalid request body")
	}
	if err := body.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, invalid("Invalid request body")
	}
	return value, nil
}

func tooLong(value *string, limit int) bool {
	return value != nil && utf8.RuneCountInString(*value) > limit
}

func checkSeconds(seconds *int64) bool {
	return seconds == nil || (*seconds > 0 && *seconds <= maxSeconds)
}

func (body *taskInput) apply(t *task, now time.Time) error {
	if body.Title.present {
		if body.Title.value == nil || strings.TrimSpace(*body.Title.value) == "" {
			return invalid("Title must not be blank")
		}
		if tooLong(body.Title.value, 4000) {
			return invalid("Title is too long")
		}
		t.Title = strings.TrimSpace(*body.Title.value)
	}
	if body.Description.present {
		if tooLong(body.Description.value, 200000) {
			return invalid("Description is too long")
		}
		t.Description = nil
		if body.Description.value != nil {
			if value := strings.TrimSpace(*body.Description.value); value != "" {
				t.Description = &value
			}
		}
	}
	if body.Deadline.present {
		t.Deadline = nil
		if body.Deadline.value != nil {
			deadline := body.Deadline.value.UTC().Truncate(time.Millisecond)
			if !deadline.After(now) {
				return invalid("Deadline must be in the future")
			}
			t.Deadline = &deadline
		}
	}
	if body.Timeout.present {
		if t.Type != classic {
			return invalid("Only CLASSIC tasks can have a timeout")
		}
		if t.Status != todo {
			return conflict("Task is not active")
		}
		if !checkSeconds(body.Timeout.value) {
			return invalid("Timeout must be between one second and seven days")
		}
		t.TimeoutAt = nil
		if body.Timeout.value != nil {
			timeout := now.Add(time.Duration(*body.Timeout.value) * time.Second)
			t.TimeoutAt = &timeout
		}
	}
	return nil
}

func (body *taskInput) build(now time.Time) (*task, error) {
	t := &task{Type: classic, Status: todo, CreatedAt: now}
	if !body.Title.present {
		return nil, invalid("Title is required")
	}
	if body.Type.present {
		if body.Type.value == nil {
			return nil, invalid("Invalid task type")
		}
		t.Type = *body.Type.value
	}
	switch t.Type {
	case timed:
		if body.Duration.value == nil || !checkSeconds(body.Duration.value) {
			return nil, invalid("Timed tasks require a duration between one second and seven days")
		}
		if body.Timeout.value != nil {
			return nil, invalid("Timed tasks cannot have a timeout")
		}
		body.Timeout.present = false
		t.Timer = newTimer(0, nil, *body.Duration.value)
	case classic:
		if body.Duration.value != nil {
			return nil, invalid("Only timed tasks can have a duration")
		}
	default:
		return nil, invalid("Invalid task type")
	}
	return t, body.apply(t, now)
}

func (body *timerInput) apply(t *timer, parent *task, now time.Time) error {
	if tooLong(body.Name.value, 1000) {
		return invalid("Name is too long")
	}
	if !checkSeconds(body.Remaining.value) {
		return invalid("Remaining time must be between one second and seven days")
	}
	if body.Name.present {
		t.Name = body.Name.value
	}
	if body.Remaining.value != nil {
		t.setRemaining(parent, *body.Remaining.value, now)
	}
	return nil
}

type operation func(http.ResponseWriter, *http.Request, time.Time) (any, error)

func (s *store) route(code int, handle operation) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UTC().Truncate(time.Millisecond)
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if err := s.sync(ctx, now); err != nil {
			writeError(w, err)
			return
		}
		value, err := handle(w, r.WithContext(ctx), now)
		if err != nil {
			writeError(w, err)
			return
		}
		if code == http.StatusNoContent {
			w.WriteHeader(code)
			return
		}
		writeJSON(w, code, value)
	}
}

func requestID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, invalid("Invalid ID")
	}
	return id, nil
}

func (s *store) taskRoute(code int, edit func(http.ResponseWriter, *http.Request, *task, time.Time) error) http.HandlerFunc {
	return s.route(code, func(w http.ResponseWriter, r *http.Request, now time.Time) (any, error) {
		id, err := requestID(r)
		if err != nil {
			return nil, err
		}
		var body *taskInput
		if r.Method == http.MethodPatch {
			body, err = decode[taskInput](w, r)
			if err != nil {
				return nil, err
			}
		}
		t, err := change(r.Context(), s.tasks, bson.M{"_id": id}, taskIdentity, func(t *task) (bool, error) {
			if body != nil {
				return false, body.apply(t, now)
			}
			return code == http.StatusNoContent, edit(w, r, t, now)
		})
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, notFound("Task")
		}
		return t.response(now), err
	})
}

func (s *store) timerRoute(code int, action string) http.HandlerFunc {
	return s.route(code, func(w http.ResponseWriter, r *http.Request, now time.Time) (any, error) {
		id, err := requestID(r)
		if err != nil {
			return nil, err
		}
		var body *timerInput
		if r.Method == http.MethodPatch {
			body, err = decode[timerInput](w, r)
			if err != nil {
				return nil, err
			}
		}
		edit := func(t *timer, parent *task) (bool, error) {
			if code == http.StatusNoContent {
				if parent != nil {
					return false, conflict("This timer belongs to a task; delete the task instead")
				}
				return true, nil
			}
			if body != nil {
				return false, body.apply(t, parent, now)
			}
			return false, t.action(parent, action, now)
		}
		t, err := change(r.Context(), s.timers, bson.M{"_id": id}, timerIdentity, func(t *timer) (bool, error) {
			return edit(t, nil)
		})
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return t.response(now), err
		}
		parent, err := change(r.Context(), s.tasks, bson.M{"timer._id": id}, taskIdentity, func(t *task) (bool, error) {
			return edit(t.Timer, t)
		})
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, notFound("Timer")
		}
		if err != nil {
			return nil, err
		}
		return parent.Timer.response(now), nil
	})
}

func (s *store) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.tasks.Database().Client().Ping(ctx, nil); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/state", s.route(http.StatusOK, func(_ http.ResponseWriter, r *http.Request, now time.Time) (any, error) {
		return s.state(r.Context(), now)
	}))
	mux.HandleFunc("POST /api/tasks", s.route(http.StatusCreated, func(w http.ResponseWriter, r *http.Request, now time.Time) (any, error) {
		body, err := decode[taskInput](w, r)
		if err != nil {
			return nil, err
		}
		t, err := body.build(now)
		if err != nil {
			return nil, err
		}
		err = s.createTask(r.Context(), t)
		return t.response(now), err
	}))
	mux.HandleFunc("PATCH /api/tasks/{id}", s.taskRoute(http.StatusOK, nil))
	mux.HandleFunc("DELETE /api/tasks/{id}", s.taskRoute(http.StatusNoContent,
		func(http.ResponseWriter, *http.Request, *task, time.Time) error { return nil }))
	for _, action := range []string{"complete", "cancel", "reopen"} {
		mux.HandleFunc("POST /api/tasks/{id}/"+action, s.taskRoute(http.StatusOK,
			func(_ http.ResponseWriter, _ *http.Request, t *task, now time.Time) error {
				return t.action(action, now)
			}))
	}
	mux.HandleFunc("POST /api/timers", s.route(http.StatusCreated, func(w http.ResponseWriter, r *http.Request, now time.Time) (any, error) {
		body, err := decode[timerInput](w, r)
		if err != nil {
			return nil, err
		}
		if body.Duration.value == nil || !checkSeconds(body.Duration.value) {
			return nil, invalid("Duration must be between one second and seven days")
		}
		if tooLong(body.Name.value, 1000) {
			return nil, invalid("Name is too long")
		}
		t := newTimer(0, body.Name.value, *body.Duration.value)
		err = s.createTimer(r.Context(), t)
		return t.response(now), err
	}))
	mux.HandleFunc("PATCH /api/timers/{id}", s.timerRoute(http.StatusOK, ""))
	mux.HandleFunc("DELETE /api/timers/{id}", s.timerRoute(http.StatusNoContent, ""))
	for _, action := range []string{"start", "pause", "reset"} {
		mux.HandleFunc("POST /api/timers/{id}/"+action, s.timerRoute(http.StatusOK, action))
	}
	mux.HandleFunc("GET /api/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(openAPISpec)
	})
	mux.HandleFunc("GET /api/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, apiDocs)
	})
	return mux
}

//go:embed openapi.json
var openAPISpec []byte

const apiDocs = `<!doctype html><html lang="en"><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Focus Helper API</title><link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
<div id="swagger-ui"></div><script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>SwaggerUIBundle({url: "/api/openapi.json", dom_id: "#swagger-ui", validatorUrl: null})</script></html>`
