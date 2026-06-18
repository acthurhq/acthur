package contract_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/acthur/acthur/internal/contract"
)

// ---------------------------------------------------------------------------
// Parser tests
// ---------------------------------------------------------------------------

func TestParseFile_ValidContract(t *testing.T) {
	path := writeContract(t, `
contract: users
version: "1"
transport: http
endpoints:
  - id: create_user
    method: POST
    path: /api/v1/users
    auth: required
    input:
      name: string(required,max:100)
      email: email(required)
    output:
      id: ulid
      name: string
      created_at: timestamp
events:
  - id: user.created
    payload:
      id: ulid
types:
  User:
    fields:
      id: ulid
      name: string
`)
	c, err := contract.ParseFile(path)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if c.Name != "users" {
		t.Errorf("expected name=users, got %q", c.Name)
	}
	if c.Version != "1" {
		t.Errorf("expected version=1, got %q", c.Version)
	}
	if c.Transport != contract.TransportHTTP {
		t.Errorf("expected transport=http, got %q", c.Transport)
	}
	if len(c.Endpoints) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(c.Endpoints))
	}
}

func TestParseFile_DefaultVersion(t *testing.T) {
	path := writeContract(t, `
contract: appointments
transport: http
endpoints: []
`)
	c, err := contract.ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Version != "1" {
		t.Errorf("expected default version=1, got %q", c.Version)
	}
}

func TestParseFile_DefaultTransport(t *testing.T) {
	path := writeContract(t, `
contract: pets
version: "1"
endpoints: []
`)
	c, err := contract.ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Transport != contract.TransportHTTP {
		t.Errorf("expected default transport=http, got %q", c.Transport)
	}
}

func TestParseFile_MissingContractName(t *testing.T) {
	path := writeContract(t, `
version: "1"
transport: http
endpoints: []
`)
	_, err := contract.ParseFile(path)
	if err == nil {
		t.Error("expected error for missing contract name, got nil")
	}
}

func TestParseFile_FileNotFound(t *testing.T) {
	_, err := contract.ParseFile("/nonexistent/path/contract.yml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestParseFile_ChecksumSet(t *testing.T) {
	path := writeContract(t, `
contract: vets
version: "1"
endpoints: []
`)
	c, err := contract.ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Checksum == "" {
		t.Error("expected checksum to be set")
	}
}

func TestParseFile_FilePathSet(t *testing.T) {
	path := writeContract(t, `
contract: storage
version: "1"
endpoints: []
`)
	c, err := contract.ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.FilePath != path {
		t.Errorf("expected FilePath=%q, got %q", path, c.FilePath)
	}
}

func TestParseFile_MultipleEndpoints(t *testing.T) {
	path := writeContract(t, `
contract: appointments
version: "1"
transport: http
endpoints:
  - id: create
    method: POST
    path: /api/v1/appointments
  - id: get
    method: GET
    path: /api/v1/appointments/:id
  - id: list
    method: GET
    path: /api/v1/appointments
  - id: delete
    method: DELETE
    path: /api/v1/appointments/:id
`)
	c, err := contract.ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.Endpoints) != 4 {
		t.Errorf("expected 4 endpoints, got %d", len(c.Endpoints))
	}
}

// ---------------------------------------------------------------------------
// Validate tests
// ---------------------------------------------------------------------------

func TestValidate_Valid(t *testing.T) {
	c := &contract.Contract{
		Name:      "users",
		Version:   "1",
		Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{ID: "create_user", Method: "POST", Path: "/api/v1/users"},
		},
	}
	errs := c.Validate()
	if len(errs) != 0 {
		t.Errorf("expected no validation errors, got %d: %v", len(errs), errs)
	}
}

func TestValidate_MissingName(t *testing.T) {
	c := &contract.Contract{
		Version:   "1",
		Transport: contract.TransportHTTP,
	}
	errs := c.Validate()
	if len(errs) == 0 {
		t.Error("expected validation error for missing name")
	}
}

func TestValidate_EndpointMissingID(t *testing.T) {
	c := &contract.Contract{
		Name:      "test",
		Version:   "1",
		Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{Method: "POST", Path: "/test"}, // no ID
		},
	}
	errs := c.Validate()
	if len(errs) == 0 {
		t.Error("expected validation error for endpoint missing ID")
	}
}

func TestValidate_EndpointMissingMethodForHTTP(t *testing.T) {
	c := &contract.Contract{
		Name:      "test",
		Version:   "1",
		Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{ID: "do_thing", Path: "/test"}, // no method
		},
	}
	errs := c.Validate()
	if len(errs) == 0 {
		t.Error("expected validation error for HTTP endpoint missing method")
	}
}

