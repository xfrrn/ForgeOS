# 13 — 数据存储、事件与审计

## 1. 存储职责

- PostgreSQL：业务实体、状态、关系、索引、审计元数据。
- pgvector：知识语义检索（可后置）。
- OpenBao：真实密钥。
- Git：代码、工程文档、重要计划快照和 ADR。
- 本地文件/对象存储：日志、截图、录屏、报告、构建产物。
- Worker SQLite：断线期间的事件缓冲和本地恢复日志。

## 2. 事件记录

事件建议采用追加式记录，至少包含：
- event_id；
- sequence；
- project_id；
- task_id；
- execution_id；
- session_id；
- event_type；
- timestamp；
- actor；
- payload；
- source；
- correlation_id。

重复事件通过唯一键去重。

## 3. 审计

审计记录：
- 谁发起；
- 谁批准；
- 哪个对象；
- 执行什么操作；
- 使用什么策略；
- 结果是什么；
- 对应证据和版本。

密钥明文、CLI Token、密码不得进入日志。

## 4. 追溯链

`Issue → Task/Goal → Plan Revision → Work Item → Execution → Session/Workspace → Commit → Review/CI → Acceptance → PR → Merge SHA`

## 5. 文件存储

大文件不直接放 PostgreSQL。数据库保存：
- artifact_id；
- storage_key/path；
- MIME；
- size；
- checksum；
- source task/execution；
- retention policy；
- access scope。

## 6. 保留策略

- 重要决策和任务元数据长期保留；
- 原始日志按可配置期限保留；
- 大体积录屏和构建产物按项目策略保留；
- 敏感材料脱敏；
- 归档不等于无限期保存所有原始输出。
