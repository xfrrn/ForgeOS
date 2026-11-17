# ForgeOS · 内部项目 Agent 工程管理中台

**复杂需求协作和多项目运行正在分项验收。** 单项目 Issue 已跑通 Codex 修改、页面验收和 GitHub 交付。维护者还可把 Issue 升级为版本化 Goal，激活里程碑下的工作项，在浏览器回答候选验收中的业务问题。控制面核对前置合并后派发后续工作，全部子项交付后关闭父 Issue。两个私有项目已完成真实交付，节点容量调度和 GitHub 写入后丢失响应的恢复均已实测。[本地运行指南](docs/local-development.md) · [各阶段验收与缺口](docs/acceptance.md)

ForgeOS 让维护者在远程提交需求，由自有机器上的 Worker 调用 Codex 等 Harness，在独立 Git Worktree 中修改代码。控制面跟踪任务、验证和验收，满足交付条件后将代码合入 GitHub，并把结果写回 Issue。生产发布由人工负责。

## 从哪里开始开发

**先运行一个本地任务。** 通过 API 登记本地项目，创建一个 `git_head` 工作项。Worker 在独立 Worktree 中读取 HEAD，回报提交号和退出码。重启控制面后，仍能从工作项查到同一次执行。后续阶段和验收方法见[总体实施路线](docs/15-mvp-roadmap.md)。

**再用真实需求验收。** 在已登记的 GitHub 克隆上选定开放 Issue 和验证方式，运行验收脚本的 `-IssueNumber`、`-Validation` 模式。验证通过后，从 `GET /tasks/{id}` 取执行 ID 和候选 SHA，检查差异，再向 `POST /tasks/{id}/acceptance` 提交决定。查询结果包含历次决定、PR 和最终合并 SHA。

了解最终产品可读[产品章程](docs/00-product-charter.md)和[端到端流程](docs/01-end-to-end-workflow.md)。开始写基座时，重点看[系统架构](docs/02-system-architecture.md)、[模块职责](docs/03-module-catalog.md)、[领域模型](docs/04-domain-model.md)和[待验证事项](docs/16-open-decisions.md)。

## 架构与技术基线

| 部分 | 建议技术与职责 |
| --- | --- |
| 控制面与管理界面 | Go 模块化单体、React + TypeScript、PostgreSQL。Orchestrator 管理调度和执行状态，Policy Engine 强制检查权限。 |
| 本地执行 | 独立的 Go Worker Daemon，可与控制面同机部署。基座使用 Git Worktree 隔离改动，单项目交付时接入 Codex 和 Local CI Runner。部署时由 systemd 管理进程。 |
| 预览、密钥与知识 | 远程验收采用 Tailscale 私网，Caddy 可代理预览路由。OpenBao 管理业务密钥。项目资料支持 PostgreSQL 全文搜索，配置 pgvector 和本地推理服务后可按语义检索并回读命中的修订。[混合检索配置](docs/08-project-knowledge-and-context.md#项目资料混合检索) |

## 查阅与维护文档

[完整文件索引](FILE_INDEX.md)按开发主题列出设计文档、已确认的 ADR、配置与消息示例以及 Mermaid 图。阅读专题设计时，可对照路线图判断哪些能力属于当前里程碑。

文档中，**已确认**表示已接受的架构原则，**建议基线**表示可在实现中调整的设计，**待验证**需要真实 CLI、项目或环境提供证据，**待决策**需要进一步选择。重大设计变化应新增 ADR，关联受影响的专题与里程碑。每个里程碑单独定义验收标准。
