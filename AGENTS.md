# Agent Guide

Use this guide for future Codex or agent work in this repository.

## Project Facts

- `runnerd` reads `./runnerd.yaml` by default, or another path passed with `--config`.
- Local development should use `runnerd.local.yaml` for secrets and sqlite state.
- UI source lives in `ui/`.
- Production UI assets are generated into `internal/server/ui/` by `task ui-build` and embedded by `internal/server/ui_assets_production.go`.
- Development UI assets are proxied to Vite by `internal/server/ui_assets_development.go`.
- Current browser entry for the admin console is `/admin/`; the `ui/` tree may also host ordinary-user UI later.
- Runtime state can use sqlite or Postgres. Do not document multi-instance support until two runnerd processes have been verified against the same database.
- Runner specs, runner groups, and repository policies are admin API/UI data, not `runnerd.yaml` fields.
- The recommended production GitHub auth path is GitHub App auth plus GitHub App OAuth admin login. Token and basic auth still exist as compatibility modes, but their long-term product status is undecided.

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

Use `task dev` for local development. It defaults to `RUNNERD_CONFIG=runnerd.local.yaml` and `RUNNERD_VITE_PORT=5173`, and starts smee forwarding when `.smee-url` exists.

Use `task smee` for standalone GitHub webhook forwarding. It reads `.smee-url` and defaults to `SMEE_TARGET=http://127.0.0.1:25500/webhooks/github`.

Use `task build` when verifying production embedded UI behavior because it rebuilds `internal/server/ui/` before compiling `bin/runnerd`.

## Editing Rules

- Do not commit real secrets, local sqlite databases, or local config files.
- Do not commit `.smee-url`; it is per-developer local webhook state.
- Do not hand-edit generated files in `internal/server/ui/`; edit `ui/` and rebuild.
- Keep `README.md`, `docs/testing.md`, and `TODO.md` aligned when changing config, build, development, or deployment workflows.
- Keep `docs/README.md` and `docs/deployment-smoke.md` aligned when adding or removing docs or deployment verification steps.
- If adding non-admin UI, keep admin routes and role-gated APIs explicit instead of assuming everything under `ui/` is admin-only.
