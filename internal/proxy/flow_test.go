package proxy_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/contract"
	"github.com/acthurhq/acthur/internal/graph"
	"github.com/acthurhq/acthur/internal/output"
	"github.com/acthurhq/acthur/internal/proxy"
)

// ---------------------------------------------------------------------------
// Flow route tests (data_flow edges → /_flow/<from>/<to>/*)
// ---------------------------------------------------------------------------

// buildFlowGraph builds a graph with a "web" service that data_flows to an
// "api" service on the given contract name. api's port is apiPort; web has no
// port of its own (it's the caller, not the callee).
func buildFlowGraph(t *testing.T, apiPort int, contractName string) *graph.Graph {
	t.Helper()
	cfg := &config.Config{
		Project: "test",
		Dev:     config.DevConfig{Domain: "test.test", Port: 4000},
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api": {Type: config.NodeTypeService, Adapter: "go:fiber", Port: apiPort},
				"web": {Type: config.NodeTypeService, Adapter: "ui:astro"},
			},
			Edges: []config.EdgeConfig{
				{From: "web", To: "api", Type: config.EdgeDataFlow, Contracts: []string{contractName}},
			},
		},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("failed to build flow graph: %v", err)
	}
	return g
}

// pingContract is a minimal contract with a single GET /ping endpoint, used to
// exercise the matching/non-matching request paths in enforcement tests.
func pingContract(name string) *contract.Contract {
	return &contract.Contract{
		Name:      name,
		Version:   "1",
		Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{ID: "ping", Method: "GET", Path: "/ping"},
		},
	}
}

// pingContractWithOutput is pingContract but with a declared Output schema,
// used to exercise response validation.
func pingContractWithOutput(name string) *contract.Contract {
	return &contract.Contract{
		Name:      name,
		Version:   "1",
		Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{ID: "ping", Method: "GET", Path: "/ping", Output: map[string]string{"pong": "bool"}},
		},
	}
}

func TestProxy_FlowRoute_StripsPrefixAndForwards(t *testing.T) {
	var gotPath string
	apiBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer apiBackend.Close()

	apiPort := extractPort(t, apiBackend.URL)
	g := buildFlowGraph(t, apiPort, "pingapi")

	p, err := proxy.New(g, 14100)
	if err != nil {
		t.Fatalf("proxy creation failed: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("proxy start failed: %v", err)
	}
	defer func() { _ = p.Stop() }()
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://localhost:14100/_flow/web/api/ping")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from backend, got %d", resp.StatusCode)
	}
	if gotPath != "/ping" {
		t.Fatalf("expected backend to see stripped path /ping, got %q", gotPath)
	}
}

func TestProxy_FlowRoute_NoRegistry_PassesThroughWithoutEnforcement(t *testing.T) {
	apiBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer apiBackend.Close()

	apiPort := extractPort(t, apiBackend.URL)
	g := buildFlowGraph(t, apiPort, "pingapi")

	// No contract registry passed — enforcement is opt-in.
	p, err := proxy.New(g, 14101)
	if err != nil {
		t.Fatalf("proxy creation failed: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("proxy start failed: %v", err)
	}
	defer func() { _ = p.Stop() }()
	time.Sleep(100 * time.Millisecond)

	// A request that would violate the (nonexistent) contract still passes.
	resp, err := http.Get("http://localhost:14101/_flow/web/api/does-not-exist")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected pass-through 200 with no registry configured, got %d", resp.StatusCode)
	}
}

