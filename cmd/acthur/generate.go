package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/contract"
	"github.com/acthur/acthur/internal/generate"
	"github.com/acthur/acthur/internal/generate/aicontext"
	genci "github.com/acthur/acthur/internal/generate/ci"
	"github.com/acthur/acthur/internal/generate/docsgen"
	gengofiber "github.com/acthur/acthur/internal/generate/gofiber"
	"github.com/acthur/acthur/internal/generate/skillgen"
	"github.com/acthur/acthur/internal/generate/visualize"
	"github.com/acthur/acthur/internal/graph"
	"github.com/acthur/acthur/internal/output"
	"github.com/acthur/acthur/internal/plugin"
	"github.com/acthur/acthur/internal/scaffold"
)

// ---------------------------------------------------------------------------
// acthur generate from-contract / model — Phase 7 Slice 3.
// Resolves the project + target nodes the same way acthur add does, runs the
// go:fiber pipeline, renumbers contract-range migrations to the next free
// slot, and persists through the write engine (generated.lock semantics).
// ---------------------------------------------------------------------------

// loadProjectGraph loads acthur.yml at root and builds (without plugins —
// generation needs the graph shape only, and plugin loading stays a
// bootstrap concern).
func loadProjectGraph(root string) (*config.Config, *graph.Graph, error) {
	ymlPath, err := config.Find(root)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := config.LoadFile(ymlPath)
	if err != nil {
		return nil, nil, err
	}
	g, err := graph.Build(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("building graph: %w", err)
	}
	return cfg, g, nil
}

// runGenerateFromContract generates the full go:fiber stack for one contract
// against every go:fiber service node (or --node).
func runGenerateFromContract(root, contractArg, nodeFlag string) ([]generate.Result, error) {
	cfg, g, err := loadProjectGraph(root)
	if err != nil {
		return nil, err
	}

	path := contractFilePath(root, strings.TrimSuffix(contractArg, ".contract.yml"))
	c, err := contract.ParseFile(path)
	if err != nil {
		return nil, err
	}

	targets, err := resolveTargetNodes(g, nodeFlag)
	if err != nil {
		return nil, err
	}

	var all []generate.Result
	for _, node := range targets {
		ctx := plugin.GeneratorContext{
			ProjectName: cfg.Project,
			NodeID:      node.ID,
			RootDir:     root,
			Extra: map[string]any{
				"module_path": scaffold.ResolveScaffoldContext(*cfg, node.ID).ModulePath,
			},
		}
		files, err := gengofiber.Generate(c, ctx)
		if err != nil {
			return nil, fmt.Errorf("generating from %s for node %q: %w", filepath.Base(path), node.ID, err)
		}
		files, err = renumberMigrations(root, files)
		if err != nil {
			return nil, err
		}
		results, err := generate.WriteFiles(root, node.ID, files)
		if err != nil {
			return nil, err
		}
		all = append(all, results...)
	}
	return all, nil
}

// runGenerateModel generates a model + migration + test from
// `acthur generate model <Name> <field:type>...` arguments.
func runGenerateModel(root, name string, fieldArgs []string, nodeFlag string) ([]generate.Result, error) {
	cfg, g, err := loadProjectGraph(root)
	if err != nil {
		return nil, err
	}

	fields := make(map[string]string, len(fieldArgs))
	for _, arg := range fieldArgs {
		parts := strings.SplitN(arg, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid field %q — expected <name>:<type>, e.g. name:string(required)", arg)
		}
		fields[parts[0]] = parts[1]
	}

	targets, err := resolveTargetNodes(g, nodeFlag)
	if err != nil {
		return nil, err
	}

	var all []generate.Result
	for _, node := range targets {
		ctx := plugin.GeneratorContext{
			ProjectName: cfg.Project,
			NodeID:      node.ID,
			RootDir:     root,
			Extra: map[string]any{
				"module_path": scaffold.ResolveScaffoldContext(*cfg, node.ID).ModulePath,
			},
		}
		files, err := gengofiber.GenerateModel(name, fields, ctx)
		if err != nil {
			return nil, err
		}
		files, err = renumberMigrations(root, files)
		if err != nil {
			return nil, err
		}
		results, err := generate.WriteFiles(root, node.ID, files)
		if err != nil {
			return nil, err
		}
		all = append(all, results...)
	}
	return all, nil
}

var migrationName = regexp.MustCompile(`^(\d{4})_(.+)\.(up|down)\.sql$`)

// contractMigrationRangeStart/End are the reserved version-number band for
// contract-generated migrations (acthur-prd.md Phase 7); plugin migrations
// (e.g. auth) reserve their own bands below it (e.g. 0100-0199).
const (
	contractMigrationRangeStart = 400
	contractMigrationRangeEnd   = 499
)

