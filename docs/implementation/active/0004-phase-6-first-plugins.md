# Implementation: Phase 6 — First Plugins

## Goal
The first four capability plugins (`migrations`, `auth`, `rbac`, `multitenancy`) generate real, working Go code for `go:fiber` projects, and `acthur add <plugin>` installs a plugin and runs its generator. Done when `acthur add auth` on a fresh go:fiber project produces compiling, runnable auth code with migrations.

## Owning Docs
- `docs/prd/phase-6-first-plugins.md` (spec) · ADRs 0003, 0005 · tracker issue #42

## Status
In-flight
- 2026-07-06 — main — PRD + this note created; slices 1–4 being filed and delegated to parallel Sonnet worktree agents; slice 5 (live witness) stays with main.
- 2026-07-06 — subagent slice 1 (#43, Sonnet worktree) — landed `cmd/acthur/add.go` (`runAdd`, `ensurePluginLoaded`, YAML plugins-list editor, generated-file writer with Overwrite/MergeMarker handling) and `internal/plugin/builtin/migrations/` (generator emitting `migrations/.keep` + `0001_init.up/down.sql`; `DatabaseURL`/`Migrate`/`Rollback`/`Status`/`CreateNext` backed by `golang-migrate/migrate/v4` as a library). Wired `acthur db migrate|rollback|status|create` in `cmd/acthur/commands.go` directly (not via `plugin.RegisterCommand` — see deviation below) and added the `migrations` blank+named import next to `testplugin`. Added `github.com/golang-migrate/migrate/v4` dependency, pinned to v4.17.0 (not @latest/v4.19.1) specifically so `go.mod`'s `go` directive stays at 1.22 as CLAUDE.md requires — `go mod tidy` with the latest migrate version wants to bump to go 1.24. Full `go test ./...`, `gofmt`, `go vet` clean. Manually witnessed `acthur add migrations` (idempotent second run skips existing files), `acthur db create`, and `acthur db status` (fails cleanly with a real postgres auth error, proving DATABASE_URL derivation is live, not stubbed — no live DB required by the test suite itself). Did not touch `testdata/vetangle/acthur.yml` (reserved for slice 5).
  - **Deviation**: `KernelAPI.RegisterCommand`/`CLICommand` has no notion of a command *group* (`Use` is a flat string handed straight to `cobra.Command`, and `registerPluginCommand` adds every command as a direct child of `Root`). `acthur db` already exists as a real cobra group in `commands.go` with `dbMigrateCmd`/`dbSeedCmd`/etc. predating this slice, so the migrations plugin does not call `k.RegisterCommand` for `db migrate|rollback|status|create` — that would either collide on `Use: "db"` with the existing group or require inventing ad-hoc multi-word `Use` strings cobra can't dispatch as true subcommands. Instead the plugin package exports `DatabaseURL`/`Migrate`/`Rollback`/`Status`/`CreateNext`, and `commands.go`'s `db` subcommands call them directly — the same "CLI assembly wires the logic" shape `contractCmd` already uses for `internal/contract`. The plugin still owns and tests all the logic; only the cobra grouping lives in `commands.go`. Flagging this so slices 2–4 don't try to force grouped commands through `RegisterCommand` either — if a later slice needs plugin-owned command *groups*, that's a KernelAPI extension, not a workaround in each plugin.
  - **Deviation**: no comment-preserving YAML writer exists in `internal/config` (confirmed: no `Save`/`Write`/`yaml.Node` usage anywhere in that package). `addPluginToYAML` (`cmd/acthur/add.go`) does a narrow, line-based insertion of `  - name: <plugin>` under the `plugins:` key — it touches only that key's block, so every other comment/format in the file survives untouched. This is not a general YAML editor (e.g. it doesn't handle an inline `plugins: []`), but it's sufficient for `acthur add`'s only write pattern (append one list entry) and is documented at the top of the editor in `add.go`.
  - **Once-per-process**: `ensurePluginLoaded` computes the delta between `cfg.Plugins` (freshly reloaded after the YAML edit) and the already-loaded set (`loadedPlugins`, populated by `bootstrapPlugins` at CLI start) and calls `plugin.Load` only on that delta, reusing the existing `kernelAPI`/`kernelBus` otherwise — so `acthur add` never re-registers an already-bootstrapped plugin's hooks. Verified by `TestEnsurePluginLoaded_ReusesKernelAPIWhenAlreadyLoadedThisProcess`.

## Current Decisions
- Migration number ranges per plugin (migrations 0001–0099, auth 0100–0199, rbac 0200–0299, multitenancy 0300–0399) so parallel slices never collide on filenames.
- Generators are driven through the real `KernelAPIImpl` load path in tests — no plugin-kernel mocks.
- Contract-derived RBAC enforcement is stubbed behind a `PolicySource` interface; static tables ship now, contract wiring later.
- `acthur db migrate|rollback|status|create` are real cobra commands in `cmd/acthur/commands.go` calling into `internal/plugin/builtin/migrations` package functions, not plugin-registered `CLICommand`s (see slice 1 deviation note above) — `KernelAPI.CLICommand` has no command-group concept today.

## Open Questions
- How generated code's third-party deps land in the target go.mod (MergeMarker vs documented `go get`) — slice 2 decides and records here.
- Should `KernelAPI` grow a way for a plugin to register a *group* of subcommands (not just flat top-level commands)? Slice 1 worked around this for `db`; slices 2–4 don't need command groups themselves but a future plugin (e.g. one wanting `acthur auth …`) will hit the same wall.

## Files/Modules Expected
`cmd/acthur/add.go`, `internal/plugin/builtin/{migrations,auth,rbac,multitenancy}/`.

## Acceptance Criteria
- [ ] `acthur add auth` on a fresh go:fiber project → generated code compiles (`go build ./...`), migrations present
- [ ] `acthur db` command group works against the graph's postgres node
- [ ] rbac loads after auth via DependsOn; its generated middleware compiles
- [ ] multitenancy generates schema-switch + provisioner code that compiles
- [ ] vetangle `plugins:` entries restored; full suite green

## Risks
Four agents touch the builtin import block in `cmd/acthur/commands.go` — expect trivial one-line merge conflicts. Generated-code compilation is the hard gate; agents must build the emitted code inside a scaffolded project in tests or accept witness-time failures.
