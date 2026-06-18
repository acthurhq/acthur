// Package contract is the safety layer of the Acthur kernel.
// It defines the Contract type, parses .contract.yml files, maintains
// a live registry of all contracts, validates requests and responses
// against registered contracts, and detects breaking changes.
package contract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Contract types
// ---------------------------------------------------------------------------

// Transport defines how a contract's endpoints are served.
type Transport string

const (
	TransportHTTP    Transport = "http"
	TransportGRPC    Transport = "grpc"
	TransportWS      Transport = "ws"
	TransportQueue   Transport = "queue"
	TransportGraphQL Transport = "graphql"
)

// Contract is the fully-parsed representation of a .contract.yml file.
// It is the source of truth for what can flow on a data_flow graph edge.
type Contract struct {
	Name      string               `yaml:"contract"`
	Version   string               `yaml:"version"`
	Transport Transport            `yaml:"transport"`
	Endpoints []Endpoint           `yaml:"endpoints"`
	Events    []EventDef           `yaml:"events"`
	Types     map[string]TypeDef   `yaml:"types"`

	// Runtime fields
	FilePath  string `yaml:"-"`
	Checksum  string `yaml:"-"`
}

// Endpoint defines one operation exposed by the contract.
type Endpoint struct {
	ID          string            `yaml:"id"`
	Method      string            `yaml:"method"`
	Path        string            `yaml:"path"`
	Auth        string            `yaml:"auth"`    // "required" | "optional" | "none"
	Roles       []string          `yaml:"roles"`
	RateLimit   RateLimitDef      `yaml:"rate_limit"`
	Input       map[string]string `yaml:"input"`   // field: type(constraints)
	Output      map[string]string `yaml:"output"`  // field: type
	Errors      []ErrorDef        `yaml:"errors"`
	Deprecated  bool              `yaml:"deprecated"`
	DeprecatedAt string           `yaml:"deprecated_at"`
	SunsetAt    string            `yaml:"sunset_at"`
	Streaming   string            `yaml:"streaming"` // for gRPC: server|client|bidirectional
}

// RateLimitDef defines rate limiting for an endpoint.
type RateLimitDef struct {
	Max    int    `yaml:"max"`
	Window string `yaml:"window"`
}

// EventDef defines an event that a service emits on a contract.
type EventDef struct {
	ID      string            `yaml:"id"`
	Payload map[string]string `yaml:"payload"`
}

// ErrorDef defines a possible error response.
type ErrorDef struct {
	Status  int    `yaml:"status"`
	Code    string `yaml:"code"`
	Message string `yaml:"message"`
}

// TypeDef defines a reusable type within a contract.
type TypeDef struct {
	Fields map[string]string `yaml:"fields"`
}

// ---------------------------------------------------------------------------
// Parser
// ---------------------------------------------------------------------------

// ParseFile reads and parses a .contract.yml file.
// It also accepts .proto, .openapi.yml, and .graphql files — those are
// compiled to the Contract representation before being returned.
func ParseFile(path string) (*Contract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read contract file %q: %w", path, err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yml", ".yaml":
		return parseYAML(path, data)
	case ".proto":
		return parseProto(path, data)
	case ".graphql", ".gql":
		return parseGraphQL(path, data)
	default:
		// Try YAML by default (e.g. .contract files)
		return parseYAML(path, data)
	}
}

func parseYAML(path string, data []byte) (*Contract, error) {
	var c Contract
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("cannot parse contract %q: %w", path, err)
	}
	if c.Name == "" {
		return nil, fmt.Errorf("contract %q: missing required field 'contract' (name)", path)
	}
	if c.Version == "" {
		c.Version = "1"
	}
	if c.Transport == "" {
		c.Transport = TransportHTTP
	}
	c.FilePath = path
	c.Checksum = checksum(data)
	return &c, nil
}

// parseProto is a stub — full implementation reads protobuf descriptors.
func parseProto(path string, _ []byte) (*Contract, error) {
	return &Contract{
		Name:      filepath.Base(strings.TrimSuffix(path, filepath.Ext(path))),
		Version:   "1",
		Transport: TransportGRPC,
		FilePath:  path,
	}, nil
}

