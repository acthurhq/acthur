// Package doctor checks whether the developer's machine meets all requirements
// for the given acthur.yml stack and can auto-fix many missing dependencies.
package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/output"
)

// ---------------------------------------------------------------------------
// Check types
// ---------------------------------------------------------------------------

// Status is the result of a single doctor check.
type Status int

const (
	StatusOK      Status = iota
	StatusMissing        // required, not found, can auto-fix
	StatusFailed         // required, not found, cannot auto-fix
	StatusOptional       // optional, not found
	StatusWarning        // found but version mismatch or concern
)

// Check represents a single environment requirement.
type Check struct {
	Name        string
	Description string
	Version     string   // found version, empty if missing
	Required    bool
	Status      Status
	FixCmd      []string // command to run to fix (empty = manual)
	FixGuide    string   // human-readable fix instructions
	Adapter     string   // which adapter requires this (empty = always)
}

// Result is the complete output of a doctor run.
type Result struct {
	Checks   []*Check
	OS       string
	Arch     string
	HasFixes bool
}

// ---------------------------------------------------------------------------
// Doctor
// ---------------------------------------------------------------------------

// Run executes all relevant environment checks for the given config.
// If cfg is nil, only core requirements are checked.
func Run(cfg *config.Config) *Result {
	r := &Result{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	// Core requirements — always checked
	r.Checks = append(r.Checks, checkGit())
	r.Checks = append(r.Checks, checkDocker())
	r.Checks = append(r.Checks, checkDockerDaemon())
	r.Checks = append(r.Checks, checkDockerCompose())

	if cfg != nil {
		// Per-adapter checks — only what the project actually uses
		adapters := collectAdapters(cfg)
		r.Checks = append(r.Checks, checksForAdapters(adapters)...)

		// Port availability
		r.Checks = append(r.Checks, checkPorts(cfg)...)
	}

	// Determine if any auto-fixes are available
	for _, c := range r.Checks {
		if (c.Status == StatusMissing || c.Status == StatusFailed) &&
			len(c.FixCmd) > 0 {
			r.HasFixes = true
		}
	}

	return r
}

// Fix runs auto-fix commands for all failing checks that support it.
// Returns a map of check name → fix error (nil means success).
func Fix(r *Result) map[string]error {
	results := make(map[string]error)
	for _, c := range r.Checks {
		if c.Status == StatusOK || c.Status == StatusOptional {
			continue
		}
		if len(c.FixCmd) == 0 {
			continue
		}
		sp := output.NewSpinner(fmt.Sprintf("fixing %s...", c.Name))
		cmd := exec.Command(c.FixCmd[0], c.FixCmd[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			sp.Stop(false, fmt.Sprintf("failed to fix %s", c.Name))
			results[c.Name] = err
		} else {
			sp.Stop(true, fmt.Sprintf("fixed %s", c.Name))
			results[c.Name] = nil
			c.Status = StatusOK
		}
	}
	return results
}

// HasBlockingFailures reports whether r contains any required check that is
// missing or failed. Used by `acthur dev` as a preflight gate: a project
// whose environment cannot possibly run (git/docker missing, wrong Go
// version, etc.) must not proceed to start processes and print a confusing
// mid-startup failure instead of a pointed doctor report.
//
// StatusWarning (e.g. "port already in use") is deliberately not blocking —
// it is not required tooling, it's a runtime condition the engine's own
// startup will surface with a clearer, node-scoped error if it matters.
func HasBlockingFailures(r *Result) bool {
	for _, c := range r.Checks {
		if !c.Required {
			continue
		}
		if c.Status == StatusMissing || c.Status == StatusFailed {
			return true
		}
	}
	return false
}

// Print renders the doctor result to the terminal.
func Print(r *Result) {
	output.Header("Environment Check")
	fmt.Printf("  OS: %s/%s\n\n", r.OS, r.Arch)

	var sections = []struct {
		title   string
		matcher func(*Check) bool
	}{
		{"Core Requirements", func(c *Check) bool { return c.Adapter == "" }},
		{"Go Toolchain", func(c *Check) bool { return c.Adapter == "go" }},
		{"Rust Toolchain", func(c *Check) bool { return c.Adapter == "rust" }},
		{"Node Toolchain", func(c *Check) bool { return c.Adapter == "node" || c.Adapter == "ui" }},
		{"Bun Toolchain", func(c *Check) bool { return c.Adapter == "bun" }},
		{"Python Toolchain", func(c *Check) bool { return c.Adapter == "python" }},
		{"System", func(c *Check) bool { return c.Adapter == "system" }},
	}

	for _, sec := range sections {
		var relevant []*Check
		for _, c := range r.Checks {
			if sec.matcher(c) {
				relevant = append(relevant, c)
			}
		}
		if len(relevant) == 0 {
			continue
		}
		fmt.Printf("  %s\n", sec.title)
		for _, c := range relevant {
			printCheck(c)
		}
		fmt.Println()
	}

	// Summary line
	ok, warn, fail := 0, 0, 0
	for _, c := range r.Checks {
		switch c.Status {
		case StatusOK:
			ok++
		case StatusWarning:
			warn++
		case StatusMissing, StatusFailed:
			if c.Required {
				fail++
			}
		}
	}

	fmt.Printf("  Summary: ")
	if fail == 0 && warn == 0 {
		output.Item(output.StatusOK, fmt.Sprintf("all %d checks passed", ok))
	} else {
		parts := []string{fmt.Sprintf("%d passed", ok)}
		if warn > 0 {
			parts = append(parts, fmt.Sprintf("%d warnings", warn))
		}
		if fail > 0 {
			parts = append(parts, fmt.Sprintf("%d required missing", fail))
		}
		fmt.Println(strings.Join(parts, " · "))
	}

	if r.HasFixes {
		fmt.Println()
		output.Info("", "Run: acthur doctor --fix to resolve issues automatically")
	}
	fmt.Println()
}

func printCheck(c *Check) {
	versionStr := ""
	if c.Version != "" {
		versionStr = fmt.Sprintf("%-12s %s", c.Version, c.Description)
	} else {
		versionStr = fmt.Sprintf("%-12s %s", "not found", c.Description)
	}

	switch c.Status {
	case StatusOK:
		output.Item(output.StatusOK, fmt.Sprintf("%-22s %s", c.Name, versionStr))
	case StatusWarning:
		output.Item(output.StatusWarn, fmt.Sprintf("%-22s %s", c.Name, versionStr))
		if c.FixGuide != "" {
			fmt.Printf("             → %s\n", c.FixGuide)
		}
	case StatusMissing:
		marker := "required"
		if !c.Required {
			marker = "optional"
		}
		output.Item(output.StatusFail, fmt.Sprintf("%-22s (%s)", c.Name, marker))
		if c.FixGuide != "" {
			fmt.Printf("             → %s\n", c.FixGuide)
		}
		if len(c.FixCmd) > 0 {
			fmt.Printf("             → fix: acthur doctor --fix\n")
		}
	case StatusOptional:
		output.Item(output.StatusSkip, fmt.Sprintf("%-22s (optional — not needed for your stack)", c.Name))
	}
}

// ---------------------------------------------------------------------------
// Individual checks
// ---------------------------------------------------------------------------

func checkGit() *Check {
	c := &Check{
		Name:        "git",
		Description: "version control",
		Required:    true,
		Adapter:     "",
		FixGuide:    "install git from https://git-scm.com",
	}
	if v := cmdVersion("git", "--version"); v != "" {
		c.Version = extractVersion(v)
		c.Status = StatusOK
	} else {
		c.Status = StatusFailed
	}
	return c
}

func checkDocker() *Check {
	c := &Check{
		Name:        "docker",
		Description: "required for infra nodes",
		Required:    true,
		Adapter:     "",
		FixGuide:    "install Docker Desktop from https://docker.com",
	}
	if v := cmdVersion("docker", "--version"); v != "" {
		c.Version = extractVersion(v)
		c.Status = StatusOK
	} else {
		c.Status = StatusFailed
	}
	return c
}

func checkDockerDaemon() *Check {
	c := &Check{
		Name:        "docker daemon",
		Description: "must be running for infra nodes",
		Required:    true,
		Adapter:     "",
		FixGuide:    "start Docker Desktop or run: sudo systemctl start docker",
	}
	cmd := exec.Command("docker", "info")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		c.Status = StatusFailed
		c.Version = "not running"
	} else {
		c.Status = StatusOK
		c.Version = "running"
	}
	return c
}

func checkDockerCompose() *Check {
	c := &Check{
		Name:        "docker compose",
		Description: "required for local infra nodes",
		Required:    true,
		Adapter:     "",
	}

	// Try docker compose v2 (plugin) first, then docker-compose v1
	if v := cmdVersion("docker", "compose", "version"); v != "" {
		c.Version = extractVersion(v)
		c.Status = StatusOK
		return c
	}
	if v := cmdVersion("docker-compose", "--version"); v != "" {
		c.Version = extractVersion(v)
		c.Status = StatusOK
		return c
	}

	c.Status = StatusMissing
	c.FixGuide = "install via: brew install docker-compose (mac) or follow https://docs.docker.com/compose/install"
	switch runtime.GOOS {
	case "darwin":
		c.FixCmd = []string{"brew", "install", "docker-compose"}
	case "linux":
		c.FixCmd = []string{"sh", "-c",
			`DOCKER_CONFIG=${DOCKER_CONFIG:-$HOME/.docker} && ` +
				`mkdir -p $DOCKER_CONFIG/cli-plugins && ` +
				`curl -SL https://github.com/docker/compose/releases/latest/download/docker-compose-linux-x86_64 ` +
				`-o $DOCKER_CONFIG/cli-plugins/docker-compose && ` +
				`chmod +x $DOCKER_CONFIG/cli-plugins/docker-compose`}
	}
	return c
}

func checkGo(minMajor, minMinor int) *Check {
	c := &Check{
		Name:        "go",
		Description: fmt.Sprintf("required for go:* adapters (≥%d.%d)", minMajor, minMinor),
		Required:    true,
		Adapter:     "go",
		FixGuide:    "install from https://golang.org/dl",
	}
	v := cmdVersion("go", "version")
	if v == "" {
		c.Status = StatusMissing
		return c
	}
	c.Version = extractVersion(v)
	if !meetsMinVersion(c.Version, minMajor, minMinor) {
		c.Status = StatusWarning
		c.FixGuide = fmt.Sprintf("upgrade Go to ≥%d.%d from https://golang.org/dl", minMajor, minMinor)
		return c
	}
	c.Status = StatusOK
	return c
}

func checkAir() *Check {
	c := &Check{
		Name:        "air",
		Description: "hot reload for go:* adapters",
		Required:    true,
		Adapter:     "go",
		FixCmd:      []string{"go", "install", "github.com/air-verse/air@latest"},
		FixGuide:    "auto-fixable: acthur doctor --fix",
	}
	if v := cmdVersion("air", "-v"); v != "" {
		c.Version = extractVersion(v)
		c.Status = StatusOK
	} else {
		c.Status = StatusMissing
	}
	return c
}

func checkGolangMigrate() *Check {
	c := &Check{
		Name:        "golang-migrate",
		Description: "database migrations (migrations plugin)",
		Required:    true,
		Adapter:     "go",
		FixCmd:      []string{"go", "install", "github.com/golang-migrate/migrate/v4/cmd/migrate@latest"},
		FixGuide:    "auto-fixable: acthur doctor --fix",
	}
	if v := cmdVersion("migrate", "-version"); v != "" {
		c.Version = extractVersion(v)
		c.Status = StatusOK
	} else {
		c.Status = StatusMissing
	}
	return c
}

func checkRustup() *Check {
	c := &Check{
		Name:        "rustup",
		Description: "required for rust:* adapters",
		Required:    true,
		Adapter:     "rust",
		FixGuide:    "install from https://rustup.rs — curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh",
	}
	if v := cmdVersion("rustup", "--version"); v != "" {
		c.Version = extractVersion(v)
		c.Status = StatusOK
	} else {
		c.Status = StatusFailed
	}
	return c
}

func checkCargoWatch() *Check {
	c := &Check{
		Name:        "cargo-watch",
		Description: "hot reload for rust:* adapters",
		Required:    true,
		Adapter:     "rust",
		FixCmd:      []string{"cargo", "install", "cargo-watch"},
		FixGuide:    "auto-fixable: acthur doctor --fix",
	}
	if v := cmdVersion("cargo-watch", "--version"); v != "" {
		c.Version = extractVersion(v)
		c.Status = StatusOK
	} else {
		c.Status = StatusMissing
	}
	return c
}

func checkNode(minMajor int) *Check {
	c := &Check{
		Name:        "node",
		Description: fmt.Sprintf("required for node:* and ui:* adapters (≥%d)", minMajor),
		Required:    true,
		Adapter:     "node",
		FixGuide:    "install from https://nodejs.org or use nvm",
	}
	v := cmdVersion("node", "--version")
	if v == "" {
		c.Status = StatusMissing
		return c
	}
	c.Version = strings.TrimPrefix(strings.TrimSpace(v), "v")
	major := parseMajorVersion(c.Version)
	if major < minMajor {
		c.Status = StatusWarning
		c.FixGuide = fmt.Sprintf("upgrade Node.js to ≥%d from https://nodejs.org", minMajor)
		return c
	}
	c.Status = StatusOK
	return c
}

func checkPNPM() *Check {
	c := &Check{
		Name:        "pnpm",
		Description: "package manager for frontend adapters",
		Required:    true,
		Adapter:     "ui",
		FixCmd:      []string{"npm", "install", "-g", "pnpm"},
		FixGuide:    "auto-fixable: acthur doctor --fix",
	}
	if v := cmdVersion("pnpm", "--version"); v != "" {
		c.Version = extractVersion(v)
		c.Status = StatusOK
	} else {
		c.Status = StatusMissing
	}
	return c
}

func checkBun() *Check {
	c := &Check{
		Name:        "bun",
		Description: "required for bun:* adapters",
		Required:    true,
		Adapter:     "bun",
		FixGuide:    "install from https://bun.sh — curl -fsSL https://bun.sh/install | bash",
	}
	if v := cmdVersion("bun", "--version"); v != "" {
		c.Version = extractVersion(v)
		c.Status = StatusOK
	} else {
		c.Status = StatusFailed
	}
	return c
}

func checkPython(minMajor, minMinor int) *Check {
	c := &Check{
		Name:        "python",
		Description: fmt.Sprintf("required for python:* adapters (≥%d.%d)", minMajor, minMinor),
		Required:    true,
		Adapter:     "python",
		FixGuide:    "install from https://python.org",
	}
	// Try python3 first, then python
	v := cmdVersion("python3", "--version")
	if v == "" {
		v = cmdVersion("python", "--version")
	}
	if v == "" {
		c.Status = StatusMissing
		return c
	}
	c.Version = extractVersion(v)
	c.Status = StatusOK
	return c
}

// checkPorts verifies that all declared service ports are available.
func checkPorts(cfg *config.Config) []*Check {
	var checks []*Check
	for id, node := range cfg.Graph.Nodes {
		if node.Port == 0 {
			continue
		}
		c := &Check{
			Name:        fmt.Sprintf("port %d", node.Port),
			Description: fmt.Sprintf("required by %s", id),
			Required:    true,
			Adapter:     "system",
		}
		if isPortAvailable(node.Port) {
			c.Status = StatusOK
			c.Version = "available"
		} else {
			c.Status = StatusWarning
			c.Version = "in use"
			c.FixGuide = fmt.Sprintf("port %d is already in use — stop the process using it or change the port in acthur.yml", node.Port)
		}
		checks = append(checks, c)
	}
	return checks
}

// ---------------------------------------------------------------------------
// Adapter detection and check dispatch
// ---------------------------------------------------------------------------

func collectAdapters(cfg *config.Config) map[string]bool {
	adapters := map[string]bool{}
	for _, node := range cfg.Graph.Nodes {
		runtime := adapterRuntime(node.Adapter)
		if runtime != "" {
			adapters[runtime] = true
		}
	}
	return adapters
}

func adapterRuntime(adapter string) string {
	parts := strings.SplitN(adapter, ":", 2)
	if len(parts) == 0 {
		return ""
	}
	switch parts[0] {
	case "go":
		return "go"
	case "rust":
		return "rust"
	case "node":
		return "node"
	case "bun":
		return "bun"
	case "python":
		return "python"
	case "ui":
		return "ui"
	}
	return ""
}

func checksForAdapters(adapters map[string]bool) []*Check {
	var checks []*Check
	if adapters["go"] {
		checks = append(checks, checkGo(1, 21))
		checks = append(checks, checkAir())
		checks = append(checks, checkGolangMigrate())
	}
	if adapters["rust"] {
		checks = append(checks, checkRustup())
		checks = append(checks, checkCargoWatch())
	}
	if adapters["node"] || adapters["ui"] {
		checks = append(checks, checkNode(20))
		checks = append(checks, checkPNPM())
	}
	if adapters["bun"] {
		checks = append(checks, checkBun())
	}
	if adapters["python"] {
		checks = append(checks, checkPython(3, 11))
	}
	return checks
}

// ---------------------------------------------------------------------------
// OS utilities
// ---------------------------------------------------------------------------

func cmdVersion(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	// CombinedOutput, not Output: some tools (air) print their version
	// banner to stderr with an empty stdout.
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// extractVersion pulls the first version-like token from a string.
// e.g. "go version go1.22.3 linux/amd64" → "1.22.3"
func extractVersion(s string) string {
	s = strings.TrimSpace(s)
	fields := strings.Fields(s)
	for _, f := range fields {
		f = strings.TrimPrefix(f, "v")
		f = strings.TrimPrefix(f, "go")
		if len(f) > 0 && (f[0] >= '0' && f[0] <= '9') {
			return f
		}
	}
	return s
}

func parseMajorVersion(v string) int {
	parts := strings.Split(v, ".")
	if len(parts) == 0 {
		return 0
	}
	n, _ := strconv.Atoi(parts[0])
	return n
}

func meetsMinVersion(v string, major, minor int) bool {
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return false
	}
	maj, _ := strconv.Atoi(parts[0])
	min, _ := strconv.Atoi(parts[1])
	if maj > major {
		return true
	}
	if maj == major && min >= minor {
		return true
	}
	return false
}

func isPortAvailable(port int) bool {
	// Quick check: try to connect — if we can, port is in use
	cmd := exec.Command("sh", "-c",
		fmt.Sprintf("(echo >/dev/tcp/localhost/%d) 2>/dev/null && echo in_use || echo available", port))
	out, err := cmd.Output()
	if err != nil {
		return true // assume available if check fails
	}
	return !strings.Contains(string(out), "in_use")
}
