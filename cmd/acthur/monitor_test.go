package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/graph"
	"github.com/acthurhq/acthur/internal/process"
)

func TestBuildMonitorRows_ReportsHealthAndPID(t *testing.T) {
	root := t.TempDir()
	g := graph.NewTestGraph(map[string]*graph.Node{
		"api":   {ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber"},
		"db":    {ID: "db", Type: config.NodeTypeInfra, Adapter: "db:postgres"},
		"proxy": {ID: "proxy", Type: config.NodeTypePlugin, Adapter: "kernel:proxy"},
	})
	if err := process.WritePIDFile(root, "api", 4242); err != nil {
		t.Fatalf("write pidfile: %v", err)
	}

	poller := fakeHealthPoller{err: map[string]error{"db": errors.New("connection refused")}}
	rows := buildMonitorRows(g, poller, root)

	if len(rows) != 2 {
		t.Fatalf("expected kernel: node excluded, got %d rows: %+v", len(rows), rows)
	}
	// Sorted by node ID: api, db.
	if rows[0].Node != "api" || rows[0].Health != "healthy" || rows[0].PID != "4242" {
		t.Errorf("api row = %+v", rows[0])
	}
	if rows[1].Node != "db" || rows[1].Health != "unhealthy" || rows[1].PID != "-" {
		t.Errorf("db row = %+v", rows[1])
	}
}

func TestWriteMonitorTable_RendersHeaderAndRows(t *testing.T) {
	var buf bytes.Buffer
	writeMonitorTable(&buf, []monitorRow{
		{Node: "api", Type: "service", Adapter: "go:fiber", Health: "healthy", PID: "123"},
	})
	out := buf.String()
	if !strings.Contains(out, "NODE") || !strings.Contains(out, "HEALTH") {
		t.Fatalf("expected header row, got %q", out)
	}
	if !strings.Contains(out, "api") || !strings.Contains(out, "healthy") || !strings.Contains(out, "123") {
		t.Fatalf("expected data row, got %q", out)
	}
}

func TestRunMonitor_OneShotPrintsOnceAndReturns(t *testing.T) {
	root := t.TempDir()
	g := graph.NewTestGraph(map[string]*graph.Node{
		"api": {ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber"},
	})
	var buf bytes.Buffer
	stop := make(chan struct{})

	runMonitor(g, fakeHealthPoller{}, root, &buf, false, time.Millisecond, stop)

	if strings.Count(buf.String(), "NODE") != 1 {
		t.Fatalf("expected exactly one table printed in one-shot mode, got %q", buf.String())
	}
}

func TestRunMonitor_WatchRefreshesUntilStopped(t *testing.T) {
	root := t.TempDir()
	g := graph.NewTestGraph(map[string]*graph.Node{
		"api": {ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber"},
	})
	var buf bytes.Buffer
	stop := make(chan struct{})

	done := make(chan struct{})
	go func() {
		runMonitor(g, fakeHealthPoller{}, root, &buf, true, 5*time.Millisecond, stop)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	close(stop)
	<-done

	if strings.Count(buf.String(), "NODE") < 2 {
		t.Fatalf("expected multiple refreshes in watch mode, got %q", buf.String())
	}
}
