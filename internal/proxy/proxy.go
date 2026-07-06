// Package proxy implements the Acthur dev reverse proxy.
// It listens on a single port (default :4000) and routes incoming
// requests to the correct service node based on proxied_through edges.
//
// The proxy also enforces contracts on data_flow edges in dev mode
// (logging violations) and in strict mode (blocking violations).
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/contract"
	"github.com/acthur/acthur/internal/graph"
	"github.com/acthur/acthur/internal/output"
)

// ---------------------------------------------------------------------------
// Route
// ---------------------------------------------------------------------------

// routeKind distinguishes the two kinds of route the proxy serves.
type routeKind int

const (
	// routeKindProxied is a plain inbound forward selected by a
	// proxied_through edge (browser → service, by path prefix).
	routeKindProxied routeKind = iota
	// routeKindFlow is a contract-checked east-west route derived from a
	// data_flow edge: /_flow/<from>/<to>/* → strip prefix → forward.
	routeKindFlow
)

// Route maps a path prefix to a backend target.
type Route struct {
	PathPrefix string
	NodeID     string
	Target     *url.URL
	Proxy      *httputil.ReverseProxy

	Kind routeKind
	// From/To identify the data_flow edge a flow route was derived from.
	// Empty for routeKindProxied routes.
	From, To string
}

// ---------------------------------------------------------------------------
// Proxy
// ---------------------------------------------------------------------------

// Proxy is the Acthur dev reverse proxy.
// Routes are derived from proxied_through edges (plain inbound forwards) and
// data_flow edges (contract-checked east-west routes) in the graph.
type Proxy struct {
	port   int
	routes []*Route
	mu     sync.RWMutex
	server *http.Server

	graph *graph.Graph

	// registry is the optional contract registry consulted on flow routes.
	// A nil registry means no enforcement — flow routes still forward, just
	// without any contract check (opt-in enforcement).
	registry *contract.Registry
	// strict blocks (422) contract violations on flow routes instead of
	// logging and forwarding them.
	strict bool

	// certFile/keyFile, when both set, switch Start from plain HTTP to TLS
	// (the https plugin's dev-cert seam — see internal/plugin/builtin/https).
	// A zero value on either means "serve plain HTTP", the existing default.
	certFile, keyFile string
}

// Option configures optional Proxy behavior at construction time.
type Option func(*Proxy)

// WithContractRegistry configures the registry the proxy consults to
// enforce contracts on data_flow flow routes. Without this option flow
// routes still forward traffic, but no contract is ever checked.
func WithContractRegistry(r *contract.Registry) Option {
	return func(p *Proxy) { p.registry = r }
}

// WithStrict switches flow-route contract enforcement from dev mode
// (log + forward) to strict mode (422 + block).
func WithStrict(strict bool) Option {
	return func(p *Proxy) { p.strict = strict }
}

// WithTLS switches Start from ListenAndServe to ListenAndServeTLS using the
// given cert/key file pair. Empty strings for either revert to plain HTTP —
// the zero value of Proxy already behaves this way, so this option only
// needs to be applied when both paths are non-empty (e.g. the https plugin
// has generated/found a dev certificate).
func WithTLS(certFile, keyFile string) Option {
	return func(p *Proxy) {
		p.certFile = certFile
		p.keyFile = keyFile
	}
}

// New creates a proxy from the graph. It builds routes from all
// proxied_through edges (ordered by path prefix length, longest prefix
// wins, ensuring /api/* matches before /*) and from all data_flow edges
// (served at /_flow/<from>/<to>/*).
func New(g *graph.Graph, port int, opts ...Option) (*Proxy, error) {
	p := &Proxy{port: port, graph: g}
	for _, opt := range opts {
		opt(p)
	}

	routes, err := p.buildAllRoutes(g)
	if err != nil {
		return nil, err
	}
	p.routes = routes

	return p, nil
}

