package health_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/graph"
	"github.com/acthurhq/acthur/internal/health"
)

// ---------------------------------------------------------------------------
// ResolvePort tests
// ---------------------------------------------------------------------------

func TestResolvePort_ExplicitPort(t *testing.T) {
	node := &graph.Node{Port: 9090}
	if got := health.ResolvePort(node); got != 9090 {
		t.Errorf("expected 9090, got %d", got)
	}
}

func TestResolvePort_PostgresDefault(t *testing.T) {
	node := &graph.Node{Adapter: "db:postgres"}
	if got := health.ResolvePort(node); got != 5432 {
		t.Errorf("expected 5432 for db:postgres, got %d", got)
	}
}

func TestResolvePort_RedisDefault(t *testing.T) {
	node := &graph.Node{Adapter: "cache:redis"}
	if got := health.ResolvePort(node); got != 6379 {
		t.Errorf("expected 6379 for cache:redis, got %d", got)
	}
}

func TestResolvePort_MinioDefault(t *testing.T) {
	node := &graph.Node{Adapter: "storage:minio"}
	if got := health.ResolvePort(node); got != 9000 {
		t.Errorf("expected 9000 for storage:minio, got %d", got)
	}
}

func TestResolvePort_NATSDefault(t *testing.T) {
	node := &graph.Node{Adapter: "queue:nats"}
	if got := health.ResolvePort(node); got != 4222 {
		t.Errorf("expected 4222 for queue:nats, got %d", got)
	}
}

func TestResolvePort_ExplicitOverridesDefault(t *testing.T) {
	node := &graph.Node{Adapter: "db:postgres", Port: 5433}
	if got := health.ResolvePort(node); got != 5433 {
		t.Errorf("expected explicit port 5433 to override default, got %d", got)
	}
}

