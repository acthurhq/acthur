// Package output provides consistent, styled terminal output for all Acthur commands.
// All CLI output goes through this package — nothing uses fmt.Println directly.
package output

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
)

// Symbols used in output lines
const (
	SymbolSuccess  = "✓"
	SymbolError    = "✗"
	SymbolWarning  = "⚠"
	SymbolInfo     = "→"
	SymbolPending  = "○"
	SymbolRunning  = "◆"
	SymbolSeparator = "─"
)

// Prefixes for component-scoped log lines
// e.g. [acthur], [graph], [api], [db]
const (
	PrefixKernel   = "acthur"
	PrefixGraph    = "graph"
	PrefixContract = "contract"
	PrefixDoctor   = "doctor"
	PrefixDeploy   = "deploy"
	PrefixPlugin   = "plugin"
)

var (
	// Colour functions — disabled automatically in non-TTY environments
	colorSuccess = color.New(color.FgGreen, color.Bold)
	colorError   = color.New(color.FgRed, color.Bold)
	colorWarning = color.New(color.FgYellow, color.Bold)
	colorInfo    = color.New(color.FgCyan)
	colorMuted   = color.New(color.FgHiBlack)
	colorBold    = color.New(color.Bold)

	// Output targets — overridable for testing
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr

	// Verbosity level
	verbose bool
)

// SetVerbose enables debug output.
func SetVerbose(v bool) { verbose = v }

// SetOutput overrides the output writers (used in tests).
func SetOutput(out, err io.Writer) {
	stdout = out
	stderr = err
}

// --- Kernel-level messages --------------------------------------------------

// Banner prints the Acthur startup banner.
func Banner() {
	_, _ = fmt.Fprintln(stdout)
	_, _ = colorBold.Fprintln(stdout, "  ✦  Acthur — Runtime Graph Operating System")
	_, _ = colorMuted.Fprintln(stdout, "     github.com/acthurhq/acthur")
	_, _ = fmt.Fprintln(stdout)
}

// Header prints a section header line.
func Header(text string) {
	_, _ = fmt.Fprintln(stdout)
	_, _ = colorBold.Fprintf(stdout, "  %s\n", text)
	_, _ = colorMuted.Fprintf(stdout, "  %s\n", strings.Repeat(SymbolSeparator, len(text)+2))
}

// --- Scoped log lines -------------------------------------------------------

// Info prints an informational message with an optional scope prefix.
//   output.Info("graph", "loading 4 nodes...")   → [graph] → loading 4 nodes...
//   output.Info("", "starting...")               → [acthur] → starting...
func Info(scope, format string, args ...any) {
	prefix := resolveScope(scope)
	msg := fmt.Sprintf(format, args...)
	_, _ = colorMuted.Fprintf(stdout, "[%s] ", prefix)
	_, _ = colorInfo.Fprintf(stdout, "%s  ", SymbolInfo)
	_, _ = fmt.Fprintln(stdout, msg)
}

// Success prints a success message.
func Success(scope, format string, args ...any) {
	prefix := resolveScope(scope)
	msg := fmt.Sprintf(format, args...)
	_, _ = colorMuted.Fprintf(stdout, "[%s] ", prefix)
	_, _ = colorSuccess.Fprintf(stdout, "%s  ", SymbolSuccess)
	_, _ = fmt.Fprintln(stdout, msg)
}

// Warn prints a warning. Warnings do not stop execution.
func Warn(scope, format string, args ...any) {
	prefix := resolveScope(scope)
	msg := fmt.Sprintf(format, args...)
	_, _ = colorMuted.Fprintf(stdout, "[%s] ", prefix)
	_, _ = colorWarning.Fprintf(stdout, "%s  ", SymbolWarning)
	_, _ = fmt.Fprintln(stdout, msg)
}

// Error prints an error message to stderr. Does not exit.
func Error(scope, format string, args ...any) {
	prefix := resolveScope(scope)
	msg := fmt.Sprintf(format, args...)
	_, _ = colorMuted.Fprintf(stderr, "[%s] ", prefix)
	_, _ = colorError.Fprintf(stderr, "%s  ", SymbolError)
	_, _ = fmt.Fprintln(stderr, msg)
}

