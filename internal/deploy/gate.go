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
)

// SecretResolver looks up a fallback value for a required env var that is not
// already exported into the deploying shell's environment. Wired from
// internal/secrets.Store.Get by the `acthur deploy` command so a secret set
// via `acthur secrets set` satisfies the gate without also needing an
// `export` — but a real production deploy environment (CI secrets, the host
// shell) is still checked first and always wins. Returns ok=false if the key
// resolves to nothing (unset, or no store configured).
type SecretResolver func(key string) (value string, ok bool)

// GateInput is everything the pre-deploy gate needs. The caller (the deploy
// command) resolves it from config + graph so the gate stays testable
// without either.
type GateInput struct {
	Root         string   // project root
	ServiceNodes []string // node IDs with buildable Go modules under Root/<node>
	RequiredEnv  []string // env vars that must be set for the target env

	// ResolveSecret is an optional fallback consulted for a RequiredEnv var
	// that isn't set in the process environment. Nil disables the fallback
	// (required vars must be exported, matching the gate's original
	// behavior).
	ResolveSecret SecretResolver
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

	for _, node := range in.ServiceNodes {
		record("build "+node, goRun(in.Root, node, "build", "./..."))
	}
	for _, node := range in.ServiceNodes {
		record("test "+node, goRun(in.Root, node, "test", "./..."))
	}
	record("env vars", checkEnv(in.RequiredEnv, in.ResolveSecret))

	if len(failures) > 0 {
		return report, fmt.Errorf("pre-deploy gate failed:\n  - %s", strings.Join(failures, "\n  - "))
	}
	return report, nil
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

func checkEnv(required []string, resolveSecret SecretResolver) error {
	var missing []string
	for _, v := range required {
		if os.Getenv(v) != "" {
			continue
		}
		if resolveSecret != nil {
			if val, ok := resolveSecret(v); ok && val != "" {
				continue
			}
		}
		missing = append(missing, v)
	}
	if len(missing) > 0 {
		return fmt.Errorf("required env vars not set: %s — export them, add them to your deploy environment, or run 'acthur secrets set <KEY> <value>' before deploying", strings.Join(missing, ", "))
	}
	return nil
}
