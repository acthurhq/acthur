# Implementation: Phase 6 — First Plugins

## Goal
The first four capability plugins (`migrations`, `auth`, `rbac`, `multitenancy`) generate real, working Go code for `go:fiber` projects, and `acthur add <plugin>` installs a plugin and runs its generator. Done when `acthur add auth` on a fresh go:fiber project produces compiling, runnable auth code with migrations.

## Owning Docs
- `docs/prd/phase-6-first-plugins.md` (spec) · ADRs 0003, 0005 · tracker issue #42

## Status
In-flight
- 2026-07-06 — main — PRD + this note created; slices 1–4 being filed and delegated to parallel Sonnet worktree agents; slice 5 (live witness) stays with main.
- 2026-07-06 — subagent slice 4 (#46, Sonnet worktree) — `internal/plugin/builtin/multitenancy/` shipped via TDD. Generator "multitenancy" (adapter `go:fiber` only) emits `internal/tenant/{tenant,store,resolver,schema,pool,provisioner}.go` (Overwrite=true, regenerable scaffolding) and `migrations/0300_tenants.{up,down}.sql` (Overwrite=false). Resolver middleware: `X-Tenant-ID` header first, subdomain fallback (first Host label), unresolved tenant → 400. `SchemaSwitch` sets `search_path TO <schema>, public` via `pgx.Identifier{...}.Sanitize()`, returns a reset func. `NewPool` tunes pgxpool with `BeforeAcquire` (ping) and `AfterRelease` (reset `search_path` to `public`) exactly as PRD step 5 describes. `Provisioner.Provision` creates the tenant's schema (`CREATE SCHEMA IF NOT EXISTS`), inserts the `public.tenants` row (ID via the target module's own `internal/ids` package, imported through `ctx.Extra["module_path"]`), then calls an injected `MigrateSchema` hook (kept migration-runner-agnostic — no migrate library imported into generated code). Migration `0300_tenants`: ULID text pk, `slug`/`schema_name` unique, `created_at` default `now()`. Tests drive the generator through a real `graph.Build` + `plugin.NewKernelAPI` (no mocks), assert the emitted file set, parse every `.go` file with `go/parser`, and assert key content (resolver header/subdomain/400, schema search_path, pool hooks, provisioner CREATE SCHEMA + module-path-qualified import, migration SQL, Overwrite flags). Additionally hand-verified (not part of the committed test suite): scaffolded a throwaway `go:fiber`-shaped module, wrote the generated files into it, ran `go mod tidy && go build ./... && go vet ./...` — compiles and vets clean against real `pgx/v5` + `fiber/v2`. No deviations from the shared conventions or PRD Slice 4 scope.

## Current Decisions
- Migration number ranges per plugin (migrations 0001–0099, auth 0100–0199, rbac 0200–0299, multitenancy 0300–0399) so parallel slices never collide on filenames.
- Generators are driven through the real `KernelAPIImpl` load path in tests — no plugin-kernel mocks.
- Contract-derived RBAC enforcement is stubbed behind a `PolicySource` interface; static tables ship now, contract wiring later.

## Open Questions
- How generated code's third-party deps land in the target go.mod (MergeMarker vs documented `go get`) — slice 2 decides and records here.

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