// Debug prints a debug message only when verbose mode is on.
func Debug(scope, format string, args ...any) {
	if !verbose {
		return
	}
	prefix := resolveScope(scope)
	msg := fmt.Sprintf(format, args...)
	_, _ = colorMuted.Fprintf(stdout, "[%s] [debug] %s\n", prefix, msg)
}

// ServiceLog prints a log line tagged with a service name (used by process manager).
// Color is derived from the service name for visual differentiation.
func ServiceLog(service, line string) {
	c := serviceColor(service)
	_, _ = c.Fprintf(stdout, "[%s] ", service)
	_, _ = fmt.Fprintln(stdout, line)
}

// --- Structured output -------------------------------------------------------

// Step prints a wizard or multi-step process step.
func Step(n, total int, text string) {
	_, _ = colorMuted.Fprintf(stdout, "\n  Step %d of %d", n, total)
	_, _ = colorMuted.Fprintln(stdout, "  "+strings.Repeat(SymbolSeparator, 40))
	_, _ = colorBold.Fprintf(stdout, "  %s\n\n", text)
}

// Item prints a single checklist item with a status indicator.
func Item(status ItemStatus, text string) {
	switch status {
	case StatusOK:
		_, _ = colorSuccess.Fprintf(stdout, "  %s  ", SymbolSuccess)
	case StatusFail:
		_, _ = colorError.Fprintf(stdout, "  %s  ", SymbolError)
	case StatusWarn:
		_, _ = colorWarning.Fprintf(stdout, "  %s  ", SymbolWarning)
	case StatusSkip:
		_, _ = colorMuted.Fprintf(stdout, "  %s  ", SymbolPending)
	case StatusRunning:
		_, _ = colorInfo.Fprintf(stdout, "  %s  ", SymbolRunning)
	}
	_, _ = fmt.Fprintln(stdout, text)
}

// ItemStatus represents the visual state of a checklist item.
type ItemStatus int

const (
	StatusOK      ItemStatus = iota
	StatusFail
	StatusWarn
	StatusSkip
	StatusRunning
)

// Table prints a simple two-column table.
func Table(rows [][2]string) {
	if len(rows) == 0 {
		return
	}
	maxLeft := 0
	for _, r := range rows {
		if len(r[0]) > maxLeft {
			maxLeft = len(r[0])
		}
	}
	for _, r := range rows {
		padding := strings.Repeat(" ", maxLeft-len(r[0])+2)
		_, _ = colorBold.Fprintf(stdout, "  %s", r[0])
		_, _ = colorMuted.Fprintf(stdout, "%s", padding)
		_, _ = fmt.Fprintln(stdout, r[1])
	}
}

// Separator prints a horizontal divider.
func Separator() {
	_, _ = colorMuted.Fprintln(stdout, "\n  "+strings.Repeat(SymbolSeparator, 45)+"\n")
}

// Blank prints a blank line.
func Blank() { _, _ = fmt.Fprintln(stdout) }

// --- Error display -----------------------------------------------------------

// Fatal prints a structured fatal error with context and a fix hint,
// then exits with the provided exit code.
func Fatal(e *ActhurError) {
	_, _ = fmt.Fprintln(stderr)
	_, _ = colorError.Fprintf(stderr, "  %s  Error: %s\n\n", SymbolError, e.Message)

	if e.Context != "" {
		_, _ = colorMuted.Fprintf(stderr, "  Context:  ")
		_, _ = fmt.Fprintln(stderr, e.Context)
	}
	if e.Problem != "" {
		_, _ = colorMuted.Fprintf(stderr, "  Problem:  ")
		_, _ = fmt.Fprintln(stderr, e.Problem)
	}
	if e.Fix != "" {
		_, _ = fmt.Fprintln(stderr)
		_, _ = colorMuted.Fprintf(stderr, "  Fix:      ")
		_, _ = fmt.Fprintln(stderr, e.Fix)
	}
	if e.DocsURL != "" {
		_, _ = fmt.Fprintln(stderr)
		_, _ = colorMuted.Fprintf(stderr, "  Docs:     ")
		_, _ = colorInfo.Fprintln(stderr, e.DocsURL)
	}
	_, _ = fmt.Fprintln(stderr)
	_, _ = colorMuted.Fprintf(stderr, "  Exit code: %d\n\n", e.Code)
	os.Exit(int(e.Code))
}

