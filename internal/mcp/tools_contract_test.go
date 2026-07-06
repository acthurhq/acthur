package mcp

import (
	"encoding/json"
	"testing"

	"github.com/acthur/acthur/internal/contract"
)

func testRegistry(t *testing.T) *contract.Registry {
	t.Helper()
	reg := contract.NewRegistry()
	c1 := &contract.Contract{
		Name: "users", Version: "v1", Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{{ID: "get_user", Method: "GET", Path: "/users/:id"}},
	}
	c2 := &contract.Contract{
		Name: "users", Version: "v2", Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{ID: "get_user", Method: "GET", Path: "/users/:id"},
			{ID: "list_users", Method: "GET", Path: "/users"},
		},
	}
	if err := reg.Register(c1); err != nil {
		t.Fatalf("register c1: %v", err)
	}
	if err := reg.Register(c2); err != nil {
		t.Fatalf("register c2: %v", err)
	}
	return reg
}

func TestContractListTool(t *testing.T) {
	tool := NewContractListTool(testRegistry(t))
	result, err := tool.Handler(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	list := result.([]contractSummary)
	if len(list) != 2 {
		t.Fatalf("expected 2 contract versions, got %d", len(list))
	}
	if list[0].Name != "users" || list[0].Version != "v1" {
		t.Fatalf("unexpected first entry: %+v", list[0])
	}
	if list[1].Endpoints != 2 {
		t.Fatalf("expected v2 to have 2 endpoints, got %d", list[1].Endpoints)
	}
}

func TestContractShowTool_LatestVersion(t *testing.T) {
	tool := NewContractShowTool(testRegistry(t))
	result, err := tool.Handler(json.RawMessage(`{"name":"users"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c := result.(*contract.Contract)
	if c.Version != "v2" {
		t.Fatalf("expected latest version v2, got %s", c.Version)
	}
}

func TestContractShowTool_ExactVersion(t *testing.T) {
	tool := NewContractShowTool(testRegistry(t))
	result, err := tool.Handler(json.RawMessage(`{"name":"users","version":"v1"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c := result.(*contract.Contract)
	if len(c.Endpoints) != 1 {
		t.Fatalf("expected v1 to have 1 endpoint, got %d", len(c.Endpoints))
	}
}

func TestContractShowTool_MissingName(t *testing.T) {
	tool := NewContractShowTool(testRegistry(t))
	_, err := tool.Handler(json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestContractShowTool_NotFound(t *testing.T) {
	tool := NewContractShowTool(testRegistry(t))
	_, err := tool.Handler(json.RawMessage(`{"name":"nope"}`))
	if err == nil {
		t.Fatal("expected error for unknown contract")
	}
}
