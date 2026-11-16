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

项目已启用审查或 Owner 单独发起审查时，缺少报告、审查失败或发现尚待核对，批准请求返回 409。退回后产生的新执行需要自己的审查。Reviewer 无权批准或交付候选。

在任务详情展开对应的 Worker 执行，可请求或重试独立审查。“查看审查报告”展示两个固定 SHA、审查会话、摘要和逐项证据。Owner 可以填写发现核对说明，或在候选验收中说明退回原因。页面与批准接口使用同一门禁状态。历史执行保留报告和 Owner 说明。

## 5. Local CI Runner

从非 root Worker 运行 `worker validate-candidate`。设置 `FORGEOS_REPO_ROOT`、`FORGEOS_WORKSPACE_ROOT`、`FORGEOS_WORKER_ID`；`FORGEOS_CI_IMAGE` 可选择本机已缓存的 Linux 镜像，默认 `golang:1.25-alpine`。镜像需提供标准 shell、sleep、cp、rm、tar 和项目声明的工具。向标准输入写入一个 JSON 对象，例如：

```json
{"repo":"example","base_commit":"<基础提交全 SHA>","commit":"<候选全 SHA>","level":"V2"}
```

Runner 读取基础提交的配置，在候选文件的隔离副本中运行。每个检查各用一个无网络、非 root、只读根文件系统的容器。临时项目副本支持编译与生成报告。Runner 将本地镜像固定到内容 ID，把使用者声明的参数数组直接交给容器执行。工作目录相对于项目根，报告和产物路径相对于本检查的工作目录。

命令输出 JSON，必需检查未通过时返回非零。工作区根下的 `ci-run-*/run.json` 保存运行身份、配置版本及 blob、候选 SHA、等级、镜像、时间、各检查退出码、状态和文件引用。超时检查的退出码为 null。每项日志最多保留 1 MiB，每个输出文件上限 8 MiB，每次运行最多收集 64 MiB。输出记录包含来源、大小和 SHA-256。缺少承诺的报告或产物会令该检查失败。每次重跑使用新的运行目录。

从正常 Issue 入口指定 `{"validation":"V2"}` 即可使用仓库检查。支持等级 V1–V4，手动导入、按标签轮询、退回修订和重新验证均沿用冻结的任务配置。仅声明 `supports_declared_checks` 的 Worker 可领取。每个检查有独立超时，整个 Issue 执行仍受 Worker 的 20 分钟预算约束。

控制面在有效执行租约内核对 Runner、执行、基础和候选 SHA，从仓库重新读取必需检查清单，并对照每项状态、输出引用、大小和摘要格式。持久化成功后，重复上报核对已保存回执。候选改动 `validation.yaml` 时会同时排队独立代码审查，即使项目没有启用全项目审查门禁。Owner 在页面的“逐项检查”展开运行记录并查看候选差异，再批准或退回。

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

## 6. 失败分类

- CODE_FAILURE；
- BUILD_FAILURE；
- TEST_ENV_FAILURE；
- INFRA_FAILURE；
- TIMEOUT；
- CONFIG_FAILURE；
- SECURITY_FAILURE。

基础设施故障不应被当成代码缺陷。

## 7. 验证规则

- 每个结果绑定固定 Commit SHA；
- 代码变化后重新执行受影响检查；
- 所有必需检查通过才能形成有效结论；
- 自动修复轮数有限，建议初始最多两轮；
- 测试脚本不能自行降低验证要求；
- 生产密钥不注入普通 CI。
