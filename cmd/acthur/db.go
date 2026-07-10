package main

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/acthur/acthur/internal/deploy"
)

// studioContainerName is fixed and namespaced so a leftover container from a
// crashed `acthur db studio` is identifiable and won't collide with
// unrelated containers.
const studioContainerName = "acthur-db-studio"

// freeLocalPort asks the OS for an ephemeral local port, then releases it —
// good enough to pick a free host port for the short-lived studio
// container (a small race exists between release and the container's own
// bind, as with any "ask the OS, then use it later" port picker).
func freeLocalPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("finding a free local port: %w", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// studioTarget is what runDbStudio needs to point adminer at the project's
// db:postgres node, parsed once from the same DATABASE_URL every other
// `acthur db` command uses (migrations.DatabaseURL) — so studio can never
// drift from the project's actual credentials.
type studioTarget struct {
	port, user, dbname string
}

// parseStudioTarget extracts the pieces of databaseURL adminer's login form
// wants pre-filled (port, user, database name — never the password, which
// is entered manually rather than placed in a URL where it could end up in
// shell history or logs).
func parseStudioTarget(databaseURL string) (studioTarget, error) {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return studioTarget{}, fmt.Errorf("parsing DATABASE_URL: %w", err)
	}
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	return studioTarget{
		port:   port,
		user:   u.User.Username(),
		dbname: strings.TrimPrefix(u.Path, "/"),
	}, nil
}

// runDbStudio starts an adminer container bound to port on the host,
// reaching the project's postgres via host.docker.internal (works with
// Docker Desktop and, on Linux docker 20.10+, via --add-host's
// host-gateway alias) — an "honest minimal" DB studio: no bespoke UI, just
// wiring an existing one to the project's database.
//
// run is the docker CLI seam (see deploy.Runner) so this is testable
// without a docker daemon. wait blocks until the command should stop the
// container — production passes waitForInterrupt (blocks on Ctrl+C); tests
// pass a no-op so they return immediately. The container always runs with
// --rm, and is explicitly stopped on the way out.
func runDbStudio(databaseURL string, run deploy.Runner, port int, onReady func(url string), wait func()) error {
	target, err := parseStudioTarget(databaseURL)
	if err != nil {
		return err
	}

	args := []string{
		"run", "-d", "--rm",
		"--name", studioContainerName,
		"--add-host", "host.docker.internal:host-gateway",
		"-p", fmt.Sprintf("%d:8080", port),
		"-e", "ADMINER_DEFAULT_SERVER=host.docker.internal",
		"adminer",
	}
	if _, err := run(args...); err != nil {
		return fmt.Errorf("starting db studio container: %w", err)
	}

	studioURL := fmt.Sprintf(
		"http://localhost:%d/?pgsql=host.docker.internal%%3A%s&username=%s&db=%s",
		port, target.port, target.user, target.dbname,
	)
	if onReady != nil {
		onReady(studioURL)
	}

	if wait != nil {
		wait()
	}

	if _, err := run("stop", studioContainerName); err != nil {
		return fmt.Errorf("stopping db studio container: %w", err)
	}
	return nil
}

// waitForInterrupt blocks until the process receives SIGINT (Ctrl+C) or
// SIGTERM (sent by process managers, CI, and `docker stop`) — the
// production `wait` for runDbStudio, keeping `acthur db studio` in the
// foreground until told to stop. Without also catching SIGTERM, a manager
// that terminates the process without a Ctrl+C would skip the deferred
// `docker stop` in runDbStudio and leak the studio container.
func waitForInterrupt() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch
}
