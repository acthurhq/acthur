package generate_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acthurhq/acthur/internal/generate"
	"github.com/acthurhq/acthur/internal/plugin"
	"gopkg.in/yaml.v3"
)

// readLock reads generated.lock at root and returns it as a plain map for
// assertions, failing the test if it can't be parsed.
func readLock(t *testing.T, root string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "generated.lock"))
	if err != nil {
		t.Fatalf("reading generated.lock: %v", err)
	}
	var m map[string]string
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatalf("parsing generated.lock: %v", err)
	}
	return m
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestWriteFiles_FreshWrite_WritesAndRecordsLock(t *testing.T) {
	root := t.TempDir()
	content := []byte("package api\n\nfunc Foo() {}\n")
	files := []plugin.GeneratedFile{
		{Path: "handler.go", Content: content},
	}

	results, err := generate.WriteFiles(root, "api", files)
	if err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}
	if len(results) != 1 || results[0].Status != generate.StatusWritten {
		t.Fatalf("expected 1 written result, got %+v", results)
	}

	got, err := os.ReadFile(filepath.Join(root, "api", "handler.go"))
	if err != nil {
		t.Fatalf("expected file written: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("file content mismatch: got %q want %q", got, content)
	}

	lock := readLock(t, root)
	want := sha256Hex(content)
	if lock["api/handler.go"] != want {
		t.Errorf("lock entry mismatch: got %q want %q (lock=%+v)", lock["api/handler.go"], want, lock)
	}
}

// TestWriteFiles_NestedPath_LockKeyUsesForwardSlash guards the generated.lock
// key format for a GeneratedFile.Path with subdirectories (e.g. the
// ecosystem plugins' "internal/security/security.go" style output). The key
// must always be forward-slash-joined ("api/internal/security/security.go"),
// never OS-native-joined: on Windows, joining nodeID and relPath with
// filepath.Join would key the entry as "api\internal\security\security.go",
// which then fails to match any "/"-separated lookup or assertion (as
// TestRunAdd_EcosystemPlugins_RoundTrip in cmd/acthur does). This can't
// reproduce the Windows failure mode on Linux — filepath.Join is "/" on
// every non-Windows GOOS — but it locks in the intended, OS-independent key
// shape so a future refactor back to filepath.Join in lockKey is caught here
// rather than only in Windows CI.
func TestWriteFiles_NestedPath_LockKeyUsesForwardSlash(t *testing.T) {
	root := t.TempDir()
	content := []byte("package security\n")
	files := []plugin.GeneratedFile{
		{Path: "internal/security/security.go", Content: content},
	}

	if _, err := generate.WriteFiles(root, "api", files); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}

	lock := readLock(t, root)
	const wantKey = "api/internal/security/security.go"
	if _, ok := lock[wantKey]; !ok {
		t.Errorf("expected lock to have forward-slash key %q, got keys: %+v", wantKey, lock)
	}
	for k := range lock {
		if strings.Contains(k, "\\") {
			t.Errorf("lock key %q contains a backslash; keys must always be forward-slash-joined", k)
		}
	}
}

