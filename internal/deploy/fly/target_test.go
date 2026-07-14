package fly

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

type fakeDeployContext struct {
	env      string
	project  string
	root     string
	services []ServiceArtifact
	logs     []string
}

func (f *fakeDeployContext) EnvName() string             { return f.env }
func (f *fakeDeployContext) ProjectName() string         { return f.project }
func (f *fakeDeployContext) Root() string                { return f.root }
func (f *fakeDeployContext) Services() []ServiceArtifact { return f.services }
func (f *fakeDeployContext) Log(format string, args ...any) {
	f.logs = append(f.logs, fmt.Sprintf(format, args...))
}

func fakeRunner(calls *[]string) func(args ...string) (string, error) {
	return func(args ...string) (string, error) {
		*calls = append(*calls, strings.Join(args, " "))
		return "", nil
	}
}

func TestTarget_Deploy_fullHappyPath(t *testing.T) {
	var pollCount int
	srv, requests := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/apps/myapp-production":
			writeJSON(w, 404, map[string]any{"error": "not found"})
		case r.Method == http.MethodPost && r.URL.Path == "/apps":
			writeJSON(w, 201, map[string]any{"name": "myapp-production"})
		case r.Method == http.MethodGet && r.URL.Path == "/apps/myapp-production/machines":
			writeJSON(w, 200, []map[string]any{})
		case r.Method == http.MethodPost && r.URL.Path == "/apps/myapp-production/machines":
			writeJSON(w, 201, map[string]any{"id": "m-1", "state": "created"})
		case r.Method == http.MethodGet && r.URL.Path == "/apps/myapp-production/machines/m-1":
			pollCount++
			state := "starting"
			if pollCount >= 2 {
				state = "started"
			}
			writeJSON(w, 200, map[string]any{"id": "m-1", "state": state})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	var runCalls []string
	target := NewTarget("secret-token", fakeRunner(&runCalls))
	target.BaseURL = srv.URL
	target.PollInterval = time.Millisecond
	target.StartTimeout = time.Second

	ctx := &fakeDeployContext{
		env:     "production",
		project: "myapp",
		root:    "/tmp/project",
		services: []ServiceArtifact{
			{NodeID: "api", DockerfilePath: "deploy/Dockerfile.api", ContextDir: "api", Port: 8080},
		},
	}

	if err := target.Deploy(ctx); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if len(*requests) == 0 {
		t.Fatal("expected requests to be made")
	}
	for _, r := range *requests {
		if r.Auth != "Bearer secret-token" {
			t.Errorf("request %s %s missing bearer auth: %q", r.Method, r.Path, r.Auth)
		}
	}
	if len(runCalls) != 3 { // login + build + push
		t.Fatalf("expected login+build+push docker calls, got %v", runCalls)
	}
	if !strings.Contains(runCalls[0], "login registry.fly.io") {
		t.Errorf("expected first docker call to be login, got %q", runCalls[0])
	}
	if len(ctx.logs) == 0 {
		t.Fatal("expected Deploy to log progress via the DeployContext logger")
	}
}

func TestTarget_Deploy_requiresRunner(t *testing.T) {
	target := &Target{Token: "tok"}
	ctx := &fakeDeployContext{env: "production", project: "myapp", root: "/tmp/project"}
	if err := target.Deploy(ctx); err == nil {
		t.Fatal("expected an error when no docker Runner is configured")
	}
}

func TestTarget_Deploy_surfacesTerminalFailure(t *testing.T) {
	srv, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/apps/myapp-production":
			writeJSON(w, 200, map[string]any{"name": "myapp-production"})
		case r.Method == http.MethodGet && r.URL.Path == "/apps/myapp-production/machines":
			writeJSON(w, 200, []map[string]any{})
		case r.Method == http.MethodPost && r.URL.Path == "/apps/myapp-production/machines":
			writeJSON(w, 201, map[string]any{"id": "m-1", "state": "created"})
		case r.Method == http.MethodGet && r.URL.Path == "/apps/myapp-production/machines/m-1":
			writeJSON(w, 200, map[string]any{"id": "m-1", "state": "failed"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	var runCalls []string
	target := NewTarget("tok", fakeRunner(&runCalls))
	target.BaseURL = srv.URL
	target.PollInterval = time.Millisecond
	target.StartTimeout = time.Second

	ctx := &fakeDeployContext{
		env:     "production",
		project: "myapp",
		root:    "/tmp/project",
		services: []ServiceArtifact{
			{NodeID: "api", DockerfilePath: "deploy/Dockerfile.api", ContextDir: "api", Port: 8080},
		},
	}

	err := target.Deploy(ctx)
	if err == nil {
		t.Fatal("expected Deploy to surface the terminal machine failure")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Errorf("expected error to mention the failure state, got: %v", err)
	}
}
