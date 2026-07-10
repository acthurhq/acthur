package main

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/acthurhq/acthur/internal/deploy"
	"github.com/acthurhq/acthur/internal/deploy/artifacts"
	"github.com/acthurhq/acthur/internal/deploy/coolify"
	"github.com/acthurhq/acthur/internal/deploy/fly"
	"github.com/acthurhq/acthur/internal/deploy/railway"
	"github.com/acthurhq/acthur/internal/deploy/render"
	"github.com/acthurhq/acthur/internal/generate"
	"github.com/acthurhq/acthur/internal/graph"
	"github.com/acthurhq/acthur/internal/output"
	"github.com/acthurhq/acthur/internal/plugin"
	"github.com/acthurhq/acthur/internal/secrets"
)

// ---------------------------------------------------------------------------
// acthur deploy — Phase 8 Slice 2. One command: pre-deploy gate → artifact
// projection (Dockerfiles + docker-compose.prod.yml through the write
// engine) → target (compose locally, coolify remotely).
// ---------------------------------------------------------------------------

var composeVarRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// runDeploy executes (or, with dryRun, plans) a deploy of env. The docker
// runner is injected for testability; the command passes deploy.DockerRunner.
func runDeploy(root, env, targetOverride string, dryRun bool, run deploy.Runner) ([]string, error) {
	cfg, g, err := loadProjectGraph(root)
	if err != nil {
		return nil, err
	}

	// Resolve environment + target. Projects that define environments must
	// name one of them; a project without an environments block deploys to
	// the local compose target.
	target := targetOverride
	host := ""
	if len(cfg.Environments) > 0 {
		envCfg, ok := cfg.Environments[env]
		if !ok {
			var defined []string
			for name := range cfg.Environments {
				defined = append(defined, name)
			}
			sort.Strings(defined)
			return nil, fmt.Errorf("environment %q is not defined in acthur.yml (defined: %s)", env, strings.Join(defined, ", "))
		}
		if target == "" {
			target = string(envCfg.Target)
		}
		host = envCfg.Host
	}
	if target == "" {
		target = "compose"
	}

	// Project the artifacts (in memory — nothing written yet).
	files, err := artifacts.Project(cfg, g, root)
	if err != nil {
		return nil, fmt.Errorf("projecting deploy artifacts: %w", err)
	}

	serviceNodes := buildableServiceNodes(root, g)
	requiredEnv := requiredEnvVars(files, target)

	plan := []string{
		fmt.Sprintf("environment: %s", env),
		fmt.Sprintf("target: %s", target),
		fmt.Sprintf("gate: build+test %s; env vars %v", strings.Join(serviceNodes, ", "), requiredEnv),
	}
	for _, f := range files {
		plan = append(plan, "artifact: "+f.Path)
	}
	if dryRun {
		return plan, nil
	}

	// Gate before anything ships. A required var not exported into this shell
	// falls back to the project's local secret store (acthur secrets set) —
	// convenient for local `acthur deploy` dev-loop testing; CI/production
	// deploy environments are expected to export real vars, which are always
	// checked first and win.
	secretStore := secrets.New(root)
	report, err := deploy.RunGate(deploy.GateInput{
		Root:         root,
		ServiceNodes: serviceNodes,
		RequiredEnv:  requiredEnv,
		ResolveSecret: func(key string) (string, bool) {
			v, err := secretStore.Get(key)
			if err != nil {
				return "", false
			}
			return v, true
		},
	})
	if err != nil {
		return plan, err
	}
	for _, c := range report.Checks {
		output.Info("deploy", "gate ✓ %s", c.Name)
	}

	// Persist artifacts through the write engine (generated.lock semantics).
	results, err := generate.WriteFiles(root, "", files)
	if err != nil {
		return plan, err
	}
	printGenerateResults(results)

	switch target {
	case "compose":
		ct := deploy.NewComposeTarget(root+"/deploy/docker-compose.prod.yml", run)
		output.Info("deploy", "compose: building images and starting the stack")
		if err := ct.Up(); err != nil {
			return plan, err
		}
		if err := ct.WaitHealthy(2*time.Minute, 2*time.Second); err != nil {
			return plan, err
		}
		output.Success("deploy", "stack is healthy")
		return plan, nil
	case "coolify":
		compose := composeContent(files)
		if compose == nil {
			return plan, fmt.Errorf("no docker-compose.prod.yml artifact projected — cannot deploy to coolify")
		}
		ctx := &coolifyContext{env: env, compose: compose, project: cfg.Project, host: host}
		return plan, coolify.NewTarget(os.Getenv("COOLIFY_TOKEN")).Deploy(ctx)
	case "fly":
		svcs := serviceArtifacts(files, g)
		ctx := &flyContext{env: env, project: cfg.Project, root: root, services: svcs}
		return plan, fly.NewTarget(os.Getenv("FLY_API_TOKEN"), run).Deploy(ctx)
	case "railway":
		svcs := serviceArtifacts(files, g)
		ctx := &railwayContext{env: env, project: cfg.Project, root: root, services: svcs}
		return plan, railway.NewTarget(os.Getenv("RAILWAY_TOKEN"), os.Getenv("RAILWAY_IMAGE_REGISTRY"), run).Deploy(ctx)
	case "render":
		svcs := serviceArtifacts(files, g)
		ctx := &renderContext{env: env, project: cfg.Project, root: root, services: svcs}
		return plan, render.NewTarget(os.Getenv("RENDER_API_KEY"), os.Getenv("RENDER_IMAGE_REGISTRY"), run).Deploy(ctx)
	default:
		return plan, fmt.Errorf("unknown deploy target %q (supported: compose, coolify, fly, railway, render)", target)
	}
}

