package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/graph"
)

// ---------------------------------------------------------------------------
// acthur plugin remove <name> — inverse of `acthur add <plugin>`.
//
// Mirrors add.go's YAML-edit-with-rollback pattern: edit acthur.yml's
// plugins: list, reload + rebuild the graph to confirm the result is still
// valid, and restore the original bytes on any failure after the edit.
// Unlike `acthur add`, this never touches generated files on disk — a
// plugin's generated code is left in place; removing it from acthur.yml
// only stops it from being loaded on the next run.
// ---------------------------------------------------------------------------

// pluginRemoveSummary is the result of runPluginRemove, used to print the
// CLI summary.
type pluginRemoveSummary struct {
	Plugin string
}

// runPluginRemove removes pluginName from acthur.yml's plugins: list under
// root. Returns a pointed error naming every currently-installed plugin if
// pluginName isn't present — mirroring checkPluginsKnown's error shape in
// commands.go.
func runPluginRemove(root, pluginName string) (*pluginRemoveSummary, error) {
	ymlPath, err := config.Find(root)
	if err != nil {
		return nil, err
	}

	cfg, err := config.LoadFile(ymlPath)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", ymlPath, err)
	}

	present := false
	installed := make([]string, 0, len(cfg.Plugins))
	for _, p := range cfg.Plugins {
		installed = append(installed, p.Name)
		if p.Name == pluginName {
			present = true
		}
	}
	if !present {
		if len(installed) == 0 {
			return nil, fmt.Errorf("plugin %q is not installed — acthur.yml has no plugins listed", pluginName)
		}
		return nil, fmt.Errorf(
			"plugin %q is not installed — acthur.yml lists: %s",
			pluginName, strings.Join(installed, ", "),
		)
	}

	original, err := os.ReadFile(ymlPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", ymlPath, err)
	}

	removed, err := removePluginFromYAML(ymlPath, pluginName)
	if err != nil {
		return nil, fmt.Errorf("updating %s: %w", ymlPath, err)
	}
	if !removed {
		// Should not happen — cfg.Plugins already confirmed pluginName is
		// present above — but never leave the caller without an error.
		return nil, fmt.Errorf("plugin %q not found in acthur.yml's plugins list", pluginName)
	}

	rollback := func() { _ = os.WriteFile(ymlPath, original, 0o644) }

	newCfg, err := config.LoadFile(ymlPath)
	if err != nil {
		rollback()
		return nil, fmt.Errorf("reloading %s: %w", ymlPath, err)
	}
	if _, err := graph.Build(newCfg); err != nil {
		rollback()
		return nil, fmt.Errorf("building graph: %w", err)
	}

	return &pluginRemoveSummary{Plugin: pluginName}, nil
}

// removePluginFromYAML deletes the "  - name: <name>" entry (and every
// nested field belonging to it, e.g. its "    config:" block) from
// acthur.yml's plugins: list at path. Returns false if no such entry was
// found — the caller should treat that as "nothing to do" (checked against
// cfg.Plugins beforehand, so in practice this only guards against a
// concurrent edit).
//
// Like addPluginToYAML in add.go, this is a deliberately narrow line-based
// edit rather than a full yaml.Marshal round-trip — there is no
// comment-preserving YAML writer in internal/config, so a full remarshal
// would drop every comment in the file.
func removePluginFromYAML(path, name string) (found bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	lines := strings.Split(string(data), "\n")

	entryLine := "  - name: " + name
	start := -1
	for i, l := range lines {
		if strings.TrimRight(l, " \t") == entryLine {
			start = i
			break
		}
	}
	if start == -1 {
		return false, nil
	}

	// Consume every line belonging to this entry: blank separator lines and
	// nested fields (indented 4+ spaces, e.g. "    config:" and its
	// children). Stop at the next list entry ("  - name: ..."), a line
	// dedented out of the plugins: block, or EOF.
	end := start + 1
	for end < len(lines) {
		l := lines[end]
		if strings.TrimSpace(l) == "" {
			end++
			continue
		}
		if strings.HasPrefix(l, "    ") {
			end++
			continue
		}
		break
	}

	newLines := make([]string, 0, len(lines)-(end-start))
	newLines = append(newLines, lines[:start]...)
	newLines = append(newLines, lines[end:]...)
	updated := strings.Join(newLines, "\n")
	// Removing the trailing entry in the file can otherwise swallow the
	// original trailing newline (the split's final "" element is itself a
	// continuation line) — always leave the file newline-terminated,
	// mirroring addInfraNodeToYAML's same fix in service.go.
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}

	return true, os.WriteFile(path, []byte(updated), 0o644)
}
