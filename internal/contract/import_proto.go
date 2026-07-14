package contract

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// ---------------------------------------------------------------------------
// Protocol Buffers (proto3) importer
// ---------------------------------------------------------------------------
//
// Maps a subset of proto3 onto the native Contract model:
//
//	package                    → Name (falls back to the file's base name)
//	message                    → Types (field → native scalar/reference type)
//	service { rpc ... }        → Endpoints (rpc name → ID)
//	  request message fields  → Input (flattened)
//	  response message fields → Output (flattened)
//
// This is a hand-rolled, line-oriented parser for the constructs acthur
// needs — proto3 messages, scalar/message/repeated fields, and service rpc
// declarations. It does not implement the full protobuf grammar: options,
// oneof, map<>, nested messages, imports, and enums declared inside a
// message are not supported and produce a pointed error rather than
// silently dropping semantics. A full protoc-gen based pipeline is a
// heavyweight dependency for this subset; this parser covers the contract
// shapes acthur's importer needs to round-trip.
//
// gRPC has no native notion of an HTTP method or path, so imported
// endpoints leave Method blank and set Path to the conventional gRPC wire
// path "/<package>.<Service>/<RPCName>" — this is informational only; the
// native Validate() function does not require Method/Path outside
// TransportHTTP.

var (
	protoPackageRe = regexp.MustCompile(`^package\s+([A-Za-z_][A-Za-z0-9_.]*)\s*;`)
	protoMessageRe = regexp.MustCompile(`^message\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`)
	protoServiceRe = regexp.MustCompile(`^service\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`)
	protoRPCRe     = regexp.MustCompile(`^rpc\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(\s*(stream\s+)?([A-Za-z_][A-Za-z0-9_.]*)\s*\)\s*returns\s*\(\s*(stream\s+)?([A-Za-z_][A-Za-z0-9_.]*)\s*\)\s*;?`)
	protoFieldRe   = regexp.MustCompile(`^(repeated\s+)?([A-Za-z_][A-Za-z0-9_.]*)\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(\d+)\s*(\[[^\]]*\])?\s*;`)
)

// protoRPCDecl is one parsed "rpc Name(Request) returns (Response);" line.
type protoRPCDecl struct {
	id, request, response string
}

func importProto(path string, data []byte) (*Contract, error) {
	lines := splitProtoLines(string(data))

	c := &Contract{
		Version:   "1",
		Transport: TransportGRPC,
		Types:     map[string]TypeDef{},
	}

	var rpcs []protoRPCDecl
	var serviceName string

	i := 0
	for i < len(lines) {
		line := lines[i]

		switch {
		case protoPackageRe.MatchString(line):
			m := protoPackageRe.FindStringSubmatch(line)
			c.Name = m[1]
			i++

		case protoMessageRe.MatchString(line):
			m := protoMessageRe.FindStringSubmatch(line)
			msgName := m[1]
			fields, next, err := parseProtoMessageBody(lines, i+1)
			if err != nil {
				return nil, fmt.Errorf("proto contract %q: message %q: %w", path, msgName, err)
			}
			c.Types[msgName] = TypeDef{Fields: fields}
			i = next

		case protoServiceRe.MatchString(line):
			m := protoServiceRe.FindStringSubmatch(line)
			serviceName = m[1]
			next, err := parseProtoServiceBody(lines, i+1, &rpcs)
			if err != nil {
				return nil, fmt.Errorf("proto contract %q: service %q: %w", path, serviceName, err)
			}
			i = next

		default:
			i++
		}
	}

	if c.Name == "" {
		base := filepath.Base(path)
		c.Name = strings.TrimSuffix(base, filepath.Ext(base))
	}

	for _, r := range rpcs {
		ep := Endpoint{
			ID:   r.id,
			Auth: "none",
		}
		if serviceName != "" {
			ep.Path = fmt.Sprintf("/%s.%s/%s", c.Name, serviceName, r.id)
		}

		if r.request != "" {
			td, ok := c.Types[r.request]
			if !ok {
				return nil, fmt.Errorf("proto contract %q: rpc %q: request message %q not found", path, r.id, r.request)
			}
			ep.Input = copyFields(td.Fields)
		}
		if r.response != "" {
			td, ok := c.Types[r.response]
			if !ok {
				return nil, fmt.Errorf("proto contract %q: rpc %q: response message %q not found", path, r.id, r.response)
			}
			ep.Output = copyFields(td.Fields)
		}
		c.Endpoints = append(c.Endpoints, ep)
	}

	c.FilePath = path
	c.Checksum = checksum(data)
	return c, nil
}

