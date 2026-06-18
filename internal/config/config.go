// Package config loads and validates acthur.yml into typed Go structs.
// It walks up the directory tree from cwd to find the nearest acthur.yml,
// matching how git finds .git — you can run acthur from any subdirectory.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const configFileName = "acthur.yml"

// ---------------------------------------------------------------------------
// Top-level Config
// ---------------------------------------------------------------------------

// Config is the fully-parsed representation of acthur.yml.
type Config struct {
	Project      string                 `yaml:"project"`
	Version      string                 `yaml:"version"`
	Identifiers  IdentifierConfig       `yaml:"identifiers"`
	Dev          DevConfig              `yaml:"dev"`
	Environments map[string]EnvConfig   `yaml:"environments"`
	Graph        GraphConfig            `yaml:"graph"`
	Plugins      []PluginEntry          `yaml:"plugins"`
	AI           AIConfig               `yaml:"ai"`

	// Runtime fields — not in YAML, populated by loader
	RootDir   string `yaml:"-"`
	ConfigPath string `yaml:"-"`
}

// ---------------------------------------------------------------------------
// Identifier Strategy
// ---------------------------------------------------------------------------

type IdentifierStrategy string

const (
	StrategyULID       IdentifierStrategy = "ulid"
	StrategyUUIDv4     IdentifierStrategy = "uuid-v4"
	StrategyCUID2      IdentifierStrategy = "cuid2"
	StrategyNanoID     IdentifierStrategy = "nanoid"
	StrategySequential IdentifierStrategy = "sequential"
)

type IdentifierConfig struct {
	Strategy       IdentifierStrategy `yaml:"strategy"`
	PublicStrategy IdentifierStrategy `yaml:"public_strategy"`
	NanoIDLength   int                `yaml:"nanoid_length"`
}

func (c *IdentifierConfig) setDefaults() {
	if c.Strategy == "" {
		c.Strategy = StrategyULID
	}
	if c.PublicStrategy == "" {
		c.PublicStrategy = c.Strategy
	}
	if c.NanoIDLength == 0 {
		c.NanoIDLength = 21
	}
}

// ---------------------------------------------------------------------------
// Dev Config
// ---------------------------------------------------------------------------

type DNSStrategy string

const (
	DNSAuto      DNSStrategy = "auto"
	DNSLocalDNS  DNSStrategy = "local-dns"
	DNSHostsFile DNSStrategy = "hosts-file"
	DNSPACProxy  DNSStrategy = "pac-proxy"
)

type DevConfig struct {
	Domain      string      `yaml:"domain"`
	HTTPS       bool        `yaml:"https"`
	Port        int         `yaml:"port"`
	DNSStrategy DNSStrategy `yaml:"dns_strategy"`
}

func (c *DevConfig) setDefaults(project string) {
	if c.Domain == "" && project != "" {
		c.Domain = project + ".test"
	}
	if c.Port == 0 {
		c.Port = 4000
	}
	if c.DNSStrategy == "" {
		c.DNSStrategy = DNSAuto
	}
}

// ---------------------------------------------------------------------------
// Environment Config
// ---------------------------------------------------------------------------

type ExecutionContext string

const (
	ContextLocal  ExecutionContext = "local"
	ContextDocker ExecutionContext = "docker"
	ContextCloud  ExecutionContext = "cloud"
)

type DeployTarget string

const (
	TargetCoolify DeployTarget = "coolify"
	TargetFly     DeployTarget = "fly"
	TargetRailway DeployTarget = "railway"
	TargetRender  DeployTarget = "render"
	TargetDocker  DeployTarget = "docker"
	TargetK8s     DeployTarget = "k8s"
)

type EnvConfig struct {
	Context ExecutionContext `yaml:"context"`
	Target  DeployTarget    `yaml:"target"`
	Host    string          `yaml:"host"`
}

// ---------------------------------------------------------------------------
// Graph Config
// ---------------------------------------------------------------------------

type GraphConfig struct {
	Nodes map[string]NodeConfig `yaml:"nodes"`
	Edges []EdgeConfig          `yaml:"edges"`
}

// ---------------------------------------------------------------------------
// Node Config
// ---------------------------------------------------------------------------

type NodeType string

const (
	NodeTypeService  NodeType = "service"
	NodeTypeInfra    NodeType = "infra"
	NodeTypePlugin   NodeType = "plugin"
	NodeTypeContract NodeType = "contract"
)

type NodeRole string

const (
	NodeRoleServer      NodeRole = "server"
	NodeRoleQueueWorker NodeRole = "queue-worker"
	NodeRoleCron        NodeRole = "cron"
	NodeRoleGateway     NodeRole = "gateway"
)

