package deploy

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Runner executes a docker CLI invocation and returns its stdout. Injected
// so the target is testable without a daemon; production uses DockerRunner.
type Runner func(args ...string) (string, error)

// DockerRunner shells out to the real docker CLI.
func DockerRunner(args ...string) (string, error) {
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out), nil
}

// ComposeTarget deploys the graph's production projection to a Docker
// daemon via docker compose — the locally witnessable deploy target.
type ComposeTarget struct {
	File string // path to docker-compose.prod.yml, relative to project root
	run  Runner
}

func NewComposeTarget(file string, run Runner) *ComposeTarget {
	return &ComposeTarget{File: file, run: run}
}

// Up builds images and starts the production stack detached.
func (t *ComposeTarget) Up() error {
	if _, err := t.run("compose", "-f", t.File, "up", "-d", "--build"); err != nil {
		return fmt.Errorf("compose up failed: %w", err)
	}
	return nil
}

// Down tears the stack down.
func (t *ComposeTarget) Down() error {
	if _, err := t.run("compose", "-f", t.File, "down"); err != nil {
		return fmt.Errorf("compose down failed: %w", err)
	}
	return nil
}

// ServiceStatus is one service's state as compose reports it.
type ServiceStatus struct {
	Service string `json:"Service"`
	State   string `json:"State"`
	Health  string `json:"Health"`
}

// Status reports every service in the stack (compose ps --format json emits
// one JSON object per line on modern docker, or a JSON array on older ones —
// both are accepted).
func (t *ComposeTarget) Status() ([]ServiceStatus, error) {
	out, err := t.run("compose", "-f", t.File, "ps", "--format", "json")
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	if strings.HasPrefix(out, "[") {
		var services []ServiceStatus
		if err := json.Unmarshal([]byte(out), &services); err != nil {
			return nil, fmt.Errorf("parsing compose ps output: %w", err)
		}
		return services, nil
	}
	var services []ServiceStatus
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var s ServiceStatus
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			return nil, fmt.Errorf("parsing compose ps line %q: %w", line, err)
		}
		services = append(services, s)
	}
	return services, nil
}

// WaitHealthy polls Status until every service is healthy (or running with
// no healthcheck), or the timeout elapses — in which case the error names
// every service still unhealthy.
func (t *ComposeTarget) WaitHealthy(timeout, interval time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastUnhealthy []string
	for {
		services, err := t.Status()
		if err != nil {
			return err
		}
		lastUnhealthy = nil
		for _, s := range services {
			ok := s.State == "running" && (s.Health == "" || s.Health == "healthy")
			if !ok {
				lastUnhealthy = append(lastUnhealthy, fmt.Sprintf("%s (state=%s health=%s)", s.Service, s.State, s.Health))
			}
		}
		if len(services) > 0 && len(lastUnhealthy) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("stack did not become healthy within %s: %s", timeout, strings.Join(lastUnhealthy, ", "))
		}
		time.Sleep(interval)
	}
}
