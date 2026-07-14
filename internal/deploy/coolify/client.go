// Package coolify is a minimal typed client for the Coolify v4 REST API,
// plus a deploy Target that pushes a project's docker-compose stack to a
// Coolify instance and waits for it to come up healthy (PRD phase-8
// deploy-runtime, slice 3, tracker #52).
//
// # API surface pinned
//
// Coolify v4's REST API is Bearer-token authenticated (the token is created
// in the Coolify UI/API keys and passed as "Authorization: Bearer <token>").
// This client implements exactly the endpoints this package needs, per the
// publicly documented shape (https://coolify.io/docs/api-reference):
//
//	GET    /api/v1/projects                    — list projects
//	POST   /api/v1/projects                     — create a project ({"name": ...})
//	GET    /api/v1/projects/{uuid}/environments — list project environments
//	POST   /api/v1/projects/{uuid}/environments — create a project environment
//	DELETE /api/v1/projects/{uuid}/environments/{name} — delete an empty environment
//	GET    /api/v1/services                     — list service stacks
//	POST   /api/v1/services                     — create a Compose service stack
//	PATCH  /api/v1/services/{uuid}              — update its Compose definition
//	DELETE /api/v1/services/{uuid}              — delete a Compose service stack
//	PATCH  /api/v1/services/{uuid}/envs         — upsert workload environment
//	POST   /api/v1/services/{uuid}/start        — deploy/redeploy the stack
//	GET    /api/v1/services/{uuid}              — read service status
//
// # Assumptions (documented — live-VPS verification still pending, see the
// implementation tracker)
//
//   - EnsureProject/EnsureComposeService resolve "does this already exist" by
//     listing and matching on name client-side, since the documented API
//     has no filter-by-name query parameter. This is O(n) in list size but
//     correct, and is the safest assumption without a live instance to
//     confirm filter support against.
//   - Service health is read from the "status" string field on the
//     GET /api/v1/services/{uuid} response, which Coolify reports as
//     "<state>:<health>" (e.g. "running:healthy", "exited:unhealthy").
//     WaitServiceHealthy treats any status containing "exited", "unhealthy",
//     "failed", or "error" as a terminal failure. Only a non-failing status
//     containing "running" is success; anything else remains in progress.
package coolify

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// UpsertServiceEnv updates one variable on a Compose service stack.
func (c *Client) UpsertServiceEnv(serviceUUID, key, value string) error {
	body := map[string]any{"key": key, "value": value, "is_literal": true}
	path := "/api/v1/services/" + serviceUUID + "/envs"
	var existing []struct {
		Key string `json:"key"`
	}
	if err := c.do(http.MethodGet, path, nil, nil, &existing); err != nil {
		return safeServiceEnvError(key, err)
	}
	method := http.MethodPost
	for _, env := range existing {
		if env.Key == key {
			method = http.MethodPatch
			break
		}
	}
	err := c.do(method, path, nil, body, nil)
	if err == nil {
		return nil
	}
	return safeServiceEnvError(key, err)
}

func safeServiceEnvError(key string, err error) error {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return fmt.Errorf("coolify: updating service environment key %q: HTTP %d", key, apiErr.Status)
	}
	return fmt.Errorf("coolify: updating service environment key %q failed", key)
}

// Client is a minimal typed Coolify v4 API client. Construct with New; the
// caller resolves the token (e.g. from the COOLIFY_TOKEN env var) — the
// client never reads the environment itself.
type Client struct {
	BaseURL string
	Token   string

	// HTTPClient is overridable for testing; defaults to http.DefaultClient.
	HTTPClient *http.Client

	// PollInterval is the delay between WaitServiceHealthy polls. Defaults to 3s;
	// tests override it to keep the suite fast.
	PollInterval time.Duration
}

// New constructs a Client for the given Coolify base URL (e.g.
// "https://coolify.example.com") and API token. The token must already be
// resolved by the caller (from COOLIFY_TOKEN or otherwise) — this package
// never reads environment variables itself.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL:      strings.TrimRight(baseURL, "/"),
		Token:        token,
		HTTPClient:   http.DefaultClient,
		PollInterval: 3 * time.Second,
	}
}

// APIError is returned when Coolify responds with a non-2xx status. It
// carries the status code and raw response body so callers (and the deploy
// gate) can surface exactly what the API said.
type APIError struct {
	Method string
	Path   string
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("coolify API %s %s: HTTP %d: %s", e.Method, e.Path, e.Status, e.Body)
}

