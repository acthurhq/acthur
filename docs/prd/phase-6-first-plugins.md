# PRD — Phase 6: First Plugins

> **Status:** ready-for-agent · **Phase:** 6 of the implementation plan (PRD §21)
> **Governing ADRs:** 0003 (pre-seal graph mutation), 0005 (CLI-assembly injection)
> **Implementation note:** `docs/implementation/active/0004-phase-6-first-plugins.md`

## Problem Statement

Phase 5 wired the plugin kernel: plugins load from `acthur.yml`, register hooks/commands/generators, and observe engine lifecycle events. But the only plugin is `test` — the registry has nothing a user would actually install. Phase 6 ships the first four capability plugins (`migrations`, `auth`, `rbac`, `multitenancy`) generating real, working Go code for `go:fiber` projects, plus the `acthur add <plugin>` command that installs a plugin and runs its generator.

**Done when:** `acthur add auth` on a fresh `go:fiber` project produces compiling, runnable Go auth code with migrations (live witness).

## Shared conventions (all slices — do not deviate)

- Each plugin lives at `internal/plugin/builtin/<name>/` and calls `plugin.Register` from `init()`. Registration reaches the CLI via one blank-import line appended to the builtin import block in `cmd/acthur/commands.go` (same pattern as `testplugin`).
- Each plugin registers **one generator named exactly after the plugin** (`k.RegisterGenerator("<name>", …)`). `Generate(adapterName, ctx)` returns `[]plugin.GeneratedFile` relative to the target node's directory (`<root>/<nodeID>/…`) unless the path starts with `migrations/` (project-root migrations dir). `SupportedAdapters()` returns `["go:fiber"]` (or `["*"]` for adapter-agnostic files).
- `GeneratedFile.Overwrite=false` files are skipped when they exist; `MergeMarker` files merge at the marker. Generated code must `go build` inside the scaffolded gofiber project (module `<prefix>/<nodeID>`) — import paths come from `ctx` (ModulePath is passed via `ctx.Extra["module_path"]`).
- Migration files follow golang-migrate naming: `migrations/NNNN_<desc>.up.sql` / `.down.sql`, sequential per plugin range (migrations owns 0001–0099, auth 0100–0199, rbac 0200–0299, multitenancy 0300–0399) so parallel plugins never collide.
- Plugin `Config` (from the `plugins:` entry in acthur.yml) arrives in `GeneratorContext.Config`.
- Behavior tests first (TDD): drive the generator through the real `KernelAPIImpl` load path, then assert the emitted files (and, where cheap, that emitted Go parses via `go/parser`). No mocks of the plugin kernel.
- **Before committing**: append your result line under `## Status` in `docs/implementation/active/0004-phase-6-first-plugins.md` (`- <date> — subagent slice N (#<issue>, Sonnet worktree) — <result>`).

## Slices

### Slice 1 — `acthur add` + generator execution + `migrations` plugin (#43)
`acthur add <plugin>`: validates the plugin exists, appends it to `acthur.yml`'s `plugins:` list (idempotent), loads it, runs its generator against each `go:fiber` service node (or `--node <id>`), writes the returned files honoring Overwrite/MergeMarker, prints a summary. `migrations` plugin: embeds golang-migrate conventions — generator emits `migrations/.keep` + a `0001_init.up.sql`/`.down.sql` pair; registers `acthur db` command group (`db migrate`, `db rollback`, `db status`, `db create <name>`) shelling to the project's database from the graph's `db:postgres` node (DATABASE_URL construction reuses the engine's Connectable env logic). `acthur db create` writes a numbered empty pair in the project migrations dir.
**Files:** `cmd/acthur/add.go(+_test)`, `internal/plugin/builtin/migrations/`.

### Slice 2 — `auth` plugin (#44)
For `go:fiber`: generates `internal/auth/` (JWT issue/verify with HS256 from env `AUTH_JWT_SECRET`, session store, magic-link token flow, OAuth2 provider scaffold with a `providers.go` registry), `internal/auth/handlers.go` (register/login/logout/me routes mountable via one `auth.Mount(app, db)` call), middleware `RequireAuth`, and migrations `0100_users.up/down.sql` + `0101_sessions.up/down.sql` (users: ULID pk, email unique, password_hash, created_at). Config keys: `strategy` (default `jwt`), `providers` (list). Generated code compiles inside a scaffolded gofiber project with only stdlib + fiber + golang-jwt + pgx (add to generated go.mod via MergeMarker or a documented `go get` line in the summary output).
**Files:** `internal/plugin/builtin/auth/`.

### Slice 3 — `rbac` plugin (#45)
`DependsOn: ["auth"]`. Generates `internal/rbac/` (roles/permissions models, `RequireRole(...)`/`RequirePermission(...)` fiber middleware, seed helper), migrations `0200_roles.up/down.sql`, `0201_permissions.up/down.sql`, `0202_user_roles.up/down.sql`. Contract-derived enforcement is **stubbed behind an interface** (`PolicySource`) — the contract wiring is a later phase; ship the static-table implementation.
**Files:** `internal/plugin/builtin/rbac/`.

### Slice 4 — `multitenancy` plugin (#46)
Schema-per-tenant strategy for postgres: generates `internal/tenant/` (tenant model + resolver middleware reading `X-Tenant-ID`/subdomain, `SchemaSwitch` helper setting `search_path` per request, provisioner that creates a tenant schema by running migrations into it), migrations `0300_tenants.up/down.sql`. Pool tuning: generated pgx pool config with `BeforeAcquire` ping and `AfterRelease` search_path reset.
**Files:** `internal/plugin/builtin/multitenancy/`.

### Slice 5 — live witness (main agent)
Fresh scaffolded go:fiber project; `acthur add auth` → generated code compiles (`go build ./...` in the node dir), migrations present; `acthur dev` boots with the plugin loaded; restore `testdata/vetangle/acthur.yml`'s `plugins:` entries.

## Out of scope
Generator engine templates/lockfile (Phase 7), contract-derived RBAC policy, community plugin loading, non-fiber adapters.
