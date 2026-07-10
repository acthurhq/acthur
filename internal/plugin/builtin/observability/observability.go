// Package observability is the built-in "observability" plugin (Phase 9,
// tracker #65). For go:fiber service nodes it generates
// internal/observability/observability.go: real Prometheus counters and a
// histogram recording every request (method/path/status/duration), and a
// GET /metrics endpoint serving them — PRD §28's "metrics" pillar.
//
// Honest scope: the PRD's full description is a three-pillar OpenTelemetry
// stack (traces via Jaeger/Tempo, metrics via Prometheus+Grafana, logs via
// Loki+Grafana), auto-instrumented DB/cache calls, and pre-built Grafana
// dashboards per adapter. This plugin implements the metrics pillar for
// real (a working /metrics endpoint a real Prometheus can scrape, not a
// stub), and deliberately does not implement: distributed tracing
// (Jaeger/Tempo — would require the OTel SDK's tracer provider + an
// exporter + a Jaeger/Tempo infra node this plugin has no seam to
// provision), the Loki logs pillar, or generated Grafana dashboards. These
// are recorded here as an explicit descope, not a silent gap — see the
// status note in docs/implementation/active/0007-prd-completion.md.
package observability

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"

	"github.com/acthurhq/acthur/internal/plugin"
)

//go:embed templates/observability.go.tmpl
var tmplFile embed.FS

var tmpl = template.Must(template.ParseFS(tmplFile, "templates/observability.go.tmpl"))

// ---------------------------------------------------------------------------
// Plugin
// ---------------------------------------------------------------------------

type observabilityPlugin struct{}

func (observabilityPlugin) Name() string        { return "observability" }
func (observabilityPlugin) Version() string     { return "0.1.0" }
func (observabilityPlugin) DependsOn() []string { return nil }

func (observabilityPlugin) Register(k plugin.KernelAPI) error {
	k.RegisterGenerator("observability", generator{})

	k.OnEvent(plugin.EventAfterNodeHealthy, func(p plugin.EventPayload) {
		k.Log(plugin.LogInfo, "observability: node %q healthy — metrics available at /metrics", p.NodeID)
	})

	return nil
}

func init() {
	plugin.Register(observabilityPlugin{})
}

// ---------------------------------------------------------------------------
// Generator
// ---------------------------------------------------------------------------

type generator struct{}

func (generator) SupportedAdapters() []string { return []string{"go:fiber"} }

func (generator) Generate(adapterName string, ctx plugin.GeneratorContext) ([]plugin.GeneratedFile, error) {
	if adapterName != "go:fiber" {
		return nil, fmt.Errorf("observability generator does not support adapter %q (supported: go:fiber)", adapterName)
	}

	nodeID := ctx.NodeID
	if nodeID == "" {
		nodeID = "service"
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, struct{ NodeID string }{NodeID: nodeID}); err != nil {
		return nil, fmt.Errorf("rendering observability.go: %w", err)
	}

	return []plugin.GeneratedFile{{
		Path:      "internal/observability/observability.go",
		Content:   buf.Bytes(),
		Mode:      0o644,
		Overwrite: false,
	}}, nil
}
