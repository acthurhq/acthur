package railway

import (
	"fmt"
	"time"

	"github.com/acthur/acthur/internal/deploy"
)

// ServiceArtifact describes one Dockerizable service node the target must
// build, push, and run as a Railway service.
type ServiceArtifact struct {
	// NodeID is the graph node ID (e.g. "api") — also used as the Railway
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
// a Target — kept self-contained, same as coolify/fly.
type DeployContext interface {
	// EnvName is the environment being deployed (e.g. "production"), used
	// to find/create the matching Railway environment within the project.
	EnvName() string
	// ProjectName is the Acthur project name (acthur.yml `project:`), used
	// as the Railway project name.
	ProjectName() string
	// Root is the absolute path to the project root, used to resolve each
	// service's Dockerfile/build context on disk.
	Root() string
	// Services lists every Dockerizable service node to deploy.
	Services() []ServiceArtifact
	// Log emits a progress line.
	Log(format string, args ...any)
}

// Target deploys a project to Railway: ensure the project/environment/
// services exist, build+push each service's image to the configured
// registry, point the Railway service at it, trigger a deploy, and wait
// for it to report success.
type Target struct {
	// Token is the Railway API token. The caller resolves it (e.g. from
	// RAILWAY_TOKEN) — Target never reads the environment.
	Token string

	// Registry is the container registry to build+push each service's
	// image to, and that Railway must be able to pull from (e.g.
	// "ghcr.io/acme" or a Docker Hub namespace). Required — Railway has no
	// registry of its own (see package doc).
	Registry string

	// BaseURL overrides the GraphQL API base URL; empty uses the real
	// API. Tests point this at an httptest server.
	BaseURL string

	// Run executes docker CLI invocations (build/push). Injected for
	// testability; production wires deploy.DockerRunner.
	Run deploy.Runner

	// PollInterval and DeployTimeout control WaitDeployed behavior. Zero
	// values fall back to sensible defaults.
	PollInterval  time.Duration
	DeployTimeout time.Duration
}

// NewTarget constructs a railway Target with the given API token, image
// registry, and docker Runner.
func NewTarget(token, registry string, run deploy.Runner) *Target {
	return &Target{Token: token, Registry: registry, Run: run, DeployTimeout: 5 * time.Minute}
}

// terminalFailureStatuses are Railway deployment "status" values that
// indicate the deployment will never reach SUCCESS on its own.
var terminalFailureStatuses = []string{"FAILED", "CRASHED", "REMOVED"}

// Deploy implements the deploy command's target interface.
func (t *Target) Deploy(ctx DeployContext) error {
	if t.Run == nil {
		return fmt.Errorf("railway target: no docker runner configured")
	}
	if t.Registry == "" {
		return fmt.Errorf("railway target: no image registry configured (set RAILWAY_IMAGE_REGISTRY — Railway has no registry of its own, see internal/deploy/railway package docs)")
	}

	client := New(t.Token)
	if t.BaseURL != "" {
		client.BaseURL = t.BaseURL
	}

	ctx.Log("railway: ensuring project %q exists", ctx.ProjectName())
	projectID, err := client.EnsureProject(ctx.ProjectName())
	if err != nil {
		return fmt.Errorf("railway target: %w", err)
	}

	detail, err := client.Project(projectID)
	if err != nil {
		return fmt.Errorf("railway target: %w", err)
	}
	environmentID := ""
	for _, e := range detail.Environments {
		if e.Name == ctx.EnvName() {
			environmentID = e.ID
			break
		}
	}
	if environmentID == "" {
		return fmt.Errorf("railway target: environment %q not found in project %q (Railway environments are created from the dashboard/CLI, not this target)", ctx.EnvName(), ctx.ProjectName())
	}

	services := ctx.Services()
	if len(services) == 0 {
		return fmt.Errorf("railway target: no Dockerizable service nodes to deploy")
	}

	timeout := t.DeployTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	for _, svc := range services {
		image := fmt.Sprintf("%s/%s-%s:%s", t.Registry, ctx.ProjectName(), ctx.EnvName(), svc.NodeID)

		ctx.Log("railway: building %s from %s", image, svc.DockerfilePath)
		if _, err := t.Run("build", "-f", ctx.Root()+"/"+svc.DockerfilePath, "-t", image, ctx.Root()+"/"+svc.ContextDir); err != nil {
			return fmt.Errorf("railway target: building %q: %w", svc.NodeID, err)
		}

		ctx.Log("railway: pushing %s", image)
		if _, err := t.Run("push", image); err != nil {
			return fmt.Errorf("railway target: pushing %q: %w", svc.NodeID, err)
		}

		ctx.Log("railway: ensuring service %q runs %s", svc.NodeID, image)
		serviceID, err := client.EnsureService(projectID, environmentID, svc.NodeID, image)
		if err != nil {
			return fmt.Errorf("railway target: %w", err)
		}

		ctx.Log("railway: triggering deploy for %q", svc.NodeID)
		if err := client.Deploy(serviceID, environmentID); err != nil {
			return fmt.Errorf("railway target: %w", err)
		}

		ctx.Log("railway: waiting for %q to succeed (timeout %s)", svc.NodeID, timeout)
		if err := t.waitDeployed(client, serviceID, environmentID, timeout); err != nil {
			return fmt.Errorf("railway target: %w", err)
		}
	}

	ctx.Log("railway: project %q is up", ctx.ProjectName())
	return nil
}

// waitDeployed polls the service's latest deployment status until it
// reports SUCCESS, a terminal failure status, or timeout elapses.
func (t *Target) waitDeployed(client *Client, serviceID, environmentID string, timeout time.Duration) error {
	interval := t.PollInterval
	if interval <= 0 {
		interval = 3 * time.Second
	}

	deadline := time.Now().Add(timeout)
	var lastStatus string
	for {
		status, err := client.LatestDeploymentStatus(serviceID, environmentID)
		if err != nil {
			return err
		}
		lastStatus = status

		if status == "SUCCESS" {
			return nil
		}
		for _, marker := range terminalFailureStatuses {
			if status == marker {
				return fmt.Errorf("deployment for %q failed with status %q", serviceID, status)
			}
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %q to succeed after %s (last status: %q)", serviceID, timeout, lastStatus)
		}
		time.Sleep(interval)
	}
}
