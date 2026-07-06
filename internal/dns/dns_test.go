package dns

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeLookup(resolves map[string]bool) LookupFunc {
	return func(host string) ([]net.IP, error) {
		if resolves[host] {
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		}
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
}

func TestResolvesToLocalhost_TrueForLoopback(t *testing.T) {
	lookup := fakeLookup(map[string]bool{"api.acme.test": true})
	if !ResolvesToLocalhost(lookup, "api.acme.test") {
		t.Fatal("expected api.acme.test to resolve to localhost")
	}
}

func TestResolvesToLocalhost_FalseOnLookupError(t *testing.T) {
	lookup := fakeLookup(map[string]bool{})
	if ResolvesToLocalhost(lookup, "api.acme.test") {
		t.Fatal("expected unresolved host to report false")
	}
}

func TestResolvesToLocalhost_FalseForNonLoopbackIP(t *testing.T) {
	lookup := func(host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}
	if ResolvesToLocalhost(lookup, "example.com") {
		t.Fatal("expected non-loopback IP to report false")
	}
}

func TestHostnames_IncludesDomainAndSortedSubdomains(t *testing.T) {
	got := Hostnames("acme.test", []string{"web", "api"})
	want := []string{"acme.test", "api.acme.test", "web.acme.test"}
	if len(got) != len(want) {
		t.Fatalf("hostnames mismatch: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("hostnames mismatch: got %v want %v", got, want)
		}
	}
}

func TestHostnames_EmptyDomainYieldsNothing(t *testing.T) {
	if got := Hostnames("", []string{"api"}); got != nil {
		t.Fatalf("expected nil for empty domain, got %v", got)
	}
}

func TestMissing_ReturnsOnlyUnresolvedHosts(t *testing.T) {
	lookup := fakeLookup(map[string]bool{"acme.test": true})
	hosts := []string{"acme.test", "api.acme.test"}
	got := Missing(lookup, hosts)
	if len(got) != 1 || got[0] != "api.acme.test" {
		t.Fatalf("expected only api.acme.test missing, got %v", got)
	}
}

func TestMissing_EmptyWhenAllResolve(t *testing.T) {
	lookup := fakeLookup(map[string]bool{"acme.test": true, "api.acme.test": true})
	got := Missing(lookup, []string{"acme.test", "api.acme.test"})
	if len(got) != 0 {
		t.Fatalf("expected no missing hosts, got %v", got)
	}
}

func TestInstructions_MentionsEveryMissingHostAndNeverSuggestsAutoSudo(t *testing.T) {
	out := Instructions([]string{"acme.test", "api.acme.test"})
	if !strings.Contains(out, "127.0.0.1 acme.test") {
		t.Fatalf("expected instructions to mention acme.test hosts line, got:\n%s", out)
	}
	if !strings.Contains(out, "127.0.0.1 api.acme.test") {
		t.Fatalf("expected instructions to mention api.acme.test hosts line, got:\n%s", out)
	}
	if !strings.Contains(out, "will not") {
		t.Fatalf("expected instructions to state acthur will not self-escalate, got:\n%s", out)
	}
}

func TestInstructions_EmptyForNoMissingHosts(t *testing.T) {
	if out := Instructions(nil); out != "" {
		t.Fatalf("expected empty instructions for no missing hosts, got %q", out)
	}
}

func TestWriteHostsEntries_AppendsLinesToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts")
	if err := os.WriteFile(path, []byte("127.0.0.1 localhost\n"), 0o644); err != nil {
		t.Fatalf("seed hosts file: %v", err)
	}

	if err := WriteHostsEntries(path, []string{"acme.test", "api.acme.test"}); err != nil {
		t.Fatalf("write hosts entries: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back hosts file: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "127.0.0.1 localhost") {
		t.Fatalf("expected existing content preserved, got:\n%s", got)
	}
	if !strings.Contains(got, "127.0.0.1 acme.test") || !strings.Contains(got, "127.0.0.1 api.acme.test") {
		t.Fatalf("expected new host lines appended, got:\n%s", got)
	}
}

func TestWriteHostsEntries_ErrorsWithoutSuppressingWhenPathUnwritable(t *testing.T) {
	err := WriteHostsEntries("/nonexistent-dir-for-acthur-test/hosts", []string{"acme.test"})
	if err == nil {
		t.Fatal("expected an error writing to an unwritable path")
	}
}
