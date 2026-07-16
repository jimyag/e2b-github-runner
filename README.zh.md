# Qiniu Sandbox GitHub Runner

[English](README.md)

这是一个小型 Go 服务，用于在 Qiniu sandbox 实例中启动临时 GitHub Actions self-hosted runner。

## 配置

运行时配置以文件为主。`runnerd` 默认读取 `./runnerd.yaml`，也可以通过 `--config` 指定其他路径。

从示例配置开始：

```bash
cp runnerd.yaml.example runnerd.yaml
```

配置文件覆盖：

- 服务监听地址和超时；
- sqlite、Postgres 或 MySQL 数据库后端和 DSN/path；
- sandbox 生命周期超时；
- GitHub webhook 设置，以及 GitHub App、PAT 或 basic auth；
- admin console 的 GitHub App OAuth 登录；
- worker lease、retry、concurrency 设置。

相对路径的 sqlite `database.dsn` 和 `github.app.private_key_file` 会按 `runnerd.yaml` 所在目录解析。旧的 `database.url` 在 `database.dsn` 为空时仍作为 deprecated alias 兼容读取。
本地和小型单节点部署建议使用 sqlite。状态存储支持 Postgres 和 MySQL，但不要在两个 runnerd 进程共用同一个数据库验证前，把 shared-database multi-instance 作为已支持能力对外说明。
当前不支持 GitHub Enterprise Server；请配置 GitHub.com App installation。
GitHub 鉴权方式必须三选一：`github.app`、`github.token` 或 `github.basic_auth`。使用 GitHub App auth 时，`github.app.installation_id` 是可选的；省略时，runnerd 会按每个 job repository 动态解析 installation，并缓存 installation transports，因此一个 GitHub App 可以服务多个已安装账号。

### GitHub App 权限

使用 GitHub App 鉴权时，只需配置下表中的权限。Runner 管理权限取决于匹配到的 Runner Spec 是否设置了 `runner_group`。

| 范围 | 权限 | 访问级别 | 必要用途 |
| --- | --- | --- | --- |
| Repository | Actions | Read-only | 查询 workflow job 和 run 状态、列出 queued jobs，并读取 job 或 run 日志；通过 GitHub App webhook 接收 `workflow_job` 和 `workflow_run` 事件时，也需要此权限。 |
| Repository | Administration | Read and write | 匹配到的 Runner Spec **未配置 `runner_group`** 时，用于生成仓库级 runner 注册令牌、查询已注册 runner，并在任务结束后清理 runner。若不配置此权限，仓库级 runner 将注册失败，只能使用配置了 `runner_group` 的组织级 runner，且不再支持个人账户仓库。 |
| Repository | Metadata | Read-only | 识别仓库及其所属账户，并获取 GitHub App installation 已授权的仓库列表。 |
| Repository | Pull requests | Read-only | 获取普通用户 PR job group 的 pull request 标题，包括私有仓库。若不配置此权限，job group 仍可使用，但标题会显示为不可用。 |
| Organization | Self-hosted runners | Read and write | 匹配到的 Runner Spec **配置了 `runner_group`** 时，用于生成组织级 runner 注册令牌、将 runner 注册到指定 group，并在任务结束后查询和清理 runner。 |

如果希望普通用户 UI 显示 Install GitHub App 链接，请设置 `github.app.slug` 为 GitHub App URL slug。
`github.allowed_repositories` 是可选 allowlist，支持 `owner/repo` 或 `owner/*`。为空表示允许所有能发送有效 webhook 且能匹配 runner labels/policies 的仓库。

`github.oauth` 用于启用内置 console 的 GitHub App OAuth 登录。使用 GitHub App 的 Client ID 和 Client secret，设置单独的 `auth.session_secret` 用于签名 session，设置单独的 `auth.encryption_key` 用于加密用户 secret，并把 App callback URL 配成 runnerd origin 下的 `/auth/github/callback`。本地 account 保存 role，OAuth identity 按 provider 和 stable subject 匹配；GitHub 的 stable subject 是 numeric user ID，login 只作为展示元数据保存。首次 OAuth callback 会创建 `role: user` 的 account 并绑定 GitHub identity。普通用户安装配置的 GitHub App；runnerd 记录返回的 installation id，并使用 workflow job installation id 判断用户可以看到哪些 job。runnerd 不会把 GitHub App repository authorization scope 的完整列表复制到本地状态。只有 `role: admin` 的 account 可以访问管理 API。首个管理员可通过 `runnerd --bootstrap-admin github:<github-user-id>` 引导。OAuth session 存储为 signed HttpOnly cookie。