func TestProxy_FlowRoute_ConformingRequestPassesThroughByteIdentical(t *testing.T) {
	wantBody := []byte(`{"ok":true}`)
	apiBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(wantBody)
	}))
	defer apiBackend.Close()

	apiPort := extractPort(t, apiBackend.URL)
	g := buildFlowGraph(t, apiPort, "pingapi")

	registry := contract.NewRegistry()
	if err := registry.Register(pingContract("pingapi")); err != nil {
		t.Fatalf("register contract: %v", err)
	}

	p, err := proxy.New(g, 14102, proxy.WithContractRegistry(registry))
	if err != nil {
		t.Fatalf("proxy creation failed: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("proxy start failed: %v", err)
	}
	defer func() { _ = p.Stop() }()
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://localhost:14102/_flow/web/api/ping")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for conforming request, got %d", resp.StatusCode)
	}
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	if !bytes.Equal(buf.Bytes(), wantBody) {
		t.Fatalf("expected byte-identical body %q, got %q", wantBody, buf.Bytes())
	}
}

func TestProxy_FlowRoute_DevMode_LogsViolationAndForwards(t *testing.T) {
	apiBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer apiBackend.Close()

	apiPort := extractPort(t, apiBackend.URL)
	g := buildFlowGraph(t, apiPort, "pingapi")

	registry := contract.NewRegistry()
	if err := registry.Register(pingContract("pingapi")); err != nil {
		t.Fatalf("register contract: %v", err)
	}

	var logBuf bytes.Buffer
	output.SetOutput(&logBuf, &logBuf)
	defer output.SetOutput(os.Stdout, os.Stderr)

	p, err := proxy.New(g, 14103, proxy.WithContractRegistry(registry))
	if err != nil {
		t.Fatalf("proxy creation failed: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("proxy start failed: %v", err)
	}
	defer func() { _ = p.Stop() }()
	time.Sleep(100 * time.Millisecond)

	// /pong doesn't match any endpoint on the "pingapi" contract → violation.
	resp, err := http.Get("http://localhost:14103/_flow/web/api/pong")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected dev mode to forward despite violation, got %d", resp.StatusCode)
	}
	logLine := logBuf.String()
	if !strings.Contains(logLine, "proxy") || !strings.Contains(strings.ToLower(logLine), "violation") {
		t.Fatalf("expected a contract violation line on the proxy stream, got %q", logLine)
	}
}

func TestProxy_FlowRoute_StrictMode_Blocks422(t *testing.T) {
	backendHit := false
	apiBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendHit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer apiBackend.Close()

	apiPort := extractPort(t, apiBackend.URL)
	g := buildFlowGraph(t, apiPort, "pingapi")

	registry := contract.NewRegistry()
	if err := registry.Register(pingContract("pingapi")); err != nil {
		t.Fatalf("register contract: %v", err)
	}

	p, err := proxy.New(g, 14104, proxy.WithContractRegistry(registry), proxy.WithStrict(true))
	if err != nil {
		t.Fatalf("proxy creation failed: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("proxy start failed: %v", err)
	}
	defer func() { _ = p.Stop() }()
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://localhost:14104/_flow/web/api/pong")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 in strict mode, got %d", resp.StatusCode)
	}
	if backendHit {
		t.Fatal("expected strict mode to block the request before it reached the backend")
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("expected JSON body, decode failed: %v", err)
	}
	if body.Error.Code != "contract_violation" {
		t.Fatalf("expected error.code=contract_violation, got %q", body.Error.Code)
	}
}

