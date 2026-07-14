// Package deploy is the production half of the runtime: the pre-deploy
// gate, the deploy execution context, and the compose target. The same
// sealed graph the dev engine runs is executed here with a production
// strategy (PRD §21 Phase 8).
package deploy

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/acthurhq/acthur/internal/contract"
	"github.com/acthurhq/acthur/internal/generate"
	"github.com/acthurhq/acthur/internal/graph"
)

// GateInput is everything the pre-deploy gate needs. The caller (the deploy
// command) resolves it from config + graph so the gate stays testable
// without either.
type GateInput struct {
	Root            string   // project root
	ServiceNodes    []string // node IDs with buildable Go modules under Root/<node>
	RequiredEnv     []string // env vars that must be set for the target env
	Graph           *graph.Graph
	AdapterResolver graph.Resolver
	// MigrationStatus is nil when the project has no migration state. The
	// command layer supplies it when migrations are configured or present,
	// keeping this package independent of the migrations plugin and database.
	MigrationStatus func() (MigrationState, error)
	// SecurityCheck is nil when no production security policy is active. The
	// command layer supplies it for configured security plugins so deploy does
	// not depend on plugin implementations.
	SecurityCheck func() error
}

// MigrationState is the narrow production fact needed by the gate. Applied
// and Dirty come from the live database; Latest comes from the project's up
// migration files.
type MigrationState struct {
	Configured bool
	Applied    uint
	Latest     uint
	Dirty      bool
}

// CheckResult is one executed gate check.
type CheckResult struct {
	Name   string
	OK     bool
	Detail string
}

// GateReport lists every check the gate ran.
type GateReport struct {
	Checks []CheckResult
}

// RunGate runs every pre-deploy check and returns a report. It runs ALL
// checks before failing so one deploy attempt tells the whole story; the
// returned error aggregates every failure with a pointed message.
func RunGate(in GateInput) (*GateReport, error) {
	report := &GateReport{}
	var failures []string

	record := func(name string, err error) {
		if err != nil {
			report.Checks = append(report.Checks, CheckResult{Name: name, OK: false, Detail: err.Error()})
			failures = append(failures, fmt.Sprintf("%s: %v", name, err))
			return
		}
		report.Checks = append(report.Checks, CheckResult{Name: name, OK: true})
	}

	record("graph", validateGraph(in.Graph, in.AdapterResolver))
	for _, node := range in.ServiceNodes {
		record("build "+node, goRun(in.Root, node, "build", "./..."))
	}
	for _, node := range in.ServiceNodes {
		record("test "+node, goRun(in.Root, node, "test", "./..."))
	}
	_, contractsErr := contract.LoadDir(in.Root)
	record("contracts", contractsErr)
	record("generated artifacts", generate.VerifyGeneratedArtifacts(in.Root))
	record("migrations", checkMigrations(in.MigrationStatus))
	record("security", checkSecurity(in.SecurityCheck))
	record("env vars", checkEnv(in.RequiredEnv))

	if len(failures) > 0 {
		return report, fmt.Errorf("pre-deploy gate failed:\n  - %s", strings.Join(failures, "\n  - "))
	}
	return report, nil
}

func checkSecurity(check func() error) error {
	if check == nil {
		return nil
	}
	return check()
}

func checkMigrations(status func() (MigrationState, error)) error {
	if status == nil {
		return nil
	}
	state, err := status()
	if err != nil {
		return fmt.Errorf("checking database migration status: %w", err)
	}
	if !state.Configured {
		return nil
	}
	if state.Dirty {
		return fmt.Errorf("database is dirty at migration %d — repair the migration state before deploying", state.Applied)
	}
	if state.Applied < state.Latest {
		return fmt.Errorf("database migration version %d is behind latest migration %d — run 'acthur db migrate' before deploying", state.Applied, state.Latest)
	}
	return nil
}

func validateGraph(g *graph.Graph, resolver graph.Resolver) error {
	if g == nil {
		return fmt.Errorf("production graph is required")
	}
	if resolver == nil {
		return fmt.Errorf("production adapter resolver is required")
	}

	validationErrors := g.Validate(resolver)
	if len(validationErrors) == 0 {
		return nil
	}
	details := make([]string, 0, len(validationErrors))
	for _, validationErr := range validationErrors {
		details = append(details, fmt.Sprintf("%s: %s", validationErr.Rule, validationErr.Message))
	}
	return fmt.Errorf("semantic validation failed: %s", strings.Join(details, "; "))
}

func goRun(root, node string, args ...string) error {
	cmd := exec.Command("go", args...)
	cmd.Dir = root + "/" + node
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Last lines carry the actual compiler/test failure.
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		tail := lines
		if len(lines) > 6 {
			tail = lines[len(lines)-6:]
		}
		return fmt.Errorf("go %s failed in node %q:\n%s", strings.Join(args, " "), node, strings.Join(tail, "\n"))
	}
	return nil
}

// GoStream runs `go <args>` in root/node with combined stdout/stderr
// streamed live to out as the command produces it — unlike goRun (which
// buffers everything and only surfaces the tail on failure), this is for
// callers that want to show progress as it happens: `acthur test` and
// `acthur build` both drive per-node go invocations through this one seam
// rather than re-implementing exec.Command plumbing.
//
// env, when non-empty, is appended to the current process's environment
// (e.g. "CGO_ENABLED=0" for production builds); pass nil to inherit as-is.
func GoStream(root, node string, out io.Writer, env []string, args ...string) error {
	cmd := exec.Command("go", args...)
	cmd.Dir = filepath.Join(root, node)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go %s failed in node %q: %w", strings.Join(args, " "), node, err)
	}
	return nil
}

func checkEnv(required []string) error {
	var missing []string
	for _, v := range required {
		if os.Getenv(v) != "" {
			continue
		}
		missing = append(missing, v)
	}
	if len(missing) > 0 {
		return fmt.Errorf("required env vars not set: %s — export them or add them to your deploy environment before deploying", strings.Join(missing, ", "))
	}
	return nil
}
