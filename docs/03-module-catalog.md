# 03 — 模块目录与边界

## 1. 控制面业务模块

| 模块 | 职责 | 不负责 |
|---|---|---|
| Project Manager | 项目导入、仓库映射、项目配置 | 读取并复制整个代码仓库 |
| GitHub Integration | Issue、评论、PR、状态同步 | 任务规划 |
| Task Manager | Task、Revision、状态和关联 | 直接启动 CLI |
| Classification Service | 规则分类和分类 Agent 协调 | 最终授权 |
| Goal Planner | Discovery、Goal、Milestone、Work Item、DAG | 进程管理 |
| Management Agent Service | 日常决策、计划审查、协调 | 绕过 Policy Engine |
| Policy Engine | 权限、风险、审批规则 | 模型推理 |
| Knowledge Service | 项目知识、决策、检索 | 把所有聊天自动转成规则 |
| Knowledge Curator | 知识提取和更新建议 | 自行赋予知识权威性 |
| Orchestrator | 队列、租约、调度、状态机 | 直接执行任意 Shell |
| Validation Coordinator | 选择验证方案、汇总结果 | 直接替代 CI Runner |
| Acceptance Manager | 测试计划、人工验收状态 | 伪造人工验收 |
| Delivery Manager | PR、合并、Issue 关闭 | 生产部署 |
| Notification Service | 站内、Telegram、飞书等通知 | 承担任务状态事实源 |

### Owner 站内通知

候选进入人工验收、验收检查点产生问题、执行失败或失联结算，以及 Management 返回需要人工处理的计划审核时，工作台可从任务事件读取对应通知。导航每 8 秒刷新仍待处理且未读的数量。通知页列出当前待处理项，也可切换到全部记录。任务与计划链接分别打开原任务与具体修订。

`GET /tasks/notifications` 返回待处理项，`?scope=history` 包含已离开处理阶段的记录。每页最多 30 条，用 `next_before` 读取更早记录。`GET /tasks/notifications/unread` 返回待处理未读数。`POST /tasks/notifications/{event_id}/read` 将首次阅读时间持久化到 PostgreSQL。工作台只有一个 Owner 身份，各浏览器共享已读状态。重复标记保留首次时间。任务处理状态取自执行、问题、计划及其修订。

### 外部通知

控制面可给指定项目的 Telegram 聊天和飞书自定义机器人发送待处理且未读的站内事项。消息携带项目、任务、事件种类和通知编号，Owner 回到工作台查看正文及处理事项。PostgreSQL 按事件和通道保存尝试次数、下次可领取时间和成功回执时间。配置与重试行为见[本地运行指南](local-development.md#外部通知)。真实账号收件待验收。

## 2. Worker 模块

| 模块 | 职责 |
|---|---|
| Worker Registry | 节点身份、心跳、能力和资源 |
| Job Receiver | 接收执行规格和租约 |
| Workspace Manager | Worktree 创建、恢复、清理 |
| Harness Registry | 已安装 Harness 和能力探测 |
| Harness Adapter | 统一不同 CLI 的调用方式 |
| Session Manager | 原生 Session ID、恢复和检查点 |
| Process Supervisor | 启动、监控、中断、退出 |
| Event Collector | 标准化 Harness 事件 |
| Resource Manager | 并发、CPU、内存、超时和预算 |
| Recovery Manager | Daemon、进程和 Session 恢复 |
| Local CI Runner | 本地检查、测试、构建 |
| Preview Manager | 预览环境生命周期和路由 |

本地 CI 按检查限制 CPU、内存、进程数与运行时间。Workspace Manager 为固定候选创建临时 Git Worktree；检查成功、失败和超时后都回收这份快照。[Linux 运行回执与资源读数](acceptance.md#ci-resource-worktree)

## 3. 横向能力

- Audit Service；
- Event Store；
- Artifact Manager；
- Configuration Manager；
- Identity & Access；
- Git Provider Adapter；
- Remote Access Gateway。

## 4. 第一版实现边界

模块先按 Go package / service interface 解耦，不要求每个模块独立部署。数据库可以共用一个 PostgreSQL 实例，但领域表和事务边界应清楚。
