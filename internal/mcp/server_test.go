package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// send writes one JSON-RPC request line to in and returns the single
// response line the server writes to out. Requests here are exercised via
// Server.Serve reading from a bytes.Reader (the whole session's input) and
// writing to a bytes.Buffer — this drives the real line-by-line protocol
// path, not a shortcut.
func serveOne(t *testing.T, s *Server, requestLine string) map[string]any {
	t.Helper()
	in := strings.NewReader(requestLine + "\n")
	var out bytes.Buffer
	if err := s.Serve(in, &out); err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}
	line := strings.TrimSpace(out.String())
	if line == "" {
		t.Fatalf("expected a response line for request %q, got none", requestLine)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v\nraw: %s", err, line)
	}
	return resp
}

func TestServer_Initialize(t *testing.T) {
	s := NewServer("acthur", "test", nil)
	resp := serveOne(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"0.1"}}}`)

	if resp["jsonrpc"] != "2.0" {
		t.Fatalf("bad jsonrpc field: %v", resp["jsonrpc"])
	}
	if resp["error"] != nil {
		t.Fatalf("unexpected error: %v", resp["error"])
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object, got %v", resp["result"])
	}
	if result["protocolVersion"] != ProtocolVersion {
		t.Fatalf("got protocolVersion %v, want %v", result["protocolVersion"], ProtocolVersion)
	}
	serverInfo, ok := result["serverInfo"].(map[string]any)
	if !ok || serverInfo["name"] != "acthur" {
		t.Fatalf("unexpected serverInfo: %v", result["serverInfo"])
	}
}

func TestServer_NotificationGetsNoResponse(t *testing.T) {
	s := NewServer("acthur", "test", nil)
	in := strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")
	var out bytes.Buffer
	if err := s.Serve(in, &out); err != nil {
		t.Fatalf("Serve error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no response to a notification, got: %s", out.String())
	}
}

func TestServer_ToolsList(t *testing.T) {
	tool := Tool{
		Name:        "ping_tool",
		Description: "returns pong",
		Handler:     func(json.RawMessage) (any, error) { return "pong", nil },
	}
	s := NewServer("acthur", "test", []Tool{tool})
	resp := serveOne(t, s, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)

	result := resp["result"].(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	first := tools[0].(map[string]any)
	if first["name"] != "ping_tool" {
		t.Fatalf("got tool name %v, want ping_tool", first["name"])
	}
	if first["description"] != "returns pong" {
		t.Fatalf("got description %v", first["description"])
	}
	if first["inputSchema"] == nil {
		t.Fatal("expected a non-nil inputSchema")
	}
}

func TestServer_ToolsCall_Success(t *testing.T) {
	tool := Tool{
		Name: "echo",
		Handler: func(args json.RawMessage) (any, error) {
			var in struct {
				Text string `json:"text"`
			}
			json.Unmarshal(args, &in)
			return map[string]string{"echoed": in.Text}, nil
		},
	}
	s := NewServer("acthur", "test", []Tool{tool})
	resp := serveOne(t, s, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"text":"hi"}}}`)

	result := resp["result"].(map[string]any)
	if result["isError"] == true {
		t.Fatalf("unexpected tool error: %v", result)
	}
	content := result["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(content))
	}
	block := content[0].(map[string]any)
	if block["type"] != "text" {
		t.Fatalf("got content type %v, want text", block["type"])
	}
	if !strings.Contains(block["text"].(string), "echoed") || !strings.Contains(block["text"].(string), "hi") {
		t.Fatalf("content text missing expected data: %v", block["text"])
	}
}

