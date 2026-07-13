# Local Testing And GitHub Setup

[Chinese](zh/testing.md)

This guide explains how to test the service with a local Qiniu sandbox environment and how to configure a GitHub repository so self-hosted runners are started automatically.

## 1. Local Config File

The service reads `./runnerd.yaml` by default. You can also pass another path with `--config`:

```bash
cp runnerd.yaml.example runnerd.yaml
mkdir -p ./secrets
```

Relative sqlite `database.dsn` and `github.app.private_key_file` paths are resolved from the directory containing `runnerd.yaml`. Legacy `database.url` is still accepted as a deprecated alias when `database.dsn` is empty. Only GitHub.com is currently supported; GitHub Enterprise Server is not supported. GitHub auth can use GitHub App, PAT token, or basic auth, but exactly one mode must be configured.

Minimal usable config example:

```yaml
server:
  http_addr: ":25500"

database:
  backend: sqlite
  dsn: ./var/runnerd.db

auth:
  session_secret: <random session signing secret>
  encryption_key: <separate random encryption key>
  session_ttl_hours: 12

github:
  webhook_secret: <random webhook secret>
  app:
    id: <github app id>
    # Optional. When omitted, runnerd resolves the installation dynamically
    # from the repository in the webhook payload.
    # installation_id: <installation id>
    private_key_file: ./secrets/github-app.pem
  oauth:
    client_id: <github app client id>
    client_secret: <github app client secret>
    redirect_url: http://127.0.0.1:25500/auth/github/callback
  # Optional. Empty means all installed App repositories that match runner
  # policies/specs are allowed.
  # allowed_repositories:
  #   - <repo owner>/*
  #   - <repo owner>/<repo name>

worker:
  max_concurrent_runners: 100
  recovery_timeout_seconds: 120
  lease_ttl_seconds: 300
  retry_base_delay_seconds: 15
  retry_max_delay_seconds: 300
  retry_max_attempts: 5
```

Sandbox service API URL and API Key are not configured in `runnerd.yaml`. After signing in to the ordinary-user UI, configure them on the account or organization Preferences page. The API Key is encrypted with `auth.encryption_key`.

Runner spec, runner group, and repository policy are not `runnerd.yaml` fields. Create them from the admin page or admin API after the service starts. Use meaningful spec names such as `ubuntu-24-04`; set `template_id` to the matching Qiniu sandbox template ID. Template access is checked when runnerd starts a sandbox with the account or organization Sandbox service config.

`database.backend` supports `sqlite`, `postgres`, and `mysql`. Prefer sqlite for local development. Before documenting shared-database multi-instance deployment as supported, verify lease behavior with two runnerd processes sharing the same database.

The state schema is mainly defined by GORM tags in `internal/state/records.go`. On startup, the service runs a small old-schema compatibility column pass and then GORM `AutoMigrate`. When changing state records, indexes, or migration helpers, run at least:

```bash
go test ./internal/state -count=1
```

Do not validate migrations only with a fresh sqlite file. Old-schema upgrade paths also need coverage, especially when adding `NOT NULL` columns, unique indexes, or relationship constraints.

## 2. Configure GitHub Auth

GitHub App is recommended. PAT token and basic auth are also supported, mainly for local verification or existing credential scenarios.

The GitHub App must be able to call the runner registration token API. Repository runners require administration permission on the target repository. When using a GitHub runner group, runnerd creates an organization runner and needs organization-level self-hosted runner management permission.

Suggested setup:

1. Open GitHub `Settings -> Developer settings -> GitHub Apps -> New GitHub App`.
2. Basic information:
   - GitHub App name: for example `runnerd-local`
   - Homepage URL: use the repository URL or local project docs URL
   - Setup URL: runnerd's `/github-app/setup`, for example `http://127.0.0.1:25500/github-app/setup`
   - Webhook: if runnerd receives repository webhooks itself, you can leave the App webhook disabled; this is different from the `workflow_job` webhook
