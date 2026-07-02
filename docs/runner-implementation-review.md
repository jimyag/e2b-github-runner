# Runnerd Implementation Review

Date: 2026-06-29

Scope:

- Current branch at refresh time: `refactor/gorm-state-migrations`
- Review target: implementation status after file-based config, GORM-backed DB schema migration, retry/lease/audit handling, admin console, embedded UI assets, and local development workflow updates.
- Local references still useful for future comparison: actions-runner-controller style reconciliation and fireactions style pool/config modeling.

## Executive Summary

Runnerd has moved past the original 2026-05-19 gap list. Runtime configuration is now file-first, runner state is DB-backed, schema creation is mostly driven by GORM model tags, retry/lease/audit fields exist, GitHub App auth can resolve installations dynamically, the admin console covers the core management workflow, diagnostics expose pprof/expvar state, and the documented local workflow includes `task dev`.

The remaining work is no longer a basic architecture catch-up. The next decisions are product and operations hardening: whether to keep token/basic auth as local compatibility modes, how to introduce non-admin UI surfaces under the shared `ui/` tree, how much config management belongs in the admin console, and what deployment smoke tests are required before treating the service as production-ready.

## Current Baseline

- Configuration is loaded from `runnerd.yaml` by default, or from `--config`. Relative sqlite database paths and GitHub App private-key paths resolve from the config file directory.
- The config schema covers server, database, OAuth session auth, Sandbox lifecycle timeouts, GitHub webhook/auth/OAuth, allowed repositories, and worker retry/lease/concurrency behavior. Sandbox service API URL and API key are account or organization Preferences rather than file config.
- Exactly one GitHub API auth mode is allowed: GitHub App, token, or basic auth. GitHub App mode supports optional static `installation_id`; when omitted, runnerd resolves installation access per job repository and caches transports.
- Runner requests, events, specs, groups, policies, retry metadata, leases, and audit events are stored in the configured database backend.
- State schema creation runs through GORM `AutoMigrate` after a small compatibility pass for older schema columns that cannot be safely added as `NOT NULL` without defaults.
- Worker processing uses DB claim/lease semantics and retry scheduling instead of only in-memory queue ownership.
- Transient E2B, GitHub, rate-limit, timeout, and temporary network failures are classified for retry or queue deferral. Deterministic auth/config/template failures fail immediately.
- Admin routes expose runner request management, retry/stop/log access, runner specs, runner groups, repository policies, match tests, audit events, and diagnostics.
- The React UI in `ui/` is embedded for production from `internal/server/ui/*`; development builds proxy UI assets to Vite through `internal/server/ui_assets_development.go`.
- `task dev` starts Vite and the Go service together in development mode. `task build` builds the UI first, then compiles `bin/runnerd` with embedded production assets.
- Diagnostics are available through the admin UI and `/diagnostics/pprof` / `/diagnostics/vars`, backed by `github.com/jimmicro/pprof` and expvar.

## Remaining Decisions

### 1. Auth Policy

Token and basic auth are still supported alongside GitHub App auth. That is useful for local verification or legacy credentials, but it means the product is not GitHub-App-only. Decide whether these modes are intentional compatibility paths or should be removed before production hardening.

### 2. UI Product Boundary

The asset package has been generalized from admin-only assets to `ui/*`, which leaves room for future ordinary-user screens. The current routed product is still admin-focused under `/admin/*`. Before adding non-admin UI, define route ownership, role-aware navigation, and API permissions so the shared UI tree does not become an accidental admin surface.

### 3. Config Management

Runtime config is file-first, but the admin console does not yet provide an effective-config view, config validation preview, reload workflow, or import/export flow. Keep the current file-only operations model unless live config operations become a clear requirement.

### 4. Deployment Smoke

Local build/lint/test coverage validates the code path, but production readiness still depends on a real GitHub App installation, real E2B templates, webhook delivery, and sandbox runner execution. Use `docs/deployment-smoke.md` for the real-deployment checklist covering webhook signature handling, installation resolution, runner spec matching, sandbox creation, GitHub job pickup, cleanup, and diagnostics.

### 5. Multi-Instance And Operations

The DB lease model is in place, but multi-process behavior should be verified with two runnerd instances against the same database before documenting multi-instance support. Expvar diagnostics cover useful counters and gauges; add histogram/export adapters only if deployment observability needs them.

### 6. Schema Compatibility

The current migration path intentionally avoids a full handwritten migration history. GORM tags in `internal/state/records.go` define the normal schema, while `internal/state/db.go` keeps only narrow upgrade backfills for older state databases. Future schema changes should include old-schema upgrade tests when they add required columns, indexes with uniqueness semantics, or relationship constraints.

## Suggested Next Order

1. Keep `task dev`, `task build`, `task lint`, and `task test` green on every branch that touches backend/UI boundaries.
2. Run and maintain the deployment smoke checklist using a real GitHub App, one repository, and one E2B template.
3. Decide whether token/basic auth remain supported modes.
4. Define the non-admin UI route and permission model before adding ordinary-user screens.
5. Add an effective-config diagnostics view only after the desired config operations model is clear.
6. Stress DB lease behavior with concurrent runnerd processes before advertising multi-instance support.
7. Preserve old-schema upgrade coverage whenever state records or GORM migration tags change.

## Verification Notes

The stale findings from the 2026-05-19 review have been retired because the referenced implementation has changed materially. Re-run the current verification commands when this document is updated:

```bash
task lint
task test
task build
```
