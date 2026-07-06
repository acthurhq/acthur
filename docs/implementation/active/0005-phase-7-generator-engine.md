# Implementation: Phase 7 — Generator Engine

## Goal
Contracts + plugins → framework-native code via `acthur generate`: a write engine with `generated.lock` idempotency, a go:fiber template library turning a contract into handlers/services/repositories/DTOs/migrations with tests alongside, and the `generate from-contract` / `generate model` commands. Done when `acthur generate from-contract users.contract.yml` produces compiling Go code with tests.

## Owning Docs
- `docs/prd/phase-7-generator-engine.md` (spec) · ADR 0005 · tracker issue #47

## Status
In-flight
- 2026-07-06 — main — PRD + this note created; slices 1–2 delegated to parallel Sonnet worktree agents; slice 3 (CLI + witness) stays with main.
- 2026-07-06 — subagent slice 1 (#48, Sonnet worktree) — built `internal/generate` (engine.go + engine_test.go): `WriteFiles(root, nodeID, files)` with full generated.lock semantics (fresh write, hash-match regenerate, hash-mismatch skip+warn, MergeMarker merge, migrations/ routing, atomic lock save via temp+rename). Hoisted `writeGeneratedFile`/`targetPathFor`/`mergeGeneratedFile` out of `cmd/acthur/add.go`; `runAdd` now calls `generate.WriteFiles` per node. Deviation: one existing add test (`TestRunAdd_Idempotent_...`) asserted `skipped` on an untouched second `add` — under full lock semantics a hash-match now regenerates (`written`), so I renamed/updated that test to assert byte-identical regeneration + a "written" status instead of weakening the engine's semantics away from the PRD spec; all other add tests pass unmodified. Also added a lock-content assertion to the first add test per the slice's ask. `go test ./...`, `gofmt`, `go vet` all clean.

- 2026-07-06 — main (Slice 2, #49; the Sonnet worktree agent for this slice was killed by a session limit with zero work products, so main implemented it inline) — `internal/generate/gofiber`: contract→code pipeline. `Generate(c, ctx)` emits dto/handler/service/repository/routes + a `_test.go` beside each, plus `migrations/0400_<contract>.up/down.sql` derived from `types:` (ULID TEXT pks, NOT NULL from non-optional fields; array/$ref fields skipped as relations). Constraint parsing covers required/max:N/email/enum/optional-`?`; endpoint inputs parse from body (POST/PUT/PATCH) or query, validate before the service call; declared `errors:` become `APIError` sentinels the handler maps by status; service+repository are editable skeletons returning ErrNotImplemented; `Mount(app, db)`/`MountWith(app, svc)` with a `RequireAuth` pass-through stub on `auth: required` routes. Deviation from the PRD: built with Go string builders over a precomputed model instead of text/template — the per-field conditional logic is the complexity, templates would only relocate it (recorded in the package doc). Gold-standard test scaffolds a real go:fiber project, writes the generated output in, and runs `go mod tidy && go build ./... && go test ./internal/users/...` — passing, including the generated tests themselves.

## Current Decisions
- `[]plugin.GeneratedFile` stays the unit of exchange between pipeline and write engine (continuity with Phase 6's `acthur add`).
- `generated.lock` semantics: hash-match → regenerate; hash-mismatch → skip+warn (user owns it) unless MergeMarker.
- Contract-derived migrations own the 0400–0499 number range.

## Open Questions
- Whether `acthur add` writes its files through the lock too (slice 1 records them; enforcement is a follow-up).

## Files/Modules Expected
`internal/generate/` (engine + lock), `internal/generate/gofiber/` (templates + pipeline), `cmd/acthur` (generate commands).

## Acceptance Criteria
- [ ] `acthur generate from-contract users.contract.yml` on a scaffolded go:fiber project → `go build ./... && go test ./...` green in the node
- [ ] Every generated source file has a generated test beside it
- [ ] Regeneration is idempotent; a user-edited file is skipped with a warning (generated.lock)
- [ ] `acthur generate model <Name> <field:type>...` produces model + migration + test
- [ ] Full suite green

## Risks
Template fidelity is the hard part — the generated tests must pass inside the scaffolded project, which forces the templates to be honest about imports, ids, and pgx usage rather than pseudo-code.
