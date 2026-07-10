// Package gofiber turns a parsed contract into framework-native go:fiber
// code: DTOs with constraint-derived validation, handlers, a service
// interface + skeleton, a pgx repository skeleton, route mounting, a
// contract-derived migration, and a test beside every source file.
//
// The pipeline builds code with Go string builders over a precomputed model
// (this file) rather than text/template: the per-field/per-endpoint logic
// (type mapping, constraint parsing, param routing) is conditional enough
// that templates would just relocate the complexity — recorded as a
// deviation in the Phase 7 implementation note.
package gofiber

import (
	"fmt"
	"sort"
	"strings"

	"github.com/acthur/acthur/internal/contract"
)

// fieldSpec is one contract field mapped to Go.
type fieldSpec struct {
	Name     string // contract name (snake_case)
	GoName   string // exported Go name
	GoType   string // mapped Go type, pointer if optional
	BaseType string // contract base type (string, int, email, enum, ulid, timestamp, bool, array, ref)
	Optional bool
	Required bool     // constraint: required
	MaxLen   int      // constraint: max:N (0 = none)
	Enum     []string // enum(...) values
	Ref      string   // $Type reference name ("" if none)
	Elem     string   // array element Go type ("" if not array)
}

// endpointSpec is one contract endpoint mapped to codegen decisions.
type endpointSpec struct {
	ID           string // create_user
	GoName       string // CreateUser
	Method       string // POST
	FiberMethod  string // Post
	Path         string // /api/v1/users/:id
	PathParams   []string
	AuthRequired bool
	Input        []fieldSpec // nil when endpoint has no input
	Output       []fieldSpec // nil when endpoint has no output
	Errors       []contract.ErrorDef
	FromBody     bool // input arrives in the body (POST/PUT/PATCH) vs query
	SuccessCode  int
}

// model is everything the emitters need, precomputed once.
type model struct {
	Contract     string // users
	Package      string // users
	ContractFile string // users.contract.yml (for headers)
	ModulePath   string // github.com/acthur/vetangle/api
	Types        []typeSpec
	Endpoints    []endpointSpec
	Errors       []contract.ErrorDef // deduped by code, sorted
	NeedsTime    bool
}

type typeSpec struct {
	Name   string
	Fields []fieldSpec
}

// buildModel maps the contract onto the codegen model.
func buildModel(c *contract.Contract, modulePath string) (*model, error) {
	m := &model{
		Contract:     c.Name,
		Package:      strings.ToLower(c.Name),
		ContractFile: c.Name + ".contract.yml",
		ModulePath:   modulePath,
	}

	for _, name := range sortedKeys(c.Types) {
		fields, err := buildFields(c.Types[name].Fields)
		if err != nil {
			return nil, fmt.Errorf("type %s: %w", name, err)
		}
		m.Types = append(m.Types, typeSpec{Name: name, Fields: fields})
	}

	seenErr := map[string]bool{}
	for _, e := range c.Endpoints {
		in, err := buildFields(e.Input)
		if err != nil {
			return nil, fmt.Errorf("endpoint %s input: %w", e.ID, err)
		}
		out, err := buildFields(e.Output)
		if err != nil {
			return nil, fmt.Errorf("endpoint %s output: %w", e.ID, err)
		}
		method := strings.ToUpper(e.Method)
		ep := endpointSpec{
			ID:           e.ID,
			GoName:       goName(e.ID),
			Method:       method,
			FiberMethod:  fiberMethodName(method),
			Path:         e.Path,
			PathParams:   pathParams(e.Path),
			AuthRequired: e.Auth == "required",
			Input:        in,
			Output:       out,
			Errors:       e.Errors,
			FromBody:     method == "POST" || method == "PUT" || method == "PATCH",
			SuccessCode:  successCode(method, out),
		}
		m.Endpoints = append(m.Endpoints, ep)

		for _, ce := range e.Errors {
			if !seenErr[ce.Code] {
				seenErr[ce.Code] = true
				m.Errors = append(m.Errors, ce)
			}
		}
	}
	sort.Slice(m.Errors, func(i, j int) bool { return m.Errors[i].Code < m.Errors[j].Code })

	m.NeedsTime = anyTimestamp(m)
	return m, nil
}

