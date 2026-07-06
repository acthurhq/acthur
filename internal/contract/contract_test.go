package contract_test

import (
	"os"
	"path/filepath"
	"strings"
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
		Name:      "users",
		Version:   "1",
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
	reg.Register(&contract.Contract{Name: "users", Version: "1", Transport: contract.TransportHTTP, Checksum: "a"})
	reg.Register(&contract.Contract{Name: "appointments", Version: "1", Transport: contract.TransportHTTP, Checksum: "b"})
	reg.Register(&contract.Contract{Name: "vets", Version: "1", Transport: contract.TransportHTTP, Checksum: "c"})

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
			{ID: "get_user", Method: "GET", Path: "/api/v1/users/:id"},
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
// §11.5 Contract Diff Rules — one table row per rule
// ---------------------------------------------------------------------------
//
// docs/acthur-prd.md §11.5 lists the exact rule set. Each row below builds a
// minimal old/new contract pair for one rule and asserts the resulting
// DiffResult classification (HasBreaking + presence of the right Change),
// not Diff's internals.

func TestDiff_Section11_5Rules(t *testing.T) {
	tests := []struct {
		name        string
		old, next   *contract.Contract
		wantBreak   bool
		wantChgType contract.ChangeType
		wantField   string // substring expected in the matching Change.Field
	}{
		// --- BREAKING -------------------------------------------------
		{
			name: "output field removed",
			old: ep(t, contract.Endpoint{
				ID: "get_user", Method: "GET", Path: "/api/v1/users/:id",
				Output: map[string]string{"id": "ulid", "email": "email"},
			}),
			next: ep(t, contract.Endpoint{
				ID: "get_user", Method: "GET", Path: "/api/v1/users/:id",
				Output: map[string]string{"id": "ulid"}, // email removed
			}),
			wantBreak: true, wantChgType: contract.ChangeBreaking, wantField: "output.email",
		},
		{
			name: "field renamed",
			// A rename is a removal of the old name plus an addition of a
			// new (non-optional) name — both classify breaking under the
			// existing added/removed rules, so a rename is breaking overall.
			old: ep(t, contract.Endpoint{
				ID: "get_user", Method: "GET", Path: "/api/v1/users/:id",
				Output: map[string]string{"id": "ulid", "email": "email"},
			}),
			next: ep(t, contract.Endpoint{
				ID: "get_user", Method: "GET", Path: "/api/v1/users/:id",
				Output: map[string]string{"id": "ulid", "email_address": "email"}, // renamed
			}),
			wantBreak: true, wantChgType: contract.ChangeBreaking, wantField: "output.email",
		},
		{
			name: "required input field added",
			old: ep(t, contract.Endpoint{
				ID: "create_user", Method: "POST", Path: "/api/v1/users",
				Input: map[string]string{"name": "string(required)"},
			}),
			next: ep(t, contract.Endpoint{
				ID: "create_user", Method: "POST", Path: "/api/v1/users",
				Input: map[string]string{
					"name": "string(required)",
					"role": "string(required)", // new required field
				},
			}),
			wantBreak: true, wantChgType: contract.ChangeBreaking, wantField: "input.role",
		},
		{
			name: "endpoint path changed",
			old: ep(t, contract.Endpoint{
				ID: "get_user", Method: "GET", Path: "/api/v1/users/:id",
			}),
			next: ep(t, contract.Endpoint{
				ID: "get_user", Method: "GET", Path: "/api/v2/users/:id",
			}),
			wantBreak: true, wantChgType: contract.ChangeBreaking, wantField: "get_user.path",
		},
		{
			name: "HTTP method changed",
			old: ep(t, contract.Endpoint{
				ID: "update_user", Method: "PUT", Path: "/api/v1/users/:id",
			}),
			next: ep(t, contract.Endpoint{
				ID: "update_user", Method: "PATCH", Path: "/api/v1/users/:id",
			}),
			wantBreak: true, wantChgType: contract.ChangeBreaking, wantField: "update_user.method",
		},
		{
			name: "stricter validation constraint (max:200 -> max:100)",
			old: ep(t, contract.Endpoint{
				ID: "create_user", Method: "POST", Path: "/api/v1/users",
				Input: map[string]string{"bio": "string(required,max:200)"},
			}),
			next: ep(t, contract.Endpoint{
				ID: "create_user", Method: "POST", Path: "/api/v1/users",
				Input: map[string]string{"bio": "string(required,max:100)"},
			}),
			wantBreak: true, wantChgType: contract.ChangeBreaking, wantField: "input.bio.max",
		},
		{
			name: "auth requirement added (none -> required)",
			old: ep(t, contract.Endpoint{
				ID: "get_data", Method: "GET", Path: "/api/v1/data", Auth: "none",
			}),
			next: ep(t, contract.Endpoint{
				ID: "get_data", Method: "GET", Path: "/api/v1/data", Auth: "required",
			}),
			wantBreak: true, wantChgType: contract.ChangeBreaking, wantField: "get_data.auth",
		},
		{
			name: "auth requirement added (optional -> required)",
			old: ep(t, contract.Endpoint{
				ID: "get_data", Method: "GET", Path: "/api/v1/data", Auth: "optional",
			}),
			next: ep(t, contract.Endpoint{
				ID: "get_data", Method: "GET", Path: "/api/v1/data", Auth: "required",
			}),
			wantBreak: true, wantChgType: contract.ChangeBreaking, wantField: "get_data.auth",
		},

		// --- NON-BREAKING ----------------------------------------------
		{
			name: "new optional output field added",
			old: ep(t, contract.Endpoint{
				ID: "get_user", Method: "GET", Path: "/api/v1/users/:id",
				Output: map[string]string{"id": "ulid"},
			}),
			next: ep(t, contract.Endpoint{
				ID: "get_user", Method: "GET", Path: "/api/v1/users/:id",
				Output: map[string]string{"id": "ulid", "avatar_url": "url?"},
			}),
			wantBreak: false, wantChgType: contract.ChangeNonBreaking, wantField: "output.avatar_url",
		},
		{
			name: "new optional input field added",
			old: ep(t, contract.Endpoint{
				ID: "create_user", Method: "POST", Path: "/api/v1/users",
				Input: map[string]string{"name": "string(required)"},
			}),
			next: ep(t, contract.Endpoint{
				ID: "create_user", Method: "POST", Path: "/api/v1/users",
				Input: map[string]string{
					"name":      "string(required)",
					"nick_name": "string",
				},
			}),
			wantBreak: false, wantChgType: contract.ChangeNonBreaking, wantField: "input.nick_name",
		},
		{
			name: "new endpoint added",
			old: ep(t, contract.Endpoint{
				ID: "get_user", Method: "GET", Path: "/api/v1/users/:id",
			}),
			next: multiEP(t,
				contract.Endpoint{ID: "get_user", Method: "GET", Path: "/api/v1/users/:id"},
				contract.Endpoint{ID: "list_users", Method: "GET", Path: "/api/v1/users"},
			),
			wantBreak: false, wantChgType: contract.ChangeNonBreaking, wantField: "list_users",
		},
		{
			name: "looser validation constraint (max:100 -> max:200)",
			old: ep(t, contract.Endpoint{
				ID: "create_user", Method: "POST", Path: "/api/v1/users",
				Input: map[string]string{"bio": "string(required,max:100)"},
			}),
			next: ep(t, contract.Endpoint{
				ID: "create_user", Method: "POST", Path: "/api/v1/users",
				Input: map[string]string{"bio": "string(required,max:200)"},
			}),
			wantBreak: false, wantChgType: contract.ChangeNonBreaking, wantField: "input.bio.max",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contract.Diff(tt.old, tt.next)
			if result.HasBreaking != tt.wantBreak {
				t.Errorf("HasBreaking = %v, want %v (changes: %+v)", result.HasBreaking, tt.wantBreak, result.Changes)
			}
			assertHasChange(t, result.Changes, tt.wantField, tt.wantChgType)
		})
	}
}

