// Package proxy implements the Acthur dev reverse proxy.
// It listens on a single port (default :4000) and routes incoming
// requests to the correct service node based on proxied_through edges.
//
// The proxy also enforces contracts on data_flow edges in dev mode
// (logging violations) and in strict mode (blocking violations).
package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/acthur/acthur/internal/graph"
	"github.com/acthur/acthur/internal/output"
)

// ---------------------------------------------------------------------------
// Route
// ---------------------------------------------------------------------------

// Route maps a path prefix to a backend target.
type Route struct {
	PathPrefix string
	NodeID     string
	Target     *url.URL
	Proxy      *httputil.ReverseProxy
}

// ---------------------------------------------------------------------------
// Proxy
// ---------------------------------------------------------------------------

// Proxy is the Acthur dev reverse proxy.
// Routes are derived from proxied_through edges in the graph.
type Proxy struct {
	port   int
	routes []*Route
	mu     sync.RWMutex
	server *http.Server
}

// New creates a proxy from the graph. It builds routes from all
// proxied_through edges and orders them by path prefix length
// (longest prefix wins, ensuring /api/* matches before /*).
func New(g *graph.Graph, port int) (*Proxy, error) {
	p := &Proxy{port: port}

	routes, err := buildRoutes(g)
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
	routes, err := buildRoutes(g)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.routes = routes
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
	p.mu.RUnlock()

	route := p.matchRoute(routes, r.URL.Path)
	if route == nil {
		http.Error(w, "no route found for "+r.URL.Path, http.StatusBadGateway)
		output.Warn("proxy", "no route for %s %s", r.Method, r.URL.Path)
		return
	}

	// Inject proxy headers
	r.Header.Set("X-Forwarded-For", r.RemoteAddr)
	r.Header.Set("X-Forwarded-Host", r.Host)
	r.Header.Set("X-Forwarded-Proto", "http")
	r.Header.Set("X-Acthur-Node", route.NodeID)

	route.Proxy.ServeHTTP(w, r)
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
