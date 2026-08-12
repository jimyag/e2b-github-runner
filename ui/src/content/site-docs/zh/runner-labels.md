# 托管 Runner 标签

使用受支持的七牛标签组合选择已维护的公共 Sandbox 模板，无需创建自定义 Runner Spec。

## 支持的标签

| Workflow 请求 | 模板状态 | 说明 |
| --- | --- | --- |
| `[qiniu, ubuntu-slim]` | 稳定 | 更小的通用镜像 |
| `[qiniu, ubuntu-22.04]` | 稳定 | Ubuntu 22.04 x64 |
| `[qiniu, ubuntu-24.04]` | 稳定 | 推荐默认值 |
| `[qiniu, ubuntu-26.04]` | 预览 | 预览镜像，请明确选择 |
| `[qiniu, ubuntu-latest]` | 稳定映射 | 当前映射到 Ubuntu 24.04 |

## 匹配规则

每个托管 spec 都声明 `self-hosted`、`linux`、`x64`、`qiniu` 和准确的操作系统标签，并要求 `qiniu` 与该操作系统标签。

匹配始终保持：

```text
必需标签 ⊆ Job 标签 ⊆ 声明标签
```

因此，`[qiniu, ubuntu-24.04]` 和完整声明标签都可以匹配。标签不完整或包含不受支持的标签时不能匹配。

## 托管与自定义的所有权

runnerd 管理托管名称、标签、必需标签、公共模板名称、优先级和默认可用性。Operator 控制 `enabled`、`max_concurrency` 和 `min_idle`。

自定义 spec 仍由 operator 管理，使用显式 template ID，并且可以定义其他声明标签和必需标签。保存自定义 spec 并不能证明模板在所选 Sandbox 区域中存在或可用。

## 模板解析

注册前，runnerd 会通过有效的账号或组织 Sandbox endpoint 解析托管公共模板名称。因此，同一个稳定公共名称可以在不同区域解析为不同 template ID，无需把某个区域的 ID 保存在 spec 中。

托管示例见[运行第一个工作流](/docs/guides/workflow)，完整自定义流程见[构建并使用自定义 Runner 模板](/docs/guides/custom-templates)。
