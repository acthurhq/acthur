package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/generate"
	"github.com/acthur/acthur/internal/graph"
	"github.com/acthur/acthur/internal/plugin"
	"github.com/acthur/acthur/internal/scaffold"
)

// ---------------------------------------------------------------------------
// acthur add <plugin> — installs a plugin and runs its generator.
//
// Sequence:
//  1. validate the plugin exists in the registry (plugin.Resolve already
//     returns the pointed "run: acthur add <name>" error shape).
//  2. append it to acthur.yml's plugins: list, idempotently.
//  3. reload config + rebuild the graph from the now-updated file.
//  4. load the plugin (only the plugins not already loaded this process —
//     see ensurePluginLoaded — so an already-bootstrapped process never
//     double-registers an existing plugin's hooks).
//  5. run its generator against every go:fiber service node (or --node).
//  6. write the returned files, honoring Overwrite/MergeMarker.
// ---------------------------------------------------------------------------

// addFileStatus classifies what happened to one generated file on disk.
// It is an alias of generate.Status — the write engine (internal/generate)
// now owns writing and generated.lock bookkeeping; acthur add just reports
// the same statuses it always has.
type addFileStatus = generate.Status

const (
	addFileWritten = generate.StatusWritten
	addFileSkipped = generate.StatusSkipped
	addFileMerged  = generate.StatusMerged
)

// addResult is one file outcome, reported back for the command's summary.
type addResult struct {
	NodeID string
	Path   string
	Status addFileStatus
}

// addSummary is the overall result of runAdd, used to print the CLI summary.
type addSummary struct {
	Plugin     string
	AlreadyHad bool // plugin was already in acthur.yml's plugins: list
	Results    []addResult
}

func (s *addSummary) record(nodeID, path string, status addFileStatus) {
	s.Results = append(s.Results, addResult{NodeID: nodeID, Path: path, Status: status})
}

// runAdd is the full acthur add implementation, factored out of addCmd.RunE
// so it can be driven directly from tests against a temp project directory.
func runAdd(root, pluginName, nodeFlag string) (*addSummary, error) {
	if _, err := plugin.Resolve(pluginName); err != nil {
		return nil, err
	}

	ymlPath, err := config.Find(root)
	if err != nil {
		return nil, err
	}

	original, err := os.ReadFile(ymlPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", ymlPath, err)
	}

	alreadyHad, err := addPluginToYAML(ymlPath, pluginName)
	if err != nil {
		return nil, fmt.Errorf("updating %s: %w", ymlPath, err)
	}
	// A failed add must not leave the plugin behind in acthur.yml — a
	// half-added entry wedges every later command at plugin load.
	rollback := func() {
		if !alreadyHad {
			_ = os.WriteFile(ymlPath, original, 0o644)
		}
	}

	cfg, err := config.LoadFile(ymlPath)
	if err != nil {
		rollback()
		return nil, fmt.Errorf("reloading %s: %w", ymlPath, err)
	}

	g, err := graph.Build(cfg)
	if err != nil {
		rollback()
		return nil, fmt.Errorf("building graph: %w", err)
	}

	k, err := ensurePluginLoaded(cfg, g)
	if err != nil {
		rollback()
		return nil, err
	}

	gen, ok := k.Generator(pluginName)
	if !ok {
		rollback()
		return nil, fmt.Errorf(
			"plugin %q registered no generator named %q — nothing to generate",
			pluginName, pluginName,
		)
	}

	targets, err := resolveTargetNodes(g, nodeFlag)
	if err != nil {
		rollback()
		return nil, err
	}

	summary := &addSummary{Plugin: pluginName, AlreadyHad: alreadyHad}
	pluginCfg := pluginConfigFor(cfg, pluginName)

	for _, node := range targets {
		ctx := plugin.GeneratorContext{
			ProjectName: cfg.Project,
			NodeID:      node.ID,
			RootDir:     root,
			Config:      pluginCfg,
			Extra: map[string]any{
				"module_path": scaffold.ResolveScaffoldContext(*cfg, node.ID).ModulePath,
			},
		}

		files, err := gen.Generate(node.Adapter, ctx)
		if err != nil {
			rollback()
			return nil, fmt.Errorf("generating %q for node %q: %w", pluginName, node.ID, err)
		}

		results, err := generate.WriteFiles(root, node.ID, files)
		if err != nil {
			return nil, fmt.Errorf("writing generated files for node %q: %w", node.ID, err)
		}
		for _, r := range results {
			summary.record(r.NodeID, r.Path, r.Status)
		}
	}

	return summary, nil
}

