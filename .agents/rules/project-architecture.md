# Project Architecture

## Runtime Shape

- `runnerd` is a single Go service that receives GitHub `workflow_job` webhooks, admits jobs by repository and labels, creates Qiniu sandboxes, registers ephemeral GitHub Actions self-hosted runners, and cleans them up.
- Runtime config is file-first. `runnerd` reads `./runnerd.yaml` by default, or another path passed with `--config`.
- Local development should use `runnerd.local.yaml` for secrets and sqlite state.
- Relative sqlite `database.dsn` and `github.app.private_key_file` paths resolve from the directory containing the config file. Legacy `database.url` remains a deprecated alias when `database.dsn` is empty.

## Admin And UI

- The current browser entry for the ordinary-user UI is `/`.
- Ordinary-user job group routes use a source-context path with the jobs view as the terminal resource, such as `/github/pulls/{owner}/{repo}/{number}/jobs`; individual runner job details remain `/jobs/{id}`.
- Ordinary-user account settings live under `/account/repositories`, `/account/preferences`, and `/organizations/{login}/...`.
- The current browser entry for the admin console is `/admin/`.
- UI source lives in `ui/`.
- Production UI assets are generated into `internal/server/ui/` by `task ui-build` and embedded by `internal/server/ui_assets_production.go`.
- Development UI assets are proxied to Vite by `internal/server/ui_assets_development.go`.
- Shared `ui/` code serves both ordinary-user and admin screens. Keep admin routes and role-gated APIs explicit when changing shared components.

## Auth And Routing

- The recommended production GitHub auth path is GitHub App auth plus GitHub App OAuth admin login.
- Token and basic auth still exist as compatibility modes; their long-term product status is undecided.
- GitHub Enterprise Server is not supported. Config validation rejects `github.api_base_url` values other than `https://api.github.com`.
- Runner specs, runner groups, and repository policies are admin API/UI data, not `runnerd.yaml` fields.
- `github.allowed_repositories` limits admitted repositories before runner spec/policy matching.

## State And Migrations

- Runtime state can use sqlite, Postgres, or MySQL.
- Do not document multi-instance support until two runnerd processes have been verified against the same database.
- State schema is defined mostly by GORM tags in `internal/state/records.go`.
- Startup migration runs a narrow legacy-column compatibility pass in `internal/state/db.go`, then GORM `AutoMigrate`.
- Keep old-schema upgrade tests when changing state records, GORM tags, indexes, required columns, or relationship constraints.
- Do not reintroduce legacy `users` migration behavior unless the user explicitly asks for that compatibility path.

## Runner Lifecycle

- Runner request states are `queued`, `creating`, `running`, `stopping`, `completed`, and `failed`.
- Global `worker.max_concurrent_runners` and per-spec `max_concurrency` are enforced by worker processing; excess work stays queued.
- Transient Qiniu sandbox placement failures, HTTP 429s, and GitHub secondary rate limits are queue deferrals. Deterministic auth/config/template failures should fail immediately.
- Control/stdout/stderr logs are persisted as runner events and exposed through the admin API/UI.