// --- Ready message -----------------------------------------------------------

// Ready prints the "all services ready" message with URL list.
func Ready(project string, urls map[string]string) {
	_, _ = fmt.Fprintln(stdout)
	_, _ = colorSuccess.Fprintf(stdout, "  ✦  %s is ready\n\n", project)
	for name, url := range urls {
		_, _ = colorMuted.Fprintf(stdout, "     %-16s", name)
		_, _ = colorInfo.Fprintln(stdout, url)
	}
	_, _ = fmt.Fprintln(stdout)
	_, _ = colorMuted.Fprintln(stdout, "  Press Ctrl+C to stop all services")
	_, _ = fmt.Fprintln(stdout)
}

// --- Spinner -----------------------------------------------------------------

// Spinner returns a simple text spinner for long-running operations.
// Call Stop() when the operation completes.
type Spinner struct {
	msg    string
	done   chan struct{}
	start  time.Time
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// NewSpinner starts a spinner with the given message.
func NewSpinner(msg string) *Spinner {
	s := &Spinner{msg: msg, done: make(chan struct{}), start: time.Now()}
	go func() {
		i := 0
		for {
			select {
			case <-s.done:
				return
			default:
				frame := spinnerFrames[i%len(spinnerFrames)]
				_, _ = colorInfo.Fprintf(stdout, "\r  %s  %s", frame, s.msg)
				time.Sleep(80 * time.Millisecond)
				i++
			}
		}
	}()
	return s
}

// Stop stops the spinner and prints a completion message.
func (s *Spinner) Stop(success bool, msg string) {
	close(s.done)
	elapsed := time.Since(s.start).Round(time.Millisecond)
	_, _ = fmt.Fprintf(stdout, "\r")
	if success {
		_, _ = colorSuccess.Fprintf(stdout, "  %s  ", SymbolSuccess)
	} else {
		_, _ = colorError.Fprintf(stdout, "  %s  ", SymbolError)
	}
	_, _ = fmt.Fprintf(stdout, "%s ", msg)
	_, _ = colorMuted.Fprintf(stdout, "(%s)\n", elapsed)
}

// --- Helpers -----------------------------------------------------------------

// resolveScope returns the scope label for a log prefix.
func resolveScope(scope string) string {
	if scope == "" {
		return PrefixKernel
	}
	return scope
}

// serviceColor assigns a stable terminal color to a service name.
// Same service name always gets the same color across a session.
var serviceColors = []*color.Color{
	color.New(color.FgCyan),
	color.New(color.FgMagenta),
	color.New(color.FgYellow),
	color.New(color.FgGreen),
	color.New(color.FgBlue),
	color.New(color.FgHiCyan),
	color.New(color.FgHiMagenta),
	color.New(color.FgHiYellow),
}

func serviceColor(name string) *color.Color {
	h := 0
	for _, ch := range name {
		h = (h*31 + int(ch)) % len(serviceColors)
	}
	return serviceColors[h]
}

// ActhurError is the structured error type used by Fatal.
// Defined here to avoid import cycles with the errors package.
type ActhurError struct {
	Code    ExitCode
	Message string
	Context string
	Problem string
	Fix     string
	DocsURL string
	Cause   error
}

func (e *ActhurError) Error() string { return e.Message }
func (e *ActhurError) Unwrap() error { return e.Cause }

// ExitCode enumerates all Acthur exit codes.
type ExitCode int

const (
	ExitSuccess         ExitCode = 0
	ExitGeneral         ExitCode = 1
	ExitGraphError      ExitCode = 2
	ExitAdapterError    ExitCode = 3
	ExitPluginError     ExitCode = 4
	ExitContractError   ExitCode = 5
	ExitEnvError        ExitCode = 6
	ExitDeployError     ExitCode = 7
	ExitConfigError     ExitCode = 8
	ExitProcessError    ExitCode = 9
	ExitContractBreak   ExitCode = 10
)
