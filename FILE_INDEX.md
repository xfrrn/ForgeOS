# 文件索引

[全路线验收记录](docs/acceptance.md)列出各阶段已有能力、实际操作结果和未完成的验收。

从[项目首页](README.md)了解项目现状，再按手头的开发任务查阅下表。编号 00–16 是专题文档，ADR 记录已确认的架构决策。

运行基座时从[本地运行指南](docs/local-development.md)开始。两个进程的入口分别是[控制面](cmd/controlplane/main.go)和[Worker](cmd/worker/main.go)，可用[验收脚本](scripts/smoke.ps1)提交并核对首个任务。

跟踪一次派发可依次查看 [HTTP 接口](cmd/controlplane/api.go)、[数据表](internal/store/001_initial.sql)、[状态和租约事务](internal/store/store.go)及 [Git 任务执行](cmd/worker/execute.go)。

核对原生进度可查看[公开消息采集](cmd/worker/model_output.go)、[工具事件和结果采集](cmd/worker/tool_activity.go)、[命令增量输出](cmd/worker/tool_stream.go)、[节点进程回报](cmd/worker/live_process.go)和[控制面进程校验](internal/store/live_process.go)。独立计划及共享资料审核另见[Management 进程采集](cmd/worker/management_process.go)及[审核历史校验](internal/store/management_process.go)和[工作台进程记录](web/src/ManagementProcesses.tsx)。

跟踪真实需求可查看 [GitHub Issue 接入](cmd/controlplane/github.go)、[Issue 快照与迁移](internal/store/002_local_issues.sql)、[Codex 调用与候选提交](cmd/worker/codex.go)及[本地验证](cmd/worker/validation.go)。

跟踪验收后的交付可查看[验收 API 与交付循环](cmd/controlplane/delivery.go)、[验收记录与交付队列](internal/store/delivery.go)及 [GitHub PR 与 Issue 回写](cmd/controlplane/delivery_remote.go)。

维护任务工作台时，从[工作台总览](web/src/Overview.tsx)、[任务列表](web/src/TaskList.tsx)、[任务详情与候选验收](web/src/TaskDetail.tsx)及[同源页面入口](cmd/controlplane/web.go)开始。

## 确定范围与实施顺序

| 要解决的问题 | 文档 |
|---|---|
| ForgeOS 负责哪些工作，第一版如何判断成功？ | [00 产品章程](docs/00-product-charter.md) |
| 一个 Issue 从接入到归档经过哪些阶段？ | [01 端到端流程](docs/01-end-to-end-workflow.md) · [端到端流程图](diagrams/end-to-end.mmd) |
| 如何从运行基座逐步扩展到多项目中台？首期要核实哪些接口？ | [15 总体实施路线](docs/15-mvp-roadmap.md) · [16 待验证与待决策事项](docs/16-open-decisions.md) |

## 接入项目、设计控制面与调度任务

| 开发任务 | 专题与决策 |
|---|---|
| 划分控制面、Worker 和模块职责 | [02 系统架构](docs/02-system-architecture.md) · [系统架构图](diagrams/system-overview.mmd) · [03 模块目录与边界](docs/03-module-catalog.md) |
| 导入项目、处理 Issue 事件、识别任务模板 | [12 GitHub 接入与交付](docs/12-github-delivery-and-feedback.md) · [ADR-002 Issue 模板触发](docs/adr/ADR-002-issue-trigger.md) |
| 区分 Task Request、Goal、Work Item 和 Execution | [04 核心领域模型](docs/04-domain-model.md) · [ADR-006 动态任务层级](docs/adr/ADR-006-dynamic-task-hierarchy.md) |
| 分配角色权限，处理决策请求与 Agent 消息 | [05 Agent 角色、决策与协作协议](docs/05-agent-roles-and-decisions.md) |
| 拆分任务、保存计划、安排并发与处理运行中的变更 | [06 规划与编排](docs/06-orchestration-and-planning.md) · [ADR-008 计划留痕](docs/adr/ADR-008-plan-storage.md) · [ADR-011 项目级并发](docs/adr/ADR-011-concurrency-policy.md) · [ADR-003 安全检查点与事件合并](docs/adr/ADR-003-checkpoint-event-batching.md) |

## 执行任务、管理环境与数据

