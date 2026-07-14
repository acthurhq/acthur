package render

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
		case r.Method == http.MethodGet && r.URL.Path == "/owners":
			writeJSON(w, 200, []map[string]any{{"owner": map[string]any{"id": "usr-1"}}})
		case r.Method == http.MethodGet && r.URL.Path == "/services":
			writeJSON(w, 200, []map[string]any{})
		case r.Method == http.MethodPost && r.URL.Path == "/services":
			writeJSON(w, 201, map[string]any{"id": "srv-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/services/srv-1/deploys":
			writeJSON(w, 201, map[string]any{"id": "dep-1", "status": "build_in_progress"})
		case r.Method == http.MethodGet && r.URL.Path == "/services/srv-1/deploys/dep-1":
			pollCount++
			status := "build_in_progress"
			if pollCount >= 2 {
				status = "live"
			}
			writeJSON(w, 200, map[string]any{"id": "dep-1", "status": status})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	var runCalls []string
	target := NewTarget("secret-token", "ghcr.io/acme", fakeRunner(&runCalls))
	target.BaseURL = srv.URL
	target.PollInterval = time.Millisecond
	target.DeployTimeout = time.Second

	ctx := &fakeDeployContext{
		env:     "production",
		project: "myapp",
		root:    "/tmp/project",
		services: []ServiceArtifact{
			{NodeID: "api", DockerfilePath: "deploy/Dockerfile.api", ContextDir: "api"},
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
	if len(runCalls) != 2 {
		t.Fatalf("expected build+push docker calls, got %v", runCalls)
	}
	if len(ctx.logs) == 0 {
		t.Fatal("expected Deploy to log progress")
	}
}

func TestTarget_Deploy_requiresRegistry(t *testing.T) {
	target := &Target{Token: "tok", Run: fakeRunner(&[]string{})}
	ctx := &fakeDeployContext{env: "production", project: "myapp", root: "/tmp/project"}
	err := target.Deploy(ctx)
	if err == nil || !strings.Contains(err.Error(), "registry") {
		t.Fatalf("expected registry error, got: %v", err)
	}
}

func TestTarget_Deploy_requiresRunner(t *testing.T) {
	target := &Target{Token: "tok", Registry: "ghcr.io/acme"}
	ctx := &fakeDeployContext{env: "production", project: "myapp", root: "/tmp/project"}
	if err := target.Deploy(ctx); err == nil {
		t.Fatal("expected an error when no docker Runner is configured")
	}
}

func TestTarget_Deploy_surfacesFailedDeploy(t *testing.T) {
	srv, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/owners":
			writeJSON(w, 200, []map[string]any{{"owner": map[string]any{"id": "usr-1"}}})
		case r.Method == http.MethodGet && r.URL.Path == "/services":
			writeJSON(w, 200, []map[string]any{{"service": map[string]any{"id": "srv-1", "name": "myapp-production-api"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/services/srv-1/deploys":
			writeJSON(w, 201, map[string]any{"id": "dep-1", "status": "build_in_progress"})
		case r.Method == http.MethodGet && r.URL.Path == "/services/srv-1/deploys/dep-1":
			writeJSON(w, 200, map[string]any{"id": "dep-1", "status": "build_failed"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	var runCalls []string
	target := NewTarget("tok", "ghcr.io/acme", fakeRunner(&runCalls))
	target.BaseURL = srv.URL
	target.PollInterval = time.Millisecond
	target.DeployTimeout = time.Second

	ctx := &fakeDeployContext{
		env:     "production",
		project: "myapp",
		root:    "/tmp/project",
		services: []ServiceArtifact{
			{NodeID: "api", DockerfilePath: "deploy/Dockerfile.api", ContextDir: "api"},
		},
	}

	err := target.Deploy(ctx)
	if err == nil {
		t.Fatal("expected Deploy to surface the failed deploy status")
	}
	if !strings.Contains(err.Error(), "build_failed") {
		t.Errorf("expected error to mention build_failed, got: %v", err)
	}
}
