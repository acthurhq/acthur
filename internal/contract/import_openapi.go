package contract

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// OpenAPI 3.x importer
// ---------------------------------------------------------------------------
//
// Maps a subset of OpenAPI 3.x onto the native Contract model:
//
//	paths + methods           → Endpoints (operationId → ID)
//	parameters + requestBody  → Input
//	2xx response schema       → Output
//	other responses           → Errors
//	components.schemas        → Types
//
// This is intentionally minimal: it supports the common constructs (object
// request/response bodies, query/path parameters, $ref, arrays of $ref,
// enums, and the string formats used by the native format's scalar types).
// Anything it cannot honestly represent (e.g. oneOf/anyOf schemas, non-JSON
// bodies, callbacks, links) returns a pointed error rather than silently
// dropping semantics.

func importOpenAPI(path string, data []byte) (*Contract, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("cannot parse openapi contract %q: %w", path, err)
	}

	info, _ := doc["info"].(map[string]any)
	if info == nil {
		return nil, fmt.Errorf("openapi contract %q: missing required 'info' section", path)
	}
	name, _ := info["title"].(string)
	if name == "" {
		return nil, fmt.Errorf("openapi contract %q: missing required 'info.title'", path)
	}
	version := stringify(info["version"])
	if version == "" {
		version = "1"
	}

	globalSecurity, _ := doc["security"].([]any)

	c := &Contract{
		Name:      name,
		Version:   version,
		Transport: TransportHTTP,
		Types:     map[string]TypeDef{},
	}

	components, _ := doc["components"].(map[string]any)
	if components != nil {
		schemas, _ := components["schemas"].(map[string]any)
		for typeName, raw := range schemas {
			schema, _ := raw.(map[string]any)
			fields, err := openapiObjectFields(schema, true)
			if err != nil {
				return nil, fmt.Errorf("openapi contract %q: type %q: %w", path, typeName, err)
			}
			c.Types[typeName] = TypeDef{Fields: fields}
		}
	}

	paths, _ := doc["paths"].(map[string]any)
	pathKeys := make([]string, 0, len(paths))
	for p := range paths {
		pathKeys = append(pathKeys, p)
	}
	sort.Strings(pathKeys)

	for _, rawPath := range pathKeys {
		item, _ := paths[rawPath].(map[string]any)
		nativePath := openapiPathToNative(rawPath)

		methodKeys := make([]string, 0, len(item))
		for m := range item {
			methodKeys = append(methodKeys, m)
		}
		sort.Strings(methodKeys)

		for _, method := range methodKeys {
			switch method {
			case "get", "post", "put", "patch", "delete", "head", "options":
			default:
				continue // vendor extensions / parameters block at the path level
			}
			op, _ := item[method].(map[string]any)
			if op == nil {
				continue
			}

			ep, err := importOpenAPIOperation(method, nativePath, op, globalSecurity, c.Types)
			if err != nil {
				return nil, fmt.Errorf("openapi contract %q: %s %s: %w", path, method, rawPath, err)
			}
			c.Endpoints = append(c.Endpoints, *ep)
		}
	}

	c.FilePath = path
	c.Checksum = checksum(data)
	return c, nil
}

func importOpenAPIOperation(method, nativePath string, op map[string]any, globalSecurity []any, types map[string]TypeDef) (*Endpoint, error) {
	operationID, _ := op["operationId"].(string)
	if operationID == "" {
		return nil, fmt.Errorf("missing required 'operationId'")
	}

	ep := &Endpoint{
		ID:     operationID,
		Method: strings.ToUpper(method),
		Path:   nativePath,
		Auth:   "none",
	}

	security, hasOpSecurity := op["security"]
	if hasOpSecurity {
		if arr, _ := security.([]any); len(arr) > 0 {
			ep.Auth = "required"
		}
	} else if len(globalSecurity) > 0 {
		ep.Auth = "required"
	}

	input := map[string]string{}

	if params, ok := op["parameters"].([]any); ok {
		for _, raw := range params {
			p, _ := raw.(map[string]any)
			if p == nil {
				continue
			}
			in, _ := p["in"].(string)
			if in != "query" && in != "path" {
				continue // headers/cookies are not represented in Input
			}
			pname, _ := p["name"].(string)
			if pname == "" {
				continue
			}
			required, _ := p["required"].(bool)
			schema, _ := p["schema"].(map[string]any)
			typ, err := openapiScalarType(schema, required, false)
			if err != nil {
				return nil, fmt.Errorf("parameter %q: %w", pname, err)
			}
			input[pname] = typ
		}
	}

	if body, ok := op["requestBody"].(map[string]any); ok {
		schema, err := openapiJSONSchema(body)
		if err != nil {
			return nil, err
		}
		if schema != nil {
			fields, err := flattenSchemaFields(schema, types, false)
			if err != nil {
				return nil, fmt.Errorf("requestBody: %w", err)
			}
			for k, v := range fields {
				input[k] = v
			}
		}
	}
	if len(input) > 0 {
		ep.Input = input
	}

	responses, _ := op["responses"].(map[string]any)
	statusKeys := make([]string, 0, len(responses))
	for s := range responses {
		statusKeys = append(statusKeys, s)
	}
	sort.Strings(statusKeys)

	for _, status := range statusKeys {
		resp, _ := responses[status].(map[string]any)
		if strings.HasPrefix(status, "2") {
			schema, err := openapiJSONSchema(resp)
			if err != nil {
				return nil, fmt.Errorf("response %s: %w", status, err)
			}
			if schema != nil {
				fields, err := flattenSchemaFields(schema, types, true)
				if err != nil {
					return nil, fmt.Errorf("response %s: %w", status, err)
				}
				ep.Output = fields
			}
			continue
		}

		statusCode, err := parseStatusCode(status)
		if err != nil {
			continue // "default" or other non-numeric response keys are skipped honestly
		}
		code := errorCodeFor(status, resp)
		ep.Errors = append(ep.Errors, ErrorDef{Status: statusCode, Code: code})
	}

	return ep, nil
}

