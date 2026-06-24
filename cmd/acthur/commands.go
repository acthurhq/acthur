// Package cmd wires all cobra commands for the Acthur CLI.
// Every command is defined here. Commands that are not yet implemented
// return a clear "coming in Phase N" message rather than silently failing.
package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/acthur/acthur/internal/adapter"
	_ "github.com/acthur/acthur/internal/adapter/backend/gofiber"
	_ "github.com/acthur/acthur/internal/adapter/infra/postgres"
	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/doctor"
	"github.com/acthur/acthur/internal/engine"
	"github.com/acthur/acthur/internal/graph"
	"github.com/acthur/acthur/internal/output"
	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// registryResolver — wires the adapter registry to the graph.Resolver interface.
// Assembled at the CLI entrypoint so the graph engine never imports the adapter package.
// ---------------------------------------------------------------------------

type registryResolver struct{}

func (registryResolver) Resolve(key string) (graph.ResolvedAdapter, bool) {
	a, err := adapter.Resolve(key)
	if err != nil {
		return graph.ResolvedAdapter{}, false
	}
	return graph.ResolvedAdapter{Name: a.Name(), Category: string(a.Category())}, true
}

func (registryResolver) Names() []string {
	return adapter.Names()
}

func (registryResolver) Adapter(key string) (adapter.Adapter, bool) {
	a, err := adapter.Resolve(key)
	if err != nil {
		return nil, false
	}
	return a, true
}

// ---------------------------------------------------------------------------
// Root command
// ---------------------------------------------------------------------------

var verbose bool

// Root is the root cobra command. All subcommands are attached to it.
var Root = &cobra.Command{
	Use:   "acthur",
	Short: "Runtime Graph Operating System",
	Long: `
  ✦  Acthur — Runtime Graph Operating System with Pluggable Infrastructure Nodes

  Acthur orchestrates your entire application — backend, frontend, and
  infrastructure — as a live graph. It generates framework-native code,
  manages your dev environment, enforces API contracts, and deploys your
  system with one command.

  Get started:
    acthur new my-project      create a new project
    acthur init                adopt an existing project
    acthur dev                 start your development environment
    acthur deploy              deploy to production

  Docs: https://acthur.dev`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		output.SetVerbose(verbose)
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	Root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose debug output")

	// Attach all subcommands
	Root.AddCommand(newCmd)
	Root.AddCommand(initCmd)
	Root.AddCommand(devCmd)
	Root.AddCommand(buildCmd)
	Root.AddCommand(deployCmd)
	Root.AddCommand(addCmd)
	Root.AddCommand(generateCmd)
	Root.AddCommand(dbCmd)
	Root.AddCommand(contractCmd)
	Root.AddCommand(graphCmd)
	Root.AddCommand(pluginCmd)
	Root.AddCommand(serviceCmd)
	Root.AddCommand(testCmd)
	Root.AddCommand(doctorCmd)
	Root.AddCommand(mcpCmd)
	Root.AddCommand(agentCmd)
	Root.AddCommand(secretsCmd)
	Root.AddCommand(flagCmd)
	Root.AddCommand(monitorCmd)
	Root.AddCommand(versionCmd)
	Root.AddCommand(adapterCmd)
}