// parseGraphQL is a stub — full implementation reads GraphQL SDL.
func parseGraphQL(path string, _ []byte) (*Contract, error) {
	return &Contract{
		Name:      filepath.Base(strings.TrimSuffix(path, filepath.Ext(path))),
		Version:   "1",
		Transport: TransportGraphQL,
		FilePath:  path,
	}, nil
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// ValidationError represents a single contract structural validation failure.
type ValidationError struct {
	Contract string
	Field    string
	Message  string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("contract %q: %s", e.Contract, e.Message)
}

// Validate checks a contract for structural correctness.
func (c *Contract) Validate() []ValidationError {
	var errs []ValidationError

	if c.Name == "" {
		errs = append(errs, ValidationError{
			Contract: c.FilePath,
			Field:    "contract",
			Message:  "contract name is required",
		})
	}

	for i, ep := range c.Endpoints {
		if ep.ID == "" {
			errs = append(errs, ValidationError{
				Contract: c.Name,
				Field:    fmt.Sprintf("endpoints[%d].id", i),
				Message:  "endpoint id is required",
			})
		}
		if c.Transport == TransportHTTP {
			if ep.Method == "" {
				errs = append(errs, ValidationError{
					Contract: c.Name,
					Field:    fmt.Sprintf("endpoints[%d].method", i),
					Message:  fmt.Sprintf("endpoint %q: HTTP method is required", ep.ID),
				})
			}
			if ep.Path == "" {
				errs = append(errs, ValidationError{
					Contract: c.Name,
					Field:    fmt.Sprintf("endpoints[%d].path", i),
					Message:  fmt.Sprintf("endpoint %q: path is required", ep.ID),
				})
			}
		}
	}

	return errs
}

// ---------------------------------------------------------------------------
// Diff Engine
// ---------------------------------------------------------------------------

// ChangeType classifies a contract change.
type ChangeType string

const (
	ChangeBreaking    ChangeType = "breaking"
	ChangeNonBreaking ChangeType = "non-breaking"
	ChangeInfo        ChangeType = "info"
)

// Change describes one difference between two contract versions.
type Change struct {
	Type        ChangeType
	Description string
	Field       string
}

// DiffResult is the output of comparing two contract versions.
type DiffResult struct {
	Old         *Contract
	New         *Contract
	Changes     []Change
	HasBreaking bool
}

// Diff compares two contract versions and classifies all changes.
func Diff(old, next *Contract) *DiffResult {
	result := &DiffResult{Old: old, New: next}

	// Check version change
	if old.Version != next.Version {
		result.Changes = append(result.Changes, Change{
			Type:        ChangeInfo,
			Description: fmt.Sprintf("version bumped from %s to %s", old.Version, next.Version),
			Field:       "version",
		})
	}

	// Check transport change
	if old.Transport != next.Transport {
		result.addBreaking(fmt.Sprintf("transport changed from %q to %q (breaking)", old.Transport, next.Transport), "transport")
	}

	// Index old endpoints by ID
	oldEPs := make(map[string]Endpoint)
	for _, ep := range old.Endpoints {
		oldEPs[ep.ID] = ep
	}
	newEPs := make(map[string]Endpoint)
	for _, ep := range next.Endpoints {
		newEPs[ep.ID] = ep
	}

	// Detect removed endpoints (breaking)
	for id := range oldEPs {
		if _, exists := newEPs[id]; !exists {
			result.addBreaking(fmt.Sprintf("endpoint %q removed", id), "endpoints."+id)
		}
	}

	// Detect added endpoints (non-breaking)
	for id := range newEPs {
		if _, exists := oldEPs[id]; !exists {
			result.addNonBreaking(fmt.Sprintf("endpoint %q added", id), "endpoints."+id)
		}
	}

	// Diff existing endpoints
	for id, oldEP := range oldEPs {
		newEP, exists := newEPs[id]
		if !exists {
			continue
		}

		// Method change
		if oldEP.Method != newEP.Method {
			result.addBreaking(
				fmt.Sprintf("endpoint %q: method changed from %q to %q", id, oldEP.Method, newEP.Method),
				"endpoints."+id+".method",
			)
		}

		// Path change
		if oldEP.Path != newEP.Path {
			result.addBreaking(
				fmt.Sprintf("endpoint %q: path changed from %q to %q", id, oldEP.Path, newEP.Path),
				"endpoints."+id+".path",
			)
		}

		// Auth requirement added
		if oldEP.Auth != "required" && newEP.Auth == "required" {
			result.addBreaking(
				fmt.Sprintf("endpoint %q: auth changed to required", id),
				"endpoints."+id+".auth",
			)
		}

		// Output field removed
		for field := range oldEP.Output {
			if _, exists := newEP.Output[field]; !exists {
				result.addBreaking(
					fmt.Sprintf("endpoint %q: output field %q removed", id, field),
					fmt.Sprintf("endpoints.%s.output.%s", id, field),
				)
			}
		}

		// Output field added (non-breaking if type ends in ?)
		for field, typ := range newEP.Output {
			if _, exists := oldEP.Output[field]; !exists {
				if strings.HasSuffix(strings.Split(typ, "(")[0], "?") {
					result.addNonBreaking(
						fmt.Sprintf("endpoint %q: optional output field %q added", id, field),
						fmt.Sprintf("endpoints.%s.output.%s", id, field),
					)
				} else {
					result.addBreaking(
						fmt.Sprintf("endpoint %q: required output field %q added — clients may break if not handled", id, field),
						fmt.Sprintf("endpoints.%s.output.%s", id, field),
					)
				}
			}
		}

		// Required input field added (breaking — callers must send it)
		for field, typ := range newEP.Input {
			if _, exists := oldEP.Input[field]; !exists {
				if strings.Contains(typ, "required") {
					result.addBreaking(
						fmt.Sprintf("endpoint %q: required input field %q added", id, field),
						fmt.Sprintf("endpoints.%s.input.%s", id, field),
					)
				} else {
					result.addNonBreaking(
						fmt.Sprintf("endpoint %q: optional input field %q added", id, field),
						fmt.Sprintf("endpoints.%s.input.%s", id, field),
					)
				}
			}
		}

		// Input field removed (non-breaking for callers, breaking for service)
		for field := range oldEP.Input {
			if _, exists := newEP.Input[field]; !exists {
				result.addNonBreaking(
					fmt.Sprintf("endpoint %q: input field %q removed", id, field),
					fmt.Sprintf("endpoints.%s.input.%s", id, field),
				)
			}
		}
	}

	// Check for removed types (breaking if used in endpoint output)
	for name := range old.Types {
		if _, exists := next.Types[name]; !exists {
			result.addBreaking(fmt.Sprintf("type %q removed", name), "types."+name)
		}
	}

	return result
}

func (r *DiffResult) addBreaking(desc, field string) {
	r.Changes = append(r.Changes, Change{Type: ChangeBreaking, Description: desc, Field: field})
	r.HasBreaking = true
}

func (r *DiffResult) addNonBreaking(desc, field string) {
	r.Changes = append(r.Changes, Change{Type: ChangeNonBreaking, Description: desc, Field: field})
}

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

// Registry maintains the live set of contracts active in the running system.
// Plugins and the graph engine register contracts here.
// The proxy reads from here to enforce contracts on data_flow edges.
type Registry struct {
	mu        sync.RWMutex
	contracts map[string]*Contract // key: "name@version"
}

// NewRegistry creates an empty contract registry.
func NewRegistry() *Registry {
	return &Registry{
		contracts: make(map[string]*Contract),
	}
}

// Register adds a contract to the registry.
// If a contract with the same name and version already exists and the
// checksum matches, it is silently skipped (idempotent).
func (r *Registry) Register(c *Contract) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := r.key(c.Name, c.Version)
	if existing, ok := r.contracts[key]; ok {
		if existing.Checksum == c.Checksum {
			return nil // identical — skip
		}
		return fmt.Errorf(
			"contract %q@%s already registered with a different checksum\n"+
				"  Bump the contract version to register a new version alongside the old one",
			c.Name, c.Version,
		)
	}

	if errs := c.Validate(); len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		return fmt.Errorf("contract %q is invalid:\n  - %s",
			c.Name, strings.Join(msgs, "\n  - "))
	}

	r.contracts[key] = c
	return nil
}

