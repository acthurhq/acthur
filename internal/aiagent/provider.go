// Package aiagent provides the LLM-provider plumbing behind `acthur agent`:
// resolving an acthur.yml `ai:` block plus environment variables into a
// usable API key, a small Provider interface real providers implement, and
// a Context builder that turns the live graph/contract/config into the
// prompt context an AI-powered command sends the model.
//
// Only one provider is implemented today (Claude, internal/aiagent/claude.go)
// — a real HTTP call to the Anthropic Messages API, not a mock. The other
// three providers named in the PRD's `ai:` config (openai, groq, ollama) are
// explicitly not implemented; ResolveProvider returns a descriptive error
// for them rather than silently no-opping. See
// docs/implementation/active/0007-prd-completion.md for the scoping note.
package aiagent

import (
	"fmt"
	"os"

	"github.com/acthur/acthur/internal/config"
)

// defaultEnvVar maps a provider to the conventional environment variable
// name PRD §17.7's example config expects (e.g. `api_key: ${ANTHROPIC_API_KEY}`).
// Used as a fallback when acthur.yml sets no `api_key` at all, so a bare
// `ai: {provider: claude}` still works if the env var is exported.
func defaultEnvVar(p config.LLMProvider) string {
	switch p {
	case config.ProviderClaude:
		return "ANTHROPIC_API_KEY"
	case config.ProviderOpenAI:
		return "OPENAI_API_KEY"
	case config.ProviderGroq:
		return "GROQ_API_KEY"
	default:
		return ""
	}
}

// ResolveAPIKey resolves an AIConfig's api_key into a usable secret value.
// acthur.yml's documented convention is `api_key: ${ANTHROPIC_API_KEY}` —
// os.ExpandEnv handles that syntax (and bare $VAR). If the field is empty
// or expands to empty, it falls back to the provider's conventional env
// var name directly. ollama needs no key (local, unauthenticated by
// default) and always resolves to "".
func ResolveAPIKey(ai config.AIConfig) string {
	if ai.Provider == config.ProviderOllama {
		return ""
	}
	if ai.APIKey != "" {
		if expanded := os.ExpandEnv(ai.APIKey); expanded != "" {
			return expanded
		}
	}
	if v := defaultEnvVar(ai.Provider); v != "" {
		return os.Getenv(v)
	}
	return ""
}

// ErrProviderNotConfigured is returned by ResolveProvider when acthur.yml
// has no `ai:` block (or an empty provider) — the honest "nothing to talk
// to" case, distinct from a misconfigured/unsupported provider.
var ErrProviderNotConfigured = fmt.Errorf(
	"no AI provider configured — add an `ai:` block to acthur.yml (see docs/acthur-prd.md §17.7), e.g.:\n" +
		"  ai:\n    provider: claude\n    model: claude-sonnet-4-5\n    api_key: ${ANTHROPIC_API_KEY}",
)

// ResolveProvider builds a Provider from acthur.yml's `ai:` block. It never
// fakes support: an unimplemented provider or missing API key is a
// pointed error, never a silent no-op or mocked response.
func ResolveProvider(ai config.AIConfig) (Provider, error) {
	if ai.Provider == "" {
		return nil, ErrProviderNotConfigured
	}

	switch ai.Provider {
	case config.ProviderClaude:
		key := ResolveAPIKey(ai)
		if key == "" {
			return nil, fmt.Errorf(
				"AI provider %q configured but no API key resolved — set `ai.api_key` in acthur.yml "+
					"(e.g. `api_key: ${ANTHROPIC_API_KEY}`) and export %s",
				ai.Provider, defaultEnvVar(ai.Provider),
			)
		}
		model := ai.Model
		if model == "" {
			model = DefaultClaudeModel
		}
		return &ClaudeProvider{APIKey: key, Model: model, BaseURL: ai.BaseURL}, nil

	case config.ProviderOpenAI, config.ProviderGroq, config.ProviderOllama:
		return nil, fmt.Errorf(
			"AI provider %q is not implemented yet — only %q is wired up to a real API today; "+
				"see docs/implementation/active/0007-prd-completion.md for the descoping note",
			ai.Provider, config.ProviderClaude,
		)

	default:
		return nil, fmt.Errorf("unknown AI provider %q (want one of: claude, openai, groq, ollama)", ai.Provider)
	}
}
