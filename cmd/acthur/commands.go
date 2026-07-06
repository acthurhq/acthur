// Package cmd wires all cobra commands for the Acthur CLI.
// Every command is defined here. Commands that are not yet implemented
// return a clear "coming in Phase N" message rather than silently failing.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/acthur/acthur/internal/adapter"
	_ "github.com/acthur/acthur/internal/adapter/backend/chi"
	_ "github.com/acthur/acthur/internal/adapter/backend/fastify"
	_ "github.com/acthur/acthur/internal/adapter/backend/gin"
	_ "github.com/acthur/acthur/internal/adapter/backend/gofiber"
	_ "github.com/acthur/acthur/internal/adapter/backend/rustaxum"
	_ "github.com/acthur/acthur/internal/adapter/frontend/astro"
	_ "github.com/acthur/acthur/internal/adapter/frontend/next"
	_ "github.com/acthur/acthur/internal/adapter/infra/postgres"
	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/contract"
	"github.com/acthur/acthur/internal/deploy"
	"github.com/acthur/acthur/internal/doctor"
	"github.com/acthur/acthur/internal/engine"
	"github.com/acthur/acthur/internal/graph"
	"github.com/acthur/acthur/internal/health"
	"github.com/acthur/acthur/internal/output"
	"github.com/acthur/acthur/internal/plugin"
	_ "github.com/acthur/acthur/internal/plugin/builtin/auth"
	"github.com/acthur/acthur/internal/plugin/builtin/migrations"
	_ "github.com/acthur/acthur/internal/plugin/builtin/multitenancy"
	_ "github.com/acthur/acthur/internal/plugin/builtin/rbac"
	_ "github.com/acthur/acthur/internal/plugin/builtin/testplugin"
	"github.com/acthur/acthur/internal/process"
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
// contractPathResolver — wires internal/contract's project-relative loading
// convention (contracts/<name>.contract.yml) to the graph.ContractResolver
// interface. Assembled at the CLI entrypoint so the graph engine never
// imports internal/contract (ADR 0005, same pattern as registryResolver).
// ---------------------------------------------------------------------------

type contractPathResolver struct {
	root string
}

func (r contractPathResolver) Resolve(name string) (string, bool) {
	path := contractFilePath(r.root, name)
	if _, err := contract.ParseFile(path); err != nil {
		return path, false
	}
	return path, true
}