func TestValidate_GRPCEndpointNoMethodRequired(t *testing.T) {
	c := &contract.Contract{
		Name:      "payments",
		Version:   "1",
		Transport: contract.TransportGRPC,
		Endpoints: []contract.Endpoint{
			{ID: "CreatePayment"}, // gRPC doesn't need HTTP method
		},
	}
	errs := c.Validate()
	if len(errs) != 0 {
		t.Errorf("expected no errors for gRPC contract without method, got: %v", errs)
	}
}

// ---------------------------------------------------------------------------
// Registry tests
// ---------------------------------------------------------------------------

func TestRegistry_Register(t *testing.T) {
	reg := contract.NewRegistry()
	c := &contract.Contract{
		Name:    "users",
		Version: "1",
		Transport: contract.TransportHTTP,
	}
	if err := reg.Register(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRegistry_RegisterFile(t *testing.T) {
	reg := contract.NewRegistry()
	path := writeContract(t, `
contract: appointments
version: "1"
transport: http
endpoints:
  - id: create
    method: POST
    path: /api/v1/appointments
`)
	c, err := reg.RegisterFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Name != "appointments" {
		t.Errorf("expected name=appointments, got %q", c.Name)
	}
}

func TestRegistry_Get_Registered(t *testing.T) {
	reg := contract.NewRegistry()
	reg.Register(&contract.Contract{Name: "users", Version: "1", Transport: contract.TransportHTTP})

	c, err := reg.Get("users", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Name != "users" {
		t.Errorf("expected name=users, got %q", c.Name)
	}
}

func TestRegistry_Get_NotRegistered(t *testing.T) {
	reg := contract.NewRegistry()
	_, err := reg.Get("nonexistent", "1")
	if err == nil {
		t.Error("expected error for unregistered contract, got nil")
	}
}

func TestRegistry_GetLatest(t *testing.T) {
	reg := contract.NewRegistry()
	reg.Register(&contract.Contract{Name: "users", Version: "1", Transport: contract.TransportHTTP, Checksum: "a"})
	reg.Register(&contract.Contract{Name: "users", Version: "2", Transport: contract.TransportHTTP, Checksum: "b"})

	c, err := reg.GetLatest("users")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Version != "2" {
		t.Errorf("expected latest version=2, got %q", c.Version)
	}
}

func TestRegistry_IdempotentRegister(t *testing.T) {
	reg := contract.NewRegistry()
	c := &contract.Contract{Name: "users", Version: "1",
		Transport: contract.TransportHTTP, Checksum: "abc123"}

	// Register twice with same checksum — should not error
	if err := reg.Register(c); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if err := reg.Register(c); err != nil {
		t.Errorf("second register with same checksum should be idempotent, got: %v", err)
	}
}

func TestRegistry_DuplicateVersionDifferentChecksum(t *testing.T) {
	reg := contract.NewRegistry()
	reg.Register(&contract.Contract{Name: "users", Version: "1",
		Transport: contract.TransportHTTP, Checksum: "aaa"})

	err := reg.Register(&contract.Contract{Name: "users", Version: "1",
		Transport: contract.TransportHTTP, Checksum: "bbb"})
	if err == nil {
		t.Error("expected error for duplicate version with different checksum, got nil")
	}
}

func TestRegistry_All(t *testing.T) {
	reg := contract.NewRegistry()
	reg.Register(&contract.Contract{Name: "users",        Version: "1", Transport: contract.TransportHTTP, Checksum: "a"})
	reg.Register(&contract.Contract{Name: "appointments", Version: "1", Transport: contract.TransportHTTP, Checksum: "b"})
	reg.Register(&contract.Contract{Name: "vets",         Version: "1", Transport: contract.TransportHTTP, Checksum: "c"})

	all := reg.All()
	if len(all) != 3 {
		t.Errorf("expected 3 contracts, got %d", len(all))
	}
}

func TestRegistry_Versions(t *testing.T) {
	reg := contract.NewRegistry()
	reg.Register(&contract.Contract{Name: "users", Version: "1", Transport: contract.TransportHTTP, Checksum: "a"})
	reg.Register(&contract.Contract{Name: "users", Version: "2", Transport: contract.TransportHTTP, Checksum: "b"})
	reg.Register(&contract.Contract{Name: "users", Version: "3", Transport: contract.TransportHTTP, Checksum: "c"})

	versions := reg.Versions("users")
	if len(versions) != 3 {
		t.Errorf("expected 3 versions, got %d", len(versions))
	}
}

// ---------------------------------------------------------------------------
// Diff tests
// ---------------------------------------------------------------------------

func TestDiff_NoChanges(t *testing.T) {
	c := &contract.Contract{
		Name:      "users",
		Version:   "1",
		Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{
				ID: "get_user", Method: "GET", Path: "/api/v1/users/:id",
				Output: map[string]string{"id": "ulid", "name": "string"},
			},
		},
	}
	result := contract.Diff(c, c)
	if result.HasBreaking {
		t.Error("expected no breaking changes when comparing identical contracts")
	}
}

func TestDiff_RemovedEndpoint_IsBreaking(t *testing.T) {
	old := &contract.Contract{
		Name: "users", Version: "1", Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{ID: "get_user", Method: "GET", Path: "/api/v1/users/:id"},
			{ID: "list_users", Method: "GET", Path: "/api/v1/users"},
		},
	}
	next := &contract.Contract{
		Name: "users", Version: "2", Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{ID: "get_user", Method: "GET", Path: "/api/v1/users/:id"},
			// list_users removed
		},
	}
	result := contract.Diff(old, next)
	if !result.HasBreaking {
		t.Error("expected breaking change when endpoint is removed")
	}
	assertHasChange(t, result.Changes, "list_users", contract.ChangeBreaking)
}

