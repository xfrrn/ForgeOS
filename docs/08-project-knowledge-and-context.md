# 08 — 项目知识与上下文服务

## 1. 两类事实来源

### Git 仓库：工程事实来源

- 架构；
- 模块设计；
- 开发规范；
- API；
- 数据库；
- 测试策略；
- 部署说明；
- 正式工程 ADR。

Agent 在执行时自主读取当前任务所需文档，不由中台预先复制整个仓库进入上下文。

### 中台：运营和长期知识来源

- 项目背景；
- 负责人和协作者；
- 机器和环境信息；
- 历史讨论；
- 用户导入资料；
- 跨会话经验；
- 决策索引；
- 任务和执行历史。

## 2. 知识分层

1. Global Knowledge；
2. Project Private；
3. Shared Experience（经过脱敏和审核的共享副本）。

项目知识默认私有。跨项目共享不能直接开放原始聊天或原始项目资料。

### 发布一份脱敏副本

Owner 在“项目接入 → 项目资料”打开要共享的具体修订，选择“起草脱敏副本”，填写接收项目编号、脱敏标题和正文。提交后会打开该副本的审核记录。通过项目页的“共享经验”可以翻阅发出的副本、刷新审核结果、按审核摘要发布或撤销。在接收项目选择“接收的经验”可核对已发布内容。

源项目 Owner 通过 `GET /projects/{id}/shares` 分页查询发出的副本，按 ID 倒序每页最多 30 项，下一页传 `before=<末项ID>`。单个审核回执及接收项目的读取接口如下。

1. Owner 从源项目向 `POST /projects/{id}/shares` 提交 `material_key`、固定 `revision`、目标项目 `recipient`、脱敏后的 `title` 和 `content`。完全照搬原文的请求返回 409。返回的 ID 绑定来源版本、接收项目和不可改写的副本。相同请求重用 ID。
2. 在源项目配置的 `worker management` 读取原版本和拟共享内容，在独立只读会话中审查其依据、是否含有私有标识或敏感细节。Owner 通过 `GET /projects/{id}/shares/{share}` 查看结论和理由。`revise` 时可重新提交修正版，`needs_owner` 时先澄清不确定的披露范围。
3. 审核为 `approve` 后，Owner 向 `POST /projects/{id}/shares/{share}/publication` 提交 `{"action":"publish","digest":"<审核过的副本摘要>"}`。目标项目的 `GET /projects/{id}/shared-experience` 开始返回副本，节点使用对应的 `/internal` 读取入口。只提供标题、正文、摘要、共享 ID 和发布时间。源资料、内部引用及审核笔记保留在源项目。
4. Owner 可用相同摘要和 `action=revoke` 撤销。撤销后查询不到该副本，原审批不能再次发布。更新源资料时，已有副本仍绑定原修订；发布新内容须重新审核和批准。

## 3. 知识类别

- 全局规则；
- 项目知识；
- 项目决策；
- 对话与事件档案；
- 导入资料。

历史聊天不自动成为正式规则。

## 4. 决策状态

- PROPOSED；
- CONFIRMED；
- REJECTED；
- SUPERSEDED；
- EXPIRED。

重要决策包含 source、decided_by、validity、supersedes、evidence_refs。

在工作台的“项目接入”打开目标项目的“项目决策”。默认展示当前有效的已确认决定。Owner 可提出带来源、适用条件和证据的决定，确认或拒绝提案，对已确认决定提议替代或标记失效。“证据与历史”展示原始引用、操作人及时间。可按五种状态或全部状态浏览，加载后续页。页面中已加载的有效决定到截止时间会重新读取服务端状态。

可用 `python scripts/probe_project_decisions.py <控制面地址> <测试项目ID>` 从两个 Chrome 会话复验提案回执丢失、竞争替代、拒绝及失效。探针使用 `FORGEOS_OWNER_TOKEN`，保留决策历史。

