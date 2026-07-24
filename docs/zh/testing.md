# 本地测试与 GitHub 配置

[English](../testing.md)

这份文档说明如何用本地 Qiniu sandbox 环境测试服务，以及如何在 GitHub 仓库中配置 self-hosted runner 自动拉起。

## 1. 本地配置文件

服务现在默认读取 `./runnerd.yaml`；也可以通过 `--config` 指向别的路径：

```bash
cp runnerd.yaml.example runnerd.yaml
mkdir -p ./secrets
```

相对 sqlite `database.dsn` 和 `github.app.private_key_file` 都相对 `runnerd.yaml` 所在目录解析。旧版 `database.url` 在 `database.dsn` 为空时仍作为 deprecated alias 兼容读取。当前只支持 GitHub.com，不支持 GitHub Enterprise Server。GitHub 鉴权可以使用 GitHub App、PAT token 或 basic auth，但只能三选一。

最小可用配置示例：

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

如果需要避免敏感值被直接展示，先构建 runnerd，再将每个原值通过 stdin 传给 `./bin/runnerd --obfuscate-config-value`，然后把生成的 `RUNNERD_ENC(v1:...)` 填入 YAML。明文配置仍保持兼容。支持的字段包括 `database.dsn`/`database.url`、`auth.session_secret`、`auth.encryption_key`、`github.webhook_secret`、`github.token`、`github.basic_auth.password` 和 `github.oauth.client_secret`。运行时包装类型还会在意外的文本格式、结构化日志、JSON 和 YAML 输出中显示掩码。该能力仅用于混淆：解码 key 位于 runnerd 内，能够检查或执行二进制的主机用户仍可恢复原值。

Sandbox service API URL 和 API Key 不在 `runnerd.yaml` 中配置。登录后可在账户或组织的 Preferences 页面配置 scoped credentials，也可由管理员在 `/admin/sandbox_service` 配置默认关闭的平台回退。fallback audience 为 `all` 或 `selected`；selected entries 按仓库 owner 的稳定 GitHub account ID 和 type 匹配。API Key 使用 `auth.encryption_key` 加密保存。解析顺序为 runner request 已保存快照、installation custom/inherited 配置、符合条件的个人账户配置、已启用且 audience eligible 的 admin default，最后才是未配置错误。

Runner spec、runner group 和 repository policy 不在 `runnerd.yaml` 中配置；服务启动后通过后台页面或 admin API 创建。spec 名称建议使用有意义的名字，例如 `ubuntu-24-04`，`template_id` 填对应的 Qiniu sandbox template ID。template 是否可访问会在 runnerd 使用对应账户或组织的 Sandbox service 配置启动 sandbox 时确认。

`database.backend` 支持 `sqlite`、`postgres` 和 `mysql`。本地开发优先使用 sqlite；共享数据库的多实例部署需要先用两个 runnerd 进程验证 lease 行为，再作为正式运行方式记录。

状态表结构主要由 `internal/state/records.go` 里的 GORM tag 定义。服务启动时，已有 SQLite `runner_requests` 表只通过创建缺失的 model columns 和 indexes 做 additive migration；它会跳过通用 SQLite `AutoMigrate` 表重建，避免历史上通过 ALTER 添加的字段丢失。未来如需对 `runner_requests` 做 non-additive 变更，必须增加窄范围显式 migration 和数据保全回归 fixture。其他表会先针对旧 columns、obsolete OAuth constraints 和不兼容的 legacy scope tables 执行窄范围 compatibility pass，再运行 GORM `AutoMigrate`。缺少 `scope_type`/`scope_id` 的旧 `account_preferences` 和 `account_secrets` 表会被删除并重建，而不是迁移原数据。升级后必须重新配置其中保存的 Sandbox Preferences 和 API keys；已保存的 GitHub OAuth tokens 也会被清除，相关用户需重新使用 GitHub 登录后才能同步 installations。修改 state record、索引或迁移 helper 时，至少先跑：