func TestWriteFiles_CleanRegenerate_OverwritesWhenDiskMatchesLock(t *testing.T) {
	root := t.TempDir()
	original := []byte("package api\n\nfunc Foo() {}\n")
	files := []plugin.GeneratedFile{{Path: "handler.go", Content: original}}

	if _, err := generate.WriteFiles(root, "api", files); err != nil {
		t.Fatalf("first WriteFiles: %v", err)
	}

	updated := []byte("package api\n\nfunc Foo() { /* v2 */ }\n")
	files2 := []plugin.GeneratedFile{{Path: "handler.go", Content: updated}}
	results, err := generate.WriteFiles(root, "api", files2)
	if err != nil {
		t.Fatalf("second WriteFiles: %v", err)
	}
	if len(results) != 1 || results[0].Status != generate.StatusWritten {
		t.Fatalf("expected regeneration to be written, got %+v", results)
	}

	got, err := os.ReadFile(filepath.Join(root, "api", "handler.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(updated) {
		t.Errorf("expected file updated to new content, got %q", got)
	}

	lock := readLock(t, root)
	if lock["api/handler.go"] != sha256Hex(updated) {
		t.Errorf("expected lock updated to new hash, got %+v", lock)
	}
}

func TestWriteFiles_UserEdited_SkipsWithWarning(t *testing.T) {
	root := t.TempDir()
	original := []byte("package api\n\nfunc Foo() {}\n")
	files := []plugin.GeneratedFile{{Path: "handler.go", Content: original}}

	if _, err := generate.WriteFiles(root, "api", files); err != nil {
		t.Fatalf("first WriteFiles: %v", err)
	}

	// Simulate the user hand-editing the generated file.
	userEdited := []byte("package api\n\nfunc Foo() { /* user wrote this */ }\n")
	if err := os.WriteFile(filepath.Join(root, "api", "handler.go"), userEdited, 0o644); err != nil {
		t.Fatal(err)
	}

	updated := []byte("package api\n\nfunc Foo() { /* regenerated */ }\n")
	files2 := []plugin.GeneratedFile{{Path: "handler.go", Content: updated}}
	results, err := generate.WriteFiles(root, "api", files2)
	if err != nil {
		t.Fatalf("second WriteFiles: %v", err)
	}
	if len(results) != 1 || results[0].Status != generate.StatusSkipped {
		t.Fatalf("expected skip, got %+v", results)
	}
	if results[0].Warning == "" {
		t.Error("expected a warning message on skip")
	}

	got, err := os.ReadFile(filepath.Join(root, "api", "handler.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(userEdited) {
		t.Errorf("expected user edits preserved, got %q", got)
	}

	lock := readLock(t, root)
	if lock["api/handler.go"] != sha256Hex(original) {
		t.Errorf("expected lock hash to remain the last-generated hash, got %+v", lock)
	}
}

func TestWriteFiles_MergeMarker_MergesInsteadOfSkipping(t *testing.T) {
	root := t.TempDir()
	marker := "// ACTHUR:GENERATED-ABOVE"
	original := []byte("package api\n" + marker + "\n")
	files := []plugin.GeneratedFile{{Path: "routes.go", Content: original, MergeMarker: marker}}

	if _, err := generate.WriteFiles(root, "api", files); err != nil {
		t.Fatalf("first WriteFiles: %v", err)
	}

	// User adds hand-written content below the marker.
	userContent := []byte("package api\n" + marker + "\nfunc UserRoute() {}\n")
	if err := os.WriteFile(filepath.Join(root, "api", "routes.go"), userContent, 0o644); err != nil {
		t.Fatal(err)
	}

	regenerated := []byte("package api\n" + marker + "\n")
	files2 := []plugin.GeneratedFile{{Path: "routes.go", Content: regenerated, MergeMarker: marker}}
	results, err := generate.WriteFiles(root, "api", files2)
	if err != nil {
		t.Fatalf("second WriteFiles: %v", err)
	}
	if len(results) != 1 || results[0].Status != generate.StatusMerged {
		t.Fatalf("expected merge, got %+v", results)
	}

	got, err := os.ReadFile(filepath.Join(root, "api", "routes.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "func UserRoute() {}") {
		t.Errorf("expected user content below the marker preserved, got %q", got)
	}
}

func TestWriteFiles_MigrationsRoute_ToProjectRoot(t *testing.T) {
	root := t.TempDir()
	content := []byte("CREATE TABLE foo (id int);\n")
	files := []plugin.GeneratedFile{
		{Path: "migrations/0001_init.up.sql", Content: content},
	}

	if _, err := generate.WriteFiles(root, "api", files); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "migrations", "0001_init.up.sql")); err != nil {
		t.Errorf("expected migration at project root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "api", "migrations")); err == nil {
		t.Error("migrations must not be written under the node directory")
	}

	lock := readLock(t, root)
	if lock["migrations/0001_init.up.sql"] != sha256Hex(content) {
		t.Errorf("expected lock keyed by the migrations/ path itself, got %+v", lock)
	}
}

// TestWriteFiles_DeployRoute_ToProjectRoot: deploy artifacts (Phase 8, #51)
// span the whole graph, not one node — a "deploy/" path must land at the
// project root exactly like "migrations/" does, regardless of the nodeID
// the caller passes to WriteFiles.
func TestWriteFiles_DeployRoute_ToProjectRoot(t *testing.T) {
	root := t.TempDir()
	content := []byte("services: {}\n")
	files := []plugin.GeneratedFile{
		{Path: "deploy/docker-compose.prod.yml", Content: content, Overwrite: true},
	}

	if _, err := generate.WriteFiles(root, "api", files); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "deploy", "docker-compose.prod.yml")); err != nil {
		t.Errorf("expected compose file at project root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "api", "deploy")); err == nil {
		t.Error("deploy artifacts must not be written under the node directory")
	}

	lock := readLock(t, root)
	if lock["deploy/docker-compose.prod.yml"] != sha256Hex(content) {
		t.Errorf("expected lock keyed by the deploy/ path itself, got %+v", lock)
	}
}

func TestWriteFiles_PresentButNotInLock_RespectsOverwriteFlag(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := []byte("legacy content\n")
	if err := os.WriteFile(filepath.Join(root, "api", "legacy.go"), existing, 0o644); err != nil {
		t.Fatal(err)
	}

	newContent := []byte("new content\n")

	// Overwrite: false -> skip, leaving the legacy file untouched, no lock entry.
	results, err := generate.WriteFiles(root, "api", []plugin.GeneratedFile{
		{Path: "legacy.go", Content: newContent, Overwrite: false},
	})
	if err != nil {
		t.Fatalf("WriteFiles (no overwrite): %v", err)
	}
	if len(results) != 1 || results[0].Status != generate.StatusSkipped {
		t.Fatalf("expected skip, got %+v", results)
	}
	got, _ := os.ReadFile(filepath.Join(root, "api", "legacy.go"))
	if string(got) != string(existing) {
		t.Errorf("expected legacy file untouched, got %q", got)
	}

	// Overwrite: true -> written, and now recorded in the lock.
	results2, err := generate.WriteFiles(root, "api", []plugin.GeneratedFile{
		{Path: "legacy.go", Content: newContent, Overwrite: true},
	})
	if err != nil {
		t.Fatalf("WriteFiles (overwrite): %v", err)
	}
	if len(results2) != 1 || results2[0].Status != generate.StatusWritten {
		t.Fatalf("expected written, got %+v", results2)
	}
	got2, _ := os.ReadFile(filepath.Join(root, "api", "legacy.go"))
	if string(got2) != string(newContent) {
		t.Errorf("expected file overwritten, got %q", got2)
	}
	lock := readLock(t, root)
	if lock["api/legacy.go"] != sha256Hex(newContent) {
		t.Errorf("expected lock entry recorded after overwrite, got %+v", lock)
	}
}

func TestWriteFiles_LockRoundTrip_AtomicNoTempFileLeftBehind(t *testing.T) {
	root := t.TempDir()
	files := []plugin.GeneratedFile{
		{Path: "a.go", Content: []byte("package api\n")},
		{Path: "b.go", Content: []byte("package api\n\nvar B = 2\n")},
	}
	if _, err := generate.WriteFiles(root, "api", files); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "tmp") || strings.HasSuffix(e.Name(), ".lock.tmp") {
			t.Errorf("expected no leftover temp file, found %s", e.Name())
		}
	}

	lock := readLock(t, root)
	if len(lock) != 2 {
		t.Fatalf("expected 2 lock entries, got %+v", lock)
	}
	if lock["api/a.go"] != sha256Hex(files[0].Content) {
		t.Errorf("a.go hash mismatch: %+v", lock)
	}
	if lock["api/b.go"] != sha256Hex(files[1].Content) {
		t.Errorf("b.go hash mismatch: %+v", lock)
	}

	// Round-trip: loading the lock again via a fresh WriteFiles call for a
	// third file must preserve the two existing entries.
	if _, err := generate.WriteFiles(root, "api", []plugin.GeneratedFile{
		{Path: "c.go", Content: []byte("package api\n\nvar C = 3\n")},
	}); err != nil {
		t.Fatalf("third WriteFiles: %v", err)
	}
	lock2 := readLock(t, root)
	if len(lock2) != 3 {
		t.Fatalf("expected 3 lock entries after round-trip, got %+v", lock2)
	}
}

func TestVerifyGeneratedArtifacts(t *testing.T) {
	t.Run("matching tracked file passes", func(t *testing.T) {
		root := t.TempDir()
		content := []byte("package api\n")
		if _, err := generate.WriteFiles(root, "api", []plugin.GeneratedFile{{Path: "generated.go", Content: content}}); err != nil {
			t.Fatal(err)
		}
		if err := generate.VerifyGeneratedArtifacts(root); err != nil {
			t.Fatalf("expected generated artifacts to verify, got: %v", err)
		}
	})

	t.Run("reports every missing or changed path", func(t *testing.T) {
		root := t.TempDir()
		files := []plugin.GeneratedFile{
			{Path: "missing.go", Content: []byte("package api\n")},
			{Path: "changed.go", Content: []byte("package api\n")},
		}
		if _, err := generate.WriteFiles(root, "api", files); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(root, "api", "missing.go")); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "api", "changed.go"), []byte("package api\n\n// user edit\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		err := generate.VerifyGeneratedArtifacts(root)
		if err == nil {
			t.Fatal("expected stale generated artifacts to fail verification")
		}
		for _, want := range []string{"api/missing.go: missing", "api/changed.go: content differs"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("expected error to contain %q, got: %v", want, err)
			}
		}
	})
}
