# TODO

This file tracks active project work. Completed behavior should move into `README.md` or `docs/`.

## Active Roadmap

- Decide whether GitHub token and basic auth remain supported compatibility modes or should be removed in favor of GitHub App-only operation.
- Define the ordinary-user UI surface before adding non-admin screens under `ui/`; include routes, navigation visibility, and API permissions.
- Add an effective-config diagnostics view or config validation workflow if operators need to inspect runtime config from the UI.
- Verify DB lease behavior with two runnerd processes sharing the same database before documenting multi-instance support.
- Decide whether expvar diagnostics need a Prometheus/export adapter or histogram-style latency views for deployment observability.

## Maintenance

- Keep `README.md`, `docs/testing.md`, and this roadmap in sync when build, dev, config, or UI asset workflows change.
- Keep `docs/deployment-smoke.md` aligned with real GitHub App, webhook, E2B template, runner pickup, cleanup, and diagnostics behavior.
- Keep generated production UI assets under `internal/server/ui/` out of hand edits; change source files in `ui/` and rebuild with `task build`.
