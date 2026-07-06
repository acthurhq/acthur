package coolify

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// fakeDeployContext implements DeployContext for target tests.
type fakeDeployContext struct {
	env         string
	compose     []byte
	projectName string
	host        string
	logs        []string
}

func (f *fakeDeployContext) EnvName() string     { return f.env }
func (f *fakeDeployContext) ComposeYAML() []byte { return f.compose }
func (f *fakeDeployContext) ProjectName() string { return f.projectName }
func (f *fakeDeployContext) Host() string        { return f.host }
func (f *fakeDeployContext) Log(format string, args ...any) {
	f.logs = append(f.logs, fmt.Sprintf(format, args...))
}

func TestTarget_Deploy_fullHappyPath(t *testing.T) {
	var pollCount int
	srv, requests := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects":
			writeJSON(w, 200, []map[string]any{})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects":
			writeJSON(w, 201, map[string]any{"uuid": "proj-1", "name": "myapp"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/applications":
			writeJSON(w, 200, []map[string]any{})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/applications/dockercompose":
			writeJSON(w, 201, map[string]any{"uuid": "app-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/deploy":
			writeJSON(w, 200, map[string]any{"deployments": []map[string]any{{"deployment_uuid": "dep-1"}}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/applications/app-1":
			pollCount++
			status := "starting"
			if pollCount >= 2 {
				status = "running:healthy"
			}
			writeJSON(w, 200, map[string]any{"uuid": "app-1", "status": status})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	target := NewTarget("secret-token")
	target.PollInterval = time.Millisecond
	target.HealthTimeout = time.Second

	ctx := &fakeDeployContext{
		env:         "production",
		compose:     []byte("services:\n  web:\n    image: nginx\n"),
		projectName: "myapp",
		host:        srv.URL,
	}

	if err := target.Deploy(ctx); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	if len(*requests) == 0 {
		t.Fatal("expected requests to be made")
	}
	if len(ctx.logs) == 0 {
		t.Fatal("expected Deploy to log progress via the DeployContext logger")
	}

	// Sanity: every request must have carried the auth token.
	for _, r := range *requests {
		if r.Auth != "Bearer secret-token" {
			t.Errorf("request %s %s missing bearer auth: %q", r.Method, r.Path, r.Auth)
		}
	}
}

func TestTarget_Deploy_surfacesFailedHealth(t *testing.T) {
	srv, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects":
			writeJSON(w, 200, []map[string]any{{"uuid": "proj-1", "name": "myapp"}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/applications":
			writeJSON(w, 200, []map[string]any{{"uuid": "app-1", "name": "myapp-production", "project_uuid": "proj-1"}})
		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/v1/applications/"):
			writeJSON(w, 200, map[string]any{"uuid": "app-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/deploy":
			writeJSON(w, 200, map[string]any{"deployments": []map[string]any{{"deployment_uuid": "dep-1"}}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/applications/app-1":
			writeJSON(w, 200, map[string]any{"uuid": "app-1", "status": "exited:unhealthy"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	target := NewTarget("tok")
	target.PollInterval = time.Millisecond
	target.HealthTimeout = time.Second

	ctx := &fakeDeployContext{
		env:         "production",
		compose:     []byte("services:\n  web:\n    image: nginx\n"),
		projectName: "myapp",
		host:        srv.URL,
	}

	err := target.Deploy(ctx)
	if err == nil {
		t.Fatal("expected Deploy to surface the failed health check")
	}
	if !strings.Contains(err.Error(), "exited:unhealthy") {
		t.Errorf("expected error to mention the failing status, got: %v", err)
	}
}
