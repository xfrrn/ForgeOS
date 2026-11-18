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

启用项目专用的 `worker management` 后，Management 可审阅实现和验收阶段的待答业务问题。“待答问题”和任务详情展示决策等级、理由、引用来源、原文摘录、独立会话与审阅时间。有具体建议时另行显示。新组件可展开“Management 问题审阅进程”，核对节点、PID、原生会话、实际退出和脱敏公开消息。即使 Owner 先答复而没有接收建议，任务历史仍保留该独立回合。Owner 可采用或修改建议，也可给出自己的答复。等待期间，项目节点从原租约继续等待 Owner。

正式节点回报该次建议会话的实际权限配置，包含 Codex 沙箱、审批模式及禁用功能。在建议下展开“Management权限配置”可核对。Owner 答复和控制面重启后，配置仍随原问题保存。新旧节点按领取响应协商配置字段，历史建议保留当时的记录。[真实问题建议验收](acceptance.md)

控制面将问题依据、本次执行的冻结需求以及对应实现的项目决策快照交给只读 Management 会话。引用须逐字匹配列出的来源。已经提交 Owner 的业务决策至少为 D2，计划已定为 D3 时维持 D3。D3 审阅必须留空建议答复。租约过期可重新领取；Owner 已经答复后提交的新建议会被拒绝。已确认的审阅保留在原问题与任务时间线上。

原生实现回合查阅现有项目决策时，可单独返回 `{"forgeos_lookup_decision":1}`，数字为决策编号。控制面按执行所属项目查找仍有效的已确认记录。独立 Management 会话核对适用范围后，可按 D1 引用完整原文，控制面在提交时再次核对来源、有效期与租约。工作台保留答复者和证据。原生会话取得 `management_reference`，其中有原文、范围、来源及回执。需要新选择、缺少适用记录或受 D3 下限约束的问题继续等待 Owner。Owner 也可在 Management 提交前答复。

### Owner 审批中心

打开工作台的“审批中心”，可跨项目查看规划中的 Goal 最新修订、待确认且仍有效的项目决策，以及 Management 已同意、等待 Owner 发布的脱敏共享副本。可按操作筛选。计划行显示修订及决策等级，共享行显示实际生效的审核等级、来源修订、接收项目和副本 SHA-256。由资料提取结果提出的决策还显示来源知识的审核等级与意见；Owner 自行起草的提案没有这份审核记录。进入对应记录核对依据，并在原详情页批准、拒绝或发布。刷新列表后，已处理的记录离开队列。

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

Owner 可用 `GET /tasks/{id}/questions/{question}/messages` 回读原生实现检查点的决策请求。等待答复时有一条 REQUEST，答复后多一条 RESPONSE。控制面从持久记录生成身份、UTC 时间、对象范围和消息 ID，校验格式后返回。普通业务问题请求的 `trust_level` 为 `agent_generated`，接收方为 Owner；Owner 答复的来源为 `owner_authenticated`，引用原请求并发回提问节点。重新读取同一问题得到相同的关联标识和幂等键。

支持 ACP 的控制面在原生实现任务的领取结果中提供 `decision_protocol: "1.0"` 和本地项目 ID。Worker 在原执行的检查点问题接口请求 `?acp=1.0`。等待期间收到空消息数组，取得答复的进程收到与 Owner 回读一致的 REQUEST 和 RESPONSE。Worker 校验格式、项目、Goal、工作项、执行、检查点、提问节点、Git 依据和答复者，并核对两条消息的关联标识。校验通过后，原生回合的 `owner_clarification` 同时包含问题、答复、请求 ID、响应 ID 和关联 ID。已按 1.0 领取的执行收到缺少消息的答复时会报告失败。旧版控制面签发的任务沿用原有答复格式。

Management 领取知识审核时可协商 `curation_acp=1.0`。控制面把 Curator 的已结算提取结果路由为项目级 RESULT，以提取回执时间、知识记录编号和来源 SHA-256 绑定原始资料。Management 在启动独立原生会话前验证发送方、接收方、信任级别、项目、幂等键及逐字引用。旧节点和旧控制面仍可使用原提取格式。[真实路由与版本兼容验收](acceptance.md#curator-result-route)

## 6. 知识来源优先级

普通规则冲突：
`用户当前明确指令 > 项目规则 > 全局默认规则`

规划与实现回合使用领取任务时仍有效的全局修订。实现子项同时取得计划引用的有效项目决策，退回修改时还会取得 Owner 对该候选的明确验收意见。工作台的执行记录可对照同一次领取所用的全局版本与项目决策。

Management 领取计划审核时，控制面另行固定当时的全局规则及草案引用的本项目有效、已确认决策。每条决策列出引用它的工作项 key，供审核者按适用范围核对。审核遵循同一优先级，并将 `dependencies` 理解为中台强制执行的前项合并和交付条件。在计划修订下展开“计划审核使用的规则依据”可回读原文；之后规则修订、决策失效或新一版计划不会改写已完成审核的依据。

业务问题建议同样遵循这一优先级。Management 使用提问执行当时的全局修订、项目决策、退回意见、已处理的 Owner 指令和此前答复；运行中问题只纳入该检查点之前的依据。建议下的“建议使用的执行依据”可回读这份固定记录。D2 仍等待 Owner 答复，D3 不提供建议选项；D1 查阅仍必须核对当前有效决定和适用范围。

但以下硬约束不可由普通提示词覆盖：
- 身份认证；
- Policy Engine；
- 密钥访问边界；
- 强制审批；
- 禁止操作。

临时指令仅作用于指定任务范围，不自动变成长期项目规则。

### 下一回合的 Owner 指令

支持 ACP 指令的控制面在实现任务的领取回执中返回 `instruction_protocol: "1.0"`。Worker 读取接收检查点时请求 `commands=1.0`。每条已冻结的指令对应一条 COMMAND，动作是 `queue_next`，接收方是本次执行节点。消息时间取该批检查点建立的时间，幂等键由执行号、接收检查点和原指令键确定。普通 Issue 评论继续作为讨论上下文进入回合。

Worker 逐条校验消息格式、任务范围、接收方、Owner 身份、顺序、原文及幂等键。原生回合取得原指令和对应的 `message_id`、`correlation_id`。声明 1.0 后，缺少或不匹配的消息会中止本回合。早期控制面签发的任务沿用原指令格式。

在任务页的“查看执行检查点”中，打开某批指令的“查看指令消息”可核对编号、节点和时间。Owner 接口为 `GET /tasks/{id}/executions/{execution}/checkpoints/{sequence}/messages`，其中 sequence 是接收指令的检查点。尚未建立的检查点返回 404，已建立但没有指令时返回空数组。历史批次可以用同一接口从原始队列回读消息表示；旧 Worker 实际收到的仍是当时的协议格式。
