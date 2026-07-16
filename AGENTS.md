# Agent Guide

Use this guide for future Codex or agent work in this repository.

## Project Facts

- `runnerd` reads `./runnerd.yaml` by default, or another path passed with `--config`.
- Local development should use `runnerd.local.yaml` for secrets and sqlite state.
- UI source lives in `ui/`.
- Production UI assets are generated into `internal/server/ui/` by `task ui-build` and embedded by `internal/server/ui_assets_production.go`.
- Development UI assets are proxied to Vite by `internal/server/ui_assets_development.go`.
- Current ordinary-user browser routes include `/`, `/repositories`, stable job-group routes such as `/github/pulls/{owner}/{repo}/{number}/jobs`, account settings under `/account/repositories`, `/account/preferences`, `/account/sandbox-templates`, and `/account/sandbox-instances`, and the matching `/organizations/{login}/...` routes.
- Sandbox service credentials and resource catalogs are ordinary-user account or organization data. Keep `/user/sandbox/templates` and `/user/sandbox/instances` separate from admin configuration APIs.
- Current admin browser routes include `/admin/`, `/admin/accounts`, and `/admin/sandbox_service`; keep admin routes and role-gated APIs explicit when changing shared `ui/` code.
- `/admin/accounts` lists OAuth/bootstrap-created accounts and linked identities. Its only mutation changes another account's `admin`/`user` role atomically with an audit event; self-role changes and changes that could leave no administrator are rejected.
- Runtime state can use sqlite, Postgres, or MySQL. Do not document multi-instance support until two runnerd processes have been verified against the same database.
- State schema is defined mostly by GORM tags in `internal/state/records.go`; startup migration runs a narrow legacy compatibility pass and then `AutoMigrate` in `internal/state/db.go`. GORM foreign-key creation is disabled intentionally, so preserve the foreign-keyless schema convention unless a separately tested migration changes it. Legacy account preference/secret tables without scope columns are intentionally reset; operator docs must warn about Sandbox reconfiguration and GitHub reauthentication before installation sync.
- Runner specs, runner groups, and repository policies are admin API/UI data, not `runnerd.yaml` fields.
- The recommended production GitHub auth path is GitHub App auth for runner operations plus GitHub App OAuth sign-in for ordinary users and administrators. Local account roles authorize management APIs. Token and basic auth still exist as compatibility modes, but their long-term product status is undecided.

## Common Commands

```bash
task deps
task ui-deps
task dev
task smee
task lint
task test
task build
task docker-check
task release-check
```

Use `task dev` for local development. It defaults to `RUNNERD_CONFIG=runnerd.local.yaml`, starts Vite on the first available localhost port at or after `5173`, and starts smee forwarding when `.smee-url` exists.

Use `task smee` for standalone GitHub webhook forwarding. It reads `.smee-url` and defaults to `SMEE_TARGET=http://127.0.0.1:25500/webhooks/github`.

Use `task build` when verifying production embedded UI behavior because it rebuilds `internal/server/ui/` before compiling `bin/runnerd`.

Use `cd ui && bun run test` for focused UI tests. `task test` rebuilds the UI, runs the Bun UI tests, and then runs Go tests with race detection and coverage.

When changing state records, GORM tags, indexes, or migration helpers, run `go test ./internal/state -count=1` first. Old sqlite schema upgrade tests are intentional compatibility coverage; do not remove them just because fresh database creation passes.

## Local Agent Assets

- `.agents/rules/development-workflow.md`: detailed workflow, generated-file boundaries, and documentation sync rules.
- `.agents/rules/project-architecture.md`: durable architecture and implementation boundaries for runnerd.
- `.agents/rules/testing-and-verification.md`: verification matrix for docs, state schema, UI, dev startup, Docker, release, and deployment smoke work.
- `.agents/skills/runnerd-state-schema/SKILL.md`: use for state records, GORM tags, indexes, and migration compatibility work.
- `.agents/skills/runnerd-dev-smoke/SKILL.md`: use for `task dev`, Vite proxy, smee forwarding, and local startup verification.

## Editing Rules

- Do not commit real secrets, local sqlite databases, or local config files.
- Do not commit `.smee-url`; it is per-developer local webhook state.
- Do not hand-edit generated files in `internal/server/ui/`; edit `ui/` and rebuild.
- Keep `README.md` and `README.zh.md`, `docs/testing.md` and `docs/zh/testing.md`, and `TODO.md` aligned when changing config, build, development, public APIs, authentication/authorization, state semantics, or deployment workflows.
- Keep `docs/README.md` and `docs/zh/README.md`, plus `docs/deployment-smoke.md` and `docs/zh/deployment-smoke.md`, aligned when adding or removing docs or deployment verification steps.
- Keep `.agents/rules/` and `.agents/skills/` aligned when a change creates durable agent rules or repeatable project workflows.
- When changing ordinary-user UI, keep account/organization Preferences and Sandbox catalog scope separate from admin-only management APIs instead of assuming everything under `ui/` is admin-only.