// Execute runs the root command. Called from main().
func Execute() {
	if err := Root.Execute(); err != nil {
		output.Error("", "%s", err)
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// Helpers shared across commands
// ---------------------------------------------------------------------------

// loadConfig loads acthur.yml from cwd upward.
// Prints a clear error and exits if not found or invalid.
func loadConfig() *config.Config {
	cwd, err := os.Getwd()
	if err != nil {
		output.Fatal(&output.ActhurError{
			Code:    output.ExitConfigError,
			Message: "cannot determine working directory",
			Cause:   err,
		})
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		output.Fatal(&output.ActhurError{
			Code:    output.ExitConfigError,
			Message: "failed to load acthur.yml",
			Problem: err.Error(),
			Fix:     "Run 'acthur new <name>' to create a project, or 'acthur init' to adopt an existing one.",
			DocsURL: "https://acthur.dev/docs/config",
		})
	}
	return cfg
}

// loadGraph loads acthur.yml and builds the graph.
func loadGraph() (*config.Config, *graph.Graph) {
	cfg := loadConfig()
	g, err := graph.Build(cfg)
	if err != nil {
		output.Fatal(&output.ActhurError{
			Code:    output.ExitGraphError,
			Message: "graph build failed",
			Problem: err.Error(),
			Fix:     "Run 'acthur graph validate' to see all graph errors.",
			DocsURL: "https://acthur.dev/docs/graph",
		})
	}
	return cfg, g
}

// notImplemented prints a "coming soon" message for Phase N commands.
func notImplemented(phase int) error {
	output.Warn("", "this command is coming in Phase %d", phase)
	output.Info("", "track progress at: https://github.com/acthur/acthur")
	return nil
}

// ---------------------------------------------------------------------------
// acthur new
// ---------------------------------------------------------------------------

var newCmd = &cobra.Command{
	Use:   "new <project-name>",
	Short: "Create a new Acthur project with the interactive wizard",
	Long: `Launches the interactive project wizard to select your stack,
plugins, and deployment target, then scaffolds a complete project.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		output.Banner()
		// Phase 9 — wizard implementation
		return notImplemented(9)
	},
}

// ---------------------------------------------------------------------------
// acthur init
// ---------------------------------------------------------------------------

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Adopt an existing project into Acthur (non-destructive)",
	Long: `Scans the current directory to detect your stack, confirms the
detected configuration, and writes acthur.yml without modifying any
existing files.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		output.Banner()
		return notImplemented(9)
	},
}

// ---------------------------------------------------------------------------
// acthur dev
// ---------------------------------------------------------------------------

var (
	devDocker bool
	devEnv    string
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Start the development environment",
	Long: `Starts all services defined in acthur.yml in the correct order,
manages their processes, starts the unified dev proxy, and enables hot reload.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, g := loadGraph()
		eng := engine.NewDevEngine(cfg, g, registryResolver{})
		return eng.Start()
	},
}

func init() {
	devCmd.Flags().BoolVar(&devDocker, "docker", false, "run all services in Docker (full containerization)")
	devCmd.Flags().StringVar(&devEnv, "env", "dev", "environment name from acthur.yml")
}

// ---------------------------------------------------------------------------
// acthur build
// ---------------------------------------------------------------------------

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build all services for production",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = loadGraph()
		return notImplemented(8)
	},
}

// ---------------------------------------------------------------------------
// acthur deploy
// ---------------------------------------------------------------------------

var (
	deployEnv    string
	deployTarget string
	deployDryRun bool
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy to production (or named environment)",
	Long: `Runs pre-deploy checks, builds all services, generates deploy manifests
from the graph, and ships to the configured target.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = loadGraph()
		return notImplemented(8)
	},
}

func init() {
	deployCmd.Flags().StringVar(&deployEnv, "env", "production", "environment to deploy")
	deployCmd.Flags().StringVar(&deployTarget, "target", "", "override deploy target")
	deployCmd.Flags().BoolVar(&deployDryRun, "dry-run", false, "show deploy plan without executing")
}

// ---------------------------------------------------------------------------
// acthur add
// ---------------------------------------------------------------------------

var addCmd = &cobra.Command{
	Use:   "add <plugin>",
	Short: "Install a plugin and generate its code",
	Long:  `Installs a plugin, runs its generators for your current adapter, and updates acthur.yml.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = loadGraph()
		return notImplemented(6)
	},
}

// ---------------------------------------------------------------------------
// acthur generate
// ---------------------------------------------------------------------------

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate code, config, or context files",
}

var generateFromContractCmd = &cobra.Command{
	Use:   "from-contract <file>",
	Short: "Generate full stack code from a contract file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = loadGraph()
		return notImplemented(7)
	},
}

var generateModelCmd = &cobra.Command{
	Use:   "model <Name>",
	Short: "Generate a model, migration, and basic CRUD",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = loadGraph()
		return notImplemented(7)
	},
}

var generateAIContextCmd = &cobra.Command{
	Use:   "ai-context",
	Short: "Generate AI coding tool context files and skills",
	Long: `Presents an interactive tool selector and generates context files
and skill files for your chosen AI coding tool (Claude Code, Cursor, etc.).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = loadGraph()
		return notImplemented(9)
	},
}

