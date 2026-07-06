package aiagent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/contract"
	"github.com/acthur/acthur/internal/graph"
)

// BuildContext renders the live graph and contract registry into a compact
// text block suitable as an LLM system prompt — the "full graph + contract
// context" every `acthur agent` command promises per PRD §17.6. It never
// includes secrets (no env values, no api_key).
func BuildContext(cfg *config.Config, g *graph.Graph, reg *contract.Registry) string {
	var b strings.Builder

	fmt.Fprintf(&b, "You are Acthur's project assistant for %q.\n", cfg.Project)
	b.WriteString("You have full context on this project's live graph and contracts below. ")
	b.WriteString("Answer using this context; be concise and concrete.\n\n")

	b.WriteString("## Graph nodes\n")
	nodes := append([]*graph.Node{}, g.Nodes()...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	for _, n := range nodes {
		fmt.Fprintf(&b, "- %s (%s, adapter=%s", n.ID, n.Type, n.Adapter)
		if n.Port != 0 {
			fmt.Fprintf(&b, ", port=%d", n.Port)
		}
		b.WriteString(")\n")
	}

	b.WriteString("\n## Graph edges\n")
	for _, e := range g.Edges() {
		fmt.Fprintf(&b, "- %s -> %s (%s", e.From, e.To, e.Type)
		if len(e.Contracts) > 0 {
			fmt.Fprintf(&b, ", contracts=%s", strings.Join(e.Contracts, ","))
		}
		b.WriteString(")\n")
	}

	contracts := reg.All()
	if len(contracts) > 0 {
		sort.Slice(contracts, func(i, j int) bool { return contracts[i].Name < contracts[j].Name })
		b.WriteString("\n## Contracts\n")
		for _, c := range contracts {
			fmt.Fprintf(&b, "- %s@%s (%s transport, %d endpoints, %d events)\n",
				c.Name, c.Version, c.Transport, len(c.Endpoints), len(c.Events))
			for _, ep := range c.Endpoints {
				fmt.Fprintf(&b, "  - %s %s %s\n", ep.ID, ep.Method, ep.Path)
			}
		}
	}

	if len(cfg.Plugins) > 0 {
		b.WriteString("\n## Plugins\n")
		for _, p := range cfg.Plugins {
			fmt.Fprintf(&b, "- %s\n", p.Name)
		}
	}

	return b.String()
}
