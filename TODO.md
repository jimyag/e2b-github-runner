# TODO

This file tracks active project work. Completed behavior should move into `README.md` or `docs/`.

## Active Roadmap

- Decide whether GitHub token and basic auth remain supported compatibility modes or should be removed in favor of GitHub App-only operation.
- Decide whether ordinary-user Activity repositories should include repository-policy configuration rows in addition to repositories observed from runner jobs.
- Add an effective-config diagnostics view or config validation workflow if operators need to inspect runtime config from the UI.
- Verify DB lease behavior with two runnerd processes sharing the same database before documenting multi-instance support.
- Decide whether expvar diagnostics need a Prometheus/export adapter or histogram-style latency views for deployment observability.
- Keep old-schema upgrade coverage whenever state records or GORM tags change; the current migration path is `AutoMigrate` plus narrow compatibility backfills, not a full handwritten migration history.

## Maintenance

- Keep `README.md`, `docs/testing.md`, and this roadmap in sync when build, dev, config, or UI asset workflows change.
- Keep `docs/deployment-smoke.md` aligned with real GitHub App, webhook, Qiniu sandbox template, runner pickup, cleanup, and diagnostics behavior.
- Keep generated production UI assets under `internal/server/ui/` out of hand edits; change source files in `ui/` and rebuild with `task build`.
- When changing `internal/state/records.go` tags or migration helpers in `internal/state/db.go`, run `go test ./internal/state -count=1` before the broader test suite.
- Keep `.agents/` focused on agent-only rules and repeatable workflows; keep operator, architecture, and deployment content in `README.md` and `docs/`.
