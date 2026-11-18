# 03 — 模块目录与边界

下列各行链接到已运行的具体场景和对应的 ForgeOS 提交。跨模块的阶段验收及外部条件见[全路线对照](acceptance.md#当前验收进度)。

## 1. 控制面业务模块

| 模块 | 职责 | 不负责 | 实测入口与提交 |
|---|---|---|---|
| Project Manager | 项目导入、仓库映射、项目配置 | 读取并复制整个代码仓库 | [接入项目，生成版本化报告](acceptance.md#check-c-8) · `9a75d9e` |
| GitHub Integration | Issue、评论、PR、状态同步 | 任务规划 | [模板触发与普通讨论的派发](acceptance.md#adr-evidence-002) · `319aa65` |
| Task Manager | Task、Revision、状态和关联 | 直接启动 CLI | [从原验收决定追溯返工的两个候选](acceptance.md#candidate-record-identity) · `683d14f` |
| Classification Service | 规则分类和分类 Agent 协调 | 最终授权 | [规则未命中时启动原生分类会话](acceptance.md#issue-classification-agent) · `439415c` |
| Goal Planner | Discovery、Goal、Milestone、Work Item、DAG | 进程管理 | [只读节点提交计划，独立审核该版草案](acceptance.md#legacy-planner-event) · `13ccb3b` |
| Management Agent Service | 日常决策、计划审查、协调 | 绕过 Policy Engine | [引用 D1 决策，审阅 D2 业务问题](acceptance.md#question-advice-event-audit) · `92127c3` |
| Policy Engine | 权限、风险、审批规则 | 模型推理 | [对项目和环境实施授权](acceptance.md#adr-evidence-001) · `72cd89d` |
| Knowledge Service | 项目知识、决策、检索 | 把所有聊天自动转成规则 | [按项目与状态检索决策](acceptance.md#decision-search) · `e6fb688` |
| Knowledge Curator | 知识提取和更新建议 | 自行赋予知识权威性 | [提取经 Owner 确认的决定](acceptance.md#knowledge-confirmed-decision) · `4d1d737` |
| Orchestrator | 队列、租约、调度、状态机 | 直接执行任意 Shell | [独占写入名额，重启后补报并继续排队任务](acceptance.md#foundation-before-report) · `3296a47` |
| Validation Coordinator | 选择验证方案、汇总结果 | 直接替代 CI Runner | [按固定 Git 配置选择分级验证](acceptance.md#check-b-6) · `35866ba` |
| Acceptance Manager | 测试计划、人工验收状态 | 伪造人工验收 | [候选退回、重新验证和人工决定](acceptance.md#candidate-record-identity) · `683d14f` |
| Delivery Manager | PR、合并、Issue 关闭 | 生产部署 | [按子项依赖合并，最后关闭父 Issue](acceptance.md#dependent-goal-275) · `73339a5` |
| Notification Service | 站内、Telegram、飞书等通知 | 承担任务状态事实源 | [站内通知及本机 TLS 重试；两种外部通道收件待验](acceptance.md#notice-channels) · `656375c` |

### Owner 站内通知

候选进入人工验收、验收检查点产生问题、执行失败或失联结算，以及 Management 返回需要人工处理的计划审核时，工作台可从任务事件读取对应通知。导航每 8 秒刷新仍待处理且未读的数量。通知页列出当前待处理项，也可切换到全部记录。任务与计划链接分别打开原任务与具体修订。

`GET /tasks/notifications` 返回待处理项，`?scope=history` 包含已离开处理阶段的记录。每页最多 30 条，用 `next_before` 读取更早记录。`GET /tasks/notifications/unread` 返回待处理未读数。`POST /tasks/notifications/{event_id}/read` 将首次阅读时间持久化到 PostgreSQL。工作台只有一个 Owner 身份，各浏览器共享已读状态。重复标记保留首次时间。任务处理状态取自执行、问题、计划及其修订。

### 外部通知

控制面可给指定项目的 Telegram 聊天和飞书自定义机器人发送待处理且未读的站内事项。消息携带项目、任务、事件种类和通知编号，Owner 回到工作台查看正文及处理事项。PostgreSQL 按事件和通道保存尝试次数、下次可领取时间和成功回执时间。配置与重试行为见[本地运行指南](local-development.md#外部通知)。真实账号收件待验收。

## 2. Worker 模块

| 模块 | 职责 | 实测入口与提交 |
|---|---|---|
| Worker Registry | 节点身份、心跳、能力和资源 | [按具名节点轮换身份与证书](acceptance.md#check-b-10) · `266f11f` |
| Job Receiver | 接收执行规格和租约 | [按原执行租约领取与结算](acceptance.md#foundation-before-report) · `3296a47` |
| Workspace Manager | Worktree 创建、恢复、清理 | [三个候选临时验证工作树的创建与回收](acceptance.md#ci-resource-worktree) · `4ef15ca` |
| Harness Registry | 已安装 Harness 和能力探测 | [读取原生 CLI 版本并核对实际协议能力](acceptance.md#native-version-matrix) · `5412238` |
| Harness Adapter | 统一不同 CLI 的调用方式 | [原生回合中断；Claude 恢复后成功交付待验](acceptance.md#claude-worker-interrupt) · `2d991f2` |
| Session Manager | 原生 Session ID、恢复和检查点 | [中断后关联原 Session 与对应检查点](acceptance.md#model-pause-audit) · `562b666` |
| Process Supervisor | 启动、监控、中断、退出 | [只读角色与托管中进程重连](acceptance.md#supervised-roles) · `ffb9103` |
| Event Collector | 标准化 Harness 事件 | [原生进程和工具输出归档到控制面](acceptance.md#central-process-archive) · `755c6c0` |
| Resource Manager | 并发、CPU、内存、超时和预算 | [容器内读取 CPU、内存、进程限额，验证检查超时](acceptance.md#ci-resource-worktree) · `4ef15ca` |
| Recovery Manager | Daemon、进程和 Session 恢复 | [节点断电后的 Git 结果补报；活动模型整机恢复待验](acceptance.md#linux-power-loss-worktree) · `f0cd68f` |
| Local CI Runner | 本地检查、测试、构建 | [本地执行固定候选，保留失败日志和结果](acceptance.md#builtin-go-archive) · `9431507` |
| Preview Manager | 预览环境生命周期和路由 | [预览申请与路由；第二台 tailnet 设备待验](acceptance.md#check-a-7) · `9a08b44` |

本地 CI 按检查限制 CPU、内存、进程数与运行时间。Workspace Manager 为固定候选创建临时 Git Worktree；检查成功、失败和超时后都回收这份快照。[Linux 运行回执与资源读数](acceptance.md#ci-resource-worktree)

## 3. 横向能力

| 能力 | 实测入口与提交 |
|---|---|
| Audit Service | [核对任务时间线的 44 类事件](task-event-acceptance.md) · `5f37d25` |
| Event Store | [按任务、执行、事件回读，并核对重放](acceptance.md#task-event-audit) · `2c755f3` |
| Artifact Manager | [中央和节点的 CI 文件校验、恢复与下载](acceptance.md#ci-backup-pending) · `0f76287` |
| Configuration Manager | [按固定版本读取计划、检查及配置 blob](acceptance.md#check-b-6) · `35866ba` |
| Identity & Access | [限制节点项目范围并验证轮换后的凭据](acceptance.md#ci-archive-worker-scope) · `68cc02e` |
| Git Provider Adapter | [远端写入成功而本地丢失响应后恢复](acceptance.md#check-b-8) · `32c0fa0` |
| Remote Access Gateway | [Caddy 私网路由；外部设备与真实提供方回调待验](acceptance.md#check-a-7) · `ec7a462` |

## 4. 第一版实现边界

模块先按 Go package / service interface 解耦，不要求每个模块独立部署。数据库可以共用一个 PostgreSQL 实例，但领域表和事务边界应清楚。
