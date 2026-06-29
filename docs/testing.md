# 本地测试与 GitHub 配置

这份文档说明如何用本地 E2B sandbox 环境测试服务，以及如何在 GitHub 仓库中配置 self-hosted runner 自动拉起。

## 1. 本地配置文件

服务现在默认读取 `./runnerd.yaml`；也可以通过 `--config` 指向别的路径：

```bash
cp runnerd.yaml.example runnerd.yaml
mkdir -p ./secrets
```

相对 sqlite `database.url` 和 `github.app.private_key_file` 都相对 `runnerd.yaml` 所在目录解析。当前只支持 GitHub.com，不支持 GitHub Enterprise Server。GitHub 鉴权可以使用 GitHub App、PAT token 或 basic auth，但只能三选一。

最小可用配置示例：

```yaml
server:
  http_addr: ":25500"

database:
  backend: sqlite
  url: ./var/runnerd.db

auth:
  session_secret: <random session signing secret>
  session_ttl_hours: 12

e2b:
  api_key: <e2b api key>
  api_url: <e2b api url>

github:
  webhook_secret: <random webhook secret>
  app:
    id: <github app id>
    # 可选。不填时会按 webhook 里的 repository 动态解析 installation。
    # installation_id: <installation id>
    private_key_file: ./secrets/github-app.pem
  oauth:
    client_id: <github app client id>
    client_secret: <github app client secret>
    redirect_url: http://127.0.0.1:25500/auth/github/callback
  # 可选。不填表示允许所有已安装 App 且能通过 runner policy/spec 匹配的仓库。
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

Runner spec、runner group 和 repository policy 不在 `runnerd.yaml` 中配置；服务启动后通过后台页面或 admin API 创建。spec 名称建议使用有意义的名字，例如 `ubuntu-24-04`，`template_id` 填对应的 E2B template ID。保存 runner spec 前，admin API 会验证该 template 存在且有 usable build。

`database.backend` 支持 `sqlite` 和 `postgres`。本地开发优先使用 sqlite；共享数据库的多实例部署需要先用两个 runnerd 进程验证 lease 行为，再作为正式运行方式记录。

## 2. 配置 GitHub 鉴权

推荐使用 GitHub App。PAT token 和 basic auth 也支持，主要用于本地验证或已有凭据场景。

GitHub App 需要能调用 runner registration token API。Repository runner 需要目标仓库的 administration 权限；使用 GitHub runner group 时会创建 organization runner，需要组织级 self-hosted runner 管理权限。

建议流程：

1. 进入 GitHub `Settings -> Developer settings -> GitHub Apps -> New GitHub App`。
2. 基础信息：
   - GitHub App name：例如 `runnerd-local`
   - Homepage URL：先填仓库地址或本地项目文档地址
   - Webhook：如果 runnerd 自己收 webhook，可以先不开 App webhook，这里和 `workflow_job` webhook 不是一回事
3. Repository permissions：
   - `Administration` 设为 `Read and write`
4. Organization permissions（如果要跑 org runner）：
   - 打开对应 self-hosted runner 管理权限
5. Where can this GitHub App be installed：
   - 本地验证一般选 `Only on this account`
6. 创建后，在 App 页面生成 private key，下载 `.pem` 文件，保存到本地例如 `./secrets/github-app.pem`
7. 安装 App 到目标仓库或组织：
   - 点 `Install App`
   - 选择目标 owner
   - 选择要授权的仓库
8. 记录这些值：
   - App ID
   - Installation ID（可选；不配置时 runnerd 会按仓库动态解析）
   - private key 文件路径

对应填入 `runnerd.yaml`：

```yaml
github:
  app:
    id: <app id>
    # installation_id: <installation id>
    private_key_file: ./secrets/github-app.pem
```

PAT 示例：

```yaml
github:
  webhook_secret: <random webhook secret>
  token: <github token>
```

Basic auth 示例：

```yaml
github:
  webhook_secret: <random webhook secret>
  basic_auth:
    username: <github username>
    password: <token or password>