func TestProxy_FlowRoute_NoMatchingEndpoint_CountsAsViolation(t *testing.T) {
	apiBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer apiBackend.Close()

	apiPort := extractPort(t, apiBackend.URL)
	g := buildFlowGraph(t, apiPort, "pingapi")

	registry := contract.NewRegistry()
	if err := registry.Register(pingContract("pingapi")); err != nil {
		t.Fatalf("register contract: %v", err)
	}

	p, err := proxy.New(g, 14105, proxy.WithContractRegistry(registry), proxy.WithStrict(true))
	if err != nil {
		t.Fatalf("proxy creation failed: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("proxy start failed: %v", err)
	}
	defer func() { _ = p.Stop() }()
	time.Sleep(100 * time.Millisecond)

	// POST /ping matches no endpoint (only GET /ping is defined) → violation → 422.
	resp, err := http.Post("http://localhost:14105/_flow/web/api/ping", "application/json", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for unmatched endpoint, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Response validation (mirrors the request-validation tests above)
// ---------------------------------------------------------------------------

func TestProxy_FlowRoute_ConformingResponse_PassesThroughByteIdentical(t *testing.T) {
	wantBody := []byte(`{"pong":true}`)
	apiBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(wantBody)
	}))
	defer apiBackend.Close()

	apiPort := extractPort(t, apiBackend.URL)
	g := buildFlowGraph(t, apiPort, "pingapi")

	registry := contract.NewRegistry()
	if err := registry.Register(pingContractWithOutput("pingapi")); err != nil {
		t.Fatalf("register contract: %v", err)
	}

	p, err := proxy.New(g, 14106, proxy.WithContractRegistry(registry))
	if err != nil {
		t.Fatalf("proxy creation failed: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("proxy start failed: %v", err)
	}
	defer func() { _ = p.Stop() }()
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://localhost:14106/_flow/web/api/ping")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for conforming response, got %d", resp.StatusCode)
	}
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	if !bytes.Equal(buf.Bytes(), wantBody) {
		t.Fatalf("expected byte-identical body %q, got %q", wantBody, buf.Bytes())
	}
}

func TestProxy_FlowRoute_ResponseViolation_DevMode_LogsWarningAndForwards(t *testing.T) {
	// Backend omits the contract's required "pong" output field.
	apiBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer apiBackend.Close()

	apiPort := extractPort(t, apiBackend.URL)
	g := buildFlowGraph(t, apiPort, "pingapi")

	registry := contract.NewRegistry()
	if err := registry.Register(pingContractWithOutput("pingapi")); err != nil {
		t.Fatalf("register contract: %v", err)
	}

	var logBuf bytes.Buffer
	output.SetOutput(&logBuf, &logBuf)
	defer output.SetOutput(os.Stdout, os.Stderr)

	p, err := proxy.New(g, 14107, proxy.WithContractRegistry(registry))
	if err != nil {
		t.Fatalf("proxy creation failed: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("proxy start failed: %v", err)
	}
	defer func() { _ = p.Stop() }()
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://localhost:14107/_flow/web/api/ping")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected dev mode to forward the response despite the violation, got %d", resp.StatusCode)
	}
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	if buf.String() != "{}" {
		t.Fatalf("expected the original response body to be forwarded unmodified, got %q", buf.String())
	}

	logLine := logBuf.String()
	if !strings.Contains(logLine, "proxy") || !strings.Contains(strings.ToLower(logLine), "response") {
		t.Fatalf("expected a response contract violation line on the proxy stream, got %q", logLine)
	}
}

func TestProxy_FlowRoute_ResponseViolation_StrictMode_Returns502(t *testing.T) {
	apiBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer apiBackend.Close()

	apiPort := extractPort(t, apiBackend.URL)
	g := buildFlowGraph(t, apiPort, "pingapi")

	registry := contract.NewRegistry()
	if err := registry.Register(pingContractWithOutput("pingapi")); err != nil {
		t.Fatalf("register contract: %v", err)
	}

	p, err := proxy.New(g, 14108, proxy.WithContractRegistry(registry), proxy.WithStrict(true))
	if err != nil {
		t.Fatalf("proxy creation failed: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("proxy start failed: %v", err)
	}
	defer func() { _ = p.Stop() }()
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://localhost:14108/_flow/web/api/ping")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502 in strict mode for a response contract violation, got %d", resp.StatusCode)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("expected JSON body, decode failed: %v", err)
	}
	if body.Error.Code != "response_contract_violation" {
		t.Fatalf("expected error.code=response_contract_violation, got %q", body.Error.Code)
	}
	if !strings.Contains(body.Error.Message, "pong") {
		t.Fatalf("expected violation message to name the missing field, got %q", body.Error.Message)
	}
}
