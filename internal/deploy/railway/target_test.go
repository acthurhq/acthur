package railway

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
	var serviceExists bool
	srv, requests := newFakeServer(t, func(w http.ResponseWriter, req gqlRequest, rec *recordedRequest) {
		switch {
		case strings.Contains(req.Query, "query { projects {"):
			writeGQL(w, map[string]any{"projects": map[string]any{"edges": []any{}}})
		case strings.Contains(req.Query, "projectCreate"):
			writeGQL(w, map[string]any{"projectCreate": map[string]any{"id": "proj-1"}})
		case strings.Contains(req.Query, "project(id:"):
			var services []map[string]any
			if serviceExists {
				services = []map[string]any{{"node": map[string]any{"id": "svc-1", "name": "api"}}}
			}
			writeGQL(w, map[string]any{"project": map[string]any{
				"environments": map[string]any{"edges": []map[string]any{{"node": map[string]any{"id": "env-1", "name": "production"}}}},
				"services":     map[string]any{"edges": services},
			}})
		case strings.Contains(req.Query, "serviceCreate"):
			serviceExists = true
			writeGQL(w, map[string]any{"serviceCreate": map[string]any{"id": "svc-1"}})
		case strings.Contains(req.Query, "serviceInstanceDeploy"):
			writeGQL(w, map[string]any{"serviceInstanceDeploy": true})
		case strings.Contains(req.Query, "deployments("):
			pollCount++
			status := "BUILDING"
			if pollCount >= 2 {
				status = "SUCCESS"
			}
			writeGQL(w, map[string]any{"deployments": map[string]any{"edges": []map[string]any{
				{"node": map[string]any{"id": "dep-1", "status": status}},
			}}})
		default:
			t.Fatalf("unexpected query: %s", req.Query)
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
			t.Errorf("request missing bearer auth: %q", r.Auth)
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

func TestTarget_Deploy_surfacesFailedDeployment(t *testing.T) {
	srv, _ := newFakeServer(t, func(w http.ResponseWriter, req gqlRequest, rec *recordedRequest) {
		switch {
		case strings.Contains(req.Query, "query { projects {"):
			writeGQL(w, map[string]any{"projects": map[string]any{"edges": []map[string]any{{"node": map[string]any{"id": "proj-1", "name": "myapp"}}}}})
		case strings.Contains(req.Query, "project(id:"):
			writeGQL(w, map[string]any{"project": map[string]any{
				"environments": map[string]any{"edges": []map[string]any{{"node": map[string]any{"id": "env-1", "name": "production"}}}},
				"services":     map[string]any{"edges": []map[string]any{{"node": map[string]any{"id": "svc-1", "name": "api"}}}},
			}})
		case strings.Contains(req.Query, "serviceInstanceUpdate"):
			writeGQL(w, map[string]any{"serviceInstanceUpdate": true})
		case strings.Contains(req.Query, "serviceInstanceDeploy"):
			writeGQL(w, map[string]any{"serviceInstanceDeploy": true})
		case strings.Contains(req.Query, "deployments("):
			writeGQL(w, map[string]any{"deployments": map[string]any{"edges": []map[string]any{
				{"node": map[string]any{"id": "dep-1", "status": "FAILED"}},
			}}})
		default:
			t.Fatalf("unexpected query: %s", req.Query)
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
		t.Fatal("expected Deploy to surface the failed deployment status")
	}
	if !strings.Contains(err.Error(), "FAILED") {
		t.Errorf("expected error to mention FAILED, got: %v", err)
	}
}
