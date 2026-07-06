// Package adapter defines the Adapter interface and the global adapter registry.
//
// An adapter is an implementation bridge between an abstract graph node and a
// concrete technology. It knows how to detect, scaffold, run, and containerize
// exactly one framework. It has ZERO knowledge of plugins, contracts, or other
// adapters.
//
// # Capability model
//
// Every adapter implements the core Adapter interface (Name, Category, Detect,
// EnvVars). Optional capabilities are expressed as separate interfaces
// (Scaffolder, Runnable, Containerized, Connectable, Migratable, Deployable,
// Dockerizable). The kernel type-asserts against these at call sites.
// CapabilitiesOf is the single source of truth for what a concrete adapter
// supports.
package adapter

import (
	"fmt"
	"sync"
)

// ---------------------------------------------------------------------------
// Category
// ---------------------------------------------------------------------------

// Category classifies what kind of technology an adapter represents.
type Category string

const (
	CategoryBackend   Category = "backend"
	CategoryFrontend  Category = "frontend"
	CategoryDatabase  Category = "database"
	CategoryCache     Category = "cache"
	CategoryStorage   Category = "storage"
	CategoryQueue     Category = "queue"
	CategorySecrets   Category = "secrets"
	CategoryAnalytics Category = "analytics"
	CategoryObserve   Category = "observability"
	CategoryDeploy    Category = "deploy"
	CategoryMobile    Category = "mobile"
	CategoryDesktop   Category = "desktop"
)

// ---------------------------------------------------------------------------
// Supporting types
// ---------------------------------------------------------------------------

// Command represents a shell command with its arguments and environment.
type Command struct {
	Bin  string
	Args []string
	Env  map[string]string
	Dir  string // working directory — relative to project root
}

func (c Command) String() string {
	if len(c.Args) == 0 {
		return c.Bin
	}
	return c.Bin + " " + joinArgs(c.Args)
}

// EnvVar declares an environment variable that an adapter requires.
type EnvVar struct {
	Key         string
	Description string
	Required    bool
	Default     string
	Secret      bool // should be in secrets provider, not .env
	// Generate marks a value the kernel synthesizes when it is otherwise unset
	// (e.g. a dev APP_SECRET). The kernel persists it for stable local dev.
	// Only meaningful for Required Secret vars that have no Default.
	Generate bool
}

// File represents a file that an adapter will scaffold into the project.
type File struct {
	Path    string // relative to project root
	Content []byte
	Mode    uint32 // unix file mode, 0644 default
}

// ScaffoldContext is the context provided to an adapter's Scaffold() method.
type ScaffoldContext struct {
	ProjectName string
	NodeID      string
	RootDir     string
	Env         map[string]string
	Extra       map[string]any // adapter-specific extra config from acthur.yml
	// ModulePath is the fully-resolved Go module path (e.g. "github.com/myorg/api").
	// Resolved by the kernel from module_prefix + "/" + nodeID; adapters use it verbatim.
	ModulePath string
	// IDStrategy is the identifier generation strategy from acthur.yml identifiers.strategy.
	// e.g. "ulid" (default) or "uuid-v4".
	IDStrategy string
}

// ---------------------------------------------------------------------------
// Container types (needed by Containerized capability)
// ---------------------------------------------------------------------------

// ContainerContext carries node-level facts the adapter needs to build a spec.
type ContainerContext struct {
	NodeID  string
	Version string // from node's version: field in acthur.yml
	Env     map[string]string
}

// ContainerSpec is a declarative description of a container resource.
// The kernel projects this onto docker run / compose / k8s in later phases.
type ContainerSpec struct {
	Image       string
	Tag         string
	Ports       []int
	Volumes     []Volume
	Env         map[string]string
	Healthcheck Healthcheck
	Cmd         []string
}

// Volume describes a named volume mount inside a container.
type Volume struct {
	Name      string
	MountPath string
}

// Healthcheck describes how the container runtime should probe liveness.
type Healthcheck struct {
	Test     []string
	Interval string
	Timeout  string
	Retries  int
}

// ---------------------------------------------------------------------------
// Capability enum
// ---------------------------------------------------------------------------

// Capability is a named capability that an adapter may expose.
type Capability string

const (
	CapabilityScaffold    Capability = "Scaffold"
	CapabilityRun         Capability = "Run"
	CapabilityContainer   Capability = "Container"
	CapabilityConnectable Capability = "Connectable"
	CapabilityMigrate     Capability = "Migrate"
	CapabilityDeploy      Capability = "Deploy"
	CapabilityDockerize   Capability = "Dockerize"
)

// ---------------------------------------------------------------------------
// Core Adapter interface
// ---------------------------------------------------------------------------

// Adapter is the minimal interface every adapter must implement.
// Optional capabilities are expressed as Scaffolder, Runnable, Containerized,
// Migratable, and Deployable. Use CapabilitiesOf to introspect them.
type Adapter interface {
	// Name returns the adapter's unique key in "runtime:framework" format.
	// Examples: "go:fiber", "rust:axum", "ui:astro", "db:postgres"
	Name() string

	// Category returns the type of technology this adapter represents.
	Category() Category

	// Detect returns true if this adapter's framework is already present
	// in the given directory. Used by `acthur init` to auto-detect the stack.
	Detect(dir string) bool

	// EnvVars returns the environment variables this adapter requires.
	// Used by acthur to generate .env.example and validate environments.
	EnvVars() []EnvVar
}

