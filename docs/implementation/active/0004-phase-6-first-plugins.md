# Implementation: Phase 6 — First Plugins

## Goal
The first four capability plugins (`migrations`, `auth`, `rbac`, `multitenancy`) generate real, working Go code for `go:fiber` projects, and `acthur add <plugin>` installs a plugin and runs its generator. Done when `acthur add auth` on a fresh go:fiber project produces compiling, runnable auth code with migrations.

## Owning Docs
- `docs/prd/phase-6-first-plugins.md` (spec) · ADRs 0003, 0005 · tracker issue #42

## Status
In-flight
- 2026-07-06 — main — PRD + this note created; slices 1–4 being filed and delegated to parallel Sonnet worktree agents; slice 5 (live witness) stays with main.
- 2026-07-06 — subagent slice 3 (#45, Sonnet worktree) — Landed `internal/plugin/builtin/rbac/`: `DependsOn() []string{"auth"}`, generator "rbac" (go:fiber only) emitting `internal/rbac/{models,policy,middleware,seed}.go` and migrations 0200_roles/0201_permissions/0202_user_roles (up+down, TEXT/ULID pks). `PolicySource` interface with `StaticTablePolicySource` (pgx-v5-backed via a `DBPool` interface subset of `*pgxpool.Pool`, so it's fakeable without a live DB); contract-derived enforcement left for a later phase. `RequireRole`/`RequirePermission` fiber middleware read the authenticated user id from `c.Locals("user_id")` — documented as an assumption in generated `models.go`/`middleware.go`, no import of the auth plugin's package. `user_roles.user_id` has no FK to `users` (auth's table, different plugin/migration range) — deliberate, avoids cross-plugin schema coupling; only the plugin *load order* (DependsOn) couples the two. Tests drive the real `plugin.Load`/`KernelAPIImpl` path with a registry-stubbed "auth" fixture (no import of the real auth package); assert load order (auth before rbac), full emitted file set, and parse every emitted `.go` file with `go/parser`. Registered via blank import in `cmd/acthur/commands.go` next to `testplugin`. `go test ./...` green except a pre-existing flaky timing test in `internal/process` (`TestProcess_OutputCapture`, passes in isolation — unrelated to this slice, not touched). No deviations from the shared conventions.

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