```

不需要固定全局 repo/org 模式；webhook 会使用 payload 里的 `repository.full_name`。默认创建 repository runner；如果匹配到的 runner spec 设置了 GitHub `runner_group`，runnerd 会按该仓库 owner 创建 organization runner，并把 `runner_group` 作为 GitHub runner registration 的 `--runnergroup` 传入。`runner_specs.default_available: true` 的规格对所有仓库默认可用；`runner_policies` 只需要用于给某个仓库或仓库通配符追加特殊 spec，例如 `jimyag/*` 或 `jimyag/template-repository`。

## 3. 启动服务

开发 UI 或后端时，优先使用开发模式：

```bash
task deps
task ui-deps
cp runnerd.yaml.example runnerd.local.yaml
task dev
```

`task dev` 默认读取 `runnerd.local.yaml`，启动 Vite dev server 到 `127.0.0.1:5173`，并用 `development` build tag 启动 Go 服务。浏览器仍然访问 runnerd 的地址，例如：

```text
http://127.0.0.1:25500/admin/
```

如需换配置文件：

```bash
RUNNERD_CONFIG=./runnerd.yaml task dev
```

生产模式或验证嵌入式前端资源时，直接启动 Go 服务：

```bash
go run ./cmd/runnerd --config ./runnerd.yaml
```

健康检查：

```bash
curl -fsS http://127.0.0.1:25500/healthz
```

后台管理页面：

```text
http://127.0.0.1:25500/admin/
```

页面会跳转到 GitHub OAuth 登录。首次登录会在数据库中创建 `role=user` 的本地 account，并把 GitHub OAuth identity 绑定到该 account；首个管理员需要在启动时显式 bootstrap：

```bash
go run ./cmd/runnerd --config ./runnerd.yaml --bootstrap-admin github:<your-github-user-id>
```

`<your-github-user-id>` 是 GitHub `/user` 返回的稳定 numeric `id`，不是可修改的 login。role 属于本地 account，OAuth identity 只保存 provider、stable subject 和 login 展示信息，因此后续可以把其他 provider identity 绑定到同一个 account。管理员登录后，浏览器会保存 signed HttpOnly session cookie，并自动带上该 cookie 访问 `/runner_requests` 等管理接口。需要用 `curl` 调管理 API 时，可以从浏览器或 OAuth 调试流程导出 cookie 到 `COOKIE_JAR`，后续示例统一使用：

```bash
export COOKIE_JAR=./runnerd.cookies
```

后台页面源码在 `ui/`，使用和 `kubevirt-console` 相同的 React、Vite、Tailwind CSS、shadcn 风格组件和主题 CSS。`task build` 会先执行 `task ui-build`，把前端产物写入 `internal/server/ui/` 后再编译 `runnerd`。开发模式下 `internal/server/ui_assets_development.go` 会把 UI 资源代理到 Vite；生产构建下 `internal/server/ui_assets_production.go` 会嵌入 `internal/server/ui/*`。管理面现在包含 runners、runner specs、runner groups、runner policies、retry、audit、label match test 和 diagnostics 页面。

先创建一个默认 runner spec：

```bash
curl -fsS -X POST http://127.0.0.1:25500/runner_specs \
  -b "$COOKIE_JAR" \
  -H 'content-type: application/json' \
  -d '{"name":"ubuntu-24-04","labels":["self-hosted","e2b"],"template_id":"<template id>","max_concurrency":100,"enabled":true,"default_available":true}' | jq
```

手动创建一个 runner：

```bash
curl -fsS -X POST http://127.0.0.1:25500/runner_requests \
  -b "$COOKIE_JAR" \
  -H 'content-type: application/json' \
  -d '{"id":"manual-001","repository_full_name":"<owner>/<repo>","runner_spec_name":"ubuntu-24-04"}' | jq
```

查看状态：

```bash
curl -fsS -b "$COOKIE_JAR" http://127.0.0.1:25500/runner_requests | jq
curl -fsS -b "$COOKIE_JAR" http://127.0.0.1:25500/runner_requests/manual-001 | jq
```

停止 runner：

```bash
curl -fsS -X DELETE -b "$COOKIE_JAR" http://127.0.0.1:25500/runner_requests/manual-001 | jq
```

状态库默认会写到：

```text
var/runnerd.db
```

runner 的 control/stdout/stderr 日志存放在 DB-backed event store 里，仍然通过管理 API 读取：

```bash
curl -fsS -b "$COOKIE_JAR" \
  http://127.0.0.1:25500/runner_requests/manual-001/logs/control.log
curl -fsS -b "$COOKIE_JAR" \
  http://127.0.0.1:25500/runner_requests/manual-001/logs/stdout.log
curl -fsS -b "$COOKIE_JAR" \
  http://127.0.0.1:25500/runner_requests/manual-001/logs/stderr.log
```

## 4. 启动后第一次自检

建议先确认 runnerd 已经正确读到 GitHub App 配置：

```bash
curl -fsS -b "$COOKIE_JAR" http://127.0.0.1:25500/diagnostics/pprof | jq
```

重点看：

- `github.auth_mode` 是否是 `app`
- 如果配置了静态 installation，`github.installation_id` 是否符合预期；动态 installation 模式下这里可以是 `0`
- `state.database` 是否指向你的 `runnerd.yaml` 里配置的数据库

## 5. 暴露 Webhook 地址

GitHub webhook 必须能访问到本地服务。任选一种方式：

使用 smee：

```bash
open https://smee.io/new
echo 'https://smee.io/<your-channel>' > .smee-url
task dev
```

把同一个 smee URL 填到 GitHub webhook 的 Payload URL。`.smee-url` 存在时，`task dev` 会自动启动 smee forwarder。也可以用 `task smee` 单独启动转发。默认转发到 `http://127.0.0.1:25500/webhooks/github`；如果 `runnerd.yaml` 使用了其他监听地址，可以设置 `SMEE_TARGET`：

```bash
SMEE_TARGET=http://127.0.0.1:25501/webhooks/github task smee
```

也可以使用 ngrok：

```bash
ngrok http 25500
```

或 cloudflared：

```bash
cloudflared tunnel create e2b-local-runner
cloudflared tunnel route dns e2b-local-runner runner.example.com
cloudflared tunnel run --url http://127.0.0.1:25500 e2b-local-runner
```

最终 webhook URL 形如：

```text
https://<public-host>/webhooks/github
```

这里的 `runner.example.com` 换成你自己的域名；不要把临时 quick tunnel 的随机 `trycloudflare.com` 地址写死到 GitHub 配置里。

公网部署时只需要把 `/webhooks/github` 暴露给 GitHub。`/runner_requests` 管理接口也可以在同一个服务上访问，但必须携带有效的 OAuth admin session cookie；生产环境建议放在 HTTPS 反向代理后面，并限制管理接口来源 IP。

## 6. 配置 GitHub Repository Webhook

在目标仓库中进入：

```text
Settings -> Webhooks -> Add webhook
```

填写：

- Payload URL：`https://<public-host>/webhooks/github`
- Content type：`application/json`
- Secret：和 `runnerd.yaml` 里的 `github.webhook_secret` 完全一致。
- Which events：选择 `Workflow jobs`。如果希望开启补偿路径，也可以同时选择 `Workflow runs`。
- Active：勾选。

保存后，GitHub 会发送一次 ping。当前服务处理 `workflow_job.queued` / `workflow_job.in_progress` / `workflow_job.completed` 作为主路径，也处理 `workflow_run.requested` / `workflow_run.in_progress` 作为补偿路径；其他事件会返回 ignored，这是正常的。

## 7. 配置 GitHub Actions Workflow

在目标仓库添加：

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

触发 `workflow_dispatch` 后预期流程：

1. GitHub 创建一个 `workflow_job.queued` webhook。
2. 本服务校验签名并在状态库里写入一条 `queued` runner request。
3. 服务创建 sandbox，获取 GitHub registration token，并在 sandbox 内启动 ephemeral runner。
4. GitHub job 被 `self-hosted,e2b` runner 接走执行。
5. runner 进程退出后，服务清理对应 sandbox。

如果同时配置了 `Workflow runs` 事件，`workflow_run.requested` / `workflow_run.in_progress` 只作为补偿信号：runnerd 会查询该 run 下仍处于 `queued` 的 jobs，并为尚未通过 `workflow_job` 入队的 job 补建 runner request。这个补偿动作本身不会让 GitHub Actions UI 立刻显示 job 正在运行；UI 会继续显示 queued / waiting for runner，直到 sandbox 内的 ephemeral runner 注册成功并被 GitHub 分配到该 job 后才会变成 in progress / running。

## 8. 排查顺序

先看服务状态：

```bash
curl -fsS -b "$COOKIE_JAR" http://127.0.0.1:25500/runner_requests | jq
```

再看 request 状态和日志：

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

常见问题：

- `invalid signature`：GitHub webhook secret 和 `github.webhook_secret` 不一致。
- `runner concurrency limit reached`：活跃 request 数量达到 `worker.max_concurrent_runners`。
- GitHub job 一直 queued：workflow 的 `runs-on` labels 必须包含 `self-hosted` 和 `e2b`。
- sandbox 创建失败：确认 `e2b.api_key`、`e2b.api_url` 和 template 配置是否匹配本地环境。
- registration token 失败：确认 GitHub App installation 对目标仓库有对应的 administration/self-hosted runner 权限。

## 9. GitHub Actions 日志怎么看

这是 repository 级 self-hosted GitHub Actions runner。job 被 sandbox 里的 runner 接走后，workflow step 的日志会正常显示在 GitHub Actions 页面里：

```text
Repository -> Actions -> 选择 workflow run -> 选择 job
```

能在 GitHub Actions 里看到：

- workflow step 的 stdout/stderr。
- checkout、build、test 等每个 step 的日志。
- job 成功、失败、取消状态。

不能完整依赖 GitHub Actions 看到：

- sandbox 创建失败日志，因为 runner 还没注册上 GitHub。
- runner 下载、`config.sh` 注册、`run.sh` 启动前的错误。
- webhook 校验失败、GitHub token 申请失败、sandbox API 调用失败。

这些控制面日志看本服务管理 API：

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

服务自身日志会输出到启动 `go run ./cmd/runnerd` 的终端。

## 10. Diagnostics / pprof

服务导入了 `github.com/jimmicro/pprof`，启动后会在 binary 所在目录生成 `.pprof` 地址文件和 dump 脚本。管理 API 可以直接查看 diagnostics：

```bash
curl -fsS -b "$COOKIE_JAR" http://127.0.0.1:25500/diagnostics/pprof | jq
curl -fsS -b "$COOKIE_JAR" http://127.0.0.1:25500/diagnostics/vars | jq
```

`/diagnostics/pprof` 会返回：

- 发现到的 pprof 地址文件
- dump 脚本路径
- 当前 DB 路径
- GitHub 鉴权模式（app、token 或 basic）
- sandbox API 配置摘要
- 最近失败的 runner request

`/diagnostics/vars` 会代理本地 pprof 服务的 `GET /debug/vars`，可以直接看到 expvar 指标摘要。当前指标覆盖 profile current/busy/idle/pending/desired、retry/lease、create/stop 次数与耗时、GitHub API 调用、runner 注册/清理，以及 workflow job queued/started/completed、conclusion、failure、queue duration 和 run duration。

## 11. 官方参考

- GitHub self-hosted runner workflow labels: https://docs.github.com/en/actions/hosting-your-own-runners/managing-self-hosted-runners/using-self-hosted-runners-in-a-workflow
- GitHub self-hosted runner autoscaling: https://docs.github.com/en/actions/hosting-your-own-runners/autoscaling-with-self-hosted-runners
- GitHub webhook `workflow_job` event: https://docs.github.com/en/webhooks/webhook-events-and-payloads