// Start begins listening on the configured port.
// Returns immediately — the server runs in the background.
func (p *Proxy) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", p.dispatch)

	p.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", p.port),
		Handler:      mux,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	if p.certFile != "" && p.keyFile != "" {
		go func() {
			output.Info("proxy", "listening on https://localhost:%d", p.port)
			if err := p.server.ListenAndServeTLS(p.certFile, p.keyFile); err != nil && err != http.ErrServerClosed {
				output.Error("proxy", "server error: %v", err)
			}
		}()
		return nil
	}

	go func() {
		output.Info("proxy", "listening on http://localhost:%d", p.port)
		if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			output.Error("proxy", "server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the proxy.
func (p *Proxy) Stop() error {
	if p.server != nil {
		return p.server.Close()
	}
	return nil
}

// UpdateRoutes replaces the route table with fresh routes from an updated graph.
// Called when the graph is mutated at runtime (e.g. a new service added).
func (p *Proxy) UpdateRoutes(g *graph.Graph) error {
	routes, err := p.buildAllRoutes(g)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.routes = routes
	p.graph = g
	p.mu.Unlock()
	output.Info("proxy", "routes updated (%d backends)", len(routes))
	return nil
}

// PrintRoutes prints the current route table for `acthur graph show`.
func (p *Proxy) PrintRoutes() {
	p.mu.RLock()
	defer p.mu.RUnlock()
	output.Header("Proxy Routes")
	for _, r := range p.routes {
		fmt.Printf("  %-30s → %s (%s)\n",
			fmt.Sprintf("http://localhost:%d%s*", p.port, r.PathPrefix),
			r.Target,
			r.NodeID,
		)
	}
	fmt.Println()
}

// ---------------------------------------------------------------------------
// Request dispatch
// ---------------------------------------------------------------------------

func (p *Proxy) dispatch(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	routes := p.routes
	g := p.graph
	p.mu.RUnlock()

	route := p.matchRoute(routes, r.URL.Path)
	if route == nil {
		http.Error(w, "no route found for "+r.URL.Path, http.StatusBadGateway)
		output.Warn("proxy", "no route for %s %s", r.Method, r.URL.Path)
		return
	}

	if route.Kind == routeKindFlow {
		r.URL.Path = stripPrefixPath(r.URL.Path, route.PathPrefix)
		r.URL.RawPath = ""
		if !p.enforceContract(w, r, route, g) {
			return // blocked in strict mode; response already written
		}
	}

	// Inject proxy headers
	r.Header.Set("X-Forwarded-For", r.RemoteAddr)
	r.Header.Set("X-Forwarded-Host", r.Host)
	r.Header.Set("X-Forwarded-Proto", "http")
	r.Header.Set("X-Acthur-Node", route.NodeID)

	route.Proxy.ServeHTTP(w, r)
}

// stripPrefixPath removes prefix from path, always returning a path that
// starts with a leading slash.
func stripPrefixPath(path, prefix string) string {
	trimmed := strings.TrimPrefix(path, prefix)
	if trimmed == "" || !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	return trimmed
}

// ---------------------------------------------------------------------------
// Contract enforcement (flow routes only)
// ---------------------------------------------------------------------------

// violationResponse is the structured body returned in strict mode.
type violationResponse struct {
	Error violationDetail `json:"error"`
}

type violationDetail struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Edge     string `json:"edge"`
	Contract string `json:"contract"`
}

// flowCheckCtxKey is the context key used to hand the matched contract +
// endpoint from request-time enforcement to the response-time check in
// ModifyResponse — the two run at different points in
// httputil.ReverseProxy's lifecycle but share the same request context.
type flowCheckCtxKey struct{}

// flowCheck carries what enforceContract matched, so checkResponseContract
// does not need to re-match the endpoint from the (now-stripped) request path.
type flowCheck struct {
	route        *Route
	contractName string
	version      string
	endpointID   string
}

// enforceContract resolves the flow route's edge contract and validates the
// request against it. Returns true if dispatch should continue forwarding
// the request (no registry configured, no contract declared, request
// conforms, or dev mode logged-and-passed). Returns false if the response
// has already been written (strict mode block).
//
// On a successful match (even one with request violations, in dev mode) it
// stashes the matched contract/endpoint on r's context so ModifyResponse can
// validate the response against the same endpoint's Output schema.
func (p *Proxy) enforceContract(w http.ResponseWriter, r *http.Request, route *Route, g *graph.Graph) bool {
	if p.registry == nil || g == nil {
		return true // enforcement is opt-in — no registry, no check
	}

	contractName := g.ContractFor(route.From, route.To)
	if contractName == "" {
		return true // edge declares no contract — nothing to enforce
	}

	c, ep, violation := p.checkContract(contractName, r)
	if ep != nil {
		ctx := context.WithValue(r.Context(), flowCheckCtxKey{}, &flowCheck{
			route:        route,
			contractName: c.Name,
			version:      c.Version,
			endpointID:   ep.ID,
		})
		*r = *r.WithContext(ctx)
	}

	if violation == "" {
		return true // conforming request
	}

	if p.strict {
		p.writeViolation(w, route, contractName, "contract_violation", violation)
		return false
	}

	output.Warn("proxy", "contract violation: %s→%s contract %q: %s", route.From, route.To, contractName, violation)
	return true
}

// checkContract validates r against the named contract. Returns the matched
// contract and endpoint (nil if no endpoint could be matched) plus an empty
// violation string when the request conforms, or a human-readable violation
// message otherwise. A request with no matching endpoint in the contract
// counts as a violation.
func (p *Proxy) checkContract(name string, r *http.Request) (*contract.Contract, *contract.Endpoint, string) {
	c, err := p.registry.GetLatest(name)
	if err != nil {
		return nil, nil, fmt.Sprintf("contract %q not found in registry: %v", name, err)
	}

	ep := matchEndpoint(c, r.Method, r.URL.Path)
	if ep == nil {
		return c, nil, fmt.Sprintf("no endpoint matches %s %s in contract %q", r.Method, r.URL.Path, name)
	}

	headers := map[string]string{}
	if auth := r.Header.Get("Authorization"); auth != "" {
		headers["Authorization"] = auth
	}

	body := map[string]any{}
	if r.Body != nil {
		data, err := io.ReadAll(r.Body)
		if err == nil {
			r.Body = io.NopCloser(bytes.NewReader(data))
			// Best-effort JSON decode — a non-JSON or empty body simply
			// yields no fields to check, it is not itself a violation here.
			_ = json.Unmarshal(data, &body)
		}
	}

	validator := contract.NewValidator(p.registry, contract.SeverityWarn)
	result := validator.ValidateRequest(c.Name, c.Version, ep.ID, headers, body)
	if result.Valid {
		return c, ep, ""
	}
	msgs := make([]string, len(result.Violations))
	for i, v := range result.Violations {
		msgs[i] = v.Message
	}
	return c, ep, strings.Join(msgs, "; ")
}

// checkResponseContract validates a flow route's backend response against
// the Output schema of the endpoint matched at request time (carried via
// resp.Request's context — see enforceContract). It is installed as the
// route's httputil.ReverseProxy.ModifyResponse hook, so it runs after the
// backend responds but before the response is written to the client,
// letting strict mode rewrite the response into a structured 502.
//
// Only 2xx responses are checked — non-2xx responses generally use the
// contract's Errors shapes, not Output, and validating them against Output
// would misreport legitimate error responses as violations.
func (p *Proxy) checkResponseContract(resp *http.Response) error {
	if p.registry == nil {
		return nil
	}
	fc, _ := resp.Request.Context().Value(flowCheckCtxKey{}).(*flowCheck)
	if fc == nil {
		return nil // no contract matched at request time — nothing to check
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil // best-effort: an unreadable body is not itself a contract violation
	}
	resp.Body = io.NopCloser(bytes.NewReader(data))

	body := map[string]any{}
	_ = json.Unmarshal(data, &body) // best-effort — a non-JSON body simply yields no fields to check

	validator := contract.NewValidator(p.registry, contract.SeverityWarn)
	result := validator.ValidateResponse(fc.contractName, fc.version, fc.endpointID, body)
	if result.Valid {
		return nil
	}

	msgs := make([]string, len(result.Violations))
	for i, v := range result.Violations {
		msgs[i] = v.Message
	}
	violation := strings.Join(msgs, "; ")

	if p.strict {
		p.writeResponseViolation(resp, fc, violation)
		return nil
	}

	output.Warn("proxy", "response contract violation: %s→%s contract %q: %s", fc.route.From, fc.route.To, fc.contractName, violation)
	return nil
}

// matchEndpoint finds the contract endpoint matching method + path.
// Path segments in the contract prefixed with ":" (e.g. "/users/:id") match
// any literal segment in the request path.
func matchEndpoint(c *contract.Contract, method, path string) *contract.Endpoint {
	reqSegs := strings.Split(strings.Trim(path, "/"), "/")
	for i := range c.Endpoints {
		ep := &c.Endpoints[i]
		if !strings.EqualFold(ep.Method, method) {
			continue
		}
		epSegs := strings.Split(strings.Trim(ep.Path, "/"), "/")
		if len(epSegs) != len(reqSegs) {
			continue
		}
		match := true
		for i, seg := range epSegs {
			if strings.HasPrefix(seg, ":") {
				continue
			}
			if seg != reqSegs[i] {
				match = false
				break
			}
		}
		if match {
			return ep
		}
	}
	return nil
}

// writeViolation writes the structured 422 response for a blocked request
// and logs the block on the proxy stream.
func (p *Proxy) writeViolation(w http.ResponseWriter, route *Route, contractName, code, detail string) {
	output.Error("proxy", "contract violation (blocked): %s→%s contract %q: %s", route.From, route.To, contractName, detail)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	body := violationResponse{Error: violationDetail{
		Code:     code,
		Message:  detail,
		Edge:     fmt.Sprintf("%s→%s", route.From, route.To),
		Contract: contractName,
	}}
	_ = json.NewEncoder(w).Encode(body)
}

// writeResponseViolation rewrites resp in place into a structured 502,
// used in strict mode when a backend's response fails contract validation.
// It runs from ModifyResponse, before the response is written to the
// client, so overwriting resp.StatusCode/Body here still reaches the caller.
func (p *Proxy) writeResponseViolation(resp *http.Response, fc *flowCheck, detail string) {
	output.Error("proxy", "response contract violation (blocked): %s→%s contract %q: %s", fc.route.From, fc.route.To, fc.contractName, detail)

	body := violationResponse{Error: violationDetail{
		Code:     "response_contract_violation",
		Message:  detail,
		Edge:     fmt.Sprintf("%s→%s", fc.route.From, fc.route.To),
		Contract: fc.contractName,
	}}
	data, _ := json.Marshal(body)

	resp.StatusCode = http.StatusBadGateway
	resp.Status = fmt.Sprintf("%d %s", http.StatusBadGateway, http.StatusText(http.StatusBadGateway))
	resp.Body = io.NopCloser(bytes.NewReader(data))
	resp.ContentLength = int64(len(data))
	if resp.Header == nil {
		resp.Header = make(http.Header)
	}
	resp.Header.Set("Content-Type", "application/json")
	// The stale Content-Length from the original backend response would
	// otherwise be copied verbatim, leaving the client to wait for bytes
	// that never arrive (or truncate a longer replacement body).
	resp.Header.Set("Content-Length", strconv.Itoa(len(data)))
	resp.Header.Del("Content-Encoding")
	resp.Header.Del("Transfer-Encoding")
}

// matchRoute returns the route with the longest matching prefix.
func (p *Proxy) matchRoute(routes []*Route, path string) *Route {
	var best *Route
	for _, r := range routes {
		if strings.HasPrefix(path, r.PathPrefix) {
			if best == nil || len(r.PathPrefix) > len(best.PathPrefix) {
				best = r
			}
		}
	}
	return best
}

// ---------------------------------------------------------------------------
// Route builder
// ---------------------------------------------------------------------------

// buildAllRoutes constructs the full route table: plain inbound forwards
// from proxied_through edges plus contract-checked flow routes from
// data_flow edges. The combined table is sorted by path prefix length
// descending (longest prefix wins).
func (p *Proxy) buildAllRoutes(g *graph.Graph) ([]*Route, error) {
	routes, err := buildRoutes(g)
	if err != nil {
		return nil, err
	}
	flowRoutes, err := p.buildFlowRoutes(g)
	if err != nil {
		return nil, err
	}
	routes = append(routes, flowRoutes...)

	// Sort by prefix length descending (longest prefix wins)
	for i := 0; i < len(routes); i++ {
		for j := i + 1; j < len(routes); j++ {
			if len(routes[j].PathPrefix) > len(routes[i].PathPrefix) {
				routes[i], routes[j] = routes[j], routes[i]
			}
		}
	}

	return routes, nil
}

// buildFlowRoutes constructs contract-checked east-west routes from
// data_flow edges: /_flow/<from>/<to>/* → strip prefix → forward to the
// target node's port. Edges whose target has no port are skipped — there
// is nowhere to forward to.
func (p *Proxy) buildFlowRoutes(g *graph.Graph) ([]*Route, error) {
	edges := g.EdgesOfType(config.EdgeDataFlow)
	routes := make([]*Route, 0, len(edges))

	for _, e := range edges {
		target := g.Node(e.To)
		if target == nil || target.Port == 0 {
			continue
		}

		prefix := fmt.Sprintf("/_flow/%s/%s", e.From, e.To)

		targetURL, err := url.Parse(fmt.Sprintf("http://localhost:%d", target.Port))
		if err != nil {
			return nil, fmt.Errorf("invalid flow target for edge %s→%s: %w", e.From, e.To, err)
		}

		rp := httputil.NewSingleHostReverseProxy(targetURL)
		rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			output.Warn(target.ID, "flow proxy error for %s %s: %v", r.Method, r.URL.Path, err)
			http.Error(w, fmt.Sprintf("service %q is unavailable", target.ID), http.StatusBadGateway)
		}
		rp.ModifyResponse = func(resp *http.Response) error {
			if err := p.checkResponseContract(resp); err != nil {
				return err
			}
			resp.Header.Set("X-Acthur-Node", target.ID)
			return nil
		}

		routes = append(routes, &Route{
			PathPrefix: prefix,
			NodeID:     target.ID,
			Target:     targetURL,
			Proxy:      rp,
			Kind:       routeKindFlow,
			From:       e.From,
			To:         e.To,
		})

		output.Debug("proxy", "flow route: %s → %s (edge: %s→%s)", prefix, targetURL, e.From, e.To)
	}

	return routes, nil
}

