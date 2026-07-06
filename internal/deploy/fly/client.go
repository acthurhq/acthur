// Package fly is a minimal typed client for the Fly.io Machines API, plus a
// deploy Target that builds+pushes a project's Dockerized services to Fly's
// own registry and runs them as Fly Machines (PRD phase-9, tracker #64).
//
// # API surface pinned
//
// The Machines API (https://api.machines.dev/v1, documented at
// https://fly.io/docs/machines/api/) is Bearer-token authenticated (the
// token is a Fly API token, e.g. from `fly tokens create deploy`, passed as
// "Authorization: Bearer <token>"). This client implements exactly the
// endpoints this package needs:
//
//	GET   /v1/apps/{app_name}                       — read an app (404 if absent)
//	POST  /v1/apps                                    — create an app ({"app_name","org_slug"})
//	GET   /v1/apps/{app}/machines                     — list machines
//	POST  /v1/apps/{app}/machines                     — create a machine
//	POST  /v1/apps/{app}/machines/{id}                — update a machine (new image, etc.)
//	GET   /v1/apps/{app}/machines/{id}                — read a machine's state
//
// # Build/push (documented assumption)
//
// The Machines API deploys already-built images — it has no "build my
// Dockerfile" endpoint. This target builds each service's Dockerfile with
// the local/injected docker CLI and pushes to Fly's own registry
// (registry.fly.io/<app>:<tag>), authenticating with `docker login
// registry.fly.io -u x -p <FLY_API_TOKEN>` — this is Fly's own documented
// convention (https://fly.io/docs/reference/registry/), not a guess: any
// Fly API token is also a valid registry password with username "x". No
// external registry account is required, unlike railway/render.
package fly

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a minimal typed Fly Machines API client. Construct with New; the
// caller resolves the token (e.g. from FLY_API_TOKEN) — the client never
// reads the environment itself.
type Client struct {
	BaseURL string
	Token   string

	// HTTPClient is overridable for testing; defaults to http.DefaultClient.
	HTTPClient *http.Client

	// PollInterval is the delay between WaitStarted polls. Defaults to 3s;
	// tests override it to keep the suite fast.
	PollInterval time.Duration
}

// New constructs a Client against the Fly Machines API for the given token.
func New(token string) *Client {
	return &Client{
		BaseURL:      "https://api.machines.dev/v1",
		Token:        token,
		HTTPClient:   http.DefaultClient,
		PollInterval: 3 * time.Second,
	}
}

// APIError is returned when Fly responds with a non-2xx status (other than
// the expected 404 on app lookups, which callers handle directly).
type APIError struct {
	Method string
	Path   string
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("fly API %s %s: HTTP %d: %s", e.Method, e.Path, e.Status, e.Body)
}

func (c *Client) do(method, path string, body any, out any) (int, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("fly: encoding request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, reqBody)
	if err != nil {
		return 0, fmt.Errorf("fly: building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("fly: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("fly: reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, &APIError{Method: method, Path: path, Status: resp.StatusCode, Body: strings.TrimSpace(string(respBody))}
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return resp.StatusCode, fmt.Errorf("fly: decoding response from %s %s: %w", method, path, err)
		}
	}
	return resp.StatusCode, nil
}

// App is the subset of a Fly app resource this client needs.
type App struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// EnsureApp creates the app under orgSlug if it does not already exist.
func (c *Client) EnsureApp(name, orgSlug string) error {
	_, err := c.do(http.MethodGet, "/apps/"+name, nil, &App{})
	if err == nil {
		return nil
	}
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != http.StatusNotFound {
		return fmt.Errorf("fly: checking app %q: %w", name, err)
	}

	if orgSlug == "" {
		orgSlug = "personal"
	}
	if _, err := c.do(http.MethodPost, "/apps", map[string]string{"app_name": name, "org_slug": orgSlug}, nil); err != nil {
		return fmt.Errorf("fly: creating app %q: %w", name, err)
	}
	return nil
}

// MachineConfig is the subset of a Fly machine config this client sets.
type MachineConfig struct {
	Image    string             `json:"image"`
	Guest    MachineGuest       `json:"guest"`
	Services []MachineSvcConfig `json:"services,omitempty"`
}

type MachineGuest struct {
	CPUKind  string `json:"cpu_kind"`
	CPUs     int    `json:"cpus"`
	MemoryMB int    `json:"memory_mb"`
}

type MachineSvcConfig struct {
	Protocol     string          `json:"protocol"`
	InternalPort int             `json:"internal_port"`
	Ports        []MachinePort   `json:"ports"`
	Checks       []MachineChecks `json:"checks,omitempty"`
}

type MachinePort struct {
	Port     int      `json:"port"`
	Handlers []string `json:"handlers"`
}

type MachineChecks struct {
	Type     string `json:"type"`
	Path     string `json:"path,omitempty"`
	Interval string `json:"interval,omitempty"`
	Timeout  string `json:"timeout,omitempty"`
}

// Machine is the subset of a Fly machine resource this client needs.
type Machine struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	State  string        `json:"state"`
	Config MachineConfig `json:"config"`
}

// EnsureMachine creates a machine named name running image in app, or
// updates it in place with the new image if a machine with that name
// already exists. Returns the machine's ID.
func (c *Client) EnsureMachine(app, name, image string, cfg MachineConfig) (string, error) {
	cfg.Image = image

	var machines []Machine
	if _, err := c.do(http.MethodGet, "/apps/"+app+"/machines", nil, &machines); err != nil {
		return "", fmt.Errorf("fly: listing machines for app %q: %w", app, err)
	}
	for _, m := range machines {
		if m.Name == name {
			if _, err := c.do(http.MethodPost, "/apps/"+app+"/machines/"+m.ID, map[string]any{"config": cfg}, nil); err != nil {
				return "", fmt.Errorf("fly: updating machine %q: %w", name, err)
			}
			return m.ID, nil
		}
	}

	var created Machine
	if _, err := c.do(http.MethodPost, "/apps/"+app+"/machines", map[string]any{"name": name, "config": cfg}, &created); err != nil {
		return "", fmt.Errorf("fly: creating machine %q: %w", name, err)
	}
	return created.ID, nil
}

// terminalFailureStates are Fly machine "state" values that indicate the
// machine will never reach "started" on its own.
var terminalFailureStates = []string{"failed", "destroyed", "destroying"}

// WaitStarted polls the machine's state until it reports "started", a
// terminal failure state, or timeout elapses — whichever comes first.
func (c *Client) WaitStarted(app, machineID string, timeout time.Duration) error {
	interval := c.PollInterval
	if interval <= 0 {
		interval = 3 * time.Second
	}

	deadline := time.Now().Add(timeout)
	var lastState string
	for {
		var m Machine
		if _, err := c.do(http.MethodGet, "/apps/"+app+"/machines/"+machineID, nil, &m); err != nil {
			return fmt.Errorf("fly: polling state for machine %q: %w", machineID, err)
		}
		lastState = m.State

		if m.State == "started" {
			return nil
		}
		for _, marker := range terminalFailureStates {
			if m.State == marker {
				return fmt.Errorf("fly: machine %q entered terminal state %q", machineID, m.State)
			}
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("fly: timed out waiting for machine %q to start after %s (last state: %q)", machineID, timeout, lastState)
		}
		time.Sleep(interval)
	}
}
