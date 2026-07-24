# Qiniu Sandbox GitHub Runner

[English](README.md)

一个轻量级 Go 服务，在 [Qiniu Sandbox](https://www.qiniu.com/) 实例中运行临时的 [GitHub Actions self-hosted runner](https://docs.github.com/en/actions/hosting-your-own-runners/about-self-hosted-runners)。每个 workflow job 都会获得一个干净、隔离的沙箱环境，并在完成后自动销毁。

## 目录

- [特性](#特性)
- [工作原理](#工作原理)
- [快速开始](#快速开始)
- [配置](#配置)
  - [配置值混淆](#配置值混淆)
- [GitHub App 设置](#github-app-设置)
  - [所需权限](#所需权限)
  - [OAuth 登录](#oauth-登录)
  - [Webhook 事件订阅](#webhook-事件订阅)
- [Webhook 与 Workflow 配置](#webhook-与-workflow-配置)
- [Runner Spec 与 Policy](#runner-spec-与-policy)
- [管理控制台](#管理控制台)
- [常见问题排查](#常见问题排查)
- [Docker](#docker)
- [构建与开发](#构建与开发)
- [文档](#文档)

## 特性

- **临时 Runner** — 每个 job 一个沙箱，完成后自动清理
- **GitHub App 鉴权** — 推荐的生产鉴权方式，内置 Web 控制台支持 OAuth 登录
- **多数据库支持** — SQLite（默认）、PostgreSQL 或 MySQL 存储运行时状态
- **并发控制** — 全局 `max_concurrent_runners` 和 per-spec `max_concurrency`，基于队列的背压机制
- **内置 Web UI** — 管理控制台（Runner Spec、分组、策略、账户、诊断）；普通用户控制台（Job 分组、日志、沙箱管理）
- **配置混淆** — 敏感值可避免在配置文件中直接暴露明文
- **重试与恢复** — 瞬时故障自动退避重试；容量信号延迟处理而非丢弃请求

## 工作原理

```
GitHub webhook (workflow_job)
        │
        ▼
   ┌─────────┐     创建沙箱             ┌──────────────────┐
   │ runnerd  │ ──────────────────────►  │  Qiniu Sandbox   │
   │ (服务端) │     注册 runner          │  (临时虚拟机)     │
   │          │ ──────────────────────►  │                  │
   └─────────┘                          │  GitHub Actions  │
        │                               │  self-hosted     │
        │  job 完成 / 超时               │  runner          │
        │◄────────────────────────────── │                  │
        │     停止并清理沙箱             └──────────────────┘
        ▼
   状态数据库 (sqlite / postgres / mysql)
```

1. GitHub 向 runnerd 发送 `workflow_job`（queued）webhook。
2. runnerd 将 job labels 与 runner spec 和 policy 进行匹配。
3. runnerd 创建 Qiniu Sandbox 实例，并在其中注册 self-hosted runner。
4. GitHub Actions 将 job 分派到该 runner，job 在沙箱中执行。
5. job 完成（或超时）后，runnerd 移除 runner 注册并停止沙箱。

## 快速开始

```bash
# 1. 构建
task build

# 2. 从示例创建配置
cp runnerd.yaml.example runnerd.yaml
#    编辑 runnerd.yaml：设置数据库、GitHub App 凭据、沙箱参数

# 3. 初始化首个管理员（一次性命令，执行后直接退出，不会启动服务）
./bin/runnerd --bootstrap-admin github:<github-user-id> --config runnerd.yaml

# 4. 启动 runnerd
./bin/runnerd --config runnerd.yaml
```

5. 打开 `http://<host>:25500/`，使用 GitHub OAuth 登录。
6. 在账户/组织 **Preferences** 中配置 **Sandbox Service** 凭据（或在 `/admin/sandbox_service` 配置管理员级兜底）。
7. 在**管理控制台**中创建 **Runner Spec**，设置有意义的 label（如 `ubuntu-24-04`），填写 `template_id`，并启用 `default_available`。
8. 配置 GitHub webhook → `POST http://<host>:25500/webhooks/github`。
9. 在 workflow 中使用 `runs-on: [self-hosted, <your-runner-label>]`。

本地开发请使用 `task dev` 配合 `runnerd.local.yaml`。详细的本地环境搭建（包括 GitHub App 创建和 webhook 转发）请参阅 [docs/zh/testing.md](docs/zh/testing.md)。

## 配置

`runnerd` 默认读取 `./runnerd.yaml`，也可通过 `--config` 指定路径。完整注释的参考配置见 [`runnerd.yaml.example`](runnerd.yaml.example)。

| 配置段     | 说明                                                             |
| ---------- | ---------------------------------------------------------------- |
| `server`   | 监听地址、读/写/空闲超时                                         |
| `database` | 后端类型（`sqlite` / `postgres` / `mysql`）和 DSN                |
| `auth`     | Session secret、加密密钥、Session TTL                            |
| `sandbox`  | 沙箱生命周期超时（创建、运行、停止）                             |
| `github`   | Webhook secret、鉴权方式（App / PAT / basic）、OAuth、允许的仓库 |
| `worker`   | Lease、重试和并发设置                                            |

要点：

- 相对路径的 `database.dsn` 和 `github.app.private_key_file` 按配置文件所在目录解析。
- 本地和单节点部署建议使用 SQLite。支持 PostgreSQL 和 MySQL，但多实例共享数据库尚未验证。
- 已有 SQLite `runner_requests` 表会在启动时补建缺失的 model columns 和 indexes，不会重建整张表。创建列表排序索引不会重写 runner rows，但大数据库可能出现短暂的启动 I/O 和锁等待；迁移与查询计划检查见 [docs/zh/testing.md](docs/zh/testing.md)。
- **不支持** GitHub Enterprise Server，请使用 GitHub.com App。
- GitHub 鉴权方式三选一：`github.app`、`github.token` 或 `github.basic_auth`。
- 省略 `github.app.installation_id` 时，runnerd 按仓库动态解析 installation，一个 App 可服务多个账号。

### 配置值混淆

敏感字段支持 `RUNNERD_ENC(v1:...)` 格式，避免配置文件中出现明文：

```bash
read -r -s secret_value
printf '%s' "$secret_value" | ./bin/runnerd --obfuscate-config-value
unset secret_value
```

支持的字段：`database.dsn`、`auth.session_secret`、`auth.encryption_key`、`github.webhook_secret`、`github.token`、`github.basic_auth.password`、`github.oauth.client_secret`。这些值在日志和序列化输出中也会显示为 `******`。

> **注意：** 此功能仅防止直接查看配置时的明文泄漏，解码 key 内置在二进制中，不能抵御主机级别的攻击者。

## GitHub App 设置

### 所需权限

| 范围         | 权限                | 访问级别     | 用途                                                             |
| ------------ | ------------------- | ------------ | ---------------------------------------------------------------- |
| Repository   | Actions             | Read-only    | 查询 job/run 状态、列出排队 job、读取日志；接收 webhook 事件所需 |
| Repository   | Administration      | Read & write | 仓库级 runner 注册（spec 未设置 `runner_group` 时）              |
| Repository   | Metadata            | Read-only    | 识别仓库及其所属账户                                             |
| Repository   | Pull requests       | Read-only    | 在 job 分组中显示 PR 标题                                        |
| Organization | Self-hosted runners | Read & write | 组织级 runner 注册（spec 设置了 `runner_group` 时）              |

设置 `github.app.slug` 可在用户 UI 中显示"安装 GitHub App"链接。使用 `github.allowed_repositories`（支持 `owner/repo` 或 `owner/*` 模式）限制哪些仓库可以使用此 runnerd 实例。

### OAuth 登录

`github.oauth` 用于启用内置控制台的 GitHub App OAuth 登录：

- 使用 GitHub App 的 **Client ID** 和 **Client Secret**。
- 将 App callback URL 设置为 `http://<host>:<port>/auth/github/callback`。
- 将 `auth.session_secret`（session 签名）和 `auth.encryption_key`（用户 secret 加密）设为不同的随机值。

首次 OAuth 登录会创建 `role: user` 的账户。使用 `--bootstrap-admin <github-user-id>` 将账户提升为管理员。

### Webhook 事件订阅

在 GitHub App 设置页面（**Settings → Developer settings → GitHub Apps → 你的 App → General**）中配置：

1. 将 **Webhook URL** 设置为 `https://<你的runnerd地址>/webhooks/github`。
2. 在 **Subscribe to events** 中勾选：
   - **Workflow jobs**（`workflow_job`）— **必需**，触发 runner 创建。
   - **Workflow runs**（`workflow_run`）— 可选，作为 `workflow_job` 丢失时的补偿信号。
3. 保存更改。

> **⚠️ 常见坑：** 如果没有订阅任何事件，GitHub 不会发送任何 webhook，job 将永远卡在 queued 状态。此配置在 **GitHub App 设置页面**，不是仓库的 webhook 设置。

## Webhook 与 Workflow 配置

1. 确保已按上述 [Webhook 事件订阅](#webhook-事件订阅) 配置好 GitHub App webhook，且 `webhook_secret` 与配置文件中的 `github.webhook_secret` 一致。
2. 在 workflow 中使用：

```yaml
runs-on: [self-hosted, <your-runner-label>]
```

runnerd 处理 `queued`、`in_progress` 和 `completed` 动作。对于 `workflow_run` 事件，runnerd 会列出该 run 下所有排队 job，并将尚未入队的匹配 job 创建 runner request。

## Runner Spec 与 Policy

Runner spec、runner group 和 repository policy 通过管理 API 和控制台管理，**不在** `runnerd.yaml` 中配置。

- **Runner Spec**：定义 runner label、沙箱模板和可选的 `runner_group`。设置 `default_available: true` 使其对所有允许的仓库可用。
- **Runner Group**：spec 设置了 `runner_group` 时，runnerd 创建组织级 runner；否则创建仓库级 runner。

> **⚠️ 个人账号注意：** `runner_group` 需要调用组织级 GitHub API。如果仓库属于个人账号（而非组织），必须将 `runner_group` 留**空**，否则 runner 注册会返回 404 错误。

- **Repository Policy**：为特定仓库授权访问默认之外的额外 spec。

每个 spec 的 `template_id` 应指向包含 GitHub runner 镜像的 Qiniu Sandbox 模板。创建沙箱时会使用仓库 owner 的 Sandbox service Preferences 检查模板访问权限。

## 管理控制台

内置 Web UI 提供：

| 路由                     | 说明                           |
| ------------------------ | ------------------------------ |
| `/admin/`                | 仪表盘：诊断、指标、最近失败   |
| `/admin/accounts`        | 账户管理：列表、搜索、角色变更 |
| `/admin/sandbox_service` | Sandbox 服务配置               |

普通用户路由包括 `/repositories`、PR job 分组（`/github/pulls/{owner}/{repo}/{number}/jobs`）、账户设置（`/account/preferences`、`/account/sandbox-templates`、`/account/sandbox-instances`），以及对应的 `/organizations/{login}/...` 路由。

Runner request 列表默认返回最新 100 行，单页最多 500 行，并且只读取公开 runner state 所需字段，不加载已保存的 webhook payload 或 Sandbox credentials。Admin 轮询使用 `(queued_at DESC, id ASC)` 索引；经过 repository 授权的普通用户轮询通过 `(github_installation_id, queued_at DESC, id ASC)` 分别查询每个 installation，再合并有界结果，同时保留精确的 installation/repository 授权关系。

## 常见问题排查

| 现象                                                 | 可能原因                                 | 解决方法                                                                           |
| ---------------------------------------------------- | ---------------------------------------- | ---------------------------------------------------------------------------------- |
| Job 一直卡在 **queued**，runnerd 日志无 webhook 记录 | GitHub App 未订阅事件                    | 进入 GitHub App 设置 → 勾选 **Workflow jobs** 事件                                 |
| `github registration token: status 404`              | 设置了 `runner_group` 但仓库属于个人账号 | 清空 runner spec 中的 `runner_group`，改用仓库级注册                               |
| 日志中出现 `invalid signature`                       | Webhook secret 不匹配                    | 确保 `github.webhook_secret` 与 GitHub App/仓库 webhook 设置中的 secret 一致       |
| `runner start deferred ... at capacity`              | 全局或 spec 并发上限已满                 | 等待运行中的 job 完成，或调大 `max_concurrent_runners` / spec 的 `max_concurrency` |
| 沙箱创建失败                                         | 未配置 Sandbox 服务凭据                  | 在账户/组织 Preferences 或 `/admin/sandbox_service` 管理面板中配置 API 凭据        |

更多本地调试步骤请参阅 [docs/zh/testing.md](docs/zh/testing.md)。

## Docker

容器镜像仅使用文件配置。将 `runnerd.yaml` 和引用的密钥文件挂载到容器中：

```bash
docker run --rm -p 25500:25500 \
  -v "$PWD/runnerd.yaml:/etc/runnerd/runnerd.yaml:ro" \
  -v "$PWD/secrets:/etc/runnerd/secrets:ro" \
  ghcr.io/qiniu/ci-runner
```

## 构建与开发

```bash
task deps          # 安装 Go 依赖
task ui-deps       # 安装 UI 依赖
task build         # 构建 runnerd（内嵌生产 UI）
task dev           # 启动本地开发环境（runnerd + Vite + smee）
task lint          # 运行代码检查
task test          # 重建 UI + 运行全部测试（Go race detection + Bun UI tests）
task docker-check  # 验证 Docker 构建
task release-check # 验证发布构建
```

单独运行 UI 测试：`cd ui && bun run test`。

### 沙箱模板

| 模板                                   | 说明                                                               |
| -------------------------------------- | ------------------------------------------------------------------ |
| `templates/github-runner-ubuntu-24.04` | 默认 GitHub runner 镜像（runner 运行时、Docker、辅助工具、rclone） |
| `templates/qbox-kodo-ubuntu-16.04`     | 旧版 Ubuntu 16.04，用于 qbox/kodo 风格的 job                       |

使用 `task template-build-prod` 构建模板。qbox-kodo 基础镜像可通过 `task qbox-kodo-base-build` 单独重建。

## 文档

| 文档                                                                                   | 说明                                                    |
| -------------------------------------------------------------------------------------- | ------------------------------------------------------- |
| [docs/zh/testing.md](docs/zh/testing.md)                                               | 本地测试、GitHub App/OAuth 设置、webhook 转发、故障排查 |
| [docs/zh/deployment-smoke.md](docs/zh/deployment-smoke.md)                             | 生产环境就绪检查清单                                    |
| [docs/zh/runner-architecture-comparison.md](docs/zh/runner-architecture-comparison.md) | 架构图及与 ARC / Fireactions 的对比                     |
| [docs/zh/runner-implementation-review.md](docs/zh/runner-implementation-review.md)     | 实现状态与 schema 迁移说明                              |
