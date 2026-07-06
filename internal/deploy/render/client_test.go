package render

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
		rec := recordedRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Auth: r.Header.Get("Authorization")}
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

func TestEnsureOwner_returnsFirstOwner(t *testing.T) {
	srv, requests := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		if r.Method != http.MethodGet || r.URL.Path != "/owners" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		writeJSON(w, 200, []map[string]any{
			{"owner": map[string]any{"id": "usr-1", "name": "acme"}},
		})
	})

	c := New("tok")
	c.BaseURL = srv.URL
	id, err := c.EnsureOwner()
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}
	if id != "usr-1" {
		t.Fatalf("expected usr-1, got %q", id)
	}
	if (*requests)[0].Auth != "Bearer tok" {
		t.Errorf("missing bearer auth: %q", (*requests)[0].Auth)
	}
}

func TestEnsureService_createsWhenMissing(t *testing.T) {
	srv, requests := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/services":
			writeJSON(w, 200, []map[string]any{})
		case r.Method == http.MethodPost && r.URL.Path == "/services":
			if rec.Body["name"] != "myapp-production-api" {
				t.Errorf("unexpected create body: %v", rec.Body)
			}
			writeJSON(w, 201, map[string]any{"id": "srv-1", "name": "myapp-production-api"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	c := New("tok")
	c.BaseURL = srv.URL
	id, err := c.EnsureService("usr-1", "myapp-production-api", "ghcr.io/acme/myapp:api")
	if err != nil {
		t.Fatalf("EnsureService: %v", err)
	}
	if id != "srv-1" {
		t.Fatalf("expected srv-1, got %q", id)
	}
	if len(*requests) != 2 {
		t.Fatalf("expected list+create, got %d requests", len(*requests))
	}
}

func TestEnsureService_findsExisting(t *testing.T) {
	srv, requests := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		writeJSON(w, 200, []map[string]any{
			{"service": map[string]any{"id": "srv-1", "name": "myapp-production-api"}},
		})
	})

	c := New("tok")
	c.BaseURL = srv.URL
	id, err := c.EnsureService("usr-1", "myapp-production-api", "ghcr.io/acme/myapp:api")
	if err != nil {
		t.Fatalf("EnsureService: %v", err)
	}
	if id != "srv-1" {
		t.Fatalf("expected srv-1, got %q", id)
	}
	if len(*requests) != 1 {
		t.Fatalf("expected only the list request, got %d", len(*requests))
	}
}

func TestDeploy_triggersAndReturnsID(t *testing.T) {
	srv, requests := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		if r.Method != http.MethodPost || r.URL.Path != "/services/srv-1/deploys" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if rec.Body["imageUrl"] != "ghcr.io/acme/myapp:api" {
			t.Errorf("unexpected deploy body: %v", rec.Body)
		}
		writeJSON(w, 201, map[string]any{"id": "dep-1", "status": "build_in_progress"})
	})

	c := New("tok")
	c.BaseURL = srv.URL
	id, err := c.Deploy("srv-1", "ghcr.io/acme/myapp:api")
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if id != "dep-1" {
		t.Fatalf("expected dep-1, got %q", id)
	}
	_ = requests
}

func TestDeployStatus_returnsStatus(t *testing.T) {
	srv, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		if r.URL.Path != "/services/srv-1/deploys/dep-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		writeJSON(w, 200, map[string]any{"id": "dep-1", "status": "live"})
	})

	c := New("tok")
	c.BaseURL = srv.URL
	status, err := c.DeployStatus("srv-1", "dep-1")
	if err != nil {
		t.Fatalf("DeployStatus: %v", err)
	}
	if status != "live" {
		t.Fatalf("expected live, got %q", status)
	}
}

func TestAPIError_surfacesBody(t *testing.T) {
	srv, _ := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		writeJSON(w, 401, map[string]any{"message": "invalid api key"})
	})

	c := New("bad-tok")
	c.BaseURL = srv.URL
	_, err := c.EnsureOwner()
	if err == nil {
		t.Fatal("expected an error")
	}
}
