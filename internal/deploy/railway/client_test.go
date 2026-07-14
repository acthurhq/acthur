package railway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type recordedRequest struct {
	Auth  string
	Query string
	Vars  map[string]any
}

func newFakeServer(t *testing.T, handler func(w http.ResponseWriter, req gqlRequest, rec *recordedRequest)) (*httptest.Server, *[]recordedRequest) {
	t.Helper()
	var requests []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req gqlRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		rec := recordedRequest{Auth: r.Header.Get("Authorization"), Query: req.Query, Vars: req.Variables}
		requests = append(requests, rec)
		handler(w, req, &requests[len(requests)-1])
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func writeGQL(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func writeGQLError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"errors": []map[string]any{{"message": message}}})
}

func TestEnsureProject_createsWhenMissing(t *testing.T) {
	srv, requests := newFakeServer(t, func(w http.ResponseWriter, req gqlRequest, rec *recordedRequest) {
		switch {
		case strings.Contains(req.Query, "projects {"):
			writeGQL(w, map[string]any{"projects": map[string]any{"edges": []any{}}})
		case strings.Contains(req.Query, "projectCreate"):
			writeGQL(w, map[string]any{"projectCreate": map[string]any{"id": "proj-1"}})
		default:
			t.Fatalf("unexpected query: %s", req.Query)
		}
	})

	c := New("tok")
	c.BaseURL = srv.URL
	id, err := c.EnsureProject("myapp")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if id != "proj-1" {
		t.Fatalf("expected proj-1, got %q", id)
	}
	for _, r := range *requests {
		if r.Auth != "Bearer tok" {
			t.Errorf("missing bearer auth: %q", r.Auth)
		}
	}
}

func TestEnsureProject_findsExisting(t *testing.T) {
	srv, _ := newFakeServer(t, func(w http.ResponseWriter, req gqlRequest, rec *recordedRequest) {
		writeGQL(w, map[string]any{"projects": map[string]any{"edges": []map[string]any{
			{"node": map[string]any{"id": "proj-1", "name": "myapp"}},
		}}})
	})

	c := New("tok")
	c.BaseURL = srv.URL
	id, err := c.EnsureProject("myapp")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if id != "proj-1" {
		t.Fatalf("expected proj-1, got %q", id)
	}
}

func TestEnsureService_createsThenUpdates(t *testing.T) {
	var serviceExists bool
	srv, requests := newFakeServer(t, func(w http.ResponseWriter, req gqlRequest, rec *recordedRequest) {
		switch {
		case strings.Contains(req.Query, "project(id:"):
			if serviceExists {
				writeGQL(w, map[string]any{"project": map[string]any{
					"environments": map[string]any{"edges": []map[string]any{{"node": map[string]any{"id": "env-1", "name": "production"}}}},
					"services":     map[string]any{"edges": []map[string]any{{"node": map[string]any{"id": "svc-1", "name": "api"}}}},
				}})
			} else {
				writeGQL(w, map[string]any{"project": map[string]any{
					"environments": map[string]any{"edges": []map[string]any{{"node": map[string]any{"id": "env-1", "name": "production"}}}},
					"services":     map[string]any{"edges": []any{}},
				}})
			}
		case strings.Contains(req.Query, "serviceCreate"):
			writeGQL(w, map[string]any{"serviceCreate": map[string]any{"id": "svc-1"}})
			serviceExists = true
		case strings.Contains(req.Query, "serviceInstanceUpdate"):
			writeGQL(w, map[string]any{"serviceInstanceUpdate": true})
		default:
			t.Fatalf("unexpected query: %s", req.Query)
		}
	})

	c := New("tok")
	c.BaseURL = srv.URL

	id, err := c.EnsureService("proj-1", "env-1", "api", "ghcr.io/acme/myapp:api")
	if err != nil {
		t.Fatalf("EnsureService (create): %v", err)
	}
	if id != "svc-1" {
		t.Fatalf("expected svc-1, got %q", id)
	}

	id, err = c.EnsureService("proj-1", "env-1", "api", "ghcr.io/acme/myapp:api2")
	if err != nil {
		t.Fatalf("EnsureService (update): %v", err)
	}
	if id != "svc-1" {
		t.Fatalf("expected svc-1 on update, got %q", id)
	}

	var sawUpdate bool
	for _, r := range *requests {
		if strings.Contains(r.Query, "serviceInstanceUpdate") {
			sawUpdate = true
		}
	}
	if !sawUpdate {
		t.Fatal("expected an update mutation for the existing service")
	}
}

func TestLatestDeploymentStatus_surfacesGraphQLError(t *testing.T) {
	srv, _ := newFakeServer(t, func(w http.ResponseWriter, req gqlRequest, rec *recordedRequest) {
		writeGQLError(w, "service not found")
	})

	c := New("tok")
	c.BaseURL = srv.URL
	_, err := c.LatestDeploymentStatus("svc-1", "env-1")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "service not found") {
		t.Errorf("expected GraphQL error message surfaced, got: %v", err)
	}
}

func TestLatestDeploymentStatus_returnsStatus(t *testing.T) {
	srv, _ := newFakeServer(t, func(w http.ResponseWriter, req gqlRequest, rec *recordedRequest) {
		writeGQL(w, map[string]any{"deployments": map[string]any{"edges": []map[string]any{
			{"node": map[string]any{"id": "dep-1", "status": "SUCCESS"}},
		}}})
	})

	c := New("tok")
	c.BaseURL = srv.URL
	status, err := c.LatestDeploymentStatus("svc-1", "env-1")
	if err != nil {
		t.Fatalf("LatestDeploymentStatus: %v", err)
	}
	if status != "SUCCESS" {
		t.Fatalf("expected SUCCESS, got %q", status)
	}
}
