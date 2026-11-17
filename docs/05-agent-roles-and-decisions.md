# 05 — Agent 角色、决策等级与协作协议

## 1. 角色

| 角色 | 职责 |
|---|---|
| Owner | 最终目标、重要业务决策、高风险审批 |
| Management Agent | 日常管理、问题回答、计划审查、Agent 协调 |
| Planner Agent | Discovery、任务拆分、依赖规划 |
| Worker Agent | 开发、审查、测试、调查等具体工作 |
| Knowledge Curator | 知识提取、整理和更新建议 |
| Orchestrator | 调度、状态、队列、会话和恢复 |
| Policy Engine | 强制权限校验 |

## 2. 决策等级

| 等级 | 处理者 | 示例 |
|---|---|---|
| D0 | Worker 自主 | 命名、局部代码组织、按规范补测试 |
| D1 | Management Agent | 沿用已确认架构、低风险计划批准 |
| D2 | Management Agent 建议，Owner 决定 | 业务规则、核心 API、重要数据模型 |
| D3 | Owner 强制审批 | 生产凭据、破坏性操作、不可逆变更 |

等级不能由 Agent 自行降低。Policy Engine 根据操作对象和环境执行硬性规则。

## 3. Management Agent 主要职责

- 检查 Goal 和 Work Item 状态；
- 回答有明确证据的普通问题；
- 审核 Planner 计划；
- 处理 Worker 决策请求；
- 将无法解决的问题汇总给 Owner；
- 检查交付证据；
- 协调任务阻塞和依赖；
- 调用 Knowledge Curator 处理知识更新。

Management Agent 默认不直接修改项目代码、不直接执行任意 Shell、不持有生产密钥。

## 4. 决策证据

重要决策必须记录：
- 决策 ID；
- 请求者；
- 决策者；
- 任务和项目；
- 决策等级；
- 问题和答案；
- 证据来源；
- 使用的规则；
- 时间；
- 是否替代旧决策。

Owner 可在工作台的“待答问题”跨项目查看处于 WAITING_INPUT 的候选。每项显示问题、依据、项目、任务、执行号和候选 SHA。填写要处理的答复后点击“提交已填答复”。每题独立记账，成功项回到原候选的 WAITING_ACCEPTANCE，空白项继续等待。响应丢失可保留草稿并重试；若另一会话已写入不同答复，页面展示该答复，保留本次草稿供核对。任务详情可查看原问题、答复、处理角色和时间。

Owner 的 `GET /tasks/decision-questions` 按问题编号倒序返回最多 20 项，使用 `next_before` 作为下一页的 `before` 参数。批量操作沿用每题的 `POST /tasks/{id}/questions/{question}/answer` 与验收检查点。

### Owner 审批中心

打开工作台的“审批中心”，可跨项目查看规划中的 Goal 最新修订、待确认且仍有效的项目决策，以及 Management 已同意、等待 Owner 发布的脱敏共享副本。可按操作筛选。计划行显示修订及决策等级，共享行显示来源修订、接收项目和副本 SHA-256。进入对应记录核对依据，并在原详情页批准、拒绝或发布。刷新列表后，已处理的记录离开队列。

Owner 接口 `GET /tasks/approvals` 每页最多返回 30 项，按操作分组、同组按记录编号倒序排列。`kind` 可选 `plan`、`decision` 或 `share`；读下一页时沿用原筛选，把 `next_before` 原样传给 `before`。候选代码的人工验收继续从“验收队列”进入。

## 5. Agent 内部通信

内部协议建议统一 JSON Schema，消息类型包括：
- EVENT；
- COMMAND；
- REQUEST；
- RESPONSE；
- RESULT。

所有 Agent 消息经过 Orchestrator / 消息路由层。Agent 不直接绕过中台修改正式任务状态。

消息至少包含：
- schema_version；
- message_id；
- correlation_id；
- project_id；
- goal_id / work_item_id；
- execution_id；
- from / to；
- message_type；
- payload；
- timestamp；
- idempotency_key；
- trust_level。

Issue 评论、附件和导入文档都是不可信输入，不得成为系统级指令。

## 6. 知识来源优先级

普通规则冲突：
`用户当前明确指令 > 项目规则 > 全局默认规则`

规划与实现回合使用领取任务时仍有效的全局修订。实现子项同时取得计划引用的有效项目决策，退回修改时还会取得 Owner 对该候选的明确验收意见。工作台的执行记录可对照同一次领取所用的全局版本与项目决策。

但以下硬约束不可由普通提示词覆盖：
- 身份认证；
- Policy Engine；
- 密钥访问边界；
- 强制审批；
- 禁止操作。

临时指令仅作用于指定任务范围，不自动变成长期项目规则。
