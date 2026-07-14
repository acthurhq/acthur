package coolify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// recordedRequest captures enough of an inbound request for assertions.
type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Body   map[string]any
}

func newFakeServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, rec *recordedRequest)) (*httptest.Server, *[]recordedRequest) {
	t.Helper()
	var requests []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := recordedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Auth:   r.Header.Get("Authorization"),
		}
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

func TestNew_setsBaseURLAndToken(t *testing.T) {
	c := New("https://coolify.example.com", "tok-123")
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestEnsureComposeServiceCreatesCurrentServiceShape(t *testing.T) {
	srv, requests := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/services":
			writeJSON(w, 200, []map[string]any{})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/services":
			writeJSON(w, 201, map[string]any{"uuid": "service-1"})
		default:
			t.Fatalf("legacy/unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	uuid, err := New(srv.URL, "tok").EnsureComposeService("project-1", "server-1", "production", "myapp-production", "services: {}")
	if err != nil {
		t.Fatal(err)
	}
	if uuid != "service-1" {
		t.Fatalf("uuid = %q", uuid)
	}
	body := (*requests)[1].Body
	for key, want := range map[string]any{"project_uuid": "project-1", "server_uuid": "server-1", "environment_name": "production", "name": "myapp-production", "docker_compose_raw": "services: {}"} {
		if body[key] != want {
			t.Fatalf("create body[%s] = %#v, want %#v; body=%#v", key, body[key], want, body)
		}
	}
}

func TestUpsertServiceEnv_ErrorDoesNotExposeValue(t *testing.T) {
	const secret = "should-never-appear-in-error"
	srv, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": "invalid value " + secret})
	})
	err := New(srv.URL, "tok").UpsertServiceEnv("app-1", "APP_SECRET", secret)
	if err == nil {
		t.Fatal("expected upsert error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("secret leaked in error: %v", err)
	}
	if !strings.Contains(err.Error(), "APP_SECRET") || !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("expected pointed key/status error, got: %v", err)
	}
}

