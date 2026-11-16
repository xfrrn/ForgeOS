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
