package fly

import (
	"fmt"
	"time"

	"github.com/acthurhq/acthur/internal/deploy"
)

// ServiceArtifact describes one Dockerizable service node the target must
// build, push, and run as a Fly Machine.
type ServiceArtifact struct {
	// NodeID is the graph node ID (e.g. "api") — also used as the Fly
	// machine name so redeploys update it in place.
	NodeID string
	// DockerfilePath is the path to the node's rendered production
	// Dockerfile, relative to Root().
	DockerfilePath string
	// ContextDir is the docker build context for this service, relative
	// to Root().
	ContextDir string
	// Port is the service's internal port, or 0 for a portless node (no
	// public service block is configured for those).
	Port int
}

// DeployContext is the small, obvious surface the deploy command hands to
// a Target — kept self-contained (no dependency on the CLI-assembly layer)
// so this package can move to its own repo later, same as coolify.
type DeployContext interface {
	// EnvName is the environment being deployed (e.g. "production").
	EnvName() string
	// ProjectName is the Acthur project name (acthur.yml `project:`), used
	// to derive the Fly app name.
	ProjectName() string
	// Root is the absolute path to the project root, used to resolve each
	// service's Dockerfile/build context on disk.
	Root() string
	// Services lists every Dockerizable service node to deploy.
	Services() []ServiceArtifact
	// Log emits a progress line.
	Log(format string, args ...any)
}

// Target deploys a project to Fly.io: ensure the app exists, build+push
// each service's image to Fly's own registry, run it as a Fly Machine, and
// wait for it to report started.
type Target struct {
	// Token is the Fly API token. The caller resolves it (e.g. from
	// FLY_API_TOKEN) — Target never reads the environment.
	Token string
	// OrgSlug is the Fly organization to create the app under. Empty
	// defaults to "personal".
	OrgSlug string
	// BaseURL overrides the Machines API base URL; empty uses the real
	// API. Tests point this at an httptest server.
	BaseURL string

	// Run executes docker CLI invocations (build/login/push). Injected for
	// testability; production wires deploy.DockerRunner.
	Run deploy.Runner

	// PollInterval and StartTimeout control WaitStarted behavior. Zero
	// values fall back to Client defaults (PollInterval) and a 5-minute
	// timeout (StartTimeout) respectively.
	PollInterval time.Duration
	StartTimeout time.Duration
}

// NewTarget constructs a fly Target with the given API token and docker
// Runner.
func NewTarget(token string, run deploy.Runner) *Target {
	return &Target{Token: token, Run: run, StartTimeout: 5 * time.Minute}
}

// Deploy implements the deploy command's target interface.
func (t *Target) Deploy(ctx DeployContext) error {
	if t.Run == nil {
		return fmt.Errorf("fly target: no docker runner configured")
	}

	appName := sanitizeAppName(ctx.ProjectName() + "-" + ctx.EnvName())
	client := New(t.Token)
	if t.BaseURL != "" {
		client.BaseURL = t.BaseURL
	}
	if t.PollInterval > 0 {
		client.PollInterval = t.PollInterval
	}

	ctx.Log("fly: ensuring app %q exists", appName)
	if err := client.EnsureApp(appName, t.OrgSlug); err != nil {
		return fmt.Errorf("fly target: %w", err)
	}

	registry := "registry.fly.io/" + appName
	ctx.Log("fly: authenticating with %s", registry)
	if _, err := t.Run("login", "registry.fly.io", "-u", "x", "-p", t.Token); err != nil {
		return fmt.Errorf("fly target: docker login: %w", err)
	}

	services := ctx.Services()
	if len(services) == 0 {
		return fmt.Errorf("fly target: no Dockerizable service nodes to deploy")
	}

	timeout := t.StartTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	for _, svc := range services {
		image := fmt.Sprintf("%s:%s", registry, svc.NodeID)

		ctx.Log("fly: building %s from %s", image, svc.DockerfilePath)
		if _, err := t.Run("build", "-f", ctx.Root()+"/"+svc.DockerfilePath, "-t", image, ctx.Root()+"/"+svc.ContextDir); err != nil {
			return fmt.Errorf("fly target: building %q: %w", svc.NodeID, err)
		}

		ctx.Log("fly: pushing %s", image)
		if _, err := t.Run("push", image); err != nil {
			return fmt.Errorf("fly target: pushing %q: %w", svc.NodeID, err)
		}

		cfg := MachineConfig{
			Guest: MachineGuest{CPUKind: "shared", CPUs: 1, MemoryMB: 256},
		}
		if svc.Port != 0 {
			cfg.Services = []MachineSvcConfig{{
				Protocol:     "tcp",
				InternalPort: svc.Port,
				Ports: []MachinePort{
					{Port: 443, Handlers: []string{"tls", "http"}},
					{Port: 80, Handlers: []string{"http"}},
				},
				Checks: []MachineChecks{{
					Type:     "http",
					Path:     "/health",
					Interval: "10s",
					Timeout:  "3s",
				}},
			}}
		}

		ctx.Log("fly: ensuring machine %q runs %s", svc.NodeID, image)
		machineID, err := client.EnsureMachine(appName, svc.NodeID, image, cfg)
		if err != nil {
			return fmt.Errorf("fly target: %w", err)
		}

		ctx.Log("fly: waiting for machine %q to start (timeout %s)", svc.NodeID, timeout)
		if err := client.WaitStarted(appName, machineID, timeout); err != nil {
			return fmt.Errorf("fly target: %w", err)
		}
	}

	ctx.Log("fly: app %q is up", appName)
	return nil
}

// sanitizeAppName lowercases and replaces any character Fly app names
// disallow (only lowercase alphanumerics and hyphens) with a hyphen.
func sanitizeAppName(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			out = append(out, r)
		case r >= 'A' && r <= 'Z':
			out = append(out, r-'A'+'a')
		default:
			out = append(out, '-')
		}
	}
	return string(out)
}
