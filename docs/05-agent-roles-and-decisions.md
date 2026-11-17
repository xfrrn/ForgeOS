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

Owner 可在工作台的“待答问题”跨项目查看处于 WAITING_INPUT 的任务。实现中的问题显示执行号、原生回合的检查点编号和基础 SHA，答复后原 Execution 回到 RUNNING。验收时提出的问题显示候选 SHA，答复后回到该候选的 WAITING_ACCEPTANCE。每题均显示业务问题和依据。填写要处理的答复后点击“提交已填答复”，空白项继续等待。响应丢失可保留草稿并重试。若另一会话已写入不同答复，页面展示该答复，保留本次草稿供核对。任务详情可查看原问题、答复、提问者和时间。

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

消息格式 1.0 对以上五类消息共用同一份 [JSON Schema](../schemas/agent-message.schema.json)。项目、消息、关联标识与幂等键为字符串。Goal、工作项和执行三个字段必须存在，无对应对象时填 `null`。发送方和接收方各有 `type`、`id`；时间使用带时区的日期时间，载荷结构由具体操作定义。

Owner 可用 `GET /tasks/{id}/questions/{question}/messages` 回读原生实现检查点的决策请求。等待答复时有一条 REQUEST，答复后多一条 RESPONSE。控制面从持久记录生成身份、UTC 时间、对象范围和消息 ID，校验格式后返回。请求的 `trust_level` 为 `agent_generated`，接收方为 Owner；Owner 答复的来源为 `owner_authenticated`，引用原请求并发回提问节点。重新读取同一问题得到相同的关联标识和幂等键。

支持 ACP 的控制面在原生实现任务的领取结果中提供 `decision_protocol: "1.0"` 和本地项目 ID。Worker 在原执行的检查点问题接口请求 `?acp=1.0`。等待期间收到空消息数组，取得答复的进程收到与 Owner 回读一致的 REQUEST 和 RESPONSE。Worker 校验格式、项目、Goal、工作项、执行、检查点、提问节点、Git 依据和答复者，并核对两条消息的关联标识。校验通过后，原生回合的 `owner_clarification` 同时包含问题、答复、请求 ID、响应 ID 和关联 ID。已按 1.0 领取的执行收到缺少消息的答复时会报告失败。旧版控制面签发的任务沿用原有答复格式。

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