// min: stricter/looser semantics: raising a "min" bound is stricter
// (harder for callers to satisfy); lowering it is looser. Kept as a
// separate, explicit test since §11.5 only spells out the "max" example.
func TestDiff_MinConstraint_StricterAndLooser(t *testing.T) {
	stricter := contract.Diff(
		ep(t, contract.Endpoint{
			ID: "create_user", Method: "POST", Path: "/api/v1/users",
			Input: map[string]string{"age": "int(required,min:0)"},
		}),
		ep(t, contract.Endpoint{
			ID: "create_user", Method: "POST", Path: "/api/v1/users",
			Input: map[string]string{"age": "int(required,min:18)"},
		}),
	)
	if !stricter.HasBreaking {
		t.Errorf("raising min:0 -> min:18 should be breaking (stricter), changes: %+v", stricter.Changes)
	}

	looser := contract.Diff(
		ep(t, contract.Endpoint{
			ID: "create_user", Method: "POST", Path: "/api/v1/users",
			Input: map[string]string{"age": "int(required,min:18)"},
		}),
		ep(t, contract.Endpoint{
			ID: "create_user", Method: "POST", Path: "/api/v1/users",
			Input: map[string]string{"age": "int(required,min:0)"},
		}),
	)
	if looser.HasBreaking {
		t.Errorf("lowering min:18 -> min:0 should be non-breaking (looser), changes: %+v", looser.Changes)
	}
}