Sandbox service API URL 和 API key 在普通用户的 account 或 organization Preferences 页面配置。API key 会使用 `auth.encryption_key` 加密；runnerd 不再从 `runnerd.yaml` 读取 Sandbox service 凭据。

登录用户可以在同一 account 或 organization scope 下查看 Sandbox 资源。Sandbox Templates 按支持的区域列出模板，Sandbox Instances 按区域和可选模板筛选 runner 创建的实例。页面位于 `/account/sandbox-templates`、`/account/sandbox-instances` 及对应的 `/organizations/{login}/...` 路由；凭据只在服务端使用，目录 API 不会返回凭据。

`/webhooks/github` 使用 GitHub HMAC signature verification。`/runner_requests` 下的手动管理 API 需要有效的 GitHub OAuth admin session cookie。

Runner state 持久化到 DB-backed store，不再使用按请求拆分的 JSON 目录。Control/stdout/stderr logs 会作为 runner events 保存，并继续通过 admin API 和 UI 查看。
Schema 创建由 GORM model 驱动，启动时会先对旧状态数据库执行窄范围兼容补列，再运行 `AutoMigrate`。修改 state records 时需要保持旧 schema upgrade tests 通过。

## 运行

```bash
go run ./cmd/runnerd --config ./runnerd.yaml
```

## 开发

首次安装本地工具和 UI 依赖：

```bash
task deps
task ui-deps
```

开发时使用本地配置文件，避免把 secret 和本地 sqlite 状态提交进仓库：

```bash
cp runnerd.yaml.example runnerd.local.yaml
task dev
```

`task dev` 会从 `5173` 开始选择第一个可用 localhost 端口启动 Vite UI dev server；当 `.smee-url` 存在时启动 smee webhook forwarder；并用 `development` build tag 运行 `runnerd`。Go server 仍监听 `runnerd.local.yaml` 中的地址，通常是 `:25500`，并把嵌入式 UI 资源代理到 Vite。开发时可打开 `http://127.0.0.1:25500/` 查看普通用户 PR/job 视图，打开 `http://127.0.0.1:25500/repositories` 查看普通用户 activity repositories，打开 `http://127.0.0.1:25500/account/repositories` 查看 GitHub App accounts 和 authorized repositories，打开 `http://127.0.0.1:25500/account/preferences` 配置个人 Sandbox service，打开 `http://127.0.0.1:25500/account/sandbox-templates` 或 `/account/sandbox-instances` 查看个人 Sandbox 目录，或打开 `http://127.0.0.1:25500/admin/` 查看 admin console。

可设置 `RUNNERD_CONFIG` 使用其他配置文件，或设置 `RUNNERD_VITE_PORT` 要求固定 Vite 端口。

本地 GitHub webhook forwarding 可创建 per-developer smee channel 文件：

```bash
echo 'https://smee.io/<your-channel>' > .smee-url
```

`task dev` 会自动启动 forwarder。`task smee` 也可单独启动 webhook forwarding，默认转发到 `http://127.0.0.1:25500/webhooks/github`。如果 runnerd 监听其他地址，请设置 `SMEE_TARGET`。

## Docker

容器镜像只使用文件配置。把 `runnerd.yaml` 和其中引用的 secret 文件挂载进容器；`HTTP_ADDR` 这类环境变量不会作为运行时配置使用。

```bash
docker run --rm -p 25500:25500 \
  -v "$PWD/runnerd.yaml:/etc/runnerd/runnerd.yaml:ro" \
  -v "$PWD/secrets:/etc/runnerd/secrets:ro" \
  ghcr.io/qiniu/ci-runner
```

普通用户 console 地址是 `http://127.0.0.1:25500/`，admin console 地址是 `http://127.0.0.1:25500/admin/`。UI 从 `ui/` 构建，使用 React、Vite、Tailwind CSS、shadcn-style components，以及与 `kubevirt-console` 相同的 theme tokens。Console 提供 GitHub sign-in，并使用 signed HttpOnly cookie 调用 API。普通用户可以在 `/` 看到两列 repository/PR job 视图，在 `/repositories` 看到本地 activity repositories，在 `/account/repositories` 看到 GitHub App accounts 和按需加载的 authorized repositories，在 `/account/preferences` 或 `/organizations/{login}/preferences` 配置 Sandbox service，并在 `/account/sandbox-templates`、`/account/sandbox-instances` 及对应的 organization 路由查看 scoped resource catalogs。Admin 用户看到管理 console；provider resource catalogs 不属于 admin 配置。

