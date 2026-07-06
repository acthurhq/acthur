// Package render is a minimal typed client for the Render REST API
// (https://api.render.com/v1), plus a deploy Target that builds+pushes a
// project's Dockerized services to an external registry and runs them as
// Render web services (PRD phase-9, tracker #64).
//
// # API surface pinned
//
// Render's REST API (documented at https://api-docs.render.com) is
// Bearer-token authenticated ("Authorization: Bearer <token>", a Render API
// key from the dashboard). This client implements exactly the endpoints
// this package needs:
//
//	GET  /v1/owners                       — list workspaces the key can act as
//	GET  /v1/services?name=&limit=        — find a service by name (list+filter)
//	POST /v1/services                     — create a service from a prebuilt image
//	POST /v1/services/{id}/deploys        — trigger a new deploy from an image
//	GET  /v1/services/{id}/deploys/{did}  — read a deploy's status
//
// # Build/push (documented assumption)
//
// Render's REST API creates a service either from a connected git repo
// (Render clones and builds it) or from an already-pullable image
// (`image.imagePath`, "REST API with ... service create" per this
// package's task scope) — it has no endpoint that accepts raw Dockerfile
// content or a local build context. Render also does not offer its own
// registry, so this target requires the caller to configure an external
// registry it can build+push to (docker CLI via the injected Runner) that
// Render's servers can pull from — see Target.Registry /
// RENDER_IMAGE_REGISTRY.
//
// # Owner resolution
//
// Service creation requires an ownerId. This client resolves it by calling
// GET /v1/owners and taking the first entry — a Render API key is scoped to
// exactly one workspace/owner in the common case; a key with access to
// multiple owners is not disambiguated here (documented limitation, same
// spirit as coolify's documented assumptions).
package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client is a minimal typed Render REST API client. Construct with New; the
// caller resolves the token (e.g. from RENDER_API_KEY) — the client never
// reads the environment itself.
type Client struct {
	BaseURL string
	Token   string

	// HTTPClient is overridable for testing; defaults to http.DefaultClient.
	HTTPClient *http.Client
}

// New constructs a Client against the Render REST API for the given token.
func New(token string) *Client {
	return &Client{
		BaseURL:    "https://api.render.com/v1",
		Token:      token,
		HTTPClient: http.DefaultClient,
	}
}

// APIError is returned when Render responds with a non-2xx status.
type APIError struct {
	Method string
	Path   string
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("render API %s %s: HTTP %d: %s", e.Method, e.Path, e.Status, e.Body)
}

func (c *Client) do(method, path string, query url.Values, body any, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("render: encoding request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	fullURL := c.BaseURL + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	req, err := http.NewRequest(method, fullURL, reqBody)
	if err != nil {
		return fmt.Errorf("render: building request: %w", err)
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
		return fmt.Errorf("render: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("render: reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{Method: method, Path: path, Status: resp.StatusCode, Body: strings.TrimSpace(string(respBody))}
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("render: decoding response from %s %s: %w", method, path, err)
		}
	}
	return nil
}

type ownerEntry struct {
	Owner struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"owner"`
}

// EnsureOwner returns the ID of the first workspace/owner the token can act
// as (see package doc for the disambiguation caveat).
func (c *Client) EnsureOwner() (string, error) {
	var owners []ownerEntry
	if err := c.do(http.MethodGet, "/owners", nil, nil, &owners); err != nil {
		return "", fmt.Errorf("render: listing owners: %w", err)
	}
	if len(owners) == 0 {
		return "", fmt.Errorf("render: token has no accessible owners/workspaces")
	}
	return owners[0].Owner.ID, nil
}

type serviceEntry struct {
	Service struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"service"`
}

type serviceResource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// EnsureService returns the ID of the service named name, creating it under
// ownerID sourced from image if it does not already exist. Render redeploys
// an existing service to a new image via Deploy rather than a service
// update, so an already-existing service's ID is simply returned here.
func (c *Client) EnsureService(ownerID, name, image string) (string, error) {
	var entries []serviceEntry
	query := url.Values{"name": []string{name}, "limit": []string{"20"}}
	if err := c.do(http.MethodGet, "/services", query, nil, &entries); err != nil {
		return "", fmt.Errorf("render: listing services: %w", err)
	}
	for _, e := range entries {
		if e.Service.Name == name {
			return e.Service.ID, nil
		}
	}

	create := map[string]any{
		"type":    "web_service",
		"name":    name,
		"ownerId": ownerID,
		"image":   map[string]string{"imagePath": image},
		"serviceDetails": map[string]any{
			"env":  "image",
			"plan": "starter",
		},
	}
	var created serviceResource
	if err := c.do(http.MethodPost, "/services", nil, create, &created); err != nil {
		return "", fmt.Errorf("render: creating service %q: %w", name, err)
	}
	return created.ID, nil
}

type deployResource struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// Deploy triggers a new deploy of serviceID from imageURL and returns the
// deploy's ID.
func (c *Client) Deploy(serviceID, imageURL string) (string, error) {
	var created deployResource
	body := map[string]string{"imageUrl": imageURL}
	if err := c.do(http.MethodPost, "/services/"+serviceID+"/deploys", nil, body, &created); err != nil {
		return "", fmt.Errorf("render: triggering deploy for %q: %w", serviceID, err)
	}
	return created.ID, nil
}

// DeployStatus reads the current status of a deploy.
func (c *Client) DeployStatus(serviceID, deployID string) (string, error) {
	var d deployResource
	if err := c.do(http.MethodGet, "/services/"+serviceID+"/deploys/"+deployID, nil, nil, &d); err != nil {
		return "", fmt.Errorf("render: reading deploy %q: %w", deployID, err)
	}
	return d.Status, nil
}
