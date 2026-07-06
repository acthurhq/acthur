package aiagent

import "context"

// Provider is the seam every `acthur agent` command completes a prompt
// through. Real network calls live behind implementations (ClaudeProvider);
// command logic and tests depend only on this interface.
type Provider interface {
	// Complete sends a system prompt (graph/contract context) and a user
	// prompt (the operator's question/instruction) to the model and
	// returns its text response.
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}