var generateCICmd = &cobra.Command{
	Use:   "ci",
	Short: "Generate CI/CD pipeline configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = loadGraph()
		return notImplemented(9)
	},
}

var generateDocsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Generate living documentation from contracts and graph",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = loadGraph()
		return notImplemented(9)
	},
}

var generateSkillCmd = &cobra.Command{
	Use:   "skill <name>",
	Short: "Generate a custom AI skill file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = loadGraph()
		return notImplemented(9)
	},
}

func init() {
	generateModelCmd.Flags().String("fields", "", "comma-separated field definitions e.g. \"name:string,age:int\"")
	generateCICmd.Flags().String("target", "github-actions", "CI target: github-actions | gitlab-ci | circleci")
	generateDocsCmd.Flags().String("target", "astro", "docs target: astro | nextra | readme | openapi")

	generateCmd.AddCommand(generateFromContractCmd)
	generateCmd.AddCommand(generateModelCmd)
	generateCmd.AddCommand(generateAIContextCmd)
	generateCmd.AddCommand(generateCICmd)
	generateCmd.AddCommand(generateDocsCmd)
	generateCmd.AddCommand(generateSkillCmd)
}

// ---------------------------------------------------------------------------
// acthur db
// ---------------------------------------------------------------------------

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database commands (migrate, seed, reset, studio)",
}

var dbMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run pending database migrations",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = loadGraph()
		return notImplemented(6)
	},
}

var dbMigrateCreateCmd = &cobra.Command{
	Use:   "migrate:create <name>",
	Short: "Create a new migration file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = loadGraph()
		return notImplemented(6)
	},
}

var dbSeedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Run database seeders",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = loadGraph()
		return notImplemented(6)
	},
}

var dbResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Drop, migrate, and seed the database",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = loadGraph()
		return notImplemented(6)
	},
}

var dbStudioCmd = &cobra.Command{
	Use:   "studio",
	Short: "Open the database studio UI in the browser",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = loadGraph()
		return notImplemented(6)
	},
}

func init() {
	dbMigrateCmd.Flags().String("env", "dev", "environment to migrate")
	dbCmd.AddCommand(dbMigrateCmd)
	dbCmd.AddCommand(dbMigrateCreateCmd)
	dbCmd.AddCommand(dbSeedCmd)
	dbCmd.AddCommand(dbResetCmd)
	dbCmd.AddCommand(dbStudioCmd)
}

// ---------------------------------------------------------------------------
// acthur contract
// ---------------------------------------------------------------------------

var contractCmd = &cobra.Command{
	Use:   "contract",
	Short: "Contract validation, diff, and management",
}

var contractValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate all contract files structurally",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = loadGraph()
		return notImplemented(4)
	},
}

var contractDiffCmd = &cobra.Command{
	Use:   "diff <contract-name>",
	Short: "Show changes vs last committed version and flag breaking changes",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = loadGraph()
		return notImplemented(4)
	},
}

var contractListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered contracts with endpoint summary",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = loadGraph()
		return notImplemented(4)
	},
}

func init() {
	contractDiffCmd.Flags().Bool("fail-on-breaking", false, "exit non-zero if any breaking changes exist")
	contractCmd.AddCommand(contractValidateCmd)
	contractCmd.AddCommand(contractDiffCmd)
	contractCmd.AddCommand(contractListCmd)
}

// ---------------------------------------------------------------------------
// acthur graph
// ---------------------------------------------------------------------------

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Graph validation, display, and visualization",
}

var graphValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the graph structure defined in acthur.yml",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(mustCwd())
		if err != nil {
			output.Fatal(&output.ActhurError{
				Code:    output.ExitConfigError,
				Message: "failed to load acthur.yml",
				Problem: err.Error(),
				Fix:     "fix the errors above and run acthur graph validate again",
			})
		}

		sp := output.NewSpinner("building graph...")
		g, err := graph.Build(cfg)
		if err != nil {
			sp.Stop(false, "graph build failed")
			output.Fatal(&output.ActhurError{
				Code:    output.ExitGraphError,
				Message: err.Error(),
			})
		}
		sp.Stop(true, "graph built")

		errs := g.Validate(registryResolver{})
		if len(errs) == 0 {
			output.Success("graph", "all %d nodes and %d edges are valid",
				len(g.Nodes()), len(g.Edges()))
			return nil
		}

		output.Error("graph", "%d validation error(s) found:", len(errs))
		fmt.Println()
		for _, e := range errs {
			severity := string(e.Severity)
			if severity == "" {
				severity = string(graph.SeverityError)
			}
			fmt.Printf("  %s  [%s] %s\n", output.SymbolError, severity, e.Message)
			if e.Rule != "" {
				fmt.Printf("     rule: %s\n", e.Rule)
			}
			if e.Fix != "" {
				fmt.Printf("     %s  fix: %s\n", output.SymbolInfo, e.Fix)
			}
			if e.DocsURL != "" {
				fmt.Printf("     %s  docs: %s\n", output.SymbolInfo, e.DocsURL)
			}
			fmt.Println()
		}
		os.Exit(int(output.ExitGraphError))
		return nil
	},
}

var graphShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print all nodes and edges in the graph",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, g := loadGraph()
		output.Header(fmt.Sprintf("Graph: %s", cfg.Project))
		fmt.Print(g.Summary())
		return nil
	},
}

var graphVisualizeCmd = &cobra.Command{
	Use:   "visualize",
	Short: "Open a Mermaid graph diagram in the browser",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = loadGraph()
		return notImplemented(9)
	},
}

func init() {
	graphCmd.AddCommand(graphValidateCmd)
	graphCmd.AddCommand(graphShowCmd)
	graphCmd.AddCommand(graphVisualizeCmd)
}

// ---------------------------------------------------------------------------
// acthur plugin
// ---------------------------------------------------------------------------

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Plugin management",
}

var pluginListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed and available plugins",
	RunE: func(cmd *cobra.Command, args []string) error {
		return notImplemented(5)
	},
}

var pluginRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an installed plugin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return notImplemented(5)
	},
}

func init() {
	pluginCmd.AddCommand(pluginListCmd)
	pluginCmd.AddCommand(pluginRemoveCmd)
}

// ---------------------------------------------------------------------------
// acthur service
// ---------------------------------------------------------------------------

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage individual services in the graph",
}

func init() {
	serviceCmd.AddCommand(&cobra.Command{
		Use:   "add <infra>",
		Short: "Add an infra node to the graph",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return notImplemented(3) },
	})
	serviceCmd.AddCommand(&cobra.Command{
		Use:   "logs <name>",
		Short: "Stream logs from a service",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return notImplemented(3) },
	})
	serviceCmd.AddCommand(&cobra.Command{
		Use:   "restart <name>",
		Short: "Restart a specific service",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return notImplemented(3) },
	})
	serviceCmd.AddCommand(&cobra.Command{
		Use:   "health",
		Short: "Show health status of all graph nodes",
		RunE:  func(cmd *cobra.Command, args []string) error { return notImplemented(3) },
	})
}

// ---------------------------------------------------------------------------
// acthur test
// ---------------------------------------------------------------------------

