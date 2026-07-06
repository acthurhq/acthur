// Package dns checks whether the hostnames `acthur dev` routes traffic
// through (the project's dev domain and its per-node subdomains) resolve to
// 127.0.0.1, and produces copy-pastable /etc/hosts instructions when they
// don't.
//
// Acthur never escalates privileges on the operator's behalf: it will never
// shell out to sudo. The only privileged action it can take is the one the
// operator explicitly opts into via `acthur dev --write-hosts`, which still
// requires the process to already have write permission on the hosts file
// (e.g. because the operator ran it with sudo themselves).
package dns

import (
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
)

// LookupFunc resolves host to a set of IPs. It is the seam that lets tests
// assert dev preflight behavior without depending on the machine's real
// resolver or /etc/hosts contents.
type LookupFunc func(host string) ([]net.IP, error)

// DefaultLookup resolves host using the OS resolver.
func DefaultLookup(host string) ([]net.IP, error) {
	return net.LookupIP(host)
}

// ResolvesToLocalhost reports whether host resolves to a loopback address
// using lookup. Any lookup error (including NXDOMAIN) counts as "does not
// resolve" — the caller doesn't need to distinguish why.
func ResolvesToLocalhost(lookup LookupFunc, host string) bool {
	if lookup == nil {
		lookup = DefaultLookup
	}
	ips, err := lookup(host)
	if err != nil {
		return false
	}
	for _, ip := range ips {
		if ip.IsLoopback() {
			return true
		}
	}
	return false
}

// Hostnames returns every hostname that must resolve to 127.0.0.1 for domain
// dev routing to work: the bare dev domain plus one subdomain per node ID
// (e.g. domain "acme.test", nodes ["api","web"] -> ["acme.test",
// "api.acme.test", "web.acme.test"]). Node IDs are sorted for stable output.
func Hostnames(domain string, nodeIDs []string) []string {
	if domain == "" {
		return nil
	}
	ids := append([]string(nil), nodeIDs...)
	sort.Strings(ids)
	hosts := make([]string, 0, len(ids)+1)
	hosts = append(hosts, domain)
	for _, id := range ids {
		hosts = append(hosts, id+"."+domain)
	}
	return hosts
}

// Missing filters hosts down to the ones that don't currently resolve to
// 127.0.0.1 via lookup.
func Missing(lookup LookupFunc, hosts []string) []string {
	var missing []string
	for _, h := range hosts {
		if !ResolvesToLocalhost(lookup, h) {
			missing = append(missing, h)
		}
	}
	return missing
}

// HostsBlock renders the /etc/hosts lines (no header/trailer) for hosts,
// one "127.0.0.1 <host>" per line.
func HostsBlock(hosts []string) string {
	lines := make([]string, len(hosts))
	for i, h := range hosts {
		lines[i] = "127.0.0.1 " + h
	}
	return strings.Join(lines, "\n")
}

// Instructions renders a human-readable, copy-pastable block explaining how
// to make the given unresolved hostnames resolve to 127.0.0.1. Acthur prints
// this instead of running anything as root itself.
func Instructions(hosts []string) string {
	if len(hosts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("The following hostnames don't resolve to 127.0.0.1 yet:\n\n")
	for _, h := range hosts {
		fmt.Fprintf(&b, "  %s\n", h)
	}
	b.WriteString("\nAdd these lines to /etc/hosts (requires sudo):\n\n")
	for _, line := range strings.Split(HostsBlock(hosts), "\n") {
		fmt.Fprintf(&b, "  %s\n", line)
	}
	b.WriteString("\nOr run it in one shot:\n\n")
	fmt.Fprintf(&b, "  sudo sh -c 'cat >> /etc/hosts <<EOF\n%s\nEOF'\n", HostsBlock(hosts))
	b.WriteString("\nActhur will not modify /etc/hosts unless you pass --write-hosts, and even\nthen it never escalates privileges — it only writes if the process already\nhas permission.\n")
	return b.String()
}

// WriteHostsEntries appends "127.0.0.1 <host>" lines for hosts to the hosts
// file at path. It never escalates privileges — if the process lacks write
// permission on path, this simply returns the underlying error for the
// caller to fall back to printing Instructions.
func WriteHostsEntries(path string, hosts []string) error {
	if len(hosts) == 0 {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open %s for append: %w", path, err)
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "\n# added by acthur dev --write-hosts\n%s\n", HostsBlock(hosts)); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
