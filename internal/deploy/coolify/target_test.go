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
	serverUUID  string
	logs        []string
	workloadEnv map[string]string
}

func (f *fakeDeployContext) EnvName() string                { return f.env }
func (f *fakeDeployContext) ComposeYAML() []byte            { return f.compose }
func (f *fakeDeployContext) ProjectName() string            { return f.projectName }
func (f *fakeDeployContext) Host() string                   { return f.host }
func (f *fakeDeployContext) ServerUUID() string             { return f.serverUUID }
func (f *fakeDeployContext) WorkloadEnv() map[string]string { return f.workloadEnv }
func (f *fakeDeployContext) Log(format string, args ...any) {
	f.logs = append(f.logs, fmt.Sprintf(format, args...))
}

func TestTarget_DeployUpsertsEnvironmentBeforeDeploymentWithoutLeakingValues(t *testing.T) {
	const secret = "super-secret-runtime-value"
	var sequence []string
	srv, requests := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		sequence = append(sequence, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects":
			writeJSON(w, 200, []map[string]any{{"uuid": "proj-1", "name": "myapp"}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/services":
			writeJSON(w, 200, []map[string]any{{"uuid": "app-1", "name": "myapp-production", "project_uuid": "proj-1"}})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/services/app-1":
			writeJSON(w, 200, map[string]any{"uuid": "app-1"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/services/app-1/envs":
			writeJSON(w, 200, []map[string]any{{"key": "APP_SECRET"}})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/services/app-1/envs":
			if rec.Body["key"] != "APP_SECRET" || rec.Body["value"] != secret {
				t.Fatalf("environment request = %#v", rec.Body)
			}
			writeJSON(w, 201, map[string]any{"message": "Environment variable updated."})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/services/app-1/start":
			writeJSON(w, 200, map[string]any{"message": "Service starting request queued."})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/services/app-1":
			writeJSON(w, 200, map[string]any{"uuid": "app-1", "status": "running:healthy"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := &fakeDeployContext{env: "production", compose: []byte("services:\n  api:\n    environment:\n      APP_SECRET: ${APP_SECRET}\n"), projectName: "myapp", host: srv.URL, serverUUID: "server-1", workloadEnv: map[string]string{"APP_SECRET": secret}}
	target := NewTarget("token")
	target.PollInterval = time.Millisecond
	if err := target.Deploy(ctx); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(sequence, "\n")
	if strings.Index(joined, "/envs") > strings.Index(joined, "/api/v1/services/app-1/start") {
		t.Fatalf("environment must be upserted before deployment:\n%s", joined)
	}
	if strings.Contains(string(ctx.compose), secret) || strings.Contains(strings.Join(ctx.logs, "\n"), secret) {
		t.Fatal("secret leaked into compose or logs")
	}
	_ = requests
}

func TestTarget_Deploy_fullHappyPath(t *testing.T) {
	var pollCount int
	srv, requests := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects":
			writeJSON(w, 200, []map[string]any{})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects":
			writeJSON(w, 201, map[string]any{"uuid": "proj-1", "name": "myapp"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/services":
			writeJSON(w, 200, []map[string]any{})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/services":
			if rec.Body["server_uuid"] != "server-1" || rec.Body["environment_name"] != "production" {
				t.Fatalf("current service create body: %#v", rec.Body)
			}
			writeJSON(w, 201, map[string]any{"uuid": "app-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/services/app-1/start":
			writeJSON(w, 200, map[string]any{"message": "Service starting request queued."})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/services/app-1":
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
		serverUUID:  "server-1",
	}

	if err := target.Deploy(ctx); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	if len(*requests) == 0 {
		t.Fatal("expected requests to be made")
	}
	var gotSequence []string
	for _, request := range *requests {
		gotSequence = append(gotSequence, request.Method+" "+request.Path)
	}
	wantSequence := []string{
		"GET /api/v1/projects",
		"POST /api/v1/projects",
		"GET /api/v1/services",
		"POST /api/v1/services",
		"POST /api/v1/services/app-1/start",
		"GET /api/v1/services/app-1",
		"GET /api/v1/services/app-1",
	}
	if strings.Join(gotSequence, "\n") != strings.Join(wantSequence, "\n") {
		t.Fatalf("current service request sequence:\n%s\nwant:\n%s", strings.Join(gotSequence, "\n"), strings.Join(wantSequence, "\n"))
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

func TestTarget_DeployRequiresServerUUIDBeforeNetwork(t *testing.T) {
	ctx := &fakeDeployContext{env: "production", projectName: "myapp", host: "http://127.0.0.1:1"}
	err := NewTarget("token").Deploy(ctx)
	if err == nil || !strings.Contains(err.Error(), "server UUID is required") {
		t.Fatalf("expected pointed service server UUID failure, got: %v", err)
	}
}

func TestTarget_Deploy_surfacesFailedHealth(t *testing.T) {
	srv, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects":
			writeJSON(w, 200, []map[string]any{{"uuid": "proj-1", "name": "myapp"}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/services":
			writeJSON(w, 200, []map[string]any{{"uuid": "app-1", "name": "myapp-production", "project_uuid": "proj-1"}})
		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/v1/services/"):
			writeJSON(w, 200, map[string]any{"uuid": "app-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/services/app-1/start":
			writeJSON(w, 200, map[string]any{"message": "Service starting request queued."})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/services/app-1":
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
		serverUUID:  "server-1",
	}

	err := target.Deploy(ctx)
	if err == nil {
		t.Fatal("expected Deploy to surface the failed health check")
	}
	if !strings.Contains(err.Error(), "exited:unhealthy") {
		t.Errorf("expected error to mention the failing status, got: %v", err)
	}
}