var testCmd = &cobra.Command{
	Use:   "test [service]",
	Short: "Run tests across the project",
	Long: `Run unit tests for all services, or specify a service name.
Use flags to run integration, contract, E2E, or load tests.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = loadGraph()
		return notImplemented(6)
	},
}

func init() {
	testCmd.Flags().Bool("integration", false, "run integration tests (requires Docker)")
	testCmd.Flags().Bool("contract", false, "run contract compliance tests")
	testCmd.Flags().Bool("e2e", false, "run end-to-end tests (starts full stack)")
	testCmd.Flags().Bool("load", false, "run k6 load tests")
	testCmd.Flags().Bool("coverage", false, "generate coverage report")
	testCmd.Flags().Bool("watch", false, "re-run on file change")
}

// ---------------------------------------------------------------------------
// acthur doctor
// ---------------------------------------------------------------------------

var doctorFix bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check environment health and auto-fix issues",
	Long: `Checks whether all tools required by your acthur.yml stack are
installed and at the correct versions. Use --fix to auto-install what can
be automatically resolved.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		output.Banner()

		// Load config if present — doctor works without it too
		var cfg *config.Config
		cwd, _ := os.Getwd()
		if config.Exists(cwd) {
			c, err := config.Load(cwd)
			if err == nil {
				cfg = c
			}
		}

		r := doctor.Run(cfg)
		doctor.Print(r)

		if doctorFix {
			if !r.HasFixes {
				output.Info("doctor", "nothing to fix")
				return nil
			}
			output.Header("Auto-fixing issues")
			results := doctor.Fix(r)
			failed := 0
			for _, err := range results {
				if err != nil {
					failed++
				}
			}
			fmt.Println()
			if failed == 0 {
				output.Success("doctor", "all issues resolved")
			} else {
				output.Warn("doctor", "%d issue(s) could not be auto-fixed — see guidance above", failed)
			}
		}
		return nil
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "auto-fix resolvable issues")
}

// ---------------------------------------------------------------------------
// acthur mcp
// ---------------------------------------------------------------------------

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "MCP server for AI coding tool integration",
}

func init() {
	mcpCmd.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "Start the Acthur MCP server for AI tools",
		RunE:  func(cmd *cobra.Command, args []string) error { return notImplemented(9) },
	})
}

// ---------------------------------------------------------------------------
// acthur agent
// ---------------------------------------------------------------------------

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "AI-powered commands with full graph context",
}

func init() {
	agentCmd.AddCommand(&cobra.Command{
		Use:   "explain <question>",
		Short: "Explain part of the system",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return notImplemented(9) },
	})
	agentCmd.AddCommand(&cobra.Command{
		Use:   "generate <feature>",
		Short: "Generate a feature with full graph context",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return notImplemented(9) },
	})
	agentCmd.AddCommand(&cobra.Command{
		Use:   "diagnose <problem>",
		Short: "Diagnose a runtime problem",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return notImplemented(9) },
	})
	agentCmd.AddCommand(&cobra.Command{
		Use:   "review",
		Short: "Review the current git diff",
		RunE:  func(cmd *cobra.Command, args []string) error { return notImplemented(9) },
	})
	agentCmd.AddCommand(&cobra.Command{
		Use:   "document <path>",
		Short: "Generate documentation for a path",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return notImplemented(9) },
	})
}

// ---------------------------------------------------------------------------
// acthur secrets
// ---------------------------------------------------------------------------

var secretsCmd = &cobra.Command{
	Use:   "secrets",
	Short: "Secret management commands",
}

func init() {
	secretsCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List secret keys (not values)",
		RunE:  func(cmd *cobra.Command, args []string) error { return notImplemented(9) },
	})
	secretsCmd.AddCommand(&cobra.Command{
		Use:   "rotate <key>",
		Short: "Rotate a secret and restart affected services",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return notImplemented(9) },
	})
}

// ---------------------------------------------------------------------------
// acthur flag
// ---------------------------------------------------------------------------

var flagCmd = &cobra.Command{
	Use:   "flag",
	Short: "Feature flag management",
}

