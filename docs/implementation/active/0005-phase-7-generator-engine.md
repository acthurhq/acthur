# Implementation: Phase 7 — Generator Engine

## Goal
Contracts + plugins → framework-native code via `acthur generate`: a write engine with `generated.lock` idempotency, a go:fiber template library turning a contract into handlers/services/repositories/DTOs/migrations with tests alongside, and the `generate from-contract` / `generate model` commands. Done when `acthur generate from-contract users.contract.yml` produces compiling Go code with tests.

## Owning Docs
- `docs/prd/phase-7-generator-engine.md` (spec) · ADR 0005 · tracker issue #47

## Status
In-flight
- 2026-07-06 — main — PRD + this note created; slices 1–2 delegated to parallel Sonnet worktree agents; slice 3 (CLI + witness) stays with main.

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
