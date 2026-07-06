package fly

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type recordedRequest struct {
	Method string
	Path   string
	Auth   string
	Body   map[string]any
}

func newFakeServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, rec *recordedRequest)) (*httptest.Server, *[]recordedRequest) {
	t.Helper()
	var requests []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := recordedRequest{Method: r.Method, Path: r.URL.Path, Auth: r.Header.Get("Authorization")}
		if r.Body != nil {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			rec.Body = body
		}
		requests = append(requests, rec)
		handler(w, r, &requests[len(requests)-1])
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func TestEnsureApp_createsWhenMissing(t *testing.T) {
	srv, requests := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/apps/myapp-production":
			writeJSON(w, 404, map[string]any{"error": "not found"})
		case r.Method == http.MethodPost && r.URL.Path == "/apps":
			if rec.Body["app_name"] != "myapp-production" {
				t.Errorf("expected app_name in create body, got %v", rec.Body)
			}
			writeJSON(w, 201, map[string]any{"name": "myapp-production", "id": "app-1"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	c := New("tok")
	c.BaseURL = srv.URL
	if err := c.EnsureApp("myapp-production", ""); err != nil {
		t.Fatalf("EnsureApp: %v", err)
	}
	if len(*requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(*requests))
	}
	for _, r := range *requests {
		if r.Auth != "Bearer tok" {
			t.Errorf("missing bearer auth: %q", r.Auth)
		}
	}
}

func TestEnsureApp_noopWhenPresent(t *testing.T) {
	srv, requests := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/apps/myapp-production":
			writeJSON(w, 200, map[string]any{"name": "myapp-production", "id": "app-1"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	c := New("tok")
	c.BaseURL = srv.URL
	if err := c.EnsureApp("myapp-production", ""); err != nil {
		t.Fatalf("EnsureApp: %v", err)
	}
	if len(*requests) != 1 {
		t.Fatalf("expected 1 request (no create), got %d", len(*requests))
	}
}

func TestEnsureMachine_createsThenUpdates(t *testing.T) {
	var machineListCalls int
	srv, requests := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/apps/myapp/machines":
			machineListCalls++
			if machineListCalls == 1 {
				writeJSON(w, 200, []map[string]any{})
			} else {
				writeJSON(w, 200, []map[string]any{{"id": "m-1", "name": "api", "state": "started"}})
			}
		case r.Method == http.MethodPost && r.URL.Path == "/apps/myapp/machines":
			writeJSON(w, 201, map[string]any{"id": "m-1", "name": "api", "state": "created"})
		case r.Method == http.MethodPost && r.URL.Path == "/apps/myapp/machines/m-1":
			writeJSON(w, 200, map[string]any{"id": "m-1"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	c := New("tok")
	c.BaseURL = srv.URL

	id, err := c.EnsureMachine("myapp", "api", "registry.fly.io/myapp:api", MachineConfig{})
	if err != nil {
		t.Fatalf("EnsureMachine (create): %v", err)
	}
	if id != "m-1" {
		t.Fatalf("expected id m-1, got %q", id)
	}

	id, err = c.EnsureMachine("myapp", "api", "registry.fly.io/myapp:api2", MachineConfig{})
	if err != nil {
		t.Fatalf("EnsureMachine (update): %v", err)
	}
	if id != "m-1" {
		t.Fatalf("expected id m-1 on update, got %q", id)
	}

	var sawUpdate bool
	for _, r := range *requests {
		if r.Method == http.MethodPost && r.Path == "/apps/myapp/machines/m-1" {
			sawUpdate = true
			body, _ := json.Marshal(r.Body["config"])
			if !strings.Contains(string(body), "myapp:api2") {
				t.Errorf("expected update body to carry new image, got %s", body)
			}
		}
	}
	if !sawUpdate {
		t.Fatal("expected an update request for the existing machine")
	}
}

func TestWaitStarted_succeedsOnStarted(t *testing.T) {
	var poll int
	srv, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		poll++
		state := "starting"
		if poll >= 2 {
			state = "started"
		}
		writeJSON(w, 200, map[string]any{"id": "m-1", "state": state})
	})

	c := New("tok")
	c.BaseURL = srv.URL
	c.PollInterval = time.Millisecond

	if err := c.WaitStarted("myapp", "m-1", time.Second); err != nil {
		t.Fatalf("WaitStarted: %v", err)
	}
}

func TestWaitStarted_surfacesTerminalFailure(t *testing.T) {
	srv, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		writeJSON(w, 200, map[string]any{"id": "m-1", "state": "failed"})
	})

	c := New("tok")
	c.BaseURL = srv.URL
	c.PollInterval = time.Millisecond

	err := c.WaitStarted("myapp", "m-1", time.Second)
	if err == nil {
		t.Fatal("expected an error for a terminal failure state")
	}
}
