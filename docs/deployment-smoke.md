# Deployment Smoke Checklist

[Chinese](zh/deployment-smoke.md)

Use this checklist before treating a runnerd deployment as ready for real GitHub Actions traffic.

## Prerequisites

- A runnerd deployment reachable over HTTPS for GitHub webhooks and console sign-in.
- A `runnerd.yaml` with `database`, `auth`, `github`, and `worker` sections configured.
- A GitHub.com App installed on the target repository or organization.
- The GitHub App has the [required repository and organization permissions](../README.md#github-app-permissions) for the runner modes used by this deployment.
- A GitHub App OAuth callback URL pointing at `/auth/github/callback` on the runnerd origin.
- A GitHub App webhook or repository webhook delivering `workflow_job` events to `POST /webhooks/github`.
- Sandbox service API URL and API key configured in the target account/organization Preferences page, or an enabled admin fallback at `/admin/sandbox_service`.
- At least one Qiniu sandbox template that contains `/opt/actions-runner/config.sh` and `/opt/actions-runner/run.sh`.
- An admin account bootstrapped with `runnerd --bootstrap-admin github:<github-user-id>`.

Do not use real secrets in this document or commit deployment-local files such as `runnerd.local.yaml`, `.smee-url`, sqlite databases, private keys, or cookie jars.

## 1. Service Health

Verify the service is reachable:

```bash
curl -fsS https://<runnerd-host>/healthz
```

Expected result: HTTP 200 with `status: ok`.

Log in through the admin console:

```text
https://<runnerd-host>/admin/
```

Expected result: GitHub OAuth completes and the signed session has `role: admin`.

Open the Accounts page with at least one secondary account:

```text
https://<runnerd-host>/admin/accounts
```

Check:

- Summary totals stay global while search, role filters, page size, and pagination change the account list.
- Linked GitHub identities load the avatar derived from their login and fall back to account initials if it is unavailable.
- The current administrator's role control is disabled.
- A `role: user` session is rejected by both the accounts list and role-update APIs, and an administrator's direct attempt to patch their own role returns a conflict.
- Changing a secondary account from `user` to `admin` takes effect immediately and creates an `account.role.update` audit event.
- With exactly two administrators and two signed-in sessions, concurrent cross-demotion attempts cannot both succeed; at least one administrator remains.
- After all role checks, use the surviving administrator to restore the original administrator if needed, then use the intended administrator to restore the secondary account's role.

## 2. Diagnostics

Open the diagnostics page in the admin console, or call:

```bash
curl -fsS -b "$COOKIE_JAR" https://<runnerd-host>/diagnostics/pprof | jq
curl -fsS -b "$COOKIE_JAR" https://<runnerd-host>/diagnostics/vars | jq
```

Check:

- `github.auth_mode` is `app` for the recommended deployment path.
- `state.database` points at the intended sqlite, Postgres, or MySQL database.
- pprof discovery files and dump scripts are visible when the local pprof service is available.
- Recent failure summaries are empty or understood.

## 3. Runner Catalog

Before creating runner specs, verify Sandbox credential precedence:

- An account with no scoped credentials can list templates only while the admin default is enabled and complete.
- In `all` mode, both a personal repository owner and an organization owner can use the complete default.
- In `selected` mode, an owner on the stable-ID audience list can use the default, while an unselected owner and an empty audience cannot.
- Add a GitHub login that has never signed in or synchronized, and verify the admin response shows GitHub's canonical login, stable ID, and account type.
- With GitHub App auth enabled, verify the first selected-owner workflow resolves and caches an otherwise unknown installation owner; a later request should not require another owner lookup.
- Saving scoped account or organization credentials changes the effective source away from `admin_default`.
- Removing an audience entry blocks new fallback resolution without changing an already-snapshotted runner request.
- Disabling the admin default makes an otherwise unconfigured account fail with `sandbox service not configured`.

Create or confirm a runner spec:

```bash
curl -fsS -X POST https://<runnerd-host>/runner_specs \
  -b "$COOKIE_JAR" \
  -H 'content-type: application/json' \
  -d '{"name":"ubuntu-24-04","labels":["self-hosted","e2b"],"template_id":"<template-id>","max_concurrency":1,"enabled":true,"default_available":true}' | jq
```

If the spec should be restricted, set `default_available: false` and create a runner policy or runner group for the target repository.

Run a match test:

```bash
curl -fsS -X POST https://<runnerd-host>/runner_specs/match \
  -b "$COOKIE_JAR" \
  -H 'content-type: application/json' \
  -d '{"repository_full_name":"<owner>/<repo>","labels":["self-hosted","e2b"]}' | jq
```

Expected result: the response includes the intended runner spec.

## 4. Webhook Delivery

In the GitHub App or target repository webhook settings, send a recent delivery or trigger a new workflow.

Expected result:

- The delivery uses `application/json`.
- The delivery includes a valid `X-Hub-Signature-256`.
- The runnerd response is a 2xx JSON response for supported `workflow_job` actions.
- Unsupported events are ignored intentionally, not treated as runner failures.

## 5. Workflow Pickup

Use a minimal workflow:

```yaml
name: runnerd-smoke

on:
  workflow_dispatch:

jobs:
  smoke:
    runs-on: [self-hosted, e2b]
    steps:
      - run: |
          uname -a
          whoami
          pwd
```

Trigger it manually.

Expected result:

- A runner request appears as `queued`, then `creating`, then `running`.
- The GitHub Actions job leaves the queued state and runs on an `e2b-*` runner.
- The job's `Set up runner` log includes the Qiniu sandbox id, runner request id, and runner name.
- After the job finishes, the runner request becomes `completed`.

## 6. Cleanup

After the workflow completes, verify:

- The Qiniu sandbox has stopped or is no longer active.
- The GitHub self-hosted runner registration has been removed or is offline and cleaned up by runnerd.
- The runner request has control/stdout/stderr logs available from the admin UI or `/runner_requests/{id}/logs/{name}`.
- `/diagnostics/vars` shows updated workflow job, runner registration, cleanup, and duration counters.

## 7. Failure Drill

Run one controlled failure while the deployment is still under observation:

- Use labels that do not match any runner spec, or
- temporarily disable the matched runner spec, or
- lower the spec concurrency and trigger two jobs.

Expected result depends on the scenario:

- unmatched labels or disabled specs are recorded as admission failures;
- concurrency pressure leaves later requests queued rather than dropped;
- retryable placement or rate-limit failures populate `next_retry_at` and remain eligible for later processing.

Record any deployment-specific notes outside the repository if they include private hosts, account names, channel URLs, secrets, or cookie data.
