# Implementation: Phase 6 — First Plugins

## Goal
The first four capability plugins (`migrations`, `auth`, `rbac`, `multitenancy`) generate real, working Go code for `go:fiber` projects, and `acthur add <plugin>` installs a plugin and runs its generator. Done when `acthur add auth` on a fresh go:fiber project produces compiling, runnable auth code with migrations.

## Owning Docs
- `docs/prd/phase-6-first-plugins.md` (spec) · ADRs 0003, 0005 · tracker issue #42

## Status
In-flight
- 2026-07-06 — main — PRD + this note created; slices 1–4 being filed and delegated to parallel Sonnet worktree agents; slice 5 (live witness) stays with main.
- 2026-07-06 — subagent slice 2 (#44, Sonnet worktree) — `auth` built-in plugin landed at `internal/plugin/builtin/auth/`. Generator "auth" (adapter `go:fiber`) renders `internal/auth/{models,password,jwt,session,magiclink,providers,middleware,handlers}.go` via `text/template` + `embed.FS`, plus the `0100_users`/`0101_sessions` up/down migrations (also embedded, static). JWT is HS256 via golang-jwt/jwt/v5, secret from `AUTH_JWT_SECRET`; sessions are DB-backed via pgx/v5 `pgxpool.Pool`; magic-link is an in-memory single-use token store (no new migration — documented as a later swap point for multi-instance deployments); `providers.go` is an OAuth2 registry seeded at generation time from `Config["providers"]`; `handlers.go` exposes one `Mount(app, db)` wiring register/login/logout/me, switching JWT-vs-session-cookie issuance on `Config["strategy"]` (default `"jwt"`); `RequireAuth` is the bearer-JWT fiber middleware. Password hashing uses `golang.org/x/crypto/bcrypt`. Registered in `cmd/acthur/commands.go` next to `testplugin`. Tests in `internal/plugin/builtin/auth/auth_test.go` drive the generator through a real `plugin.NewKernelAPI` + `plugin.Load(["auth"], ...)` (no kernel mocks): file-set assertions, `go/parser` validity on every emitted `.go` file, migration content checks, and a gold-standard test that scaffolds a real go:fiber project via the adapter's `Scaffold` + `scaffold.ResolveScaffoldContext` into `t.TempDir()`, writes the generated files in, and runs `go mod tidy && go build ./...` — passes (guarded by `testing.Short()`).
  **Open Question resolved:** no `MergeMarker` into `go.mod` needed. `golang-jwt/jwt/v5` and `jackc/pgx/v5` are already direct requires in the go:fiber adapter's scaffolded `go.mod.tmpl` (from earlier phase work) — the auth plugin's only additional dependency is `golang.org/x/crypto/bcrypt`, a transitive import that `go mod tidy` discovers and adds on its own. No `acthur add` summary line is required for it; recorded here since slice 1 (`acthur add`) isn't in this worktree yet.
  Full `go test ./...` green except a pre-existing, unrelated flake in `internal/process.TestProcess_OutputCapture` (passes in isolation/reruns; package untouched by this slice).

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
