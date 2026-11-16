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

`buffered_at` 有值、`settled_at` 为空表示仍待确认。结果冲突时 `outcome` 为 `conflict`，可在该执行行读取 `conflicting_result`。PostgreSQL 中的正式任务状态与 Owner 决定保持有效。更换状态磁盘时，保留源文件名及原身份，将校验过的 SQLite 快照放入新的私有状态目录。[备份及补报演练](14-deployment-and-operations.md#备份和恢复-worker-的-sqlite-日志)

查询原生会话及同一次执行经历的 Claude 或 Codex 进程：

```sql
SELECT a.execution_id, p.instance, p.harness, p.version, p.pid,
       p.session_id, p.started_at, p.exited_at, p.exit_code
FROM harness_processes p JOIN attempts a ON a.lease_hash=p.lease_hash
ORDER BY p.instance;
```

尚未记录退出时间仅表示缺少退出回执。核对正在运行的进程时还需比对节点、可执行文件与实际进程状态。

新控制面在实现及只读规划任务的领取回执中设置 `record_processes`。Worker 将本次执行的进程记录按本机实例号排序，与结果一同写入 SQLite 待报队列。控制面校验 Harness、版本、会话、退出码和时间后，在同一个完成事务中写入 PostgreSQL。重复回报须与已存回执一致。

从 `GET /tasks/{id}` 的执行结果读取 `processes`；也可在工作台展开对应执行下的“Harness 进程记录”。PID 的含义限定在回执中的执行节点。进程实例号和 Session 分别记录，同一 Session 可以经历多个 PID。没有采集进程数据的历史结果仍按原样展示。运行中尚未完成的进程，可从该节点的本地恢复日志查询。Codex 的每一轮记录其实际 Thread；成功回执的 Session 对应最后完成的进程。版本取自该节点所运行的 `codex --version`，保留预发布标记。

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

CI 日志、报告与产物的中央副本位于 `FORGEOS_ARTIFACT_ROOT/ci/<任务号>/<执行号>/<运行号>/<归档文件名>`。执行结果 JSON 保存对应的大小与 SHA-256。Worker 的 SQLite 表 `ci_uploads` 只保存待传文件引用和重试时间，确认上传后清除。可以用 `SELECT run_id,file,bytes,retry_at FROM ci_uploads` 查看积压。源文件损坏时，恢复与回执匹配的本地文件后，待传项会继续重试。已确认上传的副本可由 Owner 独立下载。

Owner 可用 `GET /projects/{id}/ci-retention` 读取项目策略，用 `PUT` 整体替换。以下示例让中央日志保留 7 天、检查报告 30 天、构建产物 1 天。

```json
{"log_seconds":604800,"report_seconds":2592000,"artifact_seconds":86400}
```

期限从控制面确认执行结果的时间起算，适用于该项目已有和以后的回执。各字段为非负整数秒，省略或设为零表示该类无限期保留，首次配置前各项均为零。缩短期限可使历史文件立即到期。到期后，Owner 下载和 Worker 补传返回 410，延长期限也不能恢复文件。Worker 据此移除待传项。任务详情仍可读取原候选、检查结果、文件大小和摘要。

控制面在启动时及每分钟清理已到期的中央文件。存储故障时保留待清理记录，下次扫描重试。运维可用 `SELECT execution_id,file,kind,confirmed_at,expired_at,removed_at FROM ci_archive_files WHERE expired_at IS NOT NULL AND removed_at IS NULL` 查询积压。备份恢复时应配套恢复 PostgreSQL 与同一时点的工件目录。Worker 工作区、离线备份及预览反馈有各自的生命周期。

预览反馈的 PNG 存放在控制面账号私有目录。可用绝对路径 `FORGEOS_ARTIFACT_ROOT` 指定，默认是该账号 HOME 下的 `.forgeos/artifacts`。PostgreSQL 保存反馈编号、文件键、类型、大小和 SHA-256，经原反馈关联任务、执行和候选。Owner 从原反馈读取文件时会重新校验长度和摘要，损坏返回 503。重传同一份已脱敏图片可修复文件并复用原回执。截图清理期限和跨节点存储仍待验收。

- 重要决策和任务元数据长期保留；
- 原始日志按可配置期限保留；
- 大体积录屏和构建产物按项目策略保留；
- 敏感材料脱敏；
- 归档不等于无限期保存所有原始输出。
