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

Owner 用 `PUT /projects/{id}/materials/{key}` 导入 UTF-8 文本，正文上限为 256 KiB。传入 `previous_revision`、`kind`、`title`、`source` 和 `content`。首次使用修订 0，后续传入要更新的修订号。类型可选 `material`（导入资料）、`profile`（项目背景、负责人等）和 `environment`（机器和环境说明）。凭据值存入密钥管理服务。

Owner 从 `GET /projects/{id}/materials/{key}` 读取最新版，附加 `?revision=1` 可读固定版本。每版保留原文、来源、导入时间和基于 UTF-8 原文的 SHA-256。重报相同版本和内容会返回原记录；对已存在的修订提交不同内容返回 409。

列表入口 `GET /projects/{id}/materials` 可按 `kind` 筛选。传入 `q` 返回相关度最高的 30 条当前版本记录和摘要；不传关键词时按 key 浏览，用 `after=<末项 key>` 翻页。项目节点使用同样的读取路径，在前面加 `/internal`，只允许访问授权项目。项目工程文档仍由 Agent 在对应 Git 版本按需读取。

## 7. 角色上下文

Management、Planner、Development、Review、Test、Operations 获取不同上下文和工具权限。

执行 Goal 子工作项时，控制面在领取事务中把 `goal_context` 随任务交给 Worker。其中包含 Goal ID、工作项 key、绑定的计划修订、来源需求的固定版本，以及本项直接前置任务的 PR 和合并 SHA。实现回合可据此核对原文中的细节或引用已合并的交付。可修改的范围来自分配给该子项的规格。Worker 把这些来源作为数据传给 Codex 或 Claude 的实现回合；后续评论检查点与同次执行的会话恢复继续使用领取时的数据。

## 8. Knowledge Curator

### 导入资料后提取项目经验

每次导入项目资料、档案或环境说明都会按项目、资料标识和修订号生成一条提取记录。升级前已有的资料修订也可领取。专用 `worker curator` 在独立只读 Codex 会话中提取最多八条技术观察或项目经验，每条附原文引句。控制面逐条核对引句确实出现在该修订正文中。没有足够证据时，记录空结果。

有建议时，`worker management` 在另一个只读会话核对原文、适用条件和建议。只有当前修订的 D1 批准建议进入本项目的经验列表。退回、需要 Owner 处理和较高等级的建议保留审核回执。业务决策、架构规则及跨项目共享仍通过相应的确认入口。新修订导入后，经验列表立即停止引用旧版；旧版的建议、引句、审核理由和会话可从任务工作台的“项目接入 → 项目资料 → 知识提取与审核”回读。

Owner 可读取 `GET /projects/{id}/materials/{key}/revisions/{revision}/curation`，核对提取与审核时间、两次会话及固定资料摘要。Owner 的 `GET /projects/{id}/experience` 与项目节点的 `GET /internal/projects/{id}/experience` 返回可用的项目经验，每页最多 30 条，以最后一项 ID 作为 `before` 读取下一页。[角色启动方法](local-development.md#启动知识提取角色)

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
