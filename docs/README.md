# Documentation Index

Use these docs alongside the root `README.md`.

- `testing.md`: local configuration, GitHub App/OAuth setup, webhook forwarding, admin API examples, and troubleshooting.
- `deployment-smoke.md`: production-style smoke checklist for a real GitHub App, webhook, E2B template, runner pickup, cleanup, and diagnostics.
- `runner-architecture-comparison.md`: current runnerd architecture baseline, DB-backed state model, and comparison with Fireactions and Actions Runner Controller.
- `runner-implementation-review.md`: current branch implementation status, schema migration notes, and remaining product/operations decisions.

The root `README.md` is the operator quick start. `TODO.md` tracks pending decisions and should not duplicate completed behavior already documented here.

Agent-only rules and repeatable workflows live under `.agents/`. Keep business, operations, architecture, and deployment documentation here in `docs/`; move only durable agent guidance or executable agent workflow instructions into `.agents/rules/` or `.agents/skills/`.
