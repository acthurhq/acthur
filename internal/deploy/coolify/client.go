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
//	GET    /api/v1/applications                 — list applications
//	POST   /api/v1/applications/dockercompose   — create a compose-based application
//	PATCH  /api/v1/applications/{uuid}           — update an existing application
//	POST   /api/v1/deploy?uuid={uuid}            — trigger a deployment
//	GET    /api/v1/applications/{uuid}           — read application status
//
// # Assumptions (documented — live-VPS verification still pending, see the
// implementation tracker)
//
//   - EnsureProject/EnsureComposeApp resolve "does this already exist" by
//     listing and matching on name client-side, since the documented API
//     has no filter-by-name query parameter. This is O(n) in list size but
//     correct, and is the safest assumption without a live instance to
//     confirm filter support against.
//   - The create-application request body uses field names
//     "project_uuid", "name", and "docker_compose_raw" — these are the
//     field names Coolify's own dashboard uses when creating a compose
//     application from raw YAML; the OpenAPI spec is not fully public for
//     this endpoint, so this is the minimal shape, not a verified contract.
//   - Deploy trigger response is assumed to have the shape
//     {"deployments": [{"deployment_uuid": "..."}]} — Coolify's documented
//     behavior for /api/v1/deploy?uuid=... targeting a single application
//     is to return a list because the same endpoint accepts multiple UUIDs
//     via ?uuid=a,b,c.
//   - Application health is read from the "status" string field on the
//     GET /api/v1/applications/{uuid} response, which Coolify reports as
//     "<state>:<health>" (e.g. "running:healthy", "exited:unhealthy").
//     WaitHealthy treats any status containing "running" as success and any
//     status containing "exited", "unhealthy", "failed", or "error" as a
//     terminal failure; anything else is treated as still-in-progress.
package coolify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a minimal typed Coolify v4 API client. Construct with New; the
// caller resolves the token (e.g. from the COOLIFY_TOKEN env var) — the
// client never reads the environment itself.
type Client struct {
	BaseURL string
	Token   string

	// HTTPClient is overridable for testing; defaults to http.DefaultClient.
	HTTPClient *http.Client

	// PollInterval is the delay between WaitHealthy polls. Defaults to 3s;
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

// application is the subset of a Coolify application resource this client
// needs.
type application struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	ProjectUUID string `json:"project_uuid"`
	Status      string `json:"status"`
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

// EnsureComposeApp creates a docker-compose-based application named name
// under the given project, or updates it in place with the new compose
// YAML if an application with that name already exists in the project.
// Returns the application's UUID.
func (c *Client) EnsureComposeApp(projectUUID, name, composeYAML string) (string, error) {
	var apps []application
	if err := c.do(http.MethodGet, "/api/v1/applications", nil, nil, &apps); err != nil {
		return "", fmt.Errorf("coolify: listing applications: %w", err)
	}
	for _, a := range apps {
		if a.Name == name && a.ProjectUUID == projectUUID {
			update := map[string]string{"docker_compose_raw": composeYAML}
			if err := c.do(http.MethodPatch, "/api/v1/applications/"+a.UUID, nil, update, nil); err != nil {
				return "", fmt.Errorf("coolify: updating application %q: %w", name, err)
			}
			return a.UUID, nil
		}
	}

	create := map[string]string{
		"project_uuid":       projectUUID,
		"name":               name,
		"docker_compose_raw": composeYAML,
	}
	var created application
	if err := c.do(http.MethodPost, "/api/v1/applications/dockercompose", nil, create, &created); err != nil {
		return "", fmt.Errorf("coolify: creating application %q: %w", name, err)
	}
	return created.UUID, nil
}

// deployResponse is the assumed shape of the /api/v1/deploy response — see
// the package doc comment for why this is an assumption, not a verified
// contract.
type deployResponse struct {
	Deployments []struct {
		DeploymentUUID string `json:"deployment_uuid"`
	} `json:"deployments"`
}

// Deploy triggers a deployment of the application identified by appUUID and
// returns the resulting deployment UUID.
func (c *Client) Deploy(appUUID string) (string, error) {
	query := url.Values{"uuid": []string{appUUID}}
	var resp deployResponse
	if err := c.do(http.MethodPost, "/api/v1/deploy", query, nil, &resp); err != nil {
		return "", fmt.Errorf("coolify: triggering deploy for %q: %w", appUUID, err)
	}
	if len(resp.Deployments) == 0 {
		return "", fmt.Errorf("coolify: deploy for %q returned no deployment record", appUUID)
	}
	return resp.Deployments[0].DeploymentUUID, nil
}

// terminalFailureMarkers are substrings of a Coolify application "status"
// field that indicate the deployment will never become healthy on its own.
var terminalFailureMarkers = []string{"exited", "unhealthy", "failed", "error"}

// WaitHealthy polls the application's status until it reports running, it
// reports a terminal failure, or timeout elapses — whichever comes first.
func (c *Client) WaitHealthy(appUUID string, timeout time.Duration) error {
	interval := c.PollInterval
	if interval <= 0 {
		interval = 3 * time.Second
	}

	deadline := time.Now().Add(timeout)
	var lastStatus string
	for {
		var app application
		if err := c.do(http.MethodGet, "/api/v1/applications/"+appUUID, nil, nil, &app); err != nil {
			return fmt.Errorf("coolify: polling status for %q: %w", appUUID, err)
		}
		lastStatus = app.Status

		if strings.Contains(app.Status, "running") {
			return nil
		}
		for _, marker := range terminalFailureMarkers {
			if strings.Contains(app.Status, marker) {
				return fmt.Errorf("coolify: deployment for %q failed with status %q", appUUID, app.Status)
			}
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("coolify: timed out waiting for %q to become healthy after %s (last status: %q)", appUUID, timeout, lastStatus)
		}
		time.Sleep(interval)
	}
}
