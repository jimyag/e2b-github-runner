# Runnerd 实现评审

[English](../runner-implementation-review.md)

日期：2026-07-03

范围：

- 刷新时的当前分支：`main` 上的当前 working tree。
- 评审目标：file-based config、GORM-backed DB schema migration、retry/lease/audit handling、ordinary-user UI、admin console、embedded UI assets 和 local development workflow 更新后的实现状态。
- 仍可用于后续对比的参考：actions-runner-controller 风格 reconciliation，以及 fireactions 风格 pool/config modeling。

## 摘要

Runnerd 已经越过最初 2026-05-19 的差距清单。Runtime configuration 现在是 file-first，runner state 已 DB-backed，schema creation 主要由 GORM model tags 驱动，retry/lease/audit 字段已经存在，GitHub App auth 可以动态解析 installations，ordinary-user UI 覆盖 job/repository/account setup flows，admin console 覆盖核心管理流程，diagnostics 暴露 pprof/expvar state，文档化的本地 workflow 包含 `task dev`。

剩余工作不再是基础架构补课，而是产品和运维 hardening：是否保留 token/basic auth 作为本地兼容模式，Activity repositories 是否应在 jobs 被观察到前包含 policy-configured repositories，多少 config management 应进入 admin console，以及在把服务视为 production-ready 前需要哪些 deployment smoke tests。

## 当前基线

- 配置默认从 `runnerd.yaml` 加载，也可通过 `--config` 指定。相对 sqlite database paths 和 GitHub App private-key paths 会按配置文件目录解析。
- Config schema 覆盖 server、database、OAuth session auth、Sandbox lifecycle timeouts、GitHub webhook/auth/OAuth、allowed repositories，以及 worker retry/lease/concurrency behavior。Sandbox service API URL 和 API key 是 account 或 organization Preferences，不是 file config。
- GitHub API auth mode 必须三选一：GitHub App、token 或 basic auth。GitHub App mode 支持可选静态 `installation_id`；省略时，runnerd 会按 job repository 解析 installation access 并缓存 transports。
- Runner requests、events、specs、groups、policies、retry metadata、leases 和 audit events 存储在配置的 database backend 中。
- State schema creation 会在少量旧 schema columns compatibility pass 后，通过 GORM `AutoMigrate` 执行。
- Worker processing 使用 DB claim/lease semantics 和 retry scheduling，而不是只依赖 in-memory queue ownership。
- Qiniu sandbox、GitHub、rate-limit、timeout 和 temporary network transient failures 会被分类为 retry 或 queue deferral。确定性的 auth/config/template failures 会立即失败。
- Admin routes 暴露 runner request management、retry/stop/log access、runner specs、runner groups、repository policies、match tests、audit events 和 diagnostics。
- Ordinary-user routes 暴露 `/` 的 PR/job dashboard、`/repositories` 的 local activity repositories、`/account/repositories` 的 GitHub App account setup，以及 `/account/preferences` 和 `/organizations/{login}/preferences` 的 account 或 organization Sandbox service Preferences。
- `ui/` 中的 React UI 会从 `internal/server/ui/*` 嵌入生产构建；development builds 通过 `internal/server/ui_assets_development.go` 代理到 Vite。
- `task dev` 会一起启动 Vite 和 Go service development mode。`task build` 先构建 UI，再用 embedded production assets 编译 `bin/runnerd`。
- Diagnostics 可通过 admin UI 和 `/diagnostics/pprof` / `/diagnostics/vars` 访问，底层是 `github.com/jimmicro/pprof` 和 expvar。

## 剩余决策

### 1. Auth Policy

Token 和 basic auth 仍与 GitHub App auth 并存。它们对本地验证或 legacy credentials 有用，但也意味着产品还不是 GitHub-App-only。需要决定这些模式是 intentional compatibility paths，还是应在 production hardening 前移除。

### 2. Ordinary-User Repository Scope

Ordinary-user UI 现在已经路由到 `/admin/*` 之外。Activity repositories 当前来自 runnerd-observed jobs，而 authorized repositories 可以从 GitHub App installations 按需加载。需要决定 Activity repositories view 是否也应在 jobs 被观察到前包含 repository-policy configured repositories。

### 3. Config Management

Runtime config 仍是 file-first，但 admin console 尚未提供 effective-config view、config validation preview、reload workflow 或 import/export flow。除非 live config operations 成为明确需求，否则继续保持当前 file-only operations model。

### 4. Deployment Smoke

Local build/lint/test coverage 只能验证代码路径；production readiness 仍依赖真实 GitHub App installation、真实 Qiniu sandbox templates、webhook delivery 和 sandbox runner execution。使用 `docs/zh/deployment-smoke.md` 做真实部署 checklist，覆盖 webhook signature handling、installation resolution、runner spec matching、sandbox creation、GitHub job pickup、cleanup 和 diagnostics。

### 5. Multi-Instance And Operations

DB lease model 已经存在，但在记录 multi-instance support 前，仍需用两个 runnerd instances 连接同一个 database 验证 multi-process behavior。Expvar diagnostics 覆盖有用 counters 和 gauges；只有在 deployment observability 需要时才添加 histogram/export adapters。

### 6. Schema Compatibility

当前 migration path 有意避免完整 handwritten migration history。`internal/state/records.go` 中的 GORM tags 定义正常 schema，`internal/state/db.go` 只保留针对旧 state databases 的窄范围 upgrade backfills。后续 schema changes 如果新增 required columns、带 uniqueness semantics 的 indexes 或 relationship constraints，应包含 old-schema upgrade tests。

## 建议顺序

1. 所有触及 backend/UI boundaries 的分支保持 `task dev`、`task build`、`task lint` 和 `task test` 绿色。
2. 使用真实 GitHub App、一个 repository 和一个 Qiniu sandbox template 运行并维护 deployment smoke checklist。
3. 决定是否保留 token/basic auth modes。
4. 决定 Activity repositories 是否应在 jobs 被观察到前包含 policy-configured repositories。
5. 只有在 config operations model 清晰后，再添加 effective-config diagnostics view。
6. 用并发 runnerd 进程压测 DB lease behavior，再宣传 multi-instance support。
7. 修改 state records 或 GORM migration tags 时，保留 old-schema upgrade coverage。

## 验证说明

2026-05-19 review 中的旧 findings 已废弃，因为相关实现已经发生实质变化。更新本文档时重新运行当前验证命令：

```bash
task lint
task test
task build
```
