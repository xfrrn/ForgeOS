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