```bash
go test ./internal/state -count=1
```

不要只用全新 sqlite 文件验证迁移；旧 schema 升级路径也需要覆盖，尤其是新增 `NOT NULL` 列、唯一索引或关系约束时。

验证生产 SQLite snapshot 时，应在 disposable copy 启动前后记录数据完整性计数：

```bash
sqlite3 runnerd-export.db \
  "SELECT COUNT(*), SUM(CASE WHEN github_installation_id > 0 THEN 1 ELSE 0 END), SUM(CASE WHEN sandbox_api_url <> '' THEN 1 ELSE 0 END), SUM(CASE WHEN sandbox_api_key_encrypted <> '' THEN 1 ELSE 0 END), SUM(CASE WHEN sandbox_config_source <> '' THEN 1 ELSE 0 END) FROM runner_requests;"
```

迁移需要连续运行两次。两次启动后的总行数和各字段非空计数都必须保持稳定；唯一允许增加的是可从 `github_payload_json.installation.id` 恢复的 `github_installation_id`。

仓库提供了一个 opt-in 的 state-only snapshot test。它会先复制源数据库，不会启动 runner recovery：

```bash
RUNNERD_SQLITE_SNAPSHOT=/path/to/runnerd-export.db \
  go test ./internal/state -run TestMigrateSQLiteRunnerRequestSnapshot -count=1 -v
```

## 2. 配置 GitHub 鉴权

推荐使用 GitHub App。PAT token 和 basic auth 也支持，主要用于本地验证或已有凭据场景。

