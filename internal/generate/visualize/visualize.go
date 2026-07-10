// Package visualize renders a graph.Graph as a Mermaid flowchart or a
// Graphviz DOT digraph for `acthur graph visualize`. Both renderers are
// pure string builders over the graph's own nodes/edges — no template
// engine, no external dependency — and both sort nodes and edges first so
// output is deterministic across runs with no underlying graph change.
package visualize

import (
	"fmt"
	"sort"
	"strings"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/graph"
)

// Format identifies which diagram language to render.
type Format string

const (
	FormatMermaid Format = "mermaid"
	FormatDot     Format = "dot"
)

// SupportedFormats returns the format keys Render accepts.
func SupportedFormats() []string {
	return []string{string(FormatMermaid), string(FormatDot)}
}

// Render dispatches to Mermaid or Dot based on format ("" defaults to mermaid).
func Render(g *graph.Graph, format string) (string, error) {
	f := Format(strings.ToLower(strings.TrimSpace(format)))
	if f == "" {
		f = FormatMermaid
	}
	switch f {
	case FormatMermaid:
		return Mermaid(g), nil
	case FormatDot:
		return Dot(g), nil
	default:
		return "", fmt.Errorf("unsupported visualize format %q — supported: %s",
			format, strings.Join(SupportedFormats(), ", "))
	}
}

// sortedNodes returns g.Nodes() sorted by ID.
func sortedNodes(g *graph.Graph) []*graph.Node {
	nodes := g.Nodes()
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes
}

// sortedEdges returns g.Edges() sorted by From, then To, then Type.
func sortedEdges(g *graph.Graph) []*graph.Edge {
	edges := append([]*graph.Edge(nil), g.Edges()...)
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].To != edges[j].To {
			return edges[i].To < edges[j].To
		}
		return edges[i].Type < edges[j].Type
	})
	return edges
}

// nodeLabel renders a node's ID and adapter as a single-line label. Both
// Mermaid and DOT support a literal backslash-n escape for a line break
// inside a quoted label, but %q (used to safely quote the label for both
// formats) would re-escape that backslash into a broken double-backslash —
// so the label stays single-line rather than fighting %q's escaping.
func nodeLabel(n *graph.Node) string {
	return fmt.Sprintf("%s (%s)", n.ID, n.Adapter)
}

// edgeLabel renders an edge's type plus its contract names, if any.
func edgeLabel(e *graph.Edge) string {
	label := string(e.Type)
	if len(e.Contracts) > 0 {
		label += ": " + strings.Join(e.Contracts, ",")
	}
	return label
}

// ---------------------------------------------------------------------------
// Mermaid
// ---------------------------------------------------------------------------

// Mermaid renders g as a Mermaid flowchart (`graph TD`). Node shape encodes
// kind: services are rectangles, infra nodes are cylinders (the conventional
// Mermaid shape for a database/storage), everything else is a rounded box.
func Mermaid(g *graph.Graph) string {
	var b strings.Builder
	b.WriteString("graph TD\n")

	for _, n := range sortedNodes(g) {
		id := mermaidID(n.ID)
		label := nodeLabel(n)
		switch n.Type {
		case config.NodeTypeService:
			fmt.Fprintf(&b, "    %s[%q]\n", id, label)
		case config.NodeTypeInfra:
			fmt.Fprintf(&b, "    %s[(%q)]\n", id, label)
		default:
			fmt.Fprintf(&b, "    %s(%q)\n", id, label)
		}
	}

	for _, e := range sortedEdges(g) {
		fmt.Fprintf(&b, "    %s -->|%s| %s\n", mermaidID(e.From), edgeLabel(e), mermaidID(e.To))
	}

	return b.String()
}

// mermaidID sanitizes a node ID for use as a Mermaid vertex identifier —
// Mermaid vertex IDs may not contain "-" unescaped in older parsers, so
// non-alphanumeric characters are folded to "_". Node IDs are also used as
// the visible label, so this only affects the internal identifier.
func mermaidID(id string) string {
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// DOT (Graphviz)
// ---------------------------------------------------------------------------

// Dot renders g as a Graphviz DOT digraph. Node shape encodes kind: services
// are boxes, infra nodes are cylinders, everything else is the default ellipse.
func Dot(g *graph.Graph) string {
	var b strings.Builder
	b.WriteString("digraph acthur {\n")
	b.WriteString("    rankdir=LR;\n")

	for _, n := range sortedNodes(g) {
		shape := "ellipse"
		switch n.Type {
		case config.NodeTypeService:
			shape = "box"
		case config.NodeTypeInfra:
			shape = "cylinder"
		}
		fmt.Fprintf(&b, "    %q [shape=%s, label=%q];\n", n.ID, shape, nodeLabel(n))
	}

	for _, e := range sortedEdges(g) {
		fmt.Fprintf(&b, "    %q -> %q [label=%q];\n", e.From, e.To, edgeLabel(e))
	}

	b.WriteString("}\n")
	return b.String()
}
