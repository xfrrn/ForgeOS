# 13 — 数据存储、事件与审计

## 1. 存储职责

- PostgreSQL：业务实体、状态、关系、索引、审计元数据。
- pgvector：知识语义检索（可后置）。
- OpenBao：真实密钥。
- Git：代码、工程文档、重要计划快照和 ADR。
- 本地文件/对象存储：日志、截图、录屏、报告、构建产物。
- Worker SQLite：断线期间的事件缓冲和本地恢复日志。

### Worker 执行恢复日志

正式 Worker 在启动任务前，将领取时间、任务号、执行号和工作区写入本地 SQLite。执行结束时，先在同一条记录保存候选、退出码、原生会话 ID 和完整待报结果，再发送给控制面。重启并完成身份登记后，Worker 先补报已有结果，再领取新任务。补报沿用原执行租约。控制面确认后清除待报正文和租约令牌；遇到 409 时保留去除令牌的冲突结果，供维护者和控制面记录核对。进程在写入结果前中断时，本地只有领取记录，恢复流程不会因此重跑该任务。

数据库默认位于服务用户 HOME 的 `.forgeos/worker`。可设置绝对路径 `FORGEOS_WORKER_STATE_DIR`，独立于仓库和工作区。控制面地址和 Worker ID 决定数据库文件名。重启后要沿用原身份、地址和状态目录。待补报的正文含该执行的租约令牌，目录限服务账号访问。Linux 使用 0700；Windows 选用只有服务账号和管理员可读的目录。读取本地恢复进度时，只查询所需列，例如在相应的 SQLite 文件上执行：

```sql
SELECT execution_id, task_id, workspace, claimed_at, buffered_at, settled_at,
       exit_code, candidate, session_id, outcome
FROM attempts ORDER BY claimed_at, execution_id;
```

`buffered_at` 有值、`settled_at` 为空表示仍待确认。结果冲突时 `outcome` 为 `conflict`，可在该执行行读取 `conflicting_result`。PostgreSQL 中的正式任务状态与 Owner 决定保持有效。

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

预览反馈的 PNG 存放在控制面账号私有目录。可用绝对路径 `FORGEOS_ARTIFACT_ROOT` 指定，默认是该账号 HOME 下的 `.forgeos/artifacts`。PostgreSQL 保存反馈编号、文件键、类型、大小和 SHA-256，经原反馈关联任务、执行和候选。Owner 从原反馈读取文件时会重新校验长度和摘要，损坏返回 503。重传同一份已脱敏图片可修复文件并复用原回执。截图清理期限和跨节点存储仍待验收。

- 重要决策和任务元数据长期保留；
- 原始日志按可配置期限保留；
- 大体积录屏和构建产物按项目策略保留；
- 敏感材料脱敏；
- 归档不等于无限期保存所有原始输出。