type NodeConfig struct {
	Type      NodeType   `yaml:"type"`
	Adapter   string     `yaml:"adapter"`
	Port      int        `yaml:"port"`
	HotReload *bool      `yaml:"hot_reload"`
	DevURL    string     `yaml:"dev_url"`
	Role      NodeRole   `yaml:"role"`
	Version   string     `yaml:"version"`
	Pool      PoolConfig `yaml:"pool"`

	// Raw extra config — adapter-specific keys
	Extra map[string]any `yaml:",inline"`
}

func (n *NodeConfig) IsHotReloadEnabled() bool {
	if n.HotReload == nil {
		return n.Type == NodeTypeService
	}
	return *n.HotReload
}

// PoolConfig holds database connection pool settings.
// "auto" values are resolved at runtime by the pool tuner.
type PoolConfig struct {
	MaxConns              string `yaml:"max_conns"`               // int or "auto"
	MinConns              string `yaml:"min_conns"`               // int or "auto"
	MaxConnLifetime       string `yaml:"max_conn_lifetime"`       // duration string
	MaxConnLifetimeJitter string `yaml:"max_conn_lifetime_jitter"`
	MaxConnIdleTime       string `yaml:"max_conn_idle_time"`
	HealthCheckPeriod     string `yaml:"health_check_period"`
	ConnectTimeout        string `yaml:"connect_timeout"`
	BeforeAcquire         string `yaml:"before_acquire"` // "ping" | "none"
	AfterRelease          string `yaml:"after_release"`  // "reset_search_path" | "none"
}

func (p *PoolConfig) setDefaults() {
	if p.MaxConns == "" {
		p.MaxConns = "auto"
	}
	if p.MinConns == "" {
		p.MinConns = "auto"
	}
	if p.MaxConnLifetime == "" {
		p.MaxConnLifetime = "1h"
	}
	if p.MaxConnLifetimeJitter == "" {
		p.MaxConnLifetimeJitter = "5m"
	}
	if p.MaxConnIdleTime == "" {
		p.MaxConnIdleTime = "30m"
	}
	if p.HealthCheckPeriod == "" {
		p.HealthCheckPeriod = "30s"
	}
	if p.ConnectTimeout == "" {
		p.ConnectTimeout = "5s"
	}
	if p.BeforeAcquire == "" {
		p.BeforeAcquire = "ping"
	}
}

// ---------------------------------------------------------------------------
// Edge Config
// ---------------------------------------------------------------------------

type EdgeType string

const (
	EdgeDependsOn      EdgeType = "depends_on"
	EdgeDataFlow       EdgeType = "data_flow"
	EdgeProxiedThrough EdgeType = "proxied_through"
	EdgeSatisfies      EdgeType = "satisfies"
	EdgeConsumes       EdgeType = "consumes"
	EdgeMigrates       EdgeType = "migrates"
	EdgeEmits          EdgeType = "emits"
	EdgeSubscribesTo   EdgeType = "subscribes_to"
	EdgeAppliesTo      EdgeType = "applies_to"
)

type EdgeTransport string

const (
	TransportHTTP  EdgeTransport = "http"
	TransportGRPC  EdgeTransport = "grpc"
	TransportWS    EdgeTransport = "ws"
	TransportQueue EdgeTransport = "queue"
)

type EdgeConfig struct {
	From        string        `yaml:"from"`
	To          string        `yaml:"to"`
	Type        EdgeType      `yaml:"type"`
	Contracts   []string      `yaml:"contracts"`
	Transport   EdgeTransport `yaml:"transport"`
	Events      []string      `yaml:"events"`
	PathPrefix  string        `yaml:"path_prefix"`
}

// ---------------------------------------------------------------------------
// Plugin Entry
// ---------------------------------------------------------------------------

type PluginEntry struct {
	Name   string         `yaml:"name"`
	Config map[string]any `yaml:"config"`
}

// ---------------------------------------------------------------------------
// AI Config
// ---------------------------------------------------------------------------

type LLMProvider string

const (
	ProviderClaude LLMProvider = "claude"
	ProviderOpenAI LLMProvider = "openai"
	ProviderGroq   LLMProvider = "groq"
	ProviderOllama LLMProvider = "ollama"
)

type AIConfig struct {
	Provider        LLMProvider `yaml:"provider"`
	Model           string      `yaml:"model"`
	APIKey          string      `yaml:"api_key"`
	ContextStrategy string      `yaml:"context_strategy"`
	BaseURL         string      `yaml:"base_url"`
}

// ---------------------------------------------------------------------------
// Loader
// ---------------------------------------------------------------------------

// Load finds and parses the nearest acthur.yml walking up from dir.
// Returns ErrNotFound if no acthur.yml exists in the directory tree.
func Load(dir string) (*Config, error) {
	path, err := Find(dir)
	if err != nil {
		return nil, err
	}
	return LoadFile(path)
}

