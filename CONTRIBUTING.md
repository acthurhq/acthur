# Contributing to Acthur

Thank you for your interest in contributing. Acthur is MIT-licensed and actively welcomes contributions across all four layers: the kernel, adapters, plugins, and documentation.

---

## Before You Start

Read the [PRD](./docs/PRD.md) to understand the architecture. Specifically, make sure you understand:

- [The Four-Layer Model](./docs/PRD.md#4-the-four-layer-model) — where your contribution belongs
- [The Communication Law](./docs/PRD.md#41-the-communication-law) — what each layer is allowed to do
- The strict separation between Plugins, Adapters, and generated code

If you're unsure where something belongs, open an issue before writing code.

---

## Development Setup

```bash
# Prerequisites
go 1.22+
docker (for integration tests)
make

# Clone and build
git clone https://github.com/samueloshio/acthur
cd acthur
go mod download
make build

# Verify
./bin/acthur doctor
./bin/acthur version

# Run tests
make test           # unit tests only (fast)
make test-race      # with race detector
```

---

## Adding a Backend Adapter

An adapter implements 8 methods. It knows exactly one thing: how to operate its framework.

**Step 1: Create the package**

```
internal/adapter/backend/<runtime>-<framework>/
└── <framework>.go
```

**Step 2: Implement the interface**

```go
package myadapter

import "github.com/samueloshio/acthur/internal/adapter"

type Adapter struct{}

func init() { adapter.Register(&Adapter{}) }

func (a *Adapter) Name() string            { return "go:myframework" }
func (a *Adapter) Category() adapter.Category { return adapter.CategoryBackend }
func (a *Adapter) Detect(dir string) bool  { /* check go.mod for the framework */ }
func (a *Adapter) Scaffold(ctx adapter.ScaffoldContext) ([]adapter.File, error) { /* ... */ }
func (a *Adapter) DevCommand(env map[string]string) adapter.Command { /* ... */ }
func (a *Adapter) BuildCommand(env map[string]string) adapter.Command { /* ... */ }
func (a *Adapter) TestCommand(env map[string]string) adapter.Command { /* ... */ }
func (a *Adapter) GeneratorTargets() []string { /* what code generation targets it accepts */ }
func (a *Adapter) Dockerfile(cfg adapter.BuildConfig) string { /* multi-stage Dockerfile */ }
func (a *Adapter) EnvVars() []adapter.EnvVar { /* required env vars */ }
```

**Step 3: Write tests**

See `internal/adapter/adapter_test.go` for the test pattern. Every adapter must pass:
- `TestXxx_Name` — correct name format
- `TestXxx_Detect_True/False` — detection works
- `TestXxx_DevCommand` — returns non-empty command
- `TestXxx_Scaffold_ProducesFiles` — scaffold returns valid files
- `TestXxx_Dockerfile_IsMultiStage` — Dockerfile has build + run stages

**Step 4: Register in commands.go**

Add a blank import of your adapter package to `cmd/acthur/commands.go` so its `init()` runs.

**Rules for adapters:**
- NEVER import from `internal/plugin/` — adapters do not know plugins exist
- NEVER import from `internal/contract/` — adapters do not know contracts exist
- NEVER call the kernel — adapters only implement their interface

---

## Adding a Plugin

A plugin implements 3 methods and uses `KernelAPI` to install behavior.

**Step 1: Create the package**

```
internal/plugin/<name>/
├── plugin.go
└── plugin_test.go
```

**Step 2: Implement the interface**

```go
package myplugin

import "github.com/samueloshio/acthur/internal/plugin"

type Plugin struct{}

func init() { plugin.Register(&Plugin{}) }

func (p *Plugin) Name() string        { return "my-feature" }
func (p *Plugin) Version() string     { return "1.0.0" }
func (p *Plugin) DependsOn() []string { return []string{} }

func (p *Plugin) Register(k plugin.KernelAPI) error {
    k.OnEvent(plugin.EventAfterGraphBuild, p.onGraphBuild)
    k.RegisterCommand(plugin.CLICommand{
        Use:   "add my-feature",
        Short: "Add my feature to the project",
        Run:   p.runAdd,
    })
    return nil
}
```

**Step 3: Write generators**

Plugins generate framework-native code. Add templates to `templates/<adapter>/`:

```
templates/go:fiber/my-feature/
├── handler.go.tmpl
└── migration.sql.tmpl
```

**Step 4: Tests**

Tests must verify:
- Plugin registers without error
- Hooks fire at the correct events
- Generated code compiles for the target adapter
- Generated code contains zero Acthur imports

**Rules for plugins:**
- NEVER import from `internal/adapter/backend/*/` — plugins do not call adapters
- Generated code must be pure framework code — no `github.com/samueloshio/` imports
- All plugin capabilities go through `KernelAPI` — nothing else

---

## Adding a Deploy Target

A deploy target renders the graph as a platform-specific manifest.

```go
type DeployTarget interface {
    Name() string
    Render(g *graph.Graph, cfg *config.Config) (Manifest, error)
}
```

See `internal/engine/deploy.go` for the interface and an example implementation.

---

## Commit Style

Use [Conventional Commits](https://www.conventionalcommits.org):

```
feat(adapter): add go:echo adapter
fix(graph): correct cycle detection for depends_on chains
test(contract): add breaking change detection tests
docs(readme): update CLI reference
refactor(plugin): simplify plugin loader topological sort
```

Types: `feat`, `fix`, `test`, `docs`, `refactor`, `chore`, `perf`

---

## Pull Request Checklist

- [ ] Tests pass: `make test`
- [ ] Race detector clean: `make test-race`
- [ ] Lint passes: `make lint`
- [ ] New adapter: implements all 8 methods + tests
- [ ] New plugin: uses KernelAPI only + generated code has no Acthur imports
- [ ] No cross-layer violations (adapter → plugin, plugin → adapter)
- [ ] CLAUDE.md or relevant docs updated if architecture changes

---

## Getting Help

Open a GitHub Discussion for architecture questions. Open an Issue for bugs.

For questions about where something belongs in the four-layer model, the answer is almost always in the PRD — but feel free to ask.
