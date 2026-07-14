package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/graph"
)

func nonInteractiveWizard(adapterName string, db bool, module string) wizardInput {
	return wizardInput{
		Adapter: adapterName, AdapterSet: true,
		DB: db, DBSet: true,
		Module: module, ModuleSet: true,
	}
}

func TestCLI_NewCompleteFlagsScaffoldsWithoutDatabase(t *testing.T) {
	binName := "acthur"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	bin := filepath.Join(t.TempDir(), binName)
	build := exec.Command("go", "build", "-o", bin, "./cmd/acthur")
	build.Dir = repoRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	cwd := t.TempDir()
	cmd := exec.Command(bin, "new", "widgets", "--adapter", "go:fiber", "--module", "example.com/widgets")
	cmd.Dir = cwd
	cmd.Stdin = strings.NewReader("")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("complete non-interactive scaffold failed: %v\n%s", err, out)
	}

	cfg, err := config.LoadFile(filepath.Join(cwd, "widgets", "acthur.yml"))
	if err != nil {
		t.Fatalf("load scaffolded project: %v", err)
	}
	if _, exists := cfg.Graph.Nodes["db"]; exists {
		t.Fatal("omitting --db must explicitly produce a no-database scaffold")
	}
}

// ---------------------------------------------------------------------------
// resolveNewOptions
// ---------------------------------------------------------------------------

func TestResolveNewOptions_NonInteractiveMissingFlagsErrors(t *testing.T) {
	_, err := resolveNewOptions(&bytes.Buffer{}, &bytes.Buffer{}, "p", wizardInput{}, false)
	if err == nil {
		t.Fatal("expected an error when non-interactive with no flags set")
	}
	if !strings.Contains(err.Error(), "--adapter") || !strings.Contains(err.Error(), "--module") || strings.Contains(err.Error(), "--db") {
		t.Errorf("expected error to name only required non-interactive flags, got %q", err.Error())
	}
}

func TestResolveNewOptions_NonInteractiveAllFlagsSetSucceeds(t *testing.T) {
	opts, err := resolveNewOptions(&bytes.Buffer{}, &bytes.Buffer{}, "p", nonInteractiveWizard("go:fiber", true, "github.com/acme/p"), false)
	if err != nil {
		t.Fatalf("resolveNewOptions: %v", err)
	}
	if opts.Adapter != "go:fiber" || !opts.DB || opts.Module != "github.com/acme/p" {
		t.Errorf("unexpected opts: %+v", opts)
	}
}

func TestResolveNewOptions_InteractivePromptsForMissingAnswers(t *testing.T) {
	in := bytes.NewBufferString("go:fiber\ny\ngithub.com/acme/p\n")
	out := &bytes.Buffer{}
	opts, err := resolveNewOptions(in, out, "p", wizardInput{}, true)
	if err != nil {
		t.Fatalf("resolveNewOptions: %v", err)
	}
	if opts.Adapter != "go:fiber" {
		t.Errorf("expected adapter go:fiber, got %q", opts.Adapter)
	}
	if !opts.DB {
		t.Error("expected db=true from 'y' answer")
	}
	if opts.Module != "github.com/acme/p" {
		t.Errorf("expected module github.com/acme/p, got %q", opts.Module)
	}
	if out.Len() == 0 {
		t.Error("expected wizard to print prompts to out")
	}
}

func TestResolveNewOptions_InteractiveDefaultsOnBlankAnswers(t *testing.T) {
	in := bytes.NewBufferString("\n\n\n")
	opts, err := resolveNewOptions(in, &bytes.Buffer{}, "myproj", wizardInput{}, true)
	if err != nil {
		t.Fatalf("resolveNewOptions: %v", err)
	}
	if opts.Adapter != "go:fiber" {
		t.Errorf("expected default adapter go:fiber, got %q", opts.Adapter)
	}
	if !opts.DB {
		t.Error("expected db default to be true (blank -> Y)")
	}
	if opts.Module != "myproj" {
		t.Errorf("expected module to default to project name, got %q", opts.Module)
	}
}

func TestResolveNewOptions_UnknownAdapterErrors(t *testing.T) {
	_, err := resolveNewOptions(&bytes.Buffer{}, &bytes.Buffer{}, "p", nonInteractiveWizard("does:not-exist", false, "p"), false)
	if err == nil {
		t.Fatal("expected an error for an unknown adapter")
	}
}

func TestResolveNewOptions_NonScaffoldingAdapterErrors(t *testing.T) {
	// db:postgres is registered but implements no Scaffolder — it must be
	// rejected as a project adapter choice.
	_, err := resolveNewOptions(&bytes.Buffer{}, &bytes.Buffer{}, "p", nonInteractiveWizard("db:postgres", false, "p"), false)
	if err == nil {
		t.Fatal("expected an error for a non-scaffolding adapter")
	}
}

// ---------------------------------------------------------------------------
// scaffoldProject / runNew / runInit
// ---------------------------------------------------------------------------