3. Repository permissions:
   - Set `Administration` to `Read and write`
4. Organization permissions, if you need organization runners:
   - Enable the corresponding self-hosted runner management permission
5. Where can this GitHub App be installed:
   - For local verification, usually choose `Only on this account`
6. After creating the App, generate a private key on the App page, download the `.pem`, and save it locally, for example `./secrets/github-app.pem`
7. Install the App on the target repository or organization:
   - Click `Install App`
   - Choose the target owner
   - Choose the repositories to authorize
8. Record:
   - App ID
   - App slug, the short name in the App URL such as `https://github.com/apps/<slug>`
   - Installation ID, optional; when omitted, runnerd resolves it dynamically by repository
   - private key file path

Put the values into `runnerd.yaml`:

```yaml
github:
  app:
    id: <app id>
    slug: <app slug>
    # installation_id: <installation id>
    private_key_file: ./secrets/github-app.pem
```

PAT example:

```yaml
github:
  webhook_secret: <random webhook secret>
  token: <github token>
```

Basic auth example:

```yaml
github:
  webhook_secret: <random webhook secret>
  basic_auth:
    username: <github username>
    password: <token or password>
```

No global repo/org mode is required. Webhooks use `repository.full_name` from the payload. runnerd creates repository runners by default. If the matched runner spec sets GitHub `runner_group`, runnerd creates an organization runner for the repository owner and passes the group as `--runnergroup` during GitHub runner registration. Specs with `runner_specs.default_available: true` are available to all repositories by default; `runner_policies` are only needed to grant a repository or repository wildcard an additional special spec, for example `jimyag/*` or `jimyag/template-repository`.

## 3. Start The Service

For UI or backend development, prefer development mode:

```bash
task deps
task ui-deps
cp runnerd.yaml.example runnerd.local.yaml
task dev
```

`task dev` reads `runnerd.local.yaml` by default, starts the Vite dev server on the first available port at or after `127.0.0.1:5173`, and starts the Go service with the `development` build tag. Browsers still access the runnerd address.

Ordinary-user UI:

```text
http://127.0.0.1:25500/
```

Ordinary-user Activity repositories page:

```text
http://127.0.0.1:25500/repositories
```

Ordinary-user account repositories page:

```text
http://127.0.0.1:25500/account/repositories
```

Ordinary-user personal Preferences page:

```text
http://127.0.0.1:25500/account/preferences
```

Ordinary-user personal Sandbox catalogs:

```text
http://127.0.0.1:25500/account/sandbox-templates
http://127.0.0.1:25500/account/sandbox-instances
```

Admin UI:

```text
http://127.0.0.1:25500/admin/
```

To use another config file:

```bash
RUNNERD_CONFIG=./runnerd.yaml task dev
```

To pin the Vite port:

```bash
RUNNERD_VITE_PORT=5173 task dev
```

For production mode or embedded UI asset verification, start the Go service directly:

```bash
go run ./cmd/runnerd --config ./runnerd.yaml
```

Health check:

```bash
curl -fsS http://127.0.0.1:25500/healthz
```

Ordinary-user page:

```text
http://127.0.0.1:25500/
```

The page redirects to GitHub OAuth login. The first login creates a local account with `role=user` and links the GitHub OAuth identity. The first admin must be explicitly bootstrapped at startup:

```bash
go run ./cmd/runnerd --config ./runnerd.yaml --bootstrap-admin github:<your-github-user-id>
```