// ensurePluginLoaded loads every plugin in cfg.Plugins that has not already
// been loaded in this process (tracked by the loadedPlugins/kernelAPI
// globals populated at CLI bootstrap — see commands.go), and returns the
// KernelAPI whose generator registry holds every loaded plugin's generator.
//
// Loading only the delta is what keeps `acthur add` from double-registering
// hooks/commands for plugins bootstrapPlugins already loaded against the
// pre-edit acthur.yml (the same "once per process" rule loadGraph enforces
// for the dev/build/etc. commands).
func ensurePluginLoaded(cfg *config.Config, g *graph.Graph) (*plugin.KernelAPIImpl, error) {
	already := make(map[string]bool, len(loadedPlugins))
	for _, lp := range loadedPlugins {
		already[lp.Plugin.Name()] = true
	}

	var delta []string
	for _, p := range cfg.Plugins {
		if !already[p.Name] {
			delta = append(delta, p.Name)
		}
	}

	if len(delta) == 0 {
		if kernelAPI == nil {
			return nil, fmt.Errorf("internal error: no plugins to load but no KernelAPI is available")
		}
		g.Freeze()
		return kernelAPI, nil
	}

	if err := checkPluginsKnown(delta); err != nil {
		return nil, err
	}

	satisfied := make([]string, 0, len(loadedPlugins))
	for _, lp := range loadedPlugins {
		satisfied = append(satisfied, lp.Plugin.Name())
	}

	k := plugin.NewKernelAPI(kernelBus, g, registerPluginCommand, pluginLog)
	loaded, err := plugin.LoadDelta(delta, satisfied, kernelBus, k)
	if err != nil {
		return nil, err
	}

	kernelAPI = k
	loadedPlugins = append(loadedPlugins, loaded...)
	g.Freeze()
	return k, nil
}

// resolveTargetNodes returns the node(s) to run the generator against: the
// single node named by nodeFlag (validated as go:fiber), or every go:fiber
// service node in the graph when nodeFlag is empty.
func resolveTargetNodes(g *graph.Graph, nodeFlag string) ([]*graph.Node, error) {
	if nodeFlag != "" {
		n := g.Node(nodeFlag)
		if n == nil {
			return nil, fmt.Errorf("node %q not found in the graph", nodeFlag)
		}
		if n.Adapter != "go:fiber" {
			return nil, fmt.Errorf(
				"node %q uses adapter %q — acthur add currently generates for go:fiber nodes only",
				nodeFlag, n.Adapter,
			)
		}
		return []*graph.Node{n}, nil
	}

	var targets []*graph.Node
	for _, n := range g.NodesByType(config.NodeTypeService) {
		if n.Adapter == "go:fiber" {
			targets = append(targets, n)
		}
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].ID < targets[j].ID })

	if len(targets) == 0 {
		return nil, fmt.Errorf("no go:fiber service nodes found in the graph — use --node to target a specific node")
	}
	return targets, nil
}

// pluginConfigFor returns the `config:` map for the named plugin entry in
// cfg.Plugins, or nil if it has none.
func pluginConfigFor(cfg *config.Config, name string) map[string]any {
	for _, p := range cfg.Plugins {
		if p.Name == name {
			return p.Config
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// acthur.yml plugins: list editor
//
// There is no comment-preserving YAML writer in internal/config (a plain
// yaml.Marshal of the whole Config would drop every comment in the file —
// confirmed by inspecting internal/config/config.go, which has no Save/
// Write function at all). So this is a deliberately narrow, line-based
// insertion under the `plugins:` key: it never touches any other part of
// the file, which preserves comments and formatting everywhere else. This
// is the "documented rewrite" the Phase 6 PRD allows in place of full
// comment-preserving round-tripping.
// ---------------------------------------------------------------------------

// addPluginToYAML appends "  - name: <name>" under acthur.yml's plugins:
// key at path, creating the key if absent. Returns true (and does nothing)
// if the plugin is already listed — the operation is idempotent.
func addPluginToYAML(path, name string) (alreadyPresent bool, err error) {
	cfg, err := config.LoadFile(path)
	if err != nil {
		return false, err
	}
	for _, p := range cfg.Plugins {
		if p.Name == name {
			return true, nil
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	text := string(data)
	entry := fmt.Sprintf("  - name: %s", name)

	lines := strings.Split(text, "\n")
	pluginsLine := -1
	for i, l := range lines {
		if strings.TrimRight(l, " \t") == "plugins:" {
			pluginsLine = i
			break
		}
	}

	var updated string
	if pluginsLine == -1 {
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		updated = text + "plugins:\n" + entry + "\n"
	} else {
		insertAt := pluginsLine + 1
		for insertAt < len(lines) && isYAMLListItemLine(lines[insertAt]) {
			insertAt++
		}
		newLines := make([]string, 0, len(lines)+1)
		newLines = append(newLines, lines[:insertAt]...)
		newLines = append(newLines, entry)
		newLines = append(newLines, lines[insertAt:]...)
		updated = strings.Join(newLines, "\n")
	}

	return false, os.WriteFile(path, []byte(updated), 0o644)
}

// isYAMLListItemLine reports whether l looks like a continuation of a
// `plugins:` list entry (an indented "- name: …" line or one of its nested
// "config:" fields) — used to find the insertion point after the last
// existing entry.
func isYAMLListItemLine(l string) bool {
	return strings.HasPrefix(l, "  -") || strings.HasPrefix(l, "    ")
}

// Generated-file writing (honoring GeneratedFile.Overwrite/MergeMarker) and
// generated.lock bookkeeping now live in internal/generate — see
// generate.WriteFiles, called from runAdd above.
