// Package mcp implements a Model Context Protocol server over stdio.
//
// It hand-rolls the wire protocol (JSON-RPC 2.0, newline-delimited over
// stdin/stdout — the MCP spec's stdio transport: each message is exactly one
// line of JSON, messages MUST NOT contain embedded newlines) rather than
// pulling in an SDK dependency, per this repo's "no new deps unless clearly
// better" bar. The surface implemented is deliberately the minimal subset a
// real MCP client needs to discover and call read-only tools:
//
//   - initialize / notifications/initialized handshake
//   - tools/list
//   - tools/call
//   - ping (trivial liveness check some clients send)
//
// Anything else (resources, prompts, sampling, roots, logging) is not part
// of this server's scope — acthur exposes read-only introspection tools
// only, not the full protocol surface.
package mcp

import "encoding/json"

// ProtocolVersion is the MCP protocol date-version this server implements.
const ProtocolVersion = "2024-11-05"

// jsonrpcVersion is the fixed JSON-RPC envelope version per spec.
const jsonrpcVersion = "2.0"

// request is an inbound JSON-RPC 2.0 request or notification. A notification
// has no "id" field (Go zero-values it to nil, which round-trips correctly:
// omitempty on the response ensures we never echo back a synthesized id).
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// isNotification reports whether the request carries no id — per JSON-RPC
// 2.0, notifications never receive a response.
func (r request) isNotification() bool {
	return len(r.ID) == 0
}

// response is an outbound JSON-RPC 2.0 response. Exactly one of Result/Error
// is populated.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// rpcError is a JSON-RPC 2.0 error object.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Standard JSON-RPC 2.0 error codes (spec §5.1).
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

func newResponse(id json.RawMessage, result any) response {
	return response{JSONRPC: jsonrpcVersion, ID: id, Result: result}
}

func newErrorResponse(id json.RawMessage, code int, message string) response {
	return response{JSONRPC: jsonrpcVersion, ID: id, Error: &rpcError{Code: code, Message: message}}
}

// ---------------------------------------------------------------------------
// MCP-specific message shapes
// ---------------------------------------------------------------------------

type initializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      clientInfo     `json:"clientInfo"`
}

type clientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type initializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      serverInfo     `json:"serverInfo"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// toolDescriptor is the wire shape of one entry in tools/list's result.
type toolDescriptor struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type toolsListResult struct {
	Tools []toolDescriptor `json:"tools"`
}

type toolsCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolsCallResult struct {
	Content []contentBlock `json:"content"`
	IsError bool            `json:"isError,omitempty"`
}