func TestScaffoldProject_WritesValidGraphWithProxyEdge(t *testing.T) {
	dir := t.TempDir()
	result, err := scaffoldProject(dir, "widgets", newOptions{Adapter: "go:fiber", Module: "github.com/acme/widgets"})
	if err != nil {
		t.Fatalf("scaffoldProject: %v", err)
	}

	if result.Adapter != "go:fiber" {
		t.Errorf("expected adapter go:fiber, got %q", result.Adapter)
	}
	if len(result.Files) == 0 {
		t.Error("expected scaffolded files to be reported")
	}

	// api/main.go must exist on disk.
	if _, err := os.Stat(filepath.Join(dir, "api", "main.go")); err != nil {
		t.Errorf("expected api/main.go to exist: %v", err)
	}

	cfg, err := config.LoadFile(filepath.Join(dir, "acthur.yml"))
	if err != nil {
		t.Fatalf("generated acthur.yml failed to load: %v", err)
	}

	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("graph.Build: %v", err)
	}

	if errs := g.Validate(registryResolver{}, contractPathResolver{root: dir}); len(errs) != 0 {
		t.Fatalf("expected zero validation errors, got %d: %+v", len(errs), errs)
	}
}

func TestScaffoldProject_WithDB_AddsDbNodeAndDependsOnEdge(t *testing.T) {
	dir := t.TempDir()
	result, err := scaffoldProject(dir, "widgets", newOptions{Adapter: "go:fiber", DB: true, Module: "widgets"})
	if err != nil {
		t.Fatalf("scaffoldProject: %v", err)
	}
	if !result.DB {
		t.Error("expected result.DB to be true")
	}

	cfg, err := config.LoadFile(filepath.Join(dir, "acthur.yml"))
	if err != nil {
		t.Fatalf("generated acthur.yml failed to load: %v", err)
	}
	if _, ok := cfg.Graph.Nodes["db"]; !ok {
		t.Fatal("expected a 'db' node in generated graph.nodes")
	}

	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("graph.Build: %v", err)
	}
	if errs := g.Validate(registryResolver{}, contractPathResolver{root: dir}); len(errs) != 0 {
		t.Fatalf("expected zero validation errors, got %d: %+v", len(errs), errs)
	}

	found := false
	for _, e := range g.Edges() {
		if e.From == "api" && e.To == "db" && e.Type == config.EdgeDependsOn {
			found = true
		}
	}
	if !found {
		t.Error("expected an api->db depends_on edge")
	}
}

func TestScaffoldProject_RefusesToOverwriteExistingActhurYML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "acthur.yml"), []byte("project: existing\nversion: \"1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := scaffoldProject(dir, "widgets", newOptions{Adapter: "go:fiber", Module: "widgets"})
	if err == nil {
		t.Fatal("expected an error when acthur.yml already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' in error, got %q", err.Error())
	}
}

func TestRunNew_CreatesProjectDirectoryUnderCwd(t *testing.T) {
	cwd := t.TempDir()
	wi := nonInteractiveWizard("go:fiber", false, "widgets")

	result, err := runNew(cwd, "widgets", wi, &bytes.Buffer{}, &bytes.Buffer{}, false)
	if err != nil {
		t.Fatalf("runNew: %v", err)
	}
	if result.ProjectDir != filepath.Join(cwd, "widgets") {
		t.Errorf("expected project dir %s, got %s", filepath.Join(cwd, "widgets"), result.ProjectDir)
	}
	if _, err := os.Stat(filepath.Join(result.ProjectDir, "acthur.yml")); err != nil {
		t.Errorf("expected acthur.yml under project dir: %v", err)
	}
}

func TestRunNew_RefusesNonEmptyExistingDirectory(t *testing.T) {
	cwd := t.TempDir()
	target := filepath.Join(cwd, "widgets")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "something"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := runNew(cwd, "widgets", nonInteractiveWizard("go:fiber", false, "widgets"), &bytes.Buffer{}, &bytes.Buffer{}, false)
	if err == nil {
		t.Fatal("expected an error for a non-empty existing directory")
	}
}

func TestRunInit_ScaffoldsIntoCwd(t *testing.T) {
	dir := t.TempDir()
	wi := nonInteractiveWizard("go:fiber", false, "widgets")

	result, err := runInit(dir, wi, &bytes.Buffer{}, &bytes.Buffer{}, false)
	if err != nil {
		t.Fatalf("runInit: %v", err)
	}
	if result.ProjectDir != dir {
		t.Errorf("expected project dir %s, got %s", dir, result.ProjectDir)
	}
	if _, err := os.Stat(filepath.Join(dir, "acthur.yml")); err != nil {
		t.Errorf("expected acthur.yml in cwd: %v", err)
	}
}

func TestRunInit_RefusesExistingActhurYML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "acthur.yml"), []byte("project: existing\nversion: \"1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := runInit(dir, nonInteractiveWizard("go:fiber", false, "widgets"), &bytes.Buffer{}, &bytes.Buffer{}, false)
	if err == nil {
		t.Fatal("expected an error when acthur.yml already exists in cwd")
	}
}