// contractFilePath computes the expected file path for a data_flow edge's
// contract entry. A bare name (e.g. "users") resolves to
// contracts/users.contract.yml under root, per the Phase 4 convention.
// An entry that already looks like a path (contains a '/' or already ends
// in .contract.yml/.contract.yaml) is treated as relative-to-root as-is —
// this keeps existing fixtures authored with full paths working.
func contractFilePath(root, name string) string {
	if strings.Contains(name, "/") ||
		strings.HasSuffix(name, ".contract.yml") ||
		strings.HasSuffix(name, ".contract.yaml") {
		return filepath.Join(root, name)
	}
	return filepath.Join(root, "contracts", name+".contract.yml")
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
// bootstrapPlugins makes plugin-registered commands dispatchable: cobra
// resolves the command word before any RunE runs, so when the working
// directory holds a project with a plugins list, plugins are loaded (and
// their commands attached to Root) before Execute. Failures are deliberately
// silent here — commands that need the graph re-load it and surface pointed
// errors themselves; a bare `acthur --help` outside a project must not fail.
func bootstrapPlugins() {
	cfg, err := config.Load(".")
	if err != nil || len(cfg.Plugins) == 0 {
		return
	}
	g, err := graph.Build(cfg)
	if err != nil {
		return
	}
	if err := loadPlugins(cfg, g); err != nil {
		return
	}
	g.Freeze()
	bootstrapped = bootstrapResult{cfg: cfg, g: g, ok: true}
}

// bootstrapResult caches the config+graph assembled by bootstrapPlugins so
// loadGraph can reuse them: plugins register hooks/commands on process-global
// state (kernelBus, Root), so loading them a second time in the same process
// would double every registration.
type bootstrapResult struct {
	cfg *config.Config
	g   *graph.Graph
	ok  bool
}

var bootstrapped bootstrapResult

func Execute() {
	bootstrapPlugins()
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

// loadGraph loads acthur.yml, builds the graph, and loads cfg.Plugins
// through the real KernelAPI (ADR 0005 resolver-injection pattern) before
// sealing the graph (ADR 0003 two-phase lifecycle). Plugins may mutate the
// graph (AddNode/AddEdge) only during this window — Freeze() below ends it.
func loadGraph() (*config.Config, *graph.Graph) {
	// bootstrapPlugins already assembled and sealed this project's graph
	// (plugins loaded exactly once); reuse it rather than re-registering
	// every plugin hook/command on the process-global bus and Root.
	if bootstrapped.ok {
		return bootstrapped.cfg, bootstrapped.g
	}

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

	if err := loadPlugins(cfg, g); err != nil {
		output.Fatal(&output.ActhurError{
			Code:    output.ExitPluginError,
			Message: "plugin loading failed",
			Problem: err.Error(),
			Fix:     "Run 'acthur plugin list' to see available plugins, or remove the offending entry from acthur.yml's plugins list.",
		})
	}

	g.Freeze()
	return cfg, g
}

// ---------------------------------------------------------------------------
// Plugin loading — CLI assembly wiring for internal/plugin (Phase 5, ADR 0005/0003)
// ---------------------------------------------------------------------------

// kernelBus is the single kernel event bus shared by every plugin loaded
// during this process's lifetime. Constructed once at CLI assembly.
var kernelBus = plugin.NewBus()

// kernelAPI is the KernelAPI handed to plugins by the most recent
// loadPlugins call; later phases (and `acthur generate`) consume its
// generator/schema/middleware registries.
var kernelAPI *plugin.KernelAPIImpl

// loadedPlugins holds the result of the most recent loadPlugins call, used
// by `acthur plugin list` to show which plugins are active for this project.
var loadedPlugins []*plugin.LoadedPlugin

// loadPlugins resolves cfg.Plugins against the plugin registry and loads
// them, in dependency order, against a real KernelAPI bound to g and Root.
// An unknown plugin name fails before anything is loaded or started, with a
// pointed error naming the plugin and listing every registered plugin.
//
// Graph mutation via the KernelAPI (AddNode/AddEdge) is only valid here —
// the caller must seal g (Freeze) immediately after this returns.
func loadPlugins(cfg *config.Config, g *graph.Graph) error {
	loadedPlugins = nil
	if len(cfg.Plugins) == 0 {
		return nil
	}

	names := make([]string, len(cfg.Plugins))
	for i, p := range cfg.Plugins {
		names[i] = p.Name
	}

	if err := checkPluginsKnown(names); err != nil {
		return err
	}

	k := plugin.NewKernelAPI(kernelBus, g, registerPluginCommand, pluginLog)
	kernelAPI = k

	loaded, err := plugin.Load(names, kernelBus, k)
	if err != nil {
		return err
	}
	loadedPlugins = loaded
	return nil
}

// checkPluginsKnown returns a pointed error naming the first unknown plugin
// in names and listing every plugin registered in the plugin registry.
func checkPluginsKnown(names []string) error {
	available := plugin.All()
	known := make(map[string]bool, len(available))
	availableNames := make([]string, 0, len(available))
	for _, p := range available {
		known[p.Name()] = true
		availableNames = append(availableNames, p.Name())
	}
	sort.Strings(availableNames)

	for _, name := range names {
		if known[name] {
			continue
		}
		if len(availableNames) == 0 {
			return fmt.Errorf("unknown plugin %q — no plugins are registered in this build", name)
		}
		return fmt.Errorf("unknown plugin %q — available plugins: %s", name, strings.Join(availableNames, ", "))
	}
	return nil
}

// registerPluginCommand adapts a plugin.CLICommand into a *cobra.Command
// and attaches it to Root, so it appears in `acthur --help` and is runnable.
func registerPluginCommand(cmd plugin.CLICommand) {
	cc := &cobra.Command{
		Use:   cmd.Use,
		Short: cmd.Short,
		Long:  cmd.Long,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmd.Run == nil {
				return nil
			}
			return cmd.Run(args)
		},
	}
	for _, f := range cmd.Flags {
		cc.Flags().StringP(f.Name, f.Short, f.Default, f.Usage)
	}
	Root.AddCommand(cc)
}

// pluginLog routes plugin.KernelAPI.Log calls through the kernel's output
// system, scoped under the "plugin" prefix.
func pluginLog(level plugin.LogLevel, format string, args ...any) {
	switch level {
	case plugin.LogWarn:
		output.Warn(output.PrefixPlugin, format, args...)
	case plugin.LogError:
		output.Error(output.PrefixPlugin, format, args...)
	case plugin.LogDebug:
		output.Debug(output.PrefixPlugin, format, args...)
	default:
		output.Info(output.PrefixPlugin, format, args...)
	}
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

var (
	newAdapter string
	newDB      bool
	newModule  string
)

var newCmd = &cobra.Command{
	Use:   "new <project-name>",
	Short: "Create a new Acthur project with the interactive wizard",
	Long: `Launches the interactive project wizard to select your backend
adapter, whether to include a database, and your Go module prefix, then
scaffolds a complete project into a new <project-name>/ directory.

Pass --adapter, --db, and --module to skip the wizard entirely — required
when stdin is not a terminal (CI, scripts, tests).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		output.Banner()
		wi := wizardInput{
			Adapter:    newAdapter,
			AdapterSet: cmd.Flags().Changed("adapter"),
			DB:         newDB,
			DBSet:      cmd.Flags().Changed("db"),
			Module:     newModule,
			ModuleSet:  cmd.Flags().Changed("module"),
		}
		result, err := runNew(mustCwd(), args[0], wi, cmd.InOrStdin(), cmd.OutOrStdout(), isInteractive())
		if err != nil {
			return err
		}
		printScaffoldResult(result)
		return nil
	},
}

func init() {
	newCmd.Flags().StringVar(&newAdapter, "adapter", "", "backend adapter to scaffold (e.g. go:fiber)")
	newCmd.Flags().BoolVar(&newDB, "db", false, "include a db:postgres infra node")
	newCmd.Flags().StringVar(&newModule, "module", "", "Go module prefix for scaffolded code (defaults to the project name)")
}

// ---------------------------------------------------------------------------
// acthur init
// ---------------------------------------------------------------------------

var (
	initAdapter string
	initDB      bool
	initModule  string
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Adopt an existing project into Acthur (non-destructive)",
	Long: `Scaffolds an Acthur project into the current directory — the same
wizard as 'acthur new', targeting cwd instead of a new subdirectory. Refuses
to run if acthur.yml already exists here.

Pass --adapter, --db, and --module to skip the wizard entirely — required
when stdin is not a terminal (CI, scripts, tests).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		output.Banner()
		wi := wizardInput{
			Adapter:    initAdapter,
			AdapterSet: cmd.Flags().Changed("adapter"),
			DB:         initDB,
			DBSet:      cmd.Flags().Changed("db"),
			Module:     initModule,
			ModuleSet:  cmd.Flags().Changed("module"),
		}
		result, err := runInit(mustCwd(), wi, cmd.InOrStdin(), cmd.OutOrStdout(), isInteractive())
		if err != nil {
			return err
		}
		printScaffoldResult(result)
		return nil
	},
}

func init() {
	initCmd.Flags().StringVar(&initAdapter, "adapter", "", "backend adapter to scaffold (e.g. go:fiber)")
	initCmd.Flags().BoolVar(&initDB, "db", false, "include a db:postgres infra node")
	initCmd.Flags().StringVar(&initModule, "module", "", "Go module prefix for scaffolded code (defaults to the directory name)")
}

// ---------------------------------------------------------------------------
// acthur dev
// ---------------------------------------------------------------------------

var (
	devDocker     bool
	devEnv        string
	devStrict     bool
	devSkipDoctor bool
	devWriteHosts bool
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Start the development environment",
	Long: `Starts all services defined in acthur.yml in the correct order,
manages their processes, starts the unified dev proxy, and enables hot reload.

Before starting anything, acthur dev runs the same checks as acthur doctor
and aborts with a pointed message if a required tool is missing. Pass
--skip-doctor to bypass this preflight.

With --strict, the dev proxy blocks (422) any data_flow request that
violates its edge's contract, and blocks (502) any backend response that
violates the contract's Output schema, instead of logging the violation
and forwarding it.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := loadConfig()

		if !devSkipDoctor {
			if err := runDevDoctorPreflight(cfg); err != nil {
				return err
			}
		}

		_, g := loadGraph()
		reg, err := contract.LoadDir(cfg.RootDir)
		if err != nil {
			return fmt.Errorf("loading contracts: %w", err)
		}
		eng := engine.NewDevEngine(cfg, g, registryResolver{},
			engine.WithStrict(devStrict),
			engine.WithContractRegistry(reg),
			engine.WithBus(kernelBus),
			engine.WithWriteHosts(devWriteHosts),
		)
		return eng.Start()
	},
}

func init() {
	devCmd.Flags().BoolVar(&devDocker, "docker", false, "run all services in Docker (full containerization)")
	devCmd.Flags().StringVar(&devEnv, "env", "dev", "environment name from acthur.yml")
	devCmd.Flags().BoolVar(&devStrict, "strict", false, "block (422 requests / 502 responses) data_flow traffic that violates its contract instead of logging and forwarding")
	devCmd.Flags().BoolVar(&devSkipDoctor, "skip-doctor", false, "skip the doctor preflight check")
	devCmd.Flags().BoolVar(&devWriteHosts, "write-hosts", false, "write missing dev domains to /etc/hosts (requires permission to write it)")
}

// runDevDoctorPreflight runs the same checks as `acthur doctor` against cfg
// and aborts with a pointed message if any required tool is missing or
// failed. Extracted from devCmd.RunE so it's directly testable without
// spinning up the full dev engine.
func runDevDoctorPreflight(cfg *config.Config) error {
	return devDoctorPreflight(cfg, doctor.Run)
}

// devDoctorPreflight is runDevDoctorPreflight with the doctor.Run call
// injected, so tests can assert the abort/pass decision against a canned
// *doctor.Result without depending on which tools happen to be installed on
// the machine running the test.
func devDoctorPreflight(cfg *config.Config, run func(*config.Config) *doctor.Result) error {
	r := run(cfg)
	if !doctor.HasBlockingFailures(r) {
		return nil
	}
	doctor.Print(r)
	return fmt.Errorf("environment checks failed — fix the issues above (or run 'acthur doctor --fix'), or pass --skip-doctor to bypass")
}

// ---------------------------------------------------------------------------
// acthur build
// ---------------------------------------------------------------------------

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build all services for production",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, g := loadGraph()
		if err := runBuild(mustCwd(), g, os.Stdout); err != nil {
			return err
		}
		output.Success(output.PrefixKernel, "build complete")
		return nil
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
		plan, err := runDeploy(mustCwd(), deployEnv, deployTarget, deployDryRun, deploy.DockerRunner)
		if deployDryRun && err == nil {
			for _, line := range plan {
				output.Info("deploy", "%s", line)
			}
		}
		return err
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

var addNodeFlag string

var addCmd = &cobra.Command{
	Use:   "add <plugin>",
	Short: "Install a plugin and generate its code",
	Long:  `Installs a plugin, runs its generators for your current adapter, and updates acthur.yml.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		summary, err := runAdd(mustCwd(), args[0], addNodeFlag)
		if err != nil {
			return err
		}
		printAddSummary(summary)
		return nil
	},
}

