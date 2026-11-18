# 13 — 数据存储、事件与审计

## 1. 存储职责

- PostgreSQL：业务实体、状态、关系、索引、审计元数据。
- pgvector：启用项目资料混合检索时，在已授权的资料范围内计算余弦距离。`material_embeddings` 按项目、资料、修订、模型和正文位置保存分段向量。`experience_embeddings` 按审核记录、事实编号和模型缓存经验向量。检索时重新核对来源的有效性及批准等级。普通 PostgreSQL 也能迁移和备份这两张表。[配置与结果字段](08-project-knowledge-and-context.md#项目资料混合检索)
- OpenBao：真实密钥。
- Git：代码、工程文档、重要计划快照和 ADR。
- 本地文件/对象存储：日志、截图、录屏、报告、构建产物。
- Worker SQLite：断线期间的事件缓冲和本地恢复日志。

### Worker 执行恢复日志

正式 Worker 在启动任务前，将领取时间、任务号、执行号和工作区写入本地 SQLite。执行结束时，先在同一条记录保存候选、退出码、原生会话 ID 和完整待报结果，再发送给控制面。重启并完成身份登记后，Worker 先补报已有结果，再接回等待业务答复的检查点并领取新任务。补报沿用原执行租约。控制面确认后清除待报正文和租约令牌；遇到 409 时保留去除令牌的冲突结果，供维护者和控制面记录核对。

实现回合已退出且控制面确认检查点后，节点将该次领取的上下文、租约、问题、Git 树、原生会话、版本、执行时间余额和资源申请序号写入私有 SQLite 的 `parked_questions`。恢复需使用原控制面地址、节点身份、状态目录及工作区。Owner 答复后，控制面在事务中将答复交给一个恢复进程。同一进程重试可取回答复。Worker 核对现场后移除本地待恢复记录，再启动原会话的下一回合。若启动结果不明确，维护者可根据原执行和进程回执核对。仅有领取记录的任务保持人工核对。

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

从 `GET /tasks/{id}` 的执行结果读取 `processes`；也可在工作台展开对应执行下的“Harness 进程记录”。PID 的含义限定在回执中的执行节点。进程实例号和 Session 分别记录，同一 Session 可以经历多个 PID。没有采集进程数据的历史结果仍按原样展示。普通模型任务运行期间，Owner 可从同一接口的 `attempts[].observed_process` 或工作台查看节点最近上报的 PID、原生 Session 和控制面接收时间。节点本地恢复日志保留进程记录，执行结束后完整记录随最终回执归档。Codex 的每一轮记录其实际 Thread；成功回执的 Session 对应最后完成的进程。版本取自该节点所运行的 `codex --version`，保留预发布标记。

公开模型消息的脱敏窗口保存在 Worker SQLite 的 `harness_model_output(instance, sequence, text)`，每个进程最多 8 KiB。控制面在 `observed_process.output` 和最终 `result.processes[].output` 保存对应序号和正文；原始消息不写入这张节点表。输出包含候选任务的内容，因此仍按原执行的 Owner 访问边界读取。没有该字段的旧记录保持原状。该窗口用于回看最近进度，不是完整会话转录或任务成功凭据。

工具的脱敏窗口位于 harness_tool_output(instance, seq, call, tool, text)，每个进程最多 8 KiB；通过 observed_process.tool_output 和 result.processes[].tool_output 回读。对应事件仍保存在 harness_tool_events。harness_output_redaction 按租约摘要和 model/tool 类别记录未结束的私钥块状态，不保存密钥正文；该状态与脱敏输出在同一 SQLite 事务内提交。beta #200、#202 已验证 Worker 重启后从该状态继续遮蔽 Codex CLI 和 App Server 工具结果；beta #204 验证 Claude 在业务问题检查点重启、答复后同会话续读时继承该状态。App Server 增量输出也复用该表；通过同一 instance/seq 关联 harness_tool_events 的 output 状态生成 partial=true，无需更改旧 SQLite 表结构。未完成行不写入窗口，只持久化保守遮蔽状态。窗口只保存最近一次有正文的调用更新，完整工具输出的归档和到期清理继续待补齐。

在执行节点的 SQLite 中，按实例号查询完整工具事件。两个终端或会话的相同序号须结合节点与进程实例识别。

```sql
SELECT seq, call, tool, phase, datetime(at, 'unixepoch') AS observed_utc
FROM harness_tool_events WHERE instance = :instance ORDER BY seq;
```

### 独立角色作业进程日志

控制面的 management_processes 按任务与计划修订、共享副本 ID、仓库分析任务、问题 ID、预览反馈 ID、知识提取审核 ID 、候选测试计划 ID 或 Reviewer 候选执行 ID，加租约 SHA-256 保存各次作业的节点身份、最近进程窗口及接收时间；API 不暴露租约摘要。当前计划、历史修订以及来源项目的共享回执均通过 review_processes 回读，失败、过期或被新修订替代的尝试仍保留。迁移 099 将原 plan_review_processes 更名并加入互斥的副本身份，迁移 100 加入同样互斥的 analysis_task，迁移 101 再加入 question_id，迁移 102 加入 feedback_id，迁移 103 加入 curation_id、curation_role 和 role_turn，迁移 104 加入 test_plan_id 和 test_plan_role 并外键关联对应角色作业，迁移 105 加入 reviewer_execution 并关联原 code_reviews 记录；每条记录只能关联一种来源。既有记录原样保留，已对真实三版计划和两份共享历史升级回读。仓库分析接口通过 processes 返回全部尝试，重试不删除失败进程；归纳结果仍核对首次归档的固定来源。初始化报告的版本快照保存分析结果，进程历史由原任务的仓库分析入口回读。问题的 management_processes 同时出现在待答队列与任务历史，回答后仍由任务回读，不嵌入建议或 Owner 答复。已回答问题的在途进程在租约有效期内可上报实际退出。反馈页在各条观察下通过 management_processes 回读分类尝试，租约过期重新领取不会覆盖旧进程。知识来源回执分别通过 extraction_processes、review_processes 返回尝试，同一 Curator 租约的两次引文修复不会互相覆盖。候选测试计划回执以 test_processes 和 management_processes 分别返回生成与覆盖审核历史；被替代的修订也保留进程，不将其结果发布到新修订。Reviewer 回执通过 processes 返回各个审查租约的真实 Claude 进程，失败重试不删除旧观察；进程保存在报告之外，原 Git 引文验证和 Owner 核对门禁不变。已发布的项目经验仍只返回经验字段，不附带原生进程、会话或公开消息。共享副本接收方不能读取该私有历史。公开消息沿用节点 harness_model_output 的 8 KiB 脱敏窗口，控制面拒绝旧序号、同序号改写、节点或进程身份替换及退出后修改。

独立 Curator、Management 的 SQLite 与同机普通 Worker 分开，均遵循 FORGEOS_WORKER_STATE_DIR 的私有目录要求。原生退出先记入 SQLite，再上报控制面；控制面不可达且最终回执未确认时不会提交审核结论。该角色尚无 Daemon 重启后的自动补报及原会话恢复，节点记录可用于核对。

### 本地环境启动日志

启动检查复用 `executions.live_ci_log`，以执行租约、已获 CI 名额、来源清单的固定提交、环境名和 `ci-run-<execution>` 绑定。结束后该临时窗口清除，`result.environment_run.output` 保留脱敏字节数和最近 8 KiB；最终回执不得删除或回退已观察日志。旧控制面未声明 `report_live_ci_log` 时新 Worker 沿用旧协议，旧 Worker 不补造输出。

节点在写入私有 `startup.log` 前处理完整行、已知凭据、轮换后的节点令牌和跨块私钥。未完成行最多缓冲 64 KiB，超长行丢弃并保守遮蔽到明确的私钥结束标记；结束时安全处理未换行尾部。单行脱敏窗口最多 8 KiB，文件上限为 1 MiB 加截断标记；这里的字节数是实际脱敏文件长度，不是原始输出长度。Docker 创建后直接附着启动，立即失败也保留诊断；清理完成再关闭日志文件。CLI 的 `verify-environment` 同样写入日志和 `run.json`。这些有界文件尚无自动中央上传和到期清理，不代表完整无限日志归档。

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

**Worker 本地 CI 文件。** Owner 通过 `GET /projects/{id}/local-ci-retention` 读取本地副本策略，用 `PUT` 整体替换。字段与中央 CI 策略相同，按执行结果的确认时间计算。默认各类文件无限期保留。两份策略分别设置；例如只提交 `{"artifact_seconds":86400}`，该项目的本地构建产物保留一天，本地日志和报告继续保留。

控制面记录到期文件和当时的策略修订。Worker 启动时及每分钟按节点与项目授权领取待清理文件，每页 32 项。它检查 SQLite 中对应的任务、执行和上传队列，核对本地 `run.json` 的运行号、执行号、节点及文件摘要。待传文件、内容不匹配的文件和符号链接留在原处，可修复后重试。清理前重新向中央提交内容，收到 204 或中央策略的 410 后才移除本地常规文件。运行回执、未声明的文件和其他工作区不在清理范围内。

离线期间到期的文件在节点重新连接后处理。延长期限仍保留已经到期的记录。Worker 启动及每 24 小时复查已确认的清理项，处理从旧备份恢复后重新出现的文件。PostgreSQL 可用 `SELECT execution_id,file,local_revision,local_expired_at,local_removed_at FROM ci_archive_files WHERE local_expired_at IS NOT NULL` 查询策略修订、首次到期与节点首次确认时间。确认时间为空的项会继续重试。还原 Worker 状态时应同时还原同一控制面地址及节点身份的 SQLite 与工作区。

预览反馈的 PNG 存放在控制面账号私有目录。可用绝对路径 `FORGEOS_ARTIFACT_ROOT` 指定，默认是该账号 HOME 下的 `.forgeos/artifacts`。PostgreSQL 保存反馈编号、文件键、类型、大小和 SHA-256，经原反馈关联任务、执行和候选。Owner 从原反馈读取文件时会重新校验长度和摘要，损坏返回 503。在保留期内重传同一份已脱敏图片可修复文件并复用原回执。

Owner 可用 `GET /projects/{id}/screenshot-retention` 查询反馈截图的中央保留期，用 `PUT` 提交 `{"seconds":2592000}` 设为 30 天，或提交 `{"seconds":0}` 设为无限期。默认无限期。期限从每张截图首次归档起算；改变策略会对该项目仍有效的历史截图重新计算到期时间。已到期截图的读取和同反馈同图重传返回 410。工作台的反馈历史显示“已到期”，并保留附件编号、字节数和 SHA-256。控制面启动和每分钟扫描到期 PNG；`feedback_artifacts.expires_at` 记录期限，`removed_at` 记录中央文件清理完成时间。

反馈录屏共用附件目录和读取时的 SHA-256 校验。Owner 通过 `GET /projects/{id}/recording-retention` 读策略，用 `PUT` 提交 `{"seconds":86400}` 可将本项目录屏保留一天。默认无限期；期限从附件初次归档起算。已到期的录屏保留摘要、反馈编号与策略修订，读取或重传返回 410。控制面在启动和每分钟清理过期文件。截图与录屏各自采用对应的项目期限。

Owner 可用 `GET /projects/{id}/retention-history` 追查中央及节点文件的保留期限。结果按修订编号降序，每页最多 100 条。用 `?before=<本页最后一个 id>` 读取更早的记录。每次有效变更在设置策略的事务内记录项目、范围（`ci`、`local_ci`、`feedback_screenshot` 或 `feedback_recording`）、角色、前后配置和时间；重复提交相同配置复用已有修订。角色 `owner` 表示持 Owner 身份提交，当前凭据体系只区分角色。

文件到期的 410 响应可包含 `retention_revision`，与 `ci_archive_files` 或 `feedback_artifacts` 中的同名字段对应。通过该编号可以找到让文件到期的策略；后来更改期限仍保留原关联。升级到数据库版本 37 时，已有的非默认配置以 `actor=migration` 登记一次，`previous_policy` 为空。审计启用前已到期的文件继续返回 410，响应省略未知的修订编号。

- 重要决策和任务元数据长期保留；
- 原始日志按可配置期限保留；
- 大体积录屏和构建产物按项目策略保留；
- 敏感材料脱敏；
- 归档不等于无限期保存所有原始输出。

独立角色的节点 SQLite 另有 role_receipts。主键为租约 SHA-256 与固定结果端点，保存结果摘要、待发正文、写入时间、确认时间及 HTTP 结算状态。待发正文含重放所必需的原租约，只留在节点私有目录，不进入公开消息或工作台。确认后清空正文；409 冲突记录先删除 lease 字段。摘要阻止同一租约结果被不同正文替换。启动升级仅新增此表，不改变已有原生进程记录；PostgreSQL 不增加第二套结果事实。

role_process_receipts 按本角色原生进程 instance 保存最终退出观察、写入时间、确认时间、HTTP 状态及去租约的冲突观察。补报顺序按 instance 排列，先于 role_receipts 中的最终结果；已确认后清空待发正文。新增表不会改变现存的进程历史或已结算结果。


迁移 106 新增 role_claim_receipts：以项目、角色、节点和请求编号为联合主键，保存请求摘要、分配响应原始字节及提交时间，与业务租约在同一事务提交。原始响应包含固定输入及私有租约，只通过相同角色授权的内部领取接口重放，不进入工作台公开消息。旧进程及业务结果不重写。节点 role_claim_request 只保存一个尚未确认的请求编号、路径和正文，不保存角色令牌；确认领取后删除此待发记录。完整解析 200 后，节点在同一事务中删除该请求并写入 role_assignment；204 只删除请求。

role_assignment 保存一个已领取作业的请求编号、租约摘要、原始响应、本次 Daemon 标识、原生启动标记及接收时间。原始响应包含租约和固定输入，仅留在私有节点目录；备份恢复时保持控制面地址、角色、节点及目录一致。同一 Daemon 的下一轮或已经结算的旧作业会移除该记录；换进程遇到已启动但尚无最终回执的作业时保留记录并停止。新增表不修改旧进程、退出队列或结果队列；日志旁的 .lock 文件无需删除，进程退出会释放其操作系统锁。
