package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/acthurhq/acthur/internal/aiagent"
)

// agentDeps holds `acthur agent`'s seams: root directory, output stream,
// and two injectable dependencies (provider, gitDiff) so tests never call
// a real LLM API or shell out to git. Provider is resolved from acthur.yml's
// `ai:` block when nil (the real CLI path); tests set it directly.
type agentDeps struct {
	root     string
	out      io.Writer
	provider aiagent.Provider
	gitDiff  func(root string) (string, error)
}

// runAgentTask is the shared path every `acthur agent` subcommand goes
// through: load the project's graph/contract context, resolve (or reuse an
// injected) AI provider, send a system+user prompt, print the response.
// A provider resolution failure (no `ai:` block, missing API key, or an
// unimplemented provider) is a pointed error — never a silent no-op.
func runAgentTask(d agentDeps, systemSuffix, userPrompt string) error {
	cfg, g, reg, err := loadMCPContext(d.root)
	if err != nil {
		return err
	}

	provider := d.provider
	if provider == nil {
		provider, err = aiagent.ResolveProvider(cfg.AI)
		if err != nil {
			return err
		}
	}

	systemPrompt := aiagent.BuildContext(cfg, g, reg)
	if systemSuffix != "" {
		systemPrompt += "\n\n" + systemSuffix
	}

	text, err := provider.Complete(context.Background(), systemPrompt, userPrompt)
	if err != nil {
		return fmt.Errorf("agent request failed: %w", err)
	}
	_, _ = fmt.Fprintln(d.out, text)
	return nil
}

// runAgentExplain answers a question about the system using full graph +
// contract context (PRD §17.6 `acthur agent explain`).
func runAgentExplain(d agentDeps, question string) error {
	return runAgentTask(d,
		"Task: explain the relevant part of this system to the operator, referencing the graph/contract context above.",
		"Explain: "+question)
}

// runAgentGenerate proposes an implementation plan for a feature or
// endpoint, using the project's real adapters/contracts/nodes as context.
// It deliberately does not write files itself — an LLM writing files
// unsupervised is not something this slice can honestly guarantee is safe;
// it prints a plan for the operator to apply. See
// docs/implementation/active/0007-prd-completion.md for this scoping note.
func runAgentGenerate(d agentDeps, feature string) error {
	return runAgentTask(d,
		"Task: propose a concrete implementation plan for a new feature or endpoint, referencing "+
			"the existing adapters/contracts/graph nodes above so the plan fits the project's conventions. "+
			"This command only proposes a plan — it does not write files itself.",
		"Generate a plan for: "+feature)
}

// runAgentDiagnose diagnoses a described runtime problem using graph +
// contract context (PRD §17.6 `acthur agent diagnose`).
func runAgentDiagnose(d agentDeps, problem string) error {
	return runAgentTask(d,
		"Task: diagnose the described runtime problem using the graph/contract context above — "+
			"name likely root causes and where in the graph to look.",
		"Diagnose: "+problem)
}

// runAgentReview reviews the working tree's current `git diff` (PRD §17.6
// `acthur agent review`). An empty diff is reported directly without
// spending a model call.
func runAgentReview(d agentDeps) error {
	diffFn := d.gitDiff
	if diffFn == nil {
		diffFn = defaultGitDiff
	}
	diff, err := diffFn(d.root)
	if err != nil {
		return fmt.Errorf("failed to read git diff: %w", err)
	}
	if strings.TrimSpace(diff) == "" {
		_, _ = fmt.Fprintln(d.out, "No changes to review (git diff is empty).")
		return nil
	}
	return runAgentTask(d,
		"Task: review the following git diff for bugs, risks, and consistency with the project's "+
			"contracts and graph structure above. Be specific about file and line where possible.",
		"Review this diff:\n\n"+diff)
}

// defaultGitDiff runs the real `git diff` in root. CombinedOutput (not
// Output) surfaces git's own error text (e.g. "not a git repository") in
// the returned error rather than swallowing it to stderr.
func defaultGitDiff(root string) (string, error) {
	cmd := exec.Command("git", "-C", root, "diff")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// runAgentDocument generates documentation for a file or directory (PRD
// §17.6 `acthur agent document`).
func runAgentDocument(d agentDeps, path string) error {
	content, err := readPathForDocument(d.root, path)
	if err != nil {
		return err
	}
	return runAgentTask(d,
		"Task: generate clear documentation (purpose, usage, key functions/exports) for the given path's content.",
		fmt.Sprintf("Document %s:\n\n%s", path, content))
}

// maxDocumentBytes caps how much of a file's content is sent as prompt
// context — large enough for any single source file this repo generates,
// small enough to never blow past a reasonable prompt size by accident.
const maxDocumentBytes = 64 * 1024

// readPathForDocument resolves path (relative to root, or absolute) and
// returns either its file contents (truncated to maxDocumentBytes) or,
// for a directory, a plain listing of its immediate entries.
func readPathForDocument(root, path string) (string, error) {
	full := path
	if !filepath.IsAbs(path) {
		full = filepath.Join(root, path)
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", fmt.Errorf("path %q not found: %w", path, err)
	}
	if info.IsDir() {
		entries, err := os.ReadDir(full)
		if err != nil {
			return "", fmt.Errorf("failed to read directory %q: %w", path, err)
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return "Directory listing:\n" + strings.Join(names, "\n"), nil
	}

	data, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("failed to read %q: %w", path, err)
	}
	if len(data) > maxDocumentBytes {
		data = data[:maxDocumentBytes]
	}
	return string(data), nil
}
