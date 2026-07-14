# Documentation Index

[中文文档](zh/README.md)

Use these docs alongside the root `README.md`.

- `testing.md`: local configuration, GitHub App/OAuth setup, webhook forwarding, admin API examples, and troubleshooting.
- `deployment-smoke.md`: production-style smoke checklist for a real GitHub App, webhook, Qiniu sandbox template, runner pickup, cleanup, and diagnostics.
- `runner-architecture-comparison.md`: current runnerd architecture baseline, Mermaid system/lifecycle/state diagrams, DB-backed state model, and comparison with Fireactions and Actions Runner Controller.
- `runner-implementation-review.md`: current branch implementation status, schema migration notes, and remaining product/operations decisions.
- `superpowers/plans/2026-07-14-admin-sandbox-service-default.md`: implementation plan and acceptance criteria for the admin-managed Sandbox service fallback.
- `superpowers/plans/2026-07-14-admin-sandbox-service-audience-policy.md`: implementation plan and acceptance criteria for restricting that fallback by GitHub repository owner.
- `superpowers/plans/2026-07-14-admin-sandbox-service-manual-audience.md`: implementation plan for manual GitHub account provisioning and runtime installation-owner lookup/cache behavior.

The root `README.md` is the operator quick start. `TODO.md` tracks pending decisions and should not duplicate completed behavior already documented here.

Agent-only rules and repeatable workflows live under `.agents/`. Keep business, operations, architecture, and deployment documentation here in `docs/`; move only durable agent guidance or executable agent workflow instructions into `.agents/rules/` or `.agents/skills/`.
