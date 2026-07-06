# Implementation: Phase 6 — First Plugins

## Goal
The first four capability plugins (`migrations`, `auth`, `rbac`, `multitenancy`) generate real, working Go code for `go:fiber` projects, and `acthur add <plugin>` installs a plugin and runs its generator. Done when `acthur add auth` on a fresh go:fiber project produces compiling, runnable auth code with migrations.

## Owning Docs
- `docs/prd/phase-6-first-plugins.md` (spec) · ADRs 0003, 0005 · tracker issue #42

## Status
In-flight
- 2026-07-06 — main — PRD + this note created; slices 1–4 being filed and delegated to parallel Sonnet worktree agents; slice 5 (live witness) stays with main.

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
