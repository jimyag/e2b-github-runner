<h1 align="center">Qiniu Sandbox GitHub Runner</h1>

<p align="center">
  <strong>Ephemeral, isolated GitHub Actions runners powered by Qiniu Sandbox</strong>
</p>

<p align="center">
  <a href="./README.zh.md">中文</a> ·
  <a href="#quick-start">Quick Start</a> ·
  <a href="https://app-6a6b0d723d3a24e095531129.app.qiniucc.com/">Deploy to Qiniu LAS</a> ·
  <a href="https://github.com/qiniu/ci-runner/issues/59">Deployment Guide</a> ·
  <a href="#documentation">Documentation</a> ·
  <a href="#license">License</a> ·
  <a href="#community--contributing">Community &amp; Contributing</a>
</p>

---

Qiniu Sandbox GitHub Runner provisions a clean [Qiniu Sandbox](https://www.qiniu.com/) for each GitHub Actions workflow job, registers a [self-hosted runner](https://docs.github.com/en/actions/hosting-your-own-runners/about-self-hosted-runners) just in time, and removes the runner and sandbox when the job ends. Teams keep the familiar GitHub Actions workflow while moving each job into a disposable environment.

The Qiniu CI Runner control plane is open source. Qiniu Sandbox, where workflow jobs execute, is a cloud service provided and operated by Qiniu.

## Core Capabilities

- **Ephemeral runners** — one sandbox per job, automatically cleaned up after completion
- **GitHub App auth** — recommended production path with OAuth sign-in for the built-in web console
- **Multi-database** — SQLite (default), PostgreSQL, or MySQL for runtime state
- **Concurrency control** — global `max_concurrent_runners` and per-spec `max_concurrency` with queue-based backpressure
- **Built-in web UI** — admin console for runner specs, groups, policies, accounts, and diagnostics; ordinary-user console for job groups, logs, and sandbox management
- **Config obfuscation** — sensitive values can be hidden from casual config inspection
- **Retry & recovery** — transient failures are retried with backoff; queued work and active remote runners are recovered after a service restart

## How It Works

```
GitHub webhook (workflow_job)
        │
        ▼
   ┌─────────┐     create sandbox      ┌──────────────────┐
   │ runnerd  │ ──────────────────────►  │  Qiniu Sandbox   │
   │ (server) │     register runner     │  (ephemeral VM)  │
   │          │ ──────────────────────►  │                  │
   └─────────┘                          │  GitHub Actions  │
        │                               │  self-hosted     │
        │  job completed / timeout      │  runner          │
        │◄────────────────────────────── │                  │
        │     stop & cleanup sandbox    └──────────────────┘
        ▼
   state DB (sqlite / postgres / mysql)
```

1. GitHub sends a `workflow_job` (queued) webhook to runnerd.
2. runnerd matches the job labels against runner specs and policies.
3. runnerd creates a Qiniu Sandbox instance and registers a self-hosted runner inside it.
4. GitHub Actions dispatches the job to the runner; the job executes in the sandbox.
5. When the job completes (or times out), runnerd removes the runner registration and stops the sandbox.

## Quick Start

```bash
# 1. Build
task build

# 2. Create config from example
cp runnerd.yaml.example runnerd.yaml
#    Edit runnerd.yaml: set database, GitHub App credentials, sandbox settings

# 3. Bootstrap the first admin (one-time, exits without starting the server)
./bin/runnerd --bootstrap-admin github:<github-user-id> --config runnerd.yaml

# 4. Start runnerd
./bin/runnerd --config runnerd.yaml
```

5. Open `http://<host>:25500/`. The public product landing page links to the current GitHub documentation and the protected Jobs console at `/jobs`. On the first authenticated visit to `/jobs`, a six-step product tour introduces Jobs, Repositories, Settings, and Sandbox setup; it can be replayed from the account menu.
6. Open **Repositories** to review **Runner readiness** for the account or organization. Ready sources are shown without configuration controls. If Sandbox setup is missing and you can manage that scope, use **Configure Sandbox** to open the exact account or organization Settings page. Settings lists only your account and organizations where you are an active member; outside collaborators receive a read-only readiness prompt and cannot browse that organization's Sandbox catalogs.
7. In the **Admin Console**, create a **Runner Spec** with a meaningful label (e.g. `ubuntu-24-04`), set its `template_id`, and enable `default_available`.
8. Configure a GitHub webhook → `POST http://<host>:25500/webhooks/github`.
9. Use `runs-on: [self-hosted, <your-runner-label>]` in your workflow.

For local development, use `task dev` with `runnerd.local.yaml`. See [docs/testing.md](docs/testing.md) for detailed local setup including GitHub App creation and webhook forwarding.

## Configuration

`runnerd` reads `./runnerd.yaml` by default, or the path passed with `--config`. See [`runnerd.yaml.example`](runnerd.yaml.example) for a fully commented reference.

| Section | Description |
| --- | --- |
| `server` | Listen address, read/write/idle timeouts |
| `database` | Backend (`sqlite` / `postgres` / `mysql`) and DSN |
| `auth` | Session secret, encryption key, session TTL |
| `sandbox` | Sandbox lifecycle timeouts (create, run, stop) |
| `github` | Webhook secret, auth method (App / PAT / basic), OAuth, allowed repositories |
| `worker` | Lease, retry, and concurrency settings |

Key notes:

- Relative `database.dsn` and `github.app.private_key_file` paths resolve from the config file's directory.
- Use SQLite for local and single-node deployments. PostgreSQL and MySQL are supported but multi-instance operation on a shared database has not been verified.
- Existing SQLite `runner_requests` tables add missing model columns and indexes on startup without table recreation. Creating the list-ordering indexes does not rewrite runner rows, but it can add brief startup I/O and lock contention on a large database; see [docs/testing.md](docs/testing.md) for the migration and query-plan checks.
- GitHub Enterprise Server is **not** supported; use a GitHub.com App.
- Configure exactly one GitHub auth method: `github.app`, `github.token`, or `github.basic_auth`.
- When `github.app.installation_id` is omitted, runnerd resolves the installation dynamically per repository, allowing one App to serve multiple accounts.

### Config Value Obfuscation

Sensitive fields accept `RUNNERD_ENC(v1:...)` values to avoid plaintext in the config file:

```bash
read -r -s secret_value
printf '%s' "$secret_value" | ./bin/runnerd --obfuscate-config-value
unset secret_value
```

Supported fields: `database.dsn`, `auth.session_secret`, `auth.encryption_key`, `github.webhook_secret`, `github.token`, `github.basic_auth.password`, `github.oauth.client_secret`. These values are also masked as `******` in logs and serialized output.

> **Note:** This hides plaintext from casual inspection only — the decoding key is embedded in the binary. It is not encryption against a host-level attacker.

## GitHub App Setup

### Required Permissions

| Scope | Permission | Access | Purpose |
| --- | --- | --- | --- |
| Repository | Actions | Read-only | Query job/run status, list queued jobs, read logs; required for webhook events |
| Repository | Administration | Read & write | Repository-level runner registration (when spec has no `runner_group`) |
| Repository | Metadata | Read-only | Identify repositories and owners |
| Repository | Pull requests | Read-only | Show PR titles in job groups |
| Organization | Members | Read-only | Verify active organization membership for organization Settings and scoped Sandbox management |
| Organization | Self-hosted runners | Read & write | Organization-level runner registration (when spec sets `runner_group`) |

Set `github.app.slug` to show an "Install GitHub App" link in the user UI. Use `github.allowed_repositories` (patterns like `owner/repo` or `owner/*`) to restrict which repositories can use this runnerd instance.

### OAuth Sign-in

`github.oauth` enables GitHub App OAuth login for the built-in console:

- Use the GitHub App's **Client ID** and **Client Secret**.
- Set the App callback URL to `http://<host>:<port>/auth/github/callback`.
- Set `auth.session_secret` (session signing) and `auth.encryption_key` (user secret encryption) to separate random values.

First OAuth login creates a `role: user` account. Use `--bootstrap-admin <github-user-id>` to promote an account to admin.

### Webhook Events

In your GitHub App settings (**Settings → Developer settings → GitHub Apps → your app → General**), configure:

1. Set the **Webhook URL** to `https://<your-runnerd-host>/webhooks/github`.
2. Under **Subscribe to events**, check:
   - **Workflow jobs** (`workflow_job`) — **required**, triggers runner creation.
   - **Workflow runs** (`workflow_run`) — optional, acts as a compensating signal for missed `workflow_job` events.
3. Save changes.

> **⚠️ Common pitfall:** If no events are subscribed, GitHub will not send any webhooks and jobs will stay queued forever. This is configured in the **GitHub App settings**, not in the repository's webhook settings.

## Webhook & Workflow Setup

1. Ensure the GitHub App webhook is configured as described in [Webhook Events](#webhook-events) above, with the `webhook_secret` matching `github.webhook_secret` in your config.
2. In your workflow, use:

```yaml
runs-on: [self-hosted, <your-runner-label>]
```

runnerd handles `queued`, `in_progress`, and `completed` actions. For `workflow_run`, it lists all queued jobs in the run and enqueues any matching jobs not already seen.

## Runner Specs & Policies

Runner specs, runner groups, and repository policies are managed through the admin API and console — not through `runnerd.yaml`.

- **Runner Spec**: defines a runner label, sandbox template, and optional `runner_group`. Set `default_available: true` to make it available to all allowed repositories.
- **Runner Group**: when a spec sets `runner_group`, runnerd creates an organization-level runner in that group; otherwise it creates a repository-level runner.

> **⚠️ Personal accounts:** `runner_group` requires the organization-level GitHub API. If the repository belongs to a personal account (not an organization), leave `runner_group` **empty** — otherwise runner registration will fail with a 404 error.
- **Repository Policy**: grants a specific repository access to additional specs beyond the defaults.

Each spec's `template_id` should point to a Qiniu Sandbox template containing the GitHub runner image. Template access is checked against the repository owner's effective Sandbox service shown under **Repositories → Runner readiness** at sandbox creation time.

## Admin Console

The built-in web UI provides:

| Route | Description |
| --- | --- |
| `/admin/` | Dashboard with diagnostics, metrics, and recent failures |
| `/admin/accounts` | Account management — list, search, and change roles |
| `/admin/sandbox_service` | Sandbox service configuration |

`/` is always the public Qiniu CI Runner product landing page. The ordinary-user Jobs homepage is `/jobs`; other protected routes include `/repositories`, PR job groups (`/github/pulls/{owner}/{repo}/{number}/jobs`), and account settings (`/account/preferences`, `/account/sandbox-templates`, `/account/sandbox-instances`), with matching `/organizations/{login}/...` routes. Opening a protected route without a session shows a focused GitHub sign-in page and returns to the original URL after OAuth.

Runner request lists return the newest 100 rows by default and cap pages at 500. They project only public runner-state fields instead of stored webhook payloads or Sandbox credentials. Admin polling uses the `(queued_at DESC, id ASC)` index; repository-authorized user polling queries each installation through `(github_installation_id, queued_at DESC, id ASC)` and merges the bounded results while preserving exact installation/repository access pairs.

## Troubleshooting

| Symptom | Likely Cause | Fix |
| --- | --- | --- |
| Job stays **queued** forever, no webhook in runnerd logs | GitHub App has no subscribed events | Go to GitHub App settings → Subscribe to **Workflow jobs** event |
| `github registration token: status 404` | `runner_group` is set but the repo owner is a personal account | Clear `runner_group` in the runner spec to use repository-level registration |
| `invalid signature` in logs | Webhook secret mismatch | Ensure `github.webhook_secret` matches the secret in GitHub App/repo webhook settings |
| `runner start deferred ... at capacity` | Global or per-spec concurrency limit reached | Wait for running jobs to finish, or increase `max_concurrent_runners` / spec `max_concurrency` |
| Sandbox creation fails | Repository owner has no effective Sandbox service | Open **Repositories**, select the account or organization, and complete **Runner readiness**; admins may also configure an eligible fallback at `/admin/sandbox_service` |

For detailed local debugging steps, see [docs/testing.md](docs/testing.md#8-troubleshooting-order).

## Docker

The container image uses file-config only. Mount `runnerd.yaml` and any referenced secret files into the container:

```bash
docker run --rm -p 25500:25500 \
  -v "$PWD/runnerd.yaml:/etc/runnerd/runnerd.yaml:ro" \
  -v "$PWD/secrets:/etc/runnerd/secrets:ro" \
  ghcr.io/qiniu/ci-runner
```

## Build & Development

```bash
task deps          # Install Go dependencies
task ui-deps       # Install UI dependencies
task build         # Build runnerd with embedded production UI
task ui-production-smoke # Execute the production UI bundle in Chromium
task dev           # Start local dev (runnerd + Vite + smee)
task lint          # Run linters
task test          # Rebuild UI + run all tests (Go with race detection + Bun UI tests)
task docker-check  # Verify Docker build
task release-check # Verify release build
```

For focused UI tests, run `cd ui && bun run test`. Use `task ui-production-smoke`
after changing UI dependencies, Vite/Rollup configuration, or production asset
loading.

### Sandbox Templates

| Template | Description |
| --- | --- |
| `templates/github-runner-ubuntu-slim` | Maintained Ubuntu Slim x64 runner template |
| `templates/github-runner-ubuntu-22.04` | Maintained Ubuntu 22.04 x64 runner template |
| `templates/github-runner-ubuntu-24.04` | Maintained Ubuntu 24.04 x64 runner template |
| `templates/github-runner-ubuntu-26.04` | Preview Ubuntu 26.04 x64 runner template |
| `templates/qbox-kodo-ubuntu-16.04` | Legacy Ubuntu 16.04 for qbox/kodo-style jobs |

Run `task template-check-all`, then use the four
`task template-build-ubuntu-*` targets for real qshell Sandbox builds. See
[Public Runner Templates](docs/default-runner-templates.md) for publication and
smoke commands. The qbox-kodo base image remains a separate
`task qbox-kodo-base-build` workflow.

## Documentation

| Document | Description |
| --- | --- |
| [docs/testing.md](docs/testing.md) | Local testing, GitHub App/OAuth setup, webhook forwarding, troubleshooting |
| [docs/deployment-smoke.md](docs/deployment-smoke.md) | Production-style readiness checklist |
| [docs/default-runner-templates.md](docs/default-runner-templates.md) | Public template labels, qshell release flow, regional smoke, and rollback |
| [docs/runner-architecture-comparison.md](docs/runner-architecture-comparison.md) | Architecture diagrams and comparison with ARC / Fireactions |
| [docs/runner-implementation-review.md](docs/runner-implementation-review.md) | Implementation status and schema migration notes |

## License

Qiniu CI Runner is licensed under the [Apache License 2.0](LICENSE).

## Community & Contributing

Bug reports, feature ideas, documentation improvements, and code contributions are welcome.

- [Report a bug or propose a feature](https://github.com/qiniu/ci-runner/issues).
- [Open a Pull Request](https://github.com/qiniu/ci-runner/pulls) to improve the code or documentation.
- Scan the QR code below to join the community chat.

---

<p align="center">
  <img src="./docs/assets/qrcode.png" width="220" alt="Qiniu CI Runner community chat QR code" />
</p>
<p align="center">
  <em>Scan the QR code to connect with maintainers and other Qiniu CI Runner users.</em>
</p>