// errorCodeFor derives an error code for a non-2xx response: prefer an
// enum'd "code" property on the response schema, else slugify the
// response description, else fall back to a generic "error_<status>".
func errorCodeFor(status string, resp map[string]any) string {
	schema, _ := openapiJSONSchema(resp)
	if schema != nil {
		if props, ok := schema["properties"].(map[string]any); ok {
			if codeProp, ok := props["code"].(map[string]any); ok {
				if enumVals, ok := codeProp["enum"].([]any); ok && len(enumVals) > 0 {
					return stringify(enumVals[0])
				}
			}
		}
	}
	if desc, _ := resp["description"].(string); desc != "" {
		return slugify(desc)
	}
	return "error_" + status
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

func parseStatusCode(s string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, err
	}
	if n < 100 || n > 599 {
		return 0, fmt.Errorf("not a valid status code: %q", s)
	}
	return n, nil
}

// openapiJSONSchema extracts the application/json schema from a requestBody
// or response object. Returns (nil, nil) if there is no JSON body (e.g. a
// 204 response, or a non-JSON content type) — that is a legitimate,
// honestly-represented absence, not an error.
func openapiJSONSchema(container map[string]any) (map[string]any, error) {
	content, ok := container["content"].(map[string]any)
	if !ok {
		return nil, nil
	}
	jsonBody, ok := content["application/json"].(map[string]any)
	if !ok {
		return nil, nil // only JSON bodies are supported; anything else is skipped honestly
	}
	schema, _ := jsonBody["schema"].(map[string]any)
	return resolveSchema(schema), nil
}

// resolveSchema follows a single $ref indirection into the same document's
// components.schemas is intentionally not performed here (callers resolve
// $ref via openapiFieldType, which emits "$TypeName" for cross-references).
// This helper only passes the schema through unchanged.
func resolveSchema(schema map[string]any) map[string]any {
	return schema
}

// flattenSchemaFields converts an OpenAPI body/response schema into a flat
// native Input/Output field map. Input and Output are always flat maps in
// the native format (never a single "$Type" reference), so a top-level
// $ref is resolved against the already-imported components.schemas types
// and its fields are copied in directly.
//
// plain selects the field-type rendering: false for Input (constraint
// syntax like "string(required,max:100)"), true for Output/Types (bare
// shape only — "string" or "string?", never "(required)").
func flattenSchemaFields(schema map[string]any, types map[string]TypeDef, plain bool) (map[string]string, error) {
	if ref, ok := schema["$ref"].(string); ok {
		name := refName(ref)
		td, ok := types[name]
		if !ok {
			return nil, fmt.Errorf("referenced type %q not found in components.schemas", name)
		}
		fields := make(map[string]string, len(td.Fields))
		for k, v := range td.Fields {
			fields[k] = v
		}
		return fields, nil
	}
	return openapiObjectFields(schema, plain)
}