func init() {
	addCmd.Flags().StringVar(&addNodeFlag, "node", "", "target a specific node instead of every go:fiber service node")
}

// printAddSummary reports what runAdd did: whether the plugin was newly
// added to acthur.yml, and the written/skipped/merged status of every file
// its generator produced.
func printAddSummary(s *addSummary) {
	if s.AlreadyHad {
		output.Info(output.PrefixPlugin, "%q is already in acthur.yml's plugins list", s.Plugin)
	} else {
		output.Success(output.PrefixPlugin, "added %q to acthur.yml's plugins list", s.Plugin)
	}

	output.Header(fmt.Sprintf("Generated by %s", s.Plugin))
	for _, r := range s.Results {
		switch r.Status {
		case addFileWritten:
			output.Success(output.PrefixPlugin, "%-10s %s (%s)", "written", r.Path, r.NodeID)
		case addFileMerged:
			output.Info(output.PrefixPlugin, "%-10s %s (%s)", "merged", r.Path, r.NodeID)
		case addFileSkipped:
			output.Warn(output.PrefixPlugin, "%-10s %s (%s) — already exists", "skipped", r.Path, r.NodeID)
		}
	}
}

// ---------------------------------------------------------------------------
// acthur generate
// ---------------------------------------------------------------------------

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate code, config, or context files",
}

