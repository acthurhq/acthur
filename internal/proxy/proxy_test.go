package proxy_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/graph"
	"github.com/acthurhq/acthur/internal/proxy"
)

// ---------------------------------------------------------------------------
// Proxy creation tests
// ---------------------------------------------------------------------------

func TestNew_ValidGraph(t *testing.T) {
	g := buildProxyGraph(t)
	p, err := proxy.New(g, 4000)
	if err != nil {
		t.Fatalf("expected proxy creation to succeed, got: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil proxy")
	}
}

func TestNew_EmptyGraph(t *testing.T) {
	cfg := &config.Config{
		Project: "test",
		Dev:     config.DevConfig{Domain: "test.test", Port: 4000},
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api": {Type: "service", Adapter: "go:fiber", Port: 8080},
			},
			Edges: []config.EdgeConfig{},
		},
	}
	g, _ := graph.Build(cfg)
	// No proxied_through edges — should still create proxy with no routes
	_, err := proxy.New(g, 4000)
	if err != nil {
		t.Fatalf("expected proxy with empty routes to succeed, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Route matching tests
// ---------------------------------------------------------------------------

func TestProxy_RoutesAPIRequests(t *testing.T) {
	// Start a fake API backend
	apiBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "api")
		w.WriteHeader(http.StatusOK)
	}))
	defer apiBackend.Close()

	// Start a fake web backend
	webBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "web")
		w.WriteHeader(http.StatusOK)
	}))
	defer webBackend.Close()

	apiPort := extractPort(t, apiBackend.URL)
	webPort := extractPort(t, webBackend.URL)

	g := buildGraphWithPorts(t, apiPort, webPort)
	p, err := proxy.New(g, 14000)
	if err != nil {
		t.Fatalf("proxy creation failed: %v", err)
	}

	if err := p.Start(); err != nil {
		t.Fatalf("proxy start failed: %v", err)
	}
	defer func() { _ = p.Stop() }()

	time.Sleep(100 * time.Millisecond) // let server bind

	// Request to /api should go to API backend
	resp, err := http.Get("http://localhost:14000/api/users")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.Header.Get("X-Backend") != "api" {
		t.Errorf("expected X-Backend=api, got %q", resp.Header.Get("X-Backend"))
	}
}

func TestProxy_RoutesRootToWeb(t *testing.T) {
	webBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "web")
		w.WriteHeader(http.StatusOK)
	}))
	defer webBackend.Close()

	apiBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "api")
		w.WriteHeader(http.StatusOK)
	}))
	defer apiBackend.Close()

	webPort := extractPort(t, webBackend.URL)
	apiPort := extractPort(t, apiBackend.URL)

	g := buildGraphWithPorts(t, apiPort, webPort)
	p, _ := proxy.New(g, 14001)
	_ = p.Start()
	defer func() { _ = p.Stop() }()
	time.Sleep(100 * time.Millisecond)

	// Request to / should go to web backend (catch-all)
	resp, err := http.Get("http://localhost:14001/")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.Header.Get("X-Backend") != "web" {
		t.Errorf("expected root to route to web (X-Backend=web), got %q",
			resp.Header.Get("X-Backend"))
	}
}

func TestProxy_InjectsNodeHeader(t *testing.T) {
	apiBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer apiBackend.Close()

	webBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer webBackend.Close()

	apiPort := extractPort(t, apiBackend.URL)
	webPort := extractPort(t, webBackend.URL)

	g := buildGraphWithPorts(t, apiPort, webPort)
	p, _ := proxy.New(g, 14002)
	_ = p.Start()
	defer func() { _ = p.Stop() }()
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://localhost:14002/api/test")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()

	// Proxy should inject X-Acthur-Node header in response
	if resp.Header.Get("X-Acthur-Node") == "" {
		t.Error("expected X-Acthur-Node response header to be set")
	}
}

func TestProxy_UnavailableBackend(t *testing.T) {
	// Build graph pointing at a port with no server
	cfg := &config.Config{
		Project: "test",
		Dev:     config.DevConfig{Domain: "test.test", Port: 4000},
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api": {Type: config.NodeTypeService, Adapter: "go:fiber", Port: 19999},
				"proxy-node": {Type: config.NodeTypeInfra, Adapter: "proxy:internal"},
			},
			Edges: []config.EdgeConfig{
				{From: "api", To: "proxy-node", Type: config.EdgeProxiedThrough},
			},
		},
	}
	g, _ := graph.Build(cfg)
	p, _ := proxy.New(g, 14003)
	_ = p.Start()
	defer func() { _ = p.Stop() }()
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://localhost:14003/api/test")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()
	// Should get a 502 Bad Gateway
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 502 for unavailable backend, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func buildProxyGraph(t *testing.T) *graph.Graph {
	t.Helper()
	cfg := &config.Config{
		Project: "test",
		Dev:     config.DevConfig{Domain: "test.test", Port: 4000},
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api":        {Type: config.NodeTypeService, Adapter: "go:fiber", Port: 8080},
				"web":        {Type: config.NodeTypeService, Adapter: "ui:astro", Port: 3000},
				"proxy-node": {Type: config.NodeTypeInfra, Adapter: "proxy:internal"},
			},
			Edges: []config.EdgeConfig{
				{From: "api", To: "proxy-node", Type: config.EdgeProxiedThrough},
				{From: "web", To: "proxy-node", Type: config.EdgeProxiedThrough},
			},
		},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("failed to build test graph: %v", err)
	}
	return g
}

func buildGraphWithPorts(t *testing.T, apiPort, webPort int) *graph.Graph {
	t.Helper()
	cfg := &config.Config{
		Project: "test",
		Dev:     config.DevConfig{Domain: "test.test", Port: 4000},
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api":        {Type: config.NodeTypeService, Adapter: "go:fiber", Port: apiPort},
				"web":        {Type: config.NodeTypeService, Adapter: "ui:astro", Port: webPort},
				"proxy-node": {Type: config.NodeTypeInfra, Adapter: "proxy:internal"},
			},
			Edges: []config.EdgeConfig{
				{From: "api", To: "proxy-node", Type: config.EdgeProxiedThrough},
				{From: "web", To: "proxy-node", Type: config.EdgeProxiedThrough},
			},
		},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("failed to build graph: %v", err)
	}
	return g
}

func extractPort(t *testing.T, rawURL string) int {
	t.Helper()
	var port int
	_, err := fmt.Sscanf(rawURL, "http://127.0.0.1:%d", &port)
	if err != nil {
		_, err = fmt.Sscanf(rawURL, "http://localhost:%d", &port)
	}
	if err != nil {
		t.Fatalf("could not extract port from %q: %v", rawURL, err)
	}
	return port
}