// buildRoutes constructs the route table from proxied_through edges.
// Routes are sorted by path prefix length descending (longest first).
func buildRoutes(g *graph.Graph) ([]*Route, error) {
	proxiedNodes := g.ProxiedNodes()
	routes := make([]*Route, 0, len(proxiedNodes))

	for _, node := range proxiedNodes {
		if node.Port == 0 {
			continue // node has no port — skip
		}

		// Derive path prefix from node ID
		// Conventions:
		//   "api"        → /api
		//   "web"        → /          (catch-all, shortest prefix)
		//   "backoffice" → /backoffice
		//   "admin"      → /admin
		prefix := pathPrefixFor(node)

		target, err := url.Parse(fmt.Sprintf("http://localhost:%d", node.Port))
		if err != nil {
			return nil, fmt.Errorf("invalid target for node %q: %w", node.ID, err)
		}

		rp := httputil.NewSingleHostReverseProxy(target)
		rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			output.Warn(node.ID, "proxy error for %s %s: %v", r.Method, r.URL.Path, err)
			http.Error(w, fmt.Sprintf("service %q is unavailable", node.ID), http.StatusBadGateway)
		}

		// Modify response to add node identifier header
		rp.ModifyResponse = func(resp *http.Response) error {
			resp.Header.Set("X-Acthur-Node", node.ID)
			return nil
		}

		routes = append(routes, &Route{
			PathPrefix: prefix,
			NodeID:     node.ID,
			Target:     target,
			Proxy:      rp,
			Kind:       routeKindProxied,
		})

		output.Debug("proxy", "route: %s → %s (node: %s)", prefix, target, node.ID)
	}

	// Sort by prefix length descending (longest prefix wins)
	for i := 0; i < len(routes); i++ {
		for j := i + 1; j < len(routes); j++ {
			if len(routes[j].PathPrefix) > len(routes[i].PathPrefix) {
				routes[i], routes[j] = routes[j], routes[i]
			}
		}
	}

	return routes, nil
}

// pathPrefixFor returns the URL path prefix for a node.
// Nodes named "web", "frontend", "app" get the root path ("/").
// All others get "/<node-id>".
func pathPrefixFor(node *graph.Node) string {
	switch node.ID {
	case "web", "frontend", "app", "www":
		return "/"
	default:
		return "/" + node.ID
	}
}

// ---------------------------------------------------------------------------
// WebSocket support
// ---------------------------------------------------------------------------

// isWebSocketUpgrade returns true if the request is a WebSocket upgrade.
// WebSocket connections (used by Vite/Next.js HMR) must be proxied
// without modification.
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}
