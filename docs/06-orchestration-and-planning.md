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

在工作台打开 Goal，默认展示最新修订。通过“上一版”“下一版”逐版查看里程碑、范围、依赖、验收条件和 Management 审核；“生效版”打开已获准执行的版本。有归档的计划显示 Git 提交。固定链接 `#/tasks/{goal}/plans/{revision}` 支持直接打开和刷新。

处于 PLANNING 的 Goal 在最新草案下显示“批准并激活修订 N”。Owner 可从站内审核通知直接打开该修订，核对 Management 的建议、依据和里程碑后批准。控制面按按钮上的修订号执行激活。如果另一会话已发布新版，旧请求返回 409，页面重新读取计划。响应丢失后，可在同一页面核对当前生效版本和子项。重复激活同一生效版保留原工作项 ID。

子任务页的 Goal 链接指向该子任务绑定的修订。生效计划可进入当前必需子项；历史修订仅关联仍能按修订号核实的子任务，并标明子任务的当前状态。草案中沿用相同 key 的新规格要等激活后才会关联执行。

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

Owner 可在运行中任务的详情页选择“取消任务”，也可请求 `POST /executions/{id}/cancel`。命令绑定当前执行 ID，重复请求只产生一条取消记录。收到请求后任务仍占用运行名额。下一次续约返回 HTTP 423，Worker 停止当前工作并回传结果；进行中的 CI 保留已产生的日志和检查回执。控制面收到回执后标记 CANCELLED。若 Worker 已失联，按原过期租约核对流程结算。Owner 取消本地任务不会关闭来源 Issue。

Issue 的实现回合结束后，Worker 在进入 CI 前登记固定的候选提交。此时工作台显示候选 SHA 和“暂停 CI”。暂停请求绑定该次执行，Worker 回报或 Owner 核对过期租约后，任务进入 PAUSED，释放执行名额。在任务页选择“恢复 CI”会为同一候选创建新的验证执行及 Worktree，重新运行检查。原运行及日志保留；新运行仍须通过验证和原有验收门禁。也可取消已暂停的任务。旧 Worker 没有候选检查点时，页面只提供已支持的执行控制。

来源 Issue 已关闭时，运行中的执行保留租约身份以便提交最终回执。Worker 下一次续约收到 HTTP 410 后取消当前执行上下文；有资源配额的节点此时也不能再申请下一阶段的名额。任务详情按顺序展示取消请求和结算事件。失联时沿用租约过期后的 Owner 核对流程。

## 7. 第一版并发策略

- 不同项目允许并行；
- 同项目最多一个写入型 Work Item；
- 同项目允许额外只读审查；
- 同一模块多个写入型 Agent 不允许；
- 等待人工回答的任务释放活动模型执行名额；
- 资源不足时普通任务排队；
- 紧急暂停和取消优先。

控制面在领取事务中先按节点串行核对已占名额，再锁定项目、重新检查任务状态并创建执行。来自同一节点的并发请求共用容量。名额满时返回 204，工作项留在 READY。控制面已收到完成结果或 Owner 已结束失联执行时，名额可重新分配。执行已结束、正在等待验收或答复的任务不会占用节点的执行名额。

支持阶段配额的 Worker 在运行模型或候选验证之前，还会凭执行租约向 `POST /internal/executions/{id}/resource` 申请对应名额，提交 `lease_token`、递增的 `step` 和 `kind`（model 或 ci）。收到 204 后才进入该阶段；429 表示这一类名额已满。完成时用同一步骤并带 `release: true` 释放。已释放的步骤不能再次获取名额，迟到的旧释放请求不能释放新阶段。[节点容量配置](14-deployment-and-operations.md#5-资源管理)

## 8. 审批

Management Agent 审核计划，Policy Engine 检查权限。大型计划是否需要 Owner 决策取决于决策等级，不是仅按任务大小决定。