func TestResolvePort_UnknownAdapterReturnsZero(t *testing.T) {
	node := &graph.Node{Adapter: "unknown:adapter"}
	if got := health.ResolvePort(node); got != 0 {
		t.Errorf("expected 0 for unknown adapter, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// HTTP strategy tests
// ---------------------------------------------------------------------------

func TestHTTPStrategy_HealthyServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	port := extractPort(t, srv.URL)
	node := makeServiceNode("api", port)

	checker := health.New()
	if err := checker.Poll(node); err != nil {
		t.Errorf("expected healthy service to pass health check, got: %v", err)
	}
}

func TestHTTPStrategy_ServerReturning500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	port := extractPort(t, srv.URL)
	node := makeServiceNode("api", port)

	checker := health.New()
	err := checker.Poll(node)
	if err == nil {
		t.Error("expected 500 response to fail health check")
	}
}

func TestHTTPStrategy_ServerReturning404_Passes(t *testing.T) {
	// 404 means the server IS running — it just doesn't have /health
	// This is considered "up" by the health checker (not a 5xx)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	port := extractPort(t, srv.URL)
	node := makeServiceNode("api", port)

	checker := health.New()
	if err := checker.Poll(node); err != nil {
		t.Errorf("expected 404 to pass health check (server is up), got: %v", err)
	}
}

func TestHTTPStrategy_ConnectionRefused(t *testing.T) {
	// Port 19876 has no server
	node := makeServiceNode("api", 19876)
	checker := health.New()
	err := checker.Poll(node)
	if err == nil {
		t.Error("expected connection refused to fail health check")
	}
}

func TestHTTPStrategy_NoPort(t *testing.T) {
	node := makeServiceNode("api", 0)
	checker := health.New()
	err := checker.Poll(node)
	if err == nil {
		t.Error("expected error for node with no port configured")
	}
}

// ---------------------------------------------------------------------------
// TCP strategy tests
// ---------------------------------------------------------------------------

func TestInfraStrategy_DeclaredHealthcheckRunsDockerExec(t *testing.T) {
	var gotName string
	var gotArgs []string
	strategy := health.InfraStrategy("acthur-db", []string{"CMD-SHELL", "pg_isready -U postgres"}, health.CommandRunnerFunc(
		func(ctx context.Context, name string, args ...string) error {
			gotName = name
			gotArgs = append([]string(nil), args...)
			return nil
		},
	))

	node := makeInfraNode("db", "db:postgres", 5432)
	if err := strategy.Check(context.Background(), node); err != nil {
		t.Fatalf("expected declared healthcheck to pass, got: %v", err)
	}

	if gotName != "docker" {
		t.Fatalf("expected docker command, got %q", gotName)
	}
	wantArgs := []string{"exec", "acthur-db", "sh", "-c", "pg_isready -U postgres"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("docker exec args mismatch\nwant: %#v\n got: %#v", wantArgs, gotArgs)
	}
}

func TestInfraStrategy_NoDeclaredHealthcheckFallsBackToTCP(t *testing.T) {
	strategy := health.InfraStrategy("acthur-db", nil, nil)

	if got := strategy.Name(); got != "tcp" {
		t.Fatalf("expected tcp fallback, got %q", got)
	}
}

func TestTCPStrategy_ListeningServer(t *testing.T) {
	// Start an HTTP server to listen on a port (TCP will connect successfully)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	port := extractPort(t, srv.URL)
	// Use an infra node (TCP strategy)
	node := makeInfraNode("db", "db:postgres", port)

	checker := health.New()
	if err := checker.Poll(node); err != nil {
		t.Errorf("expected TCP connect to succeed, got: %v", err)
	}
}

func TestTCPStrategy_NoServer(t *testing.T) {
	node := makeInfraNode("db", "db:postgres", 19877)
	checker := health.New()
	err := checker.Poll(node)
	if err == nil {
		t.Error("expected TCP connect to fail for port with no server")
	}
}

// ---------------------------------------------------------------------------
// DefaultPorts map tests
// ---------------------------------------------------------------------------

func TestDefaultPorts_AllAdaptersPresent(t *testing.T) {
	expectedAdapters := []string{
		"db:postgres", "db:mysql",
		"cache:redis",
		"storage:minio",
		"queue:nats",
	}
	for _, adapter := range expectedAdapters {
		node := &graph.Node{Adapter: adapter}
		if health.ResolvePort(node) == 0 {
			t.Errorf("expected default port for adapter %q", adapter)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func makeServiceNode(id string, port int) *graph.Node {
	return &graph.Node{
		ID:      id,
		Type:    config.NodeTypeService,
		Adapter: "go:fiber",
		Port:    port,
	}
}

func makeInfraNode(id, adapter string, port int) *graph.Node {
	return &graph.Node{
		ID:      id,
		Type:    config.NodeTypeInfra,
		Adapter: adapter,
		Port:    port,
	}
}

func extractPort(t *testing.T, rawURL string) int {
	t.Helper()
	var port int
	if _, err := fmt.Sscanf(rawURL, "http://127.0.0.1:%d", &port); err == nil {
		return port
	}
	t.Fatalf("could not extract port from %q", rawURL)
	return 0
}

// TestTCPStrategy_UsesAdapterDefaultPort: infra nodes routinely omit port
// in acthur.yml (the adapter's well-known default applies everywhere else —
// container specs, connection env). The TCP probe must resolve the same
// default instead of failing with "no port configured", or `acthur monitor`
// and `service health` report a healthy database as unhealthy.
func TestTCPStrategy_UsesAdapterDefaultPort(t *testing.T) {
	node := &graph.Node{ID: "queue", Type: config.NodeTypeInfra, Adapter: "queue:nats"}
	err := (&health.TCPStrategy{}).Check(context.Background(), node)
	if err != nil && strings.Contains(err.Error(), "no port configured") {
		t.Fatalf("TCP probe must fall back to the adapter default port, got: %v", err)
	}
}

// TestHTTPStrategy_NoPortStillErrors: a service node with no port and no
// adapter default genuinely can't be probed — that must stay an error.
func TestHTTPStrategy_NoPortStillErrors(t *testing.T) {
	node := &graph.Node{ID: "svc", Type: config.NodeTypeService, Adapter: "custom:thing"}
	err := health.NewHTTPStrategy().Check(context.Background(), node)
	if err == nil || !strings.Contains(err.Error(), "no port") {
		t.Fatalf("expected no-port error for unresolvable node, got: %v", err)
	}
}
