// Package railway is a minimal typed client for Railway's public GraphQL
// API, plus a deploy Target that builds+pushes a project's Dockerized
// services to an external registry and runs them as Railway services (PRD
// phase-9, tracker #64).
//
// # API surface pinned
//
// Railway's public API (https://backboard.railway.com/graphql/v2,
// documented at https://docs.railway.com/reference/public-api) is a single
// GraphQL endpoint, Bearer-token authenticated ("Authorization: Bearer
// <token>", a Railway account or project token). This client sends exactly
// the operations this package needs:
//
//	query    projects            — list projects (find-by-name, since the
//	                                schema has no filter-by-name argument)
//	mutation projectCreate        — create a project
//	query    project              — read a project's services/environments
//	mutation serviceCreate         — create a service from an image source
//	mutation serviceInstanceUpdate — point an existing service at a new image
//	mutation serviceInstanceDeploy — trigger a deployment of a service instance
//	query    deployments           — poll the latest deployment's status
//
// # Build/push (documented assumption)
//
// Railway's public GraphQL API deploys a service from a source — either a
// connected git repo or an already-pullable image reference
// (`ServiceSourceInput{image: "..."}`) — it has no endpoint that accepts
// raw Dockerfile content or a local build context. Unlike Fly, Railway does
// not offer its own registry, so this target requires the caller to
// configure an external registry it can build+push to (docker CLI via the
// injected Runner) and that Railway's servers can pull from (public, or a
// private registry with credentials already configured in the Railway
// project) — see Target.Registry / RAILWAY_IMAGE_REGISTRY.
package railway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client is a minimal GraphQL client for Railway's public API. Construct
// with New; the caller resolves the token (e.g. from RAILWAY_TOKEN) — the
// client never reads the environment itself.
type Client struct {
	BaseURL string
	Token   string

	// HTTPClient is overridable for testing; defaults to http.DefaultClient.
	HTTPClient *http.Client
}

// New constructs a Client against Railway's public GraphQL API for the
// given token.
func New(token string) *Client {
	return &Client{
		BaseURL:    "https://backboard.railway.com/graphql/v2",
		Token:      token,
		HTTPClient: http.DefaultClient,
	}
}

// GraphQLError is returned when the API responds with a top-level "errors"
// array — the standard GraphQL error envelope.
type GraphQLError struct {
	Messages []string
}

func (e *GraphQLError) Error() string {
	return fmt.Sprintf("railway API: %s", strings.Join(e.Messages, "; "))
}

type gqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type gqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// do executes a GraphQL query/mutation and decodes the "data" field of the
// response into out (if non-nil).
func (c *Client) do(query string, variables map[string]any, out any) error {
	body, err := json.Marshal(gqlRequest{Query: query, Variables: variables})
	if err != nil {
		return fmt.Errorf("railway: encoding request body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.BaseURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("railway: building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("railway: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("railway: reading response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("railway: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var gr gqlResponse
	if err := json.Unmarshal(respBody, &gr); err != nil {
		return fmt.Errorf("railway: decoding response: %w", err)
	}
	if len(gr.Errors) > 0 {
		var msgs []string
		for _, e := range gr.Errors {
			msgs = append(msgs, e.Message)
		}
		return &GraphQLError{Messages: msgs}
	}
	if out != nil && len(gr.Data) > 0 {
		if err := json.Unmarshal(gr.Data, out); err != nil {
			return fmt.Errorf("railway: decoding data: %w", err)
		}
	}
	return nil
}

// EnsureProject returns the ID of the Railway project named name, creating
// it if it does not already exist.
func (c *Client) EnsureProject(name string) (string, error) {
	var listResp struct {
		Projects struct {
			Edges []struct {
				Node struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"projects"`
	}
	if err := c.do(`query { projects { edges { node { id name } } } }`, nil, &listResp); err != nil {
		return "", fmt.Errorf("railway: listing projects: %w", err)
	}
	for _, e := range listResp.Projects.Edges {
		if e.Node.Name == name {
			return e.Node.ID, nil
		}
	}

	var createResp struct {
		ProjectCreate struct {
			ID string `json:"id"`
		} `json:"projectCreate"`
	}
	mutation := `mutation($input: ProjectCreateInput!) { projectCreate(input: $input) { id } }`
	if err := c.do(mutation, map[string]any{"input": map[string]any{"name": name}}, &createResp); err != nil {
		return "", fmt.Errorf("railway: creating project %q: %w", name, err)
	}
	return createResp.ProjectCreate.ID, nil
}

// ProjectDetail is the subset of a Railway project's services/environments
// this client needs to resolve existing resources by name.
type ProjectDetail struct {
	Environments []struct {
		ID   string
		Name string
	}
	Services []struct {
		ID   string
		Name string
	}
}

// Project reads a project's services and environments.
func (c *Client) Project(projectID string) (ProjectDetail, error) {
	var resp struct {
		Project struct {
			Environments struct {
				Edges []struct {
					Node struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"environments"`
			Services struct {
				Edges []struct {
					Node struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"services"`
		} `json:"project"`
	}
	query := `query($id: String!) { project(id: $id) {
		environments { edges { node { id name } } }
		services { edges { node { id name } } }
	} }`
	if err := c.do(query, map[string]any{"id": projectID}, &resp); err != nil {
		return ProjectDetail{}, fmt.Errorf("railway: reading project %q: %w", projectID, err)
	}

	var detail ProjectDetail
	for _, e := range resp.Project.Environments.Edges {
		detail.Environments = append(detail.Environments, struct {
			ID   string
			Name string
		}{e.Node.ID, e.Node.Name})
	}
	for _, e := range resp.Project.Services.Edges {
		detail.Services = append(detail.Services, struct {
			ID   string
			Name string
		}{e.Node.ID, e.Node.Name})
	}
	return detail, nil
}

// EnsureService creates a service named name sourced from image under
// projectID, or updates its source image in place if a service with that
// name already exists. Returns the service's ID.
func (c *Client) EnsureService(projectID, environmentID, name, image string) (string, error) {
	detail, err := c.Project(projectID)
	if err != nil {
		return "", err
	}
	for _, svc := range detail.Services {
		if svc.Name == name {
			mutation := `mutation($serviceId: String!, $environmentId: String!, $input: ServiceInstanceUpdateInput!) {
				serviceInstanceUpdate(serviceId: $serviceId, environmentId: $environmentId, input: $input)
			}`
			vars := map[string]any{
				"serviceId":     svc.ID,
				"environmentId": environmentID,
				"input":         map[string]any{"source": map[string]any{"image": image}},
			}
			if err := c.do(mutation, vars, nil); err != nil {
				return "", fmt.Errorf("railway: updating service %q: %w", name, err)
			}
			return svc.ID, nil
		}
	}

	var createResp struct {
		ServiceCreate struct {
			ID string `json:"id"`
		} `json:"serviceCreate"`
	}
	mutation := `mutation($input: ServiceCreateInput!) { serviceCreate(input: $input) { id } }`
	vars := map[string]any{"input": map[string]any{
		"projectId": projectID,
		"name":      name,
		"source":    map[string]any{"image": image},
	}}
	if err := c.do(mutation, vars, &createResp); err != nil {
		return "", fmt.Errorf("railway: creating service %q: %w", name, err)
	}
	return createResp.ServiceCreate.ID, nil
}

// Deploy triggers a deployment of serviceID in environmentID.
func (c *Client) Deploy(serviceID, environmentID string) error {
	mutation := `mutation($serviceId: String!, $environmentId: String!) {
		serviceInstanceDeploy(serviceId: $serviceId, environmentId: $environmentId)
	}`
	vars := map[string]any{"serviceId": serviceID, "environmentId": environmentID}
	if err := c.do(mutation, vars, nil); err != nil {
		return fmt.Errorf("railway: triggering deploy for %q: %w", serviceID, err)
	}
	return nil
}

// LatestDeploymentStatus returns the status string of the most recent
// deployment for serviceID in environmentID (e.g. "BUILDING", "DEPLOYING",
// "SUCCESS", "FAILED", "CRASHED").
func (c *Client) LatestDeploymentStatus(serviceID, environmentID string) (string, error) {
	var resp struct {
		Deployments struct {
			Edges []struct {
				Node struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"deployments"`
	}
	query := `query($input: DeploymentListInput!) { deployments(input: $input) { edges { node { id status } } } }`
	vars := map[string]any{"input": map[string]any{"serviceId": serviceID, "environmentId": environmentID}}
	if err := c.do(query, vars, &resp); err != nil {
		return "", fmt.Errorf("railway: listing deployments for %q: %w", serviceID, err)
	}
	if len(resp.Deployments.Edges) == 0 {
		return "", fmt.Errorf("railway: no deployments found for service %q", serviceID)
	}
	return resp.Deployments.Edges[0].Node.Status, nil
}