| 开发任务 | 专题与决策 |
|---|---|
| 启动本地 CLI、管理 Worktree、区分会话和进程 | [07 Worker 与 Harness 运行时](docs/07-harness-and-worker-runtime.md) · [ADR-009 登录状态复用](docs/adr/ADR-009-harness-auth-reuse.md) · [ADR-010 进程与会话生命周期](docs/adr/ADR-010-process-session-lifecycle.md) |
| 检索项目知识，决定哪些信息可更新或共享 | [08 项目知识与上下文](docs/08-project-knowledge-and-context.md) · [ADR-004 知识来源](docs/adr/ADR-004-project-knowledge-sources.md) · [ADR-005 项目隔离与共享](docs/adr/ADR-005-knowledge-isolation.md) · [ADR-018 知识更新权限](docs/adr/ADR-018-knowledge-update-authority.md) |
| 隔离环境凭据，控制业务密钥访问 | [09 安全、权限与密钥](docs/09-security-and-secrets.md) · [ADR-001 环境凭据隔离](docs/adr/ADR-001-environment-isolation.md) |
| 存储事件、审计记录与交付证据 | [13 数据存储与审计](docs/13-data-storage-and-audit.md) · [ADR-007 全流程追溯](docs/adr/ADR-007-traceability.md) |
| 配置中台运行主机、私网、反向代理与备份 | [14 中台部署与运行](docs/14-deployment-and-operations.md) |

## 验证、验收与交付

| 开发任务 | 专题与决策 |
|---|---|
| 为候选 Commit 选择检查、运行本地 CI、汇总审查结果 | [10 代码审查与自动化验证](docs/10-validation-and-local-ci.md) · [ADR-012 验证方案与权限](docs/adr/ADR-012-validation-ownership.md) · [ADR-013 动态验证等级](docs/adr/ADR-013-dynamic-validation-level.md) |
| 生成测试计划、开放私网预览、记录反馈与验收版本 | [11 远程预览与人工验收](docs/11-remote-preview-and-acceptance.md) · [ADR-014 预览与页面反馈](docs/adr/ADR-014-remote-preview-feedback.md) · [浏览器扩展](extension/manifest.json) · [Caddy 网关](deploy/caddy/Caddyfile) · [Staging 回调入口](deploy/caddy/Staging.Caddyfile) |
| 验收后创建 PR、合并并回写 Issue | [12 GitHub 接入与交付](docs/12-github-delivery-and-feedback.md) · [ADR-015 代码交付范围](docs/adr/ADR-015-code-delivery-scope.md) · [ADR-016 默认 Squash Merge](docs/adr/ADR-016-squash-merge.md) · [ADR-017 Issue 关闭条件](docs/adr/ADR-017-issue-close-policy.md) |

## 配置与消息示例

下列文件用于理解字段和数据形态。命令、路径和仓库名来自假设的接入项目。正式协议和配置格式的进度见[待决策清单](docs/16-open-decisions.md)。

| 文件 | 查看内容 |
|---|---|
| [project.yaml.example](schemas/project.yaml.example) · [项目配置校验](schemas/project.schema.json) | 项目标识、仓库与本地目录、Harness、交付策略、分环境密钥需求和本地启动配置。Worker 使用固定提交声明的脚本和就绪端点检查本地环境，工作台与命令行保留脱敏启动日志。采集实现见 [environment_log.go](cmd/worker/environment_log.go)，操作见[本地运行指南](docs/local-development.md)。 |
| [validation.yaml.example](schemas/validation.yaml.example) | 接入项目的检查命令、超时和检查组合 |
| [validation.schema.json](schemas/validation.schema.json) | 版本 1 的验证配置合同，控制面读取配置时使用 |
| [acceptance.yaml.example](schemas/acceptance.yaml.example) | 按模块路径选择验收方法、最低验证等级和人工验收条件 |
| [acceptance.schema.json](schemas/acceptance.schema.json) | 项目验收规则的版本 1 合同，含模块、Playbook、环境和预期行为 |
| [playbooks.json](schemas/playbooks.json) · [playbooks.schema.json](schemas/playbooks.schema.json) | 全局验收方法及版本 1 合同，按 ID 查阅前置条件、操作、判据与取证要求 |
| [agent-message.example.json](schemas/agent-message.example.json) · [消息格式 1.0](schemas/agent-message.schema.json) | Agent 消息的发送方、接收方、关联标识、幂等键、来源与载荷。原生检查点问题和 Owner 答复可由控制面按此格式回读。 |