Admin console 管理 runner requests、runner specs、runner groups、runner policies、retry actions、audit history、runner-spec match tests 和 diagnostics。Runner specs、groups 和 repository policies 通过 admin API/UI 创建，不是 `runnerd.yaml` 字段。runnerd 默认创建 repository runner；当匹配到的 runner spec 配置了 GitHub runner group 时，它会为 job repository owner 创建 organization runner，并把该 group 作为 `--runnergroup` 传入。

创建 runner spec 时使用有意义的名称，例如 `ubuntu-24-04` 或 `ubuntu-24-04-large`；每个 spec 的 `template_id` 应指向包含 GitHub runner image 的 Qiniu sandbox template。runnerd 启动 sandbox 时会使用 repository owner 的 Sandbox service Preferences 检查 template 访问权限。`default_available: true` 的 runner spec 对所有允许的已安装仓库默认可用。使用 `github.allowed_repositories` 限制哪些仓库可以使用该 runnerd 实例；当某个仓库需要额外或特殊 spec 时，使用 runner policies 授权。

Runner requests 默认分页：`GET /runner_requests` 在未提供 `limit` 和 `offset` 时返回最近 100 行，并返回 `X-Total-Count`、`X-Limit`、`X-Offset` 和 `Link` headers。Admin console 会在当前页上叠加 status、repository 和 runner-spec filters，并在 GitHub 提供 job URL 时把每个 managed request 链接到 GitHub Actions job。

runnerd 同时执行 `worker.max_concurrent_runners` 和 per-spec `max_concurrency`。超过这些限制的请求会以 `queued` 状态留在数据库中，并在之后重试；不会被丢弃。Qiniu sandbox placement failures、HTTP 429、GitHub secondary rate limits 等瞬时 capacity 信号会作为 queue deferrals 处理，因此即使普通 retry counter 达到配置上限，仍会继续等待。其他 transient failures 使用配置的 retry backoff，最终可能变成 `failed`；确定性的 auth/config/template 错误会立即失败。

runnerd 会按 repository 或 organization 缓存有效的 GitHub registration token，在 sandbox 内重试 runner registration，并在 sandbox stopped 或 recovery 时 best-effort 移除 GitHub runner registration。

sandbox runner 会安装 pre-job hook，在 GitHub Actions `Set up runner` log 中打印 Qiniu sandbox id、runner request id 和 runner name。调试 job 时可用该 sandbox id 在 Qiniu sandbox console 中查找对应实例。

二进制还会导入 `github.com/jimmicro/pprof`，因此会自动启动 local-only pprof/expvar service，并通过生成的 `.pprof` address files 和 dump scripts 发现。Admin console 提供 diagnostics 页面，汇总发现的 pprof endpoint、`/debug/vars`、DB state、GitHub auth mode、retry/lease metrics 和 recent failures。expvar metrics 包括 ARC-style workflow job counts、conclusions、failures、queue/run duration totals and counts、runner registration/cleanup counters、GitHub API operation counters，以及 Fireactions-style profile current/busy/idle/pending/desired gauges。

![Admin console](docs/images/admin-console.png)

## 构建

```bash
task build
task docker-build
task template-build-prod
```

`templates/github-runner-ubuntu-24.04` 是默认 GitHub runner image，包含 runner runtime、Docker support、helper tools 和 `rclone`。Qiniu sandbox template 构建使用 Taskfile target 调用 `qshell sandbox template build`，并使用各模板目录下 `qshell.sandbox.toml` 的临时副本，避免 qshell 生成的 `template_id` 写回已跟踪配置。`templates/qbox-kodo-ubuntu-16.04` 是额外的 legacy Ubuntu 16.04 template，用于仍需要旧 Go toolchains、apt packages、Docker support 和 `rclone` 的 qbox/kodo-style jobs。它的 Docker base image 定义在 `templates/qbox-kodo-ubuntu-16.04/base.Dockerfile`，可先运行 `task qbox-kodo-base-build` 重建，再重建 Qiniu sandbox template。

常用验证命令：

```bash
task lint
task test
task docker-check
task release-check
```

目标 workflow 使用 `runs-on: [self-hosted, e2b]`。配置 GitHub webhook，把 `workflow_job` events 发送到 `POST /webhooks/github`；runnerd 处理 `queued`、`in_progress` 和 `completed` actions。也可以加入 `workflow_run` events 作为补偿信号；runnerd 会列出该 run 下所有 queued jobs，并为尚未通过 `workflow_job` 入队的 matching jobs 创建 runner request。

生产风格 readiness pass 请使用 [docs/zh/deployment-smoke.md](docs/zh/deployment-smoke.md)。
