package mcp

import "encoding/json"

// Tool is one MCP tool exposed by the server. Handler receives the raw
// "arguments" object from a tools/call request (nil/empty for tools that
// take no arguments) and returns either a result value (marshaled to JSON
// text and wrapped in a single content block) or an error, which is
// reported to the client as a tool-level failure (isError: true), not a
// JSON-RPC protocol error — per spec, a failed tool call is still a
// successful RPC.
type Tool struct {
	Name        string
	Description string
	// InputSchema is a JSON Schema object describing the tool's arguments.
	// Use map[string]any{"type": "object", "properties": map[string]any{},
	// "additionalProperties": false} for a no-argument tool.
	InputSchema map[string]any
	Handler     func(args json.RawMessage) (any, error)
}

func (t Tool) descriptor() toolDescriptor {
	schema := t.InputSchema
	if schema == nil {
		schema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return toolDescriptor{Name: t.Name, Description: t.Description, InputSchema: schema}
}