func TestEnsureProject_authHeaderPresent(t *testing.T) {
	srv, requests := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects":
			writeJSON(w, 200, []map[string]any{})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects":
			writeJSON(w, 201, map[string]any{"uuid": "proj-uuid-1", "name": "myapp"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	c := New(srv.URL, "secret-token")
	uuid, err := c.EnsureProject("myapp")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if uuid != "proj-uuid-1" {
		t.Fatalf("expected uuid proj-uuid-1, got %q", uuid)
	}

	for _, r := range *requests {
		if r.Auth != "Bearer secret-token" {
			t.Errorf("request %s %s: expected Bearer auth header, got %q", r.Method, r.Path, r.Auth)
		}
	}
}

func TestEnsureProject_createsWhenAbsent(t *testing.T) {
	var createCalled bool
	srv, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects":
			writeJSON(w, 200, []map[string]any{
				{"uuid": "other-uuid", "name": "other-project"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects":
			createCalled = true
			if rec.Body["name"] != "myapp" {
				t.Errorf("expected name=myapp in create body, got %v", rec.Body)
			}
			writeJSON(w, 201, map[string]any{"uuid": "new-proj-uuid", "name": "myapp"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	c := New(srv.URL, "tok")
	uuid, err := c.EnsureProject("myapp")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if !createCalled {
		t.Fatal("expected create request when project absent")
	}
	if uuid != "new-proj-uuid" {
		t.Fatalf("expected new-proj-uuid, got %q", uuid)
	}
}

func TestEnsureProject_reusesWhenPresent(t *testing.T) {
	var createCalled bool
	srv, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects":
			writeJSON(w, 200, []map[string]any{
				{"uuid": "existing-uuid", "name": "myapp"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects":
			createCalled = true
			writeJSON(w, 201, map[string]any{"uuid": "should-not-happen"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	c := New(srv.URL, "tok")
	uuid, err := c.EnsureProject("myapp")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if createCalled {
		t.Fatal("did not expect create request when project already exists")
	}
	if uuid != "existing-uuid" {
		t.Fatalf("expected existing-uuid, got %q", uuid)
	}
}

func TestEnsureComposeService_createsWhenAbsent(t *testing.T) {
	var createBody map[string]any
	srv, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/services":
			writeJSON(w, 200, []map[string]any{})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/services":
			createBody = rec.Body
			writeJSON(w, 201, map[string]any{"uuid": "app-uuid-1"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	c := New(srv.URL, "tok")
	uuid, err := c.EnsureComposeService("proj-1", "server-1", "production", "web", "services:\n  web:\n    image: nginx\n")
	if err != nil {
		t.Fatalf("EnsureComposeService: %v", err)
	}
	if uuid != "app-uuid-1" {
		t.Fatalf("expected app-uuid-1, got %q", uuid)
	}
	if createBody["project_uuid"] != "proj-1" {
		t.Errorf("expected project_uuid=proj-1 in create body, got %v", createBody)
	}
	if createBody["name"] != "web" {
		t.Errorf("expected name=web in create body, got %v", createBody)
	}
	if !strings.Contains(createBody["docker_compose_raw"].(string), "nginx") {
		t.Errorf("expected docker_compose_raw to contain compose YAML, got %v", createBody["docker_compose_raw"])
	}
}

func TestEnsureComposeService_updatesWhenPresent(t *testing.T) {
	var patchCalled bool
	var createCalled bool
	srv, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/services":
			writeJSON(w, 200, []map[string]any{
				{"uuid": "app-existing", "name": "web", "project_uuid": "proj-1"},
			})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/services/app-existing":
			patchCalled = true
			if rec.Body["docker_compose_raw"] == nil {
				t.Errorf("expected docker_compose_raw in patch body, got %v", rec.Body)
			}
			writeJSON(w, 200, map[string]any{"uuid": "app-existing"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/services":
			createCalled = true
			writeJSON(w, 201, map[string]any{"uuid": "should-not-happen"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	c := New(srv.URL, "tok")
	uuid, err := c.EnsureComposeService("proj-1", "server-1", "production", "web", "services:\n  web:\n    image: nginx\n")
	if err != nil {
		t.Fatalf("EnsureComposeService: %v", err)
	}
	if createCalled {
		t.Fatal("did not expect create request when app already exists")
	}
	if !patchCalled {
		t.Fatal("expected patch request to update existing app")
	}
	if uuid != "app-existing" {
		t.Fatalf("expected app-existing, got %q", uuid)
	}
}

func TestStartService_triggersDeployment(t *testing.T) {
	srv, requests := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/services/app-uuid-1/start" {
			writeJSON(w, 200, map[string]any{"message": "Service starting request queued."})
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	c := New(srv.URL, "tok")
	err := c.StartService("app-uuid-1")
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if len(*requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(*requests))
	}
}

func TestWaitServiceHealthy_pollsUntilRunning(t *testing.T) {
	var pollCount int
	srv, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/services/app-uuid-1" {
			pollCount++
			status := "starting"
			if pollCount >= 3 {
				status = "running:healthy"
			}
			writeJSON(w, 200, map[string]any{"uuid": "app-uuid-1", "status": status})
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	c := New(srv.URL, "tok")
	c.PollInterval = time.Millisecond
	err := c.WaitServiceHealthy("app-uuid-1", time.Second)
	if err != nil {
		t.Fatalf("WaitServiceHealthy: %v", err)
	}
	if pollCount < 3 {
		t.Fatalf("expected at least 3 polls, got %d", pollCount)
	}
}

func TestWaitServiceHealthy_pointedErrorOnFailedStatus(t *testing.T) {
	srv, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		writeJSON(w, 200, map[string]any{"uuid": "app-uuid-1", "status": "exited:unhealthy"})
	})

	c := New(srv.URL, "tok")
	c.PollInterval = time.Millisecond
	err := c.WaitServiceHealthy("app-uuid-1", time.Second)
	if err == nil {
		t.Fatal("expected error for failed deployment status")
	}
	if !strings.Contains(err.Error(), "exited:unhealthy") {
		t.Errorf("expected error to mention the failing status, got: %v", err)
	}
}

func TestWaitServiceHealthy_timesOut(t *testing.T) {
	srv, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		writeJSON(w, 200, map[string]any{"uuid": "app-uuid-1", "status": "starting"})
	})

	c := New(srv.URL, "tok")
	c.PollInterval = time.Millisecond
	err := c.WaitServiceHealthy("app-uuid-1", 20*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") && !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected timeout wording in error, got: %v", err)
	}
}

func TestNonSuccessResponse_surfacesBodyInError(t *testing.T) {
	srv, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"invalid token"}`))
	})

	c := New(srv.URL, "bad-token")
	_, err := c.EnsureProject("myapp")
	if err == nil {
		t.Fatal("expected error on 401 response")
	}
	if !strings.Contains(err.Error(), "invalid token") {
		t.Errorf("expected response body to be surfaced in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected status code to be surfaced in error, got: %v", err)
	}
}