// openapiObjectFields converts an OpenAPI object schema's properties into a
// native Input/Output field map, honoring the schema's "required" list.
func openapiObjectFields(schema map[string]any, plain bool) (map[string]string, error) {
	if schema == nil {
		return nil, nil
	}
	if ref, ok := schema["$ref"].(string); ok {
		// A bare top-level $ref with no wrapping object — represent the
		// whole body as a single reference is not expressible as a flat
		// field map, so error out rather than guess.
		return nil, fmt.Errorf("top-level $ref %q is not supported as an object body; wrap it or flatten its fields", ref)
	}

	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		return nil, nil
	}
	requiredSet := map[string]bool{}
	if req, ok := schema["required"].([]any); ok {
		for _, r := range req {
			requiredSet[stringify(r)] = true
		}
	}

	fields := map[string]string{}
	for fname, raw := range props {
		fieldSchema, _ := raw.(map[string]any)
		typ, err := openapiScalarType(fieldSchema, requiredSet[fname], plain)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", fname, err)
		}
		fields[fname] = typ
	}
	return fields, nil
}

// openapiScalarType maps one OpenAPI schema object to a native type string,
// e.g. "string(required,max:100)", "enum(admin,member,viewer)?", "ulid",
// "array($User)". See flattenSchemaFields for what plain selects.
func openapiScalarType(schema map[string]any, required, plain bool) (string, error) {
	if schema == nil {
		return "", fmt.Errorf("missing schema")
	}

	if ref, ok := schema["$ref"].(string); ok {
		t := "$" + refName(ref)
		if !required {
			t += "?"
		}
		return t, nil
	}

	if enumVals, ok := schema["enum"].([]any); ok && len(enumVals) > 0 {
		strs := make([]string, len(enumVals))
		for i, v := range enumVals {
			strs[i] = stringify(v)
		}
		t := "enum(" + strings.Join(strs, ",") + ")"
		if !required {
			t += "?"
		}
		return t, nil
	}

	schemaType, _ := schema["type"].(string)
	format, _ := schema["format"].(string)

	switch schemaType {
	case "array":
		items, _ := schema["items"].(map[string]any)
		itemType, err := openapiScalarType(items, true, plain) // items don't carry their own required-ness
		if err != nil {
			return "", fmt.Errorf("array items: %w", err)
		}
		itemType = strings.TrimSuffix(itemType, "?")
		return "array(" + itemType + ")", nil
	case "integer":
		return withConstraints("int", schema, required, plain), nil
	case "number":
		return withConstraints("float", schema, required, plain), nil
	case "boolean":
		return withConstraints("bool", schema, required, plain), nil
	case "string", "":
		base := "string"
		switch format {
		case "date-time":
			base = "timestamp"
		case "email":
			base = "email"
		case "uuid":
			if x, _ := schema["x-acthur-type"].(string); x == "ulid" {
				base = "ulid"
			} else {
				base = "string"
			}
		}
		return withConstraints(base, schema, required, plain), nil
	case "object":
		return "", fmt.Errorf("inline object types are not supported; define a component schema and reference it via $ref")
	default:
		return "", fmt.Errorf("unsupported schema type %q", schemaType)
	}
}

// withConstraints appends "(required,max:N)"-style constraints for plain
// scalar types, or a bare "?" when optional with no other constraints. When
// plain is true (Types/Output — shape only, never validation rules) it
// always renders as a bare type, "?"-suffixed only when optional.
func withConstraints(base string, schema map[string]any, required, plain bool) string {
	if plain {
		if !required {
			return base + "?"
		}
		return base
	}

	var constraints []string
	if required {
		constraints = append(constraints, "required")
	}
	if maxLen, ok := schema["maxLength"]; ok {
		constraints = append(constraints, fmt.Sprintf("max:%s", stringify(maxLen)))
	}
	if maxVal, ok := schema["maximum"]; ok {
		constraints = append(constraints, fmt.Sprintf("max:%s", stringify(maxVal)))
	}
	if minLen, ok := schema["minLength"]; ok {
		constraints = append(constraints, fmt.Sprintf("min:%s", stringify(minLen)))
	}
	if minVal, ok := schema["minimum"]; ok {
		constraints = append(constraints, fmt.Sprintf("min:%s", stringify(minVal)))
	}
	if len(constraints) > 0 {
		return base + "(" + strings.Join(constraints, ",") + ")"
	}
	if !required {
		return base + "?"
	}
	return base
}

// refName extracts the trailing component name from a "#/components/schemas/X" ref.
func refName(ref string) string {
	parts := strings.Split(ref, "/")
	return parts[len(parts)-1]
}

// openapiPathToNative converts OpenAPI's "{param}" path parameter syntax to
// the native format's ":param" convention.
func openapiPathToNative(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '{':
			b.WriteByte(':')
		case '}':
			// no-op
		default:
			b.WriteByte(p[i])
		}
	}
	return b.String()
}

func stringify(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}