func TestDiff_AddedEndpoint_IsNonBreaking(t *testing.T) {
	old := &contract.Contract{
		Name: "users", Version: "1", Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{ID: "get_user", Method: "GET", Path: "/api/v1/users/:id"},
		},
	}
	next := &contract.Contract{
		Name: "users", Version: "2", Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{ID: "get_user",   Method: "GET", Path: "/api/v1/users/:id"},
			{ID: "list_users", Method: "GET", Path: "/api/v1/users"},
		},
	}
	result := contract.Diff(old, next)
	if result.HasBreaking {
		t.Error("expected no breaking changes when endpoint is added")
	}
}

func TestDiff_MethodChange_IsBreaking(t *testing.T) {
	old := &contract.Contract{
		Name: "users", Version: "1", Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{ID: "update_user", Method: "PUT", Path: "/api/v1/users/:id"},
		},
	}
	next := &contract.Contract{
		Name: "users", Version: "2", Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{ID: "update_user", Method: "PATCH", Path: "/api/v1/users/:id"},
		},
	}
	result := contract.Diff(old, next)
	if !result.HasBreaking {
		t.Error("expected breaking change when HTTP method changes")
	}
}

func TestDiff_PathChange_IsBreaking(t *testing.T) {
	old := &contract.Contract{
		Name: "users", Version: "1", Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{ID: "get_user", Method: "GET", Path: "/api/v1/users/:id"},
		},
	}
	next := &contract.Contract{
		Name: "users", Version: "2", Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{ID: "get_user", Method: "GET", Path: "/api/v2/users/:id"},
		},
	}
	result := contract.Diff(old, next)
	if !result.HasBreaking {
		t.Error("expected breaking change when path changes")
	}
}

func TestDiff_RemovedOutputField_IsBreaking(t *testing.T) {
	old := &contract.Contract{
		Name: "users", Version: "1", Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{
				ID: "get_user", Method: "GET", Path: "/api/v1/users/:id",
				Output: map[string]string{"id": "ulid", "name": "string", "email": "email"},
			},
		},
	}
	next := &contract.Contract{
		Name: "users", Version: "2", Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{
				ID: "get_user", Method: "GET", Path: "/api/v1/users/:id",
				Output: map[string]string{"id": "ulid", "name": "string"}, // email removed
			},
		},
	}
	result := contract.Diff(old, next)
	if !result.HasBreaking {
		t.Error("expected breaking change when output field is removed")
	}
}

func TestDiff_OptionalOutputFieldAdded_IsNonBreaking(t *testing.T) {
	old := &contract.Contract{
		Name: "users", Version: "1", Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{
				ID: "get_user", Method: "GET", Path: "/api/v1/users/:id",
				Output: map[string]string{"id": "ulid", "name": "string"},
			},
		},
	}
	next := &contract.Contract{
		Name: "users", Version: "2", Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{
				ID: "get_user", Method: "GET", Path: "/api/v1/users/:id",
				Output: map[string]string{
					"id":         "ulid",
					"name":       "string",
					"avatar_url": "url?", // optional — non-breaking
				},
			},
		},
	}
	result := contract.Diff(old, next)
	if result.HasBreaking {
		t.Error("expected no breaking change when optional output field added")
	}
}

