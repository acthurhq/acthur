package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Server is a minimal MCP server driven over an injected reader/writer pair
// (stdin/stdout in production, in-memory pipes in tests — see server_test.go).
// It is safe for the lifetime of a single Serve call; it is not designed to
// be reused across multiple Serve calls.
type Server struct {
	Name    string
	Version string

	tools []Tool
	byName map[string]Tool

	mu          sync.Mutex // guards writes to the output stream
	initialized bool
}

// NewServer builds a server exposing the given tools. Tool names must be
// unique; NewServer panics on a duplicate (a programmer error caught at
// registration time, never at request time).
func NewServer(name, version string, tools []Tool) *Server {
	s := &Server{Name: name, Version: version, tools: tools, byName: make(map[string]Tool, len(tools))}
	for _, t := range tools {
		if _, dup := s.byName[t.Name]; dup {
			panic(fmt.Sprintf("mcp: duplicate tool name %q", t.Name))
		}
		s.byName[t.Name] = t
	}
	return s
}

// Serve reads newline-delimited JSON-RPC messages from in and writes
// responses to out until in is exhausted (EOF) or a read error occurs.
// EOF is a normal shutdown (the client closed stdin) and is not returned
// as an error.
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	// MCP messages can carry a full graph/contract dump; default 64KiB
	// scanner token limit is too small. 16MiB is generous headroom.
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		s.handleLine(line, out)
	}
	return scanner.Err()
}

func (s *Server) handleLine(line []byte, out io.Writer) {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		s.write(out, newErrorResponse(nil, codeParseError, "parse error: "+err.Error()))
		return
	}
	if req.JSONRPC != jsonrpcVersion {
		s.write(out, newErrorResponse(req.ID, codeInvalidRequest, "unsupported jsonrpc version"))
		return
	}

	resp, ok := s.dispatch(req)
	if !ok {
		// Notification: no response per JSON-RPC 2.0.
		return
	}
	s.write(out, resp)
}

// dispatch routes one request to its handler. The bool result reports
// whether a response should be written (false for notifications).
func (s *Server) dispatch(req request) (response, bool) {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req), true
	case "notifications/initialized":
		s.mu.Lock()
		s.initialized = true
		s.mu.Unlock()
		return response{}, false
	case "ping":
		return newResponse(req.ID, map[string]any{}), true
	case "tools/list":
		return s.handleToolsList(req), true
	case "tools/call":
		return s.handleToolsCall(req), true
	default:
		if req.isNotification() {
			return response{}, false
		}
		return newErrorResponse(req.ID, codeMethodNotFound, "method not found: "+req.Method), true
	}
}

func (s *Server) handleInitialize(req request) response {
	var params initializeParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return newErrorResponse(req.ID, codeInvalidParams, "invalid initialize params: "+err.Error())
		}
	}
	result := initializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities:    map[string]any{"tools": map[string]any{}},
		ServerInfo:      serverInfo{Name: s.Name, Version: s.Version},
	}
	return newResponse(req.ID, result)
}

func (s *Server) handleToolsList(req request) response {
	descs := make([]toolDescriptor, 0, len(s.tools))
	for _, t := range s.tools {
		descs = append(descs, t.descriptor())
	}
	return newResponse(req.ID, toolsListResult{Tools: descs})
}

func (s *Server) handleToolsCall(req request) response {
	var params toolsCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return newErrorResponse(req.ID, codeInvalidParams, "invalid tools/call params: "+err.Error())
	}
	tool, ok := s.byName[params.Name]
	if !ok {
		return newErrorResponse(req.ID, codeInvalidParams, "unknown tool: "+params.Name)
	}

	result, err := tool.Handler(params.Arguments)
	if err != nil {
		return newResponse(req.ID, toolsCallResult{
			Content: []contentBlock{{Type: "text", Text: err.Error()}},
			IsError: true,
		})
	}

	text, merr := json.MarshalIndent(result, "", "  ")
	if merr != nil {
		return newResponse(req.ID, toolsCallResult{
			Content: []contentBlock{{Type: "text", Text: "failed to marshal tool result: " + merr.Error()}},
			IsError: true,
		})
	}
	return newResponse(req.ID, toolsCallResult{Content: []contentBlock{{Type: "text", Text: string(text)}}})
}

func (s *Server) write(out io.Writer, resp response) {
	data, err := json.Marshal(resp)
	if err != nil {
		// Marshaling our own response type failing is a programmer error;
		// there is nothing sane to write back, so drop it rather than
		// panic a long-lived server process.
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = out.Write(data)
	_, _ = out.Write([]byte("\n"))
}
