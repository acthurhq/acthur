# Acthur — Claude Code Project Config

Acthur is a runtime graph operating system written in Go (module: `github.com/acthur/acthur`, Go 1.22). It treats backend services, frontends, and infra as first-class nodes in a live directed acyclic graph (DAG). The kernel is structured as four layers: Graph (Ring 0) → Adapters → Contracts → Plugins.

## Agent skills

### Issue tracker

Issues live in GitHub Issues at `samueloshio/acthur`. See `docs/agents/issue-tracker.md`.

### Triage labels

Default label strings — five canonical triage roles mapped 1:1. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context repo — `CONTEXT.md` at root + `docs/adr/`. See `docs/agents/domain.md`.
