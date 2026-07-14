package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/graph"
)

func TestProductionResolver_ReachesEveryRepoAdapter(t *testing.T) {
	defined := repoAdapterNames(t)
	resolver := registryResolver{}

	var missing []string
	for _, name := range defined {
		if _, ok := resolver.Resolve(name); !ok {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		t.Fatalf("production resolver is missing repo adapter(s): %v; reachable adapters: %v", missing, resolver.Names())
	}
}

func TestProductionResolver_ValidatesPhase2AdaptersInVetangle(t *testing.T) {
	root := repoRoot(t)
	cfg, err := config.LoadFile(filepath.Join(root, "testdata", "vetangle", "acthur.yml"))
	if err != nil {
		t.Fatalf("load vetangle fixture: %v", err)
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("build vetangle graph: %v", err)
	}

	errs := g.Validate(registryResolver{})
	unresolved := unresolvedAdapterNodes(errs)
	for _, nodeID := range []string{"api", "worker", "db"} {
		if unresolved[nodeID] {
			t.Fatalf("expected production resolver to resolve Phase 2 adapter for vetangle node %q; validation errors: %v", nodeID, errs)
		}
	}
}

func repoAdapterNames(t *testing.T) []string {
	t.Helper()
	root := repoRoot(t)
	adapterDir := filepath.Join(root, "internal", "adapter")
	names := map[string]bool{}

	err := filepath.WalkDir(adapterDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "templates" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		for _, name := range adapterNamesInFile(t, path) {
			names[name] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk adapter packages: %v", err)
	}

	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func unresolvedAdapterNodes(errs []graph.ValidationError) map[string]bool {
	nodes := map[string]bool{}
	for _, err := range errs {
		if err.Rule == "unresolved-adapter" {
			nodes[err.Node] = true
		}
	}
	return nodes
}

func adapterNamesInFile(t *testing.T, path string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	var names []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name.Name != "Name" || fn.Body == nil {
			continue
		}
		for _, stmt := range fn.Body.List {
			ret, ok := stmt.(*ast.ReturnStmt)
			if !ok || len(ret.Results) != 1 {
				continue
			}
			lit, ok := ret.Results[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("unquote adapter name in %s: %v", path, err)
			}
			if strings.Contains(value, ":") {
				names = append(names, value)
			}
		}
	}
	return names
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root containing go.mod")
		}
		dir = parent
	}
}
