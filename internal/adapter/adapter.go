// Package adapter defines the Adapter interface and the global adapter registry.
//
// An adapter is an implementation bridge between an abstract graph node and a
// concrete technology. It knows how to start, build, scaffold, test, and produce
// a deployment artifact for exactly one framework.
//
// An adapter has ZERO knowledge of plugins, contracts, or other adapters.
// It only knows its own framework's toolchain.
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
	CategoryBackend  Category = "backend"
	CategoryFrontend Category = "frontend"
	CategoryDatabase Category = "database"
	CategoryCache    Category = "cache"
	CategoryStorage  Category = "storage"
	CategoryQueue    Category = "queue"
	CategorySecrets  Category = "secrets"
	CategoryAnalytics Category = "analytics"
	CategoryObserve  Category = "observability"
	CategoryDeploy   Category = "deploy"
	CategoryMobile   Category = "mobile"
	CategoryDesktop  Category = "desktop"
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
}

// BuildConfig is the context provided to an adapter's Dockerfile() method.
type BuildConfig struct {
	ProjectName string
	NodeID      string
	Port        int
	Env         map[string]string
}

// ---------------------------------------------------------------------------
// Adapter interface
// ---------------------------------------------------------------------------

// Adapter is the interface every adapter must implement.
// Implementing 8 methods is all that is required to add a new runtime
// to Acthur. The kernel calls these methods — the adapter never calls back.
type Adapter interface {
	// Name returns the adapter's unique key in "runtime:framework" format.
	// Examples: "go:fiber", "rust:axum", "ui:astro", "db:postgres"
	Name() string

	// Category returns the type of technology this adapter represents.
	Category() Category

	// Detect returns true if this adapter's framework is already present
	// in the given directory. Used by `acthur init` to auto-detect the stack.
	Detect(dir string) bool

	// Scaffold returns the minimal set of files to create for a new node
	// of this adapter type. Produces a runnable but empty starting point.
	Scaffold(ctx ScaffoldContext) ([]File, error)

	// DevCommand returns the command to start this service in development mode.
	DevCommand(env map[string]string) Command

	// BuildCommand returns the command to build this service for production.
	BuildCommand(env map[string]string) Command

	// TestCommand returns the command to run this service's tests.
	TestCommand(env map[string]string) Command

	// GeneratorTargets returns the code generation targets this adapter can
	// receive from plugins. Plugins register generators for these target names.
	// Example: ["handler", "middleware", "model", "migration", "service", "dto"]
	GeneratorTargets() []string

	// Dockerfile returns the Dockerfile content for this adapter.
	// Used by the deploy engine to build production images.
	Dockerfile(cfg BuildConfig) string

	// EnvVars returns the environment variables this adapter requires.
	// Used by acthur to generate .env.example and validate environments.
	EnvVars() []EnvVar
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