`<your-github-user-id>` is the stable numeric `id` returned by GitHub `/user`, not the mutable login. Role belongs to the local account. OAuth identity stores provider, stable subject, and login display metadata, so other provider identities can later be linked to the same account. After ordinary users sign in, they can install the configured GitHub App from `/account/repositories`, configure Sandbox service from `/account/preferences` or `/organizations/{login}/preferences`, and inspect scoped resources from the Sandbox Templates and Sandbox Instances tabs. When GitHub returns an `installation_id`, runnerd records the GitHub App installation linked to that account. Jobs visible to an ordinary user are filtered by the installation id in the workflow job payload. runnerd does not copy the full repository authorization list for that installation into local state. The account repositories page loads the installation's current authorized repositories on demand through the GitHub App API. The catalog endpoints require an ordinary-user session, use the account or selected installation's encrypted credentials, map supported region ids to server-owned endpoints, and never expose the credentials. After an admin signs in, the browser stores a signed HttpOnly session cookie and automatically sends it to management APIs such as `/runner_requests`. For `curl` examples, export a cookie jar from the browser or OAuth debug flow:

```bash
export COOKIE_JAR=./runnerd.cookies
```

UI source lives in `ui/` and uses the same React, Vite, Tailwind CSS, shadcn-style components, and theme CSS as `kubevirt-console`. `task build` runs `task ui-build`, writes frontend output to `internal/server/ui/`, and then compiles `runnerd`. In development mode, `internal/server/ui_assets_development.go` proxies UI assets to Vite. In production builds, `internal/server/ui_assets_production.go` embeds `internal/server/ui/*`. The ordinary-user UI includes GitHub App accounts and on-demand authorized repositories at `/account/repositories`, Sandbox service settings at `/account/preferences` and `/organizations/{login}/preferences`, region-filtered templates at `/account/sandbox-templates`, region- and template-filtered runner instances at `/account/sandbox-instances`, the equivalent organization routes, local activity repositories at `/repositories`, the Repo/PR job list at `/`, stable GitHub-context job-group routes such as `/github/pulls/{owner}/{repo}/{number}/jobs`, and job details at `/jobs/{id}`. The catalog uses `GET /user/sandbox/templates?region=<id>` and `GET /user/sandbox/instances?region=<id>&template_id=<id>`; the instance endpoint lists only runner-created sandboxes. The admin surface includes runners, runner specs, runner groups, runner policies, retry, audit, label match test, and diagnostics pages, but not provider resource catalogs.

Create a default runner spec first:

```bash
curl -fsS -X POST http://127.0.0.1:25500/runner_specs \
  -b "$COOKIE_JAR" \
  -H 'content-type: application/json' \
  -d '{"name":"ubuntu-24-04","labels":["self-hosted","e2b"],"template_id":"<template id>","max_concurrency":100,"enabled":true,"default_available":true}' | jq
```

Manually create a runner:

```bash
curl -fsS -X POST http://127.0.0.1:25500/runner_requests \
  -b "$COOKIE_JAR" \
  -H 'content-type: application/json' \
  -d '{"id":"manual-001","repository_full_name":"<owner>/<repo>","runner_spec_name":"ubuntu-24-04"}' | jq
```

Check state:

```bash
curl -fsS -b "$COOKIE_JAR" http://127.0.0.1:25500/runner_requests | jq
curl -fsS -b "$COOKIE_JAR" http://127.0.0.1:25500/runner_requests/manual-001 | jq
```

Stop a runner:

```bash
curl -fsS -X DELETE -b "$COOKIE_JAR" http://127.0.0.1:25500/runner_requests/manual-001 | jq
```

The default state database is written to:

```text
var/runnerd.db
```

Runner control/stdout/stderr logs are stored in the DB-backed event store and can still be read through the management API:

```bash
curl -fsS -b "$COOKIE_JAR" \
  http://127.0.0.1:25500/runner_requests/manual-001/logs/control.log
curl -fsS -b "$COOKIE_JAR" \
  http://127.0.0.1:25500/runner_requests/manual-001/logs/stdout.log
curl -fsS -b "$COOKIE_JAR" \
  http://127.0.0.1:25500/runner_requests/manual-001/logs/stderr.log
```

## 4. First Startup Check

Confirm runnerd read the GitHub App config correctly:

```bash
curl -fsS -b "$COOKIE_JAR" http://127.0.0.1:25500/diagnostics/pprof | jq
```

Check:

- whether `github.auth_mode` is `app`;
- if a static installation is configured, whether `github.installation_id` matches expectations; in dynamic installation mode it may be `0`;
- whether `state.database` points at the database configured in `runnerd.yaml`.

## 5. Expose A Webhook URL

GitHub webhooks must be able to reach the local service. Choose one option.

Use smee:

```bash
open https://smee.io/new
echo 'https://smee.io/<your-channel>' > .smee-url
task dev
```

Use the same smee URL as the GitHub webhook Payload URL. When `.smee-url` exists, `task dev` starts the smee forwarder automatically. You can also start forwarding standalone with `task smee`. By default it forwards to `http://127.0.0.1:25500/webhooks/github`. If `runnerd.yaml` uses another listener address, set `SMEE_TARGET`:

```bash
SMEE_TARGET=http://127.0.0.1:25501/webhooks/github task smee
```

You can also use ngrok:

```bash
ngrok http 25500
```

Or cloudflared:

```bash
cloudflared tunnel create e2b-local-runner
cloudflared tunnel route dns e2b-local-runner runner.example.com
cloudflared tunnel run --url http://127.0.0.1:25500 e2b-local-runner
```

The final webhook URL looks like:

```text
https://<public-host>/webhooks/github
```

Replace `runner.example.com` with your own domain. Do not hard-code a random temporary `trycloudflare.com` quick tunnel address into GitHub settings.

For public deployments, only `/webhooks/github` needs to be exposed to GitHub. The `/runner_requests` management API can be served by the same service, but it must include a valid OAuth admin session cookie. In production, put it behind an HTTPS reverse proxy and restrict management API source IPs.

## 6. Configure GitHub Repository Webhook

In the target repository, open:

```text
Settings -> Webhooks -> Add webhook
```

Fill in:

- Payload URL: `https://<public-host>/webhooks/github`
- Content type: `application/json`
- Secret: exactly the same as `github.webhook_secret` in `runnerd.yaml`
- Which events: choose `Workflow jobs`. If you want the compensating path, also choose `Workflow runs`
- Active: checked

After saving, GitHub sends a ping. The service handles `workflow_job.queued` / `workflow_job.in_progress` / `workflow_job.completed` as the main path, and `workflow_run.requested` / `workflow_run.in_progress` as a compensating path. Other events return ignored; this is expected.

## 7. Configure GitHub Actions Workflow

Add this to the target repository:

```yaml
name: e2b-runner-smoke

on:
  workflow_dispatch:

jobs:
  smoke:
    runs-on: [self-hosted, e2b]
    steps:
      - name: Print runner info
        run: |
          uname -a
          whoami
          pwd
```

After triggering `workflow_dispatch`, the expected flow is:

1. GitHub creates a `workflow_job.queued` webhook.
2. This service verifies the signature and writes a `queued` runner request into the state database.
3. The service creates a sandbox, obtains a GitHub registration token, and starts an ephemeral runner inside the sandbox.
4. The GitHub job is picked up by the `self-hosted,e2b` runner.
5. After the runner process exits, the service cleans up the sandbox.

If `Workflow runs` events are also configured, `workflow_run.requested` / `workflow_run.in_progress` are only compensating signals. runnerd queries queued jobs in the run and creates runner requests for matching jobs that have not already been enqueued through `workflow_job`. This compensating action does not immediately make the GitHub Actions UI show the job as running. The UI remains queued / waiting for runner until the ephemeral runner inside the sandbox registers successfully and GitHub assigns it to that job.

## 8. Troubleshooting Order

First check service state:

```bash
curl -fsS -b "$COOKIE_JAR" http://127.0.0.1:25500/runner_requests | jq
```

Then inspect the request state and logs:

```bash
curl -fsS -b "$COOKIE_JAR" \
  http://127.0.0.1:25500/runner_requests/<request_id> | jq
curl -fsS -b "$COOKIE_JAR" \
  http://127.0.0.1:25500/runner_requests/<request_id>/logs/control.log
curl -fsS -b "$COOKIE_JAR" \
  http://127.0.0.1:25500/runner_requests/<request_id>/logs/stdout.log
curl -fsS -b "$COOKIE_JAR" \
  http://127.0.0.1:25500/runner_requests/<request_id>/logs/stderr.log
```

Common issues:

- `invalid signature`: GitHub webhook secret and `github.webhook_secret` do not match.
- `runner concurrency limit reached`: active request count reached `worker.max_concurrent_runners`.
- GitHub job stays queued: workflow `runs-on` labels must include `self-hosted` and `e2b`, and must match runner spec labels.
- sandbox creation fails: confirm account or organization Preferences have Sandbox service config matching the template and local environment.
- registration token fails: confirm the GitHub App installation has the required administration/self-hosted runner permission for the target repository.

## 9. How To Read GitHub Actions Logs

This is a repository-level self-hosted GitHub Actions runner. After a job is picked up by the runner inside the sandbox, workflow step logs appear normally in GitHub Actions:

```text
Repository -> Actions -> choose workflow run -> choose job
```

GitHub Actions shows:

- workflow step stdout/stderr;
- logs for checkout, build, test, and other steps;
- job success, failure, or cancellation status.

Do not rely on GitHub Actions for all control-plane logs:

- sandbox creation failure logs, because the runner has not registered with GitHub yet;
- errors before runner download, `config.sh` registration, or `run.sh` startup;
- webhook validation failures, GitHub token request failures, or sandbox API call failures.

Read those control-plane logs from this service's management API:

```bash
curl -fsS -b "$COOKIE_JAR" \
  http://127.0.0.1:25500/runner_requests/<request_id> | jq
curl -fsS -b "$COOKIE_JAR" \
  http://127.0.0.1:25500/runner_requests/<request_id>/logs/control.log
curl -fsS -b "$COOKIE_JAR" \
  http://127.0.0.1:25500/runner_requests/<request_id>/logs/stdout.log
curl -fsS -b "$COOKIE_JAR" \
  http://127.0.0.1:25500/runner_requests/<request_id>/logs/stderr.log
```

The service's own logs are printed to the terminal that started `go run ./cmd/runnerd`.

## 10. Diagnostics / pprof

The service imports `github.com/jimmicro/pprof`. After startup it generates `.pprof` address files and dump scripts near the binary. Diagnostics can be read directly from the management API:

```bash
curl -fsS -b "$COOKIE_JAR" http://127.0.0.1:25500/diagnostics/pprof | jq
curl -fsS -b "$COOKIE_JAR" http://127.0.0.1:25500/diagnostics/vars | jq
```

`/diagnostics/pprof` returns:

- discovered pprof address files;
- dump script paths;
- current DB path;
- GitHub auth mode, `app`, `token`, or `basic`;
- recent failed runner requests.

`/diagnostics/vars` proxies the local pprof service's `GET /debug/vars`, so you can read expvar metrics directly. Current metrics cover profile current/busy/idle/pending/desired, retry/lease, create/stop counts and durations, GitHub API calls, runner registration/cleanup, and workflow job queued/started/completed, conclusion, failure, queue duration, and run duration.

## 11. Official References

- GitHub self-hosted runner workflow labels: https://docs.github.com/en/actions/hosting-your-own-runners/managing-self-hosted-runners/using-self-hosted-runners-in-a-workflow
- GitHub self-hosted runner autoscaling: https://docs.github.com/en/actions/hosting-your-own-runners/autoscaling-with-self-hosted-runners
- GitHub webhook `workflow_job` event: https://docs.github.com/en/webhooks/webhook-events-and-payloads
