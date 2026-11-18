# 10 — 代码审查与自动化验证

## 1. 职责分离

- 项目仓库声明如何验证；
- 中台决定本次执行哪些验证和允许的权限；
- Local CI Runner 在本地执行；
- Review Agent 独立只读审查；
- Validation Aggregator 汇总结果。

## 2. validation.yaml

建议包含：
- checks：单项检查；
- profiles：检查组合；
- command：数组形式的可执行程序和参数；
- timeout；
- working directory；
- environment profile；
- report/artifact 定义；
- 必需或可选标识。

仓库配置不能授予自身更高权限。验证配置修改需独立审查，敏感能力由 Policy Engine 控制。

版本 1 的机器可读合同见[正式 Schema](../schemas/validation.schema.json)，项目可从[现成示例](../schemas/validation.yaml.example)起步。将 `validation.yaml` 提交到项目根目录。每项检查声明类型、参数数组和 1–3600 秒超时，可声明相对工作目录、报告和产物文件。环境为 `ci`，可省略；`required` 默认 true。未知字段、重复键、越界路径和未定义的检查引用会导致整份配置无效。

Owner 通过 `GET /projects/{id}/validation-plan?commit=<完整 SHA>&level=V3` 检查固定版本。获该项目授权的节点使用相同路径前加 `/internal`。返回的 `config_blob` 是实际读取的 Git blob。控制面按等级累加各 Profile 的检查并去重。以下是可判定计划齐备所需的检查类别。

| 等级 | 目标 Profile | 本级新增必需类型 |
| --- | --- | --- |
| V1 | fast | static |
| V2 | standard | unit、build |
| V3 | integration | integration |
| V4 | critical | security、regression |

可选检查保留在计划中；只有必需检查计入覆盖。目标 Profile 或必需类别缺失时，响应 `ready=false` 并逐项列出 `missing`。这是配置检查结果。实际检查的退出码、报告和交付判定由执行流程记录。

## 3. 验证等级

- V1：格式、Lint、类型检查；
- V2：V1 + 单元测试 + 构建；
- V3：V2 + 集成测试/E2E；
- V4：V3 + 安全检查 + 关键业务回归。

最终等级由任务类型、Git Diff、敏感模块、项目规则和交付目标共同决定。Management Agent 可提高等级，不能降低 Policy Engine 最低要求。

查询项目的 `validation-assessment` 可以在执行前预估固定候选的等级。可提供 `minimum`、`task_type`、`target`、`management_minimum`。结果给出各项最低等级、具体依据、匹配组件、基础和候选 SHA、规则 blob 及最终最大值。查询参数用于评估，不会修改任务。

内置路径规则将文档置于 V1、常规代码置于 V2，integration、e2e、.github、deploy 路径片段置于 V3，auth、authentication、authorization、payment、payments、secrets、migrations 片段和根目录的验证或验收配置置于 V4。组件规则与项目整体最低等级可以进一步提高要求。任务类型 documentation、change、integration、security 分别对应 V1 至 V4；目标 local、pull_request、staging 分别最低 V1、V2、V3。

原生 Management Worker 审核自动分级任务的计划时可在计划修订回执中提交 `validation_minimum`，并写明理由和依据。Owner 可在任务详情查看建议。批准并激活该修订时，控制面在同一事务内将下限应用到 Goal 和尚未执行的子项。后续修订只能提高已生效的下限。已领取或已有执行记录的子项保留原先的策略和回执。D1–D3 决策审批等级与 V1–V4 验证下限分别保存。

## 4. Review Engine

- 独立 Session；
- 固定 Commit；
- 只读工作区；
- 不允许自行修改代码；
- 输出 Finding ID、Severity、File/Line、Description、Evidence、Recommendation、Confidence、Blocking。

严重等级：Critical、High、Medium、Low。

LLM Finding 必须核对证据，不能只凭模型标签自动认定真实漏洞。

本地入口为 `worker review-candidate`。用非 root 审查账号设置 `FORGEOS_REPO_ROOT` 和原生 Claude Code 的 `FORGEOS_CLAUDE_BIN`，通过标准输入传入一个 JSON 对象。

```json
{"repo":"example","base_commit":"<完整基础 SHA>","commit":"<完整候选 SHA>","requirement":"队列达到上限时拒绝新增工作。"}
```