// serviceArtifact is the generic (provider-agnostic) shape every remote
// target's DeployContext.Services() adapts into its own ServiceArtifact
// type — computed once here from the projected Dockerfile artifacts and the
// graph's node ports.
type serviceArtifact struct {
	NodeID         string
	DockerfilePath string
	ContextDir     string
	Port           int
}

// serviceArtifacts extracts one entry per projected deploy/Dockerfile.<node>
// artifact, matched against the graph for that node's port.
func serviceArtifacts(files []plugin.GeneratedFile, g *graph.Graph) []serviceArtifact {
	ports := map[string]int{}
	for _, n := range g.Nodes() {
		ports[n.ID] = n.Port
	}

	const prefix = "deploy/Dockerfile."
	var out []serviceArtifact
	for _, f := range files {
		if !strings.HasPrefix(f.Path, prefix) {
			continue
		}
		nodeID := strings.TrimPrefix(f.Path, prefix)
		out = append(out, serviceArtifact{
			NodeID:         nodeID,
			DockerfilePath: f.Path,
			ContextDir:     nodeID,
			Port:           ports[nodeID],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NodeID < out[j].NodeID })
	return out
}

// flyContext, railwayContext, and renderContext each adapt the deploy
// command's resolved state to that package's own DeployContext interface —
// every provider package is self-contained (no shared type), so each gets
// its own small adapter, same as coolifyContext.

type flyContext struct {
	env      string
	project  string
	root     string
	services []serviceArtifact
}

func (c *flyContext) EnvName() string     { return c.env }
func (c *flyContext) ProjectName() string { return c.project }
func (c *flyContext) Root() string        { return c.root }
func (c *flyContext) Log(format string, args ...any) {
	output.Info("deploy", format, args...)
}
func (c *flyContext) Services() []fly.ServiceArtifact {
	out := make([]fly.ServiceArtifact, len(c.services))
	for i, s := range c.services {
		out[i] = fly.ServiceArtifact{NodeID: s.NodeID, DockerfilePath: s.DockerfilePath, ContextDir: s.ContextDir, Port: s.Port}
	}
	return out
}

type railwayContext struct {
	env      string
	project  string
	root     string
	services []serviceArtifact
}

func (c *railwayContext) EnvName() string     { return c.env }
func (c *railwayContext) ProjectName() string { return c.project }
func (c *railwayContext) Root() string        { return c.root }
func (c *railwayContext) Log(format string, args ...any) {
	output.Info("deploy", format, args...)
}
func (c *railwayContext) Services() []railway.ServiceArtifact {
	out := make([]railway.ServiceArtifact, len(c.services))
	for i, s := range c.services {
		out[i] = railway.ServiceArtifact{NodeID: s.NodeID, DockerfilePath: s.DockerfilePath, ContextDir: s.ContextDir}
	}
	return out
}

type renderContext struct {
	env      string
	project  string
	root     string
	services []serviceArtifact
}

func (c *renderContext) EnvName() string     { return c.env }
func (c *renderContext) ProjectName() string { return c.project }
func (c *renderContext) Root() string        { return c.root }
func (c *renderContext) Log(format string, args ...any) {
	output.Info("deploy", format, args...)
}
func (c *renderContext) Services() []render.ServiceArtifact {
	out := make([]render.ServiceArtifact, len(c.services))
	for i, s := range c.services {
		out[i] = render.ServiceArtifact{NodeID: s.NodeID, DockerfilePath: s.DockerfilePath, ContextDir: s.ContextDir}
	}
	return out
}

// buildableServiceNodes returns service node IDs that have a Go module
// present under root — the gate builds and tests each.
func buildableServiceNodes(root string, g *graph.Graph) []string {
	var nodes []string
	for _, n := range g.Nodes() {
		if n.Type != "service" {
			continue
		}
		if _, err := os.Stat(root + "/" + n.ID + "/go.mod"); err == nil {
			nodes = append(nodes, n.ID)
		}
	}
	sort.Strings(nodes)
	return nodes
}

// requiredEnvVars extracts every ${VAR} the compose projection references
// (secrets are never baked into artifacts, so they must resolve at deploy
// time), plus target-specific credentials.
func requiredEnvVars(files []plugin.GeneratedFile, target string) []string {
	seen := map[string]bool{}
	var vars []string
	for _, f := range files {
		for _, m := range composeVarRef.FindAllStringSubmatch(string(f.Content), -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				vars = append(vars, m[1])
			}
		}
	}
	addVar := func(v string) {
		if !seen[v] {
			seen[v] = true
			vars = append(vars, v)
		}
	}
	switch target {
	case "coolify":
		addVar("COOLIFY_TOKEN")
	case "fly":
		addVar("FLY_API_TOKEN")
	case "railway":
		addVar("RAILWAY_TOKEN")
		addVar("RAILWAY_IMAGE_REGISTRY")
	case "render":
		addVar("RENDER_API_KEY")
		addVar("RENDER_IMAGE_REGISTRY")
	}
	sort.Strings(vars)
	return vars
}

func composeContent(files []plugin.GeneratedFile) []byte {
	for _, f := range files {
		if strings.HasSuffix(f.Path, "docker-compose.prod.yml") {
			return f.Content
		}
	}
	return nil
}

// coolifyContext adapts the deploy command's resolved state to the
// coolify.DeployContext interface.
type coolifyContext struct {
	env     string
	compose []byte
	project string
	host    string
}

func (c *coolifyContext) EnvName() string     { return c.env }
func (c *coolifyContext) ComposeYAML() []byte { return c.compose }
func (c *coolifyContext) ProjectName() string { return c.project }
func (c *coolifyContext) Host() string        { return c.host }
func (c *coolifyContext) Log(format string, args ...any) {
	output.Info("deploy", format, args...)
}
