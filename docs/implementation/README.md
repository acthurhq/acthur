# Implementation-in-flight Convention

Every non-trivial implementation effort (a PRD phase, a slice, a refactor) gets one
status note here, so any agent or human can see what is being built, by whom, and
where it stands — without reading git history or the issue tracker first.

```
docs/implementation/
  active/       # work currently in flight
  completed/    # done — note moved here verbatim, Status updated
  paused/       # deliberately parked — note says why and what unblocks it
```

Notes are numbered in creation order: `NNNN-<kebab-name>.md`.

## Note template

```markdown
# Implementation: <name>

## Goal
One paragraph: the user-observable outcome.

## Owning Docs
- `docs/prd/<phase>.md` (spec) · governing ADRs · tracker issue #NN

## Status
In-flight | Completed (YYYY-MM-DD) | Paused (reason)
One line per session/agent touching it: date — who (main / subagent slice N) — what moved.

## Current Decisions
Decisions made *during* implementation that the PRD/ADRs don't record yet.

## Open Questions

## Files/Modules Expected

## Acceptance Criteria
The done-when, verbatim from the PRD, with live-verified checkmarks.

## Risks
```

## Rules

- **Check `active/` before starting any task.** If an active note overlaps your
  work, read it after CONTEXT.md and the relevant ADRs.
- **Specialized subagents (Sonnet slices, worktree agents) update the note too.**
  The orchestrating agent creates the note *before* spawning slice agents and
  lists each slice + owner; every slice agent's launch prompt must include the
  note path, and the agent appends its result line under **Status** before
  committing. The orchestrator reconciles on merge.
- On completion, `git mv` the note to `completed/`, set the Status line, and add
  the closing commit hash. Never delete notes.
- Keep `CONTEXT.md` stable: in-flight details live here, not in the glossary.
  Promote language/decisions to CONTEXT.md or an ADR only once accepted.
- The note is a *status ledger*, not a spec — the PRD stays the spec; link, don't
  duplicate.