// do issues an HTTP request against the Coolify API and decodes a JSON
// response into out (if non-nil). A non-2xx response is surfaced as an
// *APIError carrying the response body.
func (c *Client) do(method, path string, query url.Values, body any, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("coolify: encoding request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	fullURL := c.BaseURL + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	req, err := http.NewRequest(method, fullURL, reqBody)
	if err != nil {
		return fmt.Errorf("coolify: building request: %w", err)
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
		return fmt.Errorf("coolify: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("coolify: reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{Method: method, Path: path, Status: resp.StatusCode, Body: strings.TrimSpace(string(respBody))}
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("coolify: decoding response from %s %s: %w", method, path, err)
		}
	}
	return nil
}

// project is the subset of a Coolify project resource this client needs.
type project struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type environment struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type service struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	ProjectUUID string `json:"project_uuid"`
	Status      string `json:"status"`
}

// ServiceStatus is the public deployment state reported by Coolify for one
// exact Compose service stack.
type ServiceStatus struct {
	UUID   string
	Name   string
	Status string
}

// ComposeServiceStatus reports the current state of the exact named service
// in projectUUID. found is false when no deployment exists yet.
func (c *Client) ComposeServiceStatus(projectUUID, name string) (ServiceStatus, bool, error) {
	var services []service
	if err := c.do(http.MethodGet, "/api/v1/services", nil, nil, &services); err != nil {
		return ServiceStatus{}, false, fmt.Errorf("coolify: listing services: %w", err)
	}
	for _, listed := range services {
		if listed.Name != name || listed.ProjectUUID != projectUUID {
			continue
		}
		var current service
		if err := c.do(http.MethodGet, "/api/v1/services/"+listed.UUID, nil, nil, &current); err != nil {
			return ServiceStatus{}, false, fmt.Errorf("coolify: reading service %q: %w", listed.UUID, err)
		}
		return ServiceStatus{UUID: current.UUID, Name: current.Name, Status: current.Status}, true, nil
	}
	return ServiceStatus{}, false, nil
}

// DeleteComposeService removes only the exact named service in projectUUID.
// It returns false without error when the service is already absent.
func (c *Client) DeleteComposeService(projectUUID, name string) (bool, error) {
	var services []service
	if err := c.do(http.MethodGet, "/api/v1/services", nil, nil, &services); err != nil {
		return false, fmt.Errorf("coolify: listing services: %w", err)
	}
	for _, listed := range services {
		if listed.Name != name || listed.ProjectUUID != projectUUID {
			continue
		}
		query := url.Values{
			"delete_configurations":     {"true"},
			"delete_volumes":            {"true"},
			"docker_cleanup":            {"true"},
			"delete_connected_networks": {"true"},
		}
		err := c.do(http.MethodDelete, "/api/v1/services/"+listed.UUID, query, nil, nil)
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("coolify: deleting service %q: %w", listed.UUID, err)
		}
		return true, nil
	}
	return false, nil
}

// WaitComposeServiceAbsent waits for Coolify's queued service deletion to
// disappear before callers remove the containing environment.
func (c *Client) WaitComposeServiceAbsent(projectUUID, name string, timeout time.Duration) error {
	interval := c.PollInterval
	if interval <= 0 {
		interval = 3 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		var services []service
		if err := c.do(http.MethodGet, "/api/v1/services", nil, nil, &services); err != nil {
			return fmt.Errorf("coolify: checking deletion of service %q: %w", name, err)
		}
		found := false
		for _, listed := range services {
			if listed.Name == name && listed.ProjectUUID == projectUUID {
				found = true
				break
			}
		}
		if !found {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("coolify: timed out waiting for service %q deletion after %s", name, timeout)
		}
		time.Sleep(interval)
	}
}

// EnsureComposeService creates or updates the named current-generation
// Coolify service-stack resource.
func (c *Client) EnsureComposeService(projectUUID, serverUUID, destinationUUID, environmentName, name, composeYAML string) (string, error) {
	var services []service
	if err := c.do(http.MethodGet, "/api/v1/services", nil, nil, &services); err != nil {
		return "", fmt.Errorf("coolify: listing services: %w", err)
	}
	for _, svc := range services {
		if svc.Name == name && svc.ProjectUUID == projectUUID {
			if err := c.do(http.MethodPatch, "/api/v1/services/"+svc.UUID, nil, map[string]string{"docker_compose_raw": composeYAML}, nil); err != nil {
				return "", fmt.Errorf("coolify: updating service %q: %w", name, err)
			}
			return svc.UUID, nil
		}
	}
	body := map[string]string{
		"project_uuid":       projectUUID,
		"server_uuid":        serverUUID,
		"environment_name":   environmentName,
		"name":               name,
		"docker_compose_raw": composeYAML,
	}
	if destinationUUID != "" {
		body["destination_uuid"] = destinationUUID
	}
	var created service
	if err := c.do(http.MethodPost, "/api/v1/services", nil, body, &created); err != nil {
		return "", fmt.Errorf("coolify: creating service %q: %w", name, err)
	}
	return created.UUID, nil
}

