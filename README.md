# E2B GitHub Runner

Small Go service that starts ephemeral GitHub Actions self-hosted runners inside E2B sandbox instances.

## Configuration

Runtime configuration is file-first. `runnerd` reads `./runnerd.yaml` by default, or the path passed with `--config`.

Start from the example:

```bash
cp runnerd.yaml.example runnerd.yaml
```

The config file covers:

- server listen address and timeouts
- database backend and DSN/path
- E2B API settings and template
- GitHub webhook settings plus GitHub App, PAT, or basic auth
- GitHub App OAuth login for the admin console
- worker lease / retry / concurrency settings

Relative sqlite `database.url` and `github.app.private_key_file` paths are resolved from the directory containing `runnerd.yaml`.
GitHub Enterprise Server is not currently supported; configure a GitHub.com App installation.
Configure exactly one GitHub auth method: `github.app`, `github.token`, or `github.basic_auth`. For GitHub App auth, `github.app.installation_id` is optional. When it is omitted, runnerd resolves the installation from each job repository and caches installation transports, allowing one GitHub App to serve multiple installed accounts.
`github.allowed_repositories` is an optional allowlist of `owner/repo` or `owner/*` patterns. Empty means all repositories that can deliver valid webhooks and match runner labels/policies are allowed.

`github.oauth` enables GitHub App OAuth login for the embedded admin console. Use the GitHub App's Client ID and Client secret, set a separate `auth.session_secret`, and configure the app callback URL as `/auth/github/callback` on your runnerd origin. Users and roles are maintained in the database by OAuth provider and stable subject; for GitHub this is the numeric user ID, while login is stored as display metadata. The first OAuth callback creates a user with `role: user` when none exists, and only users with `role: admin` can access the admin console and management API. Bootstrap the first admin with `runnerd --bootstrap-admin github:<github-user-id>`. OAuth sessions are stored as signed HttpOnly cookies.

`/webhooks/github` uses GitHub HMAC signature verification. The manual management API under `/runner_requests` requires a valid GitHub OAuth admin session cookie.

Runner state is persisted in a DB-backed store instead of per-request JSON directories. Control/stdout/stderr logs are kept as runner events and remain available from the admin API and UI.

## Run

```bash
go run ./cmd/runnerd --config ./runnerd.yaml
```

## Docker

The container image is file-config only. Mount `runnerd.yaml` and any referenced secret files into the container; environment variables such as `HTTP_ADDR` are not used for runtime config.

```bash
docker run --rm -p 25500:25500 \
  -v "$PWD/runnerd.yaml:/etc/runnerd/runnerd.yaml:ro" \
  -v "$PWD/secrets:/etc/runnerd/secrets:ro" \
  ghcr.io/qiniu/ci-runner
```

Open the embedded admin console at `http://127.0.0.1:25500/admin/`. The UI is built from `ui/` with the same React, Vite, Tailwind CSS, shadcn-style components, and theme tokens used by `kubevirt-console`. The console offers GitHub sign-in and uses a signed HttpOnly cookie for management API calls.

The admin console manages runner requests, runner specs, runner groups, runner policies, retry actions, audit history, runner-spec match tests, and diagnostics. Runner specs, groups, and repository policies are created through the admin API/UI rather than `runnerd.yaml`. runnerd creates repository runners by default; when a matched runner spec has a GitHub runner group, it creates an organization runner for the job repository owner and passes that group as `--runnergroup`.

Create runner specs with meaningful names such as `ubuntu-24-04` or `ubuntu-24-04-large`; set each spec `template_id` to the E2B template that contains the GitHub runner image. The admin API validates that the template exists and has a usable build before saving a runner spec. Runner specs with `default_available: true` are globally available to allowed installed repositories. Use `github.allowed_repositories` to limit which repositories can use this runnerd instance, and use runner policies when a repository needs access to an additional/special spec.

Runner requests are paginated by default: `GET /runner_requests` returns the most recent 100 rows unless `limit` and `offset` are provided, with `X-Total-Count`, `X-Limit`, `X-Offset`, and `Link` response headers. The admin console adds status, repository, and runner-spec filters on top of the current page and links each managed request to the GitHub Actions job when GitHub provides a job URL.

runnerd enforces both `worker.max_concurrent_runners` and per-spec `max_concurrency`. Requests above those limits remain in the DB as `queued` and are retried later; they are not dropped. Transient capacity signals such as E2B placement failures, HTTP 429, and GitHub secondary rate limits are treated as queue deferrals, so they keep waiting even after the normal retry counter reaches its configured cap. Other transient failures still use the configured retry backoff and eventually become `failed`; deterministic auth/config/template errors fail immediately.

runnerd caches valid GitHub registration tokens per repository or organization, retries runner registration inside the sandbox, and best-effort removes the GitHub runner registration when a sandbox is stopped or recovered.

The sandbox runner installs a pre-job hook that prints the E2B sandbox id, runner request id, and runner name in the GitHub Actions `Set up runner` log. Use that sandbox id to find the matching instance in the E2B console when debugging a job.

The binary also imports `github.com/jimmicro/pprof`, so a local-only pprof/expvar service is started automatically and discovered through generated `.pprof` address files and dump scripts. The admin console exposes a diagnostics page that summarizes the discovered pprof endpoint, `/debug/vars`, DB state, GitHub auth mode, sandbox API configuration, retry/lease metrics, and recent failures. The expvar metrics include ARC-style workflow job counts, conclusions, failures, queue/run duration totals and counts, runner registration/cleanup counters, GitHub API operation counters, and Fireactions-style profile current/busy/idle/pending/desired gauges.

![Admin console](docs/images/admin-console.png)

## Build

```bash
task build
task docker-build
task template-build-prod
```

`templates/github-runner-ubuntu-24.04` is the default GitHub runner image and includes the runner runtime, Docker support, helper tools, and `rclone`. `templates/qbox-kodo-ubuntu-16.04` is an additional legacy Ubuntu 16.04 template for qbox/kodo-style jobs with the required old Go toolchains, apt packages, Docker support, and `rclone`. Its Docker base image is defined by `templates/qbox-kodo-ubuntu-16.04/base.Dockerfile` and can be rebuilt with `task qbox-kodo-base-build` before rebuilding the E2B template.

Useful validation commands:

```bash
task lint
task test
task docker-check
task release-check
```

Use `runs-on: [self-hosted, e2b]` in the target workflow. Configure a GitHub webhook for `workflow_job` events pointing at `POST /webhooks/github`; runnerd handles `queued`, `in_progress`, and `completed` actions. You can also include `workflow_run` events as a compensating signal; runnerd lists all queued jobs in the run and enqueues any matching jobs not already seen from `workflow_job`.