func TestServer_ToolsCall_HandlerErrorIsToolLevelNotRPCLevel(t *testing.T) {
	tool := Tool{
		Name:    "always_fails",
		Handler: func(json.RawMessage) (any, error) { return nil, errBoom },
	}
	s := NewServer("acthur", "test", []Tool{tool})
	resp := serveOne(t, s, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"always_fails","arguments":{}}}`)

	if resp["error"] != nil {
		t.Fatalf("expected no JSON-RPC-level error, got %v", resp["error"])
	}
	result := resp["result"].(map[string]any)
	if result["isError"] != true {
		t.Fatalf("expected isError:true, got %v", result)
	}
	content := result["content"].([]any)
	block := content[0].(map[string]any)
	if !strings.Contains(block["text"].(string), "boom") {
		t.Fatalf("expected error text to mention boom, got %v", block["text"])
	}
}

func TestServer_ToolsCall_UnknownToolIsInvalidParams(t *testing.T) {
	s := NewServer("acthur", "test", nil)
	resp := serveOne(t, s, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"nope","arguments":{}}}`)

	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected a JSON-RPC error for unknown tool, got %v", resp)
	}
	if int(errObj["code"].(float64)) != codeInvalidParams {
		t.Fatalf("got code %v, want %d", errObj["code"], codeInvalidParams)
	}
}

func TestServer_UnknownMethod(t *testing.T) {
	s := NewServer("acthur", "test", nil)
	resp := serveOne(t, s, `{"jsonrpc":"2.0","id":6,"method":"bogus/method"}`)
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error for unknown method, got %v", resp)
	}
	if int(errObj["code"].(float64)) != codeMethodNotFound {
		t.Fatalf("got code %v, want %d", errObj["code"], codeMethodNotFound)
	}
}

func TestServer_ParseError(t *testing.T) {
	s := NewServer("acthur", "test", nil)
	in := strings.NewReader("{not json\n")
	var out bytes.Buffer
	if err := s.Serve(in, &out); err != nil {
		t.Fatalf("Serve error: %v", err)
	}
	var resp map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
		t.Fatalf("expected valid JSON error response, got: %s", out.String())
	}
	errObj := resp["error"].(map[string]any)
	if int(errObj["code"].(float64)) != codeParseError {
		t.Fatalf("got code %v, want %d", errObj["code"], codeParseError)
	}
}

func TestServer_Ping(t *testing.T) {
	s := NewServer("acthur", "test", nil)
	resp := serveOne(t, s, `{"jsonrpc":"2.0","id":7,"method":"ping"}`)
	if resp["error"] != nil {
		t.Fatalf("unexpected error: %v", resp["error"])
	}
}

func TestServer_MultipleMessagesInOneSession(t *testing.T) {
	tool := Tool{Name: "noop", Handler: func(json.RawMessage) (any, error) { return "ok", nil }}
	s := NewServer("acthur", "test", []Tool{tool})

	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"noop","arguments":{}}}` + "\n"

	var out bytes.Buffer
	if err := s.Serve(strings.NewReader(input), &out); err != nil {
		t.Fatalf("Serve error: %v", err)
	}

	scanner := bufio.NewScanner(&out)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	// initialize, tools/list, tools/call → 3 responses; the notification gets none.
	if len(lines) != 3 {
		t.Fatalf("expected 3 response lines, got %d: %v", len(lines), lines)
	}
	var ids []float64
	for _, l := range lines {
		var resp map[string]any
		if err := json.Unmarshal([]byte(l), &resp); err != nil {
			t.Fatalf("invalid response line %q: %v", l, err)
		}
		ids = append(ids, resp["id"].(float64))
	}
	if ids[0] != 1 || ids[1] != 2 || ids[2] != 3 {
		t.Fatalf("responses out of order or missing ids: %v", ids)
	}
}

func TestServer_DuplicateToolNamePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate tool name")
		}
	}()
	dup := Tool{Name: "dup", Handler: func(json.RawMessage) (any, error) { return nil, nil }}
	NewServer("acthur", "test", []Tool{dup, dup})
}

var errBoom = &boomError{}

type boomError struct{}

func (*boomError) Error() string { return "boom" }
