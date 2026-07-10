// Package admin is the built-in "admin" plugin (Phase 9, tracker #65). For
// The go:fiber service nodes it generates internal/admin/admin.go: a
// read-only operational dashboard (identity/uptime + this process's
// feature-flag state) at GET /admin (HTML) and /admin/api/* (JSON).
//
// Honest scope: see the doc comment on the generated template's Mount
// function (templates/admin.go.tmpl) for why a full CRUD data-model editor
// (the PRD's "data management interface for all models") isn't
// implemented — Acthur has no generic model/ORM registry to introspect
// safely today.
package admin

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"

	"github.com/acthurhq/acthur/internal/plugin"
)

//go:embed templates/admin.go.tmpl
var tmplFile embed.FS

var tmpl = template.Must(template.ParseFS(tmplFile, "templates/admin.go.tmpl"))

// ---------------------------------------------------------------------------
// Plugin
// ---------------------------------------------------------------------------

type adminPlugin struct{}

func (adminPlugin) Name() string        { return "admin" }
func (adminPlugin) Version() string     { return "0.1.0" }
func (adminPlugin) DependsOn() []string { return nil }

func (adminPlugin) Register(k plugin.KernelAPI) error {
	k.RegisterGenerator("admin", generator{})

	k.OnEvent(plugin.EventAfterNodeHealthy, func(p plugin.EventPayload) {
		k.Log(plugin.LogInfo, "admin: node %q healthy — dashboard mounted at /admin", p.NodeID)
	})

	return nil
}

func init() {
	plugin.Register(adminPlugin{})
}

// ---------------------------------------------------------------------------
// Generator
// ---------------------------------------------------------------------------

type generator struct{}

func (generator) SupportedAdapters() []string { return []string{"go:fiber"} }

func (generator) Generate(adapterName string, ctx plugin.GeneratorContext) ([]plugin.GeneratedFile, error) {
	if adapterName != "go:fiber" {
		return nil, fmt.Errorf("admin generator does not support adapter %q (supported: go:fiber)", adapterName)
	}

	nodeID := ctx.NodeID
	if nodeID == "" {
		nodeID = "service"
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, struct{ NodeID string }{NodeID: nodeID}); err != nil {
		return nil, fmt.Errorf("rendering admin.go: %w", err)
	}

	return []plugin.GeneratedFile{{
		Path:      "internal/admin/admin.go",
		Content:   buf.Bytes(),
		Mode:      0o644,
		Overwrite: false,
	}}, nil
}
