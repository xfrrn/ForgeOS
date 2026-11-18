# 工作台任务事件验收对照

2026-11-19 对照任务详情时间线的 44 个具名事件，逐项核对 Owner 下载入口的回执。每行列出一份实际样本。不同验收库会重复使用任务号和事件号，可按回执文件及项目、任务、事件三个编号定位。原始回执目录为 `D:/ForgeOS-QA-Temp/fb8e915b/`。

逐份核对了事件编号、序号、项目、任务、可选执行与会话、事件类型、带时区的原始时间、操作者、来源、载荷和关联编号。有执行时关联编号为 `EXECUTION-{id}`，其余为 `TASK-{id}`。操作记录说明各事件发生时可以确认的业务字段。

| 事件类型 | 工作台预期含义 | 实测来源 / 操作者 | 项目/任务/事件 · 实际回执 | 验收记录与实现提交 |
| --- | --- | --- | --- | --- |
| `plan_revised` | 已保存计划版本 | owner | 1/1/8 · `event-audit-plan/receipts.json` | [操作与结果](acceptance.md#timeline-decisions-plans) · `2c755f3` |
| `plan_activated` | 计划已生效 | owner | 1/1/10 · `event-audit-plan/receipts.json` | [操作与结果](acceptance.md#timeline-decisions-plans) · `2c755f3` |
| `goal_item_created` | 从计划创建工作项 | owner | 1/2/9 · `event-audit-plan/receipts.json` | [操作与结果](acceptance.md#timeline-decisions-plans) · `2c755f3` |
| `dependencies_delivered` | 前置交付已确认 | controlplane / delivery_manager | 1/3/12 · `dependency-release-live/latest.json` | [操作与结果](acceptance.md#dependency-release-audit) · `c3ac432` |
| `decision_requested` | 已请求业务决定 | owner | 1/1/4 · `event-audit-plan/receipts.json` | [操作与结果](acceptance.md#timeline-decisions-plans) · `2c755f3` |
| `decision_answered` | Owner 已回答 | owner | 1/1/5 · `event-audit-plan/receipts.json` | [操作与结果](acceptance.md#timeline-decisions-plans) · `2c755f3` |
| `management_question_reviewed` | Management 已审阅业务问题 | management | 1/1/7 · `question-advice-audit/latest.json` | [操作与结果](acceptance.md#question-advice-event-audit) · `92127c3` |
| `management_decision_answered` | Management 已引用项目决策答复 | management | 1/1/4 · `question-advice-audit/latest.json` | [操作与结果](acceptance.md#question-advice-event-audit) · `92127c3` |
| `goal_delivered` | 全部子项已交付，父 Issue 已关闭 | controlplane / delivery_manager | 1/1/12 · `delivery-audit-goal/latest.json` | [操作与结果](acceptance.md#delivery-event-audit) · `8015110` |
| `goal_item_superseded` | 已从后续计划移除 | owner | 1/3/7 · `remaining-event-audit/supersede.json` | [操作与结果](acceptance.md#additional-task-events) · `2c755f3` |
| `goal_item_revised` | 已更新待执行工作项 | owner | 1/2/12 · `event-audit-plan/receipts.json` | [操作与结果](acceptance.md#timeline-decisions-plans) · `2c755f3` |
| `delivery_base_refreshed` | 已在新的目标提交上准备候选 | owner | 1/1/19 · `delivery-audit-refresh/latest.json` | [操作与结果](acceptance.md#delivery-event-audit) · `8015110` |
| `requirement_adopted` | 已采用新的需求修订 | owner | 1/1/3 · `requirement-event-live/latest.json` | [操作与结果](acceptance.md#requirement-event-audit) · `5bce041` |
| `requirement_observed` | 已记录新的 Issue 需求版本 | owner | 1/1/4 · `requirement-event-live/latest.json` | [操作与结果](acceptance.md#requirement-event-audit) · `5bce041` |
| `requirement_revised_from_feedback` | Owner 已从预览反馈修订需求 | owner | 1/1/6 · `requirement-feedback-live/latest.json` | [操作与结果](acceptance.md#requirement-event-audit) · `5bce041` |
| `feedback_work_item_dispatched` | 已从反馈派发独立工作项 | owner | 1/1/10 · `feedback-dispatch-live/latest.json` | [操作与结果](acceptance.md#feedback-dispatch-audit) · `d3e79b0` |
| `preview_feedback_received` | 已收到预览反馈 | owner | 1/1/10 · `feedback-audit-verified/latest.json` | [操作与结果](acceptance.md#feedback-event-audit) · `160605d` |
| `preview_feedback_classified` | Management 已完成反馈分类 | management | 1/1/18 · `feedback-audit-verified/latest.json` | [操作与结果](acceptance.md#feedback-event-audit) · `160605d` |
| `feedback_screenshot_saved` | 已归档预览反馈截图 | owner | 1/1/11 · `feedback-audit-verified/latest.json` | [操作与结果](acceptance.md#feedback-event-audit) · `160605d` |
| `feedback_recording_saved` | 已归档预览反馈录屏 | owner | 1/1/12 · `feedback-audit-verified/latest.json` | [操作与结果](acceptance.md#feedback-event-audit) · `160605d` |
| `management_reviewed` | Management 已审核本版计划 | management | 1/1/4 · `remaining-event-audit/plan.json` | [操作与结果](acceptance.md#legacy-planner-event) · `2c755f3` |
| `planner_completed` | 只读 Planner 已提交草案 | worker / pause-native | 1/1/3 · `remaining-event-audit/plan.json` | [操作与结果](acceptance.md#legacy-planner-event) · `13ccb3b` |
| `task_created` | 创建任务 | owner | 1/1/1 · `event-audit-base/receipts.json` | [操作与结果](acceptance.md#task-event-audit) · `2c755f3` |
| `issue_imported` | 导入 GitHub Issue | owner | 1/1/1 · `issue-import-audit-after/latest.json` | [操作与结果](acceptance.md#issue-import-audit) · `9aeaa8f` |
| `execution_claimed` | Worker 领取任务 | worker / foundation-regression | 1/1/2 · `event-audit-base/receipts.json` | [操作与结果](acceptance.md#task-event-audit) · `2c755f3` |
| `execution_completed` | 执行完成 | worker / foundation-regression | 1/1/3 · `event-audit-base/receipts.json` | [操作与结果](acceptance.md#task-event-audit) · `2c755f3` |
| `execution_failed` | 执行失败 | worker / builtin-log | 1/1/3 · `remaining-event-audit/failure.json` | [操作与结果](acceptance.md#additional-task-events) · `2c755f3` |
| `execution_abandoned` | 结束失联执行 | owner | 1/3/9 · `remaining-event-audit/abandon.json` | [操作与结果](acceptance.md#additional-task-events) · `2c755f3` |
| `source_cancel_requested` | 来源 Issue 已关闭，等待执行结算 | github / github_poller | 1/2/9 · `source-closure-live/latest.json` | [操作与结果](acceptance.md#source-closure-audit) · `404ca39` |
| `source_issue_cancelled` | 来源 Issue 已关闭，任务已取消 | github / github_poller | 1/1/8 · `source-closure-live/latest.json` | [操作与结果](acceptance.md#source-closure-audit) · `404ca39` |
| `ci_pause_requested` | Owner 请求暂停 CI | owner | 1/1/6 · `ci-pause-event/live.json` | [操作与结果](acceptance.md#ci-pause-event-audit) · `9428f07` |
| `ci_paused` | CI 已暂停 | worker / ci-pause-audit-node | 1/1/7 · `ci-pause-event/live.json` | [操作与结果](acceptance.md#ci-pause-event-audit) · `9428f07` |
| `ci_resumed` | 已对原候选重新派发 CI | owner | 1/1/8 · `ci-pause-event/live.json` | [操作与结果](acceptance.md#ci-pause-event-audit) · `9428f07` |
| `model_pause_requested` | Owner 请求暂停模型回合 | owner | 1/1/3 · `model-pause-audit/current.json` | [操作与结果](acceptance.md#model-pause-audit) · `562b666` |
| `model_paused` | Worker 已保存模型暂停检查点 | worker / pause-audit-node | 1/1/4 · `model-pause-audit/current.json` | [操作与结果](acceptance.md#model-pause-audit) · `562b666` |
| `model_resumed` | Owner 已请求恢复模型回合 | owner | 1/1/5 · `model-pause-audit/current.json` | [操作与结果](acceptance.md#model-pause-audit) · `562b666` |
| `owner_cancel_requested` | Owner 请求取消，等待执行结算 | owner | 1/1/3 · `event-audit-cancel/foundation.json` | [操作与结果](acceptance.md#task-event-audit) · `2c755f3` |
| `owner_cancelled` | 已按 Owner 请求取消任务 | worker / foundation-regression | 1/1/4 · `event-audit-cancel/foundation.json` | [操作与结果](acceptance.md#task-event-audit) · `2c755f3` |
| `execution_retry_requested` | 请求重新执行 | owner | 1/1/4 · `retry-audit-live/latest.json` | [操作与结果](acceptance.md#workbench-failed-execution-retry) · `6b56a2c` |
| `acceptance_approved` | 候选版本通过验收 | owner | 1/1/4 · `event-audit-approve/latest.json` | [操作与结果](acceptance.md#task-event-audit) · `2c755f3` |
| `acceptance_rejected` | 候选版本已退回 | owner | 1/1/4 · `event-audit-reject/latest.json` | [操作与结果](acceptance.md#task-event-audit) · `2c755f3` |
| `pr_created` | 已创建 PR | controlplane / delivery_manager | 1/1/5 · `delivery-audit-270/latest.json` | [操作与结果](acceptance.md#delivery-event-audit) · `8015110` |
| `pr_merged` | PR 已合并 | controlplane / delivery_manager | 1/1/6 · `delivery-audit-270/latest.json` | [操作与结果](acceptance.md#delivery-event-audit) · `8015110` |
| `delivery_completed` | 交付完成 | controlplane / delivery_manager | 1/1/7 · `delivery-audit-270/latest.json` | [操作与结果](acceptance.md#delivery-event-audit) · `8015110` |
