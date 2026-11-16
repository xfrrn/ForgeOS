# 06 — 任务规划与执行编排

## 1. 动态任务层级

- 简单任务：直接 Work Item。
- 复杂任务：Goal → Milestone → Work Item。
- 执行尝试：Execution。
- 计划变化：Plan Revision。

执行过程中发现简单任务复杂度超出预期，可以升级为 Goal，保留原任务和执行历史。

## 2. Planner 流程

1. 理解目标和验收结果。
2. Discovery：读取仓库文档、相关代码和项目知识。
3. 识别未知信息、依赖和风险。
4. 先形成整体 Milestone。
5. 详细拆分近期 Milestone 的 Work Item。
6. 建立 DAG。
7. 定义每个 Work Item 的目标、范围、依赖、验收标准、交付物。
8. 提交 Management Agent 审核。
9. Policy Engine 检查权限。
10. 计划生效。

## 3. 滚动规划

不要求一次性详细拆分所有未来工作：
- 整体方向和里程碑提前规划；
- 当前里程碑详细拆分；
- 后续里程碑根据执行结果继续细化；
- 发现方案不合理时创建新 Plan Revision；
- 已完成任务保留，未执行任务可调整，正在执行任务需安全检查。

已生效的 Goal 继续使用 `POST /tasks/{id}/plans` 提交下一版。任务详情的 `active_revision` 保留已生效版本，`plan.revision` 指向最新草案。审核期间暂停派发该 Goal 的新工作，已有执行可以回报结果。生效时按工作项 key 更新尚未执行的子项和依赖，保留任务 ID。已移除的子项显示为 SUPERSEDED，可查原修订和事件；已开始或已交付的子项保留规格、执行、PR 和合并记录。修改其范围或移除它会返回 409。父 Issue 只在当前版本的必需子项均已合并后关闭。

## 4. Work Item 最小字段

- objective；
- context_refs；
- scope；
- dependencies；
- acceptance_criteria；
- deliverables；
- execution_mode；
- required_capabilities；
- risk；
- estimated_resources。

任务没有可验证验收条件时，不应直接进入 READY。

## 5. Orchestrator

负责：
- 任务依赖检查；
- READY 队列；
- Worker 选择；
- 资源和并发检查；
- Execution Lease；
- Session 生命周期；
- Command Queue；
- 重试与恢复；
- 状态同步。

## 6. 命令队列

命令类型建议：
- IMMEDIATE_INTERRUPT：暂停/取消；
- ANSWER_PENDING：回答问题；
- STEER_ACTIVE：Harness 支持时调整当前执行；
- QUEUE_NEXT：当前 Turn 结束后执行；
- CHECKPOINT_BATCH：普通上下文和评论合并处理。

普通 Issue 评论在安全检查点统一处理。安全检查点包括完成独立开发步骤、进入新模块、提交前、CI 前和当前命令结束后。

Worker 领取时报告 `supports_checkpoints`。已观测到评论的 Issue 及其子任务交给支持此能力的 Worker。每次新执行开始时按原始事件 ID 冻结一批评论，同一来源评论的多个版本只将当时最新正文提供给模型。每个批次支持翻页和幂等重读，后续事件进入下一个批次。模型把评论作为当前 Issue 范围内的讨论上下文。Planner 在只读规划开始时读取评论；写入任务在每次模型回合结束后、Git 提交之前再次取批。如果有新内容，在同一工作区完成新的原生回合后再检查。空批次后提交并验证固定候选。评论不改变需求授权，也不触发新的 Work Item。

Owner 可从任务详情的“查看执行检查点”读取各批的评论数、事件上界、时间和处理会话。API 为 `GET /tasks/{id}/checkpoints?execution_id={execution_id}`。原始版本通过任务评论历史查询。执行租约只允许读取本次运行的批次或确认对应的原生 Session。Worker 失联时已写入的批次和处理回执可分别核对。

在同一次写入执行中，Codex 通过 `exec resume` 将新评论交给上一回合的 Thread，Claude Code 通过 `--resume` 复用 Session。两个批次可对应不同 PID；检查点各自保存处理该批输入的会话。退回重做仍从新 Execution 开始。

取消和紧急暂停可请求尽快安全中断。

## 7. 第一版并发策略

- 不同项目允许并行；
- 同项目最多一个写入型 Work Item；
- 同项目允许额外只读审查；
- 同一模块多个写入型 Agent 不允许；
- 等待人工回答的任务释放活动模型执行名额；
- 资源不足时普通任务排队；
- 紧急暂停和取消优先。

控制面在领取事务中先按节点串行核对已占名额，再锁定项目、重新检查任务状态并创建执行。来自同一节点的并发请求共用容量。名额满时返回 204，工作项留在 READY。控制面已收到完成结果或 Owner 已结束失联执行时，名额可重新分配。执行已结束、正在等待验收或答复的任务不会占用节点的执行名额。

## 8. 审批

Management Agent 审核计划，Policy Engine 检查权限。大型计划是否需要 Owner 决策取决于决策等级，不是仅按任务大小决定。