var generateNodeFlag string

var generateFromContractCmd = &cobra.Command{
	Use:   "from-contract <file>",
	Short: "Generate full stack code from a contract file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		results, err := runGenerateFromContract(mustCwd(), args[0], generateNodeFlag)
		if err != nil {
			return err
		}
		printGenerateResults(results)
		return nil
	},
}

var generateModelCmd = &cobra.Command{
	Use:   "model <Name> [field:type ...]",
	Short: "Generate a model, migration, and test",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		results, err := runGenerateModel(mustCwd(), args[0], args[1:], generateNodeFlag)
		if err != nil {
			return err
		}
		printGenerateResults(results)
		return nil
	},
}

func init() {
	generateFromContractCmd.Flags().StringVar(&generateNodeFlag, "node", "", "target a specific node instead of every go:fiber service node")
	generateModelCmd.Flags().StringVar(&generateNodeFlag, "node", "", "target a specific node instead of every go:fiber service node")
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
		_, g := loadGraph()
		url, err := migrations.DatabaseURL(g)
		if err != nil {
			return err
		}
		if err := migrations.Migrate(mustCwd(), url); err != nil {
			return err
		}
		output.Success(output.PrefixKernel, "migrations applied")
		return nil
	},
}

var dbRollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Roll back the most recently applied migration",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, g := loadGraph()
		url, err := migrations.DatabaseURL(g)
		if err != nil {
			return err
		}
		if err := migrations.Rollback(mustCwd(), url); err != nil {
			return err
		}
		output.Success(output.PrefixKernel, "rolled back one migration")
		return nil
	},
}

var dbStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current migration version and dirty state",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, g := loadGraph()
		url, err := migrations.DatabaseURL(g)
		if err != nil {
			return err
		}
		version, dirty, err := migrations.Status(mustCwd(), url)
		if err != nil {
			return err
		}
		if version == 0 {
			output.Info(output.PrefixKernel, "no migrations applied yet")
			return nil
		}
		state := "clean"
		if dirty {
			state = "dirty"
		}
		output.Info(output.PrefixKernel, "version %d (%s)", version, state)
		return nil
	},
}

// runDbCreate is the shared implementation of `db create <name>` and
// `db migrate:create <name>` — the PRD lists both spellings (§19.4) as the
// same operation (write the next-numbered empty up/down migration pair), so
// both cobra commands share this one RunE rather than duplicating it.
func runDbCreate(cmd *cobra.Command, args []string) error {
	up, down, err := migrations.CreateNext(mustCwd(), args[0])
	if err != nil {
		return err
	}
	output.Success(output.PrefixKernel, "created %s", up)
	output.Success(output.PrefixKernel, "created %s", down)
	return nil
}

var dbCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Write a new numbered empty migration up/down pair",
	Args:  cobra.ExactArgs(1),
	RunE:  runDbCreate,
}

var dbMigrateCreateCmd = &cobra.Command{
	Use:   "migrate:create <name>",
	Short: "Create a new migration file (alias of `db create`)",
	Args:  cobra.ExactArgs(1),
	RunE:  runDbCreate,
}

var dbSeedFile string

var dbSeedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Run database seeders",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, g := loadGraph()
		url, err := migrations.DatabaseURL(g)
		if err != nil {
			return err
		}
		root := mustCwd()
		if dbSeedFile != "" {
			if err := migrations.SeedFile(root, url, dbSeedFile); err != nil {
				return err
			}
			output.Success(output.PrefixKernel, "ran seed file %s", dbSeedFile)
			return nil
		}
		if err := migrations.Seed(root, url); err != nil {
			return err
		}
		output.Success(output.PrefixKernel, "database seeded")
		return nil
	},
}

var dbResetYes bool

var dbResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Drop, migrate, and seed the database",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !dbResetYes {
			return fmt.Errorf("acthur db reset is destructive (rolls back every migration before re-applying and seeding) — re-run with --yes to confirm")
		}
		_, g := loadGraph()
		url, err := migrations.DatabaseURL(g)
		if err != nil {
			return err
		}
		if err := migrations.Reset(mustCwd(), url, migrations.ResetSteps{}); err != nil {
			return err
		}
		output.Success(output.PrefixKernel, "database reset (migrations reapplied + seeded)")
		return nil
	},
}