// ---------------------------------------------------------------------------
// Importer tests — .proto / .graphql / .openapi.yml import to an equivalent
// native Contract (docs/audit.md Phase 4 gap; issue #55). Fixtures live under
// testdata/contract-import/<format>/.
// ---------------------------------------------------------------------------

func TestParseFile_OpenAPIImport(t *testing.T) {
	c, err := contract.ParseFile("../../testdata/contract-import/openapi/users.openapi.yml")
	if err != nil {
		t.Fatalf("ParseFile(openapi): %v", err)
	}
	if c.Name != "users" {
		t.Errorf("Name = %q, want %q", c.Name, "users")
	}
	if c.Transport != contract.TransportHTTP {
		t.Errorf("Transport = %q, want http", c.Transport)
	}
	if _, ok := c.Types["User"]; !ok {
		t.Fatalf("expected type %q to be imported, got types: %+v", "User", c.Types)
	}

	var createUser, listUsers *contract.Endpoint
	for i := range c.Endpoints {
		switch c.Endpoints[i].ID {
		case "create_user":
			createUser = &c.Endpoints[i]
		case "list_users":
			listUsers = &c.Endpoints[i]
		}
	}
	if createUser == nil {
		t.Fatalf("expected endpoint %q, got: %+v", "create_user", c.Endpoints)
	}
	if createUser.Method != "POST" || createUser.Path != "/api/v1/users" {
		t.Errorf("create_user method/path = %s %s, want POST /api/v1/users", createUser.Method, createUser.Path)
	}
	if createUser.Auth != "required" {
		t.Errorf("create_user.Auth = %q, want required", createUser.Auth)
	}
	if !strings.Contains(createUser.Input["name"], "required") {
		t.Errorf("create_user.Input[name] = %q, want it to contain required", createUser.Input["name"])
	}
	if len(createUser.Errors) == 0 {
		t.Errorf("expected create_user to carry imported error responses")
	}

	if listUsers == nil {
		t.Fatalf("expected endpoint %q, got: %+v", "list_users", c.Endpoints)
	}
	if listUsers.Method != "GET" {
		t.Errorf("list_users.Method = %q, want GET", listUsers.Method)
	}
}

