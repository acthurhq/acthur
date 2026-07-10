package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acthur/acthur/internal/plugin"
)

// resetPluginProcessState clears the process-global plugin-kernel state
// (kernelBus/kernelAPI/loadedPlugins/bootstrapped) so each test starts from
// a clean slate, matching the pattern in
// TestLoadGraph_AfterBootstrap_DoesNotDoubleRegisterHooks
// (plugin_loading_test.go) — plugins register hooks/commands on
// process-global state, so tests that load them must reset it afterward.
func resetPluginProcessState(t *testing.T) {
	t.Helper()
	oldBus := kernelBus
	oldAPI := kernelAPI
	oldLoaded := loadedPlugins
	oldBootstrapped := bootstrapped

	kernelBus = plugin.NewBus()
	kernelAPI = nil
	loadedPlugins = nil
	bootstrapped = bootstrapResult{}

	t.Cleanup(func() {
		kernelBus = oldBus
		kernelAPI = oldAPI
		loadedPlugins = oldLoaded
		bootstrapped = oldBootstrapped
	})
}

// writeTestProject writes a minimal acthur.yml (one go:fiber service node,
// no plugins) to dir and chdir's into it for the duration of the test.
func writeTestProject(t *testing.T, dir string) {
	t.Helper()
	yml := `project: p
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
      port: 8080
`
	if err := os.WriteFile(filepath.Join(dir, "acthur.yml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
}

func TestRunAdd_AppendsPluginAndWritesMigrationsFilesAtRoot(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	writeTestProject(t, dir)

	summary, err := runAdd(dir, "migrations", "")
	if err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	if summary.AlreadyHad {
		t.Error("expected AlreadyHad=false on first add")
	}

	ymlData, err := os.ReadFile(filepath.Join(dir, "acthur.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ymlData), "  - name: migrations") {
		t.Errorf("expected acthur.yml to list the migrations plugin, got:\n%s", ymlData)
	}
	// The original node config must survive untouched (line-based insertion,
	// not a full re-marshal).
	if !strings.Contains(string(ymlData), "adapter: go:fiber") {
		t.Error("expected existing acthur.yml content to be preserved")
	}

	for _, rel := range []string{
		"migrations/.keep",
		"migrations/0001_init.up.sql",
		"migrations/0001_init.down.sql",
	} {
		p := filepath.Join(dir, rel)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to exist: %v", p, err)
		}
	}
	// Migration files must land at the project root, not under <root>/api/.
	if _, err := os.Stat(filepath.Join(dir, "api", "migrations")); err == nil {
		t.Error("migrations/ must not be written under the node directory")
	}

	wantStatuses := map[string]addFileStatus{
		"migrations/.keep":              addFileWritten,
		"migrations/0001_init.up.sql":   addFileWritten,
		"migrations/0001_init.down.sql": addFileWritten,
	}
	for _, r := range summary.Results {
		if want, ok := wantStatuses[r.Path]; !ok || want != r.Status {
			t.Errorf("unexpected result for %s: %+v", r.Path, r)
		}
	}

	// Phase 7 slice 1 (#48): files acthur add writes are now also recorded
	// in generated.lock at the project root.
	lockData, err := os.ReadFile(filepath.Join(dir, "generated.lock"))
	if err != nil {
		t.Fatalf("expected generated.lock to be written: %v", err)
	}
	for _, rel := range []string{
		"migrations/.keep",
		"migrations/0001_init.up.sql",
		"migrations/0001_init.down.sql",
	} {
		if !strings.Contains(string(lockData), rel) {
			t.Errorf("expected generated.lock to record %s, got:\n%s", rel, lockData)
		}
	}
}

// TestRunAdd_Idempotent_SecondCallRegeneratesUnchangedFilesViaLock: acthur
// add now writes through internal/generate, which records every file it
// writes in generated.lock (Phase 7 slice 1, #48). A second, identical add
// is still idempotent byte-for-byte — the on-disk content the generator
// produces the second time round is unchanged — but the *reported* status
// is now "written" rather than "skipped", because the write engine treats
// "disk hash still matches the lock" as safe-to-regenerate (the user never
// touched the file) rather than "already exists, leave alone". That is the
// documented generated.lock semantic from the Phase 7 PRD, not a behavior
// regression: nothing on disk actually changes.
func TestRunAdd_Idempotent_SecondCallRegeneratesUnchangedFilesViaLock(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	writeTestProject(t, dir)

	if _, err := runAdd(dir, "migrations", ""); err != nil {
		t.Fatalf("first runAdd: %v", err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "migrations", "0001_init.up.sql"))
	if err != nil {
		t.Fatal(err)
	}

	summary, err := runAdd(dir, "migrations", "")
	if err != nil {
		t.Fatalf("second runAdd: %v", err)
	}
	if !summary.AlreadyHad {
		t.Error("expected AlreadyHad=true on second add")
	}

	ymlData, err := os.ReadFile(filepath.Join(dir, "acthur.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(ymlData), "name: migrations") != 1 {
		t.Errorf("expected exactly one migrations plugin entry, got:\n%s", ymlData)
	}

	for _, r := range summary.Results {
		if r.Status != addFileWritten {
			t.Errorf("expected all files to be regenerated (written) on an untouched second add, got %+v", r)
		}
	}

	after, err := os.ReadFile(filepath.Join(dir, "migrations", "0001_init.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("expected regenerated content to be byte-identical since nothing changed")
	}

	if _, err := os.Stat(filepath.Join(dir, "generated.lock")); err != nil {
		t.Errorf("expected generated.lock to exist after add: %v", err)
	}
}

func TestRunAdd_UnknownPlugin_ErrorsAndLeavesYAMLUntouched(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	writeTestProject(t, dir)

	before, err := os.ReadFile(filepath.Join(dir, "acthur.yml"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = runAdd(dir, "does-not-exist", "")
	if err == nil {
		t.Fatal("expected an error for an unknown plugin")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("expected error to name the plugin, got: %v", err)
	}

	after, err := os.ReadFile(filepath.Join(dir, "acthur.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("expected acthur.yml to be unchanged when the plugin is unknown")
	}
}

func TestRunAdd_NodeFlag_RejectsNonGoFiberAdapter(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	yml := `project: p
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
      port: 8080
    web:
      type: service
      adapter: ui:astro
      port: 3000
`
	if err := os.WriteFile(filepath.Join(dir, "acthur.yml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	_ = os.Chdir(dir)

	_, err := runAdd(dir, "migrations", "web")
	if err == nil {
		t.Fatal("expected an error targeting a non-go:fiber node")
	}
	if !strings.Contains(err.Error(), "web") {
		t.Errorf("expected error to name the node, got: %v", err)
	}
}

func TestRunAdd_NodeFlag_TargetsOnlyNamedNode(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	yml := `project: p
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
      port: 8080
    worker:
      type: service
      adapter: go:fiber
      role: queue-worker
`
	if err := os.WriteFile(filepath.Join(dir, "acthur.yml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	_ = os.Chdir(dir)

	summary, err := runAdd(dir, "migrations", "api")
	if err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	for _, r := range summary.Results {
		if r.NodeID != "api" {
			t.Errorf("expected all results to target node \"api\", got %+v", r)
		}
	}
}

func TestEnsurePluginLoaded_ReusesKernelAPIWhenAlreadyLoadedThisProcess(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	writeTestProject(t, dir)

	if _, err := runAdd(dir, "migrations", ""); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	firstAPI := kernelAPI
	if firstAPI == nil {
		t.Fatal("expected kernelAPI to be set after runAdd")
	}

	// A second runAdd for the same plugin (already in acthur.yml and already
	// loaded this process) must reuse the existing KernelAPI rather than
	// loading migrations a second time on the shared kernelBus.
	if _, err := runAdd(dir, "migrations", ""); err != nil {
		t.Fatalf("second runAdd: %v", err)
	}
	if kernelAPI != firstAPI {
		t.Error("expected the second runAdd to reuse the same KernelAPI instance")
	}
	if len(loadedPlugins) != 1 {
		t.Errorf("expected exactly one loaded plugin entry, got %d", len(loadedPlugins))
	}
}

// TestRunAdd_DependencySatisfiedByAlreadyLoadedPlugin: the Phase 6 witness
// caught this — with auth already in acthur.yml and loaded at bootstrap,
// `acthur add rbac` failed with "requires plugin auth but it is not
// installed" because the delta load only considered the delta names when
// checking DependsOn. Already-loaded plugins satisfy dependencies.
func TestRunAdd_DependencySatisfiedByAlreadyLoadedPlugin(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	writeTestProject(t, dir)

	if _, err := runAdd(dir, "auth", ""); err != nil {
		t.Fatalf("add auth: %v", err)
	}
	if _, err := runAdd(dir, "rbac", ""); err != nil {
		t.Fatalf("add rbac with auth already loaded: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "migrations", "0200_roles.up.sql"))
	if err != nil || len(data) == 0 {
		t.Fatalf("expected rbac migration written, err=%v", err)
	}
}

// TestRunAdd_LoadFailure_RollsBackYAMLEdit: a failed add must not leave the
// plugin behind in acthur.yml (the witness left `- name: rbac` in the list
// after the dependency error, wedging every later command).
func TestRunAdd_LoadFailure_RollsBackYAMLEdit(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	writeTestProject(t, dir)

	// rbac depends on auth; auth is neither in the list nor loaded.
	if _, err := runAdd(dir, "rbac", ""); err == nil {
		t.Fatal("expected dependency error adding rbac without auth")
	}
	data, err := os.ReadFile(filepath.Join(dir, "acthur.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "rbac") {
		t.Fatalf("expected failed add to roll back the acthur.yml edit, got:\n%s", data)
	}
}