var dbStudioCmd = &cobra.Command{
	Use:   "studio",
	Short: "Open the database studio UI in the browser",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, g := loadGraph()
		url, err := migrations.DatabaseURL(g)
		if err != nil {
			return err
		}
		port, err := freeLocalPort()
		if err != nil {
			return err
		}
		return runDbStudio(url, deploy.DockerRunner, port, func(studioURL string) {
			output.Success(output.PrefixKernel, "db studio running at %s", studioURL)
			output.Info(output.PrefixKernel, "press Ctrl+C to stop")
		}, waitForInterrupt)
	},
}

func init() {
	dbMigrateCmd.Flags().String("env", "dev", "environment to migrate")
	dbSeedCmd.Flags().StringVar(&dbSeedFile, "file", "", "run exactly one seed file instead of the whole seeds/ directory")
	dbResetCmd.Flags().BoolVar(&dbResetYes, "yes", false, "confirm the destructive reset")
	dbCmd.AddCommand(dbMigrateCmd)
	dbCmd.AddCommand(dbRollbackCmd)
	dbCmd.AddCommand(dbStatusCmd)
	dbCmd.AddCommand(dbCreateCmd)
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
		if code := runContractValidate(mustCwd()); code != 0 {
			os.Exit(code)
		}
		return nil
	},
}

var contractListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered contracts with endpoint summary",
	RunE: func(cmd *cobra.Command, args []string) error {
		if code := runContractList(mustCwd()); code != 0 {
			os.Exit(code)
		}
		return nil
	},
}

var contractShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show endpoint detail for a single contract",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if code := runContractShow(mustCwd(), args[0]); code != 0 {
			os.Exit(code)
		}
		return nil
	},
}

var contractDiffCmd = &cobra.Command{
	Use:   "diff <old-file> <new-file>",
	Short: "Diff two contract files and flag breaking changes",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if code := runContractDiff(args[0], args[1]); code != 0 {
			os.Exit(code)
		}
		return nil
	},
}

func init() {
	contractCmd.AddCommand(contractValidateCmd)
	contractCmd.AddCommand(contractDiffCmd)
	contractCmd.AddCommand(contractListCmd)
	contractCmd.AddCommand(contractShowCmd)
}

// runContractValidate loads and structurally validates every contract under
// contracts/ in root (LoadDir folds structural Validate() into Register(),
// so a load error already means "invalid"). Returns 0 on success — including
// the "no contracts found" case, since an optional contracts/ dir is not a
// failure — or output.ExitContractError on any load/parse/structural error.
func runContractValidate(root string) int {
	reg, err := contract.LoadDir(root)
	if err != nil {
		output.Error(output.PrefixContract, "%s", err)
		return int(output.ExitContractError)
	}

	all := reg.All()
	if len(all) == 0 {
		output.Warn(output.PrefixContract, "no contracts found under %s", filepath.Join(root, "contracts"))
		return 0
	}

	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	for _, c := range all {
		output.Success(output.PrefixContract, "%s@%s is valid (%d endpoint(s), %s)", c.Name, c.Version, len(c.Endpoints), c.Transport)
	}
	return 0
}

