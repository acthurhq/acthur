package coolify

import (
	"fmt"
	"sort"
	"time"
)

// DeployContext is the small, obvious surface the deploy command hands to
// a Target. It carries everything a target needs to project and ship a
// production stack without depending on the deploy command's own types —
// keeping this package (and any future target, e.g. fly/railway/render)
// decoupled from the CLI-assembly layer that builds it (slice 2, PRD
// phase-8-deploy-runtime).
type DeployContext interface {
	// EnvName is the environment being deployed (e.g. "production"), used
	// to scope the Coolify application name so multiple environments of
	// the same project don't collide.
	EnvName() string
	// ComposeYAML is the rendered docker-compose.prod.yml content for this
	// deploy (produced by slice 1's artifact generator).
	ComposeYAML() []byte
	// ProjectName is the Acthur project name (acthur.yml `project:`),
	// used as the Coolify project name.
	ProjectName() string
	// Host is the target environment's Coolify base URL
	// (`environments.<env>.host` in acthur.yml).
	Host() string
	// ServerUUID identifies the Coolify server required by service creation.
	ServerUUID() string
	// WorkloadEnv returns values already resolved and gated by the command.
	// Values are sent only to Coolify's environment API and must never be
	// rendered into Compose or progress/error output.
	WorkloadEnv() map[string]string
	// Log emits a progress line. Implementations typically wire this to
	// output.Info/output.Success under the "deploy" scope.
	Log(format string, args ...any)
}

// Target deploys a project to a Coolify instance: ensure the project and
// compose-based application exist, push the compose YAML, trigger a
// deployment, and wait for it to report healthy.
type Target struct {
	// Token is the Coolify API token. The caller resolves it (e.g. from
	// the COOLIFY_TOKEN env var) — Target never reads the environment.
	Token string

	// PollInterval and HealthTimeout control service health polling. Zero
	// values fall back to Client defaults (PollInterval) and a 5-minute
	// timeout (HealthTimeout) respectively.
	PollInterval  time.Duration
	HealthTimeout time.Duration
}

// NewTarget constructs a coolify Target with the given API token.
func NewTarget(token string) *Target {
	return &Target{Token: token, HealthTimeout: 5 * time.Minute}
}

// Deploy implements the deploy command's target interface: ensure the
// Coolify project and compose application exist for ctx, push the compose
// YAML, trigger a deployment, and block until it reports healthy (or a
// pointed error on failure/timeout).
func (t *Target) Deploy(ctx DeployContext) error {
	if ctx.ServerUUID() == "" {
		return fmt.Errorf("coolify target: server UUID is required for service deployment")
	}
	client := New(ctx.Host(), t.Token)
	if t.PollInterval > 0 {
		client.PollInterval = t.PollInterval
	}

	ctx.Log("coolify: ensuring project %q exists", ctx.ProjectName())
	projectUUID, err := client.EnsureProject(ctx.ProjectName())
	if err != nil {
		return fmt.Errorf("coolify target: %w", err)
	}

	appName := ctx.ProjectName() + "-" + ctx.EnvName()
	ctx.Log("coolify: ensuring compose application %q exists", appName)
	appUUID, err := client.EnsureComposeService(projectUUID, ctx.ServerUUID(), ctx.EnvName(), appName, string(ctx.ComposeYAML()))
	if err != nil {
		return fmt.Errorf("coolify target: %w", err)
	}
	keys := make([]string, 0, len(ctx.WorkloadEnv()))
	for key := range ctx.WorkloadEnv() {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		ctx.Log("coolify: syncing %d workload environment variable(s)", len(keys))
	}
	for _, key := range keys {
		if err := client.UpsertServiceEnv(appUUID, key, ctx.WorkloadEnv()[key]); err != nil {
			return fmt.Errorf("coolify target: %w", err)
		}
	}

	ctx.Log("coolify: triggering deployment for %q", appName)
	if err := client.StartService(appUUID); err != nil {
		return fmt.Errorf("coolify target: %w", err)
	}

	timeout := t.HealthTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	ctx.Log("coolify: waiting for %q to become healthy (timeout %s)", appName, timeout)
	if err := client.WaitServiceHealthy(appUUID, timeout); err != nil {
		return fmt.Errorf("coolify target: %w", err)
	}

	ctx.Log("coolify: %q is healthy", appName)
	return nil
}
