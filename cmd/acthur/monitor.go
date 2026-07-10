package main

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/acthurhq/acthur/internal/graph"
	"github.com/acthurhq/acthur/internal/process"
)

// ---------------------------------------------------------------------------
// acthur monitor
//
// `acthur monitor` is a separate CLI invocation from any running `acthur
// dev` — there is no shared memory between them. Like `acthur service
// logs/health/restart` (internal/process/control.go), it reads whatever
// durable, on-disk fact the dev engine already persists under .acthur/
// rather than inventing an RPC daemon: .acthur/run/<node>.pid for the
// recorded PID, and a direct health poll (the same health.Checker.Poll
// `acthur service health` uses) for live status — never a cached or
// simulated value.
// ---------------------------------------------------------------------------

// monitorRow is one line of the status table.
type monitorRow struct {
	Node    string
	Type    string
	Adapter string
	Health  string
	PID     string
}

// buildMonitorRows polls every non-kernel node in g once via checker and
// reads its recorded pidfile (if any) under rootDir, returning rows sorted
// by node ID for deterministic output.
func buildMonitorRows(g *graph.Graph, checker healthPoller, rootDir string) []monitorRow {
	nodes := g.Nodes()
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })

	rows := make([]monitorRow, 0, len(nodes))
	for _, node := range nodes {
		// Kernel-materialized nodes (e.g. the always-present proxy) aren't
		// user-managed services — they clutter the table without adding
		// signal, same exclusion `acthur service health` already applies.
		if strings.HasPrefix(node.Adapter, "kernel:") {
			continue
		}

		row := monitorRow{Node: node.ID, Type: string(node.Type), Adapter: node.Adapter, PID: "-"}
		if pid, err := process.ReadPIDFile(rootDir, node.ID); err == nil {
			row.PID = strconv.Itoa(pid)
		}
		if err := checker.Poll(node); err != nil {
			row.Health = "unhealthy"
		} else {
			row.Health = "healthy"
		}
		rows = append(rows, row)
	}
	return rows
}

// writeMonitorTable renders rows as an aligned table to out.
func writeMonitorTable(out io.Writer, rows []monitorRow) {
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NODE\tTYPE\tADAPTER\tHEALTH\tPID")
	for _, r := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.Node, r.Type, r.Adapter, r.Health, r.PID)
	}
	tw.Flush() //nolint:errcheck // best-effort terminal output
}

// runMonitor renders the status table once to out, or — when watch is true —
// repeatedly every interval until stop is closed, clearing the screen
// between refreshes. Mirrors runServiceLogs's poll-until-signalled shape.
func runMonitor(g *graph.Graph, checker healthPoller, rootDir string, out io.Writer, watch bool, interval time.Duration, stop <-chan struct{}) {
	writeMonitorTable(out, buildMonitorRows(g, checker, rootDir))
	if !watch {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			_, _ = fmt.Fprint(out, "\033[H\033[2J")  // clear screen for a refreshed view
			writeMonitorTable(out, buildMonitorRows(g, checker, rootDir))
		}
	}
}
