package contract

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// ---------------------------------------------------------------------------
// GraphQL SDL importer
// ---------------------------------------------------------------------------
//
// Maps a subset of GraphQL SDL onto the native Contract model:
//
//	type X { ... }            → Types (object types)
//	enum X { ... }             → recorded so referencing fields render as
//	                             "enum(A,B,C)"; enums have no standalone
//	                             native representation, so they are not
//	                             themselves added to Types.
//	type Query { ... }        → Endpoints, Endpoint.Method = "QUERY"
//	type Mutation { ... }      → Endpoints, Endpoint.Method = "MUTATION"
//	type Subscription { ... } → Endpoints, Endpoint.Method = "SUBSCRIPTION"
//	field arguments            → Input (flattened)
//	field return type           → Output:
//	  - object return type      → the object's own fields, flattened in
//	                              directly (matches how OpenAPI $ref
//	                              responses flatten)
//	  - list-of-object return   → {"items": "array($Type)"} — GraphQL lists
//	                              have no field name of their own, so this
//	                              is a deliberate, documented convention
//	                              rather than a literal SDL construct
//	  - scalar return            → {"value": "<type>"}
//
// This is a hand-rolled, line-oriented parser for the constructs acthur's
// generator needs. It does not implement the full GraphQL grammar:
// directives, interfaces, unions, input types with defaults, and
// descriptions/docstrings are not supported and produce a pointed error.
//
// Contract name/version are not part of GraphQL SDL, so Name is derived
// from the file's base name and Version defaults to "1" — both are lossy
// relative to a hand-authored .contract.yml and are documented here rather
// than silently invented elsewhere.

var (
	gqlTypeRe  = regexp.MustCompile(`^type\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`)
	gqlEnumRe  = regexp.MustCompile(`^enum\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`)
	gqlFieldRe = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\s*(\(([^)]*)\))?\s*:\s*(\[)?\s*([A-Za-z_][A-Za-z0-9_]*)\s*(!)?\s*(\])?\s*(!)?`)
	gqlArgRe   = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*:\s*(\[)?\s*([A-Za-z_][A-Za-z0-9_]*)\s*(!)?\s*(\])?\s*(!)?`)
)

