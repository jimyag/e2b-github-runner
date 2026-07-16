# Documentation Index

[中文文档](zh/README.md)

Use these docs alongside the root `README.md`.

- [Local Testing And GitHub Setup](testing.md): local configuration, GitHub App/OAuth setup, webhook forwarding, admin API examples, and troubleshooting.
- [Deployment Smoke Checklist](deployment-smoke.md): production-style smoke checklist for a real GitHub App, webhook, Qiniu sandbox template, runner pickup, cleanup, and diagnostics.
- [Runner Architecture Comparison](runner-architecture-comparison.md): current runnerd architecture baseline, Mermaid system/lifecycle/state diagrams, DB-backed state model, and comparison with Fireactions and Actions Runner Controller.
- [Runnerd Implementation Review](runner-implementation-review.md): current implementation status, schema migration notes, and remaining product/operations decisions.

The root `README.md` is the operator quick start. `TODO.md` tracks pending decisions and should not duplicate completed behavior already documented here.

Agent-only rules and repeatable workflows live under `.agents/`. Keep business, operations, architecture, and deployment documentation here in `docs/`; move only durable agent guidance or executable agent workflow instructions into `.agents/rules/` or `.agents/skills/`.
