package main

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestWaitForInterrupt_ReturnsOnSIGTERM: a process manager or CI runner that
// stops `acthur db studio` sends SIGTERM, not SIGINT (Ctrl+C) — if
// waitForInterrupt only caught SIGINT, that path would skip the deferred
// `docker stop` in runDbStudio and leak the studio container.
func TestWaitForInterrupt_ReturnsOnSIGTERM(t *testing.T) {
	done := make(chan struct{})
	go func() {
		waitForInterrupt()
		close(done)
	}()
	// Give the goroutine time to reach signal.Notify before we send —
	// otherwise the signal could arrive before it's being listened for.
	time.Sleep(100 * time.Millisecond)

	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("find self process: %v", err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("waitForInterrupt did not return after SIGTERM")
	}
}

// TestParseStudioTarget_ExtractsPortUserDB: adminer's login form needs the
// port, user, and database name pre-filled from DATABASE_URL — never the
// password.
func TestParseStudioTarget_ExtractsPortUserDB(t *testing.T) {
	target, err := parseStudioTarget("postgres://appuser:s3cret@localhost:5433/appdb?sslmode=disable")
	if err != nil {
		t.Fatalf("parseStudioTarget: %v", err)
	}
	if target.port != "5433" {
		t.Errorf("expected port 5433, got %q", target.port)
	}
	if target.user != "appuser" {
		t.Errorf("expected user appuser, got %q", target.user)
	}
	if target.dbname != "appdb" {
		t.Errorf("expected dbname appdb, got %q", target.dbname)
	}
}

// TestParseStudioTarget_DefaultsPort: a DATABASE_URL without an explicit
// port falls back to postgres's default, 5432.
func TestParseStudioTarget_DefaultsPort(t *testing.T) {
	target, err := parseStudioTarget("postgres://appuser:s3cret@localhost/appdb")
	if err != nil {
		t.Fatalf("parseStudioTarget: %v", err)
	}
	if target.port != "5432" {
		t.Errorf("expected default port 5432, got %q", target.port)
	}
}

// TestRunDbStudio_StartsAdminerAndStopsOnWait: runDbStudio starts an
// adminer container bound to the given port, reports a studio URL through
// onReady, and stops the container once wait returns — all without a real
// docker daemon, via the injected Runner seam.
func TestRunDbStudio_StartsAdminerAndStopsOnWait(t *testing.T) {
	var calls []string
	run := func(args ...string) (string, error) {
		calls = append(calls, strings.Join(args, " "))
		return "", nil
	}

	var gotURL string
	waited := false
	wait := func() { waited = true }

	err := runDbStudio("postgres://appuser:s3cret@localhost:5432/appdb", run, 54329, func(url string) {
		gotURL = url
	}, wait)
	if err != nil {
		t.Fatalf("runDbStudio: %v", err)
	}

	if !waited {
		t.Error("expected wait to be called before the container is stopped")
	}
	if len(calls) != 2 {
		t.Fatalf("expected exactly 2 docker invocations (run, stop), got %d: %v", len(calls), calls)
	}
	if !strings.Contains(calls[0], "run") || !strings.Contains(calls[0], "adminer") {
		t.Errorf("expected first call to run adminer, got: %q", calls[0])
	}
	if !strings.Contains(calls[0], "54329:8080") {
		t.Errorf("expected host port 54329 mapped to adminer's 8080, got: %q", calls[0])
	}
	if !strings.Contains(calls[0], studioContainerName) {
		t.Errorf("expected fixed container name %q, got: %q", studioContainerName, calls[0])
	}
	if calls[1] != "stop "+studioContainerName {
		t.Errorf("expected second call to stop the studio container, got: %q", calls[1])
	}

	if !strings.Contains(gotURL, "54329") || !strings.Contains(gotURL, "appuser") || !strings.Contains(gotURL, "appdb") {
		t.Errorf("expected onReady URL to carry port/user/db, got: %q", gotURL)
	}
}

// TestRunDbStudio_RunFailure_NeverCallsWaitOrStop: if the container fails
// to start, runDbStudio surfaces the error immediately rather than waiting
// on (or trying to stop) a container that never ran.
func TestRunDbStudio_RunFailure_NeverCallsWaitOrStop(t *testing.T) {
	var calls int
	run := func(args ...string) (string, error) {
		calls++
		return "", errors.New("docker: image not found")
	}

	waited := false
	err := runDbStudio("postgres://appuser:s3cret@localhost:5432/appdb", run, 54330, nil, func() { waited = true })
	if err == nil {
		t.Fatal("expected an error when the container fails to start")
	}
	if waited {
		t.Error("expected wait not to be called when the container never started")
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 docker invocation (the failed run), got %d", calls)
	}
}

// TestRunDbStudio_InvalidDatabaseURL_Errors: a malformed DATABASE_URL is
// rejected before any docker invocation is attempted.
func TestRunDbStudio_InvalidDatabaseURL_Errors(t *testing.T) {
	run := func(args ...string) (string, error) {
		t.Fatal("run should not be invoked for an invalid DATABASE_URL")
		return "", nil
	}
	err := runDbStudio("://not-a-url", run, 54331, nil, nil)
	if err == nil {
		t.Fatal("expected an error for an invalid DATABASE_URL")
	}
}
