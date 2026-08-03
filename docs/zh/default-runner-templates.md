# 公共 Runner 模板

[English](../default-runner-templates.md)

Qiniu 维护 4 个用于 GitHub Actions 的 Linux x64 Sandbox 物理模板：
Ubuntu Slim、Ubuntu 22.04、Ubuntu 24.04，以及处于预览阶段的 Ubuntu 26.04。
`ubuntu-latest` 是 Runner catalog 中指向 Ubuntu 24.04 的逻辑映射，不是第
5 个镜像。

这些定义目前仍处于开发阶段。只有模板在两个受支持的 Sandbox 区域都完成
构建、发布、catalog 检查和 smoke 验证后，本文才会将其描述为公共模板。
上游版本来源、兼容性契约和各镜像差异见
[`templates/README.md`](../../templates/README.md)。

## Workflow labels

完成双区域发布和 managed Runner rollout 验证后，请根据所需环境使用准确的
label 组合：

```yaml
jobs:
  slim:
    runs-on: [qiniu, ubuntu-slim]
    steps:
      - uses: actions/checkout@v4
      - run: uname -a

  ubuntu_22:
    runs-on: [qiniu, ubuntu-22.04]
    steps:
      - uses: actions/checkout@v4
      - run: uname -a

  ubuntu_24:
    runs-on: [qiniu, ubuntu-24.04]
    steps:
      - uses: actions/checkout@v4
      - run: uname -a

  ubuntu_26_preview:
    runs-on: [qiniu, ubuntu-26.04]
    steps:
      - uses: actions/checkout@v4
      - run: uname -a

  latest:
    runs-on: [qiniu, ubuntu-latest]
    steps:
      - uses: actions/checkout@v4
      - run: uname -a
```

`qiniu` label 是必需项。Managed 匹配遵守
`required_labels ⊆ job_labels ⊆ labels`，因此 `[ubuntu-24.04]`、`[qiniu]`
和带有不受支持额外 labels 的请求都不会匹配 managed default。Operator 可以在
Admin 中禁用单个 managed spec；从 workflow 中移除 `qiniu` 则会从 workflow
侧阻止 managed-default selection。自定义 spec 仍可使用 operator 定义的
required labels 和显式 template ID。

## 环境要求

- `qiniu/qshell` 2.19.10 或更高版本；
- `task`、`jq` 和 `curl`；
- 当前 Sandbox 区域的 `QINIU_API_KEY`；
- 指向当前区域端点的 `QINIU_SANDBOX_API_URL`。

如果所需版本的 qshell 不在 `PATH` 中，请显式传入可执行文件：

```bash
task template-build-ubuntu-24-04 QSHELL=/path/to/qshell
```

任一凭据变量为空时，所有远端任务都会直接失败。仓库中的
`qshell.sandbox.toml` 只保存稳定名称和资源设置；构建任务让 qshell 使用临时
副本，因此不会提交区域相关的 template ID。

## 在单个区域构建与验证

先运行无需凭据的快速源码检查：

```bash
task template-check-all
```

然后使用 qshell 构建真正的 Sandbox 模板。每个任务都会等待 qshell 输出终态
`Status: ready`；如果进程退出码为 0，却没有出现该状态，任务仍会判定构建失败。

```bash
task template-build-ubuntu-slim
task template-build-ubuntu-22-04
task template-build-ubuntu-24-04
task template-build-ubuntu-26-04
```

4 个构建全部 ready 后再发布：

```bash
task template-publish-ubuntu-slim
task template-publish-ubuntu-22-04
task template-publish-ubuntu-24-04
task template-publish-ubuntu-26-04
task template-defaults-check
```

`template-defaults-check` 要求每个稳定物理名称都恰好对应 1 个公共的
`ready` 或 `uploaded` 模板，且 template ID 非空；缺失或重复条目都会失败。

保留 catalog 检查输出的 template ID，然后执行真实的 Sandbox smoke：

```bash
task template-smoke IMAGE_KEY=ubuntu-slim TEMPLATE_ID=<slim-template-id>
task template-smoke IMAGE_KEY=ubuntu-22.04 TEMPLATE_ID=<22.04-template-id>
task template-smoke IMAGE_KEY=ubuntu-24.04 TEMPLATE_ID=<24.04-template-id>
task template-smoke IMAGE_KEY=ubuntu-26.04 TEMPLATE_ID=<26.04-template-id>
```

Smoke 会通过 qshell 创建临时 Sandbox，检查系统版本、架构、预装 Actions
Runner、出站 HTTPS、Docker daemon、可写 work/tool-cache 路径和清理行为。
无论验证是否成功，脚本都会尝试终止临时 Sandbox。请保存命令输出的 JSON 路径
作为发布证据。完整 compatibility manifest 仍是静态 inventory contract；
逐条 runtime conformance 只作为可选诊断，不阻塞发布可用性门槛。

本地 Docker 构建和 `task template-conformance-local` 只用于按需诊断，不能替代
qshell 模板构建或 Sandbox smoke。

## 首次双区域发布

按以下顺序完成全部构建、发布、catalog 和 smoke 流程：

1. 导出
   `QINIU_SANDBOX_API_URL=https://cn-yangzhou-1-sandbox.qiniuapi.com`，并设置
   扬州区域的 `QINIU_API_KEY`。
2. 使用 qshell 构建并发布 4 个模板，运行 `task template-defaults-check`，然后
   smoke 验证返回的 4 个 ID。
3. 保存构建输出、catalog ID、smoke JSON 和相关 workflow URL。
4. 导出
   `QINIU_SANDBOX_API_URL=https://us-south-1-sandbox.qiniuapi.com`，并设置
   美国南部区域的 `QINIU_API_KEY`。
5. 重复 4 个模板的构建、发布、catalog 检查和 smoke 验证。
6. 确认两个 catalog 结果都为每个稳定物理名称返回且仅返回 1 个可运行的公共
   模板。
7. 在将模板状态改为 verified 或启用独立的 managed Runner rollout 前，把双
   区域证据附加到 Issue #38。

Qiniu 负责维护 4 个物理镜像。只有经过评审的 Runner catalog revision 并取得
新的区域 smoke 证据后，才能修改 `ubuntu-latest` 映射。`ubuntu-26.04` 在上游
正式发布前始终属于预览模板。

## 回滚

移除公共可用性前，应先禁用 managed Runner Specs，然后运行对应的可逆发布
回滚命令：

```bash
task template-unpublish-ubuntu-slim
task template-unpublish-ubuntu-22-04
task template-unpublish-ubuntu-24-04
task template-unpublish-ubuntu-26-04
```

普通回滚不要删除模板对象。保留私有模板可以保存构建历史，并允许在不改变
自定义 Runner Specs 的前提下重新发布经过评审的版本。
