package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acthurhq/acthur/internal/aiagent"
)

// fakeProvider is an injectable aiagent.Provider — no real network call.
type fakeProvider struct {
	gotSystem string
	gotUser   string
	response  string
	err       error
}

func (f *fakeProvider) Complete(_ context.Context, system, user string) (string, error) {
	f.gotSystem = system
	f.gotUser = user
	if f.err != nil {
		return "", f.err
	}
	return f.response, nil
}

func TestRunAgentExplain_SendsQuestionWithGraphContext(t *testing.T) {
	dir := writeMCPFixtureProject(t)
	provider := &fakeProvider{response: "here's the explanation"}
	var out bytes.Buffer

	err := runAgentExplain(agentDeps{root: dir, out: &out, provider: provider}, "what does api do?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(provider.gotUser, "what does api do?") {
		t.Fatalf("user prompt missing question: %q", provider.gotUser)
	}
	if !strings.Contains(provider.gotSystem, "api") || !strings.Contains(provider.gotSystem, "mcp-fixture") {
		t.Fatalf("system prompt missing graph context: %q", provider.gotSystem)
	}
	if strings.TrimSpace(out.String()) != "here's the explanation" {
		t.Fatalf("got output %q", out.String())
	}
}

func TestRunAgentGenerate_DescribesPlanOnlyScope(t *testing.T) {
	dir := writeMCPFixtureProject(t)
	provider := &fakeProvider{response: "plan text"}
	var out bytes.Buffer

	err := runAgentGenerate(agentDeps{root: dir, out: &out, provider: provider}, "add a webhook endpoint")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(provider.gotSystem, "does not write files") {
		t.Fatalf("expected system prompt to scope this to a plan, not a write: %q", provider.gotSystem)
	}
	if !strings.Contains(provider.gotUser, "add a webhook endpoint") {
		t.Fatalf("user prompt missing feature description: %q", provider.gotUser)
	}
}

func TestRunAgentDiagnose_SendsProblem(t *testing.T) {
	dir := writeMCPFixtureProject(t)
	provider := &fakeProvider{response: "diagnosis"}
	var out bytes.Buffer

	err := runAgentDiagnose(agentDeps{root: dir, out: &out, provider: provider}, "api returns 500")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(provider.gotUser, "api returns 500") {
		t.Fatalf("user prompt missing problem description: %q", provider.gotUser)
	}
}

func TestRunAgentReview_EmptyDiffSkipsProviderCall(t *testing.T) {
	dir := writeMCPFixtureProject(t)
	provider := &fakeProvider{response: "should not be called"}
	var out bytes.Buffer

	called := false
	err := runAgentReview(agentDeps{
		root: dir, out: &out, provider: provider,
		gitDiff: func(string) (string, error) {
			called = true
			return "", nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called == false {
		t.Fatal("expected gitDiff to be invoked")
	}
	if provider.gotUser != "" {
		t.Fatal("expected provider not to be called for an empty diff")
	}
	if !strings.Contains(out.String(), "No changes to review") {
		t.Fatalf("expected empty-diff message, got %q", out.String())
	}
}

func TestRunAgentReview_NonEmptyDiffSendsToProvider(t *testing.T) {
	dir := writeMCPFixtureProject(t)
	provider := &fakeProvider{response: "review feedback"}
	var out bytes.Buffer

	err := runAgentReview(agentDeps{
		root: dir, out: &out, provider: provider,
		gitDiff: func(string) (string, error) {
			return "diff --git a/foo.go b/foo.go\n+added line", nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(provider.gotUser, "added line") {
		t.Fatalf("user prompt missing diff content: %q", provider.gotUser)
	}
	if strings.TrimSpace(out.String()) != "review feedback" {
		t.Fatalf("got output %q", out.String())
	}
}

func TestRunAgentReview_GitDiffErrorPropagates(t *testing.T) {
	dir := writeMCPFixtureProject(t)
	provider := &fakeProvider{}
	var out bytes.Buffer

	err := runAgentReview(agentDeps{
		root: dir, out: &out, provider: provider,
		gitDiff: func(string) (string, error) { return "", fmt.Errorf("not a git repository") },
	})
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestRunAgentDocument_FileContent(t *testing.T) {
	dir := writeMCPFixtureProject(t)
	provider := &fakeProvider{response: "docs"}
	var out bytes.Buffer

	err := runAgentDocument(agentDeps{root: dir, out: &out, provider: provider}, "acthur.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(provider.gotUser, "mcp-fixture") {
		t.Fatalf("user prompt missing file content: %q", provider.gotUser)
	}
}

func TestRunAgentDocument_DirectoryListing(t *testing.T) {
	dir := writeMCPFixtureProject(t)
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	provider := &fakeProvider{response: "docs"}
	var out bytes.Buffer

	err := runAgentDocument(agentDeps{root: dir, out: &out, provider: provider}, "sub")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(provider.gotUser, "file.txt") {
		t.Fatalf("user prompt missing directory listing: %q", provider.gotUser)
	}
}

func TestRunAgentDocument_MissingPathErrors(t *testing.T) {
	dir := writeMCPFixtureProject(t)
	provider := &fakeProvider{}
	var out bytes.Buffer

	err := runAgentDocument(agentDeps{root: dir, out: &out, provider: provider}, "nope.txt")
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestRunAgentTask_NoProviderConfiguredIsPointedError(t *testing.T) {
	dir := writeMCPFixtureProject(t)
	var out bytes.Buffer

	err := runAgentExplain(agentDeps{root: dir, out: &out}, "anything")
	if err == nil {
		t.Fatal("expected error when no ai: block is configured")
	}
	if err != aiagent.ErrProviderNotConfigured {
		t.Fatalf("got %v, want ErrProviderNotConfigured", err)
	}
}

func TestRunAgentTask_ProviderErrorPropagates(t *testing.T) {
	dir := writeMCPFixtureProject(t)
	provider := &fakeProvider{err: fmt.Errorf("network down")}
	var out bytes.Buffer

	err := runAgentExplain(agentDeps{root: dir, out: &out, provider: provider}, "anything")
	if err == nil {
		t.Fatal("expected error to propagate from provider")
	}
	if !strings.Contains(err.Error(), "network down") {
		t.Fatalf("got %v, want it to mention network down", err)
	}
}
