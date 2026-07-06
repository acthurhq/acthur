package aiagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultClaudeModel is used when acthur.yml's `ai.model` is unset. It names
// the current Sonnet-tier model — the balanced default for an interactive
// CLI assistant (see docs/acthur-prd.md §17.7 example config, which pins an
// explicit model; this is only the fallback when the project doesn't).
const DefaultClaudeModel = "claude-sonnet-5"

const (
	defaultClaudeBaseURL = "https://api.anthropic.com"
	anthropicVersion     = "2023-06-01"
	claudeMaxTokens      = 4096
)

// ClaudeProvider is a real (non-mocked) client for the Anthropic Messages
// API, hand-rolled with net/http per this repo's "no new deps" bar rather
// than pulling in the anthropic-sdk-go module. It implements Provider.
type ClaudeProvider struct {
	APIKey  string
	Model   string
	BaseURL string // override for tests; defaults to the real API host

	// HTTPClient is injectable so tests never hit the network — see
	// claude_test.go, which points this at an httptest.Server.
	HTTPClient *http.Client
}

func (p *ClaudeProvider) httpClient() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func (p *ClaudeProvider) baseURL() string {
	if p.BaseURL != "" {
		return p.BaseURL
	}
	return defaultClaudeBaseURL
}

// messagesRequest is the wire shape of a POST /v1/messages request body —
// only the fields this provider needs (no tools, no thinking, no streaming).
type messagesRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []messagesReqTurn  `json:"messages"`
}

type messagesReqTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// messagesResponse is the wire shape of a successful (2xx) response.
type messagesResponse struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Role       string         `json:"role"`
	Content    []contentBlock `json:"content"`
	Model      string         `json:"model"`
	StopReason string         `json:"stop_reason"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// errorResponse is the wire shape of a non-2xx response per Anthropic's
// error envelope: {"type":"error","error":{"type":"...","message":"..."}}.
type errorResponse struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Complete sends a single-turn request (system + one user message) to the
// Anthropic Messages API and returns the concatenated text of every text
// content block in the response.
func (p *ClaudeProvider) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reqBody := messagesRequest{
		Model:     p.Model,
		MaxTokens: claudeMaxTokens,
		System:    systemPrompt,
		Messages:  []messagesReqTurn{{Role: "user", Content: userPrompt}},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("aiagent: failed to encode request: %w", err)
	}

	url := p.baseURL() + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("aiagent: failed to build request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", p.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := p.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("aiagent: request to Claude API failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("aiagent: failed to read Claude API response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody errorResponse
		if jsonErr := json.Unmarshal(body, &errBody); jsonErr == nil && errBody.Error.Message != "" {
			return "", fmt.Errorf("aiagent: Claude API returned %d (%s): %s",
				resp.StatusCode, errBody.Error.Type, errBody.Error.Message)
		}
		return "", fmt.Errorf("aiagent: Claude API returned %d: %s", resp.StatusCode, string(body))
	}

	var msg messagesResponse
	if err := json.Unmarshal(body, &msg); err != nil {
		return "", fmt.Errorf("aiagent: failed to decode Claude API response: %w", err)
	}

	var out bytes.Buffer
	for _, block := range msg.Content {
		if block.Type == "text" {
			out.WriteString(block.Text)
		}
	}
	return out.String(), nil
}
