// Package security is the built-in "security" plugin (Phase 9, tracker
// #65). For go:fiber service nodes it generates internal/security/
// mounting fiber's own helmet (headers), cors, and limiter (rate limiting)
// middleware — PRD §18.2's "Layer 2 — Request Security". The CORS
// allowlist is derived from the live graph's data_flow edges into the
// target node when ctx.Extra["graph"] is present (set by `acthur add`),
// falling back to plugin config otherwise — see buildAllowedOrigins.
//
// Honest scope: SQL-injection prevention via parameterized-query
// enforcement and input-sanitization DTOs (also named in the PRD's
// security-plugin description) are not generated here — those require a
// generated repository/DTO layer this plugin has no way to introspect or
// own (that's the model/contract generator's concern, not a security
// cross-cutting concern), and audit logging for auth/admin events would
// need a defined event/log sink contract that doesn't exist yet. Recorded
// as a deliberate descope, not a silent gap — see the status note in
// docs/implementation/active/0007-prd-completion.md.
package security

import (
	"bytes"
	"embed"
	"fmt"
	"net/url"
	"sort"
	"text/template"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/graph"
	"github.com/acthurhq/acthur/internal/plugin"
)

//go:embed templates/security.go.tmpl
var tmplFile embed.FS

var tmpl = template.Must(template.ParseFS(tmplFile, "templates/security.go.tmpl"))

const (
	defaultRateLimitMax           = 100
	defaultRateLimitWindowSeconds = 60
)

// ValidateProduction checks the security plugin's deploy-time policy against
// the production graph. Generation remains responsible for emitting
// middleware; this function rejects configuration that would make that
// middleware unsafe or unavailable in production.
func ValidateProduction(pluginConfig map[string]any, g *graph.Graph) error {
	if g == nil {
		return fmt.Errorf("production graph is required for security validation")
	}
	for _, node := range g.Nodes() {
		if node.Type == config.NodeTypeService && node.Adapter != "go:fiber" {
			return fmt.Errorf("service %q uses unsupported adapter %q for security plugin (supported: go:fiber)", node.ID, node.Adapter)
		}
	}
	origins, err := configuredOrigins(pluginConfig)
	if err != nil {
		return err
	}
	for _, origin := range origins {
		if origin == "*" {
			return fmt.Errorf("wildcard CORS origin is not allowed in production")
		}
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("CORS origin %q must be a valid absolute http or https origin without a path, query, credentials, or fragment", origin)
		}
	}
	if raw, supplied := pluginConfig["rate_limit_max"]; supplied {
		value, ok := raw.(int)
		if !ok || value <= 0 {
			return fmt.Errorf("rate_limit_max must be a positive integer")
		}
	}
	if raw, supplied := pluginConfig["rate_limit_window_seconds"]; supplied {
		value, ok := raw.(int)
		if !ok || value <= 0 {
			return fmt.Errorf("rate_limit_window_seconds must be a positive integer")
		}
	}
	return nil
}

func configuredOrigins(pluginConfig map[string]any) ([]string, error) {
	if pluginConfig == nil {
		return nil, nil
	}
	raw, supplied := pluginConfig["allowed_origins"]
	if !supplied {
		return nil, nil
	}
	switch origins := raw.(type) {
	case []string:
		return origins, nil
	case []any:
		out := make([]string, 0, len(origins))
		for _, raw := range origins {
			origin, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("allowed_origins must contain only strings")
			}
			out = append(out, origin)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("allowed_origins must be a list of strings")
	}
}

// ---------------------------------------------------------------------------
// Plugin
// ---------------------------------------------------------------------------

type securityPlugin struct{}

func (securityPlugin) Name() string        { return "security" }
func (securityPlugin) Version() string     { return "0.1.0" }
func (securityPlugin) DependsOn() []string { return nil }

func (securityPlugin) Register(k plugin.KernelAPI) error {
	k.RegisterGenerator("security", generator{})

	k.OnEvent(plugin.EventBeforeNodeStart, func(p plugin.EventPayload) {
		k.Log(plugin.LogInfo, "security: headers, CORS allowlist, and rate limiting active for node %q", p.NodeID)
	})

	return nil
}

func init() {
	plugin.Register(securityPlugin{})
}

// ---------------------------------------------------------------------------
// Generator
// ---------------------------------------------------------------------------

type generator struct{}

func (generator) SupportedAdapters() []string { return []string{"go:fiber"} }

type templateData struct {
	AllowedOrigins         []string
	RateLimitMax           int
	RateLimitWindowSeconds int
}

func (generator) Generate(adapterName string, ctx plugin.GeneratorContext) ([]plugin.GeneratedFile, error) {
	if adapterName != "go:fiber" {
		return nil, fmt.Errorf("security generator does not support adapter %q (supported: go:fiber)", adapterName)
	}

	data := templateData{
		AllowedOrigins:         buildAllowedOrigins(ctx),
		RateLimitMax:           defaultRateLimitMax,
		RateLimitWindowSeconds: defaultRateLimitWindowSeconds,
	}
	if ctx.Config != nil {
		if v, ok := ctx.Config["rate_limit_max"].(int); ok && v > 0 {
			data.RateLimitMax = v
		}
		if v, ok := ctx.Config["rate_limit_window_seconds"].(int); ok && v > 0 {
			data.RateLimitWindowSeconds = v
		}
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("rendering security.go: %w", err)
	}

	return []plugin.GeneratedFile{{
		Path:      "internal/security/security.go",
		Content:   buf.Bytes(),
		Mode:      0o644,
		Overwrite: false,
	}}, nil
}

// buildAllowedOrigins derives the CORS allowlist. When ctx.Extra["graph"]
// carries the live *graph.Graph (the "acthur add" convention — see
// cmd/acthur/add.go), it's every distinct DevURL of a node with a
// data_flow edge into ctx.NodeID (PRD §18.2: "Origins = all service nodes
// with proxied_through/data_flow edges" — i.e. real cross-service callers,
// never a wildcard). Falls back to the plugin's "allowed_origins" config
// list, and finally to an empty (deny-all-cross-origin) default — a safe
// default, never *.
func buildAllowedOrigins(ctx plugin.GeneratorContext) []string {
	if g, ok := ctx.Extra["graph"].(*graph.Graph); ok && g != nil {
		seen := map[string]bool{}
		var origins []string
		for _, e := range g.EdgesOfType(config.EdgeDataFlow) {
			if e.To != ctx.NodeID {
				continue
			}
			caller := g.Node(e.From)
			if caller == nil {
				continue
			}
			origin := originFor(caller)
			if origin == "" || seen[origin] {
				continue
			}
			seen[origin] = true
			origins = append(origins, origin)
		}
		if len(origins) > 0 {
			sort.Strings(origins)
			return origins
		}
	}

	if ctx.Config != nil {
		if raw, ok := ctx.Config["allowed_origins"]; ok {
			switch v := raw.(type) {
			case []string:
				sort.Strings(v)
				return v
			case []any:
				var out []string
				for _, item := range v {
					if s, ok := item.(string); ok {
						out = append(out, s)
					}
				}
				sort.Strings(out)
				return out
			}
		}
	}

	return nil
}

func originFor(n *graph.Node) string {
	if n.DevURL != "" {
		return "https://" + n.DevURL
	}
	return ""
}
