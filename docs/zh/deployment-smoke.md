# 部署 Smoke Checklist

[English](../deployment-smoke.md)

在把 runnerd 部署视为可以承载真实 GitHub Actions 流量前，使用这份 checklist 做生产风格验证。

## 前置条件

- 一个可通过 HTTPS 接收 GitHub webhooks 和 console 登录的 runnerd 部署。
- `runnerd.yaml` 已配置 `database`、`auth`、`github` 和 `worker` sections。
- 目标 repository 或 organization 已安装 GitHub.com App。
- GitHub App 已配置当前部署所用 runner 模式需要的[仓库级和组织级权限](../../README.zh.md#所需权限)。
- GitHub App OAuth callback URL 指向 runnerd origin 下的 `/auth/github/callback`。
- GitHub App webhook 或 repository webhook 将 `workflow_job` events 发送到 `POST /webhooks/github`。
- 目标 account/organization Preferences 已配置 Sandbox service API URL 和 API key，或 `/admin/sandbox_service` 已启用 admin fallback。
- 至少一个 Qiniu sandbox template 包含 `/opt/actions-runner/config.sh` 和 `/opt/actions-runner/run.sh`。
- 已通过 `runnerd --bootstrap-admin github:<github-user-id>` 引导 admin account（该命令设置 admin 后直接退出，需在启动服务前执行）。

不要在本文档中写入真实 secret，也不要提交部署本地文件，例如 `runnerd.local.yaml`、`.smee-url`、sqlite databases、private keys 或 cookie jars。

## 1. Service Health

确认服务可访问：

```bash
curl -fsS https://<runnerd-host>/healthz
```

预期结果：HTTP 200，响应包含 `status: ok`。

从 `index.html` 找到当前带哈希的 JavaScript 或 CSS 路径，验证生产 UI 的缓存和压缩响应头：

```bash
curl -sS -D - -o /dev/null https://<runnerd-host>/
curl -sS --compressed -D - -o /dev/null https://<runnerd-host>/assets/<current-hashed-asset>.js
```

预期结果：

- HTML shell 返回 `Cache-Control: no-store`。
- `/assets/` 下带内容哈希的文件返回 `Cache-Control: public, max-age=31536000, immutable`。
- 请求接受 gzip 时，大型 JavaScript 和 CSS 响应返回 `Content-Encoding: gzip`，同时包含 `Vary: Accept-Encoding`。
- 未版本化的静态文件使用短期浏览器缓存，而不是 immutable 策略。

通过 admin console 登录：

```text
https://<runnerd-host>/admin/
```

预期结果：GitHub OAuth 完成，signed session 具有 `role: admin`。

准备至少一个次要 account，并打开 Accounts 页面：

```text
https://<runnerd-host>/admin/accounts
```

检查：

- 搜索、角色筛选、每页条数和翻页只改变账户列表，全局统计总数保持不变。
- 关联 GitHub identity 会按 login 加载头像；头像不可用时回退到账户首字母。
- 当前管理员的角色控件处于禁用状态。
- `role: user` session 调用账户列表和角色修改 API 都会被拒绝；管理员直接 PATCH 自身 role 会返回 conflict。
- 把次要 account 从 `user` 改为 `admin` 后立即生效，并生成 `account.role.update` 审计事件。
- 准备两名管理员和两个已登录 session，并发执行相互降级时不能同时成功；至少保留一名管理员。
- 完成全部角色检查后，如有需要，先由存活的管理员恢复原管理员，再由预期管理员恢复次要 account 的 role。

## 2. Diagnostics

打开 admin console 的 diagnostics 页面，或调用：

```bash
curl -fsS -b "$COOKIE_JAR" https://<runnerd-host>/diagnostics/pprof | jq
curl -fsS -b "$COOKIE_JAR" https://<runnerd-host>/diagnostics/vars | jq
```

检查：

- 推荐部署路径下 `github.auth_mode` 是 `app`。
- `state.database` 指向预期的 sqlite、Postgres 或 MySQL 数据库。
- 当 local pprof service 可用时，可以看到 pprof discovery files 和 dump scripts。
- Recent failure summaries 为空，或每一项都已理解。

## 3. Runner Catalog

创建 runner specs 前先验证 Sandbox credential precedence：

- 没有 scoped credentials 的 account 仅在 admin default 已启用且完整时可以列出 templates。
- `all` 模式下，个人 repository owner 与 organization owner 都能使用完整 default。
- `selected` 模式下，stable-ID audience list 中的 owner 可以使用 default；未选择的 owner 和空 audience 都不能使用。
- 添加一个从未登录或同步的 GitHub login，确认 admin response 显示 GitHub 返回的 canonical login、stable ID 和 account type。
- 启用 GitHub App auth 后，确认 selected owner 的第一个 workflow 能解析并缓存原本未知的 installation owner；后续请求不应再次查询 owner。
- 保存 account 或 organization scoped credentials 后，effective source 不再是 `admin_default`。
- 移除 audience entry 会阻止新的 fallback resolution，但不会改变已 snapshot 的 runner request。
- 禁用 admin default 后，原本未配置的 account 会得到 `sandbox service not configured`。

创建或确认 runner spec：

```bash
curl -fsS -X POST https://<runnerd-host>/runner_specs \
  -b "$COOKIE_JAR" \
  -H 'content-type: application/json' \
  -d '{"name":"ubuntu-24-04","labels":["self-hosted","e2b"],"template_id":"<template-id>","max_concurrency":1,"enabled":true,"default_available":true}' | jq
```

如果 spec 需要限制访问，将 `default_available` 设为 `false`，并为目标仓库创建 runner policy 或 runner group。

运行 match test：

```bash
curl -fsS -X POST https://<runnerd-host>/runner_specs/match \
  -b "$COOKIE_JAR" \
  -H 'content-type: application/json' \
  -d '{"repository_full_name":"<owner>/<repo>","labels":["self-hosted","e2b"]}' | jq
```

预期结果：响应包含预期 runner spec。

## 4. Webhook Delivery

在 GitHub App 或目标 repository 的 webhook 设置中重发最近一次 delivery，或触发一个新 workflow。

预期结果：

- Delivery 使用 `application/json`。
- Delivery 包含有效的 `X-Hub-Signature-256`。
- runnerd 对支持的 `workflow_job` actions 返回 2xx JSON response。
- Unsupported events 会被有意 ignored，而不是作为 runner failures。

## 5. Workflow Pickup

使用最小 workflow：

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

手动触发。

预期结果：

- Runner request 依次显示为 `queued`、`creating`、`running`。
- GitHub Actions job 离开 queued 状态，并运行在 `e2b-*` runner 上。
- Job 的 `Set up runner` log 包含 Qiniu sandbox id、runner request id 和 runner name。
- Job 结束后，runner request 变为 `completed`。

## 6. Cleanup

Workflow 完成后确认：

- Qiniu sandbox 已停止或不再 active。
- GitHub self-hosted runner registration 已移除，或已 offline 并被 runnerd 清理。
- Runner request 的 control/stdout/stderr logs 可以通过 admin UI 或 `/runner_requests/{id}/logs/{name}` 查看。
- `/diagnostics/vars` 显示更新后的 workflow job、runner registration、cleanup 和 duration counters。

## 7. Failure Drill

部署仍在观察期时，运行一个受控失败：

- 使用不匹配任何 runner spec 的 labels，或
- 临时禁用匹配的 runner spec，或
- 降低 spec concurrency 并触发两个 jobs。

预期结果取决于场景：

- unmatched labels 或 disabled specs 会记录为 admission failures；
- concurrency pressure 会让后续 requests 保持 queued，而不是被丢弃；
- retryable placement 或 rate-limit failures 会填充 `next_retry_at`，并保持后续可处理。

如果部署说明包含 private hosts、account names、channel URLs、secrets 或 cookie data，请记录在仓库外部。