// renumberMigrations rewrites the pipeline's fixed 0400 migration numbers to
// real ones: a migration whose description already exists in the project's
// migrations dir *within the contract range* reuses its number
// (regeneration must not burn versions); a new description takes the next
// free number in the range — golang-migrate rejects duplicate versions, so
// two contracts can never share one. A description that collides with an
// existing migration *outside* the contract range (e.g. a plugin's reserved
// band) is a hard error: reusing that number would silently overwrite the
// plugin's migration content on disk via generated.lock.
func renumberMigrations(root string, files []plugin.GeneratedFile) ([]plugin.GeneratedFile, error) {
	existing := map[string]int{} // description → number
	used := map[int]bool{}
	entries, _ := os.ReadDir(filepath.Join(root, "migrations"))
	for _, e := range entries {
		if m := migrationName.FindStringSubmatch(e.Name()); m != nil {
			n, _ := strconv.Atoi(m[1])
			existing[m[2]] = n
			used[n] = true
		}
	}

	assigned := map[string]int{}
	nextFree := func() int {
		for n := contractMigrationRangeStart; n <= contractMigrationRangeEnd; n++ {
			if !used[n] {
				used[n] = true
				return n
			}
		}
		return contractMigrationRangeEnd + 1 // range exhausted; still unique, surfaced by review
	}

	out := make([]plugin.GeneratedFile, len(files))
	for i, f := range files {
		out[i] = f
		base := filepath.Base(f.Path)
		m := migrationName.FindStringSubmatch(base)
		if !strings.HasPrefix(f.Path, "migrations/") || m == nil {
			continue
		}
		desc := m[2]
		num, ok := assigned[desc]
		if !ok {
			if n, exists := existing[desc]; exists {
				if n < contractMigrationRangeStart || n > contractMigrationRangeEnd {
					return nil, fmt.Errorf(
						"contract migration %q collides with existing migration %04d_%s outside the contract range (%d-%d) — rename the contract field/model to avoid reusing a plugin-reserved migration",
						desc, n, desc, contractMigrationRangeStart, contractMigrationRangeEnd,
					)
				}
				num = n
			} else {
				num = nextFree()
			}
			assigned[desc] = num
		}
		out[i].Path = fmt.Sprintf("migrations/%04d_%s.%s.sql", num, desc, m[3])
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// acthur generate ai-context / ci / docs / skill — Phase 9.
// All four share the same shape: load cfg+graph (+contracts where needed),
// run a pure internal/generate/<surface> renderer, persist the result(s)
// through the write engine.
// ---------------------------------------------------------------------------

// runGenerateAIContext generates the AI coding-tool context file for tool
// (defaults to "claude" if empty) from cfg + graph + the project's contracts.
func runGenerateAIContext(root, tool string) ([]generate.Result, error) {
	cfg, g, err := loadProjectGraph(root)
	if err != nil {
		return nil, err
	}
	reg, err := contract.LoadDir(root)
	if err != nil {
		return nil, err
	}
	file, err := aicontext.Generate(cfg, g, reg, tool)
	if err != nil {
		return nil, err
	}
	return generate.WriteFiles(root, "", []plugin.GeneratedFile{file})
}

// runGenerateCI generates the CI/CD pipeline config for target (defaults to
// "github-actions" if empty).
func runGenerateCI(root, target string) ([]generate.Result, error) {
	cfg, _, err := loadProjectGraph(root)
	if err != nil {
		return nil, err
	}
	file, err := genci.Generate(cfg, target)
	if err != nil {
		return nil, err
	}
	return generate.WriteFiles(root, "", []plugin.GeneratedFile{file})
}

// runGenerateDocs generates per-contract API reference pages plus an index
// from the project's contract registry. target must be "markdown" (the
// default) or empty — acthur-prd.md §36 describes additional doc-site
// targets (Astro, Nextra, README, OpenAPI) that are not yet implemented;
// asking for one of those fails clearly rather than silently emitting
// markdown under a different name.
func runGenerateDocs(root, target string) ([]generate.Result, error) {
	if target != "" && target != "markdown" {
		return nil, fmt.Errorf(
			"unsupported docs target %q — supported: markdown (nextra/readme/openapi are tracked but not yet implemented)",
			target,
		)
	}
	if _, _, err := loadProjectGraph(root); err != nil {
		return nil, err
	}
	reg, err := contract.LoadDir(root)
	if err != nil {
		return nil, err
	}
	files, err := docsgen.Generate(reg)
	if err != nil {
		return nil, err
	}
	return generate.WriteFiles(root, "", files)
}

// runGenerateSkill generates a .claude/skills/<name>/SKILL.md scaffold
// derived from cfg + graph.
func runGenerateSkill(root, name string) ([]generate.Result, error) {
	cfg, g, err := loadProjectGraph(root)
	if err != nil {
		return nil, err
	}
	file, err := skillgen.Generate(cfg, g, name)
	if err != nil {
		return nil, err
	}
	return generate.WriteFiles(root, "", []plugin.GeneratedFile{file})
}

// runGraphVisualize renders the project graph as a Mermaid or DOT diagram.
func runGraphVisualize(root, format string) (string, error) {
	_, g, err := loadProjectGraph(root)
	if err != nil {
		return "", err
	}
	return visualize.Render(g, format)
}

// printGenerateResults reports what the write engine did per file.
func printGenerateResults(results []generate.Result) {
	for _, r := range results {
		switch r.Status {
		case generate.StatusWritten:
			output.Success(output.PrefixKernel, "%-10s %s (%s)", "written", r.Path, r.NodeID)
		case generate.StatusMerged:
			output.Info(output.PrefixKernel, "%-10s %s (%s)", "merged", r.Path, r.NodeID)
		case generate.StatusSkipped:
			msg := r.Warning
			if msg == "" {
				msg = "already exists"
			}
			output.Warn(output.PrefixKernel, "%-10s %s (%s) — %s", "skipped", r.Path, r.NodeID, msg)
		}
	}
}