func (c *Client) StartService(serviceUUID string) error {
	var resp struct {
		Message string `json:"message"`
	}
	if err := c.do(http.MethodPost, "/api/v1/services/"+serviceUUID+"/start", nil, nil, &resp); err != nil {
		return fmt.Errorf("coolify: starting service %q: %w", serviceUUID, err)
	}
	if resp.Message == "" {
		return fmt.Errorf("coolify: start for service %q returned no acknowledgement", serviceUUID)
	}
	return nil
}

func (c *Client) WaitServiceHealthy(serviceUUID string, timeout time.Duration) error {
	interval := c.PollInterval
	if interval <= 0 {
		interval = 3 * time.Second
	}
	deadline := time.Now().Add(timeout)
	var lastStatus string
	for {
		var svc service
		if err := c.do(http.MethodGet, "/api/v1/services/"+serviceUUID, nil, nil, &svc); err != nil {
			return fmt.Errorf("coolify: polling service status for %q: %w", serviceUUID, err)
		}
		lastStatus = svc.Status
		for _, marker := range terminalFailureMarkers {
			if strings.Contains(lastStatus, marker) {
				return fmt.Errorf("coolify: service %q failed with status %q", serviceUUID, lastStatus)
			}
		}
		if strings.Contains(lastStatus, "running") {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("coolify: timed out waiting for service %q after %s (last status: %q)", serviceUUID, timeout, lastStatus)
		}
		time.Sleep(interval)
	}
}

// EnsureProject returns the UUID of the Coolify project named name,
// creating it if it does not already exist.
func (c *Client) EnsureProject(name string) (string, error) {
	var projects []project
	if err := c.do(http.MethodGet, "/api/v1/projects", nil, nil, &projects); err != nil {
		return "", fmt.Errorf("coolify: listing projects: %w", err)
	}
	for _, p := range projects {
		if p.Name == name {
			return p.UUID, nil
		}
	}

	var created project
	if err := c.do(http.MethodPost, "/api/v1/projects", nil, map[string]string{"name": name}, &created); err != nil {
		return "", fmt.Errorf("coolify: creating project %q: %w", name, err)
	}
	return created.UUID, nil
}

// FindProject resolves name without creating or changing provider state.
func (c *Client) FindProject(name string) (string, bool, error) {
	var projects []project
	if err := c.do(http.MethodGet, "/api/v1/projects", nil, nil, &projects); err != nil {
		return "", false, fmt.Errorf("coolify: listing projects: %w", err)
	}
	for _, p := range projects {
		if p.Name == name {
			return p.UUID, true, nil
		}
	}
	return "", false, nil
}

// EnsureProjectEnvironment returns the UUID of name within projectUUID,
// creating it when needed. Coolify creates only a production environment for
// a new project, so named Acthur environments must be reconciled explicitly.
func (c *Client) EnsureProjectEnvironment(projectUUID, name string) (string, error) {
	path := "/api/v1/projects/" + projectUUID + "/environments"
	var environments []environment
	if err := c.do(http.MethodGet, path, nil, nil, &environments); err != nil {
		return "", fmt.Errorf("coolify: listing environments for project %q: %w", projectUUID, err)
	}
	for _, env := range environments {
		if env.Name == name {
			return env.UUID, nil
		}
	}

	var created environment
	if err := c.do(http.MethodPost, path, nil, map[string]string{"name": name}, &created); err != nil {
		return "", fmt.Errorf("coolify: creating environment %q: %w", name, err)
	}
	return created.UUID, nil
}

// DeleteProjectEnvironment deletes the exact environment after its service
// resources have been removed. An already absent environment is success.
func (c *Client) DeleteProjectEnvironment(projectUUID, name string) error {
	path := "/api/v1/projects/" + projectUUID + "/environments/" + url.PathEscape(name)
	err := c.do(http.MethodDelete, path, nil, nil, nil)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		return nil
	}
	if err != nil {
		return fmt.Errorf("coolify: deleting environment %q: %w", name, err)
	}
	return nil
}

// terminalFailureMarkers are substrings of a Coolify application "status"
// field that indicate the deployment will never become healthy on its own.
var terminalFailureMarkers = []string{"exited", "unhealthy", "failed", "error"}
