# Project Architecture

## Runtime Shape

- `runnerd` is a single Go service that receives GitHub `workflow_job` webhooks, admits jobs by repository and labels, creates Qiniu sandboxes, registers ephemeral GitHub Actions self-hosted runners, and cleans them up.
- Runtime config is file-first. `runnerd` reads `./runnerd.yaml` by default, or another path passed with `--config`.
- Local development should use `runnerd.local.yaml` for secrets and sqlite state.
- Relative sqlite `database.dsn` and `github.app.private_key_file` paths resolve from the directory containing the config file. Legacy `database.url` remains a deprecated alias when `database.dsn` is empty.

## Admin And UI

- The current browser entry for the ordinary-user UI is `/`.
- Ordinary-user job group routes use a source-context path with the jobs view as the terminal resource, such as `/github/pulls/{owner}/{repo}/{number}/jobs`; individual runner job details remain `/jobs/{id}`.
- Ordinary-user account settings live under `/account/repositories`, `/account/preferences`, `/account/sandbox-templates`, `/account/sandbox-instances`, and `/organizations/{login}/...`.
- `/user/sandbox/templates` and `/user/sandbox/instances` resolve encrypted Sandbox credentials from the selected account or GitHub installation scope, then the enabled admin default when the scope is incomplete. They are ordinary-user catalog APIs, not admin configuration APIs.
- The current browser entry for the admin console is `/admin/`.
- `/admin/accounts`, `GET /admin/api/accounts`, and `PATCH /admin/api/accounts/{id}/role` provide a role-gated account list and role controls. Accounts remain OAuth/bootstrap-created; linked identities are read-only display/search data on this admin surface, while provider plus stable subject still binds authentication to the local account.
- Account role updates and their audit events commit atomically. Self-role changes and changes that could leave no administrator are rejected, including concurrent demotions.
- `/admin/sandbox_service` and `/admin/api/sandbox-service-default` manage the platform fallback. Keep this singleton independent from account preferences and disabled by default.
- UI source lives in `ui/`.
- Production UI assets are generated into `internal/server/ui/` by `task ui-build` and embedded by `internal/server/ui_assets_production.go`.
- Development UI assets are proxied to Vite by `internal/server/ui_assets_development.go`.
- Shared `ui/` code serves both ordinary-user and admin screens. Keep admin routes and role-gated APIs explicit when changing shared components.

## Auth And Routing

- The recommended production GitHub auth path is GitHub App auth for runner operations plus GitHub App OAuth sign-in for ordinary users and administrators. Local account roles gate management APIs.
- Token and basic auth still exist as compatibility modes; their long-term product status is undecided.
- GitHub Enterprise Server is not supported. Config validation rejects `github.api_base_url` values other than `https://api.github.com`.
- Runner specs, runner groups, and repository policies are admin API/UI data, not `runnerd.yaml` fields.
- Sandbox credential precedence is request snapshot, installation custom/inherited config, eligible personal account config, enabled and audience-eligible admin default, then not configured. Corrupt scoped config must fail instead of falling through.
- Admin default audience mode is `all` or `selected`. Selected entries match the repository owner's stable GitHub numeric account ID and account type, never the workflow actor or organization membership; an empty selected audience matches nobody.
- Admin audience additions resolve a typed GitHub login to canonical stable ID/type through GitHub before persistence; login remains display metadata. Installation sync and runtime GitHub App lookup both populate the separate installation-owner cache used for selected-audience matching.
- `github.allowed_repositories` limits admitted repositories before runner spec/policy matching.

## State And Migrations

- Runtime state can use sqlite, Postgres, or MySQL.
- Do not document multi-instance support until two runnerd processes have been verified against the same database.
- State schema is defined mostly by GORM tags in `internal/state/records.go`.
- Startup migration runs a narrow legacy compatibility pass in `internal/state/db.go`, then GORM `AutoMigrate`. Existing SQLite `runner_requests` tables use additive missing-column/index migration instead of generic table recreation so ALTER-added historical fields remain intact.
- GORM foreign-key creation is intentionally disabled. Legacy compatibility may remove old constraints or reset incompatible legacy scope tables; keep every such action narrow and covered by old-schema tests. Pre-scope account preference/secret rows are intentionally erased, requiring Sandbox reconfiguration and GitHub reauthentication before installation sync.
- Keep old-schema upgrade tests when changing state records, GORM tags, indexes, required columns, or relationship constraints.
- Do not reintroduce legacy `users` migration behavior unless the user explicitly asks for that compatibility path.

## Runner Lifecycle

- Runner request states are `queued`, `creating`, `running`, `stopping`, `completed`, and `failed`.
- Global `worker.max_concurrent_runners` and per-spec `max_concurrency` are enforced by worker processing; excess work stays queued.
- Transient Qiniu sandbox placement failures, HTTP 429s, and GitHub secondary rate limits are queue deferrals. Deterministic auth/config/template failures should fail immediately.
- Control/stdout/stderr logs are persisted as runner events and exposed through the admin API/UI.
