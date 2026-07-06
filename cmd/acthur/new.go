package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/acthur/acthur/internal/adapter"
	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/output"
	"github.com/acthur/acthur/internal/scaffold"
	"github.com/mattn/go-isatty"
)

// ---------------------------------------------------------------------------
// acthur new / acthur init — shared scaffolding implementation.
//
// `acthur new <project>` creates <project>/ under cwd and scaffolds a
// complete project into it. `acthur init` does the same into cwd itself,
// refusing to touch a directory that already has an acthur.yml. Both share
// the same wizard (adapter, database y/n, module prefix) and the same
// scaffoldProject writer.
//
// The wizard prompts over an injectable io.Reader/io.Writer so tests never
// touch a real terminal; --adapter/--db/--module let every answer be
// supplied up front, which is required whenever stdin is not a TTY (CI,
// scripts, tests).
// ---------------------------------------------------------------------------

// newOptions holds the resolved answers for scaffolding a new project,
// whether they came from flags or the interactive wizard.
type newOptions struct {
	Adapter string
	DB      bool
	Module  string
}

// wizardInput carries the raw --adapter/--db/--module flag state so
// resolveNewOptions knows which answers still need to come from the
// interactive prompt (a bool flag has no "unset" zero value, so callers
// track *Set via cmd.Flags().Changed(...)).
type wizardInput struct {
	Adapter    string
	AdapterSet bool
	DB         bool
	DBSet      bool
	Module     string
	ModuleSet  bool
}

// scaffoldResult reports what runNew/runInit did, for the CLI summary and
// for tests to assert against.
type scaffoldResult struct {
	ProjectDir   string
	Adapter      string
	DB           bool
	ModulePrefix string
	Files        []string // paths written, relative to ProjectDir, sorted
	GitInit      bool
}