func TestParseFile_ProtoImport(t *testing.T) {
	c, err := contract.ParseFile("../../testdata/contract-import/proto/users.proto")
	if err != nil {
		t.Fatalf("ParseFile(proto): %v", err)
	}
	if c.Name != "users" {
		t.Errorf("Name = %q, want %q", c.Name, "users")
	}
	if c.Transport != contract.TransportGRPC {
		t.Errorf("Transport = %q, want grpc", c.Transport)
	}
	if _, ok := c.Types["User"]; !ok {
		t.Fatalf("expected message %q to import as a type, got types: %+v", "User", c.Types)
	}

	var createUser, listUsers *contract.Endpoint
	for i := range c.Endpoints {
		switch c.Endpoints[i].ID {
		case "CreateUser":
			createUser = &c.Endpoints[i]
		case "ListUsers":
			listUsers = &c.Endpoints[i]
		}
	}
	if createUser == nil {
		t.Fatalf("expected rpc %q, got: %+v", "CreateUser", c.Endpoints)
	}
	if createUser.Input["name"] == "" {
		t.Errorf("expected CreateUser.Input to include request message fields, got: %+v", createUser.Input)
	}
	if createUser.Output["user"] == "" {
		t.Errorf("expected CreateUser.Output to include response message fields, got: %+v", createUser.Output)
	}
	if listUsers == nil {
		t.Fatalf("expected rpc %q, got: %+v", "ListUsers", c.Endpoints)
	}
}

func TestParseFile_GraphQLImport(t *testing.T) {
	c, err := contract.ParseFile("../../testdata/contract-import/graphql/users.graphql")
	if err != nil {
		t.Fatalf("ParseFile(graphql): %v", err)
	}
	if c.Transport != contract.TransportGraphQL {
		t.Errorf("Transport = %q, want graphql", c.Transport)
	}
	if _, ok := c.Types["User"]; !ok {
		t.Fatalf("expected type %q to be imported, got types: %+v", "User", c.Types)
	}
	// Types render plain shape only (bare = required, "?" suffix = optional),
	// matching the OpenAPI importer's Types convention — not the
	// "(required)" constraint syntax used for Input fields.
	if got := c.Types["User"].Fields["name"]; got != "string" {
		t.Errorf("User.name should be a required (non-optional) string, got: %q", got)
	}
	if got := c.Types["User"].Fields["role"]; got != "enum(Role)?" {
		t.Errorf("User.role should be an optional enum reference, got: %q", got)
	}

	var getUser, createUser *contract.Endpoint
	for i := range c.Endpoints {
		switch c.Endpoints[i].ID {
		case "getUser":
			getUser = &c.Endpoints[i]
		case "createUser":
			createUser = &c.Endpoints[i]
		}
	}
	if getUser == nil {
		t.Fatalf("expected query %q to import as an endpoint, got: %+v", "getUser", c.Endpoints)
	}
	if getUser.Method != "QUERY" {
		t.Errorf("getUser.Method = %q, want QUERY", getUser.Method)
	}
	if createUser == nil {
		t.Fatalf("expected mutation %q to import as an endpoint, got: %+v", "createUser", c.Endpoints)
	}
	if createUser.Method != "MUTATION" {
		t.Errorf("createUser.Method = %q, want MUTATION", createUser.Method)
	}
	if !strings.Contains(createUser.Input["name"], "required") {
		t.Errorf("createUser.Input[name] = %q, want it to contain required", createUser.Input["name"])
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
				ID:     "create_user",
				Method: "POST",
				Path:   "/api/v1/users",
				Input:  map[string]string{"name": "string(required)", "email": "email(required)"},
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

// writeFileWithExt writes content to a temp file with the given basename
// (including extension), for exercising ParseFile's format dispatch.
func writeFileWithExt(t *testing.T, basename, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, basename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	return path
}

// ep builds a minimal single-endpoint HTTP contract for §11.5 diff table
// rows. Both sides of a row use the same contract name/version/transport so
// the diff isolates the one behavior under test.
func ep(t *testing.T, e contract.Endpoint) *contract.Contract {
	t.Helper()
	return &contract.Contract{
		Name:      "svc",
		Version:   "1",
		Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{e},
	}
}

// multiEP builds a contract with several endpoints, for the "new endpoint
// added" row.
func multiEP(t *testing.T, es ...contract.Endpoint) *contract.Contract {
	t.Helper()
	return &contract.Contract{
		Name:      "svc",
		Version:   "1",
		Transport: contract.TransportHTTP,
		Endpoints: es,
	}
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