Worker 从授权仓库的 Git 对象读取变更文件的前后内容和模式，基础必须是候选的祖先。每次审查启动独立的 Claude 会话，向其提供固定输入，关闭文件、命令、网络工具和 MCP；审查进程在临时目录工作。输出包括两个 SHA、原生 Session、摘要和发现列表。每条发现绑定 base 或 candidate 的文件与一基行号。Worker 逐条核对原文行，`evidence_verified` 表示引用是否匹配。技术结论仍需结合上下文判断。

当前文本审查入口支持最多 128 个变更路径、前后文本合计 192 KiB。二进制、非普通文件或超限输入会明确报错，保留人工审查。命令输出可独立留档。

### 让审查报告参与候选验收

控制面设置 `FORGEOS_REVIEWER_PROJECT_ID` 和专用 `FORGEOS_REVIEWER_TOKEN`。审查节点设置相同的 Reviewer 令牌、控制面回环地址、仓库根目录和 Claude 可执行文件，运行 `worker reviewer`。开发 Worker、Owner、Management 与 Reviewer 的 API 凭据各自独立。审查节点和控制面均需能读取所登记仓库内的候选提交。

| 操作 | Owner 接口与结果 |
| --- | --- |
| 为项目启用必需审查 | `PUT /projects/{id}/review-policy`，请求 `{"required":true}`。最新候选通过本地验证后进入审查。已有项目默认沿用其原验收方式。 |
| 单独请求或重试失败的审查 | `POST /tasks/{id}/code-review`，提交 `execution_id`、`commit`。相同候选复用请求，失败作业可以重新领取。重复请求保留已完成的报告。 |
| 查看执行对应的回执 | `GET /tasks/{id}/code-review?execution_id=…`。回执包含基础和候选 SHA、请求和完成时间、报告或失败时间。Worker 中断后租约六分钟到期，其他 Reviewer 可重新领取。 |
| 核对发现 | 控制面重新读取固定 Git 对象并逐项校对引用。若仍有阻塞发现或引用未核实，Owner 可退回候选，或在核对后向 `POST /tasks/{id}/code-review/check` 提交 `execution_id`、`commit`、`note`，记录判断依据。随后按原验收入口提交决定。 |