// runContractList prints name, version, transport, and endpoint count for
// every contract loadable from root.
func runContractList(root string) int {
	reg, err := contract.LoadDir(root)
	if err != nil {
		output.Error(output.PrefixContract, "%s", err)
		return int(output.ExitContractError)
	}

	all := reg.All()
	if len(all) == 0 {
		output.Warn(output.PrefixContract, "no contracts found under %s", filepath.Join(root, "contracts"))
		return 0
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	output.Header("Contracts")
	for _, c := range all {
		output.Info(output.PrefixContract, "%-20s v%-6s %-8s %d endpoint(s)", c.Name, c.Version, c.Transport, len(c.Endpoints))
	}
	return 0
}

// runContractShow prints endpoint detail for the latest version of the
// named contract loadable from root.
func runContractShow(root, name string) int {
	reg, err := contract.LoadDir(root)
	if err != nil {
		output.Error(output.PrefixContract, "%s", err)
		return int(output.ExitContractError)
	}

	c, err := reg.GetLatest(name)
	if err != nil {
		output.Error(output.PrefixContract, "%s", err)
		return int(output.ExitContractError)
	}

	output.Header(fmt.Sprintf("%s@%s (%s)", c.Name, c.Version, c.Transport))
	for _, ep := range c.Endpoints {
		output.Info(output.PrefixContract, "%-6s %-30s %s", ep.Method, ep.Path, ep.ID)
		if ep.Auth != "" {
			output.Info(output.PrefixContract, "  auth: %s", ep.Auth)
		}
		for field, typ := range ep.Input {
			output.Info(output.PrefixContract, "  in    %s: %s", field, typ)
		}
		for field, typ := range ep.Output {
			output.Info(output.PrefixContract, "  out   %s: %s", field, typ)
		}
	}
	return 0
}

// runContractDiff parses two contract files and prints the classified
// changes between them. Returns output.ExitContractBreak when any change is
// breaking, 0 otherwise (including "no changes").
func runContractDiff(oldPath, newPath string) int {
	oldC, err := contract.ParseFile(oldPath)
	if err != nil {
		output.Error(output.PrefixContract, "%s", err)
		return int(output.ExitContractError)
	}
	newC, err := contract.ParseFile(newPath)
	if err != nil {
		output.Error(output.PrefixContract, "%s", err)
		return int(output.ExitContractError)
	}

	result := contract.Diff(oldC, newC)
	if len(result.Changes) == 0 {
		output.Success(output.PrefixContract, "no changes between %s and %s", oldPath, newPath)
		return 0
	}

	for _, ch := range result.Changes {
		switch ch.Type {
		case contract.ChangeBreaking:
			output.Error(output.PrefixContract, "[breaking] %s", ch.Description)
		case contract.ChangeNonBreaking:
			output.Warn(output.PrefixContract, "[non-breaking] %s", ch.Description)
		default:
			output.Info(output.PrefixContract, "[info] %s", ch.Description)
		}
	}

	if result.HasBreaking {
		return int(output.ExitContractBreak)
	}
	return 0
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

		errs := g.Validate(registryResolver{}, contractPathResolver{root: mustCwd()})
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
		_, _ = loadGraph()

		output.Header("Loaded plugins")
		if len(loadedPlugins) == 0 {
			output.Info(output.PrefixPlugin, "no plugins loaded — add entries under 'plugins:' in acthur.yml")
		} else {
			for _, lp := range loadedPlugins {
				output.Info(output.PrefixPlugin, "%-20s v%s", lp.Plugin.Name(), lp.Plugin.Version())
			}
		}

		all := plugin.All()
		output.Header("Available plugins")
		if len(all) == 0 {
			output.Info(output.PrefixPlugin, "no plugins registered in this build")
			return nil
		}
		sort.Slice(all, func(i, j int) bool { return all[i].Name() < all[j].Name() })
		for _, p := range all {
			output.Info(output.PrefixPlugin, "%-20s v%s", p.Name(), p.Version())
		}
		return nil
	},
}

var pluginRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an installed plugin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		summary, err := runPluginRemove(mustCwd(), args[0])
		if err != nil {
			return err
		}
		output.Success(output.PrefixPlugin, "removed %q from acthur.yml's plugins list", summary.Plugin)
		return nil
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

var serviceAddName string

