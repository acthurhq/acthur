package aiagent

import (
	"os"
	"testing"

	"github.com/acthurhq/acthur/internal/config"
)

func TestResolveAPIKey_ExpandsEnvVarSyntax(t *testing.T) {
	t.Setenv("ACTHUR_TEST_KEY", "secret-123")
	got := ResolveAPIKey(config.AIConfig{Provider: config.ProviderClaude, APIKey: "${ACTHUR_TEST_KEY}"})
	if got != "secret-123" {
		t.Fatalf("got %q, want secret-123", got)
	}
}

func TestResolveAPIKey_FallsBackToConventionalEnvVar(t *testing.T) {
	_ = os.Unsetenv("ANTHROPIC_API_KEY")
	t.Setenv("ANTHROPIC_API_KEY", "fallback-key")
	got := ResolveAPIKey(config.AIConfig{Provider: config.ProviderClaude})
	if got != "fallback-key" {
		t.Fatalf("got %q, want fallback-key", got)
	}
}

func TestResolveAPIKey_OllamaNeedsNoKey(t *testing.T) {
	got := ResolveAPIKey(config.AIConfig{Provider: config.ProviderOllama, APIKey: "${ANYTHING}"})
	if got != "" {
		t.Fatalf("got %q, want empty string for ollama", got)
	}
}

func TestResolveAPIKey_EmptyWhenNothingSet(t *testing.T) {
	_ = os.Unsetenv("ANTHROPIC_API_KEY")
	got := ResolveAPIKey(config.AIConfig{Provider: config.ProviderClaude})
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestResolveProvider_NoProviderConfigured(t *testing.T) {
	_, err := ResolveProvider(config.AIConfig{})
	if err != ErrProviderNotConfigured {
		t.Fatalf("got %v, want ErrProviderNotConfigured", err)
	}
}

func TestResolveProvider_ClaudeMissingKey(t *testing.T) {
	_ = os.Unsetenv("ANTHROPIC_API_KEY")
	_, err := ResolveProvider(config.AIConfig{Provider: config.ProviderClaude})
	if err == nil {
		t.Fatal("expected error for missing API key, got nil")
	}
}

func TestResolveProvider_ClaudeSucceeds(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	p, err := ResolveProvider(config.AIConfig{Provider: config.ProviderClaude})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cp, ok := p.(*ClaudeProvider)
	if !ok {
		t.Fatalf("expected *ClaudeProvider, got %T", p)
	}
	if cp.Model != DefaultClaudeModel {
		t.Fatalf("got model %q, want default %q", cp.Model, DefaultClaudeModel)
	}
	if cp.APIKey != "test-key" {
		t.Fatalf("got key %q, want test-key", cp.APIKey)
	}
}

func TestResolveProvider_ClaudeHonorsExplicitModel(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	p, err := ResolveProvider(config.AIConfig{Provider: config.ProviderClaude, Model: "claude-opus-4-8"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cp := p.(*ClaudeProvider)
	if cp.Model != "claude-opus-4-8" {
		t.Fatalf("got model %q, want claude-opus-4-8", cp.Model)
	}
}

func TestResolveProvider_UnimplementedProvidersDescoped(t *testing.T) {
	for _, prov := range []config.LLMProvider{config.ProviderOpenAI, config.ProviderGroq, config.ProviderOllama} {
		_, err := ResolveProvider(config.AIConfig{Provider: prov})
		if err == nil {
			t.Fatalf("provider %q: expected descoping error, got nil", prov)
		}
	}
}

func TestResolveProvider_UnknownProvider(t *testing.T) {
	_, err := ResolveProvider(config.AIConfig{Provider: "bogus"})
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}
