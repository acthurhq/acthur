package render

import (
	"fmt"
	"time"

	"github.com/acthur/acthur/internal/deploy"
)

// ServiceArtifact describes one Dockerizable service node the target must
// build, push, and run as a Render web service.
type ServiceArtifact struct {
	// NodeID is the graph node ID (e.g. "api") — also used as the Render
	// service name so redeploys update it in place.
	NodeID string
	// DockerfilePath is the path to the node's rendered production
	// Dockerfile, relative to Root().
	DockerfilePath string
	// ContextDir is the docker build context for this service, relative
	// to Root().
	ContextDir string
}

// DeployContext is the small, obvious surface the deploy command hands to
// a Target — kept self-contained, same as coolify/fly/railway.
type DeployContext interface {
	// EnvName is the environment being deployed (e.g. "production"), used
	// to scope the Render service name.
	EnvName() string
	// ProjectName is the Acthur project name (acthur.yml `project:`).
	ProjectName() string
	// Root is the absolute path to the project root, used to resolve each
	// service's Dockerfile/build context on disk.
	Root() string
	// Services lists every Dockerizable service node to deploy.
	Services() []ServiceArtifact
	// Log emits a progress line.
	Log(format string, args ...any)
}

// Target deploys a project to Render: ensure each service exists, build+
// push its image to the configured registry, trigger a deploy from it, and
// wait for the deploy to report live.
type Target struct {
	// Token is the Render API key. The caller resolves it (e.g. from
	// RENDER_API_KEY) — Target never reads the environment.
	Token string

	// Registry is the container registry to build+push each service's
	// image to, and that Render must be able to pull from. Required —
	// Render has no registry of its own (see package doc).
	Registry string

	// BaseURL overrides the REST API base URL; empty uses the real API.
	// Tests point this at an httptest server.
	BaseURL string

	// Run executes docker CLI invocations (build/push). Injected for
	// testability; production wires deploy.DockerRunner.
	Run deploy.Runner

	// PollInterval and DeployTimeout control WaitLive behavior. Zero
	// values fall back to sensible defaults.
	PollInterval  time.Duration
	DeployTimeout time.Duration
}

// NewTarget constructs a render Target with the given API key, image
// registry, and docker Runner.
func NewTarget(token, registry string, run deploy.Runner) *Target {
	return &Target{Token: token, Registry: registry, Run: run, DeployTimeout: 5 * time.Minute}
}

// terminalFailureStatuses are Render deploy "status" values that indicate
// the deploy will never reach "live" on its own.
var terminalFailureStatuses = []string{"build_failed", "update_failed", "canceled", "deactivated"}

// Deploy implements the deploy command's target interface.
func (t *Target) Deploy(ctx DeployContext) error {
	if t.Run == nil {
		return fmt.Errorf("render target: no docker runner configured")
	}
	if t.Registry == "" {
		return fmt.Errorf("render target: no image registry configured (set RENDER_IMAGE_REGISTRY — Render has no registry of its own, see internal/deploy/render package docs)")
	}

	client := New(t.Token)
	if t.BaseURL != "" {
		client.BaseURL = t.BaseURL
	}

	ctx.Log("render: resolving workspace owner")
	ownerID, err := client.EnsureOwner()
	if err != nil {
		return fmt.Errorf("render target: %w", err)
	}

	services := ctx.Services()
	if len(services) == 0 {
		return fmt.Errorf("render target: no Dockerizable service nodes to deploy")
	}

	timeout := t.DeployTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	for _, svc := range services {
		name := ctx.ProjectName() + "-" + ctx.EnvName() + "-" + svc.NodeID
		image := fmt.Sprintf("%s/%s-%s:%s", t.Registry, ctx.ProjectName(), ctx.EnvName(), svc.NodeID)

		ctx.Log("render: building %s from %s", image, svc.DockerfilePath)
		if _, err := t.Run("build", "-f", ctx.Root()+"/"+svc.DockerfilePath, "-t", image, ctx.Root()+"/"+svc.ContextDir); err != nil {
			return fmt.Errorf("render target: building %q: %w", svc.NodeID, err)
		}

		ctx.Log("render: pushing %s", image)
		if _, err := t.Run("push", image); err != nil {
			return fmt.Errorf("render target: pushing %q: %w", svc.NodeID, err)
		}

		ctx.Log("render: ensuring service %q exists", name)
		serviceID, err := client.EnsureService(ownerID, name, image)
		if err != nil {
			return fmt.Errorf("render target: %w", err)
		}

		ctx.Log("render: triggering deploy for %q from %s", name, image)
		deployID, err := client.Deploy(serviceID, image)
		if err != nil {
			return fmt.Errorf("render target: %w", err)
		}

		ctx.Log("render: waiting for %q to become live (timeout %s)", name, timeout)
		if err := t.waitLive(client, serviceID, deployID, timeout); err != nil {
			return fmt.Errorf("render target: %w", err)
		}
	}

	ctx.Log("render: project %q is up", ctx.ProjectName())
	return nil
}

// waitLive polls the deploy's status until it reports "live", a terminal
// failure status, or timeout elapses.
func (t *Target) waitLive(client *Client, serviceID, deployID string, timeout time.Duration) error {
	interval := t.PollInterval
	if interval <= 0 {
		interval = 3 * time.Second
	}

	deadline := time.Now().Add(timeout)
	var lastStatus string
	for {
		status, err := client.DeployStatus(serviceID, deployID)
		if err != nil {
			return err
		}
		lastStatus = status

		if status == "live" {
			return nil
		}
		for _, marker := range terminalFailureStatuses {
			if status == marker {
				return fmt.Errorf("deploy %q for service %q failed with status %q", deployID, serviceID, status)
			}
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for deploy %q to become live after %s (last status: %q)", deployID, timeout, lastStatus)
		}
		time.Sleep(interval)
	}
}
