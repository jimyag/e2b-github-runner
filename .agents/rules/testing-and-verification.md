# Testing And Verification

Choose the lightest credible verification for the change, then report exactly what ran.

## Documentation Only

For docs, rules, or skill-only changes:

```bash
test -f AGENTS.md
test -d .agents/rules
test -d .agents/skills
git diff --check
```

Also inspect the diff and keep `docs/README.md` aligned when adding, removing, or reclassifying docs.

## State Schema

When touching `internal/state/records.go`, GORM tags, indexes, or migration helpers in `internal/state/db.go`, run:

```bash
go test ./internal/state -count=1
```

If the change affects callers outside the state package, follow with:

```bash
go test ./...
task test
```

Old-schema upgrade coverage is required when adding required columns, changing uniqueness semantics, or altering relationship constraints. Fresh sqlite creation is not enough.

## Go Server Or API

- For focused backend changes, start with the relevant package test.
- For broad server/API behavior, run `go test ./...`.
- For pre-merge confidence, run `task test`; it rebuilds UI assets and runs Go tests with race and coverage.

## UI

- Edit source under `ui/`, not generated files under `internal/server/ui/`.
- For UI source changes, run `task ui-lint` or `task build` depending on scope.
- Use `task build` when verifying production embedded UI behavior.
- Use the real ordinary-user entries `/`, `/repositories`, `/account/repositories`, `/account/preferences`, `/account/sandbox-templates`, and `/account/sandbox-instances` when changing user UI. Also exercise the corresponding `/organizations/{login}/...` route when scope resolution changes.
- Use the real admin entries `/admin/` and `/admin/sandbox_service`; do not assume the `ui/` tree is all admin-only.
- For Sandbox fallback changes, verify scoped override, enabled-default fallback, disabled/incomplete default rejection, catalog access, and config-source display without exposing endpoint/key or audience metadata to ordinary users.
- For audience changes, verify `all`, selected match/miss, selected-empty, user/org stable identity, login rename tolerance, manual preconfiguration before sign-in/sync, GitHub 404 rejection, installation-owner lookup/cache behavior, audit events, and saved snapshot behavior.

## Development Startup

For `task dev`, Vite proxy, or smee startup changes, prefer a real startup smoke using temporary ports if defaults are occupied:

```bash
RUNNERD_VITE_PORT=<free-port> RUNNERD_CONFIG=<local-config> task dev
curl -fsS http://127.0.0.1:<runnerd-port>/healthz
curl -I http://127.0.0.1:<runnerd-port>/admin/
```

Keep `SMEE_TARGET` aligned with the runnerd port when testing webhook forwarding.

## Docker, Templates, And Release

- Dockerfile-only validation: `task docker-check`.
- Local binary and embedded UI: `task build`.
- GoReleaser config: `task release-check`.
- Snapshot release behavior: `task release-snapshot`.
- Template changes may require the relevant `template-*` or `qbox-kodo-*` task.

## Deployment Smoke

Real deployment readiness still requires `docs/deployment-smoke.md` with a GitHub.com App, webhook delivery, a usable Qiniu sandbox template, runner pickup, cleanup, and diagnostics. Do not claim production readiness from local tests alone.
