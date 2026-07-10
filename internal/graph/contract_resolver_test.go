package graph_test

import (
	"testing"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/graph"
)

// ---------------------------------------------------------------------------
// Phase 4 Slice 1 — contract-loadability rule (issue #36)
//
// The graph engine never imports internal/contract (ADR 0005). It is
// injected a graph.ContractResolver — the same abstraction pattern as
// graph.Resolver for adapters — assembled at the CLI entrypoint.
// ---------------------------------------------------------------------------

// fakeContractResolver is a map-backed test helper implementing
// graph.ContractResolver: name -> expected file path, loadable or not.
type fakeContractResolver struct {
	loadable map[string]string // name -> path, present means loadable
	paths    map[string]string // name -> expected path, for names that are NOT loadable
}

func (f fakeContractResolver) Resolve(name string) (string, bool) {
	if path, ok := f.loadable[name]; ok {
		return path, true
	}
	if path, ok := f.paths[name]; ok {
		return path, false
	}
	return "", false
}

func dataFlowGraph(t *testing.T, contracts []string) *graph.Graph {
	t.Helper()
	nodes := map[string]*graph.Node{
		"web": {ID: "web", Type: config.NodeTypeService, Adapter: "ui:astro"},
		"api": {ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber"},
	}
	g := graph.NewTestGraph(nodes)
	g.AddEdge(&graph.Edge{From: "web", To: "api", Type: config.EdgeDataFlow, Contracts: contracts})
	return g
}

// Behavior 1: a data_flow edge naming a contract with no loadable file
// produces a ValidationError whose message includes the expected path.
func TestValidate_ContractNotLoadable_ProducesErrorWithExpectedPath(t *testing.T) {
	g := dataFlowGraph(t, []string{"users"})
	cr := fakeContractResolver{
		paths: map[string]string{"users": "contracts/users.contract.yml"},
	}

	errs := g.Validate(graph.EmptyResolver{}, cr)

	var found *graph.ValidationError
	for i := range errs {
		if errs[i].Rule == "unloadable-contract" {
			found = &errs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected ValidationError with Rule=unloadable-contract, got: %v", errs)
	}
	if found.Severity != graph.SeverityError {
		t.Errorf("expected SeverityError, got %q", found.Severity)
	}
	if !containsSubstr(found.Message, "contracts/users.contract.yml") {
		t.Errorf("expected message to include expected path, got: %q", found.Message)
	}
}

// Behavior 2: a loadable contract produces no unloadable-contract error.
func TestValidate_ContractLoadable_NoError(t *testing.T) {
	g := dataFlowGraph(t, []string{"users"})
	cr := fakeContractResolver{
		loadable: map[string]string{"users": "contracts/users.contract.yml"},
	}

	errs := g.Validate(graph.EmptyResolver{}, cr)

	for _, e := range errs {
		if e.Rule == "unloadable-contract" {
			t.Fatalf("expected no unloadable-contract error, got: %v", e)
		}
	}
}

// Behavior 3: no ContractResolver supplied — the check is skipped entirely
// (backward compatible with existing g.Validate(resolver) call sites).
func TestValidate_NoContractResolverSupplied_SkipsCheck(t *testing.T) {
	g := dataFlowGraph(t, []string{"users"})

	errs := g.Validate(graph.EmptyResolver{})

	for _, e := range errs {
		if e.Rule == "unloadable-contract" {
			t.Fatalf("expected check to be skipped without a ContractResolver, got: %v", e)
		}
	}
}

// Behavior 4: multiple contracts on one edge — only the unloadable one errors.
func TestValidate_MultipleContracts_OnlyUnloadableOneErrors(t *testing.T) {
	g := dataFlowGraph(t, []string{"users", "appointments"})
	cr := fakeContractResolver{
		loadable: map[string]string{"users": "contracts/users.contract.yml"},
		paths:    map[string]string{"appointments": "contracts/appointments.contract.yml"},
	}

	errs := g.Validate(graph.EmptyResolver{}, cr)

	count := 0
	for _, e := range errs {
		if e.Rule == "unloadable-contract" {
			count++
			if !containsSubstr(e.Message, "appointments") {
				t.Errorf("expected error to name 'appointments', got: %q", e.Message)
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 unloadable-contract error, got %d: %v", count, errs)
	}
}