// splitProtoLines strips comments and blank lines and trims whitespace, so
// the block parsers below can match against clean, single-statement lines.
func splitProtoLines(src string) []string {
	raw := strings.Split(src, "\n")
	var out []string
	for _, l := range raw {
		if idx := strings.Index(l, "//"); idx >= 0 {
			l = l[:idx]
		}
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		out = append(out, l)
	}
	return out
}

// parseProtoMessageBody parses field declarations until the matching closing
// brace, starting at lines[start] (the line after "message X {").
func parseProtoMessageBody(lines []string, start int) (map[string]string, int, error) {
	fields := map[string]string{}
	i := start
	for i < len(lines) {
		line := lines[i]
		if line == "}" {
			return fields, i + 1, nil
		}
		m := protoFieldRe.FindStringSubmatch(line)
		if m == nil {
			return nil, 0, fmt.Errorf("unsupported message body line: %q (nested messages, oneof, map<>, and options are not supported)", line)
		}
		repeated := m[1] != ""
		protoType := m[2]
		fieldName := m[3]
		fields[fieldName] = protoFieldType(protoType, repeated)
		i++
	}
	return nil, 0, fmt.Errorf("unterminated message body")
}

// parseProtoServiceBody parses rpc declarations until the matching closing
// brace, starting at lines[start] (the line after "service X {").
func parseProtoServiceBody(lines []string, start int, out *[]protoRPCDecl) (int, error) {
	i := start
	for i < len(lines) {
		line := lines[i]
		if line == "}" {
			return i + 1, nil
		}
		m := protoRPCRe.FindStringSubmatch(line)
		if m == nil {
			return 0, fmt.Errorf("unsupported service body line: %q", line)
		}
		*out = append(*out, protoRPCDecl{
			id:       m[1],
			request:  stripProtoPackage(m[3]),
			response: stripProtoPackage(m[5]),
		})
		i++
	}
	return 0, fmt.Errorf("unterminated service body")
}

// stripProtoPackage drops a leading package qualifier (e.g. "users.User" →
// "User") since Types are keyed by bare message name within one file.
func stripProtoPackage(t string) string {
	if idx := strings.LastIndex(t, "."); idx >= 0 {
		return t[idx+1:]
	}
	return t
}

// protoFieldType maps one proto3 scalar or message field type to the native
// type string. Message-typed fields become "$TypeName" references,
// resolved (or left as a forward reference) the same way the OpenAPI
// importer does. Proto3 has no field-level "required" — all singular
// fields are optional-by-presence at the wire level — so scalar fields are
// rendered bare (no "(required)") and message references are rendered
// without the openapi-style "?" suffix, matching proto3 semantics honestly.
func protoFieldType(protoType string, repeated bool) string {
	var base string
	switch protoType {
	case "string":
		base = "string"
	case "bool":
		base = "bool"
	case "float", "double":
		base = "float"
	case "int32", "int64", "uint32", "uint64", "sint32", "sint64", "fixed32", "fixed64", "sfixed32", "sfixed64":
		base = "int"
	case "bytes":
		base = "string"
	default:
		// Message or enum type reference within this file.
		base = "$" + protoType
	}
	if repeated {
		return "array(" + base + ")"
	}
	return base
}

func copyFields(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