// RegisterFile parses and registers a contract from a file path.
func (r *Registry) RegisterFile(path string) (*Contract, error) {
	c, err := ParseFile(path)
	if err != nil {
		return nil, err
	}
	if err := r.Register(c); err != nil {
		return nil, err
	}
	return c, nil
}

// Get returns the contract with the given name and version.
func (r *Registry) Get(name, version string) (*Contract, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.contracts[r.key(name, version)]
	if !ok {
		return nil, fmt.Errorf("contract %q@%s not registered", name, version)
	}
	return c, nil
}

// GetLatest returns the highest-versioned contract with the given name.
func (r *Registry) GetLatest(name string) (*Contract, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var latest *Contract
	for _, c := range r.contracts {
		if c.Name == name {
			if latest == nil || c.Version > latest.Version {
				latest = c
			}
		}
	}
	if latest == nil {
		return nil, fmt.Errorf("no contract registered with name %q", name)
	}
	return latest, nil
}

// All returns all registered contracts.
func (r *Registry) All() []*Contract {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Contract, 0, len(r.contracts))
	for _, c := range r.contracts {
		result = append(result, c)
	}
	return result
}

// Versions returns all registered versions for a contract name.
func (r *Registry) Versions(name string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var versions []string
	for _, c := range r.contracts {
		if c.Name == name {
			versions = append(versions, c.Version)
		}
	}
	return versions
}