Owner 向 `POST /projects/{id}/decisions` 提交提案。必填字段为 `request_key`、`statement`、`source`、`validity` 和非空的 `evidence_refs` 数组。可选的 `valid_until` 为带时区的 RFC 3339 时间，`supersedes` 为本项目已有决策的 ID。相同 key 和内容重报返回同一提案；同一 key 改写内容返回 409。

通过 `POST /projects/{id}/decisions/{decision}/status` 提交 `{"status":"CONFIRMED"}` 或 `REJECTED`。确认替代提案时，旧决定必须仍然有效，旧记录变为 SUPERSEDED 与新确认在同一事务完成。Owner 可将有效决定置为 EXPIRED。有截止时间的待确认或已确认记录到期自动显示 EXPIRED。每条记录保留有关操作的身份、状态、时间和原始依据。截止时间自然到期可由 `valid_until` 核对。

Owner 用 `GET /projects/{id}/decisions` 查询，项目节点用 `GET /internal/projects/{id}/decisions` 查询。默认只返回当前有效的已确认决定。使用 `status=ALL` 或一个状态名查历史。按 ID 倒序每页最多 30 条，下一页传 `before=<本页最后一个 ID>`。

## 5. 检索方式

混合检索：
- 结构化查询；
- PostgreSQL 全文关键词搜索；
- pgvector 语义检索。

先做权限过滤，再做检索和排序。结果携带来源、项目、状态、时间、版本和引用位置。

## 6. Agent 项目查询 API

建议能力：
- project.get_profile；
- project.get_environment；
- project.search_docs；
- project.get_decisions；
- project.search_history；
- project.get_task_context；
- project.get_dependencies；
- project.get_secret_status（只返回状态，不返回明文）。

可通过受控 API 或 MCP 暴露。

### 查询工作项的任务上下文与依赖

项目节点用 `GET /internal/projects/{project}/tasks/{task}/context`，Owner 用 `GET /projects/{project}/tasks/{task}/context`。一次查询返回同一数据库快照中的工作项状态、固定基础提交、执行配置 Git blob、工作项自己的 Issue 规格及其绑定的来源需求。子任务的 `issue` 描述本项工作，`requirement.issue` 保存来源 Issue 在该需求修订时的原文。

子任务的 `plan` 取其绑定修订；Goal 本身的 `plan` 取最新修订。两者的 `active_revision` 均指 Goal 当前生效的版本。通过 `plan_key` 可在原计划找到子任务的范围、验收条件与上下文引用。`dependencies` 列出当前直接前置任务的 ID、key、状态及已记录的 PR 地址、合并 SHA。已移除工作项的原依赖仍可在绑定计划正文中追溯。独立任务没有前置项时返回空数组，缺失的需求或计划返回 null。

节点需持有该项目的显式授权。没有项目权限返回 403；路径中的项目与任务不匹配返回 404。

### 已实现的项目历史查询

Owner 通过 `GET /projects/{id}/history` 检索某一项目。按项目授权的节点使用 `GET /internal/projects/{id}/history`。两者支持相同的查询参数。

| 参数 | 用途 |
| --- | --- |
| `q` | 可选。PostgreSQL 全文搜索，同一字段内也支持不区分大小写的子串匹配，可查中文片段。最多 512 字节 |
| `source` | 可选。支持 `requirement`（来源需求）、`comment`（评论）、`merge`（已确认的 PR 合并）或 `goal`（已交付的 Goal） |
| `task_id` | 可选。按记录所属任务筛选。子项 PR 属于子工作项，Goal 关闭记录属于父任务 |

结果最多 30 条，按相关度和记录时间排序。每条带项目、任务、来源 ID、版本、观察时间、原文链接和前 500 字符。需求的版本是修订号，评论的版本是不可变的采集事件 ID。`latest` 表示最近观察到的来源版本；其他版本标为 `historical`。这些状态只说明来源的新旧，正式决策另有确认流程。

