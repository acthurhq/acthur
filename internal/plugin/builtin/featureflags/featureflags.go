// Package featureflags is the built-in "feature-flags" plugin (Phase 9,
// tracker #65). It does not reimplement flag storage — that's
// internal/flags's job, already backing `acthur flag create/list/toggle`
// and already wired into the dev engine (every node process gets
// ACTHUR_FLAG_<NAME>=true|false for every flag, unconditionally — see
// internal/engine/dev.go's buildEnv). This plugin's job is the generated,
// in-service half: a typed Go client (internal/flags/flags.go, generated
// into the target project) that reads those env vars and a fiber
// middleware that gates a route behind a flag — the "flag evaluation
// middleware and typed flag registry" the PRD names for this plugin.
package featureflags

import (
	"bytes"
	"embed"
	"fmt"
	"sort"
	"text/template"

	acthurflags "github.com/acthur/acthur/internal/flags"
	"github.com/acthur/acthur/internal/plugin"
)

//go:embed templates/flags.go.tmpl
var tmplFile embed.FS

var tmpl = template.Must(template.ParseFS(tmplFile, "templates/flags.go.tmpl"))

// ---------------------------------------------------------------------------
// Plugin
// ---------------------------------------------------------------------------

type featureFlagsPlugin struct{}

func (featureFlagsPlugin) Name() string        { return "feature-flags" }
func (featureFlagsPlugin) Version() string     { return "0.1.0" }
func (featureFlagsPlugin) DependsOn() []string { return nil }

func (featureFlagsPlugin) Register(k plugin.KernelAPI) error {
	k.RegisterGenerator("feature-flags", generator{})

	// Hook: before a node starts, report how many project flags are
	// currently enabled — a real, observable effect of the plugin being
	// active, using the project's actual .acthur/flags.json state (not a
	// static message). root_dir is only present when the engine has a real
	// project root (see internal/engine/dev.go's emit) — a bare unit-test
	// DevEngine never fires this.
	k.OnEvent(plugin.EventBeforeNodeStart, func(p plugin.EventPayload) {
		rootDir, _ := p.Data["root_dir"].(string)
		if rootDir == "" {
			return
		}
		store := acthurflags.New(rootDir)
		list, err := store.List()
		if err != nil {
			return
		}
		enabled := 0
		for _, f := range list {
			if f.Enabled {
				enabled++
			}
		}
		k.Log(plugin.LogInfo, "feature-flags: %d/%d flags enabled (node %q starting)", enabled, len(list), p.NodeID)
	})

	return nil
}

func init() {
	plugin.Register(featureFlagsPlugin{})
}

// ---------------------------------------------------------------------------
// Generator
// ---------------------------------------------------------------------------

type generator struct{}

func (generator) SupportedAdapters() []string { return []string{"go:fiber"} }

// Generate renders internal/flags/flags.go. Config key "flags" ([]string,
// or []any of strings from YAML) seeds the typed Registry — purely
// informational (Enabled/Require work for any flag name regardless of
// whether it's listed), but gives callers a compile-time-checkable list
// mirroring the flags actually declared via `acthur flag create`.
func (generator) Generate(adapterName string, ctx plugin.GeneratorContext) ([]plugin.GeneratedFile, error) {
	if adapterName != "go:fiber" {
		return nil, fmt.Errorf("feature-flags generator does not support adapter %q (supported: go:fiber)", adapterName)
	}

	var flagNames []string
	if ctx.Config != nil {
		if raw, ok := ctx.Config["flags"]; ok {
			switch v := raw.(type) {
			case []string:
				flagNames = append(flagNames, v...)
			case []any:
				for _, item := range v {
					if s, ok := item.(string); ok {
						flagNames = append(flagNames, s)
					}
				}
			}
		}
	}
	sort.Strings(flagNames)

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, struct{ Flags []string }{Flags: flagNames}); err != nil {
		return nil, fmt.Errorf("rendering flags.go: %w", err)
	}

	return []plugin.GeneratedFile{{
		Path:      "internal/flags/flags.go",
		Content:   buf.Bytes(),
		Mode:      0o644,
		Overwrite: false,
	}}, nil
}
