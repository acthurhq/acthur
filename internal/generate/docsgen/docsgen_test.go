package docsgen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acthur/acthur/internal/contract"
	"github.com/acthur/acthur/internal/generate/docsgen"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up")
		}
		dir = parent
	}
}

func usersRegistry(t *testing.T) *contract.Registry {
	t.Helper()
	reg := contract.NewRegistry()
	path := filepath.Join(repoRoot(t), "testdata", "vetangle", "contracts", "users.contract.yml")
	if _, err := reg.RegisterFile(path); err != nil {
		t.Fatalf("RegisterFile: %v", err)
	}
	return reg
}

func TestGenerate_WritesIndexAndPerContractPage(t *testing.T) {
	files, err := docsgen.Generate(usersRegistry(t))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var index, usersPage *string
	for i := range files {
		f := files[i]
		content := string(f.Content)
		switch f.Path {
		case "docs/api/README.md":
			index = &content
		case "docs/api/users.md":
			usersPage = &content
		}
	}
	if index == nil {
		t.Fatal("expected docs/api/README.md")
	}
	if !strings.Contains(*index, "users") {
		t.Errorf("expected index to reference the users contract, got:\n%s", *index)
	}

	if usersPage == nil {
		t.Fatal("expected docs/api/users.md")
	}
	page := *usersPage
	for _, want := range []string{
		"# users", "POST /api/v1/users", "create_user",
		"| email |", "`email`", "409", "email_taken",
		"## Types", "### User", "user.created", "## Events",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("expected users.md to contain %q, got:\n%s", want, page)
		}
	}
}

func TestGenerate_EmptyRegistry_ProducesIndexOnly(t *testing.T) {
	files, err := docsgen.Generate(contract.NewRegistry())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(files) != 1 || files[0].Path != "docs/api/README.md" {
		t.Fatalf("expected only the index for an empty registry, got %+v", files)
	}
}

func TestGenerate_NilRegistry_Errors(t *testing.T) {
	if _, err := docsgen.Generate(nil); err == nil {
		t.Fatal("expected error for nil registry")
	}
}

func TestGenerate_Deterministic(t *testing.T) {
	reg := usersRegistry(t)
	f1, err := docsgen.Generate(reg)
	if err != nil {
		t.Fatal(err)
	}
	f2, err := docsgen.Generate(reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(f1) != len(f2) {
		t.Fatalf("expected same file count, got %d vs %d", len(f1), len(f2))
	}
	for i := range f1 {
		if f1[i].Path != f2[i].Path || string(f1[i].Content) != string(f2[i].Content) {
			t.Errorf("expected deterministic output at index %d", i)
		}
	}
}