继续前，请先配置[必要的 GitHub App 权限](../../README.zh.md#所需权限)。以下步骤只说明本地设置细节。

建议流程：

1. 进入 GitHub `Settings -> Developer settings -> GitHub Apps -> New GitHub App`。
2. 基础信息：
   - GitHub App name：例如 `runnerd-local`
   - Homepage URL：先填仓库地址或本地项目文档地址
   - Setup URL：填 runnerd 的 `/github-app/setup` 地址，例如 `http://127.0.0.1:25500/github-app/setup`
   - Webhook：如果 runnerd 自己收 webhook，可以先不开 App webhook，这里和 `workflow_job` webhook 不是一回事
3. 在 `Permissions` 中应用[必要权限表](../../README.zh.md#所需权限)中的设置。
4. Where can this GitHub App be installed：
   - 本地验证一般选 `Only on this account`
5. 创建后，在 App 页面生成 private key，下载 `.pem` 文件，保存到本地，例如 `./secrets/github-app.pem`
6. 安装 App 到目标仓库或组织：
   - 点 `Install App`
   - 选择目标 owner
   - 选择要授权的仓库
7. 记录这些值：
   - App ID
   - App slug（App URL 里的短名称，例如 `https://github.com/apps/<slug>`）
   - Installation ID（可选；不配置时 runnerd 会按仓库动态解析）
   - private key 文件路径

对应填入 `runnerd.yaml`：

```yaml
github:
  app:
    id: <app id>
    slug: <app slug>
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

`task dev` 默认读取 `runnerd.local.yaml`，从 `127.0.0.1:5173` 开始选择第一个可用端口启动 Vite dev server，并用 `development` build tag 启动 Go 服务。浏览器仍然访问 runnerd 的地址。

普通用户界面：

```text
http://127.0.0.1:25500/
```

普通用户 Activity repositories 页面：

```text
http://127.0.0.1:25500/repositories
```

普通用户 account repositories 页面：

```text
http://127.0.0.1:25500/account/repositories
```

普通用户 personal Preferences 页面：

```text
http://127.0.0.1:25500/account/preferences
```

普通用户个人 Sandbox 目录：

```text
http://127.0.0.1:25500/account/sandbox-templates
http://127.0.0.1:25500/account/sandbox-instances
```

管理员界面：

```text
http://127.0.0.1:25500/admin/
```

如需换配置文件：

```bash
RUNNERD_CONFIG=./runnerd.yaml task dev
```

如需固定 Vite 端口：

```bash
RUNNERD_VITE_PORT=5173 task dev
```

生产模式或验证嵌入式前端资源时，先重新构建 UI 和 binary，再启动 runnerd：

```bash
task build
./bin/runnerd --config ./runnerd.yaml
```

健康检查：

```bash
curl -fsS http://127.0.0.1:25500/healthz
```

普通用户页面：

```text
http://127.0.0.1:25500/
```

页面会跳转到 GitHub OAuth 登录。首次登录会在数据库中创建 `role=user` 的本地 account，并把 GitHub OAuth identity 绑定到该 account；首个管理员需要在启动服务之前单独执行一次 bootstrap 命令；该命令会设置管理员角色后直接退出，不会启动 runnerd：

```bash
go run ./cmd/runnerd --config ./runnerd.yaml --bootstrap-admin github:<your-github-user-id>
```

`<your-github-user-id>` 是 GitHub `/user` 返回的稳定 numeric `id`，不是可修改的 login。role 属于本地 account，OAuth identity 只保存 provider、stable subject 和 login 展示信息，因此后续可以把其他 provider identity 绑定到同一个 account。普通用户登录后可以在 `/account/repositories` 安装配置文件中定义的 GitHub App，在 `/account/preferences` 或 `/organizations/{login}/preferences` 配置 Sandbox service，并通过 Sandbox Templates 和 Sandbox Instances tabs 查看 scoped resources。GitHub 带 `installation_id` 回调后，runnerd 会记录该 account 绑定的 GitHub App installation。普通用户能看到的 job 按精确的 `(installation_id, repository_full_name)` 过滤；runnerd 使用已保存的 GitHub App user access token，获取用户仓库权限与每个已绑定 App installation 仓库范围的交集，并统一保护列表、详情、分组、日志和终端操作。runnerd 不会把完整的仓库授权列表复制到本地状态；成功结果在内存中的硬过期时间仍为 30 秒。缓存满 20 秒后的首次请求会立即使用仍有效的交集，并只触发一次后台刷新。共享刷新使用 server 级 timeout，不受首个调用者取消影响；installation 或 OAuth 变更会推进 account cache epoch，旧 epoch 的刷新不能回填缓存，也不能服务后续请求。GitHub 拒绝 token 时会立即清除缓存；瞬时刷新错误可以重试，但不会把授权延长到原 30 秒期限之后。account repositories 页面按需加载相同的交集。用户 token 缺失或被 GitHub 拒绝时会默认拒绝访问，并要求用户重新使用 GitHub 登录。GitHub 返回无权访问的已绑定 installation 时，runnerd 会跳过该 installation，既不会暴露其 job，也不会阻止其他可访问 installation 正常加载。目录接口要求普通用户 session，使用 account 或选中 installation 的加密凭据，把支持的 region id 映射到服务端维护的 endpoint，且不会暴露凭据。管理员登录后，浏览器会保存 signed HttpOnly session cookie，并自动带上该 cookie 访问 `/runner_requests` 等管理接口。需要用 `curl` 调管理 API 时，可以从浏览器或 OAuth 调试流程导出 cookie 到 `COOKIE_JAR`，后续示例统一使用：

```bash
export COOKIE_JAR=./runnerd.cookies
```

管理员账户页面：

```text
http://127.0.0.1:25500/admin/accounts
```

顶部统计卡展示账户总数、管理员、普通用户和已绑定 OAuth identity 数量；这些统计是全局口径，不受搜索、角色筛选或分页影响。账户列表可搜索关联 OAuth identity 的 login、provider 和 stable subject；`role` 可筛选 `admin` 或 `user`，`limit` 和 `offset` 用于分页，默认每页 20 条、最多 100 条。关联 GitHub identity 会按 login 加载约定的 GitHub 头像 URL；头像不可用时回退到账户首字母。页面只能把其他 account 的 role 在 `admin` 和 `user` 之间切换；account 仍由 OAuth/bootstrap 创建，页面不能创建或删除 account，也不能绑定或解绑 identity。角色修改会立即生效，并写入 `account.role.update` 审计事件。系统会拒绝修改自身角色，以及可能导致没有管理员的变更，包括相互竞争的并发降级操作。

```bash
curl -fsS -b "$COOKIE_JAR" \
  'http://127.0.0.1:25500/admin/api/accounts?q=octo&role=admin&limit=20&offset=0' | jq
curl -fsS -X PATCH -b "$COOKIE_JAR" -H 'content-type: application/json' \
  http://127.0.0.1:25500/admin/api/accounts/<account-id>/role \
  -d '{"role":"admin"}' | jq
```

管理员通过显式的 role-gated API 管理平台回退。省略 `api_key` 会保留已有密文，省略 `audience_mode` 会保留当前模式，响应永远不会返回 API Key。`selected` 模式没有 audience entries 时不会匹配任何 account。添加 audience 时可提交 `login` 或 `@login`；runnerd 会先向 GitHub 查询 canonical login、stable numeric ID 和 user/organization type，再保存稳定身份。已同步或已缓存的 owner 只作为可选建议，不是添加前提。selected owner 的第一个 workflow 如果没有本地 installation row，runnerd 会通过 GitHub App auth 查询 installation owner 并缓存该稳定身份。

```bash
curl -fsS -b "$COOKIE_JAR" http://127.0.0.1:25500/admin/api/sandbox-service-default | jq
curl -fsS -X PUT -b "$COOKIE_JAR" -H 'content-type: application/json' \
  http://127.0.0.1:25500/admin/api/sandbox-service-default \
  -d '{"enabled":true,"audience_mode":"selected","api_url":"https://us-south-1-sandbox.qiniuapi.com","api_key":"<sandbox-api-key>"}' | jq
curl -fsS -X POST -b "$COOKIE_JAR" -H 'content-type: application/json' \
  http://127.0.0.1:25500/admin/api/sandbox-service-default/audiences \
  -d '{"account_login":"octo-org"}' | jq
curl -fsS -X DELETE -b "$COOKIE_JAR" \
  http://127.0.0.1:25500/admin/api/sandbox-service-default/audiences/<audience-id> | jq
curl -fsS -X DELETE -b "$COOKIE_JAR" \
  http://127.0.0.1:25500/admin/api/sandbox-service-default/api-key | jq
```

页面源码在 `ui/`，使用和 `kubevirt-console` 相同的 React、Vite、Tailwind CSS、shadcn 风格组件和主题 CSS。`task build` 会先执行 `task ui-build`，把前端产物写入 `internal/server/ui/` 后再编译 `runnerd`。开发模式下 `internal/server/ui_assets_development.go` 会把 UI 资源代理到 Vite；生产构建下 `internal/server/ui_assets_production.go` 会嵌入 `internal/server/ui/*`。普通用户界面包含 `/account/repositories` 的 GitHub App accounts 和按需加载的授权 repositories、`/account/preferences` 和 `/organizations/{login}/preferences` 的 Sandbox service 设置、`/account/sandbox-templates` 的区域过滤模板、`/account/sandbox-instances` 的区域和模板过滤 runner instances、对应的 organization 路由、`/repositories` 的本地 activity repositories、`/` 的 Repo/PR 列表、`/github/pulls/{owner}/{repo}/{number}/jobs` 这类稳定的 GitHub-context job-group 路由，以及 `/jobs/{id}` 的 job 详情。首次进入页面时只加载当前路由实际使用的资源。Jobs 首页加载第一页 `GET /user/runner_requests?limit=100&offset=0` 并每 5 秒轮询该页，同时保留已经加载的历史；稳定 job-group 路由和 Load older jobs 操作可以加载受限的 500 行历史窗口。API 会拒绝 `limit + offset` 超过 500 的请求，也不会返回不可用的 next link。GitHub App metadata 和 Preferences 不进入轮询。Admin 路由只加载当前 section 所需的 request/spec/group/policy/audit 依赖，且只有 Overview 和 Runner Requests 会轮询 runner requests。目录使用 `GET /user/sandbox/templates?region=<id>` 和 `GET /user/sandbox/instances?region=<id>&template_id=<id>`；实例接口只列出 runner 创建的 sandboxes，并使用统一的 scoped/default credential resolver。管理面包含 `/admin/accounts` 的账户列表与角色控制、`/admin/sandbox_service` 的平台回退、runners、runner specs、runner groups、runner policies、retry、audit、label match test 和 diagnostics 页面，不包含 provider resource catalogs。

只运行 UI unit tests 时使用：

```bash
cd ui && bun run test
```

`task test` 会重新构建 UI、运行同一套 Bun tests，然后执行带 race detection 和 coverage 的 Go tests。Bun suite 覆盖 helper 和 server-rendered component output；导航、dialog、头像加载/回退，以及角色变更后的权限切换仍需在真实浏览器中验证。

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
- `runner start deferred because global concurrency is at capacity` 或 `runner start deferred because profile is at capacity`：request 会保持 queued，直到全局或 per-spec 容量可用。
- GitHub job 一直 queued：workflow 的 `runs-on` labels 必须包含 `self-hosted` 和 `e2b`，并与 runner spec 的 labels 保持一致。
- sandbox 创建失败：确认账户/组织 Preferences 或已启用的 admin default 具有与 template 和本地环境匹配的完整 Sandbox service 配置；Runner detail 会显示实际选择的来源。
- registration token 失败：检查 [GitHub App 权限表](../../README.zh.md#所需权限)。未配置 `runner_group` 的 spec 需要 repository `Administration`；配置了 `runner_group` 的 spec 需要 organization `Self-hosted runners`。

## 9. GitHub Actions 日志怎么看

runnerd 默认创建 repository 级 self-hosted GitHub Actions runner；如果 spec 配置了 `runner_group`，则会为 repository owner 创建 organization runner。job 被 sandbox 里的 runner 接走后，workflow step 的日志会正常显示在 GitHub Actions 页面里：

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

服务自身日志会输出到 runnerd 的 stdout/stderr，由启动终端或 service manager 收集。

## 10. Diagnostics / pprof

服务导入了 `github.com/jimmicro/pprof`，启动后会在 binary 所在目录生成 `.pprof` 地址文件和 dump 脚本。管理 API 可以直接查看 diagnostics：

```bash
curl -fsS -b "$COOKIE_JAR" http://127.0.0.1:25500/diagnostics/pprof | jq
curl -fsS -b "$COOKIE_JAR" http://127.0.0.1:25500/diagnostics/vars | jq
```

`/diagnostics/pprof` 会返回：

- 发现到的 pprof 地址文件
- dump 脚本路径
- database backend 和经过脱敏的 DSN/path
- GitHub 鉴权模式（app、token 或 basic）
- 最近失败的 runner request

`/diagnostics/vars` 会代理本地 pprof 服务的 `GET /debug/vars`，可以直接看到 expvar 指标摘要。当前指标覆盖 profile current/busy/idle/pending/desired、retry/lease、create/stop 次数与耗时、GitHub API 调用、runner 注册/清理，以及 workflow job queued/started/completed、conclusion、failure、queue duration 和 run duration。

## 11. 官方参考

- GitHub self-hosted runner workflow labels: https://docs.github.com/en/actions/hosting-your-own-runners/managing-self-hosted-runners/using-self-hosted-runners-in-a-workflow
- GitHub self-hosted runner autoscaling: https://docs.github.com/en/actions/hosting-your-own-runners/autoscaling-with-self-hosted-runners
- GitHub webhook `workflow_job` event: https://docs.github.com/en/webhooks/webhook-events-and-payloads
