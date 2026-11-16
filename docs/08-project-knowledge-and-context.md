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

### 已实现的项目历史查询

Owner 通过 `GET /projects/{id}/history` 检索某一项目。按项目授权的节点使用 `GET /internal/projects/{id}/history`。两者支持相同的查询参数。

| 参数 | 用途 |
| --- | --- |
| `q` | 可选。PostgreSQL 全文搜索，同一字段内也支持不区分大小写的子串匹配，可查中文片段。最多 512 字节 |
| `source` | 可选。选择 `requirement`（Issue 需求版本）或 `comment`（评论版本） |
| `task_id` | 可选。限制到保存原始来源的任务，例如 Goal 的父任务 |

结果最多 30 条，按相关度和记录时间排序。每条带项目、任务、来源 ID、版本、观察时间、原文链接和前 500 字符。需求的版本是修订号，评论的版本是不可变的采集事件 ID。`latest` 表示最近观察到的来源版本；其他版本标为 `historical`。这些状态只说明来源的新旧，正式决策另有确认流程。

节点令牌必须显式获准查询该项目。使用单个兼容 Worker 令牌的部署要为节点配置项目授权后才能查询历史。

### 项目资料、档案和环境说明

Owner 用 `PUT /projects/{id}/materials/{key}` 导入 UTF-8 文本，正文上限为 256 KiB。传入 `previous_revision`、`kind`、`title`、`source` 和 `content`。首次使用修订 0，后续传入要更新的修订号。类型可选 `material`（导入资料）、`profile`（项目背景、负责人等）和 `environment`（机器和环境说明）。凭据值存入密钥管理服务。

Owner 从 `GET /projects/{id}/materials/{key}` 读取最新版，附加 `?revision=1` 可读固定版本。每版保留原文、来源、导入时间和基于 UTF-8 原文的 SHA-256。重报相同版本和内容会返回原记录；对已存在的修订提交不同内容返回 409。

列表入口 `GET /projects/{id}/materials` 可按 `kind` 筛选。传入 `q` 返回相关度最高的 30 条当前版本记录和摘要；不传关键词时按 key 浏览，用 `after=<末项 key>` 翻页。项目节点使用同样的读取路径，在前面加 `/internal`，只允许访问授权项目。项目工程文档仍由 Agent 在对应 Git 版本按需读取。

## 7. 角色上下文

Management、Planner、Development、Review、Test、Operations 获取不同上下文和工具权限。

## 8. Knowledge Curator

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
