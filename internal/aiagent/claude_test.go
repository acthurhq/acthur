package aiagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClaudeProvider_Complete_SendsExpectedRequest(t *testing.T) {
	var gotPath, gotAPIKey, gotVersion, gotContentType string
	var gotBody messagesRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		gotContentType = r.Header.Get("content-type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := messagesResponse{
			ID:      "msg_1",
			Type:    "message",
			Role:    "assistant",
			Content: []contentBlock{{Type: "text", Text: "hello from claude"}},
			Model:   gotBody.Model,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &ClaudeProvider{
		APIKey:     "test-key",
		Model:      "claude-sonnet-5",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	text, err := p.Complete(context.Background(), "system context", "what is this?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "hello from claude" {
		t.Fatalf("got %q, want %q", text, "hello from claude")
	}
	if gotPath != "/v1/messages" {
		t.Fatalf("got path %q, want /v1/messages", gotPath)
	}
	if gotAPIKey != "test-key" {
		t.Fatalf("got x-api-key %q, want test-key", gotAPIKey)
	}
	if gotVersion != anthropicVersion {
		t.Fatalf("got anthropic-version %q, want %q", gotVersion, anthropicVersion)
	}
	if !strings.Contains(gotContentType, "application/json") {
		t.Fatalf("got content-type %q", gotContentType)
	}
	if gotBody.Model != "claude-sonnet-5" {
		t.Fatalf("got model %q, want claude-sonnet-5", gotBody.Model)
	}
	if gotBody.System != "system context" {
		t.Fatalf("got system %q, want %q", gotBody.System, "system context")
	}
	if len(gotBody.Messages) != 1 || gotBody.Messages[0].Content != "what is this?" {
		t.Fatalf("unexpected messages: %+v", gotBody.Messages)
	}
}

func TestClaudeProvider_Complete_ConcatenatesMultipleTextBlocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		resp := messagesResponse{
			Content: []contentBlock{
				{Type: "text", Text: "part one. "},
				{Type: "text", Text: "part two."},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &ClaudeProvider{APIKey: "k", Model: "claude-sonnet-5", BaseURL: server.URL, HTTPClient: server.Client()}
	text, err := p.Complete(context.Background(), "", "q")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "part one. part two." {
		t.Fatalf("got %q", text)
	}
}

func TestClaudeProvider_Complete_NonOKStatusReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"type": "error",
			"error": map[string]string{
				"type":    "authentication_error",
				"message": "invalid x-api-key",
			},
		})
	}))
	defer server.Close()

	p := &ClaudeProvider{APIKey: "bad", Model: "claude-sonnet-5", BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := p.Complete(context.Background(), "", "q")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "authentication_error") || !strings.Contains(err.Error(), "invalid x-api-key") {
		t.Fatalf("error %q missing expected detail", err.Error())
	}
}
