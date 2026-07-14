package engine

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/graph"
)

// alwaysMissingLookup reports every hostname as unresolved — the seam for
// asserting the "needs setup" path without depending on a real resolver.
func alwaysMissingLookup(host string) ([]net.IP, error) {
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

// alwaysResolvesLookup reports every hostname as already pointing at
// 127.0.0.1 — the seam for asserting the "nothing to do" path.
func alwaysResolvesLookup(host string) ([]net.IP, error) {
	return []net.IP{net.ParseIP("127.0.0.1")}, nil
}

// TestCheckDNS_NoDomainIsNoop asserts a project with no configured dev
// domain (e.g. a minimal test graph) never attempts a DNS check or a hosts
// file write.
func TestCheckDNS_NoDomainIsNoop(t *testing.T) {
	api := &graph.Node{ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber"}
	g := graph.NewTestGraph(map[string]*graph.Node{"api": api})
	eng := NewDevEngine(&config.Config{}, g, fakeDevResolver{})
	eng.dnsLookup = alwaysMissingLookup

	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")
	if err := os.WriteFile(hostsPath, []byte("127.0.0.1 localhost\n"), 0o644); err != nil {
		t.Fatalf("seed hosts file: %v", err)
	}
	eng.hostsPath = hostsPath
	eng.writeHosts = true

	eng.checkDNS()

	data, _ := os.ReadFile(hostsPath)
	if strings.Contains(string(data), "acthur") {
		t.Fatalf("expected no hosts file write with no configured domain, got:\n%s", data)
	}
}

// TestCheckDNS_AllResolvedIsNoop asserts a fully-resolved domain never
// touches the hosts file even when --write-hosts is set.
func TestCheckDNS_AllResolvedIsNoop(t *testing.T) {
	api := &graph.Node{ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber"}
	g := graph.NewTestGraph(map[string]*graph.Node{"api": api})
	eng := NewDevEngine(&config.Config{Dev: config.DevConfig{Domain: "acme.test"}}, g, fakeDevResolver{})
	eng.dnsLookup = alwaysResolvesLookup

	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")
	if err := os.WriteFile(hostsPath, []byte("127.0.0.1 localhost\n"), 0o644); err != nil {
		t.Fatalf("seed hosts file: %v", err)
	}
	eng.hostsPath = hostsPath
	eng.writeHosts = true

	eng.checkDNS()

	data, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("read hosts file: %v", err)
	}
	if string(data) != "127.0.0.1 localhost\n" {
		t.Fatalf("expected hosts file untouched, got:\n%s", data)
	}
}

// TestCheckDNS_WriteHostsAppendsMissingHostnames asserts --write-hosts
// appends every unresolved hostname (domain + node subdomains) to the
// configured hosts file.
func TestCheckDNS_WriteHostsAppendsMissingHostnames(t *testing.T) {
	api := &graph.Node{ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber"}
	g := graph.NewTestGraph(map[string]*graph.Node{"api": api})
	eng := NewDevEngine(&config.Config{Dev: config.DevConfig{Domain: "acme.test"}}, g, fakeDevResolver{})
	eng.dnsLookup = alwaysMissingLookup

	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")
	if err := os.WriteFile(hostsPath, []byte("127.0.0.1 localhost\n"), 0o644); err != nil {
		t.Fatalf("seed hosts file: %v", err)
	}
	eng.hostsPath = hostsPath
	eng.writeHosts = true

	eng.checkDNS()

	data, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("read hosts file: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "127.0.0.1 acme.test") {
		t.Fatalf("expected acme.test appended, got:\n%s", got)
	}
	if !strings.Contains(got, "127.0.0.1 api.acme.test") {
		t.Fatalf("expected api.acme.test appended, got:\n%s", got)
	}
}

// TestCheckDNS_WithoutWriteHostsNeverTouchesFile asserts the default
// (no --write-hosts) path only prints instructions — it must never write to
// hostsPath even if one is configured.
func TestCheckDNS_WithoutWriteHostsNeverTouchesFile(t *testing.T) {
	api := &graph.Node{ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber"}
	g := graph.NewTestGraph(map[string]*graph.Node{"api": api})
	eng := NewDevEngine(&config.Config{Dev: config.DevConfig{Domain: "acme.test"}}, g, fakeDevResolver{})
	eng.dnsLookup = alwaysMissingLookup

	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")
	if err := os.WriteFile(hostsPath, []byte("127.0.0.1 localhost\n"), 0o644); err != nil {
		t.Fatalf("seed hosts file: %v", err)
	}
	eng.hostsPath = hostsPath
	eng.writeHosts = false

	eng.checkDNS()

	data, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("read hosts file: %v", err)
	}
	if string(data) != "127.0.0.1 localhost\n" {
		t.Fatalf("expected hosts file untouched without --write-hosts, got:\n%s", data)
	}
}
