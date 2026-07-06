package main

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/acthur/acthur/internal/deploy"
	"github.com/acthur/acthur/internal/deploy/artifacts"
	"github.com/acthur/acthur/internal/deploy/coolify"
	"github.com/acthur/acthur/internal/generate"
	"github.com/acthur/acthur/internal/graph"
	"github.com/acthur/acthur/internal/output"
	"github.com/acthur/acthur/internal/plugin"
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
	files, err := artifacts.Project(cfg, g)
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

	// Gate before anything ships.
	report, err := deploy.RunGate(deploy.GateInput{
		Root:         root,
		ServiceNodes: serviceNodes,
		RequiredEnv:  requiredEnv,
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
	default:
		return plan, fmt.Errorf("unknown deploy target %q (supported: compose, coolify)", target)
	}
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
	if target == "coolify" && !seen["COOLIFY_TOKEN"] {
		vars = append(vars, "COOLIFY_TOKEN")
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
