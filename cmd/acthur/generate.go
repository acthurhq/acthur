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
	gengofiber "github.com/acthur/acthur/internal/generate/gofiber"
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
		files = renumberMigrations(root, files)
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
		files = renumberMigrations(root, files)
		results, err := generate.WriteFiles(root, node.ID, files)
		if err != nil {
			return nil, err
		}
		all = append(all, results...)
	}
	return all, nil
}

var migrationName = regexp.MustCompile(`^(\d{4})_(.+)\.(up|down)\.sql$`)

// renumberMigrations rewrites the pipeline's fixed 0400 migration numbers to
// real ones: a migration whose description already exists in the project's
// migrations dir reuses its number (regeneration must not burn versions);
// a new description takes the next free number — golang-migrate rejects
// duplicate versions, so two contracts can never share one.
func renumberMigrations(root string, files []plugin.GeneratedFile) []plugin.GeneratedFile {
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
		for n := 400; n <= 499; n++ {
			if !used[n] {
				used[n] = true
				return n
			}
		}
		return 500 // range exhausted; still unique, surfaced by review
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
				num = n
			} else {
				num = nextFree()
			}
			assigned[desc] = num
		}
		out[i].Path = fmt.Sprintf("migrations/%04d_%s.%s.sql", num, desc, m[3])
	}
	return out
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
