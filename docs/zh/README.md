# 文档索引

[English](../README.md)

这些文档和根目录 `README.zh.md` 配合阅读。

- [本地测试与 GitHub 配置](testing.md)：本地配置、GitHub App/OAuth 设置、webhook forwarding、admin API 示例和排障流程。
- [部署 Smoke Checklist](deployment-smoke.md)：面向真实 GitHub App、webhook、Qiniu sandbox template、runner pickup、cleanup 和 diagnostics 的生产风格 smoke checklist。
- [Runner 架构对比](runner-architecture-comparison.md)：当前 runnerd 架构基线、Mermaid 系统/生命周期/状态图、DB-backed state model，以及和 Fireactions、Actions Runner Controller 的对比。
- [Runnerd 实现评审](runner-implementation-review.md)：当前实现状态、schema migration notes，以及剩余产品/运维决策。

根目录 `README.zh.md` 是中文 operator quick start。`TODO.md` 记录仍待决策的事项，不应重复已经在这些文档中描述的已完成行为。

Agent-only rules 和 repeatable workflows 放在 `.agents/` 下。业务、运维、架构和部署文档保留在 `docs/`；只有 durable agent guidance 或可执行 agent workflow instructions 才移动到 `.agents/rules/` 或 `.agents/skills/`。