// fiberMethodName renders an HTTP method as the fiber router method call
// name (e.g. "GET" -> "Get"). method is always a plain-ASCII HTTP verb, so a
// manual first-letter capitalization avoids the Unicode word-boundary
// caveats of the deprecated strings.Title.
func fiberMethodName(method string) string {
	lower := strings.ToLower(method)
	if lower == "" {
		return lower
	}
	return strings.ToUpper(lower[:1]) + lower[1:]
}

func successCode(method string, out []fieldSpec) int {
	switch {
	case method == "POST":
		return 201
	case len(out) == 0:
		return 204
	default:
		return 200
	}
}

func anyTimestamp(m *model) bool {
	for _, t := range m.Types {
		for _, f := range t.Fields {
			if f.BaseType == "timestamp" {
				return true
			}
		}
	}
	for _, e := range m.Endpoints {
		for _, f := range append(append([]fieldSpec{}, e.Input...), e.Output...) {
			if f.BaseType == "timestamp" {
				return true
			}
		}
	}
	return false
}

// buildFields maps a contract field map (name → "type(constraints)") into
// sorted fieldSpecs.
func buildFields(raw map[string]string) ([]fieldSpec, error) {
	var fields []fieldSpec
	for _, name := range sortedKeys(raw) {
		f, err := parseField(name, raw[name])
		if err != nil {
			return nil, err
		}
		fields = append(fields, f)
	}
	return fields, nil
}

// parseField parses one contract field declaration like
// "string(required,max:100)", "email(required)", "enum(a,b)?", "int?",
// "array($User)", "$User", "ulid", "timestamp".
func parseField(name, decl string) (fieldSpec, error) {
	f := fieldSpec{Name: name, GoName: goName(name)}
	d := strings.TrimSpace(decl)

	if strings.HasSuffix(d, "?") {
		f.Optional = true
		d = strings.TrimSuffix(d, "?")
	}

	base := d
	var args string
	if i := strings.IndexByte(d, '('); i >= 0 && strings.HasSuffix(d, ")") {
		base = d[:i]
		args = d[i+1 : len(d)-1]
	}

	switch {
	case strings.HasPrefix(base, "$"):
		f.BaseType = "ref"
		f.Ref = strings.TrimPrefix(base, "$")
		f.GoType = f.Ref
	case base == "array":
		f.BaseType = "array"
		elem := strings.TrimSpace(args)
		if strings.HasPrefix(elem, "$") {
			f.Elem = strings.TrimPrefix(elem, "$")
		} else {
			f.Elem = goPrimitive(elem)
		}
		f.GoType = "[]" + f.Elem
	case base == "enum":
		f.BaseType = "enum"
		for _, v := range strings.Split(args, ",") {
			f.Enum = append(f.Enum, strings.TrimSpace(v))
		}
		f.GoType = "string"
	default:
		f.BaseType = base
		f.GoType = goPrimitive(base)
		if f.GoType == "" {
			return f, fmt.Errorf("field %q: unknown contract type %q", name, decl)
		}
		for _, c := range strings.Split(args, ",") {
			c = strings.TrimSpace(c)
			switch {
			case c == "":
			case c == "required":
				f.Required = true
			case strings.HasPrefix(c, "max:"):
				_, _ = fmt.Sscanf(c, "max:%d", &f.MaxLen)
			default:
				// min: is accepted but not yet enforced in generated
				// validation; other unknown constraints are tolerated too
				// (forward compat)
			}
		}
	}

	if f.Optional && !strings.HasPrefix(f.GoType, "[]") {
		f.GoType = "*" + f.GoType
	}
	return f, nil
}

func goPrimitive(base string) string {
	switch base {
	case "string", "email", "ulid":
		return "string"
	case "int":
		return "int64"
	case "bool":
		return "bool"
	case "timestamp":
		return "time.Time"
	case "float":
		return "float64"
	default:
		return ""
	}
}

// goName converts snake_case to exported CamelCase with Go initialisms.
func goName(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		switch strings.ToLower(p) {
		case "id":
			parts[i] = "ID"
		case "url":
			parts[i] = "URL"
		case "api":
			parts[i] = "API"
		default:
			if p != "" {
				parts[i] = strings.ToUpper(p[:1]) + p[1:]
			}
		}
	}
	return strings.Join(parts, "")
}

func pathParams(path string) []string {
	var params []string
	for _, seg := range strings.Split(path, "/") {
		if strings.HasPrefix(seg, ":") {
			params = append(params, strings.TrimPrefix(seg, ":"))
		}
	}
	return params
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