func init() {
	flagCmd.AddCommand(&cobra.Command{
		Use:   "create <name>",
		Short: "Create a new feature flag",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return notImplemented(9) },
	})
	flagCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all feature flags",
		RunE:  func(cmd *cobra.Command, args []string) error { return notImplemented(9) },
	})
	flagCmd.AddCommand(&cobra.Command{
		Use:   "toggle <name>",
		Short: "Toggle a feature flag on or off",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return notImplemented(9) },
	})
}

// ---------------------------------------------------------------------------
// acthur monitor
// ---------------------------------------------------------------------------

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Open the monitoring dashboard",
	RunE: func(cmd *cobra.Command, args []string) error {
		return notImplemented(9)
	},
}

// ---------------------------------------------------------------------------
// acthur version
// ---------------------------------------------------------------------------

// Version is set at build time via ldflags:
// go build -ldflags "-X github.com/acthur/acthur/cmd.Version=0.1.0"
var Version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the Acthur version",
	Run: func(cmd *cobra.Command, args []string) {
		output.Table([][2]string{
			{"Version:", Version},
			{"Docs:", "https://acthur.dev"},
			{"Source:", "https://github.com/acthur/acthur"},
		})
		fmt.Println()
	},
}

// ---------------------------------------------------------------------------
// acthur adapter
// ---------------------------------------------------------------------------

var adapterCmd = &cobra.Command{
	Use:   "adapter",
	Short: "Adapter introspection",
}

var adapterListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered adapters grouped by category",
	RunE: func(cmd *cobra.Command, args []string) error {
		all := adapter.All()

		// Group by category.
		grouped := map[adapter.Category][]adapter.Adapter{}
		for _, a := range all {
			grouped[a.Category()] = append(grouped[a.Category()], a)
		}

		// Collect and sort category keys.
		categories := make([]string, 0, len(grouped))
		for c := range grouped {
			categories = append(categories, string(c))
		}
		sort.Strings(categories)

		for _, cat := range categories {
			fmt.Printf("\n  %s\n", cat)
			adapters := grouped[adapter.Category(cat)]
			sort.Slice(adapters, func(i, j int) bool {
				return adapters[i].Name() < adapters[j].Name()
			})
			for _, a := range adapters {
				caps := adapter.CapabilitiesOf(a)
				capNames := make([]string, len(caps))
				for i, c := range caps {
					capNames[i] = string(c)
				}
				fmt.Printf("    %-20s  %v\n", a.Name(), capNames)
			}
		}
		fmt.Println()
		return nil
	},
}

var adapterInspectCmd = &cobra.Command{
	Use:   "inspect <key>",
	Short: "Show adapter name, category, and capability table",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		a, err := adapter.Resolve(key)
		if err != nil {
			return fmt.Errorf("adapter %q not found — run 'acthur adapter list' to see available adapters", key)
		}

		fmt.Printf("\n  Name:     %s\n", a.Name())
		fmt.Printf("  Category: %s\n\n", a.Category())

		allCaps := []adapter.Capability{
			adapter.CapabilityScaffold,
			adapter.CapabilityRun,
			adapter.CapabilityContainer,
			adapter.CapabilityConnectable,
			adapter.CapabilityMigrate,
			adapter.CapabilityDeploy,
		}
		caps := adapter.CapabilitiesOf(a)
		capSet := make(map[adapter.Capability]bool)
		for _, c := range caps {
			capSet[c] = true
		}

		fmt.Println("  Capabilities:")
		for _, c := range allCaps {
			symbol := "✗"
			if capSet[c] {
				symbol = "✓"
			}
			fmt.Printf("    %s  %s\n", symbol, c)
		}
		fmt.Println()
		return nil
	},
}

func init() {
	adapterCmd.AddCommand(adapterListCmd)
	adapterCmd.AddCommand(adapterInspectCmd)
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

func mustCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		output.Fatal(&output.ActhurError{
			Code:    output.ExitGeneral,
			Message: "cannot determine working directory",
			Cause:   err,
		})
	}
	return cwd
}