func TestDiff_RequiredInputFieldAdded_IsBreaking(t *testing.T) {
	old := &contract.Contract{
		Name: "users", Version: "1", Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{
				ID: "create_user", Method: "POST", Path: "/api/v1/users",
				Input: map[string]string{"name": "string(required)", "email": "email(required)"},
			},
		},
	}
	next := &contract.Contract{
		Name: "users", Version: "2", Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{
				ID: "create_user", Method: "POST", Path: "/api/v1/users",
				Input: map[string]string{
					"name":  "string(required)",
					"email": "email(required)",
					"role":  "string(required)", // new required field — breaking
				},
			},
		},
	}
	result := contract.Diff(old, next)
	if !result.HasBreaking {
		t.Error("expected breaking change when required input field added")
	}
}

func TestDiff_TransportChange_IsBreaking(t *testing.T) {
	old := &contract.Contract{Name: "svc", Version: "1", Transport: contract.TransportHTTP}
	next := &contract.Contract{Name: "svc", Version: "2", Transport: contract.TransportGRPC}
	result := contract.Diff(old, next)
	if !result.HasBreaking {
		t.Error("expected breaking change when transport changes")
	}
}

func TestDiff_AuthAdded_IsBreaking(t *testing.T) {
	old := &contract.Contract{
		Name: "public", Version: "1", Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{ID: "get_data", Method: "GET", Path: "/api/v1/data", Auth: "none"},
		},
	}
	next := &contract.Contract{
		Name: "public", Version: "2", Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{ID: "get_data", Method: "GET", Path: "/api/v1/data", Auth: "required"},
		},
	}
	result := contract.Diff(old, next)
	if !result.HasBreaking {
		t.Error("expected breaking change when auth becomes required")
	}
}

// ---------------------------------------------------------------------------
// Validator tests
// ---------------------------------------------------------------------------

func TestValidator_ValidRequest(t *testing.T) {
	reg := contract.NewRegistry()
	reg.Register(&contract.Contract{
		Name:      "users",
		Version:   "1",
		Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{
				ID: "create_user", Method: "POST", Path: "/api/v1/users",
				Auth:  "required",
				Input: map[string]string{"name": "string(required)", "email": "email(required)"},
			},
		},
	})

	v := contract.NewValidator(reg, contract.SeverityWarn)
	result := v.ValidateRequest("users", "1", "create_user",
		map[string]string{"Authorization": "Bearer token123"},
		map[string]any{"name": "Alice", "email": "alice@example.com"},
	)

	if !result.Valid {
		t.Errorf("expected valid request, got violations: %v", result.Violations)
	}
}

func TestValidator_MissingRequiredField(t *testing.T) {
	reg := contract.NewRegistry()
	reg.Register(&contract.Contract{
		Name:      "users",
		Version:   "1",
		Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{
				ID:    "create_user",
				Method: "POST",
				Path:  "/api/v1/users",
				Input: map[string]string{"name": "string(required)", "email": "email(required)"},
			},
		},
	})

	v := contract.NewValidator(reg, contract.SeverityWarn)
	result := v.ValidateRequest("users", "1", "create_user",
		map[string]string{},
		map[string]any{"name": "Alice"}, // email missing
	)

	if result.Valid {
		t.Error("expected invalid request for missing required field")
	}
	if len(result.Violations) == 0 {
		t.Error("expected at least one violation")
	}
}

func TestValidator_MissingAuthHeader(t *testing.T) {
	reg := contract.NewRegistry()
	reg.Register(&contract.Contract{
		Name:      "users",
		Version:   "1",
		Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{
			{ID: "get_user", Method: "GET", Path: "/api/v1/users/:id", Auth: "required"},
		},
	})

	v := contract.NewValidator(reg, contract.SeverityWarn)
	result := v.ValidateRequest("users", "1", "get_user",
		map[string]string{}, // no Authorization header
		map[string]any{},
	)

	if result.Valid {
		t.Error("expected invalid request when auth required but header missing")
	}
}

func TestValidator_UnknownContract(t *testing.T) {
	reg := contract.NewRegistry()
	v := contract.NewValidator(reg, contract.SeverityWarn)
	result := v.ValidateRequest("nonexistent", "1", "endpoint", nil, nil)
	if result.Valid {
		t.Error("expected invalid result for unknown contract")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeContract(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.contract.yml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write contract: %v", err)
	}
	return path
}

func assertHasChange(t *testing.T, changes []contract.Change, field string, ct contract.ChangeType) {
	t.Helper()
	for _, c := range changes {
		if c.Type == ct && containsStr(c.Field, field) {
			return
		}
	}
	t.Errorf("expected a %q change involving %q, got changes: %v", ct, field, changes)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