// isInteractive reports whether stdin looks like a real terminal. When it
// doesn't (CI, piped input, tests), the wizard refuses to prompt and
// requires every answer via flags instead of hanging on a read that will
// never be satisfied.
func isInteractive() bool {
	fd := os.Stdin.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// backendScaffoldAdapters returns the names of every registered backend
// adapter that can scaffold a project, sorted.
func backendScaffoldAdapters() []string {
	var names []string
	for _, a := range adapter.ByCategory(adapter.CategoryBackend) {
		if _, ok := a.(adapter.Scaffolder); ok {
			names = append(names, a.Name())
		}
	}
	sort.Strings(names)
	return names
}

// defaultBackendAdapter returns go:fiber when it can scaffold (the canonical
// default — stable as new adapters register), else the first (alphabetically)
// registered backend adapter that can, or "" if none is registered.
func defaultBackendAdapter() string {
	names := backendScaffoldAdapters()
	for _, n := range names {
		if n == "go:fiber" {
			return n
		}
	}
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// resolveNewOptions returns the fully-resolved newOptions for scaffolding.
// Any answer already supplied via flags (tracked in wi) is used as-is;
// every other answer is prompted for over in/out — unless interactive is
// false, in which case a missing answer is a pointed error rather than a
// prompt that would hang forever on a non-TTY stdin.
func resolveNewOptions(in io.Reader, out io.Writer, projectName string, wi wizardInput, interactive bool) (newOptions, error) {
	opts := newOptions{Adapter: wi.Adapter, DB: wi.DB, Module: wi.Module}

	if (!wi.AdapterSet || !wi.DBSet || !wi.ModuleSet) && !interactive {
		var missing []string
		if !wi.AdapterSet {
			missing = append(missing, "--adapter")
		}
		if !wi.DBSet {
			missing = append(missing, "--db")
		}
		if !wi.ModuleSet {
			missing = append(missing, "--module")
		}
		return opts, fmt.Errorf(
			"no terminal detected — pass %s to run non-interactively",
			strings.Join(missing, ", "),
		)
	}

	reader := bufio.NewReader(in)

	if !wi.AdapterSet {
		choices := backendScaffoldAdapters()
		def := defaultBackendAdapter()
		fmt.Fprintf(out, "Which backend adapter? (%s) [%s]: ", strings.Join(choices, ", "), def)
		if line := readLine(reader); line != "" {
			opts.Adapter = line
		} else {
			opts.Adapter = def
		}
	}
	if opts.Adapter == "" {
		opts.Adapter = defaultBackendAdapter()
	}

	a, err := adapter.Resolve(opts.Adapter)
	if err != nil {
		return opts, fmt.Errorf("unknown adapter %q: %w", opts.Adapter, err)
	}
	if _, ok := a.(adapter.Scaffolder); !ok {
		return opts, fmt.Errorf("adapter %q cannot scaffold a project (no Scaffold capability)", opts.Adapter)
	}

	if !wi.DBSet {
		fmt.Fprint(out, "Include a database (db:postgres)? [Y/n]: ")
		line := strings.ToLower(readLine(reader))
		opts.DB = line == "" || line == "y" || line == "yes"
	}

	if !wi.ModuleSet {
		fmt.Fprintf(out, "Module prefix [%s]: ", projectName)
		if line := readLine(reader); line != "" {
			opts.Module = line
		} else {
			opts.Module = projectName
		}
	}
	if opts.Module == "" {
		opts.Module = projectName
	}

	return opts, nil
}

// readLine reads one line from r and returns it trimmed. Returns "" on EOF
// or a blank line — both mean "use the default" to callers.
func readLine(r *bufio.Reader) string {
	line, _ := r.ReadString('\n')
	return strings.TrimSpace(line)
}

// runNew implements `acthur new <project-name>`: creates project-name/ under
// cwd — which must not already exist as a non-empty directory — and
// scaffolds a complete project into it.
func runNew(cwd, projectName string, wi wizardInput, in io.Reader, out io.Writer, interactive bool) (*scaffoldResult, error) {
	dir := filepath.Join(cwd, projectName)
	if info, err := os.Stat(dir); err == nil {
		if !info.IsDir() {
			return nil, fmt.Errorf("%s already exists and is not a directory", dir)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", dir, err)
		}
		if len(entries) > 0 {
			return nil, fmt.Errorf("directory %s already exists and is not empty", dir)
		}
	}

	opts, err := resolveNewOptions(in, out, projectName, wi, interactive)
	if err != nil {
		return nil, err
	}

	return scaffoldProject(dir, projectName, opts)
}

// runInit implements `acthur init`: scaffolds into cwd itself instead of a
// new subdirectory. scaffoldProject already refuses to overwrite an
// existing acthur.yml, which is exactly the non-destructive guarantee
// `acthur init` needs.
func runInit(cwd string, wi wizardInput, in io.Reader, out io.Writer, interactive bool) (*scaffoldResult, error) {
	projectName := filepath.Base(cwd)
	opts, err := resolveNewOptions(in, out, projectName, wi, interactive)
	if err != nil {
		return nil, err
	}
	return scaffoldProject(cwd, projectName, opts)
}

// scaffoldProject writes acthur.yml, scaffolds the chosen adapter's files
// under dir/api/, optionally declares a db:postgres infra node, and runs
// `git init`. It refuses to run if dir already has an acthur.yml — the
// shared guard behind both `acthur new`'s fresh directory and `acthur
// init`'s non-destructive-adoption guarantee.
func scaffoldProject(dir, projectName string, opts newOptions) (*scaffoldResult, error) {
	ymlPath := filepath.Join(dir, "acthur.yml")
	if _, err := os.Stat(ymlPath); err == nil {
		return nil, fmt.Errorf("%s already exists — acthur new/init refuses to overwrite an existing project", ymlPath)
	}

	a, err := adapter.Resolve(opts.Adapter)
	if err != nil {
		return nil, fmt.Errorf("unknown adapter %q: %w", opts.Adapter, err)
	}
	scaffolder, ok := a.(adapter.Scaffolder)
	if !ok {
		return nil, fmt.Errorf("adapter %q cannot scaffold a project (no Scaffold capability)", opts.Adapter)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating %s: %w", dir, err)
	}

	yml := renderProjectYAML(projectName, opts)
	if err := os.WriteFile(ymlPath, []byte(yml), 0o644); err != nil {
		return nil, fmt.Errorf("writing %s: %w", ymlPath, err)
	}

	cfg, err := config.LoadFile(ymlPath)
	if err != nil {
		return nil, fmt.Errorf("generated acthur.yml is invalid: %w", err)
	}

	sctx := scaffold.ResolveScaffoldContext(*cfg, "api")
	files, err := scaffolder.Scaffold(sctx)
	if err != nil {
		return nil, fmt.Errorf("scaffolding %q: %w", opts.Adapter, err)
	}

	written := make([]string, 0, len(files))
	for _, f := range files {
		rel := filepath.Join("api", f.Path)
		target := filepath.Join(dir, rel)
		mode := os.FileMode(f.Mode)
		if mode == 0 {
			mode = 0o644
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, fmt.Errorf("creating directory for %s: %w", rel, err)
		}
		if err := os.WriteFile(target, f.Content, mode); err != nil {
			return nil, fmt.Errorf("writing %s: %w", rel, err)
		}
		written = append(written, rel)
	}
	sort.Strings(written)

	gitInit := runGitInit(dir) == nil

	return &scaffoldResult{
		ProjectDir:   dir,
		Adapter:      opts.Adapter,
		DB:           opts.DB,
		ModulePrefix: opts.Module,
		Files:        written,
		GitInit:      gitInit,
	}, nil
}

// renderProjectYAML renders a minimal, valid acthur.yml for a freshly
// scaffolded project: one "api" service node using opts.Adapter, an
// optional "db" infra node using db:postgres, and the edges that satisfy
// graph validation's no-orphan-node rule — every node must have at least
// one edge, including the always-present kernel "proxy" node, so "api" is
// always wired to it via proxied_through.
func renderProjectYAML(projectName string, opts newOptions) string {
	var b strings.Builder
	fmt.Fprintf(&b, "project: %s\n", projectName)
	b.WriteString("version: \"1\"\n")
	fmt.Fprintf(&b, "module_prefix: %s\n\n", opts.Module)
	b.WriteString("graph:\n")
	b.WriteString("  nodes:\n")
	b.WriteString("    api:\n")
	b.WriteString("      type: service\n")
	fmt.Fprintf(&b, "      adapter: %s\n", opts.Adapter)
	b.WriteString("      port: 8080\n")
	b.WriteString("      hot_reload: true\n")
	if opts.DB {
		b.WriteString("    db:\n")
		b.WriteString("      type: infra\n")
		b.WriteString("      adapter: db:postgres\n")
	}
	b.WriteString("  edges:\n")
	if opts.DB {
		b.WriteString("    - from: api\n")
		b.WriteString("      to: db\n")
		b.WriteString("      type: depends_on\n\n")
	}
	b.WriteString("    - from: api\n")
	b.WriteString("      to: proxy\n")
	b.WriteString("      type: proxied_through\n")
	return b.String()
}

// runGitInit initializes a git repository at dir, unless one already
// exists there. Best-effort: git is a core doctor requirement, but a
// missing/failing git must not make scaffolding itself fail — the caller
// reports success via scaffoldResult.GitInit instead.
func runGitInit(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return nil
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

// printScaffoldResult prints the CLI summary for a successful `acthur new`
// or `acthur init` run.
func printScaffoldResult(r *scaffoldResult) {
	output.Success(output.PrefixKernel, "scaffolded project in %s", r.ProjectDir)
	output.Info(output.PrefixKernel, "adapter: %s", r.Adapter)
	if r.DB {
		output.Info(output.PrefixKernel, "database: db:postgres")
	}
	if r.GitInit {
		output.Info(output.PrefixKernel, "git repository initialized")
	}

	output.Header("Next steps")
	fmt.Printf("  cd %s\n", r.ProjectDir)
	fmt.Println("  acthur dev")
}
