package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/acthur/acthur/internal/plugin/builtin/admin"
	_ "github.com/acthur/acthur/internal/plugin/builtin/featureflags"
	_ "github.com/acthur/acthur/internal/plugin/builtin/https"
	_ "github.com/acthur/acthur/internal/plugin/builtin/observability"
	_ "github.com/acthur/acthur/internal/plugin/builtin/security"
)

// TestRunAdd_EcosystemPlugins_RoundTrip proves the full CLI-level flow
// (acthur add <plugin> → acthur.yml updated → generator run → files
// written + recorded in generated.lock) works for each of the five Phase 9
// "ecosystem" plugins (tracker #65), the same way TestRunAdd_Appends... in
// add_test.go already proves it for "migrations". Each plugin's own
// package has deeper generator-output and compile tests; this only proves
// the CLI wiring (blank import → plugin registry → runAdd) is intact.
func TestRunAdd_EcosystemPlugins_RoundTrip(t *testing.T) {
	cases := []struct {
		plugin       string
		expectedFile string
	}{
		// feature-flags/admin/observability/security generate per-service
		// code, written under the target node's own directory (here: "api").
		{"feature-flags", "api/internal/flags/flags.go"},
		{"admin", "api/internal/admin/admin.go"},
		{"observability", "api/internal/observability/observability.go"},
		{"security", "api/internal/security/security.go"},
		// https is project-scoped (a dev cert isn't per-service), so its
		// generated file lands at the project root under .acthur/.
		{"https", ".acthur/certs/README.md"},
	}

	for _, tc := range cases {
		t.Run(tc.plugin, func(t *testing.T) {
			resetPluginProcessState(t)
			dir := t.TempDir()
			writeTestProject(t, dir)

			summary, err := runAdd(dir, tc.plugin, "")
			if err != nil {
				t.Fatalf("runAdd(%q): %v", tc.plugin, err)
			}
			if summary.AlreadyHad {
				t.Errorf("expected AlreadyHad=false on first add of %q", tc.plugin)
			}

			ymlData, err := os.ReadFile(filepath.Join(dir, "acthur.yml"))
			if err != nil {
				t.Fatal(err)
			}
			if want := "  - name: " + tc.plugin; !strings.Contains(string(ymlData), want) {
				t.Errorf("expected acthur.yml to list plugin %q, got:\n%s", tc.plugin, ymlData)
			}

			p := filepath.Join(dir, tc.expectedFile)
			if _, err := os.Stat(p); err != nil {
				t.Errorf("expected %s to exist after acthur add %s: %v", p, tc.plugin, err)
			}

			lockData, err := os.ReadFile(filepath.Join(dir, "generated.lock"))
			if err != nil {
				t.Fatalf("expected generated.lock to be written: %v", err)
			}
			if !strings.Contains(string(lockData), tc.expectedFile) {
				t.Errorf("expected generated.lock to record %s, got:\n%s", tc.expectedFile, lockData)
			}
		})
	}
}
