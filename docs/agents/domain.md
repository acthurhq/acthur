# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

- **`CONTEXT.md`** at the repo root — the project glossary (domain terms, what they mean in Acthur)
- **`docs/adr/`** — architectural decision records; read any ADRs that touch the area you're about to work in
- **`docs/implementation/active/`** — implementation notes for work currently in flight; if one overlaps your task, read it after the docs above (convention: `docs/implementation/README.md`)

If either file doesn't exist, **proceed silently**. Don't flag their absence; don't suggest creating them upfront. The `/grill-with-docs` skill creates them lazily when terms or decisions get resolved.

## File structure (single-context)

```
/
├── CLAUDE.md
├── CONTEXT.md
├── docs/
│   ├── adr/
│   │   └── 0001-*.md
│   └── agents/
└── internal/
```

## Use the glossary's vocabulary

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis, a test name), use the term as defined in `CONTEXT.md`. Don't drift to synonyms the glossary explicitly avoids.

If the concept you need isn't in the glossary yet, that's a signal — either you're inventing language the project doesn't use (reconsider) or there's a real gap (note it for `/grill-with-docs`).

## Flag ADR conflicts

If your output contradicts an existing ADR, surface it explicitly rather than silently overriding:

> _Contradicts ADR-0001 (proxy-as-implicit-kernel-node) — but worth reopening because…_