// LoadFile parses a specific acthur.yml file.
func LoadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", path, err)
	}

	cfg.ConfigPath = path
	cfg.RootDir = filepath.Dir(path)

	cfg.applyDefaults()

	if errs := cfg.Validate(); len(errs) > 0 {
		return nil, &ValidationError{Errors: errs}
	}

	return &cfg, nil
}

// Find walks up from dir searching for acthur.yml.
func Find(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(abs, configFileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			// Reached filesystem root without finding acthur.yml
			return "", ErrNotFound
		}
		abs = parent
	}
}

// Exists returns true if acthur.yml exists at or above dir.
func Exists(dir string) bool {
	_, err := Find(dir)
	return err == nil
}

// applyDefaults fills in zero-value fields with sensible defaults.
func (c *Config) applyDefaults() {
	if c.Version == "" {
		c.Version = "1"
	}
	c.Identifiers.setDefaults()
	c.Dev.setDefaults(c.Project)

	for id, node := range c.Graph.Nodes {
		node.Pool.setDefaults()
		if node.Role == "" {
			node.Role = NodeRoleServer
		}
		c.Graph.Nodes[id] = node
	}

	if c.AI.ContextStrategy == "" {
		c.AI.ContextStrategy = "graph-first"
	}
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// ValidationError aggregates all acthur.yml validation failures.
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("acthur.yml has %d validation error(s):\n  - %s",
		len(e.Errors), joinStrings(e.Errors, "\n  - "))
}

// Validate checks the config for structural correctness.
// Returns a slice of human-readable error messages.
func (c *Config) Validate() []string {
	var errs []string

	if c.Project == "" {
		errs = append(errs, "project name is required")
	}
	if c.Version == "" {
		errs = append(errs, "version is required (currently only \"1\" is supported)")
	}

	// Validate edges reference existing nodes.
	// "proxy" is a built-in implicit kernel node (type: infra, adapter: kernel:proxy)
	// that is always present at runtime, so it is pre-seeded here.
	nodeIDs := make(map[string]bool)
	nodeIDs["proxy"] = true
	for id := range c.Graph.Nodes {
		nodeIDs[id] = true
	}

	for i, edge := range c.Graph.Edges {
		if edge.From == "" {
			errs = append(errs, fmt.Sprintf("edge[%d]: 'from' is required", i))
		} else if !nodeIDs[edge.From] {
			errs = append(errs, fmt.Sprintf("edge[%d]: 'from' node %q does not exist", i, edge.From))
		}
		if edge.To == "" {
			errs = append(errs, fmt.Sprintf("edge[%d]: 'to' is required", i))
		} else if !nodeIDs[edge.To] {
			errs = append(errs, fmt.Sprintf("edge[%d]: 'to' node %q does not exist", i, edge.To))
		}
		if edge.Type == "" {
			errs = append(errs, fmt.Sprintf("edge[%d] (%s→%s): edge type is required", i, edge.From, edge.To))
		}
		if edge.From == edge.To {
			errs = append(errs, fmt.Sprintf("edge[%d]: self-referencing edge on node %q is not allowed", i, edge.From))
		}
		if edge.Type == EdgeDataFlow && len(edge.Contracts) == 0 {
			errs = append(errs, fmt.Sprintf(
				"edge[%d] (%s→%s): data_flow edges require at least one contract\n"+
					"  Add: contracts: [contracts/<name>.contract.yml]",
				i, edge.From, edge.To,
			))
		}
		if (edge.Type == EdgeEmits || edge.Type == EdgeSubscribesTo) && len(edge.Events) == 0 {
			errs = append(errs, fmt.Sprintf(
				"edge[%d] (%s→%s): %s edges require at least one event name",
				i, edge.From, edge.To, edge.Type,
			))
		}
	}

	// Validate node types
	validNodeTypes := map[NodeType]bool{
		NodeTypeService: true, NodeTypeInfra: true,
		NodeTypePlugin: true, NodeTypeContract: true,
	}
	for id, node := range c.Graph.Nodes {
		if !validNodeTypes[node.Type] {
			errs = append(errs, fmt.Sprintf("node %q: unknown type %q", id, node.Type))
		}
		if node.Adapter == "" {
			errs = append(errs, fmt.Sprintf("node %q: adapter is required", id))
		}
	}

	return errs
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// ErrNotFound is returned when no acthur.yml is found in the directory tree.
var ErrNotFound = errors.New(
	"no acthur.yml found\n" +
		"  Run 'acthur new <project-name>' to create a new project\n" +
		"  Run 'acthur init' to adopt an existing project",
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