用 `source=merge&q=<候选或合并 SHA>` 可定位实际 PR。结果以 PR 编号为来源 ID、以中台确认合并的事件 ID 为版本；`delivery` 给出执行号、候选 SHA、合并 SHA 和目标分支。`source=goal` 可按计划目标或 Issue 标题检索，结果链接到父 Issue，来源 ID 为 Goal 任务号，版本为完成时的生效计划修订。合并或父项交付写入正式事件后，历史查询即可读到记录，已有项目的旧回执也可直接检索。

节点令牌必须显式获准查询该项目。使用单个兼容 Worker 令牌的部署要为节点配置项目授权后才能查询历史。

### 项目资料、档案和环境说明

在工作台的“项目接入”选择对应项目的“项目资料”，填写标识、类型、标题、来源和正文即可导入。搜索可组合关键词与类型；不填关键词时可继续加载后续资料。打开资料可查看正文、时间、来源和摘要，逐版回读。两位 Owner 同时编辑时，后提交者的草稿保留在当前页面，并显示服务器上的修订。核对后可明确选择新修订作为编辑基础。

在已登记的测试项目中，用已配置的 `FORGEOS_OWNER_TOKEN` 运行 `python scripts/probe_project_materials.py <控制面地址> <项目ID>`。脚本使用 Playwright 和 Chrome，创建一条独立资料并核对两份浏览器会话的冲突与历史回读。

Owner 用 `PUT /projects/{id}/materials/{key}` 导入 UTF-8 文本，正文上限为 256 KiB。传入 `previous_revision`、`kind`、`title`、`source` 和 `content`。首次使用修订 0，后续传入要更新的修订号。类型可选 `material`（导入资料）、`profile`（项目背景、负责人等）、`environment`（机器和环境说明）和 `memory`（Owner 明确要求记住）。凭据值存入密钥管理服务。

Owner 从 `GET /projects/{id}/materials/{key}` 读取最新版，附加 `?revision=1` 可读固定版本。每版保留原文、来源、导入时间和基于 UTF-8 原文的 SHA-256。重报相同版本和内容会返回原记录；对已存在的修订提交不同内容返回 409。

列表入口 `GET /projects/{id}/materials` 可按 `kind` 筛选。传入 `q` 返回相关度最高的 30 条当前版本记录和摘要；不传关键词时按 key 浏览，用 `after=<末项 key>` 翻页。项目节点使用同样的读取路径，在前面加 `/internal`，只允许访问授权项目。项目工程文档仍由 Agent 在对应 Git 版本按需读取。

## 7. 角色上下文

Management、Planner、Development、Review、Test、Operations 获取不同上下文和工具权限。

执行 Goal 子工作项时，控制面在领取事务中把 `goal_context` 随任务交给 Worker。其中包含 Goal ID、工作项 key、绑定的计划修订、来源需求的固定版本，以及本项直接前置任务的 PR 和合并 SHA。实现回合可据此核对原文中的细节或引用已合并的交付。可修改的范围来自分配给该子项的规格。Worker 把这些来源作为数据传给 Codex 或 Claude 的实现回合；后续评论检查点与同次执行的会话恢复继续使用领取时的数据。

### 在计划里引用已确认决策

在项目决策页复制“计划引用”（例如 `/projects/1/decisions/7`），粘贴到计划编辑器对应子项的“上下文引用”，每行一项。仓库路径可继续写在同一字段。领取实现工作时，控制面只解析该子项绑定的计划修订，按引用顺序返回本项目当前有效的已确认决定，重复引用只出现一次。每条包含决定 ID、原文、适用条件、确认时间、截止时间、来源和引用路径。

项目任务上下文接口的 `decisions` 展示查询时仍然有效的决定。工作台的执行记录可展开“领取时引用的项目决策”，核对该次派发时的原文。控制面在领取事务中保存 `decision_context`，Codex 和 Claude 从同一份 `goal_context.decisions` 获取依据。失效或替代会影响后续派发；已派发回合和历史执行保留原依据。升级前没有采集这一字段的执行保留原状。

