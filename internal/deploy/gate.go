// Package deploy is the production half of the runtime: the pre-deploy
// gate, the deploy execution context, and the compose target. The same
// sealed graph the dev engine runs is executed here with a production
// strategy (PRD §21 Phase 8).
package deploy

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// GateInput is everything the pre-deploy gate needs. The caller (the deploy
// command) resolves it from config + graph so the gate stays testable
// without either.
type GateInput struct {
	Root         string   // project root
	ServiceNodes []string // node IDs with buildable Go modules under Root/<node>
	RequiredEnv  []string // env vars that must be set for the target env
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
	record("env vars", checkEnv(in.RequiredEnv))

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

func checkEnv(required []string) error {
	var missing []string
	for _, v := range required {
		if os.Getenv(v) == "" {
			missing = append(missing, v)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("required env vars not set: %s — export them (or add them to your deploy environment) before deploying", strings.Join(missing, ", "))
	}
	return nil
}