var serviceAddCmd = &cobra.Command{
	Use:   "add <infra>",
	Short: "Add an infra node to the graph",
	Long: `Adds an infra node to acthur.yml's graph.nodes (e.g. "acthur service add
cache:redis" adds a "redis" node using the cache:redis adapter) and rebuilds
the graph to confirm the result is still valid. Use --name to pick a
different node ID than the adapter's default.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := loadConfig()
		summary, err := runServiceAdd(cfg.RootDir, args[0], serviceAddName)
		if err != nil {
			return err
		}
		if summary.AlreadyHad {
			output.Info(summary.NodeID, "already present in the graph — nothing to do")
			return nil
		}
		output.Success(summary.NodeID, "added (%s) to the graph", summary.Adapter)
		return nil
	},
}

func init() {
	serviceAddCmd.Flags().StringVar(&serviceAddName, "name", "", "node ID to use instead of the adapter's default")
	serviceCmd.AddCommand(serviceAddCmd)

	serviceCmd.AddCommand(&cobra.Command{
		Use:   "logs <name>",
		Short: "Stream logs from a service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			path := process.LogPath(cfg.RootDir, args[0])

			quit := make(chan os.Signal, 1)
			signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
			stop := make(chan struct{})
			go func() {
				<-quit
				close(stop)
			}()

			return runServiceLogs(path, cmd.OutOrStdout(), stop, 300*time.Millisecond)
		},
	})

	serviceCmd.AddCommand(&cobra.Command{
		Use:   "restart <name>",
		Short: "Restart a specific service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			return runServiceRestart(cfg.RootDir, args[0], nil)
		},
	})

	serviceCmd.AddCommand(&cobra.Command{
		Use:   "health [name]",
		Short: "Show health status of one node, or all graph nodes",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, g := loadGraph()
			checker := health.New()

			if len(args) == 1 {
				return runServiceHealth(g, checker, args[0])
			}

			var failed int
			for _, node := range g.Nodes() {
				if strings.HasPrefix(node.Adapter, "kernel:") {
					continue
				}
				if err := runServiceHealth(g, checker, node.ID); err != nil {
					failed++
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d node(s) unhealthy", failed)
			}
			return nil
		},
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
		for _, name := range []string{"integration", "contract", "e2e", "load", "coverage", "watch"} {
			on, _ := cmd.Flags().GetBool(name)
			if on {
				return fmt.Errorf("acthur test --%s is not implemented yet — only unit tests (`acthur test`, `acthur test <service>`) are wired up", name)
			}
		}
		_, g := loadGraph()
		service := ""
		if len(args) > 0 {
			service = args[0]
		}
		if err := runTest(mustCwd(), g, service, os.Stdout); err != nil {
			return err
		}
		output.Success(output.PrefixKernel, "tests passed")
		return nil
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
		Long: `Starts a Model Context Protocol server over stdio (JSON-RPC 2.0,
newline-delimited), exposing this project's live graph, contracts, and
service health as read-only tools to any MCP-compatible AI tool (Claude
Code, Cursor, and others). Runs until stdin is closed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPServe(mustCwd(), os.Stdin, os.Stdout)
		},
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
		Long: `Answers a question about the project using its full live graph and
contract context. Requires an 'ai:' block in acthur.yml (see docs/acthur-prd.md
§17.7) with a working API key — only the "claude" provider is implemented today.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentExplain(agentDeps{root: mustCwd(), out: os.Stdout}, args[0])
		},
	})
	agentCmd.AddCommand(&cobra.Command{
		Use:   "generate <feature>",
		Short: "Propose an implementation plan for a feature, with full graph context",
		Long: `Proposes an implementation plan for a feature or endpoint using the
project's real adapters/contracts/graph as context. Prints the plan — it does
not write files itself. Requires an 'ai:' block in acthur.yml.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentGenerate(agentDeps{root: mustCwd(), out: os.Stdout}, args[0])
		},
	})
	agentCmd.AddCommand(&cobra.Command{
		Use:   "diagnose <problem>",
		Short: "Diagnose a runtime problem",
		Long: `Diagnoses a described runtime problem using the project's live graph and
contract context. Requires an 'ai:' block in acthur.yml.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentDiagnose(agentDeps{root: mustCwd(), out: os.Stdout}, args[0])
		},
	})
	agentCmd.AddCommand(&cobra.Command{
		Use:   "review",
		Short: "Review the current git diff",
		Long: `Reviews the working tree's current 'git diff' for bugs, risks, and
contract consistency, using the project's graph/contract context. Requires an
'ai:' block in acthur.yml. Prints a message and does nothing else if the diff
is empty.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentReview(agentDeps{root: mustCwd(), out: os.Stdout})
		},
	})
	agentCmd.AddCommand(&cobra.Command{
		Use:   "document <path>",
		Short: "Generate documentation for a path",
		Long: `Generates documentation for a file (its contents, truncated to 64KB) or
directory (a listing of its immediate entries), using the project's graph/
contract context. Requires an 'ai:' block in acthur.yml.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentDocument(agentDeps{root: mustCwd(), out: os.Stdout}, args[0])
		},
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