## 8. Knowledge Curator

### 导入资料后提取项目经验

每次导入项目资料、档案或环境说明都会按项目、资料标识和修订号生成一条提取记录。升级前已有的资料修订也可领取。专用 `worker curator` 在独立只读 Codex 会话中提取最多八条技术观察或项目经验，每条附原文引句。来源明确请求 Owner 对具体业务或架构提案作决定时，Curator 可以按待确认提案摘录。控制面逐条核对引句确实出现在该修订正文中。没有足够证据时，记录空结果。

有建议时，`worker management` 在另一个只读会话核对原文、适用条件和建议。只有当前修订的 D1 批准建议进入本项目的经验列表。退回、需要 Owner 处理和较高等级的建议保留审核回执。业务决策、架构规则及跨项目共享仍通过相应的确认入口。新修订导入后，经验列表立即停止引用旧版；旧版的建议、引句、审核理由和会话可从任务工作台的“项目接入 → 项目资料 → 知识提取与审核”回读。

Owner 可读取 `GET /projects/{id}/materials/{key}/revisions/{revision}/curation`，核对提取与审核时间、两次会话及固定资料摘要。Owner 的 `GET /projects/{id}/experience` 与项目节点的 `GET /internal/projects/{id}/experience` 返回可用的项目经验，每页最多 30 条，以最后一项 ID 作为 `before` 读取下一页。[角色启动方法](local-development.md#启动知识提取角色)

在“项目接入 → 项目资料”选择“记住一条项目经验”，填写标题、来源与观察原文，提交记忆请求。工作台为草稿生成可重复使用的资料标识。保存回执丢失时可直接重试；后续补充或更正可在同一标识下编辑新修订。通过类型“Owner 记忆请求”可筛选这些来源。Curator 和 Management 按固定修订提取、审核，Owner 可在“知识提取与审核”追溯结果。

### 从已确认决策提取经验

在“项目决策”中打开决策编号，选择“知识提取与审核”。Owner 确认决定时，控制面按确认事件生成一次提取任务。重复确认使用原记录。升级前确认过的决定也可领取，包括后来被替代或失效的历史决定。只提出或拒绝提案不会触发。

来源类型为 `confirmed_decision`，修订号取确认事件 ID。供逐字核对的正文由决定原文、适用条件和可选的 UTC 截止时间组成。回执显示确认事件号与正文的 SHA-256。领取内容另含原始出处、证据引用和确认身份。Curator 从中摘录有证据的技术观察；单纯的业务规定可以产生空结果。引句必须连续出现在正文中，角色发现引句不匹配时会在同一租约内重新提取一次。

D1 审核通过且源决定当前有效时，项目经验列表返回 `decision_id`、来源类型、修订、摘要及 `receipt_path`。决定被替代、手动失效或自然到期后，派生经验退出当前列表。Owner 可从 `GET /projects/{id}/decisions/{decision}/curation` 继续核对历史审核。只有完成过确认的本项目决定能查到回执。

### 从已合并 PR 提取经验

控制面在 GitHub 交付中确认合并时，以 `pr_merged` 事件号登记提取任务。等待必需检查的 PR 继续停留在交付队列。来源正文保存 PR 编号、目标分支、候选和合并 SHA，以及 Owner 对该候选的验收说明。Curator 可据此摘录有原文支持的观察；交付元数据本身不会自动发布为经验。旧库中可与批准记录和交付回执核对的合并事件也能领取。

在工作项的任务时间线展开“PR 已合并 → 知识提取与审核”，可查看来源、两个 SHA、正文摘要、提取引句和独立审核会话。Owner 的回执路径为 `GET /projects/{id}/tasks/{task}/events/{event}/curation`。审核通过后，本项目的经验列表提供相同的事件号和回执路径。

### 从已交付 Goal 提取经验

父 Issue 完成交付后，控制面在记录 `goal_delivered` 的同一事务内保存生效计划修订、父 Issue 地址和已交付子项。每项列出候选、基线和合并提交。只有子项实际基线等于声明的前置子项合并 SHA 时，来源才会注明这项依赖关系。滚动计划移除的子项不进入完成回执。

在父任务时间线展开“全部子项已交付，父 Issue 已关闭 → 知识提取与审核”，可核对各子项、原始交付和两次角色会话。该回执沿用任务事件查询入口。升级时，已有的完成事件从已保存的计划、执行和交付记录补入队列。

### 从已提交的仓库文档变更提取经验

控制面每 30 秒检查一次已登记项目的本地 Git HEAD。工作台的“检查当前 HEAD”也会尝试同步；“项目资料 → 仓库文档变更 → 同步当前提交”可立即执行一次。首次观察保存提交与文档路径、blob 的对应关系。后续观察按项目、路径对照两个提交，新增和修改的正文进入 Curator 队列。如果上次扫描的提交位于当前 HEAD 的第一父提交链上，控制面从已存位置顺序回放中间提交，包含随后撤回的文档。每 32 个提交保存一次进度，重启从已保存的位置继续。合并以文档进入所观察分支的版本为准。分支分叉或旧 Git 对象已不可用时，以已保存的文档树和当前提交做一次对照。

扫描范围包括入口文档、Markdown（含 MDX）、reStructuredText、AsciiDoc 和 `docs/` 下的文本文件。控制面读取 Git 中的普通文件，排除符号链接、子模块以及 `vendor/`、`node_modules/`。空文档、删除和非文本变化保留记录。单个正文超过 256 KiB 时记录大小限制，提取保持待处理。任一路径发生新变更时，旧版经验退出当前列表；原审核与固定引句仍可回读。

Owner 通过 `GET /projects/{id}/document-changes` 查看扫描提交和每页最多 30 条变更，下一页传 `before=<末项编号>`。同一路径的 POST 会先同步当前提交。可提取的变更提供 `GET /projects/{id}/document-changes/{change}/curation`，其中含原始正文、提交、路径、前后 blob、正文 SHA-256 和角色回执。已批准的当前经验沿用本项目的经验查询入口。

### 从审核建议提出正式决策

审核结果为 `needs_owner`，或 D2、D3 的 `approve` 时，Owner 可在“知识提取与审核”中选定一条建议，编辑拟采用的决定、填写适用条件，并保存为待确认提案。打开对应的项目决策可查看证据、确认或拒绝。可从同一条原始建议另提修正版；已拒绝的提案和原始审核继续保留。仅已确认且有效的决定进入项目默认决策列表。跨项目发布和环境操作沿用各自的审批入口。

Owner 向 `POST /projects/{id}/decisions` 提交 `request_key`、`statement`、`validity` 和 `curation: {"id": 提取回执编号, "fact": 从 1 开始的建议序号}`。控制面核对项目、审核结论和序号，从原始回执写入固定修订路径、建议引用、正文摘要及审核会话。相同 key 和载荷可重试。历史审核回执的 `decisions` 列出每条建议对应的提案编号和当前状态。Owner 可从 `GET /projects/{id}/decisions/{decision}` 读取单项，获准的项目节点使用对应的 `/internal` 入口。

触发：
- 决策确认；
- Goal 完成；
- PR 合并；
- 用户导入资料；
- 文档变化；
- 用户明确要求记住。

权限：
- 普通元数据、索引、统计：自动更新；
- 项目经验和技术发现：Management Agent 审核；
- 重要业务决策、架构规则、跨项目共享：必须有明确确认和相应审批。

## 9. 仓库版本绑定

每次任务记录读取的项目 Base Commit 和关键文档版本。正在执行任务发现主分支变化时，在安全检查点评估是否需要刷新上下文。