func (r *Registry) key(name, version string) string {
	return name + "@" + version
}

// ---------------------------------------------------------------------------
// Runtime Validator
// ---------------------------------------------------------------------------

// ViolationSeverity indicates how a violation is handled.
type ViolationSeverity string

const (
	SeverityWarn  ViolationSeverity = "warn"  // dev mode: log + pass through
	SeverityBlock ViolationSeverity = "block" // strict mode: reject request
)

// Violation represents a single contract violation.
type Violation struct {
	Field     string
	Message   string
	Direction string // "request" | "response"
}

// ValidationResult is the output of validating a request or response.
type ValidationResult struct {
	Valid      bool
	Violations []Violation
	Contract   string
	EndpointID string
}

// Validator validates HTTP traffic against registered contracts.
type Validator struct {
	registry *Registry
	mode     ViolationSeverity
}

// NewValidator creates a validator backed by the given registry.
func NewValidator(registry *Registry, mode ViolationSeverity) *Validator {
	return &Validator{registry: registry, mode: mode}
}

// ValidateRequest checks an incoming request against the contract for the
// given endpoint. Returns a ValidationResult with any violations found.
func (v *Validator) ValidateRequest(
	contractName, version, endpointID string,
	headers map[string]string,
	body map[string]any,
) ValidationResult {
	result := ValidationResult{
		Valid:      true,
		Contract:   contractName,
		EndpointID: endpointID,
	}

	c, err := v.registry.Get(contractName, version)
	if err != nil {
		// Contract not registered — this is a config issue, not a violation
		result.Violations = append(result.Violations, Violation{
			Message:   fmt.Sprintf("contract %q@%s not found in registry", contractName, version),
			Direction: "request",
		})
		result.Valid = false
		return result
	}

	// Find the endpoint definition
	var ep *Endpoint
	for i := range c.Endpoints {
		if c.Endpoints[i].ID == endpointID {
			ep = &c.Endpoints[i]
			break
		}
	}
	if ep == nil {
		result.Violations = append(result.Violations, Violation{
			Message:   fmt.Sprintf("endpoint %q not defined in contract %q", endpointID, contractName),
			Direction: "request",
		})
		result.Valid = false
		return result
	}

	// Check auth header present when required
	if ep.Auth == "required" {
		if headers["Authorization"] == "" && headers["authorization"] == "" {
			result.Violations = append(result.Violations, Violation{
				Field:     "Authorization",
				Message:   "Authorization header is required for this endpoint",
				Direction: "request",
			})
			result.Valid = false
		}
	}

	// Check required input fields
	for field, typeDef := range ep.Input {
		if strings.Contains(typeDef, "required") {
			if _, exists := body[field]; !exists {
				result.Violations = append(result.Violations, Violation{
					Field:     field,
					Message:   fmt.Sprintf("required field %q is missing from request body", field),
					Direction: "request",
				})
				result.Valid = false
			}
		}
	}

	return result
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func checksum(data []byte) string {
	// Simple FNV-1a checksum for change detection
	h := uint32(2166136261)
	for _, b := range data {
		h ^= uint32(b)
		h *= 16777619
	}
	return fmt.Sprintf("%08x", h)
}