控制面可把该项目 Reviewer 纳入共享 Claude 账号预算。正式审查节点声明支持控制面签发的租约截止时间，并以该时间约束本次调用；已配置预算的项目等待支持此能力的节点。配置与查看占用见[部署与运维](14-deployment-and-operations.md#多个执行节点共用一个模型账号)。

项目已启用审查或 Owner 单独发起审查时，缺少报告、审查失败或发现尚待核对，批准请求返回 409。退回后产生的新执行需要自己的审查。Reviewer 无权批准或交付候选。

在任务详情展开对应的 Worker 执行，可请求或重试独立审查。“查看审查记录”在运行中及失败时也可打开，展示 Reviewer 原生进程和最近公开消息，成功后再显示两个固定 SHA、审查会话、摘要和逐项证据。Owner 可以填写发现核对说明，或在候选验收中说明退回原因。页面与批准接口使用同一门禁状态。历史执行保留报告和 Owner 说明。新 Reviewer 在报告中记录 Claude Code 原生审批模式和工具列表。Owner 可展开“Reviewer 权限配置”核对；空列表对应原生参数 `--tools ""`。旧角色及旧控制面完成的审查沿用原报告格式。

有候选规则依据时，在报告内展开“审查使用的候选依据”，可核对实现回合领取的全局修订、项目决定以及本次修改所回应的 Owner 退回意见。后续发布新默认值或使项目决定失效后仍能复核该候选。专用 Reviewer 领取时声明 `supports_candidate_context`。需要这些依据的任务会等待支持它的 Reviewer；旧执行已经完成的审查保留原回执。 目标分支前移后选择重新验证时，报告列出的审查 SHA 仍是本次重验的候选。规则依据另列原实现执行号和提交。多次换基通过每次已归档的交付、任务、需求版本、基础和候选 SHA 追溯到同一原实现。

## 5. Local CI Runner

从非 root Worker 运行 `worker validate-candidate`。设置 `FORGEOS_REPO_ROOT`、`FORGEOS_WORKSPACE_ROOT`、`FORGEOS_WORKER_ID`；`FORGEOS_CI_IMAGE` 可选择本机已缓存的 Linux 镜像，默认 `golang:1.25-alpine`。镜像需提供标准 shell、sleep、cp、rm、tar 和项目声明的工具。向标准输入写入一个 JSON 对象，例如：

```json
{"repo":"example","base_commit":"<基础提交全 SHA>","commit":"<候选全 SHA>","level":"V2"}
```

Runner 读取基础提交的配置，在候选文件的隔离副本中运行。每个检查各用一个无网络、非 root、只读根文件系统的容器。临时项目副本支持编译与生成报告。Runner 将本地镜像固定到内容 ID，把使用者声明的参数数组直接交给容器执行。工作目录相对于项目根，报告和产物路径相对于本检查的工作目录。

每个检查容器的上限为 2 个 CPU、2 GiB 内存和 256 个进程。检查超时可从运行回执的 `TIMEOUT` 和空退出码判断。临时验证 Worktree 随本次运行释放。[普通 Linux 用户执行的三个候选](acceptance.md#ci-resource-worktree)

命令输出 JSON，必需检查未通过时返回非零。工作区根下的 `ci-run-*/run.json` 保存运行身份、配置版本及 blob、候选 SHA、等级、镜像、时间、各检查退出码、状态和文件引用。超时检查的退出码为 null。每项日志保留前 1 MiB，发生截断时另加一行提示。检查的 `log_digest` 保存最终日志文件的字节数和 SHA-256，空日志也有摘要。每个输出文件上限 8 MiB，每次运行最多收集 64 MiB。输出记录包含来源、大小和 SHA-256。缺少承诺的报告或产物会令该检查失败。每次重跑使用新的运行目录。

在工作台的“逐项检查”可核对日志文件名、字节数及摘要，点击文件名旁的下载按钮即可取回日志、报告和产物。Owner API 为 `GET /tasks/{task}/executions/{execution}/ci/{run}/{file}`，其中 `file` 取回执内的归档文件名。返回下载附件，每次发送前核对大小与 SHA-256。任务、执行、运行号或文件名不匹配时返回 404，已登记的文件暂缺或损坏时返回 503。历史日志缺少摘要时只显示原有文件名。

实现 Worker 进入 CI 前向控制面登记候选提交。任务页可暂停这次 CI，查看原检查的取消回执、日志和已经产生的文件，再对同一提交恢复完整验证。两次运行各有独立 Execution 与运行编号。工作台操作见[专题 06 的命令队列](06-orchestration-and-planning.md#6-命令队列)。

支持集中归档的控制面确认执行结果后，Worker 在清除待报正文的同一 SQLite 事务中登记待传文件。后台独立传输，失败的文件在 30 秒后可重试。控制面只保存与该执行回执完全一致的字节；具名 Worker 的项目与执行授权同样适用于上传。旧控制面或旧 Worker 仍可上报原有验证结果。需要补传历史文件或修复损坏副本时，使用该节点的 Worker 凭据向上述路径前加 `/internal`，以 PUT 发送原文件字节。成功和重传均返回 204。备份或恢复本地运行目录时，用独立回执调用 `worker verify-ci-run`。[CI 归档恢复步骤](14-deployment-and-operations.md#核对恢复的-ci-运行目录)

从正常 Issue 入口指定 `{"validation":"auto","task_type":"documentation","target":"pull_request"}` 可按候选变化选级。项目基础提交包含 validation.yaml 或 acceptance.yaml 时，显式指定 V1–V4、docs 或 go-test 也会启用风险评估。指定等级作为最低要求，docs 和 go-test 分别对应 V1 和 V2。GitHub Issue 以 PR 为最低交付目标，可指定 staging 提高要求。没有仓库配置且按原方式请求 docs 或 go-test 的任务继续使用原检查。手动导入与按标签轮询采用相同规则，Goal 子项继承策略，修订或重新验证会针对新的 SHA 重新计算。

自动分级任务要求 Worker 同时声明 `supports_declared_checks` 和 `supports_risk_policy`。回执保存选级依据、命中组件、配置 blob 和所执行的等级。独立 Runner 也接受 `"level":"auto"` 及可选的 `policy` 对象。每个检查有独立超时，整个 Issue 执行仍受 Worker 的 20 分钟预算约束。

控制面在有效执行租约内核对 Runner、执行、基础和候选 SHA，按任务的冻结策略独立复算完整风险判定，重新读取必需检查清单，并对照每项状态、输出引用、大小和摘要格式。持久化成功后，重复上报核对已保存回执。候选改动 `validation.yaml` 或 `acceptance.yaml` 时会同时排队独立代码审查，即使项目没有启用全项目审查门禁。Owner 在页面的“逐项检查”展开选级依据及运行记录，查看候选差异，再批准或退回。

接口建议：
- create_run；
- get_run；
- get_logs；
- get_artifacts；
- cancel_run；
- retry_run。

结果必须包含：
- Commit SHA；
- validation 配置版本；
- Profile；
- 每个 Check 的状态和退出码；
- 日志和报告引用；
- Runner 身份；
- 开始结束时间。

### 运行中读取检查输出

在任务详情展开正在执行的尝试，“实时 CI 日志”显示当前声明检查或内置 go-test 的名称、运行 ID、候选 SHA、节点日志字节数及最近的完整行。Worker 每秒采样，任务详情沿用 8 秒刷新。控制面核对节点、执行租约、固定候选和检查顺序，只保存当前检查最多 8 KiB 的显示文本。已识别的凭据行、私钥块和控制字符行替换为 `[redacted]`。没有新行时可继续核对运行 ID 和字节数。短检查可能在下一次采样前完成。

控制面重启后仍可回读活动租约的最近样本。暂停并恢复时，新执行使用新的 CI 运行 ID。结束后实时样本清除；声明检查的完整记录和可下载原始日志沿用项目归档策略。

没有 `validation.yaml` 的项目选择内置 `go-test` 时，检查名为 `go-test`。节点创建本次执行的 `.go-test.log` 后开始采样。Go 可能按测试包缓冲输出，页面只显示已经写出的完整行。

支持内置日志归档的 Worker 与控制面会为每次运行分配独立 ID。候选验证结束后，在任务详情展开对应执行的「逐项检查」即可下载 `go-test.log`。成功和失败的日志分别绑定候选 SHA、执行号、退出码、字节数及 SHA-256。节点在写入归档副本前逐行脱敏，最多保留 1 MiB 加截断标记。已上传的副本可在节点离线时下载；上传、保留期限和恢复校验沿用本节 CI 文件流程。未启动容器检查且未打开日志时，回执仅保留原错误。

工作区相邻的 `.go-test.log` 仍是节点私有诊断文件，按工作区管理。旧组件组合继续使用按执行编号生成的实时运行 ID 和原验证摘要，历史执行仅对已登记的归档开放下载。[失败、修订、交付和恢复实测](acceptance.md#builtin-go-archive)

## 6. 失败分类

- CODE_FAILURE；
- BUILD_FAILURE；
- TEST_ENV_FAILURE；
- INFRA_FAILURE；
- TIMEOUT；
- CANCELLED（执行上下文已取消）；
- CONFIG_FAILURE；
- SECURITY_FAILURE。

基础设施故障不应被当成代码缺陷。

关闭来源 Issue 时，已开始的容器检查会随 Worker 中断。运行和被中断的检查记录 CANCELLED，该检查的退出码为空。已完成的检查、候选 SHA 和写入的日志仍可按运行回执读取。

## 7. 验证规则

- 每个结果绑定固定 Commit SHA；
- 代码变化后重新执行受影响检查；
- 所有必需检查通过才能形成有效结论；
- 自动修复轮数有限，建议初始最多两轮；
- 测试脚本不能自行降低验证要求；
- 生产密钥不注入普通 CI。

### 候选审查与测试使用的 Owner 指令

Reviewer 领取候选、Owner 创建候选测试计划时，控制面从原实现执行读取已确认的下一回合指令。每批记录处理检查点、原生会话及顺序排列的请求键和正文。候选测试计划的依据摘要包含这些记录，Management 使用同一版资料审核覆盖情况。Owner 可从任务详情分别展开审查报告和各版测试计划，核对当时的依据。升级前已归档的报告和计划保留其原始内容。

候选测试计划的进程观察分别关联 Test Agent 生成和 Management 覆盖审核。工作台保存各版原生会话、公开消息与实际退出；成功回报核对进程会话和退出码后，仍执行固定候选、原始标准、独立角色及计划摘要校验。covered 仅表示拟定用例覆盖标准，用例执行记录仍须由实际测试产生。被替代修订的在途进程可以完成退出回报，但不能发布过时结果。