// splitGQLLines strips "#"-comments and blank lines and trims whitespace, so
// the block parsers below can match against clean, single-statement lines.
func splitGQLLines(src string) []string {
	raw := strings.Split(src, "\n")
	var out []string
	for _, l := range raw {
		if idx := strings.Index(l, "#"); idx >= 0 {
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

func importGraphQL(path string, data []byte) (*Contract, error) {
	lines := splitGQLLines(string(data))

	c := &Contract{
		Version:   "1",
		Transport: TransportGraphQL,
		Types:     map[string]TypeDef{},
	}

	enums := map[string]bool{}
	objectTypes := map[string]map[string]gqlField{} // typeName -> fieldName -> parsed field
	var queryFields, mutationFields, subscriptionFields map[string]gqlField

	i := 0
	for i < len(lines) {
		line := lines[i]
		switch {
		case gqlEnumRe.MatchString(line):
			m := gqlEnumRe.FindStringSubmatch(line)
			enums[m[1]] = true
			next, err := skipGQLBlock(lines, i+1)
			if err != nil {
				return nil, fmt.Errorf("graphql contract %q: enum %q: %w", path, m[1], err)
			}
			i = next

		case gqlTypeRe.MatchString(line):
			m := gqlTypeRe.FindStringSubmatch(line)
			typeName := m[1]
			fields, next, err := parseGQLTypeBody(lines, i+1)
			if err != nil {
				return nil, fmt.Errorf("graphql contract %q: type %q: %w", path, typeName, err)
			}
			switch typeName {
			case "Query":
				queryFields = fields
			case "Mutation":
				mutationFields = fields
			case "Subscription":
				subscriptionFields = fields
			default:
				objectTypes[typeName] = fields
			}
			i = next

		default:
			i++
		}
	}

	// Resolve object types into native Types now that all types/enums are known.
	for typeName, fields := range objectTypes {
		nativeFields := make(map[string]string, len(fields))
		for fname, f := range fields {
			t, err := f.nativeType(enums)
			if err != nil {
				return nil, fmt.Errorf("graphql contract %q: type %q: field %q: %w", path, typeName, fname, err)
			}
			nativeFields[fname] = t
		}
		c.Types[typeName] = TypeDef{Fields: nativeFields}
	}

	c.Endpoints = append(c.Endpoints, buildGQLEndpoints(queryFields, "QUERY", objectTypes, enums)...)
	c.Endpoints = append(c.Endpoints, buildGQLEndpoints(mutationFields, "MUTATION", objectTypes, enums)...)
	c.Endpoints = append(c.Endpoints, buildGQLEndpoints(subscriptionFields, "SUBSCRIPTION", objectTypes, enums)...)

	if c.Name == "" {
		base := filepath.Base(path)
		c.Name = strings.TrimSuffix(base, filepath.Ext(base))
	}

	c.FilePath = path
	c.Checksum = checksum(data)
	return c, nil
}

// gqlField is a parsed field/operation signature: name, optional arguments,
// and a return/value type (list-ness and null-ability included).
type gqlField struct {
	args     map[string]gqlField // for Query/Mutation/Subscription root fields only
	list     bool
	nonNull  bool
	typeName string
}

// nativeType renders a gqlField's type as a native type string. required
// selects constraint-syntax rendering (used for Input/arguments); object and
// list handling matches the OpenAPI importer's conventions.
func (f gqlField) nativeType(enums map[string]bool) (string, error) {
	return f.render(enums, false)
}

func (f gqlField) render(enums map[string]bool, asInput bool) (string, error) {
	base := gqlScalarType(f.typeName, enums)
	if f.list {
		base = "array(" + base + ")"
	}
	if asInput {
		if f.nonNull {
			return base + "(required)", nil
		}
		return base + "?", nil
	}
	if !f.nonNull {
		return base + "?", nil
	}
	return base, nil
}

func gqlScalarType(t string, enums map[string]bool) string {
	switch t {
	case "ID", "String":
		return "string"
	case "Int":
		return "int"
	case "Float":
		return "float"
	case "Boolean":
		return "bool"
	}
	if enums[t] {
		return "enum(" + t + ")" // enum value set is not re-declared inline here; documented lossy mapping
	}
	return "$" + t
}

func parseGQLTypeBody(lines []string, start int) (map[string]gqlField, int, error) {
	fields := map[string]gqlField{}
	i := start
	for i < len(lines) {
		line := lines[i]
		if line == "}" {
			return fields, i + 1, nil
		}
		f, name, err := parseGQLFieldLine(line)
		if err != nil {
			return nil, 0, err
		}
		fields[name] = f
		i++
	}
	return nil, 0, fmt.Errorf("unterminated type body")
}

func skipGQLBlock(lines []string, start int) (int, error) {
	i := start
	for i < len(lines) {
		if lines[i] == "}" {
			return i + 1, nil
		}
		i++
	}
	return 0, fmt.Errorf("unterminated block")
}

// parseGQLFieldLine parses one SDL field/operation line, e.g.:
//
//	name: String!
//	getUser(id: ID!): User
//	listUsers: [User!]!
func parseGQLFieldLine(line string) (gqlField, string, error) {
	m := gqlFieldRe.FindStringSubmatch(line)
	if m == nil {
		return gqlField{}, "", fmt.Errorf("unsupported field/operation line: %q (directives, interfaces, and unions are not supported)", line)
	}
	name := m[1]
	argsRaw := m[3]
	list := m[4] == "[" && m[7] == "]"
	typeName := m[5]
	var nonNull bool
	if list {
		nonNull = m[8] == "!" // outer "!" after the closing bracket
	} else {
		nonNull = m[6] == "!"
	}

	f := gqlField{list: list, nonNull: nonNull, typeName: typeName}

	if argsRaw != "" {
		f.args = map[string]gqlField{}
		matches := gqlArgRe.FindAllStringSubmatch(argsRaw, -1)
		for _, am := range matches {
			argName := am[1]
			argList := am[2] == "[" && am[5] == "]"
			argType := am[3]
			var argNonNull bool
			if argList {
				argNonNull = am[6] == "!"
			} else {
				argNonNull = am[4] == "!"
			}
			f.args[argName] = gqlField{list: argList, nonNull: argNonNull, typeName: argType}
		}
	}

	return f, name, nil
}

// buildGQLEndpoints converts one root operation type's fields (Query,
// Mutation, or Subscription) into native Endpoints.
func buildGQLEndpoints(fields map[string]gqlField, method string, objectTypes map[string]map[string]gqlField, enums map[string]bool) []Endpoint {
	if fields == nil {
		return nil
	}
	var eps []Endpoint
	for name, f := range fields {
		ep := Endpoint{ID: name, Method: method, Path: "/graphql", Auth: "none"}

		if len(f.args) > 0 {
			input := map[string]string{}
			for argName, arg := range f.args {
				t, err := arg.render(enums, true)
				if err != nil {
					continue
				}
				input[argName] = t
			}
			ep.Input = input
		}

		switch {
		case f.list:
			// A list return has no field name of its own in SDL; represent
			// it under a conventional "items" key (documented above).
			itemType := gqlScalarType(f.typeName, enums)
			ep.Output = map[string]string{"items": "array(" + itemType + ")"}
		case objectTypes[f.typeName] != nil:
			// Object return type: flatten its own fields directly into
			// Output, mirroring the OpenAPI importer's $ref-flattening.
			out := map[string]string{}
			for fname, of := range objectTypes[f.typeName] {
				t, err := of.nativeType(enums)
				if err != nil {
					continue
				}
				out[fname] = t
			}
			ep.Output = out
		default:
			ep.Output = map[string]string{"value": gqlScalarType(f.typeName, enums)}
		}

		eps = append(eps, ep)
	}
	return eps
}