// ---------------------------------------------------------------------------
// Capability interfaces
// ---------------------------------------------------------------------------

// Scaffolder can produce a minimal set of files for a new project node.
type Scaffolder interface {
	Scaffold(ctx ScaffoldContext) ([]File, error)
}

// Runnable can produce dev, build, and test shell commands.
type Runnable interface {
	DevCommand(env map[string]string) Command
	BuildCommand(env map[string]string) Command
	TestCommand(env map[string]string) Command
}

// Containerized can describe itself as a declarative container spec.
// The kernel projects this onto docker run / compose / k8s.
type Containerized interface {
	Container(ctx ContainerContext) ContainerSpec
}

// Connectable can export environment variables that consumers use to connect.
type Connectable interface {
	ConnectionEnv(ctx ContainerContext) map[string]string
}

// Migratable can produce a migration command (database schema management).
// No implementations in this phase — defined for forward compatibility.
type Migratable interface {
	MigrateCommand(env map[string]string) Command
}

// Deployable can produce a deploy command for production release.
// No implementations in this phase — defined for forward compatibility.
type Deployable interface {
	DeployCommand(env map[string]string) Command
}

// DockerfileContext carries the node-level facts a Dockerizable adapter
// needs to render a production Dockerfile. It is deliberately narrower than
// ScaffoldContext (project scaffolding, run once at `acthur add`) and
// ContainerContext (declarative container spec for infra resources) — a
// production Dockerfile only ever needs the node's identity and the port
// it serves on.
type DockerfileContext struct {
	NodeID string
	// Port is the node's configured port. Zero means the node has no
	// listening port (e.g. a queue-worker service) — the adapter must omit
	// EXPOSE and HEALTHCHECK in that case rather than guess one.
	Port int
}

// Dockerizable can render a production-ready, multi-stage Dockerfile for a
// service node. Infra adapters (e.g. db:postgres) do not implement this —
// they run from official upstream images, not a project-owned build.
type Dockerizable interface {
	DockerfileFor(ctx DockerfileContext) ([]byte, error)
}

// ---------------------------------------------------------------------------
// CapabilitiesOf — single source of truth
// ---------------------------------------------------------------------------

// CapabilitiesOf returns the capabilities supported by adapter a.
// It type-asserts against each capability interface; adapters must NOT
// implement a Capabilities() method — this free function is authoritative.
func CapabilitiesOf(a Adapter) []Capability {
	var caps []Capability
	if _, ok := a.(Scaffolder); ok {
		caps = append(caps, CapabilityScaffold)
	}
	if _, ok := a.(Runnable); ok {
		caps = append(caps, CapabilityRun)
	}
	if _, ok := a.(Containerized); ok {
		caps = append(caps, CapabilityContainer)
	}
	if _, ok := a.(Connectable); ok {
		caps = append(caps, CapabilityConnectable)
	}
	if _, ok := a.(Migratable); ok {
		caps = append(caps, CapabilityMigrate)
	}
	if _, ok := a.(Deployable); ok {
		caps = append(caps, CapabilityDeploy)
	}
	if _, ok := a.(Dockerizable); ok {
		caps = append(caps, CapabilityDockerize)
	}
	return caps
}

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

var (
	mu       sync.RWMutex
	registry = map[string]Adapter{}
)

// Register adds an adapter to the global registry.
// Called from each adapter package's init() function.
// Panics if an adapter with the same name is already registered.
func Register(a Adapter) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := registry[a.Name()]; exists {
		panic(fmt.Sprintf("adapter %q is already registered", a.Name()))
	}
	registry[a.Name()] = a
}

// Resolve returns the adapter registered under the given name.
// Returns an error if no adapter is found.
func Resolve(name string) (Adapter, error) {
	mu.RLock()
	defer mu.RUnlock()
	a, ok := registry[name]
	if !ok {
		return nil, &NotFoundError{Name: name}
	}
	return a, nil
}

// All returns all registered adapters.
func All() []Adapter {
	mu.RLock()
	defer mu.RUnlock()
	result := make([]Adapter, 0, len(registry))
	for _, a := range registry {
		result = append(result, a)
	}
	return result
}

// ByCategory returns all adapters of a given category.
func ByCategory(c Category) []Adapter {
	mu.RLock()
	defer mu.RUnlock()
	var result []Adapter
	for _, a := range registry {
		if a.Category() == c {
			result = append(result, a)
		}
	}
	return result
}

// Names returns all registered adapter names, sorted.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sortStrings(names)
	return names
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// NotFoundError is returned when Resolve cannot find an adapter.
type NotFoundError struct {
	Name string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf(
		"adapter %q is not registered\n"+
			"  Available adapters: %v\n"+
			"  Check your adapter name in acthur.yml or install the adapter package.",
		e.Name, Names(),
	)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func joinArgs(args []string) string {
	result := ""
	for i, a := range args {
		if i > 0 {
			result += " "
		}
		result += a
	}
	return result
}

func sortStrings(ss []string) {
	for i := 0; i < len(ss); i++ {
		for j := i + 1; j < len(ss); j++ {
			if ss[j] < ss[i] {
				ss[i], ss[j] = ss[j], ss[i]
			}
		}
	}
}
