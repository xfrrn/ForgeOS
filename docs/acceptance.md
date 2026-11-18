# 全路线验收记录

状态按当前工作树的实测结果记录。下表中的历史运行可作交叉核对，真实环境要求仍需按当前版本复验。验收仓库的合并提交与 ForgeOS 实现提交分别记录。[18 份 ADR 验收对照](adr-acceptance.md)列出原始决策、验收入口与实现提交。[专题 16 的 28 项对照](16-open-decisions.md#requirement-status)记录真实环境、协议和产品交互的剩余条件。

## 当前验收进度

截至 beta #269、#271 的恢复验收，专题 16 的 28 项中，21 项已验收，3 项部分验收，4 项待环境验收。18 份 ADR 中 15 份已验收，3 份待外部条件。两份清单有重叠，四阶段最终端到端复验另计。下表列出这两份清单的缺口；专题 00–15 的其他要求见[专题覆盖与实现提交](#专题覆盖与实现提交)。

| 尚需推进 | 已取得的证据 | 仍缺的验收 |
| --- | --- | --- |
| B.2 · 适配器控制 | [工作台向活动 Codex 回合投递并取得原生回执](#live-workbench-turn-input)，正式 Worker 已记录 Codex 原生 Turn，并从工作台中断真实 Claude 回合。[Worker 重启后按原生历史确认工作台输入](#native-input-history-recovery)已验；原生协议追加输入、拒绝过时回合、Owner 处理 D2/D3 业务问题均已实测。[原生命令](#workbench-native-commands)与[文件补丁](#workbench-file-patches)的单次决定、执行回执及受保护分支交付均已验收。 | Claude 恢复后的完整交付（模型服务 HTTP 403，余额不足）。 |
| B.4 · 执行恢复 | 已从原工作树恢复完成的草稿并产生候选。beta #237 在确认两个原生回合前分别终止 Worker，Owner 排队指令只执行一次。[操作与结果](#completed-writer-input)。[beta #254](#queued-model-grant-recovery) 和 [#255](#first-model-grant-recovery) 分别验收下一回合和首个回合在名额回执丢失时的重启。[#256、#257](#returned-writer-recovery) 已验原生回合返回后、名额释放期间的恢复及 Owner 控制。[beta #258](#active-writer-recovery) 已验原生 Turn 登记后，首回合和排队指令回合在写入中途恢复。[beta #260](#writer-startup-recovery) 已验发送模型请求前的启动中断。[beta #261](#writer-start-confirmation) 已验请求发出后丢失回合记录的恢复与取消。[已完成回合](#writer-terminal-receipt)可从原进程的退出回执与冻结草稿恢复。 | [缺少启动或退出证据时的取消与到期](#writer-evidence-deadline)已验。[普通 Codex App Server 写入回合的存活进程接回](#supervised-writer)已验。其他适配器、独立角色及整机重启仍待验。 |
| C.3 · 进程日志与工具输出 | 普通执行和四个独立角色已从工作台下载并核对中央日志（[普通执行](#central-process-archive)、[独立角色](#role-process-central)、[Reviewer 的失败进程](#reviewer-central-log)），环境检查已验收[启动日志下载](#startup-log-download)；中文查询已能检索英文项目决策和[已发布给本项目的脱敏经验](#shared-experience-search)。六类角色的冻结依据已从[原生记录](#knowledge-role-matrix)核对。 | [MCP 正文与错误](#mcp-public-output)和[网页搜索文本结果](#web-search-public-output)已分别取得双协议回执并在工作台回放核对。[两版 CLI 的实测命令](#cli-command-output-timing)只在完成时返回正文。Claude 的工具增量输出仍待验收。 |
| A.3、A.4、A.5、A.7 及外部通知 | 本机节点、已授权的两座私库和站内通知已有验收记录。 | Linux 服务账号的模型登录和限额、受限 GitHub 身份与第二人批准、外部 tailnet 设备、Telegram／飞书测试接收方。 |
| 路线一至四最终复验 | 四个阶段分别有运行记录，见下方阶段对照。 | 其余功能完成后，从空库派发、单项目交付、复杂 Goal 到多项目隔离重新执行，核对最终提交和证据。 |

<a id="supervised-writer"></a>

### 专题 07、16 B.4 · Daemon 替换后接回存活的原生写入回合

环境为 Windows、PostgreSQL 18、正式 Worker、真实 Codex 和 390 像素宽的 Chrome。普通 Worker 启用 `FORGEOS_CODEX_TRANSPORT=app-server`、`FORGEOS_SUPERVISE_WRITERS=1`，本机容量设为一项。在模型写下随机草稿编号后记录 Daemon、独立执行进程和原生 PID。强制终止 Daemon，核对其确已退出，随后以相同节点身份和私有目录启动替代进程。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 [beta #269](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/269) 等待恢复后的候选，从任务页按 SHA 批准交付 | 原生 PID、Session、Turn 均保持相同，草稿只出现一次，必需检查通过 | Daemon 从 41792 换成 38496，执行进程为 45520，原生 PID 始终为 27652，只有一条进程记录。候选 `8647c5c75bd180fc4d64c33827c476aec3857053` 经 [PR #270](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/270) 合并为 `7c671c2c9c60628a7d092013fa472a6bb1c3eb76`。两项必需检查均成功，任务 DONE，来源 Issue CLOSED，交付评论一条。 |
| 替换 Daemon 后从工作台取消同一执行 | 原生回合收到取消，保留原进程记录，不生成候选 | Daemon 从 8120 换成 47316，原生 PID 38796、Thread 和 Turn 保持相同。工作项 CANCELLED，无候选。 |
| 在 [beta #271](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/271) 接回运行中的进程，在 Chrome 向当前回合追加随机标记，同时给第二个项目提交连通性任务 | 同一原生回合接收并仅写入一次；运行时仍占一个名额，结束后另一项目获派 | 0.147.0 的 PID 30452 保持相同，候选 `55bd21f337a3b65352ab7a9475d7a1342d600ece`。0.169.0-alpha.11.1 的 PID 48388 保持相同，候选 `e7f844234595ab9e2f82eee97d1ba3f166849e9c`。每轮仅一条进程记录；两个标记分别在各自候选出现一次，第二项目均随后 DONE。 |
| 保留 Daemon，终止持有原生管道的独立执行进程 | 确认原进程退出，从原 Session 的检查点恢复同一份草稿 | Daemon 28404 持续运行。原生 PID 从 45992 换成 28920，两个 Turn 的 Session 均为 `01a1ef4d-08c1-7e83-aff7-41a4915eb165`，候选 `da032985115c1b013a8b5b575300a9158f7b5214` 保留原草稿。 |
| 让替代 Daemon 跟踪原进程，控制面接受结果后延迟 SQLite 确认写盘，再终止执行进程 | 已缓冲回执重报并确认，既有候选保持相同，不启动另一个模型 | 原生 PID 41092，候选 `79dfabdea12c23aad9998cdb848ee0e06ccd6ccd`。原回执摘要 `5b3b83862d46be0023f2ba06e3f5f069e6035f15defd15bfc5552646990e3874`，本地结算 confirmed，跟踪记录随确认清理。 |

回执位于 `supervised-writer-{delivery,cancel,stable,steer,runner-crash,receipt}/latest.json`。交付演练中验收脚本查询 Actions 时遇到一次 EOF，控制面随脚本退出。从原 PostgreSQL 的停机副本启动正式控制面后，它认出既有 PR 和合并 SHA，补齐交付评论与关闭，Chrome 回读 DONE。其余场景在独立数据库中运行。此次接回验证限于存活的普通 Codex App Server 写入进程，整机重启仍待实测。

<a id="writer-evidence-deadline"></a>

### 专题 07、16 B.4 · 启动或退出证据不完整时处理取消与截止时间

从正常入口将私库 [beta #268](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/268) 派给 Windows Worker 和真实 Codex App Server。在各自的 PostgreSQL 18 和节点状态目录中，先用临时 SQLite 触发器延迟回执写盘，再终止原 Worker。对于启动回执，改变验收副本里待确认输入的摘要。对于退出回执，在退出码落盘前终止原 Worker。每次都重启一次 Worker，观察恢复后的同一 Daemon 能否持续处理该执行。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在节点已拒绝错误启动摘要后，从 Chrome 点击取消 | 不重发模型输入，原执行结算为取消 | 原 PID 8504，退出码未知（-1）。页面回读 Owner 请求停止，任务 CANCELLED，新增模型进程 0，无候选。 |
| 在错误摘要已被拒绝后，关闭真实来源 Issue | GitHub 轮询发现关闭，节点按原租约交回停止回执 | 原 PID 34056，任务 CANCELLED。工作台回读来源关闭原因，新增模型进程 0，无候选，GitHub Issue CLOSED。 |
| 在错误摘要的验收副本中将剩余执行时间设为 40 秒，启动替代 Worker 并等候 | 在截止前保留检查点，到期后按失败结算 | 原 PID 24620，先观察到 RUNNING，随后 Chrome 显示 `context deadline exceeded`，任务 FAILED，新增模型进程 0，无候选。 |
| 在已收到原生完成事件、仍缺少原进程退出码的副本中保留 40 秒预算，启动替代 Worker | 原生完成事件不足以交付，已消耗的时间计入预算 | 原 PID 5652，Turn `01a1ef3c-0e87-71c2-8e26-3ef9dcb81e6d`。先等候退出证明，到期后 FAILED，新增模型进程 0，无候选。 |

修复前，错误启动摘要场景中，同一 Daemon 在收到取消或越过截止时间后连续 90 秒均未结算。修复后，节点在原租约中继续核对历史和执行时限，控制面明确停止时交回结果。Windows 和非 root Ubuntu 均通过启动历史、退出码和截止时间专项。四份回执见 `ambiguous-verified-{cancel,source,deadline}/latest.json` 和 `ambiguous-terminal-expired/latest.json`。缩短预算仅发生在独立验收数据库副本中。实现提交 `5d15472d41fad287101a392041d59e786e9fb667` 经 PostgreSQL 18、Docker 全包回归，578 项具名测试及子测试通过，0 项失败、0 项跳过。报告为 `ambiguous-verified-deadline/regression.jsonl`。

<a id="cli-command-output-timing"></a>

### 专题 07、16 C.3 · 两版 Codex CLI 命令输出的到达时间

在只读临时工作区用 `codex exec --json` 运行单个 PowerShell 命令，先输出 `FORGEOS_BEGIN`，等待 9 秒，再输出 `FORGEOS_END`。禁用 MCP、网页搜索、插件和 hooks。按原生标准输出逐行记录单调时钟，对同一调用核对事件类型、退出码和标记。

| CLI 版本 | 开始事件 | 完成事件 | 中间事件与结果 |
| --- | --- | --- | --- |
| 0.147.0 | 10.771 秒 | 20.456 秒 | 开始时无正文，中间未返回命令事件。两个标记都随 completed 到达，退出码 0。 |
| 0.169.0-alpha.11.1 | 7.929 秒 | 17.352 秒 | 开始时无正文，中间未返回命令事件。两个标记都随 completed 到达，退出码 0。 |

回执为 `native-cli-command-events-147.json` 和 `native-cli-command-events.json`。首次尝试执行本地脚本退出 1，改用单条内联命令后才取得上述证据。这两次 CLI 调用可展示的正文从完成事件起算。App Server 的运行中片段另有[真实工作台验收](#app-command-delta)，Claude 工具增量仍待验证。另在相同节点用原生 Claude 的只读工具和临时文件复测，模型服务再次返回账号或额度相关错误，未取得文件回执。探针仅记录事件种类和是否包含临时标记，见 `native-claude-tool-timing.json`。

<a id="web-search-public-output"></a>

### 专题 07、16 C.3 · 核对网页搜索返回的文本结果

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在隔离的临时只读工作区，用 Codex 0.169.0-alpha.11.1 的 CLI 与 App Server 各搜索一次 RFC 8259 的标题，限定来源为 RFC Editor | 两个协议均能区分查询、操作与返回的搜索结果 | CLI 调用 `exec-b96e89a6-23b5-43c8-a1e4-b3a6b1edbb11`，App Server 调用 `exec-92032338-1c53-42d5-b893-b4c24da98261`。两次均返回 15 个 `text_result`。 |
| 在独立 PostgreSQL 18 中向普通 Worker 回放原生回执，打开 390 像素宽的 Chrome 任务页，然后核对最终回执及节点归档 | 所有标题、来源域名和摘要按原生次序展示，字段缺失时保留其余正文 | 两次工作台回放均显示首条 RFC Editor 摘要及其来源，最终工具状态 completed。导出的 15 项文本与各自原生记录逐字相同，正文均为 3338 个字符，SHA-256 均为 `75e5f9c923ac3976f127d0cf5c341482ac55371525b6cdafdbbeb1498382e6cf`。 |
| 将仅含查询的记录、未知结果类型及缺少正文的记录送入日志入口 | 保留调用事件，不生成工具输出正文 | 专项通过。查询、动作参数、完整 URL 和原生引用编号均未进入文本归档。 |

原生记录见 `native-web-search-{app,exec}.json`，工作台与节点回执见 `structured-web-replay/{app,cli}.json`。原生搜索只在验收探针中开启，工作台通过测试传输回放相同事件。产品角色按原有策略禁用网页搜索。此版本从空 PostgreSQL 18 和实际 Docker 运行 `go test -count=1 -json ./...`，共 578 项测试及子测试通过，0 项失败、0 项跳过，另外 3 个包没有测试文件。报告为 `structured-web-replay/regression.jsonl`，`go vet ./...` 与工作台生产构建均通过。

<a id="mcp-public-output"></a>

### 专题 07、16 C.3 · 核对 Codex MCP 公开输出

使用 Codex 0.169.0-alpha.11.1 在临时只读工作区调用节点的测试工具 `forgeos_receipt.sample`。测试工具无参数，固定返回两段文本、一份公开 JSON 和一条私有元数据。生产角色的 MCP 配置沿用现有权限策略。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 用原生 App Server 单次批准指定工具和空参数；用 CLI 调用声明只读的同一测试工具 | 两个适配器取得公开正文，工具声明与权限决定可区分 | 两者均返回 completed、两段有序文本及 `{"receipt":"public-receipt"}`。App Server 字段名为 `structuredContent`，CLI 为 `structured_content`。 |
| 在原生 CLI 的 `never` 策略下调用需要批准的版本 | 保留拒绝原因 | 原生状态 failed，错误消息为 `MCP tool call requires approval, but approval policy is never`。 |
| 在独立 PostgreSQL 18 库向普通 Worker 回放以上三份原生回执，从 390 像素宽的 Chrome 任务页展开“最近完成的工具输出”，再运行 `process-logs export` | 运行窗口、最终进程记录与归档正文一致；不含调用参数和私有元数据 | 两个成功窗口均依次显示两段文本及公开 JSON，拒绝窗口显示原始原因。三份归档各有三条记录，末行均为 `verified:true`。 |
| 向节点日志入口送入混合文本、助手专用块、嵌入资源、大整数和节点凭据 | 已知凭据脱敏，保留公开数据的精度，排除非公开块 | Worker 与脱敏测试通过，归档保留 `9007199254740993` 和公开文本。 |

原生输入文件为 `native-tool-app-local.json`、`native-tool-annotated-exec.json`、`native-tool-exec.json`。工作台回放程序自报版本 `0.0.1-replay`，操作记录见 `structured-tool-replay/{app,cli-positive,cli}.json`，各运行目录有截图和 JSONL 导出。此记录验证原生协议和产品日志入口，生产角色直接调用 MCP 仍受原权限策略约束。

<a id="workbench-file-patches"></a>

### 专题 05、07、16 A.1/B.2/C.5 · 工作台按确切补丁批准文件修改

2026-11-18，PostgreSQL 18、Windows 普通 Worker、真实 Codex App Server、Chrome 窄屏工作台。项目在节点配置中指定允许申请的文件名，Owner 从审批中心核对完整补丁。所有场景均额外尝试用另一份摘要批准，接口返回 409，原请求继续等待。旧版列表参数只返回已有的命令类型。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 [beta #263](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/263) 先允许只读命令、拒绝重复调用，再批准新文档的补丁 | 只有指定文件写入，候选文件与审批内容逐字一致 | 0.169.0-alpha.11.1 的工具项 `exec-1c8cd1dd-281d-4d3a-8c5b-10eb9ac06ea7` 报告 completed。补丁 SHA-256 为 `c82445dbe56b77b4268aefe20ac7b4fd886e6b765c272c6967b0b62ffd9be244`。对照 Git 文件内容（统一工作树换行）通过。 |
| 在同一 Issue 的独立执行中让目标在审批期间出现，再点击允许 | 已审阅的新增操作过时，现场文件不被覆盖 | Owner 决定为 accept，工具项 `exec-ad62987a-672e-4ee0-aff5-1bebe205c3f7` 报告 declined，现场文字保留。随后取消任务。 |
| 从审批中心取消文件补丁所在回合 | 无文件、无候选，Owner 能区分回合中断和工具完成 | 工具项 `exec-158c4eae-7880-4fb6-aae9-3ecccd5f9771` 所在回合 interrupted，未收到该工具项终态。工作台单独保存此结果；执行为 FAILED。 |
| 用 [beta #267](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/267) 在独立验收克隆中一次新增、修改、删除和移动，核对既有文件摘要并批准或拒绝 | 一个决定绑定整份补丁，候选树与各操作一致 | 0.147.0 的工具项 `exec-689ab564-3d4f-4cc1-b4b4-14532889aa70` 执行成功。候选 `abf88953562ea0e2097b871d8510266ccf99c6bb` 逐一核对了三个输出文件及两个消失的路径。0.169.0-alpha.11.1 的独立拒绝回执为 declined，新增目标未出现。测试用原始文件由验收克隆的本地种子提交提供。 |
| 从任务页批准 #263 的候选并等待 GitHub 交付 | 必需 CI 通过、保护分支合并、Issue 关闭 | 候选 `12a27e4106140c91561f1f4308ab7c6ba1248611` 经 [PR #266](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/266) 合并为 `64c07a0334923c740d7845568f15d948f2c4349c`。演练的首次失败检查重跑后，两项必需状态均 SUCCESS，任务 DONE，Issue CLOSED。 |

实现提交 `d46b0c5`。PostgreSQL 18 与 Docker 环境运行 `go test -count=1 -json ./...`，576 项测试及子测试通过，0 项失败；另有 3 个包无测试文件。工作台生产构建通过。全量结果为 `workbench-native-patch/regression.jsonl`。操作脚本为 `workbench-native-patch.py` 和 `workbench-native-multi.py`，回执为 `workbench-native-patch/{latest,stale,cancel,stable,decline}.json`。各轮运行目录保留审批与任务回读截图。独立协议探针的实现为 `8490cdd`，双版本探针结果见[原生协议验收](#native-file-approval-probe)。

<a id="native-file-approval-probe"></a>

### 专题 07、16 A.1 · 双版本原生文件补丁审批

运行 `python -B scripts/probe_codex_app_server.py --bin <原生 CLI 路径> --file-approvals`，稳定版指定 `--model gpt-5.6-sol`。在临时目录的只读 Thread 中，让原生模型逐个申请新增文件。审批前逐字核对单个目标、文件内容和工具项，且请求须无额外授权根目录。

| 操作 | 预期 | 0.147.0 与 0.169.0-alpha.11.1 实际结果 |
| --- | --- | --- |
| 允许第一次新增 | 内容与预先审阅的补丁一致 | 均收到 completed，逐字核对包含随机标记和末尾换行的文件通过。 |
| 同一个 Thread 拒绝第二次新增 | 产生新的审批请求，文件不存在 | 两版本均报告 declined，磁盘无第二个文件。 |
| 取消第三次新增 | 回合中断，第三个文件不存在 | 两版本都收到 interrupted，磁盘无第三个文件；截至回合结束均未收到该工具项的完成事件。 |

0.147.0 的 Thread 为 `01a1ef19-eeb3-7622-bb76-48f763992eb9`，0.169.0-alpha.11.1 为 `01a1ef19-e825-7853-a8a2-f01265d7e197`。各自三个工具项的原始回执在 `workbench-native-command/native-file-{147,169}.json`。新版的原生命令探针另行通过，回执 `command-probe-after-file.json`。此处验收范围为原生协议，工作台结果见[文件补丁审批](#workbench-file-patches)。

<a id="workbench-native-commands"></a>

### 专题 05、07、16 A.1/B.2/C.5 · 工作台的单次原生命令审批

2026-11-18，在 Windows 普通 Worker、Codex App Server 0.169.0-alpha.11.1、PostgreSQL 18 和 390 像素宽的 Chrome 工作台验收。私库 [beta #264](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/264) 使用项目专用的固定命令，以独占创建方式写入一个 Markdown 文件并输出 `62800913676`。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从审批中心核对命令、目录、基线、进程和回合，选“仅允许本次” | 只有对应工具项获得执行许可，原生输出可核对 | 回合 `01a1ef11-cc7e-7952-9fd9-476ea7d6ab8c` 的工具项 `exec-caef1a13-8f86-4760-9a73-bc2417c9b2b0` 报告 completed；在冷启动的原生会话历史读到对应工具项和上述输出。 |
| 同回合再次请求完全相同的命令，选择“拒绝本次” | 新调用单独等候决定，拒绝后模型可正常结束 | 新工具项 `exec-ebce4d18-524b-45bd-afcf-bac385e1c02a` 报告 declined，任务生成候选 `93c60ef813970f4f3dd73e6cdc24bd8094d13e8f`；Chrome 执行详情保留两项 Owner 决定与各自的原生回执。 |
| 在独立执行的第一条请求选择“取消回合” | 原生工具不执行，回合中断，无候选 | 工具项 `exec-ab53e25d-47ac-4c2b-b737-81b12d13478d` 的回执为 declined。原生回合 `01a1ef12-53b4-7201-82ee-6dbb6609de33` 为 interrupted，执行以 FAILED 结束，候选为空。 |
| 切换到 0.147.0 正式 Worker，在 beta #263 的第一条命令请求选择取消 | 旧版也能把决定交回等待的工具项并中断回合 | 工具项 `exec-601d3c15-d9fe-4763-8e7d-4b9ffafa85a0` 报告 declined，回合 `01a1ef18-ea8f-75f0-8fd9-d63503c9bde0` 为 interrupted，候选为空。 |
| 在受保护分支上再次运行同一 Issue，从任务页批准候选 | PR 必需检查通过、候选合并、Issue 关闭 | 候选 `09e5b8d659d66d3c3ba9aad44205869d627df869` 经 [PR #265](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/265) 合并为 `cf7377181c19636ad65277d65d0647c6be11f537`。预设失败的检查按演练重跑后，`acceptance-check` 和 `forgeos/local-ci` 均 SUCCESS。控制面为 DONE，GitHub Issue 为 CLOSED。 |

实现提交 `f39bfed36bc240a6f287111bb2ea07e2a9b610b5`。使用 PostgreSQL 18 和 Docker 执行 `go test -count=1 -json ./...`，575 项测试及子测试通过，0 项失败、0 项跳过，另有 3 个包无测试文件；工作台生产构建通过。回归原始结果为 `workbench-native-command/receipt-regression.jsonl`。验收回执保存在 `workbench-native-command/{decisions,cancel,stable-cancel,latest}.json`，每次运行目录有审批及任务记录的 Chrome 截图。原生历史可能省略被拒工具项，因此节点在运行中按原始工具项编号保存完成事件，控制面分别记录 Owner 决定和原生终态。早先的 [beta #263](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/263) 在两项命令决定后提出了独立文件补丁权限请求，执行停止。该项随后经[PR #266 的补丁审批](#workbench-file-patches)完成交付。

### 2026-11-18 · 本轮实现提交与回归

| 文档要求 | 真实入口及证据 | ForgeOS 提交 |
| --- | --- | --- |
| 专题 08 按权限、来源和版本检索项目历史 | [beta #251–253，Owner 及节点查询，远端合并与 Goal 关闭](#history-hybrid-search) | `9c6ffd037da73d6430d37ca2ede6dfbe6df1137c` |
| 专题 08 检索固定版本的工程文档并定位原文 | [从合并提交与父提交分别检索，同 Git 原文字节核对](#git-document-hybrid) | `7f0806c132f8159bca14625a6219449d77faf17c` |
| 专题 07、16 B.4 下一回合的名额恢复 | [beta #254，第二次模型分配回执丢失后重启](#queued-model-grant-recovery) | `a7ca585a1395d5f284ecd4db2ad8754f7c5d3f28` |
| 专题 07、16 B.4 首次模型启动前恢复 | [beta #255，原始分配回执丢失及停机期间取消](#first-model-grant-recovery) | `0c977d76a699ff7fe23dd2844bbe8459b526abcd` |
| 专题 07、16 B.4 保留原生回合结果 | [beta #256–257，名额释放、暂停、取消和业务答复](#returned-writer-recovery) | `94ad23ba0590e9b07e67ac6e8f83d0c39b7a5579` |
| 专题 07、16 A.1/B.4 重连活动 Codex 回合 | [私有套接字断开时原命令仍在运行](#native-active-reattach) | `9f80869e8f68737f6cd6c4e0e67afebab1997cf5` |
| 专题 16 A.1 双版本原生兼容 | [两个版本的输入、审批、中断和接回](#native-version-matrix) | `fe726b748c4ee942c5fb39024ff126bbf2397a15` |
| 专题 07、16 A.1/B.2 恢复运行中输入的回执 | [beta #250，两次独立写盘故障，补报与保留待确认](#native-input-history-recovery) | `9e0ba64249e7d936b6ecf55007de51f96b1d1bc9` |
| 专题 07、16 A.1/B.2 进程报告过期后关闭实时输入 | [beta #250，报告中断、恢复及数据库升级](#stale-turn-input) | `5b28e20d0bdeedadc701162507b1fac76a99748a` |
| 专题 07、16 B.4 普通 Worker 在写入中途恢复 | [beta #258，首回合、后续排队指令、暂停和取消](#active-writer-recovery) | `8d8ad70e8a35a91d97d2096b225108d902be63c4` |
| 专题 07、16 A.1 稳定版 App Server 的正式 Worker 交付 | [beta #258、PR #259，按节点选模型、原会话恢复、必需 CI、合并与 Issue 关闭](#stable-worker-delivery) | `7a52344c587d9f5ff984effcd1008db44b630f89` |
| 专题 07、16 B.4 发送模型任务前的节点恢复 | [beta #260，无 Session、已有 Session、后续指令、停机期间取消](#writer-startup-recovery) | `bc25a193122ad45bbebb52f460b49f71a638ff1b` |
| 专题 07、16 A.1/B.4 原生启动请求的历史核对 | [两个 Codex 版本，完成及中断回合的唯一原文](#native-start-receipts) | `1290aab3e1bb64e9f54f5ab25d2686fc2fcfb233` |
| 专题 07、16 B.4 原生启动已送达后的回合确认 | [beta #261，首回合、排队指令、取消及错误摘要](#writer-start-confirmation) | `be69180aaddb78015c050cb64b4204427cf20a6d` |
| 专题 07、16 B.4 原生回合完成后的节点恢复 | [beta #261、#257，完成草稿、业务答复、取消、草稿改变和未知退出码](#writer-terminal-receipt) | `dd83053fb855288fa44c8c2d209e7fb4e51f6b74` |
| 专题 07、14、16 C.3 独立 Reviewer 中央日志 | [在 Chrome 离线下载失败进程的日志，与节点 CLI 核对全文与摘要](#reviewer-central-log) | 实现 `06bb5fcbc11e2a5bed43193450e3a1327a8b1119`，验收 `b064477acc319d008e76b70768fbfaddebb05ef1` |
| 专题 07、16 B.4 未知退出码过期处置及恢复后交付 | [原进程退出证据、截止时间和 PR #262 的受保护分支交付](#writer-terminal-receipt) | `a6197b79f4cd2b5001b173f84d3c98725832cf5b` |
| 专题 07、09、16 A.1 按角色隔离账号 MCP 工具 | [双版本 Worker 新建与恢复 Thread、工作台答复和只读探针](#thread-mcp-isolation) | `2f82f7b8a4f9b1b740818ab91bd6e6aa7b706fe4` |
| 专题 05、07、16 A.1/B.2/C.5 · Owner 单次命令审批 | [Chrome 三种决定、原生回执和受保护 PR #265](#workbench-native-commands) | `f39bfed36bc240a6f287111bb2ea07e2a9b610b5` |
| 专题 07、16 B.4 · 启动及退出证据缺失 | [beta #268 的两种取消与两种执行到期](#writer-evidence-deadline) | `5d15472d41fad287101a392041d59e786e9fb667` |
| 专题 07、16 C.3 · 搜索文本结果 | [原生 CLI 和 App Server 各 15 项文本与归档逐字核对](#web-search-public-output) | `d892eec9c46fa458cc656be9e9df7daedee2f0a7` |
| 专题 07、16 C.3 · MCP 公开输出 | [原生 CLI 与 App Server 的成功与拒绝、工作台回放及归档](#mcp-public-output) | `cfe87cadc422c399bfee899644520c66c5c6d386` |
| 专题 05、07、16 A.1/B.2/C.5 · 文件补丁审批 | [双版本、多文件修改、过时基线与受保护 PR #266](#workbench-file-patches) | `d46b0c5` · [原生探针](#native-file-approval-probe) `8490cdd` |
| 专题 07、16 B.4 按已收到的输入回执验收原生中断 | [两版从独立进程核对中断及完整输入](#thread-mcp-isolation) | `c1ded60ddec9683e0ba5d379f3d994ce3178a761` |

在 `2f82f7b` 的产品代码上，从空 PostgreSQL 18 并启用真实 Docker 执行 `go test -count=1 -json ./...`。573 个具名测试及子测试全部通过，9 个包通过，3 个包无测试，具名测试无跳过。`go vet ./...` 通过，原始记录为 `mcp-policy-regression.jsonl`。随后仅改进了原生验证探针，并在两个受支持版本实际复测。

在 `a6197b7` 上从空 PostgreSQL 18、启用真实 Docker 执行全包回归。568 个具名测试及子测试全部通过，9 个包通过，3 个包无测试，具名测试无跳过。原始记录为 `terminal-deadline-regression.jsonl`。Go 静态检查、TypeScript 检查和工作台生产构建通过。

在 `dd83053` 的产品代码上，以独立 PostgreSQL 18 和真实 Docker 再次执行全量 Go 回归。568 个具名测试及子测试全部通过，9 个包通过，3 个包无测试。具名测试无失败和跳过，原始记录为 `terminal-receipt-regression.jsonl`，`go vet ./...` 通过。

在 `be69180` 的产品代码上，以独立 PostgreSQL 18 和真实 Docker 运行 `go test -count=1 -json ./...`，566 个具名测试及子测试全部通过，9 个包通过，3 个包无测试，具名测试无跳过。原始记录为 `receipt-start-regression.jsonl`。针对启动结果与资源释放的补充断言单独复测通过。`go vet ./...`、工作台 TypeScript 检查和生产构建通过。

在 `9e0ba64` 的产品代码上，使用独立的空 PostgreSQL 18 实例并启用真实 Docker 用例执行全量 Go 回归，557 个具名测试及子测试全部通过，9 个测试包通过，3 个包无测试，原始记录为 `input-history-regression.jsonl`。该轮补充的回执断言又单独运行通过。`go vet ./...`、TypeScript 检查和工作台生产构建通过。四阶段最终端到端复验仍待执行。

此前在 `94ad23b` 上，以空的独立 PostgreSQL 18 实例并启用 Docker 用例，执行 `go test -count=1 -json ./...`。550 个具名测试及子测试通过，9 个测试包通过，3 个包无测试，具名测试没有跳过或失败。原始输出为 `writer-return-regression.jsonl`。同时执行 `go vet ./...` 和 web 目录的 `npm run build`，类型检查及生产构建通过。四阶段最终端到端复验仍待执行。

<a id="thread-mcp-isolation"></a>

### 专题 07、09、16 A.1 · 任务 Thread 隔离账号配置的 MCP 服务

在现有登录身份上，用两个已支持版本检查原生 Thread 的工具清单；另外在独立 PostgreSQL 18 上从任务工作台运行 [beta #257](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/257)。清单核对仅记录数量和状态。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在隔离验收目录读取原生权限，应用 Thread 配置后再次读取 | 账号中的工具、资源和资源模板均不可供任务使用 | 此账号配置了 5 个服务。仅使用原有空表设置时原生清单仍有 66 个工具。逐项禁用后，0.169.0-alpha.11.1 的 5 个服务均报告 disabled；0.147.0 未提供运行状态字段。两版的工具、资源和模板均为零。 |
| 0.147.0 正式 Worker 领取任务，在完成回合后重启节点，从 Chrome 回答业务问题 | 新 Thread 和恢复的 Thread 均通过权限核对，答案进入候选 | 两个实际进程正常退出，共用 Session `01a1eefb-92b9-74b2-8537-914a561a7b18`，候选 `34e86ba2ae8da031c452793578ab0f1c43ef1c62` 的 H1 为 `Review ae2e6e56`。 |
| 0.169.0-alpha.11.1 重复同一工作台流程 | 仍在同一会话中完成指定内容 | 两个实际进程正常退出，Session `01a1eefb-8ae8-74e0-a49a-8ebd1f980e87`，候选 `a1c95dacf30d41f3ddaaf373f3c73301eb81f9cf` 的 H1 为 `Review 4ca648b7`。 |
| 两版分别执行只读探针与私有套接字重连 | 新建、恢复、运行中重连均保留只读策略和工具隔离 | 两版的重连命令均退出 0。0.147.0 在一次立即中断后未能从历史确认启动回执，独立重跑从已落盘的原文与 UUID 确认了中断。 |

工作台回执为 `mcp-policy-{147,169}-question/latest.json`。回归测试覆盖原生清单的后续页、未知服务、连接仍存活、资源残留与权限字段缺失。工作台的原生工具审批和 MCP 结构化输出分别维持原验收状态。

默认生命周期探针现等候携带指定客户端 UUID 的原生用户消息回执，再发送中断并从另一个进程回读历史。0.147.0 在 Thread `01a1eefe-c9c1-7843-87f2-f038860c8ed5` 找回中断回合 `01a1eefe-fc8e-7e53-af04-b8786e82983d`；0.169.0-alpha.11.1 在 Thread `01a1eefe-cae7-71e1-bde1-c4bd7511c979` 找回 `01a1eeff-e28b-7f60-b9a8-44eac69b0cdc`。两次均核对首回合与中断回合的唯一完整输入。另用 `--steer` 复测当前版本，过时 Turn 被拒绝，正确 Turn 接收并持久化指定输入。

<a id="writer-terminal-receipt"></a>

### 专题 07、16 B.4 · 原生模型完成后、Worker 保存返回结果前重启

在独立 PostgreSQL 18、真实 Codex App Server、Windows Worker 和 Chrome 上复用 [beta #261](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/261) 和 [beta #257](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/257)。用临时 SQLite 触发器延迟从完成事件转为最终返回记录。读取已冻结的草稿树、当前 Turn 与原进程退出码后终止 Daemon，去掉触发器，以同一身份重启 Worker。未知退出码场景另在原生进程退出记录写盘前终止 Daemon。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 恢复已写完的文档，从 Chrome 查看候选，并用 Git 读取文件 | 使用原草稿，只运行一次模型 | 原 PID 45916 退出码 0，Turn `01a1eee7-1344-7a61-aeab-b3dff1ebc929`。候选 `bd657fc717e0c1c71107ab8a01a89f0e926decf3` 与冻结文件逐字节一致（统一换行），保留草稿编号 `d97c094ec28c`。 |
| 恢复已经提出业务问题的回合，在 Chrome 填写随机标题 | 原问题保留一次，回答后从原 Session 写入指定标题 | 原 PID 33948 退出码 0。两个回合共用 Session `01a1eee7-0346-7da2-88f2-3523346bb7fb`。候选 `e8a15e4a506160b19ccc9825cc300bdb4de81766` 的 H1 为 `Review dd7cbcab`。 |
| 在节点停机时从 Chrome 取消 | 任务取消，已完成的工作不生成候选 | 任务为 `CANCELLED`，仅原 PID 49076，退出码 0。 |
| 停机时修改验收副本的草稿 | 保留检查点，避免交付外部修改 | 原 PID 37964 已退出 0，任务为 `RUNNING`，新增模型进程数为 0。 |
| 保留原生完成记录，但丢失原进程的退出码 | 成功退出仍待确认，保留检查点 | 原 PID 45104 记为 -1，任务为 `RUNNING`，新增模型进程数为 0。 |
| 同样丢失退出码，在停机的验收副本中将执行截止时间缩短为 3 秒，等待 4 秒后重启 | 按已耗尽的时限结束执行，保留原进程记录 | Chrome 显示 `context deadline exceeded`，任务为 `FAILED`。原 PID 32776 退出码为 -1，无候选，新增模型进程数为 0。 |

六份验收回执为 `terminal-final-{resume,question,cancel,edited,unknown,expired}/latest.json`。本项从已保存原生完成事件的边界起验证。Windows 与 WSL Ubuntu 均另行验证了成功、失败和未知退出码，覆盖期限内保留与过期拒绝。

从私库 beta 的受保护 `main` 分支重新派发 #261，在同一边界终止 Worker。按原草稿核对候选 `5c55a5f75fa5f383a5289eac43b6611c42736d96`，只有 `docs/interrupted-writer.md` 改动，草稿编号为 `b00485b5d2a8`。原 PID 36104 是唯一的模型进程，退出码 0。在 Chrome 按该 SHA 通过验收后，GitHub 的验收工作流如预设在首次尝试失败，复核失败步骤并重跑，`acceptance-check` 和 `forgeos/local-ci` 均成功。PR 合并为 `44b6b277db8347bcbde3741cebfbda336f0910fc`，工作项为 `DONE`，Issue 已关闭。回执为 `terminal-final-delivery/latest.json`。[beta PR #262](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/262) · [必需 CI](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44319423788)

<a id="writer-start-confirmation"></a>

### 专题 07、16 B.4 · 启动请求已送达，Worker 丢失原生 Turn 记录

使用独立 PostgreSQL 18、真实 Windows Worker、Codex App Server 和 Chrome 派发文档任务。在本次验收节点的 SQLite 临时延迟指定进程的 Turn 插入。确认待确认请求已写盘、工作树已有模型生成的草稿、进程记录仍无 Turn，然后强制终止 Worker，去掉故障并启动替代 Worker。[beta #261](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/261)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 首次写入时丢失记录，再从 Chrome 读取候选 | 从原生历史找回准确 Turn，保留原草稿编号，只补一个最终标题 | UUID `82dda415-8edd-4e14-bbbf-911b305391e5` 找回 Turn `01a1eed9-021d-7ef0-af2b-44cc515747c2`。原 PID 33224 的退出码为未知值 -1，PID 2532 在原 Session 正常退出。候选 `d174b230a25621e7d8d5191af97e1acc1f4509d5` 保留 `e7be940ecdde`。 |
| 将 Owner 指令排入下一回合，在该回合已写下标记时丢失记录 | 从原 Session 完成这条指令，先前草稿和新标记各保留一份 | 三条进程记录的退出码为 0、-1、0，Session 均为 `01a1eed8-eecd-7e21-a78c-5d66415ff970`。候选 `2d7551acda1a3dc2d9f8ff56c9720b52638fbe81` 保留草稿 `a1d2a7c41774` 和一份 Owner 标记。 |
| 停机期间从 Chrome 取消任务 | 结束执行，保留原进程观测，候选为空 | 原 PID 35020 的 Turn 未确认；任务进入 `CANCELLED`，没有第二个进程。 |
| 在独立副本中将待确认输入的摘要改错，再重启 | 无法证明原始输入，保留待确认记录 | UUID `8bd74330-1cd8-4f08-b16c-f005405f5467` 保留，状态为 `RUNNING`，新进程数为 0。 |

四份回执位于 `receipt-start-final-{resume,next,cancel,negative}/latest.json`。专项测试另覆盖不确定响应保留模型名额、明确拒绝后释放名额、其他工作目录、不同正文、旧回合和重复 UUID 的拒绝。

<a id="native-start-receipts"></a>

### 专题 07、16 A.1/B.4 · 从原生历史核对启动请求

运行 `python -B scripts/probe_codex_app_server.py --bin <原生可执行文件>`。0.147.0 另用其模型列表中的 `--model gpt-5.6-sol`。首回合与中断回合各携带不同的客户端 UUID。两个执行进程退出后，再启动一个 App Server，以 `thread/read` 检查原工作目录和历史。预期每个 UUID 只匹配一条用户消息，完整正文、回合编号和终态都与发送时一致。

| 版本 | 原生 Thread | 完成回合的请求 UUID | 中断回合的请求 UUID | 实际结果 |
| --- | --- | --- | --- | --- |
| 0.147.0 | `01a1eecf-5bab-7b90-9e60-040cf4709e44` | `d7ca2629-f5dc-4678-8984-e72c99bb77e0` | `5dfd5a56-96f5-4efe-b5b7-1069dd2c953d` | 两条均唯一，全文一致，终态分别为 completed、interrupted。 |
| 0.169.0-alpha.11.1 | `01a1eecf-5b4f-7f02-afe4-7a9fea798082` | `9b2635be-bfbb-445e-9a00-13a99aca44a9` | `7e918fe6-fc77-46c2-8695-0644e9a6f39e` | 两条均唯一，全文一致，终态分别为 completed、interrupted。 |

完整回执为 `worker-start-147.json` 和 `worker-start-169.json`。正式 Worker 的任务入口实测见[启动请求恢复](#writer-start-confirmation)。

<a id="writer-startup-recovery"></a>

### 专题 07、16 B.4 · 启动原生进程后、发送模型请求前终止 Worker

2026-11-18，从私库 Issue 派发到正式 Windows Worker 和 Codex App Server。验收程序只挂起本次任务的原生子进程，终止原 Worker，启动替代节点并观察进程数，再放行旧进程自行退出。每个场景使用独立 PostgreSQL 18、克隆、Worktree 和节点目录，成功候选从 Chrome 与 Git 逐项核对。[beta #260](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/260)

| 中断点及操作 | 预期 | 实际结果 |
| --- | --- | --- |
| 首回合已有 PID，Session 和 Turn 仍为空 | 旧进程挂起期间不能新增写入进程，旧进程退出后执行原任务 | PID 13304 退出码未知，两个原生编号均为空。替代 PID 39224 正常完成，候选 `da1051b2d1ec56015fcbb30b6e42633f672adbb1` 只含一份要求的标题与正文。 |
| 首回合的 Session 已登记，模型请求尚未发送 | 保留旧 Session 记录，只有替代进程实际启动模型回合 | PID 14228 保留 Session、没有 Turn，退出码 -1。PID 37152 的新 Session 完成唯一模型回合，候选为 `e54bc9fc11ce203773d1aecc439ba5eee8187719`。 |
| 从 Chrome 将指令排入下一回合，在第二次模型启动时终止 Worker | 第一回合的草稿与原 Session 保留，指令只由一个模型回合执行 | 三条进程记录的退出码为 0、-1、0，中间一条没有 Turn。首末进程属于同一 Session。候选 `1d9140229906b773663d5773a125cd882c9cb663` 含原句和一个 Owner 指定的标题。 |
| 首回合启动中断，停机期间从 Chrome 取消 | 替代节点结算取消，没有新的模型回合和候选 | 状态为 `CANCELLED`，只保留 PID 47260，候选为空。 |

四份回执分别为 `writer-startup-final-initial/latest.json`、`writer-startup-final-session/latest.json`、`writer-startup-final-next/latest.json` 和 `writer-startup-final-cancel/latest.json`。Windows 及非 root Ubuntu 另验收了对存活进程的等待，以及拒绝将已经登记的 Turn 当作未启动进程。消费启动记录后到确认新 Turn 之前发生的故障，继续列为待验收。

在提交 `bc25a193122ad45bbebb52f460b49f71a638ff1b` 上，使用空 PostgreSQL 18 和真实 Docker 执行 `go test -count=1 -json ./...`，562 个具名测试及子测试全部通过，9 个包通过，3 个包无测试。原始报告为 `writer-startup-committed-regression.jsonl`。另已通过 `go vet ./...`、前端类型检查和生产构建。四阶段最终端到端验收仍待执行。

<a id="active-writer-recovery"></a>

### 专题 07、16 B.4 · Worker 在普通写入回合中途退出

2026-11-18，使用独立 PostgreSQL 18、正式 Windows Worker、Codex App Server、控制面及 Chrome，从私库 Issue 派发任务。原生模型在工作树中写入一次性草稿编号后，验收程序强制终止 Worker。各场景使用独立的数据库、克隆与节点目录。[beta #258](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/258)

| 实际操作 | 预期 | 实际结果 |
| --- | --- | --- |
| 重启 Worker，等待工作台显示候选，读取候选中的文件 | 沿用原会话，保留草稿编号，只补一个验收标题 | PID 16516 的退出码记为未知值 `-1`，PID 24584 在原 Session 的新 Turn 正常退出。候选 `fdb2652d267ff54549c3933a3a863efa3cef54f2` 保留编号 `ed3fb738c475`，标题出现一次。 |
| 原 Worker 停机期间从 Chrome 暂停模型，重启后检查进程数，再从工作台恢复 | 暂停完成前只有原进程记录，Owner 恢复后才继续 | 任务进入 `PAUSED`，节点只有一条原进程记录。恢复后沿用 Session，候选 `38869851f798efc804985f7cf585b97a3b721632` 保留编号 `d7a7217fee1d`，标题出现一次。 |
| 原 Worker 停机期间从 Chrome 取消任务，再启动替代 Worker | 任务取消，没有新模型进程和候选 | 状态为 `CANCELLED`，只保留 PID 6328 的记录，候选 SHA 为空。 |
| 首回合中从 Chrome 将指令排入下一回合，第二回合写下新的编号后终止 Worker，再启动替代 Worker | 第一回合结果和第二回合的部分编辑同时保留，同一条指令只生成一个编号和一个最终标题 | 三个原生进程的退出码依次为 0、-1、0，Session 始终为 `01a1eeb6-b5e6-7cd1-b0f6-51353d154917`。候选 `ebd124eb2aa12e1b3f0896994e22022ffd8cf9b8` 同时保留 `fce08250fadb` 和 `ce21ebb9d60e`，两处标题各出现一次。 |

本机原始回执分别为 `active-writer-resume/latest.json`、`active-writer-pause/latest.json`、`active-writer-cancel/latest.json` 和 `active-writer-next/latest.json`。回执记录进程、原生 Session、Turn、草稿原文、候选与最终任务状态。原生历史缺失、版本或工作目录不符、最新回合无法确认为中断时，节点保留待恢复记录。

在 `8d8ad70` 上，以独立 PostgreSQL 18 和真实 Docker 执行全量 Go 回归，559 个具名测试及子测试全部通过，9 个测试包通过，3 个包无测试。Windows 专项观察到原进程退出后工具子进程随之退出；非 root Ubuntu 专项确认原生父进程被终止后仍能检测存活的工具进程组。原始记录为 `active-writer-regression.jsonl`。`go vet ./...`、前端类型检查和生产构建通过。

<a id="stale-turn-input"></a>

### 专题 07、16 A.1/B.2 · 原生进程报告中断时关闭实时输入

2026-11-18，在隔离 PostgreSQL 18 中运行正式控制面、Worker、Codex App Server 和 Chrome，使用 [beta #250](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/250)。代理仅中断该执行的进程报告，节点心跳、模型名额和租约继续工作。临时挂起本次测试的 Codex 进程，以便在同一个回合中核对恢复。前后使用同一份工作台构建。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 旧控制面在最近进程报告后等待 47 秒，再从 Owner 入口登记新输入 | 复现失去进程观察后仍可登记的问题 | Turn `01a1eea5-55fd-7bd0-afa7-72156d524ba9`，47.17 秒后 Chrome 仍展示输入框，API 返回 204。 |
| 新控制面在相同条件下登记新输入 | 超过 45 秒后关闭输入框，API 拒绝新登记，历史中没有新增行 | PID 38152，Turn `01a1eea3-a795-73a3-a148-4c8164740f83`，47.02 秒后返回 409。 |
| 恢复进程报告，向原回合登记新的标题，再恢复原生进程 | 入口重新出现，原模型消费输入并交付 | 同一进程在 20:31:05 和 20:32:01（北京时间）分别有中断前和恢复后的报告。候选 `ad0f3b79bb491716bcddd9373088a3a9a1cb9081` 的标题 `Harbor 3229e4b5` 恰有一次，输入记录为 accepted。 |
| 在版本 119 登记真实回合输入，停止旧控制面，使用新版原地升级同一数据库 | 既有输入逐字段相同，提交相同请求仍幂等，工作台可回读 | schema 升至 120。Turn `01a1eea6-f75d-76d2-8056-3feb3d119ca7` 的输入 `eae39aa9-5a40-4a7d-872c-fa1a857a21fd` 历史完全相同，重试返回 204，Chrome 可见原文。 |

独立复跑时，先等 Chrome 的进程报告区域渲染完成，再核对实时输入框消失。恢复上报后，Turn `01a1eea6-5f32-7d30-a396-478e11c67c1a` 收到标题并产生候选 `55571f378c9465f6b532bc9a683b7b4d022ad4a2`。回执为 `stale-native-current/latest.json`。

数据库专项另验已登记请求在报告过期后的幂等重试、过期期间无法领取，以及同一回合恢复报告后并发领取只成功一次。使用隔离的 PostgreSQL 18 并启用真实 Docker 用例，全量 Go 回归 557 个具名测试及子测试通过，具名测试没有跳过或失败。`go vet ./...`、前端类型检查和生产构建通过。原始回执为 `stale-native-old/latest.json`、`stale-native-current/01a1eea3-a795-73a3-a148-4c8164740f83.json`、`stale-native-upgrade/latest.json` 和 `turn-fresh-regression.jsonl`。

<a id="native-input-history-recovery"></a>

### 专题 07、16 A.1/B.2 · Worker 重启后核对缺失的原生输入回执

2026-11-18，[beta Issue #250](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/250)，Windows 正式 Worker、Chrome、Codex App Server 0.169.0-alpha.11.1 和隔离 PostgreSQL 18。只在测试节点的 SQLite 中注入回执写入丢失。控制面、原生模型和浏览器均使用正常入口。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 Chrome 向执行中的原回合提交标题，令节点无法保存原生确认，等候候选 | 模型取得输入，工作台仍显示待确认 | 原生 PID 27228 正常退出，候选 `5b5efd9bd93ee4fcad402b737abc74d70b71aad2` 恰有一个 `Harbor c3d60f9c` 二级标题。节点有一项待核对输入、零条已保存回执。 |
| 移除测试故障，重启相同节点身份的 Worker，重新打开任务 | 从原回合取回证据，工作台显示已接受，原候选保持有效 | Thread `01a1ee9a-0cfa-7172-a42d-e506810e60c2`，Turn `01a1ee9a-1052-7ea2-b02b-ca08f7286685`，输入 UUID `fdabd729-7e5b-4a81-a235-3a14ab111292`。Chrome 回读同一正文及接受状态。待核对记录清零，执行只登记了原来的一个模型进程。原生历史仅一个回合，输入项为 `01a1ee9a-2b13-78b0-80e3-65357c16db10`。 |
| 原生命令仍在执行，节点已收到投递响应但写盘故障仍在；立即终止 Worker，再从原节点启动 | 若原生历史没有该消息，保留待确认，禁止再次向模型发送 | 原生 PID 38540，Turn `01a1ee9d-fb87-7e93-993b-ea8226e345f0` 最终为 interrupted。原生历史中 UUID `690fe7de-c793-4087-b580-ce14dac4eae0` 出现零次。Chrome 显示“节点已领取，投递待确认”；SQLite 保留一条待核对记录，只有原来的一个模型进程。 |
| 改换 Thread、Turn、工作目录或正文摘要，或使同一 UUID 出现零次／两次 | 这些证据均不能确认投递 | Worker 专项断言通过。原生拒绝正常生成拒绝回执，错绑回合的响应保留未确认记录。 |

实测回执为 `turn-input-history/latest.json` 和 `turn-input-crash-history/latest.json`。原生双版本协议的对照提交为 `fe726b748c4ee942c5fb39024ff126bbf2397a15`。

<a id="stable-worker-delivery"></a>

### 专题 07、16 A.1 · Codex 0.147.0 正式 Worker 的恢复与交付

在隔离的 PostgreSQL 18 和真实私库中派发 beta #258，节点选择 `gpt-5.6-sol`，等待原生模型落下草稿后终止 Worker，再用同一节点目录启动。本次从实际入口发现两处仅运行协议探针时未暴露的差异。

| 操作 | 预期 | 实际结果 |
| --- | --- | --- |
| 首次使用旧版恢复原 Thread，分别检查带和不带 `excludeTurns` 的响应 | 明确恢复失败的协议条件 | 带该字段返回 -32600，说明需要实验能力。省略后原 Thread 可以恢复。Worker 只在收到这一明确拒绝时重试，继续核对同一目录、权限、模型和 Thread。 |
| 模型完成写入后关闭标准输入，观察真实进程退出 | 成功回合应收集真实退出码 | 两次独立原生探测在 7.69 秒、9.05 秒后退出 0。原 Worker 五秒后强制终止，误将已写好标题的执行结算为失败。成功终态的退出宽限调整为 15 秒。 |
| 正式 Worker 重启后，再从 Chrome 查看并批准精确候选 | 保留原生 Session、草稿编号和文件内容，模型正常退出 | PID 45400 与替代 PID 2524 属于同一 Session `01a1eec2-209f-74f2-a525-d616ac0fa8a7`，退出码分别为 -1、0。候选 `a9e3c93687920a12281d3d13d8df8b2802333875` 保留编号 `54b98089d947` 和一个最终标题。 |
| 检查受保护分支的 PR、专门设计为首轮失败的 Actions 和本地 CI | 必需检查全绿后才允许合并，远端 PR 的候选应等于 Owner 批准的 SHA | [Actions 44310333580](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44310333580) 第二次通过。`acceptance-check` 与 `forgeos/local-ci` 均成功，[PR #259](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/259) 以 `8cabaf5771c8808301ebc63d1cf5b5f181ecd472` 合入验收分支。 |

交付完成后，GitHub 回读 [Issue #258](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/258) 已关闭，留有一条交付评论。控制面最终状态是 `DONE`。本机回执为 `active-writer-stable-delivery/latest.json`。较新版本从原生实现回合到受保护分支的交付可核对 [PR #169](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/169)。

使用独立 PostgreSQL 18 和真实 Docker 执行全量 Go 回归，562 个具名测试及子测试全部通过，9 个包通过、3 个包无测试。报告为 `stable-delivery-regression.jsonl`。原生参数核对、旧版恢复、正常退出与中断兜底专项及 `go vet ./...` 通过。

<a id="native-version-matrix"></a>

### 专题 07、16 A.1 · 两个已安装版本的 Codex 原生协议

2026-11-18，Windows。分别使用 npm 安装的 `codex-cli 0.147.0` 与桌面版的 `0.169.0-alpha.11.1` 原生可执行文件。0.147.0 使用本机已配置模型运行时，服务返回 HTTP 400，提示升级客户端。读取该版本的 `model/list` 后，用 `--model gpt-5.6-sol` 指定其返回的默认模型，仅对本次探针进程生效。

| 操作及预期 | 0.147.0 实际结果 | 0.169.0-alpha.11.1 交叉核对 |
| --- | --- | --- |
| stdio 首轮完成，换进程恢复同一 Thread 并中断下一轮 | PID 47952 → 46004，Thread `01a1ee94-0578-73d2-bca0-59f8e9f659db`，两轮完成，第三轮中断 | [初始化、恢复与中断](#native-thread-interrupt) |
| 拒绝错回合，追加输入并核对历史 | 错回合返回 -32600。回合 `01a1ee94-0d66-7600-8ee2-2218d36a1782` 的输入 UUID、正文和随机回复一致 | [原生输入回执](#native-active-reattach) |
| 同一 Thread 批准一次、再次拒绝、拒绝写文件、取消 | Thread `01a1ee94-05c6-7b22-939c-a69de763d682` 四次请求均有独立工具编号，获准命令输出 125810899301，标记文件始终不存在 | [四回合回归](#native-active-reattach) |
| 命令运行期间断线，接回原进程，要求原命令退出 0、输入 UUID 和随机回复一致 | PID 23604，Turn `01a1ee95-57ec-75d1-9bed-89bc5624f6c9`。重连后收到原命令 `exec-72acf194-35ab-47c1-8087-3b770e52a1a1` 完成事件，退出 0。重建历史包含输入和回复，省略命令项 | PID 25788，Turn `01a1ee95-5e29-76f2-ad08-fad5a0feaa01`，原命令实时完成事件和历史项均可查，退出 0 |

复验命令为 `python -B scripts/probe_codex_app_server.py --bin <原生路径>`，分别运行默认模式、`--steer`、`--approvals` 和 `--reconnect`。稳定版各命令另加上述模型参数。原始回执为 `protocol-matrix-{baseline,steer,approvals}.json`、`compat-147-events.json`、`compat-169-events.json`。新版的私有套接字探针首次实现提交为 `9f80869e8f68737f6cd6c4e0e67afebab1997cf5`。0.147.0 已完成[正式 Worker 的恢复与交付](#stable-worker-delivery)。Owner 原生工具审批仍待验收。

<a id="native-active-reattach"></a>

### 专题 07、16 A.1/B.4 · Codex 执行命令时断开并接回同一原生进程

2026-11-18，Windows，`codex-cli 0.169.0-alpha.11.1`。运行 `python -B scripts/probe_codex_app_server.py --bin codex.exe --reconnect`。本次只读 Thread 和套接字由探针创建。控制目录的 Windows ACL 仅授予当前用户访问权。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 收到命令开始事件后关闭第一条连接，在新代理中读取并恢复同一 Thread | 原服务进程继续运行，读到一个进行中的原回合 | PID `11516`，Thread `01a1ee8f-0973-7682-b2f5-ad19273e498c`，Turn `01a1ee8f-0d83-7200-b589-9d184777660d` 为 `inProgress`。 |
| 绑定原回合追加一条含随机标识的输入，再按原生命令编号和输入编号读取历史 | 原命令成功退出，输入只出现一次，最终回复与标识相同 | 命令 `exec-a4a4602a-1116-45a2-8ef5-e476a0933753` 退出码 0。历史中的输入 `01a1ee8f-29d4-7930-9b90-045ed3fae0f3` 保留请求 UUID `31e5dac9-08e8-443f-ab13-0865a70bb4eb`。回合完成，回复 `FORGE-b25d28f488f4ef45`，工作区为空。 |
| 复跑原有 stdio 输入和审批模式 | 过时回合被拒，单次批准、再次拒绝及取消仍生效 | stdio 输入历史核对通过，过时回合返回 `-32600`。四个审批回合分别返回一次成功、两次拒绝和一次取消，标记文件始终不存在。 |

原始回执为 `native-reattach-key.json`、`native-stdio-final.json` 和 `native-approvals-regression.json`。正式 Worker 的进程接回和整机重启继续列为待验收。

<a id="returned-writer-recovery"></a>

### 专题 07、16 B.4 · 写入回合返回后、模型名额释放期间重启

2026-11-18，用正式 Windows Worker、Codex App Server、Chrome 和隔离 PostgreSQL 18，分别验证成功返回、Owner 取消、暂停与业务答复。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 beta #256 丢弃控制面已接受的模型释放回执，终止 Worker，再重启控制面与 Worker | 仅重报原步骤的释放；沿用实际退出记录和工作区，不重复调用模型 | 原进程 36824，Session `01a1ee7f-d02c-7422-9d91-0d0c4c0290b7`，恢复前后始终只有这一个进程。两次释放均为步骤 1，CI 仅步骤 2。候选 `e4fccfbb05650ed77f94e534b46edd32d02af147` 保留原生回合写出的完整正文。[Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/256) |
| 在同一断点停机期间用 Chrome 取消，再重启节点 | 取消生效，原草稿不进入 CI | 终态 CANCELLED，进程表仅有原进程；没有候选、CI 或残留的恢复记录。 |
| 暂缓转发原进程退出通知及模型释放请求，在 Chrome 请求暂停后终止 Worker | 恢复时先进入暂停状态，Owner 确认恢复后继续 | 页面进入 PAUSED，SQLite 中一条暂停记录、零条候选结算记录，CI 尚未启动。点击恢复后仍只有进程 26400，会话 `01a1ee7f-d04c-71b3-a07f-9f8af35c828e`；候选 `bfb9ecd571c40fad1035975e496338af358ec7af` 包含原稿。 |
| 在 beta #257 首个回合提出业务问题后丢弃释放回执，重启并通过 Chrome 答复 | 提问及依据保留，答复前不写文件，仅答复后的回合执行写入 | 原问题要求 Owner 指定手册 H1，停机时文件尚不存在。页面显示原生问题，Owner 答复 Review 484e98ea；原会话 `01a1ee82-3d57-78f0-a388-85d7131fabc6` 的两次进程分别提问和实现。候选 `02e2362506146704e8d754adddb42d8101935551` 含指定标题，仅一次 CI。[Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/257) |

<a id="first-model-grant-recovery"></a>

### 专题 07、16 B.4 · 首个写入回合启动前重启

实现提交 `0c977d76a699ff7fe23dd2844bbe8459b526abcd`。在 [beta #255](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/255) 上使用两个隔离 PostgreSQL 18 实例分别验证恢复和取消。两次均在控制面实际批准第一个模型名额后丢弃 204 响应并终止 Worker。操作前后从工作台、控制面 API、节点 SQLite 和 Git 交叉核对。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 阻断第一次名额回执，查看日志再终止 Worker | 已保存任务来源和工作区；尚无模型进程 | 检查点 0，基线 `6114d2205497635a8f25348f3df0bb8057048817`，两棵树均为 `7e08857110f24a9de45e55927b0094315d1da60a`。进程表为空；工作区中尚无目标文件。 |
| 重启正式控制面和同身份 Worker，恢复响应，用 Chrome 核对结果 | 在原工作区启动一次模型、使用一次 CI，生成符合要求的候选 | 名额成功响应的资源步骤依次为 1、1；唯一模型进程 44352，会话 `01a1ee79-d78b-7e80-a1b6-7e5cd5b75fa3`，仅一次 CI（步骤 2）。候选 `00de3032a14f0a7e90da3927e25ffc7e76a0ac04` 中有要求的标题和正文，页面与 Git 一致，恢复记录已消费。 |
| 在第二套实例用 Chrome 取消同一来源的任务，再启动 Worker | 取消生效，无模型或 CI 执行 | 取消请求返回 204；终态 CANCELLED，原生进程数 0，无 CI 名额、无候选，恢复记录已清理。 |

<a id="queued-model-grant-recovery"></a>

### 专题 07、16 B.4 · 下一写入回合已获模型名额时重启

实现提交 `a7ca585a1395d5f284ecd4db2ad8754f7c5d3f28`。2026-11-18，在 [beta #254](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/254) 上运行正式 Windows Worker、Codex App Server 和 PostgreSQL 18。Chrome 在首次写入完成后排入 Owner 指令，要求保留原文并追加一次指定标题。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 控制面确认分配第二个模型步骤后，代理丢弃成功响应，终止普通 Worker | 模型尚未收到指令，草稿和检查点仍可恢复 | SQLite 仅有首次原生进程 23044，退出码 0。检查点保留序号 0、Session、原 HEAD、暂存区及草稿树 `bb83f3320ce077518166fc8049f6bc972586ccae`。原稿仅有标题和一行正文。 |
| 重启控制面和同身份 Worker，恢复网络，用 Chrome 查看候选 | 使用同一名额和 Session，第二回合执行一次，原稿完整 | 两次成功分配请求均为步骤 2；进程 30864 在原 Session `01a1ee75-3319-7992-a8b0-35011f6e8612` 完成，候选包含原文及恰好一个 Recovered slot 8fe17a4a 标题。仅有一次 CI（步骤 3），任务进入 WAITING_ACCEPTANCE，窄屏 Chrome 显示候选 `b005c497b34c38ce539bdaca3ce74de17ae9902a`。 |

启动前的草稿变更以及重复消费同一恢复记录另有节点测试。此次断点位于原生模型启动之前。

<a id="git-document-hybrid"></a>

### 专题 08 · 在指定 Git 提交中用中文检索英文工程文档

实现提交 `7f0806c132f8159bca14625a6219449d77faf17c`。2026-11-18，以 [beta #253 的合并提交](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/253)和它的第一父提交作为两个版本，在新登记的私库克隆和空向量缓存上启动正式控制面。模型与下方[项目历史检索](#history-hybrid-search)相同。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在节点 documents 接口对合并提交查询“恢复备份后如何校验完整性”，与未配置模型时比较 | 自然语言能找到相应版本的英文手册，可直接用 Git 验证引用 | 字面量查询为零。混合检索首屏 30 条，more 为 true；备份手册排第 2，blob `d4d5a7226cb11f5e4f8ac7295a11d236df5747dc`，引句与 Git 原文字节 [0,95) 完全一致。[原文](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/blob/82281762168441c93fd5bcf927dd14c169aea347/docs/backup-verification-note.md) |
| 改查第一父提交 `9da699577a1f8772912f912d6a5124bddc7613dc`，随后重启控制面，以 Owner 身份回查合并提交 | 旧树排除新文档；同一版本的结果稳定 | 父提交里没有这份手册。重启后完整响应相同，模型仅收到查询词。两次检索及本地补充样本共缓存 103 个 blob、564 个 384 维片段。 |
| 提交带多字节前缀的文档及末尾含 NUL 的大文件，另留未提交草稿，用大写词查询；换用跨项目节点、无效 Owner 和不可达 SHA | 保留精确字节位置，只检索当前获准的已提交文本 | CERULEAN 定位原文 cerulean，范围 [37,45)。编码器未收到二进制文件的文本前缀、工作树草稿或 .env 内容。跨项目 403，无效 Owner 401，不可达版本 422，三次请求均未调用模型。 |
| 第一次冷启动后再运行完整查询，模拟分批编码时模型维度改变 | 冷启动可在一次请求中完成；部分索引不能被当作完整 blob | 初测暴露原节点请求的 15 秒超时。调整文档检索的请求期限后，空库一次检索通过。数据库测试另验证第二批失败时首批回滚、重试成功和向量复用。 |

<a id="history-hybrid-search"></a>

### 专题 08 · 需求修订、评论、合并与 Goal 的混合检索

实现提交 `9c6ffd037da73d6430d37ca2ede6dfbe6df1137c`。2026-11-18，在隔离的 PostgreSQL 17、pgvector 和真实多语言 MiniLM 模型（修订 `e8f8c211226b894fcb81acc59f3b34ba3efd5f42`，384 维）上，从 Owner 与授权节点的 history 接口复验。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 采集 beta #251 的两次需求修订及评论，升级数据库后浏览，再用中文询问停电恢复 | 浏览保留两个版本；中文能定位英文旧版 | 修订 1 的英文备份恢复要求排在修订 2 的排版要求之前，标记 historical，带原 Issue 链接。按来源查询仅生成两个需求向量；限定 comment 后才生成第三个向量。[需求](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/251) |
| 以完整评论链接回查，重启后重复查询；换用未授权节点及外项目任务号 | 固定来源可重复定位，越权请求无法读取或触发编码 | 返回评论 6976917941、版本 1；重启复用缓存。Owner 鉴权失败 401，节点跨项目 403；空范围、外项目任务号和完整评论链接均未调用编码器。[评论](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/251#issuecomment-6976917941) |
| 批准 beta #252 的一项 D2 计划，验收原生 Worker 候选，在必需检查失败后重跑 44294472323 | 检查通过才合并，父 Goal 关闭；查到的 PR、候选及合并 SHA 与 GitHub 相同 | #253 已合并，候选 `8fa9ba2ec62224ba4f78e2e35d5128b4d34623c0`，合并 `82281762168441c93fd5bcf927dd14c169aea347`。授权节点用中文提问、Owner 按合并 SHA 回查，均返回同一子任务与合并事件版本 10。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/253) |
| 用同一中文问题检索父任务的 goal 来源，重启控制面再查 | 定位已交付的生效计划及原 Issue | GitHub 为 CLOSED；首次与重启后的结果均为父任务 1、计划修订 1，引用 beta #252。[Goal](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/252) |

<a id="knowledge-role-matrix"></a>

### 专题 08、16 C.7 · 知识工作台和六类角色的实测对照

2026-11-18，在提交 `59964ae73cf815e6cfb209e77aeb3fb645fa5297` 上启动正式控制面和隔离 PostgreSQL 18，分别运行仓库内的 probe_project_materials.py、probe_project_decisions.py。Chrome 两会话复验均通过。资料 probe-0ef1e61e2b8f4fa997175f697c7e192b 从修订 1 走到 3；决策 probe-cf0bdbbf77294d08a4b3162d502bb4e4 从提案 #1 复验竞争替代、拒绝及失效。两项探针的命令输出保存在 knowledge-owner-regression/latest.json。本轮[跨项目发布与检索](#shared-experience-search)另验证审核、发布、接收方隔离及撤销。

对照专题 08 第 7 节列出的角色，以下会话均已回读。五份 Codex 原生记录包含实际沙箱和审批模式；独立 Reviewer 从原生 Claude 会话核对审查结论。两份开发交付另用 GitHub CLI 查询已合并 PR 的 headRefOid 与 mergeCommit。

| 角色 | 来源及实际结果 | ForgeOS 实现提交 |
| --- | --- | --- |
| Management | beta #173 的会话 01a1eae5-e09a-7823-a9d8-bb0d2604d5a7 使用只读沙箱，核对计划引用的 capacity 子项和固定全局规则 Generic queue guide，退回与项目决定冲突的草案。 | `20b8067c71a9a24ae374bee66bc7fed13a152bdb` |
| Planner | beta #81 的只读会话 01a1e76b-4aec-7243-b97f-7675a1cf6c4a 将冻结规则中特有的名称 Milestone 8bb0a8d811 写入计划。 | `3744ecc3498e25d8214558f13fedf0a7fc65e7d0` |
| Development | Codex 写入会话 01a1e779-0b9f-76b2-ba5f-394c482e60b0 处理 Owner 指定文档。另回读两次既有交付，[Claude PR #82](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/82) 与 [Codex PR #84](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/84) 均为 MERGED，候选及合并 SHA 与原验收相同。 | `3744ecc3498e25d8214558f13fedf0a7fc65e7d0` |
| Review | 独立 Claude 会话 064aaa2d-af24-4a8f-9eac-422cf128f2d8 的审查原文包含候选路径 docs/rules-codex02-owner.md 及 Review Owner 4c156e23，对照实现时的规则定位到第 1 行。 | `e566c170ecdb08caae37799dec46fa450f079f59` |
| Test | 只读会话 01a1e798-616e-7b12-8a39-b51835d43944 的输入含候选 e699b87538bb2765e843f92842c4b07b6aae627e、冻结全局规则及 requirement/1；原生结果为该条标准生成 documentation 用例，控制面归档为 TC-001。 | `da40dd469296d019380fcf59a769d01025ff666a` |
| Operations | [beta #249 的只读会话](#operations-planner-context)引用恢复手册和本次 Runbook gate 4319de9b，计划保留现场待核实事项；原工作区干净。 | `80b8b1a50816647b71e6c06b75c4e657d1b2264f` |

C.7 的产品管理入口及上述角色已验收。专题 08 其余检索来源和四阶段最终回归按各自记录继续跟踪。独立 Reviewer 的中央日志离线下载见 [C.3 实测](#reviewer-central-log)。

<a id="live-workbench-turn-input"></a>

### 专题 07、16 A.1／B.2 · 从工作台向当前 Codex 回合发送输入

2026-11-18，从正式 Issue 入口派发 [beta #250](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/250)，使用 Windows Worker、Codex App Server、独立 PostgreSQL 18 与 390 像素宽的 Chrome。要求在活动模型回合期间，由 Owner 临时决定新文档的小标题。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 以普通节点凭据调用 Owner 入口，另用 Owner 提交过时的 Turn | 越权和错误回合均拒绝 | 分别得到 401 和 409，原生会话继续运行。 |
| 从工作台向 PID 22776 的活动回合发送小标题，丢弃第一次 Owner 登记响应后用原 key 重试 | 只登记一条，正文改变时拒绝复用 key | 重试得到 204，更改正文得到 409；执行历史只有一条 key 为 04c43f12-9a79-4640-a106-c4e5c83d12cc 的记录。 |
| 丢弃节点第一次成功领取响应，再丢弃其第一次成功补报的响应 | 节点能重新取得同一输入，重复确认不会重复注入 | 领取重试使用相同领取编号。Thread 01a1ee50-8b8c-74b2-962e-5afd59c2d3ac、Turn 01a1ee50-90c8-7473-bf19-8dd3aa96f79a 只有一条匹配的原生 userMessage，原生输入项为 01a1ee50-b7a6-72f2-b95a-8f753ca4910b。 |
| 核对正式执行产出的 Git 候选，退出并重启控制面，在 Chrome 回读 | Owner 输入在同一回合生效；终态可审计，旧请求可重读 | 原生进程退出 0，候选 fb00273f285e11b4c3371dab78383ee1851d4f4e 的 docs/active-turn-receipt.md 仅出现一次 H2 “Harbor 91847850”。终态重报原请求返回 204，新的输入返回 409。重启后 Chrome 显示原文及“原生回合已接受”。 |
| 分别用新控制面配旧 Worker、旧控制面配新 Worker 实际派发并取消 | 仍显示原生 Turn，只有支持投递的组合显示输入框 | 两组都能从工作台取消，输入框均未出现；直接提交分别为 409、404。 |

实现提交 `59964ae73cf815e6cfb209e77aeb3fb645fa5297`。操作脚本 turn-input-final-live.py、turn-input-mixed-live.py，回执 turn-input/final.json、oldworker.json、oldcontrol.json。专题 16 的原生工具审批仍待验收。

<a id="operations-planner-context"></a>

### 专题 01、08、16 C.7 · 运维任务的只读规划依据

2026-11-18，创建真实 [beta Issue #249](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/249)，要求为恢复后的 CI 证据制定核对计划。通过正式控制面的 Issue 入口派发，使用本机 Windows Worker 和原生 Codex 生成修订 1。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 发布全局规则，要求在计划中写出本次的检查点名，再导入带 Operations 前缀的 Issue | 采用冻结规则和运维类型，在固定提交读取适用手册 | 任务 #1 识别为 operations，模式为 plan，基础提交 6114d2205497635a8f25348f3df0bb8057048817。原生输入含运维角色说明和检查点名 Runbook gate 4319de9b；实际工具调用读取了 docs/check-log-restore.md。 |
| 核对原生会话、Git 工作区及计划修订 | 在只读环境生成计划，计划指出适用环境、依据和核对方法 | 会话 01a1ee41-aa96-7f13-bf9d-7d1d6c6c5e5b 的沙箱为 read-only，审批为 never，正常退出 0。计划引用 beta 目标分支、恢复手册、git show 和 git merge-base --is-ancestor 的核对命令，列出现场仍需提供的持久化证据。工作区仍在原 SHA 且干净，任务没有交付记录。 |
| 重启控制面后，从 390 像素宽 Chrome 打开任务的计划修订 1 | 能从历史计划复核这次使用的路径和检查点 | 计划 API 与重启前逐字段一致，任务页可见手册路径和本次检查点名。 |

实现提交 `80b8b1a50816647b71e6c06b75c4e657d1b2264f`。操作脚本 operations-plan-live.py，实测回执 operations-planning/latest.json。普通开发规划和运维规划的适配器分支均通过独立测试。

<a id="shared-experience-search"></a>

### 专题 08、16 C.7 · 接收项目检索已发布的脱敏经验

2026-11-18，在两个私有验收仓库登记的项目间起草脱敏经验，由真实 Management Codex 独立审核，Owner 按摘要发布。通过 390 像素宽的 Chrome、正式控制面、授权节点身份、Docker pgvector 数据库和本地多语言编码服务验收。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 从 schema 115 升级，先关闭编码服务，在接收项目浏览并搜索 | 原有浏览逐字段一致，关键词可独立工作 | 两条已发布副本保持一致；checksum 命中恢复手册，中文问题没有字面命中。 |
| 开启模型，在工作台“接收的经验”和节点接口查“断电后该怎样恢复” | 两个入口首选同一份英文手册，仅返回已发布字段 | 首位均为 #1 Recovery procedure，摘要 603a8e45481d02851f9b32b1cacc733d347c0703024db521b4386e27959439a2。两份副本各缓存一个 384 维向量，节点复查后缓存版本相同。 |
| 以错误身份和越权节点发起查询，检查实际发送给编码服务的内容 | 先授权、后编码，源项目私有资料和待审核的 #3 保留在源项目 | 分别返回 401、403，未产生编码请求；合法请求只涉及查询和已发布副本，未包含源资料位置、私有正文标记或草稿标记。 |
| 由源项目撤销 #1，刷新接收项目、再从节点及重启后的控制面查询 | 已撤销副本立即退出结果，删除其缓存 | #1 的缓存已清除，#2 的 384 维缓存保留；工作台、节点和重启后查询均未返回 #1。PostgreSQL 回归另在编码响应返回前执行撤销，旧模型和正在写入的新模型均无残留。 |

首次试用的食堂菜单被 Management 退回，理由为缺少其他项目可复用的经验。上述两条发布记录分别为恢复手册和字体可读性检查，审核会话为 01a1ee3d-bf4a-7632-b312-9a690d6c4726、01a1ee3d-f014-7a70-91d8-f89c5c7dcec0。实现提交为 `cf3fbbe10453e03e0bd6a2c67d236f6432c76c6d`。运行记录为 shared-semantic/latest.json，数据库为 forgeos_decisions_ecfe28b7。全包回归 shared-semantic-regression.jsonl 共 536 项测试和子测试通过，另有撤销并发场景的独立回归通过。

<a id="startup-log-download"></a>

### 专题 07、13、14、16 C.3 · 从工作台下载环境启动日志

2026-11-18，从 390 像素宽 Chrome 的仓库清单展开 `project.yaml`，通过正式控制面和 Windows Worker 在 Docker 中运行两项启动检查。实现提交为 `e5289ca2872b0feadb0ab565617bdb6e862333d9`。归档对象为节点实际保留的脱敏 `startup.log`，使用项目的日志保留策略。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| Local Mock 输出超过 8 KiB 的启动诊断，然后退出 7 | 保留失败结果，下载文件包含页面窗口之前的输出 | 任务 2 为 FAILED。归档 23,721 字节，首条 `startup-line-0-` 在下载文件中可见。合成口令已脱敏。SHA-256 为 `fe128915cbf4e7037534b89427c8c2944e3aa0d4db540cf56807c900bc78c066`。 |
| Local Integrated 通过隔离网络访问 PostgreSQL 测试依赖 | 数据库查询成功、HTTP 就绪并保存脱敏日志 | 任务 3 为 DONE，HTTP 204。数据库记录真实就绪查询，两次启动容器均已移除。集成日志 132 字节，SHA-256 为 `605506d4ec05def07db79ef20406609437d90597561d246768e61d712591b485`。 |
| 停止节点，重启控制面，从两个任务的“启动日志”下载 | 下载不依赖在线节点，Owner 下载内容与本机文件一致 | 两份 Chrome 下载均名为 `startup.log`，字节数及摘要匹配。普通节点凭据下载返回 401。 |
| 将节点日志保留期设为 1 秒并重启 Worker，再单独使中央日志到期 | 节点和中央分别清理，中央不再提供已到期的内容 | 两份节点文件清理后中央仍返回 200。中央策略生效后两次下载返回 410，重启控制面后两份中央正文均被清理。 |
| 用旧控制面配新 Worker、新控制面配旧 Worker 分别重跑 Local Mock | 旧组合保留输出窗口，不显示无法兑现的归档按钮 | 两次均记录 23,721 字节和终态，未声明归档摘要，Chrome 没有下载按钮，直接请求归档返回 404。 |

启动日志沿用节点现有的 1 MiB 采集上限和截断标记。历史回执没有文件摘要时，工作台继续展示已保存的输出窗口。

<a id="role-process-central"></a>

### 专题 07、14、16 C.3 · 独立角色的中央日志下载

实现提交 `06bb5fcbc11e2a5bed43193450e3a1327a8b1119`。2026-11-18，使用正式 Curator、Management、Test Agent、原生 Codex、Chrome 和 PostgreSQL 18。日志以角色作业的进程记录和节点实例标识。控制面和节点均使用私有验收库与独立凭据。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 提交资料 `queue-handbook` 修订 1，阻断上传并丢弃第一次退出确认 | 业务提取、审核正常完成，两个节点各留一个待传档案 | Curator 会话 `01a1ee26-3091-7c52-892f-b5b3a47fbee2` 保存 390 字节。Management 会话 `01a1ee26-65dd-72c2-af96-07ede794bd88` 保存 538 字节。Owner 下载均返回 409。 |
| 重启节点和控制面，重放已结算作业的原始退出回执，放行上传并丢弃一次成功确认 | 同一回执找回原进程 ID，后台核对已接收字节，清空节点待传队列 | 重放返回 204 和相同 ID。两份归档均完成，队列清零。补传前备份两个私有日志库并将已退出回执置于保留期外，新旧 Worker 离线清理命令均返回零条，待传正文完整。节点停止后，Chrome 从知识提取与审核页分别下载，逐字节等于对应节点 CLI 的已验证记录。 |
| 用其他角色访问上传进度，用角色和普通节点凭据下载，然后损坏一份验收库中的块并使这份归档到期 | 凭据隔离，损坏与到期可区分，其他归档继续可读 | 跨角色返回 404，非 Owner 下载返回 401。损坏返回 422，到期返回 410。控制面重启后已清除到期正文、保留原清单，另一份仍返回 200。 |
| 恢复 `native_reviewer_profile/before-review.dump` 的真实历史候选，在任务 1 工作台生成测试计划，运行 Test Agent 和独立 Management | 候选计划和覆盖审核分别关联自己的进程，离线下载能逐字节核对 | Test Agent 会话 `01a1ee22-8157-7040-9183-04c6f5015cc6` 的 2,512 字节、Management 会话 `01a1ee23-1eff-7c61-8342-821234ad4546` 的 1,144 字节均从 Chrome 下载并与节点导出一致。 |
| 分别以支持清单的旧控制面配新节点、新控制面配旧节点，在 [beta #247](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/247) 派发后从需求历史回读 | 两种组合继续显示本地清单，只为协商了中央传输的进程显示下载按钮 | 两种组合分别显示 352、358 字节清单，均与节点 CLI 校验一致。Chrome 两次回读都没有中央下载按钮。 |

<a id="reviewer-central-log"></a>

2026-11-18，在独立 PostgreSQL 18 恢复同一真实候选 `d689d8e011d2c9e1e49e93cfa632d31b132c2228`，从 Chrome 任务详情启动独立 Claude Reviewer，使用中央归档实现 `06bb5fcbc11e2a5bed43193450e3a1327a8b1119`。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 等待审查终态，读取进程退出记录和日志传输进度 | 模型异常时保留原退出码与公开诊断，任务可追溯 | PID 1564、Session `862108d3-4fc9-43ef-aab4-65f0e6748e76`。模型服务返回 HTTP 403，进程退出码 1，审查状态为失败，中央收到 1 条、269 字节。 |
| 停止 Reviewer 并重启控制面，从 Chrome 展开「查看审查记录」下载，再与节点 CLI 导出核对 | 离线下载可用，校验链、记录数和内容一致 | 下载文件与已验证的节点导出逐字节相同，SHA-256 链值为 `9cbd966facfd2d17d39982a8c2a5bcb0b8b13c84837cf4e9f5cee503f91c0be6`。 |
| 使用普通节点、Reviewer 和 Management 的凭据分别尝试下载 | 只有 Owner 可读中央正文 | 三次请求均为 HTTP 401。 |

运行回执为 `reviewer-central-receipt/latest.json`。首次验收在归档尚未传完时停止了节点，浏览器未取得文件。复测等到中央可读后再停节点和重启控制面，离线下载通过。成功审查的历史证据见[角色对照](#knowledge-role-matrix)；本次 HTTP 403 仍阻塞需要 Claude 生成新结果的场景。

<a id="role-process-archive-manifest"></a>

### 专题 07、14、16 C.3 · 核对独立角色的节点日志清单

实现提交 `5468b9d4f9e6a705826844a32ce15e88fcdcd550`。2026-11-18，使用 [beta #247](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/247)、正式 Management 节点、Chrome、PostgreSQL 18 和节点日志导出 CLI。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在讨论收件箱展开分类进程，等候原生回合退出 | 进程清单显示实际节点记录数、字节数及摘要 | PID 23556、会话 `01a1ee13-2189-77a3-a411-9ce616651565` 成功退出。公开输出尾部为 239 字节，节点归档为 1 条 JSONL、352 字节，上限 32 MiB，省略 0 条。 |
| 停止节点，执行 `worker process-logs export`，独立对原始 UTF-8 记录重算链摘要 | 导出的记录与工作台清单逐项一致 | 导出末行标记已验证。记录数、字节数以及 SHA-256 `ed75128172a81580ee44f1685b1308a85c05402010c6b79b2834b862b51b64a7` 均一致。 |
| 重启控制面，从工作台改选调查，打开任务 6 的需求修订 1 | 进程退出和归档清单保留原值 | Chrome 在历史页显示同一会话及完整链摘要。其他项目查询为 404，节点和角色凭据访问 Owner 入口均为 401。 |
| 将资料 `queue-handbook` 修订 1 送入 Curator 提取和 Management 知识审核，停止两个节点后从工作台回读 | 两个独立角色各保留自己的节点、原生会话和归档证据 | Curator PID 40364、会话 `01a1ee16-e972-74d2-ba3d-ea2557c10381` 归档 1 条、385 字节；Management PID 28404、会话 `01a1ee17-1f4f-72e0-8f94-67859648312f` 归档 1 条、518 字节。两个角色各自的 CLI 导出均通过逐字节校验，Chrome 所示摘要一致。 |

四种角色共用退出补报协议。自动化测试覆盖各角色在断连、重启后重报原始退出数据和归档摘要，以及控制面拒绝修改已持久化的清单。已实测 Management 和 Curator 原生回合；Test Agent、Reviewer 的该项原生回合仍待验收。PostgreSQL 18 与 Docker 全包回归通过 536 项具名测试及子测试，零失败、零跳过。Go 静态检查、TypeScript 和生产构建通过。独立角色的中央全文下载另行验收。

<a id="issue-classification-process"></a>

### 专题 03、16 C.3 · 从讨论到需求修订追溯分类进程

实现提交 `792e99012df2657f68b5afc033d3539a04c208ff`。2026-11-18，在 390 像素宽的 Chrome、PostgreSQL 18 和实际 Management 节点验收。[beta #247](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/247) 由原生 Codex 分类。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 节点启动前展开讨论的“查看分类进程”，保持页面打开并启动 Management | 从等待回执自动更新为运行中，再显示真实退出 | 分类作业 1、进程记录 1 显示 PID 26196。原生会话 `01a1ee0c-fabc-73d1-85d3-4f2217456e43` 以 0 退出，页面的公开输出与控制面保存的 226 字节一致。 |
| 停止节点，重启控制面，从收件箱将建议类型改为调查并派单 | 需求历史能追溯派单时所见的版本和进程 | 任务 6 的需求修订 1 关联原分类作业；从工作台重新展开后看到相同会话和输出。 |
| 在项目 2 查询该分类，分别用节点和 Management 凭据访问 Owner 入口，核对普通讨论 #245 | 项目和凭据边界生效，普通讨论没有模型进程入口 | 项目 2 返回 404，两种非 Owner 凭据均返回 401，#245 未带分类作业 ID。 |

本项验收覆盖公开输出尾部。独立角色的完整归档下载仍待验收。启用 PostgreSQL 和 Docker 的 Go 回归通过 536 项测试及子测试，9 个有测试的包通过，另有 3 个空测试包。Go 静态检查、TypeScript 检查和生产构建通过。

<a id="central-process-archive"></a>

### 专题 07、14、16 C.3 · 普通 Worker 的中央日志下载

实现提交 `6d401c4d52952b36a2d43ed76a0da9fefae2a0de`。2026-11-18，Windows 普通 Worker、原生 Codex 只读 Planner、PostgreSQL 18 和 Chrome。验收使用 [beta #241](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/241) 的真实需求。节点通过独立测试中继访问控制面；中继只制造传输故障。工作台直接访问控制面。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 暂时阻断归档上传，让正式 Worker 完成规划 | 业务回执照常结算，上传等待重试；Owner 尚不能读到部分正文 | 执行 1、实例 1、原生会话 `01a1ede3-d577-7ea1-9fb8-9ea0f6110888` 返回规划。私有 SQLite 有 1 条待传记录；Owner 下载返回 409，节点令牌尝试 Owner 下载返回 401。 |
| 重启控制面与节点，恢复上行，丢弃控制面已经接收的分块确认 | 已收到的内容可以核对并结算，避免重复保存或重复运行模型 | 正式节点从接收偏移恢复并清空原队列；控制面保留 1 个分块。 |
| 停止节点，再次重启控制面，从 390 像素宽 Chrome 下载进程日志并和节点 CLI 导出逐字节比较 | 离线节点的日志仍可下载，内容与原有摘要一致 | 下载文件有 10 条记录、19,822 字节，没有省略；与节点 JSONL 的全部记录行逐字节相同。独立重算链 SHA-256 为 `288c651ac53c2f59597c155a87daa2e02c219743a259609caed88dfd73df8cfe`。更换任务编号返回 404。 |
| 在隔离验收库将此归档的到期时间移至过去，重启执行清理 | 下载立即返回到期；正文清除，业务回执与摘要留存 | 清理前后均返回 410；分块数从 1 降至 0。任务保留原进程和归档摘要。 |

本次真实文件使用一个分块。超过 256 KiB 并在单条记录中间断开、重复请求、受损的上传前缀清除、下载前发现损坏且输出零字节、待传正文保护另由 PostgreSQL／SQLite 测试验证。具名节点的 PostgreSQL 接口专项验证，其他项目节点读取接收位置或上传均返回 403，Owner 令牌调用节点入口返回 401。启用 PostgreSQL 和 Docker 的 Go 全量回归 536 个具名测试与子测试通过，零具名跳过、零失败；随后另复跑节点断点上传和 Worker 项目权限专项。`go vet ./...` 及前端生产构建通过。独立角色各自的作业归属及中央传输继续待验收。


用 `70bbfc0` 的仅摘要组件与 `6d401c4` 的中央传输组件双向混用，从同一私库 Issue 分别运行原生只读 Planner，重启对应控制面后在 Chrome 核对结果。

| 组合 | 原生会话 | 节点记录与工作台摘要 |
| --- | --- | --- |
| 旧控制面、新 Worker | `01a1edff-68e0-7070-ae74-09a0d4f61273` | 18 条、16,640 字节，链摘要 `e9435985f1c8aaf995433e3c10813a8888f730553d5eff9aaa263929f32afaf3`。 |
| 新控制面、旧 Worker | `01a1edff-68e0-7cf2-975a-ca9707523ec1` | 18 条、19,741 字节，链摘要 `8f746f5188490e51a2359ded6ae4021c22b3612d223ff06079ef2c23854bd2cd`。 |

两种组合均完成原任务并展示原有摘要，工作台仅在双方协商中央传输时显示下载按钮。

<a id="process-archive-receipts"></a>

### 专题 07、16 C.3 · 从任务详情核对节点完整归档

2026-11-18，正式 Control Plane、Worker、PostgreSQL 18、Codex CLI 和 Chrome。摘要实现提交 `70bbfc0c6cc8476ce461896b0dd52d282f3e9a06`。中央传输和下载的后续实测见上节。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 从 [beta #241](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/241) 派发只读规划，等候正式回执 | 每个有本地归档的进程分别报告其摘要、数量及容量省略；规划可以照常结算 | 会话 `01a1edd5-b12f-7162-b84b-4c340d207722` 生成草案。执行 1 的进程实例 1、PID 33056 记录 10 条、22,073 字节，省略 0 条；链摘要为 `c1c3a88fcd8656bdfb20eb944e9191b2472b51b6ca567add54fab268f8146068`。 |
| 从同一实例运行 `worker process-logs export`，对每条原始 UTF-8 记录逐字节重算 | 顺序、字节数、链摘要和进程身份与任务回执一致 | 10 条序号连续；首行和末行、独立计算及中央回执的摘要完全一致。最终 Worker 二进制导出的整个 JSONL 与此次归档逐字节相同。 |
| 停止 Worker、重启控制面，在 390 像素宽 Chrome 打开任务的 Harness 进程记录 | 节点离线时仍可核对已结算回执 | 页面显示同一实例、数量、字节数和完整 64 位摘要。 |

首次模型调用在 220 秒观察窗口内仍为 RUNNING，未计为通过。复验启动独立执行后取得上述原生结果。PostgreSQL 和 Docker 全量回归 534 个具名测试及子测试通过，零失败和具名跳过；随后单独复跑存储包 120 项及节点归档专项，覆盖中断交接的原进程记录与最终新增摘要并存。前端构建、`go vet ./...` 通过。

<a id="issue-classification-agent"></a>

### 专题 01 §03、专题 03 · 分类 Agent 与 Owner 决定

实现提交 `439415c11b344ed97e4f5d04689f738f414d7204`。2026-11-18，正式 Poller 从 beta 私库同步两条带 `forgeos-agent` 的 Issue。二者均无类型标签，标题也未给出动作词。Management Worker 以禁用工具的只读 Codex 处理，PostgreSQL 18 核对原生回执，Windows Chrome 验证人工决定。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 领取 [#247](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/247) 并对照 GitHub 正文 | 识别正文中的交付物，给出逐字引文，保留给 Owner 决定 | 原生会话 `01a1edbe-e7e4-7332-b679-3ed2f60c6322` 建议测试，引文为 “Please provide passing and failing cases as the deliverable.”；进程 38944 退出 0。建议生成后任务数仍为零（按该 Issue 计）。 |
| 领取 [#248](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/248)，另查普通讨论 [#245](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/245) | 无明确动作时指出需要澄清的内容；普通讨论保留原状 | 会话 `01a1edbf-3f91-7640-b802-afdde3ec43a8` 引用 “The intended outcome and requested action have not been decided.”，请 Owner 指明目标和动作。进程 40624 退出 0。#248 仍在收件箱，#245 无模型建议或任务。未声明分类能力的领取请求返回 204。 |
| 重启控制面，在 390 像素宽 Chrome 查看 #247 引文，再改选调查 | 转换前须明确选择。转换后仍能区分模型建议和最终决定 | 初始转换按钮禁用。Owner 改选后任务 6 进入只读调查规划，任务详情显示 Owner 确认；打开需求修订 1 仍显示原测试建议、引文和会话。 |

另从 Owner 预算接口配置单槽 `intake-account`，限定项目 1 的 `issue_classification`。Chrome 节点页显示该预算。两项都没有建议且第一项正在执行时，预算占用为 1，第二次分类领取返回 204。原生会话 `01a1edc6-0b88-7ca2-b907-d488d5e94ff3` 与 `01a1edc6-3a8c-7481-b46d-b066d3c0053d` 依次结束，两个进程均退出 0，各自的回执关联该预算，最终占用回到 0。

第一次实测中，模型将 #247 的主题标题当作另一项交付要求，建议澄清。调整分类说明后，以相同 GitHub 来源重新运行并得到表中结果。数据库测试覆盖来源已换版、虚构引文、缺少成功进程、错误会话、重复回报以及运行中配置单槽预算。分类建议本身不会创建工作项。启用 PostgreSQL 18 和 Docker 后，Go 全量回归 533 个具名测试及子测试通过，零跳过、零失败；`go vet ./...` 和前端生产构建通过。

<a id="issue-work-type-rules"></a>

### 专题 01、03、12 · GitHub Issue 规则分类、澄清与需求修订

2026-11-18，私库 beta、正式控制面、PostgreSQL 18、Windows Worker、原生 Codex 和 Chrome。规则分类实现为 `bbd7773fac25263b7aa0a78859da961aa78b37c8`。五类工作均核对了派发规格。开发、调查两类另运行真实模型；代码审查、测试、运维随后均完成原生规划并从重启后的工作台回读，见下表。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 为 [#239 开发](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/239)、[#240 代码审查](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/240)、[#241 测试](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/241)、[#242 调查](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/242)、[#243 运维](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/243) 配置派发信号，运行 Poller，并对照手动导入 | 五类均有固定分类，非开发型从只读入口开始；反复导入同一来源应得到同一任务 | 五类各关联一项任务。#241 通过标题首词 `Verify` 判定。#242 的只读 Session `01a1eda6-2c0f-7d22-9c8e-af35df07c70a` 在计划中引用 `docs/queue-preview/index.html` 和准确页面标题 `Queue handbook`，工作树干净。 |
| 从正式导入入口分别派发 #240、#241、#243，运行普通 Worker；重启控制面后在 Chrome 打开三个任务 | 分别按代码审查、测试、运维生成只读计划，保留原类别、固定提交和清洁工作树 | 三项均在 `34c03d512b6018110659bae8742b91286884e5f0` 进入 PLANNING。#240 生成核对手册文件及引用的清单，会话 `01a1edc2-0dbf-7593-bc5b-d0e51fc3f3f2`；#241 计划验证页面标题和一级标题，会话 `01a1edc3-0b99-7891-9764-9092ec0ea603`；#243 制定依据本地检出内容的预览就绪检查，会话 `01a1edc4-21ed-7c00-accd-2f539b1029bf`。三份草案有里程碑、工作项和交付物，工作树无变更。 |
| 显式选择 Codex 执行开发用例并在 Chrome 按候选验收 | 产生指定文档，通过本地验证和目标分支门禁 | #239 只修改 `docs/work-type-check.md`，候选 `1bb586c3bb66f7991cf12a332c86192eac7c6ec4` 经 [PR #246](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/246) 合并，合并提交 `9da699577a1f8772912f912d6a5124bddc7613dc`。首次试跑继承项目的 Claude 配置，保持 READY；实际交付使用本次明确选择的 Codex。 |
| 给 [#244](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/244) 设置冲突类型，给 [#245](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/245) 只声明测试类型；在收件箱打开后修改 #244 正文 | 冲突和普通讨论都没有自动执行。过时的 Owner 选择必须拒绝 | 两项均留在收件箱。#244 未选择类型时按钮不可用；直接导入及过时页面提交均返回 409。刷新后重新选择运维，HTTP 200，任务 13 进入只读规划。同一选择重试回到任务 13，另一个类型返回 409。#245 保持普通讨论。 |
| 重启控制面；只移除 #244 的冲突标签，再采用新修订 | 新的类型可追溯，采用前保留旧决定 | 重启后第 1 版仍为 Owner 确认的运维。第 2 版为规则识别的调查。采用前任务仍绑定第 1 版，采用后保持只读规划。重新登录及整页刷新后，390 像素宽 Chrome 显示“任务类型 · 调查（GitHub 规则）”。 |

新前端与旧控制面的桌面及窄屏转换、两个窗口重复操作、101 条分页样本均通过。启用 PostgreSQL 18 和真实 Docker 的全量 Go 回归共 528 项测试及子测试通过，零失败、零跳过；`go vet ./...`、TypeScript 检查和前端生产构建通过。分类 Agent 的调用和建议仍待实现。

<a id="discussion-inbox-ui"></a>

### 专题 01、12 · 从工作台转换普通讨论

2026-11-18，正式 Poller 从私有 beta 仓库读取开放讨论，Windows Chrome 通过正式 Owner 接口连接 PostgreSQL 18 控制面。使用独立测试数据库，未启动执行节点。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在“项目接入”打开 beta 的“讨论收件箱”，在 390 像素宽的第二个浏览器窗口打开同一页 | 两个窗口均可阅读来源并转换；普通讨论进入收件箱后不自动开始执行 | 两个窗口均显示 [beta #235](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/235) 的标题和 GitHub 原文链接，窄屏无横向溢出。另一个项目为空；Worker 令牌读取 Owner 收件箱返回 401。 |
| 两个已打开的窗口先后将同一讨论转为实现任务 | 应回到同一项目任务，重复操作不产生第二个任务 | 两个窗口均打开任务 1，来源为 #235，验证策略为 `auto`。重新打开收件箱后 #235 已移出，重启控制面后结果相同。 |
| 在独立测试项目放入 101 条分页样本，先加载下一页，再刷新 | 无重复或漏项，刷新后回到最新一页 | Chrome 依次显示 100、101、100 条讨论。真实来源及转换使用私库 #235，分页边界使用测试样本。 |

## 剩余范围估算（2026-11-18）

下表保留历史工作包估算；此后的实测结果按专题追加。当前 ADR 状态见页首对照表。

四个阶段均有真实运行记录。空库派发、单项目交付、带依赖的双子项 Goal、两个项目分别交付的操作、结果与提交列在下面两张表中。截至 `30515200fcfede6f692292d0c3ec49b15ee55ace`，按可以分别开发、复验和提交的工作粗估，余下约 20–30 项。各项规模不同；同一场景同时验证多个专题时只计一次。

| 剩余范围 | 粗估项数 | 完成时须取得的证据 |
| --- | ---: | --- |
| 执行控制与会话恢复（06、07、12、16 A.1–4/B.4/C.3） | 5–7 | 从工作台查看模型及其他执行阶段的输出、发送排队指令；在模型回合暂停或取消，核对来源关闭、进程退出、整机重启和会话交接后的实际执行。 |
| 角色配置、适配器与决策（03–07、16 B.1–3/C.4–5） | 4–6 | 回读生效的 Agent Profile；正式适配器报告实际能力；从不同角色经 ACP 路由请求，按 D0–D3 执行并留下决策与审批证据。App Server 实现、规划、恢复与原生中断已从正式 Worker 验收；[运行中输入](#live-workbench-turn-input)已验收，[原生命令审批](#workbench-native-commands)及[文件补丁审批](#workbench-file-patches)均已验收。 |
| 多节点、共享预算与 Linux 服务身份（07、14、16 A.2–4） | 3–4 | 使用同一模型账号的两节点竞争名额，独立角色计入预算；从 Linux 服务身份复用登录、经配置代理发起实际模型请求，核对并发及限额行为。 |
| 知识检索与角色上下文（05、08、16 C.7） | 2–3 | 按语义检索并回到固定来源，在尚未覆盖的角色回合核对引用版本、范围和优先级。 |
| 外部通知与集成（02、03、11、16 A.7） | 2–4 | 由外部通道接收实际通知，核对投递与重试；从提供方发起回调，关联预览环境和实际回执。 |
| 数据、审核和收尾（04、09、10、13–16） | 4–6 | 补核其他验证器与独立审核、审计载荷、事件缓冲和工件保留与恢复；最后从四个阶段入口复跑端到端，逐项对照文档要求、证据与 ForgeOS 提交。 |

此后，跨节点共享账号预算已从两个普通 Codex Worker 的竞争、控制面重启和两次 GitHub 交付验收，见本页“跨节点共享模型名额”。上表的 20–30 项保留为原估算；扣除这一项后约余 19–29 项。独立角色预算已验证 Reviewer、候选 Test Agent 和 Management 的覆盖审核。知识提取和知识审核也已通过两份资料的争用与重启验收。其余 Management 任务和 Linux 专用账号仍在范围内。

任务详情的下一回合指令已完成独立验收，扣除此项后约余 18–28 个工作包。两种适配器的版本探测和基础错误分类已有实测，运行中审批、中断等操作仍列在适配器工作内。

在 beta #125 的验收和交付后，当前已实现的独立模型角色均已验证共享名额。按上表的工作包口径扣除“独立角色计入预算”，约余 17–27 个工作包。Linux 服务身份复用登录、供应方限流和最终全路线回归仍在估算内。

真实 Claude、Codex 实现与只读 Planner 均已在进程被直接终止后恢复并通过验收。扣除原生进程崩溃恢复这一项，约余 16–26 个工作包；整机重启、会话丢失后的交接和来源关闭时中断执行继续单列。

beta #131 和 #132 已分别验证来源关闭和 Owner 主动取消对原生模型、运行中工具及下一任务的影响。按一个执行控制工作包扣除，目前约余 15–25 个工作包。运行中暂停模型、整机重启和会话交接仍待验收。

beta #137 已验证暂停 Codex 实现回合、其他项目借用模型名额、控制面重启及原会话恢复。beta #19 另已从只读 Planner 的工作台入口复验主动暂停，见页末。beta #139 已验证 Claude 实现回合恢复及交付。扣除工作台主动暂停并恢复普通模型回合这一项，约余 14–24 个工作包，其中仍包括节点重启与跨会话交接。

beta #155、#157 已验证等待业务答复期间的原生会话交接。#159、#161 已验证没有主动暂停时，Claude 和 Codex 写入进程异常退出且原会话不可恢复，保留草稿并在新会话完成交付。扣除会话交接工作包，当前约余 13–23 个工作包。整机重启后的真实模型恢复仍在估算内。

另有四类由 Owner 确认保留待验收，单独计数：Telegram 和飞书的真实通知投递、从第二台设备经 Tailscale 访问预览、使用 GitHub App 或细粒度凭据实测最小权限、第二位维护者批准受保护分支的 PR。全路线最终回归仍待执行。

| 路线要求 | 已有实现或证据 | 待补齐或待验收 |
| --- | --- | --- |
| 一 · 空库迁移、登记、派发、重启查询 | 2026-11-16 在独立 PostgreSQL 18 空库启动控制面，通过 HTTP 登记仓库、创建任务。独立 Worker 在 execution-2 工作区返回退出码 0 和仓库 HEAD `65d40b2a22dcec366c5146ed5924c607b09f7cbd`。重启控制面，任务 2 完整 JSON 相同，符合预期。 | Linux 专用身份与 systemd 见专题 07、14。 |
| 一 · 并发、失败、重报、失联 | 通过 HTTP 领取任务 1，再创建任务 2。第二次领取返回 204，任务 2 为 READY。回报受控失败后任务 1 为 FAILED，重报返回 204，任务和事件完全相同。PostgreSQL 集成测试覆盖过期租约及人工结束。 | 真实节点中断、在已有远端副作用下恢复见阶段四。 |
| 二 · Issue 识别、去重和执行 | beta 实库通过 `GET /projects/1/inbox` 得到普通讨论 [2,1]，任务列表为空。Worker 查询返回 401。Owner 两次转换 #2 均得任务 1，重启后收件箱只剩 #1。另一验收库为 beta #1 加模板标签后，Poller 创建任务 7，按编号同步仍返回同一任务。 | 通过。收件箱实现 `319aa65526b5e5e84fb04caca0300a939ee9a34c`；[工作台转换](#discussion-inbox-ui)实测对应 `abcd802df8806903b9c0cadcdb6e706435304dd9`。 |
| 二 · 验证、页面验收与退回 | alpha 任务 3 重复导入不变。在 Chrome 打开候选差异，退回 `93a7c42e`，真实 Codex 在新工作区补写示例，生成 `785df9cf`。两个候选各有对应 SHA 的 docs 验证。旧批准返回 409，新批准在页面返回 ACCEPTED。 | 通过。两次按 Owner 身份的页面操作均由 Playwright 执行。 |
| 二 · PR、必需 CI、Squash Merge、Issue 回写 | alpha 首次检查失败时 PR 为 OPEN/BLOCKED，任务交付为 WAITING。重跑通过后任务 DONE，Issue 已回写候选及合并 SHA `469cef06b81601ac2f382e230157c2e125787e39`。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/3) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/43815409052) | 通过。beta 再现同一场景，控制面明确指出已验收 head 的 GitHub 检查失败；停止控制面、重跑检查、重启，复用原 PR 合并为 `3b935429ad7588b2e6f4124ac8e4e04d6c80abb0`。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/3) · [检查](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43815783260) |
| 三 · Goal、Milestone、依赖与双重计划记录 | 在收件箱验收库将 beta #2 升级为 Goal 草案，保存两版计划。重报返回原版本，过时改写返回 409。重启后可读取历史，Chrome 展示任务树。beta #4 已走通依赖调度、业务答复、退回、新 SHA 验收、两个子 PR 和父 Issue 关闭，证据见下文。 | 显式 Owner 规划与激活已验收。只读 Planner 已验收（见任务 16）。Management 审核和激活后的滚动修订均在 alpha #4 验收，见下文专题 05–06。 |
| 三 · 决策、事件与恢复 | 子项 15 在验收边界提出业务问题 1，状态变为 WAITING_INPUT。重报只保留一个问题，直接批准返回 409。重启控制面后 Worker 回答返回 401。在 Chrome 以 Owner 回答后，恢复原执行 11、Session `01a1e315-16d2-7f31-8de3-7485865d1c95`、候选 `68d6097a` 和 WAITING_ACCEPTANCE。beta #94 和 #96 在实现回合提问，重启控制面并答复后从同一原生会话交付。 | beta #98 在同一个双子项 Goal 中通过运行中提问、原会话恢复、退回重做及父 Issue 关闭的阶段验收。跨操作的角色与决策分级仍待验收。 |
| 三 · 审查、验证规则与验收方法 | alpha #10 经独立审查后交付。beta #12 经风险分级后交付。Goal 34 的 Management 审核将子项提升至 V4。beta #17 从全局方法生成测试计划、澄清并复审，两项用例实测通过后交付。 | 通过上述候选场景。预览和反馈见下一行。 |
| 三 · 预览、批注与知识 | Chrome、Caddy 双向 TLS、DOM 与 Canvas 批注、六类反馈、阻塞澄清、从反馈退回修订及新候选批准均已实测。beta #19 还验证了从反馈修订需求、采用和再次执行。项目知识的来源、决策索引和审核后的共享副本见专题 08。 | 外部设备与 tailnet、真实提供方投递待验收。动态 local-mock 应用的工作台交互和双向 TLS 网关已实测。扩展原生录屏已验收。已从扩展采集候选视口，在工作台遮挡、标注并归档。React 与 Vue 的同源 iframe 批注已实测。独立新工作项已从反馈 #2 实测到 beta PR #21 合并。 |
| 四 · 多项目授权与密钥 | 节点按项目和环境读取 OpenBao 状态。Broker 在执行租约、用途、版本和 TTL 满足时分发一次性包装响应；脱敏共享和密钥恢复也已实测。 | 生产专用操作、执行资源调度和跨节点恢复见专题 07、09、14。 |
| 四 · 多 Worker、多 Harness 与运维 | 两个真实项目各交付了一个工作项，beta 在控制面停止后重启，复用既有 PR 完成回写。Claude Code 已交付 Owner 明确选择的 beta #22，以及项目配置和标签自动派发的 #24。持久节点登记、心跳和能力查询见专题 07。数据库、反馈截图和工作区恢复演练见专题 14。 | 普通执行的节点容量、跨项目派发及失联占位已实测，见本页末尾。GitHub 四类写入的响应丢失恢复见 beta #27。普通 Worker 的模型与候选 CI 独立名额已在 alpha #20、beta #45 验收。独立角色预算、压力与节点原生会话恢复待验收。 |

MVP 页面验收与 GitHub 门禁证据对应基线、收件箱和交付诊断提交，最后一项为 `32c0fa07cc09eaf98750e588682d0f9701dd92f4`。两个私有项目已在同一 PostgreSQL 中分别派发和交付，并验证项目专用业务凭据的访问边界，见本页阶段四实测。GitHub 操作共用本机 CLI 身份，GitHub App 最小授权仍待验收。

阶段三计划草案实现为 `5d30bcf451d838fae10d684d5c6bd718f5a4f73a`。在 beta 克隆通过 Owner 归档接口分别重复提交修订 1、2，各自返回相同 SHA。Git 快照分别为 `4903725d96065d014e8d4437d6d50b4b71d3bdf5` 和 `933478ea8f00aa0c71797386dc52ec99acd0f0b4`。用 `git show` 读取文件逐字段比对 API 版本成功，工作区状态与 HEAD 前后相同。回归测试还验证了暂存内容保留和引用冲突。

基线回归使用独立 PostgreSQL 测试库执行 `go test -count=1 -json ./...`。44 项测试通过，容器测试因未启用而跳过。前端执行 `npm --prefix web ci` 和 `npm --prefix web run build` 成功。专题待实测与待细化事项详见[环境和协议清单](16-open-decisions.md)。

基座验收对应 ForgeOS 提交 `1a1782246225ab810ca34f3a5c1fd73194242ab8`。随后启动本机 Docker Linux 引擎，启用 `FORGEOS_TEST_DOCKER=1` 单独运行 `TestGoValidationInLocalContainer`，真实容器中的成功与失败案例均符合预期（27.45 秒）。

归档功能对应 `83ea26272aebbb001a4ef0a521075fc0580a1e52`。该版本启用独立 PostgreSQL 和 Docker 后运行全套 Go 测试，48 项通过。另以 HTTP Worker 身份领取任务 11（执行 8）后断开，重启控制面、实际等待两分钟租约到期，原执行仍可查询，尝试领取返回 204；Owner 核对并结束执行后才派发任务 12。

阶段三在 beta 的真实 Issue #4 将修订 1 连续激活两次，只得到子项 14 和 15。Chrome 任务树展示 15 等待 14，并能从子项返回父项 13。子项 14 经 Codex、docs 验证、页面按 SHA 批准后，PR #5 合并为 `e45f9066d9865c64d0702d00320df0f39d5d63ee`。GitHub 查询显示父 Issue 为 OPEN，评论已包含子项 14 的候选和合并 SHA。[子 PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/5) · [父 Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/4)

外部设备通过 Tailscale 的验收保持待验收。2026-11-16 用户确认当前没有可用 tailnet 和第二台验收设备。

Goal 激活与首项交付对应 `94b6b0845c7bbb370701110c1ab2f841fffdc953`。PR #5 合并后启动新版交付器，子项 15 从 BLOCKED 转为 READY，以前置合并提交 `e45f9066d9865c64d0702d00320df0f39d5d63ee` 为基础。通过 Git 对象查询确认其中已有 `docs/status.md`，Worker 随后领取任务 15。数据库回归覆盖未交付时保持阻塞、回写后冻结新基础和实际领取。

依赖释放实现为 `e981f071a0a860fb4b88c0f86e21497b75936c3f`，业务问题检查点为 `70781ba9badb584e94c9594954bcfbcdae2ddbda`。在页面回答后退回子项 15，要求补写恢复条件。第二次执行通过 docs 验证，在 Chrome 确认新增文案并批准 `8dccb433c7cf630c011071b2e58dddb7b7868611`。PR #6 合并为 `5ab5f7f71c697aead887c16438ebb3818bb70fe9`。API 返回父项 13 和两个子项全部 DONE，GitHub 返回父 Issue CLOSED，Goal 完成摘要仅一条。[第二个子 PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/6) · [父 Issue 的两次交付与完成摘要](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/4)

父 Issue 关闭实现对应 `544744520aeb6ea13724c7aaf4166ebf0a5c09b0`。检查规划和派发的并发边界时，用真实 PostgreSQL 锁住 READY → PLANNING 的状态更新，让 Worker 请求在写入时等待。旧实现会在锁释放后把父任务误领走。修复后连续五次数据库回归通过；独立控制面的 `POST /internal/claim` 在锁释放后返回 204，任务保留 PLANNING，执行次数为零。

派发竞态修复为 `b4e7c54b06726fd8949b20faa052f75a0aeb8d1a`。alpha #4 通过按编号导入入口选择规划模式，旧 Worker 的领取请求返回 204。真实 Codex CLI 0.169.0-alpha.11.1 在只读沙箱中完成任务 16，Session 为 `01a1e322-fda7-7751-903d-93519a6cae56`。修订 1 包含两项工作、README 和 CI 上下文、依赖及验收条件。基础 SHA 与导入时一致，Git 工作区干净，草案未自动派发子任务；重启控制面后任务 JSON 相同。[规划输入](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/4)

## 专题覆盖与实现提交

下列短 SHA 属于本地 ForgeOS 仓库。PR 内的 SHA 属于验收项目。历史检索版本 `a31c02ceb9ea5419f356d00aebb2989480a1a28b` 在专用 PostgreSQL 18 实例中启用数据库和 Docker 测试，`go test -count=1 -json ./...` 得到 63 项通过、零跳过，`go vet ./...` 通过。前端生产构建在 `bd1c734` 通过。

| 专题 | 已验证的入口和证据 | 实现 SHA | 仍待补齐或实测 |
| --- | --- | --- | --- |
| 00–01 产品与端到端流程 | [规则分类](#issue-work-type-rules)和[原生分类建议](#issue-classification-agent)已在私库、正式节点及 Chrome 验收。两个私有项目均从 Issue 到已合并 PR；复杂 Goal 13 从版本化计划到父 Issue 关闭。工作台登记、首次检查、固定 SHA 启动验证、版本化初始化报告与 Ruby、Gradle 仓库样例已验收 | `1a17822`、`32c0fa0`、`5447445`、`d57f60d`、`42dee55`、`9a75d9e`、`f724eea`、`d2c094a`、`673295d`、`bbd7773`、`439415c` | 外部设备访问、其他知识归档触发条件、其余初始化场景 |
| 02–03 系统与模块 | 独立控制面、Worker、PostgreSQL、React 工作台、只读规划、交付器、Management 审核、知识查询和静态预览均从实际入口运行。工作台可打开有租约的动态应用，普通 Worker 分别限制模型与 CI 的名额。站内通知及已读回执见 beta #65；当前独立模型角色均已验证共用名额，包括 beta #125 的问题建议及 [#247、#248 的 Issue 分类](#issue-classification-agent) | `1a17822`、`cc2011f`、`f8503c5`、`a31c02c`、`7fe001f`、`99de4f6`、`9a08b44`、`f876721`、`e8a84ea`、`31a46df`、`ff58493` | Telegram、飞书发送及重试已有本机 TLS 证据，真实收件待验收；其余管理协调。普通 Worker 共享账号实测见 alpha #22、beta #115 |
| 04 领域模型 | 同一任务关联历史需求版本、执行、计划修订、候选、验收、子 PR 和 Merge SHA。已归档独立 CI Run ID、检查输出的相对文件名和内容摘要。可下载其他节点的 CI 文件。运行中的 Codex、Claude 在工作台显示节点、PID 与会话，控制面重启后仍可查询（beta #57、#59）。另从[正式节点](#worker-app-turn)对照进程实例、Thread 和 Turn。 | `a7f8b48`、`5d30bcf`、`94b6b08`、`70781ba`、`ee695b3`、`91a150b`、`39d3f68`、`6f9b0c1`、`db6155b`、`1f31794` | 其他跨节点工件及独立审核角色 |
| 05 角色与决策 | 任务 15 等待业务答复；Goal 16 的 Management 审核退回两版、批准第三版。跨项目验收问题的批量答复、Owner 按原生审核意见修改并批准指定修订见下文。换基后的三个角色与开发会话各自独立，见 beta #88。原生业务问题、Management 建议和 D1 项目决策查阅见 beta #94–111。已对照[五类 ACP 业务路由](#protocol-route-matrix)，并从审批中心处理[有来源的审核等级](#approval-source-levels)与[原问题的 D2/D3 答复](#approval-questions)。 | `70781ba`、`f8503c5`、`abb5257`、`d9fc283`、`a6f05da`、`85d9c5c`、`2672efc`、`0fad34a`、`5c5657b`、`913caea`、`d329bba` | 其余业务操作的 D0–D3 策略 |
| 06 规划与编排 | 任务 16 从 Planner 草案、独立审核到运行中修订和两次子 PR 合并；Goal 13 显式激活。真实 Issue 评论分批进入回合，预览阻塞问题及候选退回在验收检查点处理。来源关闭取消、工作台取消及暂停后恢复同一候选的 CI 已实测（beta #52、#54、#55）。Owner 从工作台修订、批准并归档计划，beta #67 完成子项及父 Issue 交付 | `cc2011f`、`5d30bcf`、`83ea262`、`94b6b08`、`e981f07`、`f8503c5`、`f519840`、`4d31e7f`、`bf27241`、`7cbd8a3`、`a5d2193`、`84758c5`、`a221a0d`、`d9fc283`、`a6f05da`、`616dcb6` | Codex 实现回合暂停和同会话恢复见 beta #137。只读 Planner 主动暂停见 beta #19。只读 Planner 已完成跨 Thread 交接，见页末。Codex 和 Claude 的首轮暂停草稿交接分别见 beta #145、#149。整机重启及其余写入回合交接待验；下一回合指令已验收，原生取消见 beta #131、#132 |
| 07 Worker 与 Harness | Windows 原生 Codex 和 Claude Code 实现任务、只读规划、失联租约及评论检查点；Ubuntu 非 root systemd。独立探针已验证 Claude Code 会话恢复、Codex App Server 会话恢复和中断。已验收节点登记与普通执行容量、模型终态回执丢失后的恢复、运行中与结束后的进程记录及工具调用（beta #61、#63）。beta #127、#129 的原生 Claude、Codex 实现进程被终止后在原会话继续，beta #19 的只读 Planner 也在同一 Thread 恢复并保存草案；Linux 服务已探测两种原生 CLI。beta #155、#157 已验证等待业务答复时原会话丢失后的交接，beta #159、#161 已验证未主动暂停的写入进程崩溃后的会话交接。已验证[原生运行中输入](#native-steer-probe)、[中断回退](#native-interrupt-fallback)、[工作台的原生回合编号](#worker-app-turn)、[Claude 工作台中断](#claude-worker-interrupt)及[完成写入回合后恢复排队指令](#completed-writer-input)。 | `1a17822`、`cc2011f`、`b4e7c54`、`4d31e7f`、`bd1c734`、`1d9617e`、`fe35218`、`b11089b`、`611850e`、`9f6d396`、`e8a84ea`、`d78ae18`、`9f83fff`、`db6155b`、`3c9226b`、`c18af6d`、`332568f`、`9da6a8e`、`c1b4847`、`a77cf1c`、`cfe0bbc`、`bb78b3c`、`1f31794`、`2023af5`、`2d991f2`、`f8f9e8d` | Claude 暂停恢复后的交付、草稿首次保存前及存活进程接回、整机重启与 Linux 服务身份 |
| 08 项目知识 | 固定版本工程事实、历史检索、私有资料与跨项目脱敏审批已验收。节点可查询固定需求、计划与依赖；原生实现回合的使用见 beta #69。确认合并及已完成 Goal 的历史检索见 beta #72。资料导入和 Owner 记忆请求已从原生提取到独立审核；提案修改、确认、拒绝及固定来源见 R17、R18 验收。子项引用有效决策见 beta #74，确认合并和 Goal 完成触发原生提取及独立审核见 beta #76、#78；Git 文档变化与全局规则发布见下文双项目实测。规划与实现的规则优先级见 beta #81、#83，独立审查使用的候选依据见下文。测试计划及覆盖审核使用相同的候选依据 | `cc2011f`、`a31c02c`、`5225e60`、`7ae7254`、`e80e961`、`4713fdc`、`f7a9bbe`、`a6c797e`、`4bfc017`、`4be0ceb`、`2df857e`、`382aa91`、`2359fe3`、`ddbaae2`、`4d1d737`、`44a77fc`、`6237e70`、`8b2dbf1`、`07aeae4`、`a5309b4`、`766d452`、`3744ecc`、`e566c17`、`feba4f4`、`da40dd4`、`e6fb688` | [项目决策](#decision-search)、项目资料及 D1 经验的混合检索、审核回执、固定 Git 版本的工程文档查询已验收，见页末。六类角色已从[原生会话](#knowledge-role-matrix)回核。[四类项目历史的混合检索](#history-hybrid-search)和[固定 Git 版本的工程文档混合检索](#git-document-hybrid)均已验收 |
| 09 权限与密钥 | Owner/Worker 分权、项目和环境成对授权、OpenBao 真实部署与恢复、状态接口、执行租约与用途绑定、一次性解包、签发与失败审计。具名 Worker 在活动租约中轮换令牌与 mTLS 客户端证书 | `1a17822`、`319aa65`、`70781ba`、`c513401`、`ab013b6`、`72cd89d`、`8c83ac8`、`266f11f`、`40736ea` | 生产专用操作策略及其他节点身份场景 |
| 10 审查与验证 | 真实 Docker V1–V4 检查、GitHub Status、Actions 门禁、重新验证、独立审查和页面验收。Issue 强制风险等级，Management 可提高下限。任务页跨节点下载 CI 文件（beta #49），中央及 Windows、Linux 节点按策略清理，真实候选 CI 的取消和恢复见专题 06。beta #88 和归档的 #55 已核对实现会话及重验来源 | `1a17822`、`32c0fa0`、`85cd1cb`、`9400068`、`427bc24`、`35866ba`、`ee695b3`、`80836d6`、`749351c`、`fdc4192`、`48a32f2`、`0420f45`、`6f9b0c1`、`b465048`、`9f0c704`、`84758c5`、`a221a0d`、`85d9c5c` | 其他验证器与独立审核角色场景 |
| 11 预览与人工验收 | 工作项归档计划、覆盖审核及用例实测；Caddy、静态预览、DOM/Canvas 和同源嵌套 iframe 选择、用例关联、六类 Management 分类、阻塞澄清、按反馈退回和批准、从反馈修订需求及派发独立工作项（beta #19–21）、工作台 PNG 标注与遮挡归档。扩展截图、标签页录屏及归档回放，动态应用预览，隔离网络中的独立 Staging 网关均已实测。测试计划的规则和候选 SHA 在 beta #88、#91 复验 | `1a17822`、`70781ba`、`fdc4192`、`86aff28`、`7565fa6`、`e71b7c8`、`978d184`、`71e6c9a`、`7fe001f`、`ec7a462`、`17bb819`、`c561e40`、`99de4f6`、`bf27241`、`7cbd8a3`、`5a5d685`、`d34260b`、`2b805b4`、`5279f11`、`c382473`、`732a4a8`、`6280acd`、`a6ade1d`、`9a08b44`、`f876721`、`ba4a31d`、`da40dd4`、`36ebbfd` | 真实提供方回调投递、外部 tailnet 设备访问 |
| 12 GitHub 接入与交付 | 标签派单、普通讨论转换、受保护分支阻塞与合并、子 PR 按序合并、来源需求修订与检查点换版、目标分支前移后重新验证与交付。签名 Webhook 原文归档与补充轮询见 beta #51。来源 Issue 关闭后终止候选 CI（beta #52）或正在运行的原生模型及工具（beta #131） | `319aa65`、`32c0fa0`、`94b6b08`、`5447445`、`a7f8b48`、`20c30e6`、`391309e`、`4d31e7f`、`e2912c5`、`4ea0363`、`a5d2193` | 细粒度凭据、第二人审查 |
| 13 数据与审计 | 数据库迁移、重启恢复、幂等事件、Git 计划快照、交付证据链与反馈截图校验回读。已从配套备份恢复节点 SQLite 并补报。中央与节点 CI 文件、反馈截图及录屏均有保留与到期证据。四类独立角色还已验证[冻结输入与私有工作目录到期清理](#role-recovery-retention)。 | `1a17822`、`83ea262`、`5447445`、`c382473`、`ab36ad9`、`75b1495`、`b465048`、`9f0c704`、`b5f3992`、`6280acd`、`2bbf2cc` | 中央进程日志全文、完整审计载荷及其余进程和会话事件缓冲 |
| 14 部署与运维 | Windows 主机、Docker Linux 引擎、Ubuntu systemd 重启与派发；PostgreSQL、OpenBao 和反馈截图备份恢复；Caddy 双向 TLS 网关。恢复工作区后已用独立 Worker 继续派发。CI 文件与原始回执、Worker SQLite 与配套数据库的恢复见本页。具名节点与独立角色均已实测共享账号名额；Linux 服务重启后能读取实际 CLI 版本 | `1a17822`、`32c0fa0`、`bd1c734`、`06ed00b`、`ab013b6`、`ec7a462`、`42ba820`、`ff5d714`、`ff58493`、`c18af6d` | 普通 Worker 的模型与候选 CI 独立名额见 alpha #20、beta #45。普通 Worker 的跨节点共享预算见本页 alpha #22、beta #115。外部设备私网访问、Linux 模型登录复用、其他工件恢复、失联模型会话恢复待验收 |
| 15–16 路线与开放项 | 基座、单项目闭环、两子项 Goal 场景、第二个项目交付有以上记录。当前可从[28 项逐项对照](16-open-decisions.md#requirement-status)及[18 份 ADR](adr-acceptance.md)回到实测。 | 本表所列提交、`0899cf3` | [开放事项 A–C](16-open-decisions.md)中其余真实环境与协议要求，以及全路线最终回归 |

## 专题 05–06 · Management 对计划修订的真实审核

在 alpha 私有项目启动控制面和独立的 `worker management`，使用原生 Codex、独立令牌与 PostgreSQL。普通 Worker 访问审核领取入口返回 401。[输入 Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/4)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 自动领取 Goal 16 的修订 1，沿用默认 D2 | 提供审核依据，等待 Owner | 会话 `01a1e32e-b07c-73a0-91f1-54337bd91c4a` 要求核实 README 与 CI 断言。状态 PLANNING，零子项 |
| 保存修订 2，并明确授权 D1 | 审核退回时保留草案 | 会话 `01a1e32f-1ec6-7612-b741-5a9a5160cdb5` 再次指出证据缺口，零子项。重启控制面后仍可读取历史审核 |
| 从固定提交 `469cef06b81601ac2f382e230157c2e125787e39` 核实两个文件，将摘录和检查步骤纳入修订 3 | D1 批准后派发独立项并阻塞依赖项 | 会话 `01a1e330-3dec-73d2-bb84-d017aa401852` 批准，Goal ACTIVE，一个 READY、一个 BLOCKED。Chrome 的任务详情显示 D1、依据和同一会话 ID |
| PostgreSQL 集成测试提交过期租约、旧版结果、跨项目结果、重复结果和试图降低 D3 的建议 | 过期和越权写入被拒；重放不重复派发，D3 留待 Owner | 全部通过。启用 PostgreSQL 与 Docker 的回归 52 项通过、零跳过；Go vet、前端生产构建通过 |

## 专题 06 · 已激活 Goal 的滚动修订

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在真实 Worker 执行任务 17 时替换其范围 | 拒绝变更，保留该执行 | HTTP 409。执行 14 继续，并以候选 `3583fb690a323a556a478dc3d46b146a4cb5606d` 通过 docs 验证 |
| 保存修订 4，更新未执行的任务 18，新增检查项 | 审核期间沿用修订 3；批准后按 key 调整依赖 | Goal 16 显示 PLANNING、active_revision=3、两个子项。批准后恢复 ACTIVE；17 仍属于修订 3，18 保留原 ID 并绑定修订 4，新项 19 等待 18 |
| 修订 5 移除暂列的检查项，重启控制面 | 当前计划反映移除，历史版本仍可读取 | 19 为 SUPERSEDED；修订 4 有三个里程碑、修订 5 有两个。18 的内容和创建记录仍指向修订 4 |
| 完成第一个子项 | 保留原执行，父 Issue 保持开放 | 任务 17 的 PR 已合并，Merge SHA 为 `1dea711e585b05dc1d6aa4a79cc9cf1c96e565ab`；任务 18 开始执行时父 Issue 仍为 OPEN。[PR #5](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/5) |
| 从旧的依赖检查返回结果、覆盖已交付子项、在关闭租约期间再提交计划 | 各次写入被拒，旧结果不能放行已改版任务 | PostgreSQL 集成测试通过，已交付子项的执行 ID 和 Merge SHA 保留。全套 PostgreSQL/Docker 回归 53 项通过、零跳过 |
| 完成修订后的任务 18 | 从前置 Merge SHA 开始验证，两个必需子项交付后关闭父 Issue | 执行 15 基于 `1dea711e585b05dc1d6aa4a79cc9cf1c96e565ab`，候选 `8ee909d7b313ca88cb3877ced22b44102ff69d65` 经 docs 验证与必需 CI 后合并为 `b179f04fec439a392af1fe1504450931f9f1f203`。Goal 16 为 DONE，Issue 已关闭，回写计数为 2。[PR #6](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/6) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/43823799864) · [父 Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/4) |

## 专题 04、12 · 来源需求修订与执行绑定

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从 GitHub 导入 alpha #7，然后在真实 Worker 执行时修改正文并重复导入 | 工作项保持唯一，修订 2 可读取，执行继续使用版本 1 | 任务 20，执行 16，绑定 Requirement ID 5。两个版本都能查到相应正文；候选 `114d73ef1e26ed33eaf2cc58b6b74fe5b56a03dc` 通过 docs 验证。[输入 Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/7) |
| 再改一次无派单标签的 Issue，启动真实轮询器，打开 Chrome 任务详情 | 自动观察修订 3，浏览新旧正文，既有候选保持待验收 | 页面显示三版，可从第 3 版回看第 2 版。原工作项仍为 WAITING_ACCEPTANCE，只有一次执行、两条需求变化事件 |
| 重启控制面，重放同步，查询既有子任务 18 的需求版本 | 没有新增执行或修订，既有子执行可追溯到原 Goal | 任务 20 保持最新版本 3、执行绑定 1；18 的执行与 Goal 16 原始需求对应同一个 ID |
本次 PostgreSQL 与 Docker 回归 54 项通过，零失败、零跳过。Go vet 和前端生产构建通过。


## 专题 06、12 · 在验收检查点采用新需求

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 Chrome 打开任务 20 的需求历史，阅读第 3 版并选择采用 | 保留工作项，重新入队；重复提交不产生第二次转换 | HTTP 200，任务进入 READY。再次采用后只有一个 `requirement_adopted` 事件 |
| 提交已过时的修订 2，或尝试批准修订 1 的旧执行和旧 SHA | 拒绝两次提交 | 两次均返回 409。PostgreSQL 测试还验证了运行期间拒绝更换需求，换版后不会带入旧版退回意见 |
| 启动真实 Worker，检查差异，再重启控制面 | 新执行绑定修订 3，旧版本仍可追溯 | 原执行 16 保留候选 `114d73ef1e26ed33eaf2cc58b6b74fe5b56a03dc`；新执行 17 产生 `c789c99feb7edd751f3de912796bb679099b6566`。差异包含修订 3 指定的完整句子，docs 验证通过，重启后两次执行均可查询。[验收 Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/7) |

## 专题 06、12 · GitHub 评论采集

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 向 alpha #7 添加两条普通评论，启动真实轮询器 | 保存来源与正文，运行状态保持原状 | 任务 20 出现事件 1、2，来源评论 ID 分别为 6910489666、6910490346。任务仍为 WAITING_ACCEPTANCE，执行 17、需求修订 3 均相同。[第一条评论](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/7#issuecomment-6910489666) |
| 编辑第二条，重启并再次扫描同一 Issue | 旧正文可追溯，重复项去重，游标可读取新版本 | 历史为事件 1、2、4。游标 2 仅返回事件 4，新旧正文均保留。另一个 Goal 查询为空，普通 Worker 读取 Owner 接口返回 401。[第二条评论](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/7#issuecomment-6910490346) |


## 专题 06、07、12 · 原生回合间的评论合并

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 退回任务 20 的候选，让旧 Worker 和新 Worker 分别领取 | 由支持检查点的 Worker 处理已有评论 | 旧 Worker 返回 204。新 Worker 开始执行 18，批次 0 截至事件 4，两条有效评论只包含各自最新正文 |
| 在第一个 Codex 回合进行期间向真实 Issue 添加第三条评论并轮询 | 当前命令继续；回合结束后才读取新信息 | 批次 0 在原生会话 `01a1e356-8989-7270-8a48-67fb7043678c` 完成。批次 1 处理事件 7，另一个原生会话 `01a1e357-2cda-7932-89c6-770526eeae30` 在同一工作区续做。[回合期间的原评论](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/7#issuecomment-6910610401) |
| 提交前再取批，在 Chrome 打开检查点和候选差异 | 零新增时提交；候选应覆盖修订 3 和讨论内的补充示例 | 批次 2 截至事件 7，零新增。页面显示三个批次的评论数 2、1、0。候选 `7daa15661a39d02287b2750a4c3936637278a802` 含修订 3 句子、`forge status` 和 `Waiting tasks remain visible.`，docs 验证通过 |
| 重放批次、跨租约读取、带着未处理评论提交成功结果 | 同一批次分页读取稳定，错租约和未处理完的回报被拒 | PostgreSQL 测试通过。启用 PostgreSQL 和 Docker 的全套回归 57 项通过、零跳过，Go vet 和前端生产构建通过 |
| 通过 Chrome 批准该候选并查看远端门禁 | 必需 CI 和分支保护分别判定 | PR #8 的必需 CI 重跑后成功，目标分支已前进，GitHub 状态为 BEHIND，交付保持 WAITING。新基线上的验证与验收另行记录。[PR #8](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/8) · [CI](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/43827169232) |


## 专题 10、12 · 严格分支保护下的新基线验收

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 查看 PR #8 检查与目标分支保护 | 已成功 CI 仍不能放行落后的候选 | `acceptance-check` 成功，保护配置 strict=true，GitHub 为 BEHIND，任务保持 WAITING |
| 在 Chrome 选择重新验证，再重放同一请求 | 原差异应用到最新目标，保留原验收记录，旧候选不能直接放行 | PR #8 已关闭且未合并。归档条数为 1，原执行 18 的差异仍可读取。新基线 `b179f04fec439a392af1fe1504450931f9f1f203`，新候选 `deb856d2e1e1b0c1e0102869998450c396899a5c`。[原 PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/8) |
| 让旧 Worker 领取，再启动具备验证能力的 Worker | 旧 Worker 返回 204；新 SHA 单独验证、重新人工验收 | 执行 19 检出固定 SHA，docs 检查通过，未启动模型。Chrome 同屏查看历史交付与新差异，重新核对三项内容后按新 SHA 批准 |
| 在新 PR 上运行必需 CI 并回写 | 新候选获得独立远端检查和合并记录 | 新 PR #9 的必需检查先失败，按同一 head 重跑通过，Merge SHA 为 `15ebbfc2dc682075de1e8c9d96f42346a00a1c88`。任务 20 为 DONE，Issue #7 为 CLOSED。[PR #9](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/9) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/43828534029) · [Issue #7](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/7) |


## 专题 09、14、15 · 多 Worker 项目授权

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 为 alpha-node 授权项目 2，beta-node 授权项目 4，先排 beta 再排 alpha | 凭据和 principal 匹配，只能领取允许项目的工作 | alpha 以 beta 名称领取返回 403。改回本名后领取 alpha 的工作，beta 的较早任务仍在队列；alpha 再领取返回 204 |
| 将上述执行的有效租约与 beta 凭据组合，读取检查点或回报结果 | 跨身份请求应被拒，原身份可正常回报 | 两个跨节点请求均为 403，原节点回报受控失败返回 204 |
| 分别启动两个真实 Daemon，配置各自项目仓库、独立工作区及秘密令牌 | 查到与项目、节点、执行和实际检出 SHA 一致的记录 | beta 完成任务 24、执行 21，返回 `3b935429ad7588b2e6f4124ac8e4e04d6c80abb0`，期间 alpha 工作仍为 READY。alpha 随后完成任务 26、执行 22，返回 `469cef06b81601ac2f382e230157c2e125787e39`，均与各自 Git HEAD 一致 |

## 专题 07、14 · Ubuntu systemd 服务与重启

在 WSL Ubuntu 的 systemd 系统实例安装本仓库的两个 unit，使用 Linux amd64 二进制、独立 PostgreSQL schema 和本地 Git 项目。验收完成后清理临时服务、账号、工作区和 schema。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 校验并启动 unit，检查 MainPID、UID、HOME、工作目录和 Worker 环境 | 以不同非 root 用户运行，HOME 权限为 0700，Worker 只持节点凭据 | 控制面与 Worker 为 UID 997、995。两者工作目录均为各自 HOME，代理绕过回环地址，Worker 进程环境中无 Owner 令牌和数据库连接 |
| 在控制面所有的组共享克隆上派发 git_head，随后给 Worker 配置该克隆的精确路径 | 默认 Git 所有权检查生效，显式授权后节点完成真实检出 | 原任务 1 失败，Git 报 dubious ownership。授权后任务 2 为 DONE，工作区 `/var/lib/forgeos-worker/work/execution-2` 归 Worker 所有，结果与 HEAD `9b09c97060aeec20b3779a5db346f89c64292fa2` 相同 |
| systemctl restart 两个服务，通过 Owner API 重新查询并派发新任务 | 原记录完整，新进程可以继续工作 | PID 从 2249、2250 变为 2283、2284。任务 2 的完整响应前后一致；任务 3 由新 Worker 完成并返回同一 SHA |

该版本启用 PostgreSQL 和 Docker 的完整 Go 回归为 61 项通过、零跳过。`go vet ./...` 与前端生产构建通过。Linux 专用用户下的 Codex 和 Claude 登录与会话恢复仍待验收。

## 专题 13–14 · 从 PostgreSQL 备份恢复到全新实例

使用 ForgeOS `bd1c734d80413c87e032cb250c12baea9ad314cc` 与 PostgreSQL 18。源库包含两个真实 GitHub 项目的交付历史。以下演练未启用轮询、Worker 或交付器。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 用 pg_dump 创建 custom 格式备份，列出归档目录，再 initdb 启动独立实例 | 能读取备份，目标库中尚无应用表 | 备份 60,275 字节，SHA-256 为 `380414258a6f3b6232c29deca6445584ffb95cc78be33e1e62803cd955e8351b`。恢复前目标 public schema 表数为 0 |
| pg_restore 单事务恢复，连接目标库启动控制面 | 数据结构、事件和可读历史与源库一致 | 全部 15 张应用表计数相同，包括 19 项任务、22 次执行、120 个事件和 7 次交付。逐字段比较 24 个 Owner API 响应均相同，覆盖全部任务详情、列表、任务 20 的评论和检查点、需求历史及 Goal 16 的计划修订 |
| 核对已合并交付，再从恢复控制面新建任务 | 显示原先合并的候选，恢复库可继续写入 | 任务 20 保留 PR #9、候选 `deb856d2e1e1b0c1e0102869998450c396899a5c` 和合并 SHA `15ebbfc2dc682075de1e8c9d96f42346a00a1c88`。新任务 27 为 READY，在源库查询该 ID 为 0 行 |

PostgreSQL 恢复演练对应提交 `06ed00b8e47fa764e31611b655fbc5ca9e7fa2d1`。

## 专题 08 · 按项目检索历史来源

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 以 Owner 和 alpha 节点凭据分别查项目 2，参数为 `source=comment&task_id=20&q=requirement+revision` | 只检索该项目的源记录，两种授权入口返回一致结果，可区分原文和修订 | 两个响应逐字段相同。[同一条真实评论](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/7#issuecomment-6910490346) 返回事件 4（latest）和事件 2（historical），均带时间、引用与各版本原文摘要 |
| 以 beta 节点查本项目相同评论，再搜索 `source=requirement&q=queue` | 根据来源和项目筛选 | 评论查询返回空数组；需求查询仅返回项目 4 的任务 7 和 13 |
| 交换两个节点的目标项目，再尝试无效来源和负任务编号 | 授权失败时拒绝访问，参数错误可区分 | 两个跨项目请求均为 403；两个非法参数均为 400。数据库测试另以同一关键词和中文子串验证项目隔离与旧版本匹配 |

<a id="check-a-2"></a>

## 专题 07、16 A.2 · Claude Code 原生权限交互与恢复

在 Windows 已登录账号执行 `python scripts/probe_claude.py --bin <claude.exe>`。CLI 报告版本 2.1.310，使用原生 stream-json 双向通信。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 用进程 13936 启动会话，等待 AskUserQuestion 并通过相同 request_id 回答 | 工具交互到达宿主，模型得到选择的答案 | 收到 can_use_tool 请求，宿主选择 Green 并回传；同一 Session ID 为 `90231784-7dea-4ee8-994f-552af586ab52` |
| 拒绝对临时目录 probe.txt 的 Write 请求 | 调用受权限门禁控制，文件未落盘 | 宿主返回 deny；任务结束后实际检查文件不存在 |
| 退出进程，使用 --resume 在新进程 44392 恢复 | 新进程返回相同 Session ID，并能同时回忆第一轮随机标识和已选答案 | 第二次结果与随机标识和 Green 完全匹配。此项覆盖协议探测，Worker 自动恢复、Linux 登录与并发仍需独立验收 |

Claude Code 原生探测对应提交 `1d9617e11095434c41f68956f6e8a0d45c014108`。

<a id="native-thread-interrupt"></a>

## 专题 07、16 A.1 · Codex App Server 线程与中断

在 Windows 本机从安装的 `codex-cli 0.169.0-alpha.11.1` 生成协议 schema，运行 `python scripts/probe_codex_app_server.py --bin <codex.exe>`。两个服务器均使用 stdio。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 初始化、新建只读 Thread，发送携带随机标识的文本，关联原生事件 | 服务确认只读和 never 审批，首轮正常完成 | PID 1580 建立线程 `01a1e385-6183-7192-8b93-23dee8be39b2`。首轮 `01a1e385-63fe-7372-927d-1b0f9de0850b` 收到 started、agentMessage 和 completed |
| 关闭服务器，在新服务器按原 Thread ID 恢复，上下文不重发 | 返回同一线程，模型能答出上一进程提供的标识 | PID 47960 的轮次 `01a1e385-8d3d-7fb3-8ca3-b5526ebdd9cd` 返回完全相同的随机值，服务确认轮次完成 |
| 对第三轮发送中断，核对同一 Thread/Turn 的最终状态 | 运行中的 Turn 变为 interrupted | 在 start 响应后立即中断曾得到协议错误 -32600。改在收到 turn/started 后发送，轮次 `01a1e385-b2b7-7c41-83b9-f563f4d56966` 的 turn/completed 状态为 interrupted |

上述 App Server 协议验收对应提交 `fe35218b32cf0bb296173ef0bc18b9a8a7e17d4a`。

<a id="adr-evidence-018"></a>

## 专题 08、ADR-018 · 有明确确认和依据的项目决策索引

在已有真实项目和交付记录的 PostgreSQL 18 实例上升级迁移，从控制面 HTTP API 执行以下操作。提案依据为 beta 任务 15 的第 1 个业务问题及其交付记录。[依据 PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/6)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 提交有依据和适用范围的提案，重报后查询默认索引 | 提案唯一，Owner 确认前不进入有效决策集 | 提案 1 为 PROPOSED，decided_by 为空。相同请求返回相同提案，改写同一 key 返回 409。Worker 尝试确认返回 401 |
| 通过 Owner 入口确认，再用两个节点读取；尝试从其他项目引用旧决定 | 只有获准项目的节点可读，确认身份可追溯 | beta 可读，返回 decided_by=owner；alpha 查 beta 返回 403。在项目 2 创建替代 beta 决策的提案返回 404 |
| 提案 5 替代决定 1；再尝试确认另一份指向决定 1 的提案 6 | 替代关系仅在确认时生效，原决定只能被替代一次 | 提案 5 创建后决定 1 仍然有效。确认 5 后，1 的历史依次为 PROPOSED、CONFIRMED、SUPERSEDED。确认 6 返回 409，Owner 可将 6 退为 REJECTED |
| 将决定 5 置为 EXPIRED，重启控制面并按状态查询 | 失效决定退出默认索引，历史和依据完整保留 | 重启前后 Owner 的完整历史响应相同，beta 节点读取结果也一致。真实数据库测试另覆盖两个替代提案并发确认和截止时间自动失效 |

该功能启用 PostgreSQL 与 Docker 的全套 Go 回归为 64 项通过、零跳过。`go vet ./...` 通过。

决策索引实现对应 `5225e601300177bc0a768fbc5d144c3a72c1e4f0`。

## 专题 08 · 私有项目资料导入与版本查询

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 用 Owner PUT 入口将 UTF-8 运维记录导入 alpha，在 beta 使用相同 key 导入不同内容 | 两项目分别保留原文与校验值 | alpha 的 `handbook-ops-89ebca61` 修订 1 与本地文件逐字节计算的 SHA-256 一致，为 `34c5f2dd73bcc3ecafdaa5ff211cd5ed2498cfa3fdbae0dc42f06a10300b3d76`。beta 节点在本项目搜索 alpha 独有词返回空 |
| 导入项目档案、环境说明，以项目节点按类型和词查询 | 可以分别获取本项目的两类资料，节点仅有读权限 | alpha 查询命中对应的 profile 和 environment。交换目标项目后的两个读取请求均返回 403；节点尝试写入返回 401 |
| 发布修订 2，再重发原始请求、尝试改写和跳号 | 新版本用于检索，旧版本可按固定引用读取，冲突写入被拒 | 修订 2 的 SHA-256 为 `30175a8b30190f4c80cf9daa809133750b54c7dae41d9086ac1dbf20bec17b5b`。重报原始请求仍返回修订 1；改写与跳号均为 409。控制面重启后两个版本的完整 JSON 与重启前相同 |

资料导入实现对应 `7ae7254394ae1f6bd4279b2314a4f9e035ac38a2`。

## 专题 08、09 · 从 alpha 向 beta 发布脱敏副本

在控制面、PostgreSQL 和真实 `worker management` 上，以 alpha 资料 `handbook-ops-89ebca61` 的修订 1 为来源。beta 的节点只获准查询项目 4。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 尝试复制整个原文，然后提交含测试用私有标识的摘要 | 全文复制被拒，保留私有标识的摘要不能发布 | 全文返回 409。副本 1 的原生 Management 会话 `01a1e393-f0bc-7bc1-8836-171ecb259479` 要求修改。Owner 尝试发布返回 409 |
| 提交通用操作经验供独立审核，核对发布前的接收项目 | 有依据的脱敏副本可以通过内容审核，随后须经 Owner 显式批准 | 副本 2 返回 needs_owner，保持未发布。澄清两种审批的职责并另交副本 5 后，Management 会话 `01a1e395-0306-71e3-89ce-dde46eb05fab` 判为 approve。beta 仍查不到副本；错误摘要发布返回 409，Worker 发布返回 401 |
| Owner 按审核摘要发布，重报并用 beta 凭据查询 | 接收方获得被批准的副本，源资料和审核内容维持私有 | 重报复用相同发布时间。beta 只收到 ID、标题、正文、发布时间和摘要 `8e096f1715403fecc316a78594d596acb452279b966754632d4c904963b0d764`。直读 alpha 私有资料返回 403，私有审核入口返回 401 |
| 重启后重新查询，再按原摘要撤销、尝试重新发布 | 恢复保留发布状态，撤销收回后续读取 | 重启后副本内容一致。撤销后 beta 查询不到该 ID，复用已撤销审批发布返回 409 |

该版本启用 PostgreSQL 和 Docker 的 Go 回归为 66 项通过、零跳过。`go vet ./...` 通过。

脱敏副本审核与发布实现对应 `66a84f646b54a18f7b4d2247e858d21ecf052c96`。

<a id="check-a-9"></a>

## 专题 09、14、16 A.9 · OpenBao 部署、隔离与恢复

在本机 Docker 用仓库中的 Compose 和 HCL 启动真实 OpenBao 2.7.2。使用短期 TLS 证书、随机验收值以及两个独立的单节点实例。结束后清理临时容器、数据卷和测试私钥。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 初始化三个解封份额，每次提交一个，通过 TLS 写入两个项目及两种环境的 KV v2 数据 | 一个份额保持 sealed，第二个可解封；各路径和版本独立 | 达到 2/3 后正常服务。项目 2 的 local-mock 同一键分别写入版本 1、2；staging 和项目 4 的同名键保留独立值 |
| 以仅有项目 2 local-mock 精确读权限的身份尝试跨环境、跨项目和写入，撤销初始 Root Token | 只读本路径，其他请求被拒 | 三类越权操作均为 403，Root Token 撤销后读取也为 403。独立限时备份身份仍可取快照 |
| 获取 Raft 快照并导出独立文件审计，再用独立恢复证书分别进行 CMS AES-256-GCM 加密并解密核对 | 数据与审计能独立恢复，审计不含测试密钥、令牌和解封份额明文 | 快照 26,137 字节；审计 44,924 字节，共 35 条，含拒绝记录和 HMAC。两份解密内容与归档前逐字节相等。密文 SHA-256 分别为 `3ede5ce266f4396f8356c003c228be869aa40b024e377364889cf97f09470aa4` 和 `9837969f98c1ef2e0bf2cdcd81614f7afe0c42af1882dde9c8ede947d519cb59` |
| 移除源容器和源数据卷，在独立空 Raft 实例恢复，使用源份额解封 | 旧数据、版本和权限策略恢复，目标初始化身份不再有效 | 恢复前目标键为 404。导入快照后节点封存，用源份额 2/3 解封。两个历史版本均匹配原随机值；跨环境和跨项目仍为 403，目标引导令牌返回 403 |

OpenBao 部署及恢复对应 `ab013b603c4bb3368e1daf777dcae7b6f3599b02`。

## 专题 09 · Go Secret Manager 的环境权限与实时状态

验收使用独立 TLS OpenBao、真实 Go 控制面、PostgreSQL、两个项目和三个节点身份。项目 2 的 local-mock、staging 及项目 4 的 local-mock 使用相同名称 `status-fd4d9c2f`，分别写入不同的随机值。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 通过 Owner 入口登记三个环境键，尝试附加 value 字段 | 登记风险与环境，不接收值；未写入时显示 MISSING | 相同风险重报的元数据相同，三个未写入路径均显示 MISSING；含 value 的注册请求为 400 |
| 写入两个版本，删除、恢复、销毁当前版本；用控制面和 alpha 节点读取 | Owner 和有对应环境授权的节点实时看到同一版本，普通查询只返回状态 | 版本 2 的状态依次为 AVAILABLE、DELETED、AVAILABLE、DESTROYED；各次响应只含项目、环境、名称、风险、更新时间、版本与状态 |
| 改用其他节点和环境，或直接用控制面元数据身份读 OpenBao 数据路径 | 仅授权的项目环境可读，元数据身份无法读取值 | 同项目跨环境、跨项目、仅有项目授权三个场景均为 403。beta 的合法请求保留其独立 S0 风险。元数据身份直读 data 路径也为 403 |
| 写入无效身份文件，替换为新的限时令牌并重启 | 失效身份不误报为缺失；进程内可轮换，重启后状态一致 | 请求先返回 503，替换后在同一进程恢复，重启结果与先前相同。扫描 public schema 的完整数据归档及控制面日志，均未出现四个测试值和两次有效元数据令牌 |

该版本启用 PostgreSQL 和 Docker 的 Go 回归为 67 项通过、零跳过。`go vet ./...` 与前端生产构建通过。

环境密钥状态对应 `72cd89d8f6ddeb4e5674805a2ee61afcd5225ac8`。

## 专题 09 · 执行绑定的限时凭据领取

通过真实控制面 API、PostgreSQL 和 TLS OpenBao 2.7.2 验证。alpha 节点领取项目 2 的任务 27、执行 23。测试用签名值仅写入独立 OpenBao。控制面分别使用只读元数据身份与强制响应包装的 Broker 身份。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| Owner 对正在运行的 alpha 执行批准固定版本、签名用途和一次用量，再重报相同请求 | 复用同一授权。其他项目、同项目其他节点、错误租约和用途不能领取 | 复用授权 1。跨项目 403，其他三项 409；Owner 查询使用记录为零。Broker 身份不带包装头直读 OpenBao 返回 403 |
| 同时发起两次领取，用得到的包装令牌解包并校验随机挑战的 HMAC | 只返回一个有效包装令牌，可以执行获准操作，重复解包失败 | 并发结果为 200 和 409。签名比对成功，第二次解包被拒。回执仅有一次 ISSUED |
| 创建 8 秒授权，等待 9 秒后解包；撤销另一份授权；模拟 Broker 身份无效 | 过期不能解包或再领取，撤销生效，后端失败可审计 | 到期后解包失败，再领取 409。撤销后领取 409。无效 Broker 身份返回 503，审计记录 FAILED |
| 在 execution-23 的独立 Worktree 核对 HEAD 后提交结果，重启控制面；尝试生产环境和 S3 | 任务结束后租约不能签发包装令牌，敏感凭据保留专用策略 | 工作项 DONE，结果为 `469cef06b81601ac2f382e230157c2e125787e39`，结束后领取返回 409，生产环境和 S3 的普通分发请求均为 403。重启前后审计相同。扫描数据库归档和控制面日志，未出现签名值、两类 OpenBao 身份令牌、执行租约或包装令牌 |

启用 PostgreSQL 和 Docker 的 Go 回归为 68 项通过、零跳过，`go vet ./...` 通过。并发和风险变更数据库测试另连续运行三次均通过。

执行绑定 Broker 对应提交 `8c83ac81ca46585cac3e17588ec19f1e6fd04a03`。

## 专题 07、10 · 固定提交的独立只读审查

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 用 `worker review-candidate` 审查队列配额代码。工作树已位于修复版，传入有边界错误的历史候选 | 审查读到固定候选，报告具体行号和可核对证据 | Claude Code Session `bfda2a07-502c-4ae6-8012-64ce4798e0e5` 在 `63d21b5ef9fb07acdd9c7e8107c5094e7252638f` 报告 High、blocking，指出 quota.py 第 3 行的 `<=`。Git 对象核对 evidence_verified=true |
| 改传修复候选，再审查此前通过真实 CI 的 alpha PR #9 候选 | 每次产生独立会话，报告绑定各自候选，保留原工作树 | `b1a07b8e4619487ba9842e78859da8157c24c937` 的会话 `1d1d8a72-2923-46fe-aadc-4fa44fbd4192` 无发现。alpha 的 `deb856d2e1e1b0c1e0102869998450c396899a5c` 由 `c6d8dbf4-1407-4c3b-8a3b-f5c27f8467ba` 完成，亦无发现。前后工作树 HEAD 及含未跟踪文件的状态一致 |
| 回归错误引用、删除行、重复 Finding ID 和二进制输入 | 错行或工作区独有内容不能伪造 Git 证据，无法完整处理的输入明确失败 | Git 对象测试通过，原页面 diff 的截断和外部 diff 禁用测试通过 |

只读候选审查对应 ForgeOS 提交 `85cd1cb0b3763e160f24bff722531c99cfce0469`。

<a id="adr-evidence-015"></a>

## 专题 07、10、12 · 独立审查参与真实 GitHub 交付

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 启用项目 2 的审查门禁，按编号导入 alpha #10，并让开发 Worker 完成 | 得到可追溯的候选及本地验证。缺报告或请求审查其他 SHA 时拒绝 | 任务 28、执行 24、候选 `eeee49d87d1d503c97f2397bac6d2063646cbb5a`。报告尚未完成时批准返回 409，旧 SHA 请求审查返回 409。开发身份领取审查、审查身份提交批准都返回 401 |
| 按固定执行请求审查，在独立进程启动 `worker reviewer` | Claude 与开发会话分离，控制面核对固定对象并保存独立回执 | 会话 `39ae331b-2e6e-4512-9b7d-f05d8e2bc3a1` 完成，未报告发现。回执与开发结果的基础和候选 SHA 逐字段一致。Owner 批准成功，控制面重启后回执完全相同 |
| 让交付器创建 PR，等待必需检查，在相同 head 重跑首次失败的检查 | 检查通过才允许合并，Issue 回写并关闭 | PR #11 的 `acceptance-check` 成功，合并 SHA 为 `e20d92779c846102f3e58d1fd33169bbabb59d8b`。任务 DONE，Issue CLOSED。[Issue #10](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/10) · [PR #11](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/11) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/43841822142) |
| 用 PostgreSQL 测试失败、重试、错租约、开发会话复用、报告重放和引用未核实 | 无有效报告或缺核对说明时不能批准，重试使用新租约 | `TestCodeReviewGate` 通过。启用 PostgreSQL 和 Docker 的全套 Go 测试共 70 项通过、零跳过，`go vet ./...` 通过 |

审查门禁及以上交付对应 ForgeOS 提交 `940006841453be623726e3b2d0d58c859c940fa5`。

## 专题 10、11 · 从任务页面核对审查发现

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 390 × 844 的 Chrome 窗口打开已交付任务 28，展开执行 24 的审查报告 | 加载关联会话、候选和摘要，窄屏可读 | 页面显示原生会话 `39ae331b-2e6e-4512-9b7d-f05d8e2bc3a1` 和“未报告发现”，页面无横向溢出 |
| 在独立 PostgreSQL 测试 schema 中从页面请求审查，受控返回失败，再点击重试 | 批准被锁定，退回仍可用。重试使用新租约 | Chrome 中按钮状态符合预期，重试生成与原值不同的租约 |
| 把此前配额样例的真实 Claude 报告提交给控制面，在页面核对并退回候选 | 行证据可核对，空说明无法保存。Owner 可据此退回，历史记录保留 | 页面展示 High、阻塞、`quota.py:3`、原文和引用匹配结果。保存核对说明后批准按钮可用；因确认边界错误，实际选择退回。任务变为 READY，重载后仍能打开原报告并读到说明 |

第二、三项使用预置开发执行和真实 Git 对象，回放已实测的原生报告。浏览器、控制面、租约、证据核对和数据库写入均按实际入口运行；该样例未创建远端 PR。

审查页面对应 ForgeOS 提交 `427bc24871b091cae2eb43204cbc2799aef3609c`。该版本 70 项 Go 测试通过、零跳过，前端生产构建和 `go vet ./...` 通过。

<a id="check-b-6"></a>

## 专题 10、16 · 仓库验证配置合同

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 将仓库示例提交到本地验收项目 8，再改写工作树，从 Owner API 分别请求 V1–V4 | 读取固定 Git 配置，缺检查时指出具体缺口 | 提交 `ac17ea0bf8e757b296d5ebf3ec1e876eeb2c3ba9`、配置 blob `db25f663f8560417c4994d5fa4bd46bb2db04926`。V1、V2、V3 分别列出 3、5、6 项。V4 缺 critical Profile、security 和 regression 必需检查 |
| 新增两种检查和 critical Profile，再用具有和不具有项目授权的节点查看 V4 | 完整配置返回 8 项去重检查；项目授权独立 | 授权节点读取提交 `108844db6b36d65e8b476b22eb5c410076d43ec8`、配置 blob `1ef073028e3c4a765d82c21280f592389b0d76bc`，ready=true。其他节点返回 403 |
| 在另一提交添加 privileged 字段，另以 HEAD 请求计划 | 配置不能为自己加权；只能按固定提交检查 | 提交 `1d07b4ec714e3ce820000c8eb1bdb89e463efa7a` 返回 422。HEAD 请求也返回 422。操作前后的未提交配置一致 |

本项验收的是可执行检查清单的解析与覆盖判定。启用 PostgreSQL 和 Docker 的全套 Go 回归共 81 项通过、零跳过；`go vet ./...` 和 `go mod verify` 通过。

仓库检查配置对应提交 `35866baa5cd8635d4a5906fd21a4016c90bf48bd`。

## 专题 10 · 在隔离容器中执行仓库检查并归档证据

通过非 root Worker 的 `validate-candidate` 命令验收。配置基础为 `c8cb70674f17701f75ab0655405f8b165d64bf8f`，实际 Linux 容器镜像为 `sha256:c05ba4b73604069d376c4f41346b05374335b5ca0c46fb6dfede5a59f5196931`。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 以 V1 检查历史候选 `df7547f581d76629137ff4f21a0cdab490875c18`，另留未提交内容，并在宿主设置两个凭据哨兵 | 检查读取固定候选；容器没有凭据、默认路由或源挂载写权限；可选失败不阻断通过 | `ci-run-1876710922` 的容器 UID 为 65534。必需检查退出 0，可选检查退出 7。命令返回 0。工作树仍保留原未提交内容。报告和产物字节均与固定候选内容一致，按文件复算 SHA-256 匹配 |
| 在候选中改写检查命令并提供错误输入 | 原配置仍生效且归档失败检查的证据 | `6675f97283beb8450e9540424c3987f1f1beb389` 的运行 `ci-run-2228911632` 返回 CODE_FAILURE，配置 blob 与前次一致，报告内容对应失败的候选 |
| V2 中让一个必需检查超时、另一个构建退出 3；另请求缺少 Profile 的 V4 | 区分超时、构建失败、配置缺项；未执行不能生成通过记录 | `ci-run-1095856787` 同时记录 TIMEOUT（退出码 null）和 BUILD_FAILURE（退出码 3）。V4 运行 `ci-run-907231703` 为 CONFIG_FAILURE，检查记录为空。命令完成后按容器名查询无残留 |

PostgreSQL 和 Docker 全套 Go 回归为 82 项通过、零跳过。额外覆盖命令退出 0 但未生成承诺的报告，结果为 TEST_ENV_FAILURE。

离线容器 Runner 对应 ForgeOS 提交 `ee695b39ca1d61ec7d3d8bfd0ebaefe1681cfaf6`。

<a id="adr-evidence-012"></a>

## 专题 10、12 · 从真实 Issue 发起仓库检查并交付

在 beta 私有验收仓库通过 PR #7 引入 Go 包及验证配置，合并后的基础为 `9d0213cfa67f8fded11bda56d7c4a6fc86713981`。随后通过 Owner 接口按编号导入 Issue #8，选 V2。[配置 PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/7) · [需求](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/8)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 让旧 Worker 和支持仓库检查的 Worker 先后领取 | 旧 Worker 无任务；新 Worker 执行固定格式、单元和构建检查 | 旧领取返回 204。任务 29、执行 25 的三项退出码均为 0。首次候选有 953 个变更文件，页面差异截断。核查发现带入编译缓存，因此通过 Owner 入口退回 |
| 修订后重新检查并核对 SHA、文件与报告 | 新候选有新运行记录，实际交付只含需求范围内的实现和测试 | 执行 26、运行 `ci-run-418587960`，候选 `84997d86d4d4a1cfd32a36cf14c090c66e3e2721`。仅有 queue.go 和 queue_test.go 两个变更文件，覆盖等于、超过上限和负数输入。两次执行均绑定配置 blob `24c99f7c289bd37b2fd5a6f3d1a981ee001d3828`；第二次三个检查全通过，归档报告复算 SHA-256 与 API 一致 |
| 在 Chrome 展开执行 26 的“逐项检查”，查看运行编号与代码差异，批准并等待远端交付 | 本地检查有独立证据，GitHub 的必需检查仍约束合并。重启可读原回执 | PR #9 的首次远端检查失败，同一 head 重跑成功。合并 SHA `bc83d9a1399a0ab8eb78f2cb0f0f6464e19a8ebf`，任务 DONE、Issue CLOSED。重启前后完整验证回执相等。[PR #9](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/9) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43847029410) |
| 用测试仓库提交其他 Runner、错执行号、错误配置 blob、遗漏检查和将必需检查改为可选的回执 | 控制面逐一拒绝；候选若修改验证配置，即使通过仍需独立审查 | `TestDeclaredChecksRequiredForIssue` 通过，配置变动的候选处于 WAITING_ACCEPTANCE，直接批准返回 409。Go 回归 83 项通过、零跳过；前端生产构建通过 |

Issue 仓库检查和交付对应 ForgeOS 提交 `80836d649574426c6e6984c66aaff5feab0e2da3`。

<a id="check-a-6"></a>

## 专题 10、12、16 · 本地 CI 的 GitHub Commit Status

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 对已交付的 beta 任务 29、执行 26 并发补写，重启后再次请求；尝试错 SHA 和其他项目 | 同一回执，远端仅一条；不允许错目标 | 两个请求均返回 status ID `59014274755`。重启后不变，远端计数 1。错 SHA、跨项目和仅在本地的候选都返回 409 |
| 在 beta 严格分支保护中保留 acceptance-check 并临时加入 forgeos/local-ci，再交付 Issue #10 | 两项均成功后才允许合并，状态与验收 SHA 一致 | 任务 32、执行 27，候选 `a54213eddb57ea1cc89539581037c1d2bc0e560d`，本地状态 `59014432428` 自动写回，Chrome 执行详情可见。Actions 初次失败，同一 head 重跑通过。PR #11 合并为 `42a7f8dfcaf18e91aec089169335de96e158d69d`，Issue CLOSED。实测结束后恢复原必需检查及 strict=true。[PR #11](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/11) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43848242665) |
| 将一个故意失败的测试提交预置到专用远端分支，以隔离测试 schema 派发该 SHA 的真实 V2 验证。模拟 GitHub 已接收但本地未记录确认 | Worker 得到非零退出码，回传 failure；恢复沿用原 status ID | 提交 `b784f0f8fa9f57d4d2c6312ba88f406496e1037d` 的单元检查失败，任务 FAILED，状态 `59014437445` 为 failure。移除测试 schema 中该回执后，经 API 恢复相同 ID，远端计数保持 1。此分支未创建 PR。[验收提交](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/commit/b784f0f8fa9f57d4d2c6312ba88f406496e1037d) |

含 PostgreSQL 和 Docker 的 84 项 Go 测试全部通过，零跳过。原生测试还覆盖超时映射为 error、未知远端 SHA 和远端写入后丢失响应。前端生产构建与 `go vet ./...` 通过。

GitHub Commit Status 对应 ForgeOS 提交 `749351cf484cb665f275355fc25e67912651fb8c`。

<a id="adr-evidence-013"></a>

## 专题 10、11 · 固定候选的风险判定与 V3、V4 容器实测

项目 9 的基础提交为 `86470cf7df0a3245e9d7ad78a5b3deb5d23f80db`，验收规则 blob 为 `cbe4e01e3d6e197d048afc3c73bb0db8f0c24ce4`。通过控制面查询评估，将返回等级交给 `worker validate-candidate`。本次覆盖独立评估与 Runner 入口。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 将只修改文档的候选按 staging 目标评估，请求最低 V1，再执行选出的等级 | 目标提高到 V3，运行格式、单元、构建与 HTTP 集成 | 候选 `91a226260d7c3c1962720f1531132cf6b7586976`，运行 `ci-run-3762549469`。四项均退出 0，HTTP 检查在独立 Linux 容器内启动服务并实际发送请求 |
| 将 accounting 下的文档移出组件目录，管理建议 V1 | 基础版本中的旧路径命中 ledger 规则，保持 V4，追加安全与关键回归 | 候选 `a4a9f6373d89115f2aacebfda5585e0aa16a097e` 的三条差异路径中命中 `accounting/guide.md`。运行 `ci-run-527488150` 的六项检查全部退出 0，含 go vet 和队列边界回归 |
| 从 Owner 与项目 Worker 入口读取匹配详情，尝试跨项目；在候选中降低验收规则 | 两种身份可读取授权项目的相同规则和人工验收要求；候选不能改写本次选级 | Worker 在其他项目被拒绝。ledger 返回 api-backend、payment、staging 人工验收及容量边界预期。降低规则的候选仍按基础规则评估 |

固定候选风险评估对应 ForgeOS 提交 `fdc4192e440223fefd375fbb64e8ec979583d516`。启用 PostgreSQL 和 Docker 时 86 项 Go 测试通过，零跳过。

## 专题 10、12 · 自动分级贯穿 Issue、容器检查和交付

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 通过 Owner 接口将 beta #12 以 docs、documentation、local 导入，再以仅有旧检查能力的 Worker 请求领取 | 策略冻结。GitHub 工作项最低为 PR 目标；旧节点不能领取 | 任务 33 保存自动分级、Owner 最低 V1 和 PR 目标。旧节点收到 204。新 Worker 领取执行 28，原生开发会话只修改了 docs/queue-inputs.md |
| 在 Chrome 查看选级依据、检查和差异并批准，等待真实 GitHub 回执 | 文档差异为 V1，PR 提高到 V2，三项检查和远端必需检查通过才能交付 | 候选 `0a6de37033a90b6df3b270bf1e8dd3bf55a6f9ee`，运行 `ci-run-3420652059` 的 format、unit、build 均退出 0。状态 ID `59016109988` 标注 V2。远端必需检查首次失败，同 head 重跑成功；合并 SHA `fdef7f0867f55743acffd6c0b564736e3b6ff7a4`。任务 DONE、Issue CLOSED；重启后回执一致。[Issue #12](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/12) · [PR #13](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/13) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43851591297) |
| 用真实 Git 对象构造一个有完整 V1 通过回执、但变更策略文件的候选，伪报自动等级为 V1 | 控制面从固定输入独立复算出 V4 并拒绝低等级回执 | `TestDeclaredChecksRequiredForIssue` 得到冲突；原固定 V1 历史任务仍可提交真实 V1 回执。87 项 Go 回归通过，零跳过；go vet 和前端生产构建通过 |
| 独立 Runner 使用 auto、V1 最低要求与 PR 目标重新检查 ledger 文档移出目录的候选 | 本地 Runner 自行匹配原路径，强制运行 V4 | `ci-run-3800396296` 在候选 `a4a9f6373d89115f2aacebfda5585e0aa16a097e` 运行六项检查，全部通过。run.json 中保存 ledger 组件、基础规则及升级理由 |

Issue 强制风险分级对应 ForgeOS 提交 `48a32f2110a4e1818fe42dbab7f62f89b80ef3bd`。

## 专题 05、06、10 · Management 提高计划的验证下限

先在 beta 私有验收项目提交 HTTP 集成测试与 V4 Profile，经必需 CI 后合并到 `bea34fc766114c19c9dd79d3598a4f27838ce4e1`。随后导入只要求修改发布清单文档的 Issue #15，Owner 指定 docs、documentation；Issue 要求对候选运行关键回归。[环境 PR #14](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/14) · [Issue #15](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/15)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 由独立 Management Worker 领取 Goal 34 的修订 1，在 Chrome 查看审核，随后从 Owner 入口激活计划 | V4 建议有具体依据；D2 待 Owner 激活，建议在审批前不改变执行策略 | 原生 Codex 会话 `01a1e3fc-8946-71c2-aad3-9d057da9b5db` 引用了 Issue 对 HTTP、go vet 与边界回归的要求，返回 D2、approve、V4。激活前为 PLANNING、零子项；激活后子项 35 的冻结策略含管理下限 V4 |
| 在 PostgreSQL 中依次审核 V3、V4、V1 三版计划，第一版激活后先让 Worker 领取一个子项 | 后续提级覆盖未启动子项；在途执行沿用原策略。低建议不能覆盖已生效下限 | `TestManagementValidationMinimumAppliesOnActivation` 通过。等待的子项由 V3 升至 V4，第三版仍为 V4；已领取的子项维持 V3 |
| 用开发 Worker 完成子项，核对清单内的边界示例与测试名称，批准该 SHA 并等待 Goal 关闭 | 文档 V1、PR V2、Management V4；必须实际跑完六项。子项交付后才关闭父 Issue | 执行 29、`ci-run-3288283744`、候选 `bb383121eea44590f4db686735466844fc8ad865`。仅修改 docs/release-checklist.md，format、unit、build、http、vet、boundary 均退出 0。GitHub Status `59017049649` 写回 V4 和六项检查。PR #16 合并为 `f4972a88317187fda5d169d788287c8d7a7ffe4e`，子项与 Goal 均 DONE，父 Issue CLOSED。重启后原计划审核仍可读取。[PR #16](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/16) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43853419062) |

本次 PostgreSQL 与 Docker 全套 Go 回归 88 项通过，零跳过。`go vet ./...` 和前端生产构建通过。

Management 提高计划验证下限对应 ForgeOS 提交 `0420f455c76949ddb9d5240e1632b53f594bcd40`。

## 专题 11 · 全局验收方法库

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从 Owner 和获项目 9 授权的 Worker 入口读取库，与源码数据逐项比较并独立复算摘要 | 两种入口返回文档列出的十二类方法及项目示例引用的 security，所有方法可按 ID 查阅 | 两份响应一致，包含 13 项。版本为 1，SHA-256 为 `c3aee4047dc85cebf29caca6f145c9b548019b0d4810a8087e4372d7f8f06942`。重启后结果一致 |
| 从固定候选的组件匹配结果读取方法 ID，再查全库；用其他项目节点读取项目 9，或用 Owner 查询不存在的项目 | 匹配的 ID 对应具体步骤与取证要求，越权和不存在的项目返回不同错误 | ledger 的 api-backend、payment 均能取到三步操作和两类证据。跨项目请求返回 403，未登记项目返回 404。此次核对方法内容与访问入口，各方法的业务实测由对应项目的验收计划记录 |

方法库 Schema、专题范围和示例引用测试通过，对应 ForgeOS 提交 `86aff28aff1ff9b0b6559f9d65dfc73897ae9334`。

### 容器回归复核

首次并行回归时，容器集成测试的两个候选共用一分钟超时。第一项通过，第二项启动时上下文已超时，未取得 Git 工作树。为每个候选单独设置测试时限后，在真实 Docker 再次验证正常报告和缺失报告两条路径。预期第一项归档匹配的 SHA-256，第二项即使命令退出 0 也应标记 TEST_ENV_FAILURE；两项断言在 8.50 秒内通过。项目声明的单项检查仍使用原超时。

在专用 PostgreSQL 18 和本机 Docker 上运行 `go test -count=1 -p 1 -json ./...`，89 项通过、零跳过。`go vet ./...` 通过。

容器回归时限修复对应 ForgeOS 提交 `b02d69924d6bad2e847e777cd7688618cda60434`。

## 专题 11 · 从固定候选生成测试计划

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从 `worker plan-tests` 提交 beta #15 的文档候选、两个明确标准与冻结的 V4 策略 | 输出可操作的用例，每例能追溯原文，保留待测状态 | 初次生成发现库中缺少文档核对方法。补入 documentation 后，候选 `bb383121eea44590f4db686735466844fc8ad865` 的原生会话 `01a1e412-0029-7933-919b-ae0a7652bca2` 返回 TC-001、TC-002。分别核对两个测试名称与允许/拒绝示例，均为 pending。方法库摘要 `f9af76bc56b37bd6c52fd99841d263ac3bf5041b591c9f1a9d853587dd61722c` |
| 对移出 ledger 模块的候选指定容量标准，再加入“足够快”的模糊性能标准 | 使用基础版本的组件预期，要求 staging 人工操作；缺阈值时提问 | 候选 `a4a9f6373d89115f2aacebfda5585e0aa16a097e` 的原生会话 `01a1e412-320a-7bc0-b173-153e4865a9b5` 生成两项 pending 用例。组件用例绑定 component/ledger/1、api-backend、human、staging。requirement/2 返回指标、阈值、负载、环境及观测时长的澄清问题 |
| 尝试提交虚构的预期、已通过的结果、错误环境、错误方法或遗漏标准 | 拒绝无来源的期望及未执行先报通过；允许无业务预期时只提问 | `TestPlanBindingsAndClarifications` 逐项通过。独立生成入口没有写入人工测试结果 |

独立测试计划生成对应 ForgeOS 提交 `7565fa6af37a2395fb45310bf9dff7e291f3445d`。含 PostgreSQL 与 Docker 的 90 项 Go 回归通过，零跳过。

## 专题 11 · Management 对待测计划逐项审查

通过 `worker review-test-plan` 提交原始请求和完整计划。以下三个审查均使用原生 Codex 独立会话。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 检查 beta #15 两项文档用例，核对步骤和证据 | 两项均有可操作的检查，总结果 covered，用例保持 pending | 会话 `01a1e415-6ed0-7642-889b-0b74ab713b14` 分别引用了对应的 TC 编号。候选 `bb383121eea44590f4db686735466844fc8ad865`，计划摘要 `3c803e466bb47638d205d9b3b722caa7251834aee0a1aab9bddaa1648c3b3e63` |
| 在同一候选把第一个用例改为仅查看 docs 目录，保留原标准、预期和编号 | 目录截图无法证明文中出现指定测试名称，审核指出缺口 | 会话 `01a1e415-6ecf-78a2-a030-ebafa28200dc` 将第一项判为 gap、第二项为 covered，总结果 changes_requested，收到不同计划摘要 `879bc089893f4eda00b975e37bd4ee043e025c18fcb9411598e34f18d4649a4f` |
| 核查 ledger 计划中的模糊性能要求；另以已修改的原始标准提交旧计划 | 未解决的标准保持澄清；来源发生变化时拒绝旧计划 | 会话 `01a1e415-6ed0-75a0-863c-4a9ca31d5a80` 给两个容量标准 covered、性能标准 needs_clarification。错来源请求返回非零且没有审查回执 |

独立覆盖审查对应 ForgeOS 提交 `e71b7c8b142f4dceccf7d3665c54ae7d037edae3`。

## 专题 11、13 · 候选任务内派发计划、覆盖门禁与修订归档

导入 beta #17，开发 Worker 生成任务 36、执行 30、候选 `3f20ee3a6a1cf53c37fe90d9b326cc5b992f49de`。基础为 `f4972a88317187fda5d169d788287c8d7a7ffe4e`，唯一变化是 docs/empty-queue.md。[Issue #17](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/17)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 Chrome 生成测试计划，附加“应该很快”的标准。尝试普通 Worker 领取 Test Agent 作业、使用错误 SHA、重发原请求并尝试直接批准候选 | 身份与候选必须匹配，重发复用修订，覆盖未通过时不能批准 | 错角色 401，错 SHA 与批准请求均为 409，原请求返回同一修订。Test Agent 会话 `01a1e41e-b9b4-7dd3-86fb-032cfd20532c` 和 Management 会话 `01a1e41f-0bb7-7a63-9d69-88a916166c7f` 对修订 1 返回 needs_clarification |
| 通过 Chrome 将补充标准明确为两种容量输入和对应布尔结果，生成修订 2；阅读审查、翻回修订 1，并重启控制面 | 新修订保留原始 Issue，独立生成与审核；两个版本均可复查 | 两个新会话分别为 `01a1e41f-5855-7ea1-b352-1087a77f273f` 和 `01a1e41f-ce12-7741-be2d-3bc561206adc`。第二版 covered，两项用例仍为 pending，计划摘要 `359a406bbb3a8c0565e41a600183684196b069d8556a3ea72e7d2b6fa34bd47b`。重启前后两个完整回执分别相等 |
| 在 PostgreSQL 中模拟并发领取、修订后的迟到结果、其他项目身份、开发会话冒充 Test Agent、伪造覆盖摘要及重复上报 | 只接收当前版本、正确身份和匹配计划的结果；重试不得重复派发 | `TestCandidatePlanRolesAndRevisions` 通过。生成及覆盖审查期间批准均返回冲突。覆盖通过后恢复批准入口 |

工作项内计划与审查对应 ForgeOS 提交 `978d1849db027f2af78a7b4e59491465dd45c267`。该版 PostgreSQL 与 Docker 回归有 92 项通过。以下实测在此基础上新增了逐用例的批准条件。

<a id="check-b-7"></a>

## 专题 11 · 冻结候选的用例执行与交付

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 beta #17 的执行 30、修订 2 通过覆盖审查后直接批准；以错误项目、错误 SHA 或 human 身份上报 runner 用例 | 拒绝提前批准和错误来源 | 提前批准 409、其他项目 403、错误 SHA 与错误用例身份均为 409 |
| 具名项目节点 case-runner-beta 在固定候选检查变更文件、队列实现和文档，运行 `go test -count=1 -v -run 'TestCanEnqueue/(empty_queue\|zero_capacity)' ./...`。先在基础版本读取示例，再在正确候选复测 | 两项用例均有实际操作和可复核的证据。缺少文档时记录阻塞，复测保留前次结果 | TC-001 通过；TC-002 先受阻后通过。Chrome 展开第 2 次实测读到文档 blob `936801e8bb1b8e31fb5e6377842d240bf398438d`。重启控制面、重放同一回报，两项共三条记录 |
| 在 Chrome 批准该候选，等待 GitHub 必需检查并读取 Issue 与 PR | 全部用例通过后才可批准和交付 | 候选 `3f20ee3a6a1cf53c37fe90d9b326cc5b992f49de` 合并为 `218f96c03ea383724ccd5915ce16d602758ff9d6`，Issue 已关闭。[PR #18](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/18) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43859752697) |
| 在单独的 PostgreSQL schema 中登记本地仓库及测试 Issue，运行真实 Worker、Docker、Test Agent 与 Management。项目规则要求 Owner 在 Chrome 检查 docs/owner-check.md。打开差异核对新增的“Queue is empty.”，经人工实测表单逐例填写操作、观察和文件行号 | 无计划时批准返回冲突，覆盖通过后仍须采证，提交两项结果后可以批准 | 候选 `e87782520198c8d974fd9dc149f1a734203ac7e3`，计划摘要 `c7d1b423e08e0f0845b29bfb77a4bbda901af4ddb2f0eb90a50c4ce03d3a989c`。TC-001 和 TC-002 均记录 actor=owner、tester=human。批准返回 ACCEPTED。此次通过 Playwright 操作本机 Chrome 的 Owner 入口 |

该版本启用 PostgreSQL 和 Docker 运行 `go test -count=1 -p 1 -json ./...`，92 项通过、零跳过。`go vet ./...`、前端生产构建通过。

用例实测与交付门禁对应 ForgeOS 提交 `71e6c9adb9d8e3c452c3a7d30baf3cc4d1c73537`。

## 专题 11 · 本机静态站点预览

在独立 PostgreSQL schema 登记本地站点仓库，执行 1 在工作树 execution-1 以真实 Docker 完成 V2 检查。候选为 `273616480856ec0a7d064ded86f13c032bb01f98`。通过 Playwright 操作本机 Chrome。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在执行详情选择 public，启动并打开预览。另用普通 Worker 身份、错误候选、缺少入口的目录分别尝试启动 | 页面和根路径脚本可加载，执行、工作树、环境、访问策略、SHA 在回执中一致；拒绝错误请求 | Chrome 标题区显示 Candidate queue，脚本将计数更新为 0 waiting。该页面的 localStorage 初始为空，响应头与候选一致。错误身份返回 401、错 SHA 返回 409、缺少入口返回 422 |
| 分别修改项目克隆与执行工作树的 index.html，重新打开；尝试访问公开目录外文件、隐藏文件、Git 元数据、控制面路由及无预览域名的回环地址 | 页面保持候选版本，越界请求不可读 | 刷新后仍显示 Candidate queue。五个路径与错误域名均返回 404，Git 对象读取回归另覆盖符号链接 |
| 重启控制面，重复启动，再在任务详情停止预览；新建另一个三秒预览并等待到期 | 重启后原地址有效，停止及到期均能收回访问权限 | 原回执完全相同且页面返回 200。点击停止后旧地址返回 404。新预览由 200 变为 404，历史列表同时保留 expired 与 stopped |

重启后分别重复停止已停止和已到期的实例，两个完整回执均保持一致。启用 PostgreSQL 与 Docker 的 Go 回归有 93 项通过、零跳过，Go vet 与前端生产构建通过。

本机静态预览对应 ForgeOS 提交 `7fe001fa7cc59ba849d09dd1a8542126626fe1d7`。

<a id="check-a-7"></a>

## 专题 11、14 · 经 Caddy 打开及撤销 HTTPS 预览

使用 Caddy 2.11.4、隔离的临时 CA 和客户端证书、PostgreSQL 与本机 Chrome。测试客户端将专用测试域名解析至本机；服务端证书覆盖该区，回环网关端口为 55791。此项验证 HTTPS 网关。外部设备、Tailscale DNS 和 ACL 保持待验收。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 通过 TLS 访问工作台，分别使用无客户端证书、有效证书但无 Owner 令牌、证书和有效令牌 | 设备和应用分别鉴权 | 无证书时 TLS 连接被拒；有证书的客户端可打开登录页，读取任务列表返回 401，加 Owner 令牌查询任务 1 返回 200 |
| 从工作台域名用有效 Worker 令牌向内部领取入口发请求；从预览域名查询任务 1 | 私网入口仅提供各域名的指定功能 | 两个请求均返回 404 |
| 在 Chrome 登录工作台，点击“打开私网预览”。核对候选与脚本运行结果，随后在原工作台停止预览并刷新另一标签页 | 独立域名加载固定候选，停止立即收回访问 | 页面显示 Candidate queue 和 0 waiting，响应头绑定 `273616480856ec0a7d064ded86f13c032bb01f98`，预览来源的 localStorage 为空。停止后链接消失，原页刷新返回 404 |

启用 PostgreSQL 和 Docker 的回归有 94 项通过、零跳过，Go vet 和前端生产构建通过。

Caddy 网关对应 ForgeOS 提交 `ec7a462baf075e210456a364a0d697777773695c`。

## 专题 11、13 · 预览反馈登记与测试用例关联

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在固定候选的公开预览中查看标题和队列计数，再从任务详情进入同一实例的反馈表单，选择测试用例并提交观察 | 工作台保留预览、执行、SHA 和所选计划版本；反馈可复查并等待分类 | 本机 Chrome 显示 Candidate queue、0 waiting。反馈 #7 记下观察和计数无障碍标签建议，绑定任务 1、执行 1、候选 `273616480856ec0a7d064ded86f13c032bb01f98`、计划 v1 的 TC-001。该计划来自原生 Test Agent 会话 `01a1e441-461f-7180-92e8-b8486faa77fb`，覆盖审查已通过 |
| 从四个并发客户端重发同一提交，再使用不同意见、错误 SHA、不存在的用例、含查询参数的页面路径或 Worker 身份请求 | 重发幂等，错误绑定与不适合归档的路径被拒绝，读取需 Owner 身份 | 四次重报返回原编号。不同意见、错误 SHA、错误用例均为 409，错误路径 400，Worker 读取 401。PostgreSQL 回归另验证四个同时首次提交的客户端只产生一个编号与一个任务事件 |
| 重启控制面，从原入口查询历史和候选状态 | 保留原反馈及来源，等待 Management 处理 | 两次完整历史响应一致，author 为 owner、状态为 awaiting_review。工作项保持 WAITING_ACCEPTANCE |

预览反馈登记对应 ForgeOS 提交 `17bb81908ff8f730102abb8e2d2931485f2598b6`，启用 PostgreSQL 和 Docker 后 96 项回归通过，零跳过。

## 专题 11 · 浏览器扩展选择元素并提交反馈

使用独立配置目录的 Chrome for Testing 156.0.8078.4 加载仓库的 Manifest V3 扩展。在浏览器触发已安装扩展的工具栏动作，由 Playwright 操作实际预览、确认表单和登录页。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 连接本机工作台，在候选预览进入选择模式，按 Esc，再次进入后点击队列计数 | 取消时退出选择模式；点击后得到可重新定位的元素与原候选 | 定位器 `html:nth-of-type(1) > body:nth-of-type(1) > output:nth-of-type(1)` 在原页唯一匹配显示 0 waiting 的元素。工作台回执记录 Chrome/156、视口 1280 × 720、执行 1 和 SHA `273616480856ec0a7d064ded86f13c032bb01f98` |
| 在确认页选择 TC-001，填写观察并提交；随后从相同预览选择整页反馈 | 保存明确提交的元素反馈；整页草案允许省略定位器 | 反馈 #15 绑定计划 v1 的 TC-001。再次从工具栏打开的整页反馈表单没有元素定位器 |
| 打开带测试查询和片段的页面，选择带虚构私密属性及值的密码框；另以错 SHA 查看表单 | 草案仅含结构和路径；版本发生变化时不得提交 | 草案中没有查询、片段、表单名、ID 或输入值，仍能辨认 input 标签。错 SHA 页面提示重新选择，提交按钮禁用 |

扩展元素反馈对应 ForgeOS 提交 `c561e4022bce42ae4ff79cc9eba436f7966e4cdf`。

## 专题 11 · Management 分类、取证与重启复查

从 Owner 接口为同一预览提交六条分类样例，绑定既有候选 `273616480856ec0a7d064ded86f13c032bb01f98` 与 TC-001。启动两个该项目的 Management Worker，由原生只读 Codex 分别分类。表中描述是分类输入，每条回执均保存了输入原文中的证据片段。

| 操作（提交的反馈） | 预期类别 | 实际编号、类别和 Management 会话 |
| --- | --- | --- |
| 明确确认标题和计数符合 TC-001 | 验收通过 | #16 acceptance · `01a1e453-e1eb-7952-8423-1e4ab0b90789` |
| 描述已约定的计数行为、复现操作与不符的计数 | Bug | #17 bug · `01a1e454-1c16-7042-8609-7c858507ed1a` |
| 要求只修改计数的间距和字号 | UI 修改 | #18 ui_change · `01a1e454-3133-7b82-8ab5-fcd4e06d81a8` |
| 将已约定的容量计算改为包含暂停项 | 需求变更 | #19 requirement_change · `01a1e454-5a51-7973-b98b-fbb996c2f5a1` |
| 请求在当前范围外独立开发历史导出 | 新工作项 | #20 new_work_item · `01a1e454-7894-7e92-8039-7d6268c42f91` |
| 因预期未定义而无法继续，要求澄清 | 阻塞或澄清 | #21 blocked · `01a1e454-9764-7fa1-bddd-66a62f04c861` |

Chrome 打开 #21 的分类依据，显示同一会话。重启控制面后完整反馈历史一致，任务保持 WAITING_ACCEPTANCE。普通 Worker 上报分类返回 401。真实 PostgreSQL 回归另验证过期租约回收、跨项目拒收、无出处引用拒收、相同回执重报与分类事件去重。六条归类不会直接更改候选、执行命令或代替 Owner 验收。

### 本轮验收对照

| 文档要求 | 可复核证据 | ForgeOS 提交 |
| --- | --- | --- |
| 专题 11、14，私网网关的应用与预览路由 | Caddy 2.11.4 双向 TLS，工作台 200/401，内部路径 404，撤销后预览 404。外部 tailnet 单列待验收 | `ec7a462baf075e210456a364a0d697777773695c` |
| 专题 11、13，固定版本与用例关联的反馈 | #7 绑定执行 1、计划 v1、TC-001；并发重报只写一次事件 | `17bb81908ff8f730102abb8e2d2931485f2598b6` |
| 专题 11、ADR-014，浏览器内元素批注与页面上下文 | Chrome 工具栏操作产生 #15；结构定位器可重新选中原元素，虚构的私密值未进入草案 | `c561e4022bce42ae4ff79cc9eba436f7966e4cdf` |
| 专题 11，Management 将反馈归入六类 | #16–21 分别有独立原生会话、原文引用与理由，重启后可核对 | `99de4f602db0095167dd0312a7fbdad41291d673` |

在分类实现完成后的稳定源码上启用 PostgreSQL 18 和真实 Docker，运行 `go test -count=1 -p 1 -json ./...`。9 个包、97 项测试通过，零跳过。`go vet ./...` 和前端生产构建通过。此前一次跨越源码编辑的回归遇到未定义类型，该次运行未作为验收证据。

## 专题 06、11 · 将阻塞反馈转入验收检查点

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从 Chrome 打开反馈 #21，发起关于暂停队列计数的澄清 | 保存分类引用与候选，等待答复期间禁止批准 | 任务 1 进入 WAITING_INPUT，问题 #1 关联反馈 #21、执行 1 和候选 `273616480856ec0a7d064ded86f13c032bb01f98`。提前批准返回 409 |
| 在任务页确认暂停处理会保留计数，再回到预览下读取答复；三个客户端重放原请求，随后重启控制面 | 恢复同一候选，保留问题和答复各一份 | Chrome 显示原答复，任务恢复 WAITING_ACCEPTANCE，执行仍为 1。重启前后的完整预览反馈记录一致 |
| 改用错误 SHA、错引用、UI 修改反馈或 Worker 身份 | 拒绝错误绑定与身份，不写入问题 | 前三种请求返回 409，Worker 返回 401。PostgreSQL 回归另验证既有普通问题与反馈问题使用相同字符串键时，两者均可独立查询和回答 |

实现提交为 `bf272410bb833322a04a74b777b920ea46a6341c`。

## 专题 06、11 · 从预览反馈退回修订并验收新候选

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 执行 1 的逐用例实测尚未完成，提交验收类反馈 #16 请求批准 | 批准继续受原有门禁约束 | HTTP 409，状态仍为 WAITING_ACCEPTANCE。错误 SHA、重复编号、混入新工作项的退回及仅引用验收反馈的退回也返回 409；Worker 身份返回 401 |
| 在 Chrome 任务页选中 #7 和 #18，填写 Owner 修订说明并退回 | 一次决定同时关联两条反馈，新执行消费修订说明 | 任务 1 进入 READY。两个旧预览记录都能读到该说明。调整编号顺序重放成功，减少一个编号返回 409 |
| 启动真实 Worker 修订计数样式和无障碍名称，打开执行 2 的静态预览 | 生成新的已验证候选并实际显示要求的效果 | 原生会话 `01a1e462-92c6-71f0-9879-0b10559a13d1`，Docker V2 通过。相对旧候选 `273616480856ec0a7d064ded86f13c032bb01f98` 仅变更 `public/index.html`。新候选 `b7fb608448ee82653a0cef95b6014d2a0e2cc844` 保持冻结基线 `0f5ef712f527768825a74590e6d3e17ff2c85568`。Chrome 读到标题 Candidate queue、名称 Waiting queue count、值 0 waiting、20px 字号及 12px 顶边距 |
| 在新候选登记通过反馈 #22，待 Management 分类后在 Chrome 引用并批准，重启控制面 | 旧反馈不能验收新候选。新反馈与批准记录可追溯 | 复用旧编号返回 409。Management 会话 `01a1e463-b845-7b31-abeb-4a53207a63a9` 判为 acceptance。任务进入 ACCEPTED，执行 1 的退回与执行 2 的批准分别保留。此项目使用本机交付策略 |

本阶段在 PostgreSQL 18 和真实 Docker 下运行 `go test -count=1 -p 1 -json ./...`，9 个包、98 项测试通过，无跳过。`go vet ./...` 和前端生产构建通过。

实现提交为 `7cbd8a3539800ca521007780cc5b5dabafa3c731`。

## 专题 11、ADR-014 · Canvas 内位置批注

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 打开 Docker V2 验证后的图表候选，从 Chrome for Testing 工具栏选择画布上的队列标记 | 能把点击位置与真实候选和元素一起带回工作台 | 执行 1、提交 `d5179fb57031f553c6972649ccb683c517128cf8`。画布为 400×200 像素，显示为 200×100 CSS 像素。在显示坐标 (50,75) 点击时，草案显示左 25.00%、上 75.00%，结构路径唯一指向 canvas |
| 从工作台提交并重新打开反馈地址，再重启控制面 | 历史中可区分同一画布上的不同位置，重试只写一次 | PostgreSQL 中反馈 #1 的 `canvas_point` 为 `25.00% 75.00%`，普通历史页可见。重试复用 #1，重启前后记录一致 |
| 提交超过 100% 的坐标，或把该位置关联到 div | 拒绝无效批注，保留已有观察 | 两次均返回 400，反馈总数仍为 1 |

独立扩展探针在首次运行时出现工具栏弹窗调试连接超时。调整探针等待弹窗关闭的时序后，连续两次从空白 Chrome 配置通过 Canvas、普通元素、Esc 取消和隐私字段回归。

Canvas 实现提交为 `5a5d685f0e70b600c7a7ca8f97442f82c19aae89`。包含该实现的阶段回归启用 PostgreSQL 18 和真实 Docker，9 个包、98 项 Go 测试通过，零跳过。`go vet ./...` 和前端生产构建通过。上方专题对照表中的 52 个不同 ForgeOS 提交均已通过本地 Git 对象核对。

## 专题 06、11、12 · 从已分类反馈修订需求

通过私有 beta 项目的正式 GitHub 导入入口派发手册任务。真实 Worker 生成首版页面，Management 在会话 `01a1e472-afef-7290-8fa7-5477682cf535` 将预览反馈 #1 归为需求变更。[验收 Issue #19](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/19)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 Chrome 的反馈页读取需求 1 并编辑完整正文，保存修订 2，再次从 GitHub 同步 | 保留原始需求，当前执行继续绑定第 1 版 | Issue 正文仍为“暂停项计入容量”。任务处于 WAITING_ACCEPTANCE，候选仍为 `562d84d6262d1aeef80831fe8aaf65e882fb0bea`。本地最新修订为 2，带有来源反馈 #1 |
| 4 个客户端重报同一请求，再发送改写的回执、错候选、错执行、错反馈或过期修订号 | 只写一版和一个事件，拒绝错误引用 | 4 次返回同一修订；改变回执或绑定返回 409，空正文返回 400，Worker 身份返回 401 |
| Owner 在任务页采用修订 2，启动 Worker，在 Chrome 分别打开两版候选 | 新执行采用新规则，旧页面仍可核对 | 执行 1 显示 “Paused jobs count toward waiting capacity.”。执行 2 的原生会话为 `01a1e475-8b5b-7e70-bee0-a66c78349eca`，候选 `908869f3ff4eec2dcdc5e0d93d70df06ab165d1f` 通过 Docker V2，显示 “Only active jobs count toward waiting capacity.” |
| 重启控制面，查询项目历史，在任务需求修订中回到原预览 | 能追溯反馈及完整来源 | 搜索 active 返回修订 2 和反馈 #1。Chrome 从需求历史打开原反馈并看到回到修订 2 的入口。PostgreSQL 回归还验证了运行中保存修订不会改变执行输入，后续真正的 GitHub 编辑会独立存档 |

需求修订实现提交为 `d34260b9d8ed4683867f85d8ddb379556b59cb05`。

## 专题 06、11、12 · 从预览反馈派发独立工作项

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 #19 的新候选提出独立的历史报告文档，等待 Management 分类 | 保留原任务，单独处理新增范围 | 反馈 #2 绑定执行 2、候选 `908869f3ff4eec2dcdc5e0d93d70df06ab165d1f`，会话 `01a1e47b-6786-7602-8571-3bc72d167287` 归类为 new_work_item |
| 在 Chrome 核对独立 Issue 编号并派发，打开新工作项及其反馈来源链接，四个客户端重复派发 | 建立一条可双向追溯的关系，只派发一次 | [独立 Issue #20](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/20) 成为 T-3。错误 SHA、来源任务、分类和原 Issue 编号均返回 409。Worker 调用返回 401。重复操作均返回 T-3，来源事件只有一次 |
| 运行 Worker、批准并等待必需 CI | 新工作项按自己的 Issue 正文开发、验证和交付 | 执行 3、会话 `01a1e481-005b-7803-84fa-3bc015ca8019`。候选 `372bdf01873e958227005a3be85e7d9f55e57fea` 的历史文档包含 completed queue events，Docker V2 通过。GitHub 首次必需检查按验收仓库的工作流故意失败，确认失败步骤后在同一 head 重跑成功。[Actions 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43874497425) |
| 核对合并、两个 Issue 的状态和控制面重启后的重复派发 | 只关闭独立目标，已关闭的目标可用原回执查询 | [PR #21](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/21) 合并为 `638dff62d68f33d0ccc5b36e667d9b029aa95b6c`，T-3 为 DONE，#20 CLOSED。[来源 Issue #19](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/19) 仍为 OPEN，T-1 保持原候选和 WAITING_ACCEPTANCE。重启后再次派发返回同一个已完成的任务 |

首次 Chrome 派发暴露了工作台请求帮助函数仅允许任务路径的问题，修复后通过上述真实流程。本阶段启用 PostgreSQL 18 和真实 Docker 的 Go 回归中，9 个包、99 项测试通过，零跳过。`go vet ./...` 和工作台生产构建均通过。

独立工作项分派的实现提交为 `2b805b4674f863582f51dc97b1324dc8cc103fdc`。

<a id="check-a-8"></a>

## 专题 11、16 A.8 · 跨框架与嵌套 iframe 元素批注

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 独立 Worker 验证已提交的 React 19.3.1 和 Vue 3.5.45 页面，从正式静态预览入口打开 | 同源嵌套页面可运行，跨来源页面不能嵌入此候选 | 执行 1、候选 `304948ec31c7e6616230934f8d6de1b97f6da2cf` 通过 Docker V2。Chrome 成功渲染两级框架，另一个来源的嵌入收到 frame-ancestors 拒绝 |
| 在一层 React 框架和两层 Vue 框架分别操作按钮，再从扩展工具栏选择并提交 | 保存有序的框架路径与最内层元素，能重新定位到同一按钮 | 反馈 #1、#2 分别存有一段、两段 iframe 路径。逐层重新定位读到 React queue 1 和 Vue waiting 1，选择操作没有触发额外计数。再从空白 Chrome 配置运行，反馈 #4、#5 同样通过 |
| 查看反馈历史、重启控制面并重放；在独立探针里选择期间插入两级 iframe，在内层按 Esc，再选密码框 | 取消清理所有监听器，反馈中仅含结构，重启后仍能追溯执行 | Chrome 历史页显示框架顺序，重放返回同一编号，重启后 API 响应一致。内层 Esc 可取消。框架查询参数及样例密码没有进入草案；包含属性选择器或缺少目标元素的请求均返回 400 |

首轮完整场景在 Chrome 扩展工具栏的调试连接处超时。复用原候选完成诊断复跑后，再以原验收脚本独立跑通。

iframe 选择对应提交 `5279f11e3f93b6eb557580e570c054d9bcb46fd0`。该版本使用真实 PostgreSQL 和 Docker 运行 9 个包，99 项测试通过，零跳过。

## 专题 11、13 · 工作台截图遮挡、标注与归档

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 Chrome 打开上述候选的实际页面，截取 800 × 450 PNG。在反馈 #4 选图，按坐标遮挡 React 按钮，拖动绘制红框，撤销并重新绘制，点击确认 | 确认前文件留在本机，上传图仅保留遮挡与标注后的像素 | 确认前零次上传。提交体内从 (18,147) 起的 103 × 21 像素均为纯黑，红框存在；归档回读图的全部像素与上传图一致 |
| 由 Owner 在历史页查看截图，比较文件系统、API 与回执；并发重传两份 | 固定执行与反馈可追溯，重放保持同一编号和审计事件 | 执行 1、反馈 #4、附件 #1，2731 字节，SHA-256 `b8d921842bcf9d90c1958b9eae8e13ca0f0637ebfd2e3cce77727c53470ecc42`。历史画布显示不透明遮挡，两次返回同一回执，审核事件共一条 |
| 以 Worker 和错误的任务或反馈访问，上传非 PNG；损坏隔离验收目录内归档，重传并重启控制面 | 身份、归属、格式和完整性校验生效，修复后恢复原图 | Worker 401，错误归属 404，错误类型 415，坏 PNG 400。磁盘损坏后 GET 返回 503，重传恢复原始字节和回执。服务重启后历史与文件逐字节一致 |

本次回归在 PostgreSQL 18 和真实 Docker 上运行，9 个包、101 项测试通过，零跳过。Go 静态检查与工作台生产构建通过。

截图编辑与归档的实现提交为 `c382473c31775ebcc074682f8f55ce59adbad745`。

## 专题 14 · 从数据库备份和文件归档恢复反馈截图

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从源控制面读取执行 1、反馈 #4、附件 #1，停写后用 PostgreSQL 18 导出验收 schema，依据回执逐文件验算并在 Ubuntu 打包 | 私有备份包含可配对的数据库、清单和附件 | 数据库归档 123503 字节，SHA-256 `391361939ae5cf3e8b5a5c863dfe140e0b156d40e627bf6e07afa2fc1369d1bb`。图片归档 1267 字节，SHA-256 `0bf3539f9478855c5fc8645d150ddfa91cfcb40a577754b23e0fa7a27c1ec0b8`。备份整套校验通过 |
| 在新初始化的 PostgreSQL 实例单事务恢复，先用空附件目录启动控制面，再从备份解包 | 缺文件时保留反馈记录并报告不可读；恢复文件后可查原证据 | 目标库恢复前该 schema 有零张表。只恢复库时反馈历史一致，取图 503。恢复库导出的清单与源清单相同。解包后逐文件校验通过 |
| 重启隔离控制面，用 Chrome 打开原反馈和截图，同时验证 API | 归档像素、摘要、归属与原权限一致 | Chrome 显示原编号和摘要，遮挡区仍为不透明纯黑。Owner 读回的 2731 字节与源文件逐字节相等，Worker 读取返回 401 |

反馈截图备份与恢复演练对应提交 `42ba8206c286a65cd31e27f31d9f124f646035eb`。

## 专题 07、14 · 在隔离 Linux 环境恢复工作区并继续派发

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 Linux 容器通过 Owner API 登记仓库并派发 `git_head`，独立非 root Worker 完成任务 1。在工作区分别准备已暂存、未暂存和未跟踪的内容，停写并备份 | 数据库、项目克隆与执行工作区可在同一时间点恢复 | 执行 1 的 HEAD 为 `e90616bd3f91593d3098e056b6a721ddd18031b5`。状态为 `MM queue.txt` 与 `?? pending.txt`。工作区和克隆归档共 10859 字节，SHA-256 `142c389fc017eed4f3763ffd5a583a435b146261d8f293a8bae9ec8fe5b62408` |
| 启动独立 PostgreSQL 16.15 实例与空 Linux 卷；校验、解包到原绝对路径，恢复数据库；运行 Git 完整性检查并逐项对照 | Git 对象、工作树注册、索引差异、工作区差异和未跟踪内容一致 | 两端 HEAD、worktree list、status、两种二进制 diff、文件摘要和 fsck 的完整输出相同，SHA-256 `5ff4cf4a0cd68d92934c51d708336219c5e58e97183f5da28835997f5c1b1f86`。恢复控制面返回的任务 1 与源 JSON 完全相同 |
| 在恢复实例通过 Owner API 派发第二项任务 | 独立 Worker 使用新目录，不改变源实例 | 任务 2 完成于 `/state/work/execution-2`，报告相同 HEAD。源数据库仍只有任务 1，源克隆没有 execution-2 工作树 |

本次源、目标使用隔离 Linux 容器和不同的数据卷，服务进程均以 UID 1000 运行。两端绝对路径一致。本项验证 Git 与数据库恢复，运行中的模型进程和原生 Session 恢复仍待验收。

工作区恢复演练对应提交 `d83eb4be8bda77ddd2699317ac360c8c6e8f06dd`。

## 阶段四、专题 07 · 通过正式 Worker 交付 Claude Code 实现任务

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| Owner 从私有 beta 导入 #22，选择 Claude；只声明 Codex 的节点尝试领取，再启动具备 Claude 能力的 Worker | 控制面冻结所选 Harness，仅匹配节点可领取；不支持的组合有明确反馈 | 未知 Harness、Claude 规划请求均返回 400。Codex 节点返回 204，任务随后由 Claude Code 2.1.310 领取，工作项和执行编号均为 1。[输入 Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/22) |
| Worker 在固定工作树修改队列恢复手册，运行候选验证，再在 Chrome 核对执行详情 | 普通任务入口可读 Harness、版本、会话与验证结果 | 唯一改动为 `docs/queue-recovery.md`，正文说明恢复后待办项保持原顺序。会话 `5aad3ae6-5cd1-4e6d-a3eb-eb5a308f94bf`，候选 `669f0718010c3fb944f4ea2c069a4fa704ac7843`，V2 的三项检查通过 |
| Owner 批准，等待受保护分支检查并交付。重启控制面再次查询任务 | 检查失败时保留候选，全部通过后才可合并并关闭 Issue | 首次 Actions 运行的受控失败步骤阻止交付；重跑同一工作流，第二次通过。PR #23 的 `acceptance-check` 与 `forgeos/local-ci` 均成功，Squash Merge 为 `31b1f3a2745a45f51099c8a965b970cc9d7e020c`。#22 已关闭，任务 DONE，重启查询逐字段一致。[PR #23](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/23) · [Actions 运行](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43881504764) |

本次使用 Windows 本机已有的模型网关设置。受限模式默认忽略用户设置，首次探测返回未登录；显式交给 CLI 服务账号的设置文件后，真实编辑与交付成功。未把模型凭据写入仓库或任务回执。启用 PostgreSQL 18 与真实 Docker 的回归测试覆盖 9 个包，102 项通过，零跳过。Go 静态检查及前端生产构建通过。

按工作项选择 Claude Code 并交付的实现提交为 `b11089be5de8948af1eaef40353aebd5a50bed8c`。

## 阶段四、专题 07 · 项目默认 Harness 与标签派发

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 beta 私有库的专用受保护验收分支提交 `project.yaml` 指定 Claude，本机工作区将同一字段改为 Codex 并留作未提交改动。给 #24 添加派单标签 | Poller 使用基础提交，工作台可追溯配置，只有匹配的 Worker 可领取 | 基础提交 `da7796669227f979503c5cdf915e4ac93909ea88`，配置 blob `1008b690a8b5527149b87f5b21563102636e8fde`。Chrome 任务页显示 Claude Code 与该 blob。仅声明 Codex 的节点领取返回 204。[标签 Issue #24](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/24) |
| 由正式 Worker 实现，Owner 核对唯一改动的手册文件后批准，通过受保护分支交付 | 使用项目选定的 Harness，候选和已合并版本可追溯 | 会话 `14e58ae1-e6d6-4788-91a9-ab1b74358aa9`，候选 `c2c051d3e298c9ff1b2df42fd07b64b680ec413e`。PR #25 的 Actions 受控首轮失败后重跑成功，本地 CI 成功；合并到 `codex/accept-project-harness-9c4836`，Merge SHA 为 `81630b95429f0debfd9593544bba181ba0981708`。Issue 已关闭，重启控制面后任务记录相同。[PR #25](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/25) · [Actions 运行](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43884038472) |
| 在隔离克隆提交未知的默认 Harness 并尝试导入。切回原基础，Owner 明确选择 Codex，再按项目默认重放 | 无效声明阻止派发，Owner 显式选择可覆盖默认，已有快照稳定 | 无效配置返回 409 且任务数为零。显式请求得到 Codex 与原配置 blob，重放后任务逐字段一致 |
| 在独立数据库按规划模式导入，Owner 保存并激活一项计划，用两种节点声明分别尝试领取子项 | 规划和子项分开选择能力；子项保留来源配置 | 父项 1 归档默认子项 Harness 为 Claude。Owner 激活修订 1 后子项 2 为 Claude 并保留原 blob。Codex 领取返回 204，Claude 领取返回 200；此路由演练用受控失败结束领取 |

本次改动完成后启用 PostgreSQL 18 与真实 Docker，Go 全量回归 9 个包、103 项通过，零跳过。Go 静态检查和工作台生产构建均通过。

项目默认 Harness 对应提交 `22c724a04e53c53a48e43cfe5ca3a760c08eb6f5`。

## 阶段四、专题 03、07 · Worker 登记、心跳与节点工作台

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在空 PostgreSQL schema 启动控制面与持有项目专用令牌的正式 Worker，派发 Git HEAD 任务，从 Chrome 打开“执行节点” | 同一节点可关联身份、能力、资源与已完成执行 | 初始列表为空。节点 registry-edge 报告 windows/amd64、16 个逻辑 CPU、1 个名额，启动探测给出 Codex、Claude 和规划能力。执行 1 回报 `65d40b2a22dcec366c5146ed5924c607b09f7cbd`。页面显示在线及已占名额 0 / 1 |
| 等待第二次心跳，停止节点，等待超时，再依次重启控制面和节点 | 实际超时后离线，重连复用记录，查询保留首次登记时间 | 首次登记 22:24:53，第二次心跳 22:25:08。超过 45 秒后显示超时。重启控制面后离线记录逐字段相等，22:25:53 重连时首次登记时间相同 |
| 使用其他节点 ID、零名额和节点令牌分别测试登记与列表。再从真实 Caddy 双向 TLS 入口访问 | 授权、资源校验和网关路由分别生效 | 伪报另一节点 403，零名额 400，Worker 读取列表 401。持有客户端证书经 Caddy 访问列表时 Owner 200、缺少 Owner 令牌 401，访问内部登记路径 404 |

网关演练首次在预览环节打开了历史任务的另一条执行。按回执中的执行编号打开后，Chrome 的节点页、候选预览和撤销均通过。启用 PostgreSQL 18 与真实 Docker 后全量回归 9 个包、103 项通过，零跳过。Go 静态检查和前端生产构建通过。

节点登记与心跳对应提交 `611850ea12f91417f6e2b5e5167a7eb1acc36a7c`。

<a id="adr-evidence-011"></a>

## 阶段四、专题 06、07、14 · 跨项目节点容量与失联占位

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| Windows 启动一个单名额 Worker，派发两个不同项目的 Git HEAD 任务。在专用 PostgreSQL schema 用锁延迟结果确认，并以同一节点身份并发发送八次领取 | 已领取的任务占用唯一名额，Worker 忙碌时继续上报心跳；确认后继续派发 | 任务 1 运行、2 就绪，八个领取请求均返回 204。心跳持续更新，释放结果锁后两个任务均完成，回执匹配各自的 Git SHA |
| 把主节点设为两个名额，向三个项目派发四项，再启动独立令牌的单名额第二节点。在 Chrome 检查节点页后释放结果锁 | 同项目串行，不同项目可分配给有空位的节点，工作区互相独立 | 主节点同时执行 3、5；同项目任务 4 排队。第二节点领取第三个项目的任务 6。页面显示占用 2/2 与 1/1。放行后前六项各执行一次，六个工作区独立，全部回执与所属仓库 HEAD 相符 |
| 以主节点令牌领取任务 7 后停止回报；重启控制面和同 ID 的正式 Worker，等待两分钟租约自然到期。随后 Owner 通过执行接口放弃 7 | 失联执行持续占用名额，Owner 处理后释放 | 到期后 7 仍运行，另一项目任务 8 仍就绪，再次领取返回 204。放弃后 7 失败，正式 Worker 领取并完成 8 |

本次使用三个本地 Git 仓库，未制造远端 PR 或合并副作用。三个 HEAD 分别为 `f324698324ad1248913694bad9cc35e6f72f1d46`、`229f9e3522c1cc1563fbd2795a2a383fea6a2ba1`、`ae39c7d32ef4217982738dddde763674e4d75f7c`。启用 PostgreSQL 18 与真实 Docker 后全量回归 9 个包、104 项通过，零跳过；Go 静态检查通过。

节点容量与失联占位对应提交 `9f6d39672a24da8ee6b989f722b5b779df49f276`。

## 阶段四、专题 06、07 · 按登记能力派发

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 通过 Owner 入口导入私有 beta #26，沿用项目声明的 Claude 和自动验证；以项目专用令牌登记只支持 Codex 的节点，再在领取请求中声明支持 Claude | 节点缺少目标 Harness 时不生成执行，控制面重启后继续使用已登记能力 | 重启前后均返回 204，占用为零。此前尝试导入已关闭的 #24，入口按设计返回 409，随后创建开放的验收 Issue。[Issue #26](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/26) |
| 登记支持 Claude 但缺少声明式检查的节点；随后登记完整能力，分别在领取中关闭风险策略和 Claude | 心跳和本次请求都须满足候选条件 | 三次均返回 204；任务 1 保持 READY，尚无执行历史 |
| 由同一身份上报并请求完整能力，检查工作项和名额，回报受控失败 | 匹配请求只产生一次执行，回报后释放名额 | 领取返回 200，执行 1 指向任务 1，快照指定 Claude。已占名额先为 1，确认结果后为 0。此场景验收到派发边界，随后关闭验收 Issue |

测试夹具补上自动验证策略快照后，启用 PostgreSQL 18 和真实 Docker 的全量回归共 9 个包、105 项通过，零跳过。Go 静态检查通过。

登记能力派发对应提交 `aaaf99292e7cc038c66f7deb786d1f5e2d4385aa`。

<a id="check-b-8"></a>

## 阶段四、专题 12、16 B.8 · GitHub 写入成功后丢失响应

正式控制面、Windows Worker 和 PostgreSQL 18 使用隔离的验收数据。只在验收控制面的进程环境里配置了 GitHub CLI 包装器。包装器调用真正的 `gh`，对指定仓库和 Issue 的四个写入分别丢弃第一次成功返回的响应。每次从任务 API 看到远端操作尚未确认后，停止并重启控制面。验收程序记录每种写入的成功次数，并另行查询 GitHub 和数据库核对最终结果。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从私有 beta #27 建立任务，由项目选用的 Claude 完成手册更新，核对差异和本地 CI，再批准执行 1 | 交付固定于已验证的 SHA | 仅修改 `docs/delivery-receipt.md`，说明如何核对验收和合并提交。基础 `81630b95429f0debfd9593544bba181ba0981708`，候选 `c856cf590ebf8421581d39edc7dd772c1ed98d43`，会话 `58168bb8-3701-46c4-8584-a0c0f5e406a0`。[Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/27) |
| 22:44:57 丢弃创建 PR 的响应，在交付仍为 PENDING 时重启 | 按交付分支找到已有 PR，遵守必需检查 | 复用 PR #28。确认首次 Actions 失败来自受控故障步骤后重跑，第二次成功；同一候选的 `forgeos/local-ci` 成功。[Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43889008466) |
| 22:46:22 丢弃受保护分支 Squash Merge 请求成功响应，在 WAITING 时重启 | 查询已合并 PR，恢复实际 Merge SHA | PR #28 已合并到 `codex/accept-project-harness-9c4836`，远端和本地均记录 `8a4e6a86c118e658bfefc5dcfd3c21f0aedf6091`。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/28) |
| 22:47:06 丢弃发布评论响应，22:47:43 丢弃关闭 Issue 响应，分别在 MERGED 时重启；交付完成后再次重启并比较任务 JSON | 复用交付评论、确认关闭状态，最终任务和事件唯一 | 四种成功远端写入各一次。同分支只有 PR #28，Issue 已关闭且只有一条评论，包含 PR、候选和合并 SHA。任务 DONE，最终重启前后 JSON 相等。`pr_created`、`pr_merged`、`delivery_completed` 事件各一条。[交付回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/27#issuecomment-6919716568) |

时间均为 2026-11-16 Asia/Shanghai。本次复用既有交付协调逻辑，相关实现提交为 `1a1782246225ab810ca34f3a5c1fd73194242ab8`、`544744520aeb6ea13724c7aaf4166ebf0a5c09b0`。实测二进制对应 `aaaf99292e7cc038c66f7deb786d1f5e2d4385aa`。Worker 进程中断后的原生会话恢复另行验收。

GitHub 四类写入的真实恢复验收对应提交 `aee11fb07db8c6fd3addf12da819bd02a790788c`。

## 阶段四、专题 07、13 · Worker 断线回报与本地恢复日志

在独立 PostgreSQL 18 schema 使用项目专用节点令牌，经正式 Windows Worker 领取三个 Git HEAD 任务。测试代理仅改变结果请求的传输：第一种返回 503 且不送达，第二种向真实控制面送达请求后断开响应。每次观察本地 SQLite 已提交待报结果后，强制终止 Worker，停止控制面，再重新启动两个进程。三个任务期望的仓库 HEAD 均为 `4208e82bbd0c537cb4a0c8693bdcbc1e9e01051d`。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 执行 1 的回报未送达；控制面与 Worker 均重启 | 使用原执行补报，控制面从 RUNNING 变为 DONE | Worker 在领取新任务前发送同一正文，收到 204。候选等于仓库 HEAD，本地记录 confirmed，清除待报正文 |
| 执行 2 的回报已生效，丢弃响应后重启 | 幂等确认，没有第二次执行和新增事件 | 补报返回 204。重启前后完整任务 JSON 相等，只有一次尝试和三个任务事件 |
| 执行 3 无法送达，停止 Worker，等待两分钟租约到期。Owner 从执行 API 放弃，再重启 Worker | Owner 决定生效，迟到回报留存冲突证据 | 收到 409；任务保持 FAILED。SQLite 记录 conflict，保存原候选回执并移除租约令牌。三个任务共三个工作区，均只执行一次 |

Ubuntu 非 root 用户另行运行原生 Linux Worker 测试，覆盖 503 重试、重开数据库补报及 409 冲突。首次运行发现测试框架创建的目录权限较宽，改用服务自行创建的私有子目录后通过。持久缓冲覆盖已产生的执行结果；Harness 在结果写入前崩溃的会话恢复待验收。

本次代码在 PostgreSQL 18 和真实 Docker 环境全量回归 9 个包、108 项通过，零跳过。Go 静态检查通过。

Worker 断线回报与恢复日志对应提交 `ab36ad9759dbe700098aea7df33bb33c63db0c85`。

<a id="check-b-5"></a>

## 阶段四、专题 07、16 B.5 · 正在执行的任务独立续约

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 两个项目使用各自的节点令牌。节点二持有执行 1；节点一尝试续约，随后节点二用错误的执行令牌尝试 | 项目授权和当前执行令牌均须匹配 | 跨节点返回 403；正确节点提交错误令牌返回 409。领取回执报告 Git 的租期为 120 秒 |
| 正式 Windows Worker 在项目一执行 Git HEAD。验收程序将 Git worktree 命令延迟 43 秒后调用真实 Git，使任务跨过 30 秒续约周期。在执行途中重启控制面 | 同一次执行延长租约并回报真实候选 | 执行 2 的截止时间由 23:06:46.912344 延长至 23:07:16.922013，随后 DONE。只有一次尝试，回执 HEAD 为 `9c21a277e8b4f1473691801658b57d05644d6e63` |
| 对执行 3 使用同样的延迟，仅在验收 schema 缩短该行的租约，等正式 Worker 发送下一次续约 | 过期租约不能复活；正在等待的操作被取消，并留下原因 | 续约返回 409，Worker 回报 `execution lease renewal rejected (HTTP 409)`。任务 FAILED、尝试一次，未产生 Git 工作区 |
| 让节点二持有的原始租约自然到期，再次续约。Owner 核对并放弃后，再尝试一次 | 到期和人工结束都排除续约；到期本身保留原记录 | 两次均为 409。Owner 放弃前状态 RUNNING，放弃后为 FAILED |

实测时间为 2026-11-16 Asia/Shanghai。第一次演练在缩短测试行时使用了另一种数据库客户端的连接参数，修正验收程序后在全新 schema 重跑全部步骤。数据库回归另行验证续约等待行锁时跨过截止时间，Worker 回归验证连续服务不可用直至本地确认期限耗尽。此处验证运行中执行的租约，原生模型会话重连仍待验收。

启用 PostgreSQL 18 和真实 Docker 的全量 Go 回归共 9 个包、112 项通过，零跳过。Go 静态检查通过。

运行中的执行续约对应提交 `6e865cec3ee03cbfb541145230e1d75f60b5dbd1`。

## 阶段四、专题 07 · Claude 适配器中断后的同会话恢复

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在私有 beta #29 通过正式 Worker 调用 Claude Code 2.1.310 编辑手册。验收适配器转发真实事件，在原生 CLI 完成后丢弃终态并以 1 退出 | Worker 确认已有会话及进程退出，核对租约、原工作区和起始提交，只恢复原 Session | 两个原生进程 PID 为 51084、36232，均确认会话 `bc51f9e0-f24b-481f-8a15-c4f9003ef87b`。SQLite 记录其直接启动的两个适配器 PID 47864、14580，退出码分别为 1、0。[Issue #29](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/29) |
| 对照原编辑与恢复后的文件，检查正式执行回执、本地验证、保护检查和最终回写 | 已有编辑保留；同一工作项、执行和工作区产生一个已验收候选 | 两次文件 SHA-256 均为 `35f73b387974ab65966d11e1ab4199a1aec540c43a2ccd9028104bcd9efb3086`。任务 1 只有执行 1，候选 `b2e8f370cc092e2935d8de3bfb13970cc81265b5`。PR #30 在本地 CI 和 Actions 均成功后合并为 `8e076bfca9fc2bf3ab9231913ae89dc3f9d50b66`。Issue 已关闭，重启控制面后任务 JSON 相同。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/30) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43894562492) · [回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/29#issuecomment-6920510928) |
| 回归夹具进程写入部分内容后自行以 23 退出，分别允许或拒绝续约；另输入错误会话和明确的模型失败事件 | 续约成功时追加剩余编辑，失败时保留原现场；已报告的模型失败不恢复 | 成功分支在同一文件保留两次编辑，两个进程退出码 23、0；409 分支只有一个进程且没有候选。错误会话和终态失败分别被识别 |

真实环境覆盖适配器在终态传输丢失后退出。自动审批拒绝了验收命令中按 PID 强制终止原生 Claude 的操作，返回“blocked by policy”。原生 CLI 被强制终止、节点重启后恢复及 Codex 的同类场景保持待验收。首次查询两张 SQLite 表时列名有歧义；修正验收查询后复用原候选完成交付。真实交付演练期间补充了结构化失败与异常退出的区分，这一分支由最终版本回归覆盖。PostgreSQL 18 与真实 Docker 上全量回归 9 个包、116 项通过，零跳过；Go 静态检查通过。

Claude 中断回合恢复对应提交 `d78ae18dc75305efe6e36386356e5358be96ea16`。

## 阶段四、专题 04、07、13 · 进程记录归档与 Owner 查询

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从私有 beta #31 派发项目默认的 Claude 实现任务，读取运行中的原生可执行文件与 PID，待执行结果产生后比较控制面回执 | 同一次 Execution 关联真实进程、原生会话、起止时间与退出结果 | 直接运行 Claude Code 2.1.310，观测 PID 49780。执行 1 归档的实例号为 1、PID 为 49780、会话为 `1930470a-c11a-4a00-9d55-72f05f83a321`，起止时间分别为 15:22:15、15:22:39 UTC，退出码 0。[Issue #31](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/31) |
| 在验收代理中先把进程会话改成另一值，随后送达完整原始回执并丢弃成功响应，让正式 Worker 重报 | 拒绝不匹配的证据，正确结果和过程记录幂等写入 | 篡改请求 409。两次原始正文摘要相同，均返回 204，最终只有一条尝试和一个进程实例 |
| 停止 Worker，通过 Chrome 以 Owner 身份展开对应执行的进程记录，核对 PID 与原生会话，批准候选并等待交付，重启控制面 | 不依赖在线节点即可读取记录；归档与交付关联稳定 | Chrome 可见 PID 和 Session。候选 `71c36f2e15a7237ce29ac722ee523868b5808e1d` 的受保护 PR #32 经两项必需检查后合并为 `7703deef8476a79fa6e6dd3c5f964de8aed52d14`。Issue 已关闭，完整任务 JSON 在控制面重启前后相等。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/32) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43895866795) · [交付回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/31#issuecomment-6920701144) |

首次浏览器定位同时匹配了外围执行和内部检查的折叠控件。改为选择进程记录自己的标题后，从同一候选继续验收。当前中央工作台显示已回报的 Claude 执行；运行中状态仍须查询节点日志。PostgreSQL 18 与真实 Docker 全量回归 9 个包、117 项通过，零跳过。Go 静态检查及工作台生产构建通过。

Claude 进程记录归档与工作台对应提交 `91a150bc0759fb8449707dbfb1dfa2e126679d9a`。

## 阶段四、专题 04、07、13 · Codex 实现与只读规划的进程回执

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在项目配置默认 Claude 的私有 beta 库中，为 #33 明确选择 Codex 实现。比对运行中的原生可执行文件、SQLite、控制面 API 和 Owner 工作台 | 实际 PID、完整版本、原生 Thread 与退出结果一致 | 四处均指向 PID 17244。版本 `0.169.0-alpha.11.1`，Thread `01a1e4f9-de2d-74e1-9df3-9d9eca1821fd`，退出码 0。错误 Thread 的回报返回 409；正确回报的响应丢失后，相同正文重报得到 204。[Issue #33](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/33) |
| 核对唯一改动的手册、固定候选和本地 CI，批准并等待受保护分支交付 | 一次尝试产生同一候选和已确认合并回执 | 候选 `c4c6e7e5840afb74e41d6a2c2b6dc5febf964e1a`，PR #34 两项检查均成功，合并为 `8124de0fae6c74100b3ad9e25b59f56386ad2fa6`。Issue 已关闭，重启控制面后任务 JSON 相同。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/34) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43897051354) |
| 在独立 schema 中，于同一 Issue 开放期间以规划模式导入。正式 Worker 只读生成计划，在 Chrome 展开进程并重启控制面 | 规划保持基础提交不变，执行绑定规划进程和版本 | 实测 PID 2004，Thread `01a1e4fa-6876-7100-879e-2f5fd64804b3`，版本相同，退出码 0。任务为 PLANNING，修订 1 与执行计划相同，工作区干净、HEAD 仍为 `7703deef8476a79fa6e6dd3c5f964de8aed52d14`。重启后任务 JSON 相同 |

进程记录在执行结果回报后进入中央工作台。独立管理、审查进程及节点重启后的在途会话恢复仍待验收。

启用 PostgreSQL 18 和真实 Docker 后，全量 Go 回归 9 个包、118 项通过，零跳过。Go 静态检查及工作台生产构建通过。

Codex 实现与只读规划的进程回执对应提交 `39d3f6899ab5c2a75bf72e635b349788e2ae1b64`。

## 阶段四、专题 07 · Codex 终态传输丢失后恢复原 Thread

2026-11-16，在 Windows 用正式 Worker、Codex CLI 0.169.0-alpha.11.1 和私有 beta #35 验收。适配器在首个原生 CLI 正常退出后丢弃 `turn.completed` 并以 1 退出；恢复进程转发完整事件。分别核对原生会话中的两条 `turn_context`、原生 PID、适配器退出码和 Owner 任务页。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 选 Codex 实现 #35，丢失首个终态，让 Worker 自行恢复 | 同一 Execution、Thread 和工作区完成授权改动，保留原权限 | 原生 PID 35548 → 17848，Thread `01a1e4ff-b270-7df1-8696-df3e87d4a033`。两次均为 workspace-write、网络关闭、审批 never。适配器 PID 45196 → 868，退出码 1 → 0。只有一次执行，候选只修改指定手册。[Issue #35](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/35) |
| 提交错误进程会话、重复提交正确回执，经 Owner 验收后等待受保护分支交付 | 错误回执被拒，正确回报幂等；两项必需检查通过后才可合并 | 分别为 409、204。候选 `e8a172b7e5f2d8ae9526352131236c8b23faa300` 通过本地 CI；Actions 首次按演练失败，重跑通过，PR #36 合并为 `4e1d4b148c06c4575effd6c4b16bd5f675c90b4e`。Issue 已关闭，只有一条交付评论。控制面重启后任务 JSON 相同。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/36) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43898251585) · [交付评论](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/35#issuecomment-6921039550) |
| 在独立 schema 中将同一开放 Issue 作为只读规划导入，重复丢失首个终态 | 同一只读 Thread 在干净的固定基础提交重新输出计划 | 原生 PID 38656 → 3284，Thread `01a1e4ff-cc08-7562-a8f5-b4fdc03e83f0`。两次均为 read-only、审批 never，工作目录一致。适配器 PID 37724 → 50792，退出码 1 → 0。修订 1 与执行结果相同，工作区干净，HEAD 保持 `8124de0fae6c74100b3ad9e25b59f56386ad2fa6`，控制面重启后可复查 |
| 回归夹具写入半个文件并以 23 退出，再分别返回原 Thread、错误 Thread 和明确失败事件 | 成功时保留已有编辑；会话身份不符或模型明确失败时禁止产生候选 | 原 Thread 补全了同一文件，另外两种情况均返回失败且无候选；明确失败场景没有续约请求 |

原生模型进程被强制终止和 Worker 自身重启后的在途恢复仍待验收。

启用 PostgreSQL 18 和真实 Docker 后，全量 Go 回归 9 个包、122 项通过，零跳过。Go 静态检查通过。

Codex 终态传输丢失后恢复原 Thread 对应提交 `9f83fff323be37a01b9066ec6d73578ac424834b`。

<a id="adr-evidence-003"></a>

## 阶段三、专题 06、07 · 在原 Codex Thread 处理运行中的 Issue 评论

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在私有 beta 导入 #37，指定 Codex。观察正式 Worker 启动原生进程后，通过 GitHub 在 Issue 下补充手册示例，让正式轮询器采集评论 | 普通评论进入下一个安全检查点；原回合正常结束，后续回合沿用同一 Thread | 三个检查点依次收到 0、1、0 条评论。前两个确认同一 Thread `01a1e506-342b-76f2-873d-4a375463810f`，空的末批没有开启第三个进程。该执行的两个原生 PID 为 35272、35092，均正常退出。两条原生回合记录的工作区相同、审批为 never、workspace-write 禁网。[原始评论](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/37#issuecomment-6921147784) |
| 在任务详情展开进程记录，核对候选差异和本地 CI，提交候选验收 | 一次执行覆盖原始需求和评论补充，只产生一个候选 | Chrome 可见同一 Thread 的两条进程记录。候选 `48f935199c017f96492f3fa1ead4d1b860793ac9` 只修改指定手册，包含原需求的验收和合并说明，以及评论要求的完整示例句。错误进程回执为 409，正确回报在丢失响应后重报为 204 |
| 等待本地 CI 和 Actions，核对受保护分支、Issue 评论和重启后的控制面任务 | 合并已验收的候选，保留原讨论和交付回执 | Actions 第一次按演练失败，重跑成功。唯一的 PR #38 合并为 `ca1953f7d1e8ac05e22b8e82612b2a032503dd18`，Issue 已关闭，保留原评论和一条交付回执，重启后任务 JSON 相同。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/38) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43899626076) · [交付回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/37#issuecomment-6921230220) |

验收代理在下一批次请求前等正式轮询器读取评论，以复现实测时序。原生 Codex 直接由 Worker 启动。启用 PostgreSQL 18 和真实 Docker 后，全量 Go 回归 9 个包、123 项通过，零跳过。Go 静态检查通过。

普通评论沿用原生 Thread 对应提交 `97b20b20cb962ecfdc441244dae880858fa3d6b5`。

<a id="adr-evidence-001"></a>

## 阶段四、专题 09 · 两个交付项目的执行与业务凭据隔离

2026-11-16 至 17 日，Windows 上的两个节点分别领取私有 alpha #14、beta #39。共用 PostgreSQL 18 和正式控制面程序，使用真实 Codex CLI 和 Docker 中启用 TLS 的 OpenBao 2.7.2。两边在 ci-test 环境使用同名 receipt-signing、相同用途和 KV 版本 1，各自生成随机测试密钥。验收代理在两次正式领取后，以对应节点身份调用 Broker；解包和签名比对在受信验收进程内完成。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| Owner 为两个运行中的工作项分别批准一次使用。在两个方向试用另一节点身份、另一执行的租约、另一执行编号和 staging 环境 | 均拒绝，不消耗本项目的授权 | alpha 为项目 1、任务 1、执行 2、授权 1；beta 为项目 2、任务 2、执行 1、授权 2。两个方向依次返回 403、409、409、403，随后查询各授权均为零次使用 |
| 用各自身份和租约领取包装响应，解包并对同一随机挑战计算 HMAC，再次解包；执行结束后重试领取 | 各签名只匹配自己的项目。一次性包装和执行结束门禁生效 | 两份签名分别通过本项目校验，交换预期密钥后均不匹配。第二次解包返回 400，执行结束后领取为 409。两份审计各有一次 ISSUED，任务及审计 JSON 在控制面重启前后相同 |
| 核对两个候选的文档差异，分别从对应交付入口批准并等待受保护分支 | 每项只改本库指定文件，合并自己的候选、回写并关闭来源 Issue | alpha 的 docs 检查通过，候选 `510365200cdda471b03ffcd01b4d4f4bb9257790` 合并为 `f5db71cfa976dcc019bf06738e531b98f1edbb63`。[PR #15](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/15) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/43902625675) · [回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/14#issuecomment-6921648615)。beta 的三个声明式检查通过，候选 `072b5953a9477c8126281794893e693f7a213126` 合并为 `af98f5d6022c3ccf5374cf8cffb0ffe938121c10`。[PR #40](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/40) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43902399671) · [回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/39#issuecomment-6921616298) |

首次将 alpha 配为 auto 校验时，其基础提交缺少 validation.yaml，正式 Worker 返回 CONFIG_FAILURE。另一次将 beta 的批准提交到仅对 alpha 开启交付的控制面，按现有配置只记录本地验收。最终演练使用各自项目的验证配置和交付入口。beta 端口首次绑定失败，经重新确认两个服务均可访问后，复用原候选完成交付。最终演练没有重新扫描数据库导出和日志中的明文；该项既有证据见专题 09 的 Broker 验收。

新增数据库回归还让同一节点同时持有两个项目的租约，交换授权和执行时均被拒，读取另一项目的回执返回未找到。启用 PostgreSQL 18 和真实 Docker 的全量 Go 回归 9 个包、124 项通过，零跳过。Go 静态检查通过。

双项目凭据隔离与交付验收对应提交 `81e2b1139c30e61856aa2b81db6fd1e43f265f97`。

<a id="adr-evidence-016"></a>

## 阶段四、专题 12 · 单控制面按项目选择交付分支

2026-11-17，通过一份操作员配置让同一个控制面同时负责 alpha 的 main 和 beta 的 codex/accept-project-harness-9c4836。使用各自项目的节点、真实 Codex、Chrome Owner 工作台和受保护的 GitHub 分支。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从同一控制面导入 alpha #16 和 beta #41，分别核对页面差异和目标分支，点击通过验收 | 两项批准各自冻结所属项目的目标，后台独立等待对应仓库的检查 | 两份批准 HTTP 200，响应中的目标分支分别为 main 和 codex/accept-project-harness-9c4836。两次 Actions 首轮按演练失败，重跑完成后才合并 |
| 从原任务核对 alpha 的必需检查、状态回执和 Issue 关闭 | 只交付 alpha 自己的候选 | 执行 1，候选 `53e4185a05b093d9f8f95b264aab7d4f317bb118`，状态回执 59043755074，合并为 `5592a34169cd7fd7f7c8874b1f65b01a79fe8f8d`。[PR #17](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/17) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/43903355852) · [Issue 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/16#issuecomment-6921749246) |
| 从同一入口核对 beta 的两项必需检查、状态回执和 Issue 关闭，重启控制面 | beta 使用自己的固定分支；两项交付记录可恢复 | 执行 2，候选 `f559c4f5a431fe00978ce740578f049db3a55c6d`，状态回执 59043754713，合并为 `5d40f83c39bb0b51537ab4d03ba4e10061e2f6ba`。重启前后两份完整任务相同。[PR #42](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/42) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43903355714) · [Issue 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/41#issuecomment-6921749145) |
| 重启时仅启用 alpha，重放 beta 的原批准、发布状态和刷新请求；再将单项目环境变量和文件配置指向同一项目的不同分支 | 原验收可核对，已停用项目不能执行新远端操作；冲突配置启动失败 | 原批准重发返回相同任务；beta 的状态发布、交付刷新均返回 403。alpha 重用原状态回执。分支冲突时控制面非零退出并报告冲突 |

两库实际使用同一个 GitHub CLI 账号。每项目 GitHub App 身份和多项目自动轮询仍待验收。启用 PostgreSQL 18 和真实 Docker 后，全量 Go 回归 9 个包、125 项通过，零跳过。Go 静态检查通过。

单控制面多项目交付对应提交 `977910ee99f313832a367dd4ccd3cc17aee2c7ac`。

<a id="adr-evidence-002"></a>

## 阶段四、专题 12、ADR-002 · 单控制面同时轮询两个私有仓库

2026-11-17，先登记 alpha 和 beta，分别在 GitHub 创建带 forgeos-agent 标签的 Issue。alpha 之前没有该标签，在其验收库中创建后复用。以下步骤均从控制面正式轮询入口开始，两个工作项都由后台自动导入。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 起初只配置 alpha 的 docs 轮询，再把 beta 的 auto 配置加入文件并重启 | 首次只导入 alpha；第二次保留原 ID，按 beta 自己的规则新建工作项。讨论按仓库进入收件箱 | 首次只有 alpha #18、任务 1，beta 收件箱为空。启用两项后 alpha 仍为任务 1，使用 Codex 和 docs；beta #43 成为任务 2，使用项目默认 Claude 和 auto。alpha 收件箱只有普通讨论 #2；beta 收件箱为 #19、#2。所有链接均属于对应仓库 |
| 停止控制面，分别给两个 Issue 添加普通评论。重启并等待真实扫描，再启动两个具名节点 | 各工作项收到本库评论，重复扫描仍只有两项工作 | 每项都保存了一条对应的原始评论。工作项 ID 保持 1、2，最终各只有一次执行。[alpha 评论](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/18#issuecomment-6921873049) · [beta 评论](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/43#issuecomment-6921873963) |
| 在 Chrome 以 Owner 核对两份文档差异并批准，待 GitHub 必需检查重跑成功后核对 PR、回写和控制面重启 | 两库分别交付按本项目规则生成的候选，保留讨论和交付评论 | alpha 候选 `eca73bb07027627a0640121d21b2a004662569d3`，合并 `8c8a314b6a72c939f0d9c214c25c016f7376f530`。[PR #19](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/19) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/43904786638) · [回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/18#issuecomment-6921948584)。beta 候选 `4bd43cb76582ac651cad9e09f37a6650e42d4fc8`，合并 `42c39e9698aebb2fc6f05080f2f045ff206fde5c`。[PR #44](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/44) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43904791081) · [回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/43#issuecomment-6921950067)。两项均已关闭 Issue，重启前后任务 JSON 相同 |

启用 PostgreSQL 18 和真实 Docker 的全量 Go 回归 9 个包、126 项通过，零跳过。Go 静态检查通过。两库共用 GitHub CLI 身份；GitHub App 最小授权仍待实测。

单控制面轮询两个项目对应提交 `76499c179711ac2c93fece4c6b4a3aef659ef67f`。

## 阶段四、专题 06、07、14 · 同一 Worker 分别限制模型和候选 CI

2026-11-17，在两个私有项目各派发一个 Issue。节点 budget-worker 的普通工作项容量为 2，模型和 CI 各为 1。alpha 使用 Codex 和 docs 校验，beta 使用 Claude Code 和项目声明的三项 Docker 检查。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 经转发代理领取模型名额，使首次成功响应丢失；观察两个项目的竞争 | 重试保留原名额，每个执行只启动一个原生模型进程 | alpha 执行 2，Codex Thread `01a1e529-90f7-7953-ac98-2cdbfbff42ec`；beta 执行 1，Claude Session `08d010e1-ad8f-44a9-b140-2056ec661d83`。beta 等待模型时收到 31 次 429。每项各一个原生进程，均退出 0 |
| 延迟转发已结束阶段的释放请求，在 Chrome 打开执行节点，重启控制面，再放行；让 beta 尝试领取尚占用的 CI 名额 | 不同资源的名额可同时持有，同类请求排队；重启保留占用，结算后释放全部名额 | 页面与接口均读取到工作项 2/2、模型 1/1、CI 1/1，控制面重启后仍相同。beta 领取 CI 收到一次 429，释放后完成三项 Docker 检查。两项候选等待 Owner 时三类占用均为零。并存观察针对名额，代理人为延长了首项 CI 的持有时间 |
| Owner 在 Chrome 核对并批准两个候选，等待各自 Actions 从失败重跑至通过，再重启并核对回执 | 每库只合并批准 SHA，并关闭原 Issue；交付证据可重新读取 | alpha 候选 `a4f790b45e29c1f7def58d3c28d7b4938c24662e`，合并 `99b295b93bab431978cc80d1dd90fc6c55231559`。[PR #21](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/21) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/43906842688) · [Issue 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/20#issuecomment-6922234379)。beta 候选 `40825fbf01d81125066cc4cba6731c76fb5eb13b`，合并 `b06f526c9b6955597c277cc16733ced24079763b`。[PR #46](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/46) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43906853449) · [Issue 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/45#issuecomment-6922235994) |

PostgreSQL 18 回归覆盖并发申请、旧步骤重放、旧节点升级及租约过期后由 Owner 结束执行。启用 PostgreSQL 和真实 Docker 的 Go 全量回归为 9 个包、127 项通过、零跳过。Go 静态检查和 Web 生产构建通过。独立角色、共用同一模型账号的多节点总额度及持续高负载仍待验收。

普通 Worker 独立模型与 CI 名额对应提交 `e8a84ea087fd7d6d4ff1c40db93c5727d2c50352`。

## 专题 10、13、14 · 恢复 CI 运行回执、日志、报告和产物

2026-11-17，从私有 beta #47 导入正式工作项。Codex 生成候选，Worker 在 Docker 中按项目配置执行三项检查。另用独立 Runner 检查含非空日志、报告、产物及可选失败项的固定候选。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| Owner 在 Chrome 核对候选并批准，等待两个必需检查成功，核对已关闭的 Issue | 已交付候选保留绑定执行和提交的 CI 回执 | 任务 1、执行 1、Runner `archive-runner`、运行 `ci-run-3326424383`，候选 `7ca38462210cca41418fc92e7e0c782b55443ffa`，合并 `4e85a882d1173169966556a22200034f6a3a7f04`。[PR #48](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/48) · [Actions 重跑](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43909046862) · [Issue 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/47#issuecomment-6922587678) |
| 停止演练服务，用 pg_dump 及 tar 备份。用 pg_restore 恢复到同机另一个空数据库，将运行目录还原到新的工作区根；从隔离控制面查询原任务及 Chrome 检查详情 | 恢复库独立给出同一份回执，包括三个空日志的大小和 SHA-256。运行元数据、日志、报告均可核验 | 任务 JSON 与源端相同。备份库摘要 `d172e090895f3a68e2eb87a0ac9d28349a8e10845974ad51b2fb6333b4259ca7`，运行目录压缩包摘要 `310c4e26b87eab208cb09877b298ded7ba0dde4bf3b0fefe72518081b43a80f7`。源端和恢复端均核对了三个日志和两个输出文件；逐个移除均返回非零，复原后全部通过 |
| 用独立入口重新验证固定候选 `df7547f581d76629137ff4f21a0cdab490875c18`。在副本中分别更改非空日志、报告和产物的一个字节，保持大小；另改运行元数据，再提供历史回执 | 内容或身份变化均被发现，旧日志缺少独立摘要时应点明缺项 | 运行 `ci-run-3857521277` 的两个日志、报告和产物均通过原始核验。220 字节的 manifest.log、manifest-1.bin 和 manifest-2.bin 分别被改写时，核验退出非零。Runner 身份变化及未知元数据字段均被拒。复原后重新通过。历史 `ci-run-1876710922` 明确报告缺少日志摘要 |

Docker 回归还覆盖超出 1 MiB 的检查输出，核对截断提示也计入最终文件摘要。启用 PostgreSQL 18 和真实 Docker 后，Go 全量回归 9 个包、127 项通过、零跳过，Go 静态检查和 Web 生产构建均通过。按项目定期清理、跨节点在线回读原始文件仍待验收。

恢复 CI 文件并按独立回执校验对应提交 `ff5d714e71c7e93d035b3f159928822c969511de`。

## 专题 07、13、14 · 从 SQLite 与 PostgreSQL 配套备份恢复 Worker 补报

2026-11-17，Windows 上的项目专用 Worker 经本机代理完成三项 Git HEAD 执行，仓库 HEAD 为 `f0e767426662ab396be6d709cca5fd3365405232`。每次在 SQLite 已持久保存待报结果后执行 VACUUM INTO，停止源 Worker，以 pg_dump 备份对应的 PostgreSQL 18 schema。恢复至三个独立空库和三个新状态目录，保持原控制面地址、Worker ID、项目及工作区路径。校验两份备份的 SHA-256 和 SQLite 完整性后启动恢复服务。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 执行 1 的请求在代理处遭遇 503。源控制面仍为 RUNNING。从配套备份启动两个服务后放行回报 | 新目录中恢复的待报结果先于下一次领取送达 | 同一执行回报 204，任务成为 DONE，SQLite 为 confirmed 且待报正文已清除。快照 SHA-256 为 `916872e0246a00238da8aa3d58c83f40f91efe312ab445e1fe967e981676bcd1` |
| 执行 2 的回报已到达控制面，代理丢弃响应。从备份恢复并再次发送 | 补报获得确认，保持原任务和事件 | 恢复前后完整任务 JSON 相同，204，confirmed。快照 SHA-256 为 `950d394660c7670f4613c42662a072e851f4508ecb822b4c666d3fdd44eea64d` |
| 执行 3 的回报未送达。恢复后等待两分钟租约自然到期，Owner 结束原执行，再启动恢复的 Worker | 已结束的执行保留 Owner 决定，迟到结果可查 | 收到 409，任务 FAILED，SQLite 记录 conflict，保留去掉租约的原结果。快照 SHA-256 为 `786c33de04aceb0d144f93ac6628a388ba33e8428d8327cc4d51195bc62fc19f`。三个场景各只有一次执行、三个任务事件和一个工作区 |
| 用运维文档中的两段 Shell 命令备份并恢复 beta #47 的已完成节点日志；比较两份 SQLite 的执行、进程及会话记录 | 归档文件名与节点身份匹配，恢复后能查询同一候选和原生进程 | Git Bash 和 SQLite CLI 实际执行通过。执行 1、进程实例 1、源 PID 3792、退出码 0，Codex Thread `01a1e534-be3a-7302-be18-12830d27a6fe` 与候选 `7ca38462210cca41418fc92e7e0c782b55443ffa` 均一致。[已交付 PR #48](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/48) |

本项复用正式 Worker 的补报入口，变更为运维手册和验收证据。演练在同一主机切换数据库和状态目录，运行中 Harness 的跨主机接管仍待验收。

Worker 日志配套备份恢复对应提交 `75b1495e7095bd8a2c5cc01eb0e0e99e26f31996`。

## 专题 07、10、13 · 在任务页下载其他节点产出的 CI 文件

2026-11-17，导入私有 beta #49，正式 Windows Worker 运行 Codex 和三项 Docker 检查。使用项目专用节点令牌，在验收代理上模拟上传中断和确认丢失。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 阻断文件上传，查询已确认的任务，再向同一 Worker 派发另一项 Git HEAD 任务 | 元数据和文件分别可追踪；归档积压期间仍可派发 | 任务 1、执行 1、运行 `ci-run-3034990605` 登记五个文件。首次下载返回 503。五次上传均遇到 503，SQLite 保留五项待传，任务 2 已完成 |
| 停止 Worker，恢复上传入口，重启 Worker 并丢弃首次成功 PUT 的响应。在 Chrome 下载三个日志及两个声明的输出 | 队列恢复，响应丢失后再次送达同一文件；下载字节、文件名与回执一致 | build.log 有两次 204，其余各一次；队列清空。五次浏览器下载均与源文件逐字节相同。unit-1.bin 的 SHA-256 为 `9344058c877bf855bace4f71bb95cf35d81889f82e839e6a68c03ffb0f00ce9c` |
| 更换节点令牌、任务号或文件名；上传同长度错误内容和超长内容。另在中央副本中更改一个字节后尝试下载，再补传原件 | 只能上传自己的执行所登记的原件，Owner 可发现并修复损坏 | 另一具名节点上传为 403，Worker 下载为 401，换任务或未登记文件为 404。两种错误内容均为 422。中央副本损坏时下载为 503；原件补传为 204，重新下载通过 |
| 在 Chrome 批准候选并等待受保护分支；重启控制面后重查任务与文件 | 按已验收的 SHA 交付，保留原始文件及关闭回执 | 候选 `34faab0d235cbc0ea274b250826f02e232e246ff`，两项必需检查成功，合并 `ddb3745b8af35b9f3d6952f3759f6a6a7fd2946e`。任务 JSON 及五个文件与重启前一致。[PR #50](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/50) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43912064819) · [Issue 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/49#issuecomment-6923001067) |
| 让 Windows Worker 从五项待传的 SQLite 快照恢复，连接 Linux 容器中的非 root 控制面。容器使用空的项目目录和独立工件存储，未挂载 Worker 工作区。上传后停止 Worker，从 Linux 控制面下载 | 跨系统领取和传输均可工作，节点离线后仍能核验附件 | 首次演练中 Linux 控制面误判 Windows 盘符路径，领取返回 400；修复后五项全部上传。Linux 控制面又派发任务 3，Windows 节点完成执行 3，工作区实际 HEAD 和回执均为 `4e85a882d1173169966556a22200034f6a3a7f04`。停止节点后 Chrome 从 Linux 控制面下载五份文件，摘要全部匹配 |

另从未推送的本地测试分支导入 beta #19，项目配置使 build 写出日志和产物后以 3 退出。真实 Codex 生成候选 `2b14e98d37b2038b035bcd2df86dcacca79cbc21`，真实 Docker 运行 `ci-run-1446598496` 的结果为 BUILD_FAILURE。停止 Worker 后，从 Chrome 的失败执行中下载 build.log（17 字节）和声明为 artifact 的 build-1.bin（12 字节），内容均与预期完全相同。产物 SHA-256 为 `c70ffde6c4269f6436eee0705b70ececf548f8a3603eab2de875c6b08b8e192c`。本场景关闭了远端交付。[输入 Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/19)

Windows 与 Linux 容器共用一台物理主机和 PostgreSQL，项目此前登记在 Windows。异地设备、独立站点网络、文件保留和清理策略仍待验收。

启用 PostgreSQL 18 和真实 Docker 后，全量 Go 回归 9 个包、130 项通过、零跳过。回归包含归档队列写入失败后的原子回滚与重启补报。Go 静态检查和 Web 生产构建通过。

跨节点 CI 文件归档及 Owner 下载对应提交 `6f9b0c141aad99dd9635586e47db8ac363f5dbe3`。

## 专题 07、10、14 · 不同挂载路径上的执行和独立审查

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在空 schema 启动 Linux 容器控制面，从 Owner API 以 `team/beta` 登记只读挂载的 Git 克隆，创建 Git HEAD 任务。Windows 节点使用自己的仓库根和工作区领取 | 控制面登记 Linux 路径；节点从本机对应项目执行 | 登记路径 `/mnt/projects/team/beta`。任务 1、执行 1 在 Windows 完成，Git Worktree、源克隆与控制面回执的 HEAD 均为 `4e85a882d1173169966556a22200034f6a3a7f04` |
| 在独立的控制面、Worker 和 Reviewer 根目录下各建一份 `team/beta` 克隆。正常导入 beta #19，在 Worker 镜像上生成候选。测试转发器在发送结果前用 Git 将固定候选对象同步给控制面 | 领取同时带登记绝对路径和相对路径。模型修改节点上的克隆，控制面能核对固定候选 | 普通领取和独立审查领取均保留控制面的绝对路径并传递 `team/beta`。Codex Thread `01a1e555-48ef-79f2-8f5f-5e9a1abd6a34` 生成 `693b82ff5c7cd0b21a9fe17a1e10bf6a006cd84e`，只更改指定页面。运行 `ci-run-1475177422` 通过本地 CI。[输入 Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/19) |
| 让尚未同步候选的 Reviewer 领取，再将固定对象同步到审查镜像，重试同一执行。在 Chrome 阅读报告和差异，核对标题及段落后批准；重启控制面 | 缺少候选时审查失败，可用对象到位后按原 SHA 审查和批准 | 首次报告候选不是可读取的 commit。同步后独立 Claude Session `8c97a582-98db-4cec-bda6-9c3752ceb21d` 对原候选给出零条发现。Owner 批准为 ACCEPTED，重启后任务和报告均相同。本场景关闭了 GitHub 交付 |

两次演练均使用同一物理主机。独立镜像的 Git 对象同步由演练显式执行。全量回归启用 PostgreSQL 18 和真实 Docker，9 个包、131 项通过、零跳过；Go 静态检查通过。

不同挂载路径上的执行与独立审查对应提交 `ef5699fdfd0d6386ce4a9be0a659011e8520285d`。

## 专题 10、13 · 中央 CI 文件按项目到期清理

对 beta 两次真实 Docker CI 的 PostgreSQL 18 数据做 `pg_dump`，恢复到新数据库，并复制中央文件目录。源任务及文件保留。控制面从迁移 34 启动，按原确认事件的时间登记失败运行的 6 份文件及已交付运行的 5 份文件。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 先在第二个项目设置期限，再用 Owner 和具名 Worker 调用源项目的策略与下载接口 | 只影响所选项目；仅 Owner 能设置策略 | 源项目的三个期限仍为零，11 份原件均可读，文件字节逐一匹配。Worker 尝试修改策略返回 401 |
| 在失败运行 `ci-run-1446598496` 设置日志期限，让日志在几秒后自然到期，构建产物立即到期，报告无限期保留。在 Chrome 打开原任务并下载 | 到期前可读，到期后拒绝下载和补传，报告仍可使用 | 从 Owner API 先读回 17 字节的 build.log；到期后 Chrome 提示文件已按项目策略到期，报告下载与原件逐字节一致。12 字节的 build-1.bin 及三份日志均返回 410。任务仍为 FAILED，候选与原始摘要完整 |
| 在隔离副本中将到期的 build.log 临时替换为含标记的目录，等待正常的分钟扫描，再恢复普通文件并重启控制面 | 存储故障可重试，其他文件照常清理，目录中的内容受到保护 | 本轮清除了其他两份日志及一份构建产物，保留了标记和 build.log 的待清理记录。重启后自动移除了恢复的文件，四条记录都有清理时间，两份报告仍存在 |
| 在已交付运行 `ci-run-3034990605` 使日志到期，再把期限改回无限期。恢复含五个待传项的 SQLite 快照，从真实 Worker 启动；移走其本地一份日志以模拟源文件遗失 | 延长期限不会恢复已到期文件；迟到的补传可收敛 | 三份日志均返回 410。Worker 对缺失文件查询是否到期，向中央补传两份保留的报告，队列由 5 降至 0。磁盘上仍只有两份报告，任务及交付 JSON 与操作前完全相同。[原 PR #50](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/50) |

PostgreSQL 18 与 Docker 全量回归 9 个有测试的包、131 项通过、零测试跳过，`go vet ./...` 通过。新增检查覆盖策略与进行中上传的并发、清理失败重试及重复扫描。本项验收覆盖控制面的中央副本。Worker 本地工作区、反馈截图和录屏的保留策略另行验收。

中央 CI 日志、报告与产物的项目保留策略对应提交 `b46504858df38effd7f7f4a7aa7e0612e04d9747`。

## 专题 11、13 · 到期反馈截图与验收决定

从 PostgreSQL 快照及文件备份恢复静态预览候选 `304948ec31c7e6616230934f8d6de1b97f6da2cf` 的原反馈 #4，在独立数据库和工件目录中启动正式控制面。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 Chrome 打开归档截图 #1 并读取遮挡区像素，再由 Owner 在隔离项目退回候选并记录复测理由 | 图片可在期限内核验，验收决定绑定固定候选 | 归档图为 2731 字节，像素 RGBA 为 0、0、0、255；读回原件摘要为 `b8d921842bcf9d90c1958b9eae8e13ca0f0637ebfd2e3cce77727c53470ecc42`。退回后任务为 READY，保存一条验收决定 |
| 给另一个项目设置 1 秒期限，再给源项目设置 6 秒，同时上传新的 94 字节 PNG。尝试读取和重传已经到期的旧截图 | 项目隔离；每张截图从各自的首次归档时刻起算 | 第二项目配置未影响原图。源项目配置后旧图的读取、同图重传均为 410。新图 #12 先能读回并匹配摘要，7 秒后返回 410。Worker 修改策略为 401，负期限为 400 |
| 把源项目改回无限期，刷新 Chrome 反馈历史，再故意让旧图的清理遇到非普通文件。等待分钟扫描并重启复测 | 已到期图片不复活；故障可重试，其他图片仍清理 | Chrome 对两图均显示“已到期”、原编号和摘要。首次扫描移除新图，保留旧图路径下的标记；恢复普通文件后，控制面重启自动移除旧图，两份回执都有清理时间 |
| 重启后从 Owner 接口查询任务、历史反馈，再次提交原图 | 已归档的决定、候选和反馈可追溯，重传不能新增附件或事件 | 退回决定、理由、候选 SHA 和反馈内容与清理前相同，历史任务仅新增新图首次归档的一条事件（9 → 10）。原图重传仍为 410，两条附件回执均保留 |

PostgreSQL 18 与真实 Docker 全量回归 131 项通过，零测试跳过；Go 静态检查、TypeScript 检查和 Web 生产构建通过。此项只覆盖中央归档 PNG。录屏、节点本地文件和外部设备预览仍待验收。

反馈截图保留策略与工作台到期状态对应提交 `b5f399204936c0378eecb8e5f058eaaa97d03ee1`。

## 专题 13 · 从已清理文件追查策略修订

分别恢复真实的失败 CI 和已遮挡截图，再用旧版和新版控制面顺序访问同一份恢复数据。下面的修订编号分别来自两套独立的数据库 schema。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在版本 36 配置非默认策略，分别使 build.log 和截图 #1 到期，保留一份构建产物和新截图。用版本 37 接管 | 能读出迁移时的配置，审计启用前的删除保留未知来源 | 两个项目各有一条编号 1 的迁移记录，原配置与旧版接口一致，操作者为 migration，前一配置为空。已到期文件返回 410 且无修订号；保留的原件仍可下载 |
| 在恢复库中临时使项目策略写入失败；再并发四次提交同一有效变更 | 策略与审计同步提交，失败与重试不制造修订 | 故障返回 500，旧配置、文件和历史均不变。解除故障后，四个请求只新增一条由 Owner 发起的修订 3，前后配置和操作范围均正确。Worker 查询历史或修改策略为 401 |
| 将两项期限分别恢复为无限期，重启控制面，重查到期文件和历史 | 下载回执继续指向有效的到期依据 | build-1.bin 和截图 #10 均已物理清理，410 回执与数据库各自指向修订 3；修改后的修订 4 和迁移记录 1 保持独立。较早已到期的文件仍无修订编号 |
| 在独立项目提交 102 次不同配置，读第一页后再提交一项，再按游标读下一页 | 并发新增记录不改变已经取得的游标，同时保持项目隔离 | 两页分别返回 100 和 2 条记录，依次还原原来的 102 个期限，没有遗漏或重复；源项目仍只有三条历史。错误游标为 400，不存在的项目为 404 |

PostgreSQL 18 与 Docker 回归 131 项通过、零测试跳过，`go vet ./...` 通过。本项审计记录保留策略的成功变更，其他操作的审计覆盖见专题总表。

保留策略修订和已清理文件的关联对应提交 `079dec80a252a462355ad442305c541665be196a`。

<a id="check-c-4"></a>

## 专题 05、16 · 跨项目批量回答验收问题

恢复已验证的预览候选 `304948ec31c7e6616230934f8d6de1b97f6da2cf`，作为项目 1 的任务 1。另在项目 2 从 GitHub 导入私有 beta #19，正式 Worker 用 Codex Thread `01a1e578-9f8c-7531-9ba2-14d78c6025a4` 完成任务 2，候选为 `343179caa60ab2cc43e635ee7067832c4d1f6ed2`。两项均从 Owner API 在各自固定候选上提出问题，通过 Chrome 工作台回答。本次关闭远端交付。[第二个项目的输入 Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/19)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从侧栏进入待答问题，核对两个任务的依据、SHA、任务详情链接，再用 Worker 查询队列 | Owner 能看到跨项目的待答事项，Worker 无权读队列 | 两题分别指向任务 1、2 及各自候选，依据完整；Worker 收到 401，非法游标为 400 |
| 填写问题 #2、#1，一次提交；在 Chrome 测试转发器中，等 #2 写入成功后丢弃 HTTP 200 响应，再重试未确认的草稿 | 同批其他答复仍完成；重试不会重复写事件 | 页面先显示 1 条成功、1 条未确认，保留 #2 原文。重试后队列清空，两项各有一个 Owner 答复、一个答复事件 |
| 各提出新问题后，在队列填写两项。另一 Owner 会话先回答 #4，随后提交整批 | 保护已记录的决定，仍允许同批其他问题恢复 | #4 显示另一会话的实际答复，并保留本页草稿供比较；#3 按本页答复成功。两个任务各新增一个事件 |
| 第三轮只填写 #6。检查 #5 仍待答，再在宽 390 像素的 Chrome 页面提交 #5，重启控制面 | 空白项继续等待，已答项回到原候选验收，重启保留每次决策 | #5 在 API 中仍为 WAITING_INPUT。全部回答后，两个任务均为 WAITING_ACCEPTANCE，各有三条答复和三个答复事件；原执行号、SHA 及重启前后任务 JSON 均一致。窄屏无横向溢出 |

另用 22 条分页测试数据走实际工作台，提交首页的 20 条后显示“本页问题已处理”，点击“加载更多问题”显示余下两条。PostgreSQL 游标回归覆盖分页间新增问题及其他会话答题。启用 PostgreSQL 18 和 Docker 的 132 项 Go 测试全部通过、零测试跳过，Go 静态检查、TypeScript 检查和 Web 生产构建通过。

跨项目批量回答验收问题对应提交 `abb5257b9899049c22868821721030556f70a5ab`。

## 专题 12、13 · 真实 GitHub Webhook 接入

在私有 beta 仓库使用 GitHub 官方 CLI 转发器建立临时开发 Webhook，将 GitHub 实际发送的原始请求头和正文转发给回环控制面。仓库数字 ID 为 1459615168。工作项留在 READY，以便核对 Webhook 的派发结果。[Issue #51](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/51)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 创建普通 Issue，等签名投递，再从收件箱检查 | 提前同步且只进入收件箱 | opened 投递 `ed3fabd8-e1e7-11f1-82d3-fbb36c2bbed0` 返回 202，7.01 秒内可读，未创建工作项 |
| 添加 forgeos-agent 标签，修改正文，再新增一条评论 | 只产生一个任务；正文更新和评论可分别追溯 | 6.56 秒内出现任务 1。labeled、edited 和 issue_comment.created 各返回 202，需求升为第 2 版，原评论从任务评论接口可读 |
| 用 Owner 身份读回四份原文，与转发器收到的请求比较，并用仓库密钥独立验签 | 投递编号、长度、摘要和原始字节均一致 | 四份原文依次为 9009、9539、9362、10925 字节。评论投递 `f99347f0-e1e7-11f1-811e-bca69b1fa4d6` 的 SHA-256 为 `5b46470c4b9d3b8e65f95258df5996c25a8b421e2d1068a31ce84af095433f4a`。Worker 读取返回 401 |
| 在原正文后加空格沿用原签名，再重投完整原件；重启控制面并再次重投 | 篡改被拒，真实重复投递复用原回执 | 篡改返回 401，重投均为 200。重启后的四条投递、原文字节和完整任务 JSON 均与重启前一致 |

结束转发后已核实 GitHub 自动移除临时开发 Webhook，验收 Issue 已关闭。启用 PostgreSQL 18 和 Docker 的 134 项 Go 测试通过、零测试跳过，`go vet ./...` 通过。回归覆盖错配仓库、同号异文、原文回读及限流期间收到通知。本次验证了 GitHub 签名到本机正式入口的全程，生产公网代理部署和关闭 Issue 后的任务处置另行验收。

Webhook 接入实现及真实投递证据对应 ForgeOS 提交 `4ea0363db2e93e80be7722e4190e07eacda6c012`。

## 专题 06、10、12 · 来源 Issue 关闭时取消执行或归档 Goal

在私有 beta 仓库创建两个真实 Issue。第一项使用正常 Worker、Codex 和 V2 容器验证。在本地验收克隆的固定基础提交 `65a55e135a89f301e42e8612ecfb7564f60f52a1` 延长构建步骤，以便在运行中关闭 Issue。第二项从 Owner API 建立带依赖的 Goal。两个场景均通过定时轮询读取 GitHub 的实际状态。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 观察到 Docker 正在执行 build 后创建一个同项目任务，再在 GitHub 将来源 Issue 以 not_planned 关闭 | 取消请求先入库，在原执行交回名额前保留队列顺序 | 02:21:00 关闭，02:21:03 观测到请求。执行 1 仍为 RUNNING，后续任务 2 为 READY。02:21:30 原任务转为 CANCELLED，随后任务 2 完成。时间均为 UTC+8。[Issue #52](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/52) |
| 核对运行回执、容器、中央日志，并重启控制面从 Chrome 打开任务详情 | 候选和已执行检查可追溯，容器已回收，取消记录可查 | 候选 `51488d9b97d1ecb94332ea85ed800cd56dde4b47`；运行 `ci-run-312700951` 与 build 均为 CANCELLED，build 退出码为空。中央日志与本地同为 20 字节，SHA-256 为 `aed2fe905c544caa28ec2c21fd69f5f719fb0bb571f3036f23f675346bdf3d5b`。原容器已移除，任务无交付记录，重启后页面与状态筛选都能查到关闭记录 |
| 激活含 READY、BLOCKED 两个子项的 Goal，关闭父 Issue，再重新打开并导入一次 | 原 Goal 和子项停止派发，保留计划，不重复创建任务 | Goal 1、子项 2 和 3 全部为 CANCELLED，子项无执行且关联同一关闭回执，原计划逐字段相同。重新激活返回 409，重开后导入仍指向原 Goal，取消列表仍为三项。验收后已再次关闭 Issue。[Issue #53](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/53) |

PostgreSQL 18 和真实 Docker 的全量回归共 9 个有测试的包、137 项通过、零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。回归还覆盖远端对象错配、过时观察、运行中重复关闭、迟到的成功回执及已批准子项的历史保留。

来源 Issue 关闭的实现及验收对应 ForgeOS 提交 `a5d21934d8ecd6c288d20a169b8170e16d822ab3`。

## 阶段一、专题 01、16 · 工作台登记项目与首次 Worker 检查

在新的 PostgreSQL schema 中启动正式控制面。用 Chrome 从工作台登记一个新建的本地 Git 仓库，通过正式 Worker 检查从空仓库到首个提交的状态。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 输入允许范围外的路径，再用相对和绝对路径登记同一仓库 | 拒绝错误路径，登记结果可重复查询 | 浏览器显示路径错误，随后两次均获得项目 1。Worker 身份读取项目列表、创建首次检查均返回 401 |
| 点击运行 HEAD 检查，在控制面完成请求后丢弃浏览器收到的响应，再次点击 | 同一项检查可继续查看 | 两次指向任务 1；Worker 在尚无提交的仓库执行失败，留下非零退出码和 `git worktree add failed` |
| 提交 README 后从桌面和 390 像素宽的独立浏览器会话重试同一项失败检查 | 仅派发一次新检查，留存此前失败 | 两个页面均打开任务 2；Worker `project-setup` 在 execution-2 返回退出码 0，HEAD 为 `0bc57a3402ecc47044c70d2768c7f04b1642582b`，与直接读取仓库一致。再次重报仍为两个任务，窄屏无横向溢出。重启后任务 2 完整 JSON 相同 |
| 另在 31 个项目中从浏览器翻页，在两页之间新登记项目 32 | 已登记项目可完整浏览，旧脚本的 HEAD 检查能复用 | 首页 30 项，下一页只增 1 项。项目 1 显示旧 API 创建的检查 1。刷新后看见新项目 32，非法游标返回 400 |

PostgreSQL 18 和 Docker 全量回归 138 项通过，零测试跳过。Go 静态检查、TypeScript 检查与生产构建均通过。

工作台项目接入与验收对应 ForgeOS 提交 `d57f60da4b1dff1892b1c5f50559add4acd00e38`。

## 专题 01、08 · 固定提交上的仓库入口与依赖清单

在私有 beta 的本地克隆中准备 Go 模块、嵌套 Node.js 包、一个无效包清单和合成的隐私测试标记。通过真实 GitHub 轮询绑定仓库 ID 1459615168，由 Worker `inventory-accept` 完成 HEAD 检查后再推进本地分支。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| HEAD 检查尚未完成时请求扫描，再用 Worker 令牌调用 Owner 入口 | 拒绝无执行依据的扫描及未授权请求 | 分别为 409 和 401；首次归档前读回为 404 |
| 分支已经前进时并发发起两次扫描，逐文件核对 Git 对象 | 使用原检查提交，两次返回同一份归档 | 原提交 `7b38ce432cf62bfa1b4007fd34d20bd996b7706b`，新 HEAD 为 `0e4abdfb92320abec3e8d90ed3f117979fc2c7a5`。归档 6 个文件，所有 blob 均与原提交一致，两份响应字节相同 |
| 展开嵌套包，同时核对无效清单、Go 模块和测试标记 | 正常来源可使用，局部解析失败明确可见 | app/package.json 的 blob 为 `7ec36398254882b7d845d83bed745205891b50a4`，保留 start、test 和三项依赖名。Go 清单包含 example.org/queue-fixture，无效包标记 invalid_manifest。归档不含 .env、脚本正文与依赖地址中的测试标记，脚本未执行 |
| 重启控制面并将仓库根切换为空目录，在 390 像素宽的 Chrome 中读取 | 从数据库恢复原清单，能够打开同一份包详情 | GET 与重复 POST 返回原归档，页面可展开 npm run start，窄屏无横向溢出 |
| 再从项目页选“检查当前 HEAD”，由同一 Worker 创建第二份清单 | 新检查读取更新后的脚本，原任务继续显示原清单 | 独立复验中任务 1 绑定 `b0b7cf3d08963a135aff8cf6c4d08ee7e1bf5013`，任务 2 绑定 `f6ed6f19006a25d2917556e3b0e757443f31e0b2`。第二个页面只显示新脚本 npm run different，第一份归档逐字段相同 |

PostgreSQL 18 和 Docker 全量回归 139 项通过，零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

仓库入口和依赖清单对应 ForgeOS 提交 `e73088cbe63e0da79133d342e477d018c640021c`。

## 专题 01、09、ADR-001 · 按环境归档项目声明的密钥需求

在独立 PostgreSQL schema 中用正式控制面、Worker 和 Chrome 生成三次仓库清单。验收仓库使用合成名称。预先通过 Owner API 登记 local-mock/queue-api 的实际元数据为 S2。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在首个提交声明 local-mock 与 ci-test 下两项同名需求，再声明 production 的独立需求。执行 HEAD 检查，在 Chrome 展开项目配置 | 显示各环境声明，记录其固定来源 | 原提交 `6467fb6366933bb412dcb0e6f9ee467033bc3b5d`，根配置 blob 为 `372091daa991c34239188453d4a91e2326fbd880`。Chrome 展示两条 queue-api/S1 和一条 production/queue-admin/S3 |
| 比较扫描前后的元数据、授权和使用记录 | 声明不得修改实际风险、登记新凭据或授予使用权限 | PostgreSQL 唯一既有元数据的风险仍为 S2，更新时间相同。ci-test/queue-api 的 Owner 状态查询返回 404。执行授权和使用记录均为零 |
| 在需求中加入不受支持的 value 字段，再用第三次提交移除声明 | 错误可定位，回执不含字段值。空清单与配置错误能区分 | 第二次提交 `5783bb288bcb2514b1957994d3941a7222a7498d` 在浏览器提示配置无效；归档保留 blob 和 invalid_project，无测试字段值。第三次提交 `ce8ac6a156c3db4f10a9456cfba11f74044a691c` 返回空数组 |
| 重启控制面，并切换到此前生成的旧版清单 | 已归档的旧证据与首次扫描一致 | 前两版回执重启后逐字段相同。旧版项目清单仍未包含当时没有解析的字段 |

PostgreSQL 18 和 Docker 全量回归 140 项通过，零测试跳过。Go 静态检查、TypeScript 检查与生产构建通过。

分环境密钥需求对应 ForgeOS 提交 `6e51e433ac87e43f8b432d968908e5eca7bc1b22`。

## 专题 01 · 固定提交的本地环境清单与启动验证

验收使用正式控制面、Worker、Chrome、Docker 以及独立 PostgreSQL 16.15 容器。两次运行均通过。下表记录第二次运行，控制面使用 PostgreSQL 18 的独立 schema `startup_04196f07`。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 登记本地测试仓库、派发 HEAD 检查，从 Chrome 打开项目配置清单 | 两种环境均能追溯至同一次检查的 Git 提交 | 浏览器展示 Local Mock 与 Integrated 的脚本、端口及探测路径，绑定提交 `592602d4bce5cface58ebfd0aee9726debb97baf` |
| 在仓库 HEAD 更新后，对上述提交运行 Worker 的 verify-environment，选择 local-mock | 运行原提交脚本，禁用外部网络，就绪后清理容器 | `environment-run-436666246` 使用 none 网络，第一次探测返回 HTTP 204 |
| 为 local-integrated 配置带项目标签的内部 Docker 网络和数据库，再运行同一入口 | 应用完成真实数据库查询后才就绪 | `environment-run-3398551146` 两次探测后返回 204，PostgreSQL 日志记录了读取预置行的 SELECT |
| 选择返回重定向的新版本，再将 Integrated 指向错误网络 | 重定向无法通过就绪检查，错误网络在启动前被拒绝 | `environment-run-3494789491` 记录 302 并超时；`environment-run-1326673787` 记录网络不匹配，探测次数为零 |
| 停止数据库后重跑 Integrated，再检查一个启动脚本立即退出的版本 | 分别记录依赖故障和服务退出 | `environment-run-787047279` 记录 503 并超时；`environment-run-3387302772` 记录就绪前退出 |
| 比较每次命令的标准输出与本地 run.json，再查询 Docker | 失败也保留回执，所有已创建的启动容器均移除 | 六份 JSON 逐字段相同，已创建的探测容器均清理，验收数据库容器及临时网络已移除 |

PostgreSQL 18 和 Docker 全量回归 142 项通过，零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。工作台派发及汇总启动检查另行验收。

本地启动清单与 Worker 命令行验证对应 ForgeOS 提交 `b5e07294ca091f1b79000f818c808cce6ffc2cf4`。

## 专题 01、08 · 工作台派发本地启动检查

在 PostgreSQL schema `queue_01858544` 启动控制面、独立 Worker、桌面 Chrome 和 Docker 中的应用及 PostgreSQL 16.15 依赖。使用工作台派发全部四次检查。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 完成来源 HEAD 检查，读取清单后更改测试仓库 HEAD，再从清单派发 Mock 和 Integrated | 两项任务均运行清单固定的提交；Integrated 实际访问数据库 | 来源 T-1 固定 `f12eec09686793d8b7740830f4984b522deae8b3`，新 HEAD 为 `e6028b8c7c2096dae6135ba46e0575d5044bdea6`。T-2 Mock、T-3 Integrated 分别在第 1、2 次探测返回 204；数据库日志出现对预置行的 SELECT |
| 停止数据库，从 Chrome 重跑 Integrated，然后恢复同名数据库与预置行，再次重跑 | 故障保留独立回执，修复后可重新验证 | T-4 在 17 次探测后记录 HTTP 503，状态 FAILED。T-5 第一次探测取得 204，状态 DONE。两者固定同一提交和启动脚本 |
| 每次派发后重发相同请求，随后核对工作台、节点本地回执和重启后的控制面 | 请求重放复用任务，回执可回读 | 列表只含 T-2 至 T-5；节点 startup-node 的四份 run.json 与 API 逐字段一致。Chrome 可从各执行详情返回来源 HEAD，四次均显示容器已清理。重启控制面后 T-5 回执逐字段一致 |
| 用无凭据请求读列表、用 Worker 凭据尝试派发，再验证不匹配的节点、资源和回执 | Owner 管理入口限制访问，调度和回执绑定来源 | 两项真实 HTTP 请求均返回 401。数据库回归覆盖并发派发、跨项目授权、旧节点、错配集成项目、模型配额和不完整或篡改的成功回执 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 143 项通过，零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

工作台派发与启动回执归档对应 ForgeOS 提交 `42dee55abb05f6627a2acd0086a747cb5b4f25fe`。

## 专题 01 · PHP Composer 依赖和脚本清单

在独立 PostgreSQL schema `composer_7a54b0ad` 运行正式控制面和 Worker，从 Chrome 打开两次 HEAD 检查的归档。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 对同时包含 PHP 和 Node.js 的仓库执行 HEAD 检查，在 Chrome 展开两个包清单 | 各自显示正确的依赖和脚本入口 | T-1 对应 `90a66dc71c930fa8c7c1ec28ec904dcf1d8ab2e5`。PHP 条目包含 ext-pdo、monolog/monolog、php、phpunit/phpunit，脚本为 composer run-script start 和 test。Node.js 条目显示 npm run test |
| 同时提供损坏的清单、vendor 子目录、建议安装的包以及含合成标记的脚本和仓库 URL | 错误定位到文件；归档只含声明名称；扫描不执行脚本 | 损坏的条目在 Chrome 显示“清单格式无效”，vendor 下的清单与建议包未进入结果。标准回执中没有合成标记和脚本正文，测试脚本没有创建文件 |
| 更新 PHP 清单，从项目页再次检查 HEAD，重启控制面后回读两次结果 | 新旧清单独立归档 | T-2 对应 `e6007d00ce03e8babf5fe020c6a48d513692c808`，只显示 php 和 composer run-script verify。两次配置 blob 分别为 `bd018c27b8770c89506a7283c71910ba912050e8`、`eda3693ea975adcda0fd43013e96674df04a67a3`，重启后回执逐字段相同 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 144 项通过，零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

PHP Composer 清单对应 ForgeOS 提交 `a0b90a45b69f054b8537ccc11ef950b838cff1f2`。

## 专题 01 · 原生 Agent 归纳固定版本的入口文档

在 `analysis_5a06be3e` 使用正式控制面、独立 HEAD Worker、项目专用 Management Agent、已登录的原生 Codex 和 Chrome。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从 Chrome 读取 T-1 清单，更改 README 并提交，再从旧任务派发归纳 | Agent 使用 T-1 的文档和包清单 | 来源为 `e0276e7490e6c6d4ba0917fb5493a2fb6d13f82f`，新 HEAD 为 `1eeb42cb37929ef830b62423187961b251212b5f`。原生会话 `01a1e5d4-b0ae-7c51-9c54-f2c2e528f0b7` 返回五类事实，其内容没有新版命令 |
| 展开启动命令并核对依据 | 每项能定位到入口原文或清单摘要 | Chrome 展开 go run ./cmd/rain，显示 README.md 的引用及 blob `6a25245a5bb677f5d9214d0bcd1198c7e864e10b`。测试命令为 go test ./...，依赖 github.com/lib/pq 来自 go.mod 摘要 |
| 重复派发、用 Worker 凭据访问 Owner 接口、重启后回读 | 同一检查复用归纳结果且只向授权身份提供 | 未生成第二次归纳，Worker 访问返回 401；重启控制面后 GET 和 POST 的回执均与原结果逐字段相同。测试 .env 及其合成标记均未进入回执 |
| 在真实 PostgreSQL 中使旧租约过期，提交错误来源或凭空写出的命令，再提交正确结果 | 拒绝旧租约、跨项目写入和不匹配的证据；允许正确结果重放 | 数据库回归通过，旧轮次的重复重试未重新排队；正确结果重复提交保持同一份归档 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 145 项通过，零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。补充拒绝 PostgreSQL 无法归档的空字符后，相关数据库回归和 Go 静态检查再次通过。

固定来源的 Agent 归纳对应 ForgeOS 提交 `3541ac70b38e92bf10b93b1051732f255a767ce3`。

<a id="check-c-8"></a>

## 专题 01 · 版本化初始化报告

使用 Chrome、正式控制面及 Worker、原生 Management Agent、Docker 和真实 PostgreSQL 测试依赖重复验收两次。下表记录 schema `initreport_4253245f` 中固定提交 `76b1f05deca8fbb8cf29e857459b5f7c42e28513` 的三版报告。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 T-1 读取清单、归档报告，随即重发归档请求 | 初版记录当时可得的证据，重复操作复用版本 | 第 1 版包含项目路径、GitHub origin、分环境声明和 station-test-key/S1 密钥需求。Agent 为 NOT_REQUESTED，启动记录为零。两次请求返回同一版本，Worker 身份读取报告返回 401 |
| 归纳入口文档并运行 Mock 和 Integrated，停止数据库后重跑 Integrated，再归档 | 报告将固定来源的项目画像与真实失败回执保存在一起 | 第 2 版收录原生会话 `01a1e5dd-30d9-7f00-b8b7-c65b253b20b1`。T-2 和 T-3 返回 204，测试数据库记录实际 SELECT；T-4 返回 503，状态 FAILED |
| 恢复数据库并重跑，从工作台归档、切换旧版、下载新版 JSON | 新版记录恢复，旧版保留故障及当时的归纳状态 | 第 3 版收录四次检查，T-5 返回 204。第 1 版仍显示未派发。下载文件与第 3 版 API 逐字段相同，文件 SHA-256 为 `3ac6ee53bfb2f8bb9d9a173d83df6483d1e1a5fb8b946cfb2fc5423c61adbf52` |
| 将控制面仓库根切到空目录并重启，逐版回读、重复归档；另从数据库并发申请归档 | 无原始克隆也能读取，证据没有变化时不增加版本 | 三版报告均与首次结果相同，重复归档返回第 3 版。数据库回归中四个并发请求均得到第 1 版；新画像资料产生第 2 版，旧版内容保持相同 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 146 项通过，零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

初始化报告与版本回读对应 ForgeOS 提交 `9a75d9e04a2cdd2be90b833da5f8bd6c38d1e698`。

## 专题 01 · Python 依赖、动态字段与命令入口

使用真实控制面、HEAD Worker 和 Chrome 分别检查同一仓库的两个提交。在 schema `pydeps_9b6d6680` 核对工作台、报告 JSON 和项目专用 Management 领取的数据。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 读取带运行依赖、可选依赖、构建依赖、依赖组、具名 wheel URL、脚本及动态字段的 pyproject.toml | 包名规范化并去重；命令按 Python 入口显示 | T-1 固定 `7460e99bcbfa1b6c22e6b85228c6d412314734a3`。清单列出 http-x、pytest、ruff、setuptools、wheel-pkg，在 Chrome 显示 station-start 和动态字段 gui-scripts |
| 同时读取 requirements.txt、损坏的 TOML 及 Node.js 清单 | 解析具名要求，明确其余来源；错误只影响对应文件 | 读出 requests，远程包含与本地可编辑安装计入 2 项未解析来源；Chrome 显示损坏清单的格式错误，Node.js 条目仍显示 npm run test |
| 归档报告并以项目 Management 身份领取归纳输入 | 报告保留清单，Agent 可用依赖和入口名作引用 | 报告的清单逐字段一致。Agent 输入含包名与 station-start；其来源 blob 为 `8933ad4fe3a41824bfec3f0628c59396cf0aff80`。合成的私有 URL 标记和 Python 导入目标均未出现在输入中 |
| 更新清单，从项目页重新检查，再重启控制面 | 两次回执保留对应版本 | T-2 固定 `d1466a28a293aaf410d0dee951ab65b768f8ed31`，仅有 ruff 与 station-check，清单 blob 为 `3da111b613112a1be544786e5e194a0b9abd4297`。两版重启后回读均与首次相同 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 147 项通过，零测试跳过。Go 静态检查、模块校验、TypeScript 检查和 Web 生产构建通过。

Python 包清单对应 ForgeOS 提交 `c5608ae0ab3068485c40649a115a5bfc8c4c267d`。

## 专题 01 · Rust Cargo 工作区与固定版本的继承来源

在独立 PostgreSQL schema `cargo_d35e64dc` 中使用正式 Worker、控制面和 Chrome。将同一个验收克隆只读挂载到禁网容器，使用 Cargo 1.85.1 的 `cargo metadata --format-version=1 --offline --no-deps` 独立核对成员的直接依赖。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 对虚拟工作区、子 crate、重命名的共享依赖、开发依赖和条件构建依赖执行 HEAD 检查，再从 Chrome 展开成员清单 | 工作台与原生 Cargo 的直接包名集合相同，根清单另外显示共享声明 | T-1 固定 `27a292d88035429c718b6a1890c253e0290e0040`。两者均为 fixture-kit、rain-first、win-helper。unused-shared 仅出现在根清单的共享声明中 |
| 读取被工作区排除的子目录以及包含坏 TOML 的独立文件 | 无法继承的包保留未解析状态，其他条目可继续读取 | 原生 Cargo 在排除目录退出 101，提示找不到工作区根。ForgeOS 的该条目显示一项未解析、无继承包名；坏 TOML 标记 invalid_manifest |
| 只修改根清单的别名目标，再从项目页检查 HEAD | 成员文件未变，第二次检查显示新包名与新的根清单 blob | T-2 固定 `2ef9e6750295af87200aa1e66d56bc536c268d80`。成员 blob 均为 `f5998b86185ef975d4ffe7f9197dd78707f82422`，根 blob 从 `d31f256fc45d1aeb47925604db3c1af2cdfd15b0` 变为 `b6cc5c41ef9d256a9f51cad160c0ca411e68112b`，两端均读到 rain-next |
| 在两次任务各归档初始化报告、领取一次 Agent 输入并重启控制面 | 报告保留同一清单，Agent 收到声明名，历史版本可回读 | 两份报告中的清单逐字段相同于对应任务；成员与根清单的文本摘要不含声明中的私有路径标记。控制面重启后两版清单逐字段一致 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 148 项通过，零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

Cargo 工作区清单对应 ForgeOS 提交 `9e9e55269894dcb65e904c1906e07d539b739eee`。

## 专题 01 · Maven 父 POM 与条件依赖

使用 PostgreSQL 18 schema `maven_60738cc1`、正式 Worker、控制面和 Chrome。将同一提交只读挂载到禁网容器，用 Maven 3.9.16 Model Builder 分别构造默认模型和启用 qa 的模型。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在项目页执行 HEAD 检查，打开 app/pom.xml，展开 qa | 普通、受管和条件依赖的坐标与原生模型一致 | 提交 `2f5d5f1195a586a994fc2ccf5ddaf507da67b856`。两端均读到 org.first:core、org.fixture:helper、org.parent:base。受管的 org.managed:only 和 qa 的 org.qa:integration 分别展示 |
| 仅修改父 POM 的属性，再次检查并归档 | 子 POM blob 不变，新报告使用新的父属性 | 提交 `36063349f0a4fecd07e53972d6234b134fd0e31f`。子 blob 均为 `f1b0ce4f2e44914539cdaf2aa12a4f4bccd97a89`，父 blob 从 `25d55cb1c9990f73b4ca38b1dfe217d6156a2297` 变为 `e6b4e2b22e222d8caf12c9864e204ae0d6601406`。两端均读到 org.next:core |
| 同时扫描坏 XML、仅远端可取的父 POM 和环境变量 | 各文件独立记录读取结果，工作台可以继续归档 | 坏 XML 标为 invalid_manifest；远端父和环境属性共两项未解析 |
| 归档两版报告、领取 Management 输入，重启控制面回读 | 每版包含当时的依赖和来源，Agent 输入保留声明种类 | 两份报告与当次清单逐字段一致。Management 输入含直接及受管坐标，未包含验收用的私有标记。重启后两版清单均与重启前相同 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 149 项通过，零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

Maven 清单对应 ForgeOS 提交 `9e88bdd57971956e382d9141057b4ed9861e6fe7`。

## 专题 06、10 · Owner 从工作台取消运行中的 CI

真实验收使用私有 beta 的 Issue #54、正式 Codex Worker、Docker、Chrome 和 PostgreSQL 18 schema `ownercancel_e93befcb`。以下时间为 UTC+8。[验收 Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/54)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 等待 Codex 自然结束并生成候选，观察真实 Docker build 后，排入同项目 HEAD 任务，在任务页选择“取消任务 · 执行 #1” | 请求绑定当前执行；结算前后续任务仍排队。重复提交无新增请求事件 | 候选 `b2bf1acda60f9adfa8eef62c527a75cfb75e26e9`，原生会话 `01a1e5fc-84fe-7103-85ce-00f13a891722`。04:12:38 浏览器收到 204，T-1 仍为 RUNNING，T-3 为 READY；再次 POST 返回 204，共一条 owner_cancel_requested |
| 等待续约、读取 CI 回执，并从控制面下载 build.log 与 Worker 原件比较 | CI 取消保留已有日志，回收容器，后续任务可派发 | 04:13:03 T-1 转入 CANCELLED。运行 `ci-run-2538423736` 和 build 均标为 CANCELLED；下载的日志 SHA-256 为 `9f6a670ca638ae057abe96b4e405841c1e074ec2330e02c312c115e4a70478a5`，逐字节相同。原容器已回收，T-3 完成 |
| 重启控制面，在 Chrome 回读执行与事件并查询远端 Issue | 已取消任务保持终态，重复请求安全；来源 Issue 仍开放 | 浏览器显示 Owner 取消结算事件，取消按钮已移除。原执行保留请求时间，无交付记录，GitHub 回读 OPEN。核对后另行关闭验收 Issue |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 150 项通过，零测试跳过。已覆盖并发重报、成功回执与取消交错、过期租约结算和非 Owner 请求。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

Owner 主动取消运行中任务对应 ForgeOS 提交 `84758c5b8d561b37ddefc2a103c75e2b2b4e9710`。

## 专题 06、10 · 暂停 CI 后恢复同一候选的验证

真实验收使用私有 beta、PostgreSQL 18 schema `cipause_5e3a49a2`、正式 Worker、Chrome 和 Docker。时间均为 UTC+8。[验收 Issue #55](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/55)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 等原生 Codex 结束，确认 Docker 正在运行 build，排入 T-2，再从任务页暂停 T-1 的 CI | 先固定候选并登记请求，随后停止检查、释放名额 | 执行 1，候选 `aae51b5ff79b882a489d52e9869bd11aa3e47c11`。04:21:40 点击暂停，重复 POST 返回 204。T-1 当时仍 RUNNING。04:22:09 收到原 V2 运行 `ci-run-2186646805` 的 CANCELLED 回执，进入 PAUSED，T-2 随后完成 |
| 下载原检查的 build.log，检查原容器，重启控制面后筛选 PAUSED | 暂停任务、日志及固定 SHA 可以复核 | 日志与节点原件逐字节相同，SHA-256 为 `62fde2cd4ebcf34475c2f2458b1a5ff5566f6f9567368c56ee91da7cbeeee5b2`。原容器已回收。筛选结果包含 T-1，浏览器可见暂停事件及恢复按钮 |
| 从 Chrome 恢复，重复同一次恢复请求，再启动 Worker | 只排入一次验证，复用候选 SHA、重新执行全部检查并等待 Owner 验收 | 04:22:12 进入 READY。执行 3 在新 Worktree 验证原 SHA，V2 运行 `ci-run-3733003939` 通过，04:23:46 进入 WAITING_ACCEPTANCE。新执行没有模型 Session。执行 1 的原生会话 `01a1e604-db7d-7882-9d40-5cccf796c57d` 与取消回执均留存。GitHub 仍为 OPEN，核对后单独关闭测试 Issue |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 151 项通过，零测试跳过。任务列表覆盖 PAUSED 状态，数据库用例覆盖错租约、换 SHA、提前恢复、旧请求重放和暂停期间关闭来源 Issue。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

暂停及恢复同一候选的 CI 对应 ForgeOS 提交 `a221a0d94f9484c83d0fd347f737b8972b54df94`。

## 专题 08 · Owner 在工作台管理项目资料

使用两个 Git 项目、PostgreSQL 18 schema `materialui_434da048`、正式控制面和独立 Chrome 会话。浏览器通过“项目接入 → 项目资料”操作现有的版本化资料服务。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在项目 1 导入中文标识“部署-说明”，记录来源及含标记的正文，同时在项目 2 使用同一标识 | 保存 UTF-8 原文和 SHA-256，项目独立 | 项目 1 修订 1 摘要 `c48e9095bf0d203c39b99de8cd30f9e9b41424f3500d84253f3cff6b19164a3b`，标记以文本显示；项目 2 初始列表为空，导入后的摘要为 `71cfed9fe624e5891d5a5d39564aa0030aea6c05b19afc4da2adba039ad0b84b` |
| 两个浏览器会话从修订 1 独立编辑，先保存第一份，再保存第二份 | 碰到并发修订时保留草稿，Owner 可以核对当前版本 | 第一位保存修订 2，第二位收到 409。页面同时显示修订 2 和第二位的原草稿；明确选择修订 2 为基础后保存修订 3，其摘要为 `5aca50fc003e4436eb872d8690e4acf5c6b634778c9cbd33f71042bb44fb97e3` |
| 从修订 3 回读 2、1，再切回最新版。检索关键词及环境类型，清空筛选后继续翻页 | 每版均可查到原文与来源，列表按项目和条件分页 | 三份原文、来源及按字节重算的摘要一致。组合检索 1 条，全部浏览由 30 条增至 32 条 |
| 重启控制面再从项目页检索和打开，另在新空库运行仓库内的浏览器探针 | 已保存的修订可回读，冲突场景可复现 | 两项目记录及三版摘要与重启前一致。探针资料 `probe-12f7f10441aa4fe29ccce2b660b3ab5b` 独立复验三版与并发冲突 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 151 项通过，零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

项目资料工作台对应 ForgeOS 提交 `e80e961bd779abc241c28b2f9d75a83ca37e18ce`。

## 专题 08 · 从工作台管理项目决策

PostgreSQL 18 schema `decisionprobe_f48f0d3e`，两个 Git 项目、正式控制面和独立 Chrome 会话。原决定为 #1，工作台验收的提案前缀为 `probe-1833d8e23972413cb32fa20b24f5e48e`。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 填写提案，转发真实 POST 后丢弃响应，通过原表单重试并确认 | 重试只生成一项；保留适用条件、来源、引用及确认人 | 原提案只有 #1，按原键返回 200；确认后显示 owner，重启前后其完整记录一致 |
| 两个浏览器会话分别提出替代 #1 的提案，并先后确认 | 一项生效，过时的替代请求不能再推翻前一决定 | 第一项确认后 #1 为 SUPERSEDED；第二项确认返回 409，仍可拒绝。第一项随后从工作台标记 EXPIRED；原决定列出三次操作及两项证据 |
| 浏览全部 34 项，观察有明确截止时间的测试提案，查看另一项目 | 列表继续翻页；截止后提案不再可确认；项目之间互不混入 | 先读取 30 项，再加载至 34 项。页面到期后自动移除待确认提案，切换 EXPIRED 可见该项。第二项目的全部状态为空，重启后相同 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 151 项通过，零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

项目决策工作台对应 ForgeOS 提交 `4713fdc9d8139e3e9f92609f44ee4651876b8e16`。

<a id="adr-evidence-005"></a>

## 专题 08、09 · 在工作台发布和撤销项目间共享副本

使用 PostgreSQL 18 schema `shareui_0e5bbecf`、两个独立项目、项目 2 的专用节点身份、真实 Chrome、正式控制面和原生 Codex Management 会话。原始资料与私有标识均为验收数据。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从项目 1 打开 handoff 的修订 2，切换至修订 1，在浏览器填写共享经验并提交给项目 2 | 副本固定到所选修订，待审时接收方无记录 | 副本 #1 为修订 1；Owner 提前发布返回 409，项目 2 的浏览器显示空列表 |
| 启动源项目 Management 进程，在工作台刷新并展开审核依据 | 独立只读审核给出依据，会话可查；审核自身不发布 | 原生会话 `01a1e61b-8a48-7110-9d75-db776d7d1fed` 给出 approve，生效等级 D3。此时接收方仍为空；提交错误摘要返回 409 |
| 在 Owner 页面发布，通过接收项目的页面及其专用节点查询 | 两个入口均只收到脱敏副本；源项目记录保持私有 | 摘要 `454147c7c97a3bfc99815de59d03cf922a94a66dc23db646d5093f07b779f201`。节点响应仅含 id、title、content、digest 和 published_at；读源资料返回 403，读私有发送列表返回 401 |
| 重启、回读、从源项目的发送列表撤销，再刷新接收页面 | 发布记录在重启后保留；撤销后副本不再返回 | 接收响应重启前后逐字段一致；撤销后接收页面为空。重放原摘要发布返回 409 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 151 项通过，零测试跳过。发送列表另有分页边界、项目隔离及 Owner 鉴权回归。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

跨项目共享工作台对应 ForgeOS 提交 `f7a9bbe390ca5e39d69367e7cf97685178679345`。

## 专题 07、09、14 · 活动租约中的具名 Worker 令牌轮换

私有验收仓库 [beta Issue #56](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/56)，PostgreSQL 18 schema `nodekey_73884dff`。正式轮询器导入 Issue，具名 Worker 领取执行 #1。原生 Codex 会话为 `01a1e622-3279-7030-bb26-e9a65329d898`，候选提交为 `033116c5167da73068505fc0f65337c60719b2fa`。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 Docker 构建运行期间配置与第二个节点相同的摘要并重载 | 拒绝整份配置，原权限继续工作 | Owner 收到 409；原令牌访问本项目返回 200，第二节点访问同一项目返回 403 |
| 配置独立新摘要，分别用节点和 Owner 重载 | 仅 Owner 能启用新配置。重叠期间新旧令牌拥有同样的项目权限 | 节点重载 401，Owner 重载 204。新旧令牌读取本项目均为 200 |
| 原子替换节点令牌文件，删除授权文件中的旧摘要，再次重载 | 撤销立即约束后续请求；原进程用新凭据续原租约 | 旧令牌 401，新令牌 200，新令牌读取另一项目 403。执行 #1 在 RUNNING 中的租约截止时间延长，Worker PID 25268，全程仅一个执行尝试 |
| 等待 CI 结束，通过 Owner 入口下载构建日志，再重启控制面 | 原执行完成上传，重启后保留撤销和候选状态 | CI `ci-run-2247421916` 成功，下载内容逐字节匹配本地文件，SHA-256 为 `8940cfbb78a4a7a0e66261bef38092b05f1f758b9fd983a6ea8dacfa1fb9c542`。重启后旧令牌 401、新令牌 200，候选仍为 WAITING_ACCEPTANCE |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 153 项通过，零测试跳过。Go 静态检查通过。

活动租约中轮换节点令牌对应 ForgeOS 提交 `266f11fe1c6598a047b7dac217fa7793a51d8794`。

<a id="check-b-10"></a>

## 专题 07、09、14 · 具名 Worker 客户端证书轮换

Windows 本机启用两个独立端口、PostgreSQL 18 schema `nodetls_275f1ec9`、专用私有 CA、两个节点授权记录与正式 Worker 进程。通过 Owner 的任务入口创建三次 Git HEAD 检查。下表的握手和 HTTP 请求均走实际 TCP 连接。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 分别省略客户端证书、使用未受信 CA 的证书、在客户端仅信任错误服务端 CA | 在 TLS 握手阶段拒绝 | 三种握手均失败。合法连接协商 TLS 1.3 |
| 混用节点 A/B 的 bearer 和证书；用 B 的完整身份读项目 1；用 A 令牌走明文回环；从节点端口访问 Owner 路由 | 证书、令牌、项目须匹配，Owner 路由独立 | 三种跨身份或项目访问均为 403，明文回环为 403；Owner 任务和授权重载路由均返回 404 |
| 用旧证书领取任务，添加新指纹并重载，替换 Worker 的证书和密钥文件，再次领取 | 两张证书在重叠期均可用；进程无须重启 | Owner 重载 204，新旧证书读取本项目均为 200。PID 45816 完成任务 #1、#2，各一个执行 |
| 保持旧 TLS 连接打开，移除旧指纹并重载；从原连接、旧证书的新连接及新证书查询，再创建第三个任务 | 撤销对后续请求即时生效，相同节点继续回报 | 保持原 TCP 套接字的请求 403，旧证书重连 403，新证书 200。相同 PID 完成任务 #3；三次回报均为 `4e85a882d1173169966556a22200034f6a3a7f04` |
| 重启控制面再查询 | 已撤销授权和任务结果持久保留 | 旧证书 403、新证书 200，任务 #3 仍为 DONE |

旧、新叶证书的 DER SHA-256 分别为 `591105d8abeb8afef6474d8ac9ce67ce5398466c14a44a4d713fd96f0cb66a86`、`dbcaefdeaef041e330e8d344df07c533571f31e38b0c7bfe24c76f61373a7d3b`。本次环境为同一主机，专题 11 的外部设备 Tailscale 验收另行记录。

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 154 项通过，零测试跳过。Go 静态检查通过。

具名 Worker 客户端证书轮换对应 ForgeOS 提交 `40736ea2141c80ac4a6f1e5b4ba7bd1f0857c72a`。

## 专题 10、13、14 · Worker 本地 CI 日志、报告和产物保留期限

从历史真实候选备份恢复 PostgreSQL、Worker SQLite、CI 运行目录和归档，以原节点身份、原控制面地址运行正式程序。下面的文件故障和旧备份还原均只发生在独立验收副本。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 读取默认策略，仅给项目 2 设置期限。用项目 1 的节点尝试修改策略，再由 Owner 将项目 1 的日志期限设为 1 秒 | 默认无限期，项目隔离，仅 Owner 能改策略 | 项目 1 初始队列为空。节点写策略 401，负数 400。Owner 设置后产生 `local_ci` 修订。另一节点冒名查询 403，以自身身份查询为空，代为确认 404 |
| 在节点离线时让三个历史日志到期，损坏 format.log，给 build.log 恢复真实待传引用，并移走中央 unit.log。启动 Windows Worker | 先修复中央副本。损坏及待传文件留在本地，其他文件独立处理 | unit.log 的中央下载与原文件逐字节相同，随后本地副本移除。format.log 和 build.log 保留，待修复及补传后均被清理；两份报告、run.json 和额外文件保持原内容 |
| 将本地策略恢复为无限期、重启服务、重新放回已清理的日志 | 已到期项继续处理，中央策略、候选回执独立保留，节点复查旧备份 | 三份中央日志和两份报告仍可逐字节下载；旧日志再次清理，重复确认返回 204。候选 `34faab0d235cbc0ea274b250826f02e232e246ff` 的原 CI 回执完整 |
| 在 Linux 对失败候选分别设置中央产物、本地报告与产物期限，将一份报告替换为指向工作区外的符号链接 | 中央 410 允许本地产物清理；符号链接及其目标保留，其他报告正常归档 | UID 65534 的 Worker 保留链接和目标，仅在恢复正确常规文件后清理该报告。两份中央报告可回读，build-1.bin 的中央读取 410，恢复旧本地产物后重启节点再次清理。三个日志与 run.json 逐字节相同 |

Windows 恢复库为 `forgeos_local_retention_d25cd87d`，运行 `ci-run-3034990605`。Linux 恢复库为 `forgeos_local_artifact_ecabc830`，运行 `ci-run-1446598496`，失败候选为 `2b14e98d37b2038b035bcd2df86dcacca79cbc21`。两个本地清理策略的修订编号均为 2。启用 PostgreSQL 18 和 Docker 的全量 Go 回归 155 项通过，零测试跳过。Go 静态检查通过。

Worker 本地 CI 文件保留策略对应 ForgeOS 提交 `9f0c70425ac15942d093bcf6f940754486832c2f`。

## 专题 11 · 从预览扩展截图到 Owner 确认归档

在 Chrome for Testing 156.0.8078.4 加载仓库内的扩展，用正式控制面和 Web 生产构建打开固定候选 `304948ec31c7e6616230934f8d6de1b97f6da2cf`。PostgreSQL 18 恢复库为 `forgeos_capture_982e64b4`。测试在实际候选页面的 DOM 中添加了五个合成隐私区域，便于逐像素核对遮挡。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从工具栏配对工作台，打开候选，点击“截取当前视口” | 采集原文档且排除受保护控件；能丢弃草案 | 普通洋红色参考块保持原像素，密码框、文本区、iframe、显式私有区域和封闭 Shadow DOM 宿主五处均呈背景白色。源控件随后恢复可见。截图和工作台地址不携带原页面的查询参数或片段 |
| 在采集期间连续改变隐私标记；对稳定页面重新采集，再从扩展页打开工作台 | 隐私边界变化时拒绝传图；稳定时仅把草案传给配对的工作台 | 变化中的页面显示重试提示，发送按钮不可见；另一稳定草案进入已登录工作台。登记前反馈计数保持原值，丢弃入口也从真实工具栏验收 |
| 提交观察后丢弃第一次 HTTP 回应，再次提交；收起并重开截图编辑器 | 复用反馈编号并恢复原图，需要 Owner 单独确认才产生附件 | 两次提交均对应反馈 #7，登记后附件数为零。“重新载入扩展截图”恢复原图。在参考块上添加遮挡后点击“确认并提交截图”，得到唯一附件 #10 |
| 核对上传像素、错版本和控制面重启后的下载 | 已遮挡像素与回执一致；错版本不可提交；归档可恢复 | 参考像素从洋红变为黑色，五个私有位置保持白色。归档 PNG 为 784 × 292、1573 字节，SHA-256 为 `44680831acd10ab0519f32f5242448e6dbd138700286be667b7332da86ad9df7`；重启下载字节一致。改为其他提交后，工作台显示版本不符，提交按钮禁用 |

`scripts/probe_preview_extension.py` 的实际工具栏回归通过，覆盖此次截图与既有 DOM、Canvas、嵌套 iframe、取消和隐私行为。TypeScript 检查、Web 生产构建、Go 静态检查通过。启用 PostgreSQL 18 和 Docker 的全量 Go 回归 155 项通过，零测试跳过。

预览扩展截图对应 ForgeOS 提交 `732a4a89b1d034d2b643dc71c36e94b1c86e2456`。

## 专题 11、13 · 在反馈工作台归档、回放和清理录屏

从 Chrome 录制真实候选预览 `304948ec31c7e6616230934f8d6de1b97f6da2cf`，加入用于核验的音轨、标题后，在独立恢复库 `forgeos_clip_fd0e9828` 通过 Owner 页面归档。此处验收文件进入工作台后的全过程。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 选取录屏并回放，在上传回应返回浏览器之前断开，再次确认 | 提交前仅本地回放，丢失回应后重试复用同一回执 | 反馈 #7 原有截图 #10；选择视频后仍只有一份附件。两次提交后只有一个录屏 #11 |
| 在工作台回放已归档文件，检查下载轨道、大小、摘要，重启控制面再下载 | 可播放，保留视频及时间信息，音轨和输入标题移除 | 已在 Chrome 播放并暂停。640 × 360、1.807 秒、16809 字节。FFprobe 确认单一视频轨且原测试标题已移除。SHA-256 为 `aedb85417dc8624e4e6e2e3df96ed8a7e7d101c856e79e14adb8193076924402`。重启后字节相同 |
| 用节点身份或其他任务路径读取附件，上传无效内容 | 限定 Owner、本任务与有效视频 | 节点返回 401，跨任务读取 404，无效上传 400 |
| 将本项目录屏保留期设为 1 秒，到期后恢复为无限期并重启 | 录屏到期不可恢复，截图仍有效，记录可审计 | 节点改策略 401。录屏读取及重传返回 410，关联修订 #1。重启清理了物理 WebM；反馈仍有已到期的录屏 #11 和可读取的截图 #10 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 156 项通过，零测试跳过。Go 静态检查、TypeScript 检查与 Web 生产构建通过。

反馈录屏归档与保留期限对应 ForgeOS 提交 `6280acdf49459b731cf283087e677a281fce251b`。

<a id="adr-evidence-014"></a>

## 专题 11、ADR-014 · 从候选标签页录制交互并绑定反馈

在 Chrome for Testing 156.0.8078.4，从原生工具栏启动和停止扩展录制，使用正式控制面、工作台生产构建及 PostgreSQL 18 恢复库 `forgeos_native_record_5d233bdc`。候选 `304948ec31c7e6616230934f8d6de1b97f6da2cf` 的实际预览中放置了可点击的参考色块和五处合成隐私内容。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在候选上开始录制，切换到独立红色标签页，再回原候选点击参考块，停止录制 | 录下原标签页的交互；其他标签页和私有控件不进入证据 | 共解码 9 帧，未出现红色标签页。密码框、文本区、iframe、显式私有区域和封闭 Shadow DOM 的采样点在所有有画面的帧中均为白色。回放中参考块从 RGB(199,0,201) 变为 (21,190,45)。停止后源控件恢复 |
| 预览录屏，打开工作台，登记观察，再确认提交录屏 | 绑定固定任务、预览、执行及提交；只有 Owner 确认上传后生成附件 | 登记前反馈数量未变；反馈 #7 初次生成时附件为空。Owner 本地回放并确认后生成附件 #10，800 × 500、2.473 秒、1986 字节，FFprobe 仅见 VP8 视频轨 |
| 重启控制面，再次下载，并在独立工具栏回归中测试取消、中途换文档与隐私变化 | 归档可回读；丢弃的片段不能发送，页面遮挡能撤销 | 重启下载字节一致，SHA-256 为 `4f109dd195c49793341641c818d784158425af18b68cd79b41dec10aa654f017`。录制中插入后移除封闭 Shadow DOM、重载源文档均触发丢弃。关闭录屏核对页后，表单恢复可见 |

同版工作台还重跑了扩展截图、反馈提交丢失后的重试和 PNG 归档，原遮挡像素与摘要一致。启用 PostgreSQL 18 和 Docker 的全量 Go 回归 156 项通过，零测试跳过，Go 静态检查、TypeScript 检查和 Web 生产构建通过。

### 专题 11 · Worker 动态应用预览生命周期

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 启动真实 Worker Preview Manager，以固定提交申请 local-mock，将工作区启动脚本改成退出 | 运行提交中的服务，保持 Docker 沙箱隔离 | 候选 `04726f294b96963ccc1753dd42274eeca3c80bba` 启动了容器 `b5f66017996c`。应用只监听容器内 127.0.0.1，Docker 显示网络 none、用户 65534:65534、无端口映射 |
| 重复申请同一实例，提交带查询串和正文的 POST，再重启 Manager | 复用原实例和内存状态，服务凭据与浏览器 Cookie 不进入应用 | 同一租期返回 204，不同租期返回 422。应用核对请求头后返回 201 和计数 2。重启 Manager 后 GET 仍为 2，HEAD 无正文 |
| 连续两次停止，再新建 8 秒实例并退出 Manager | 停止可重试，过期无需控制进程在线 | 两次停止均为 204，旧地址返回 404，Docker 查无原容器。新实例初始值为 0，离线等待 9 秒后 Docker 查无该实例 |

### 专题 11 · 工作台打开固定候选的动态应用

使用 PostgreSQL 18、正式 Worker 和控制面、Docker、Chrome 及单独的 Caddy 进程。测试库 schema 为 `dynamic_preview_a80dd6c1`。Worker 从固定候选完成实际验证，任务 #1 的执行 #1 进入 WAITING_ACCEPTANCE。随后将克隆的启动脚本改成退出。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| Owner 从任务详情打开“本机预览”，启动 local-mock，打开应用并点击 Add two | 应用来自通过验证的候选，能处理带正文和查询的同源 POST | 项目、任务、执行、工作树和 SHA `87dbdee4aa4fd35598abe4c0f885d40eed9e99cd` 与执行相符。Chrome 显示 2，GET /state 返回 200、同一 SHA 和正文 2。错误 SHA 返回 409，Worker 凭据返回 401。跨来源 POST 返回 403 |
| 重复申请，重启控制面与 Manager，再经 Caddy 的候选 HTTPS 域名提交同源 POST | 原状态保留，私网域名能执行操作，网关和服务凭据不会进入应用 | 重复申请返回相同回执。重启后读到 2，持客户端证书的请求使计数变为 4，返回 201。应用核对未收到 Authorization、Cookie 或内部实例头。无证书握手失败 |
| 从工作台停止，再打开历史反馈。创建 8 秒预览并等待 9 秒 | 停止和到期均撤销路由并移除容器；历史反馈可查看 | 两个地址都返回 404，均查无容器。工作台历史页显示 local-mock，两个回执分别为 stopped 和 expired。相同候选的静态目录仍返回 200 |
| 关闭 Manager 后重新申请动态预览 | 报告启动失败，不增加活动回执，独立的静态预览仍可用 | API 返回 503，查询所得历史与申请前一致，静态地址仍为 200 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 159 项通过，零跳过；Go vet、TypeScript 检查和 Web 生产构建通过。网关实测在本机完成。外部设备经 Tailscale 访问及独立 Staging 的第三方回调仍待验收。

### 专题 11 · 独立 Staging 回调网关

Caddy 2.11.4 使用正式 Staging.Caddyfile 和短期 TLS 证书。发送者、网关和接收器分别运行在三个 Docker 容器中。使用独立的前端网络 `staging-4a7f9046-edge` 和应用网络 `staging-4a7f9046-app`，只有网关加入两者。测试签名密钥只挂载给发送者与接收器。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 验证配置，发送带 state 的 GET 到 /callback，再发送含投递 ID、签名和 JSON 原文的 POST | 原查询、正文和签名到达独立应用的 /provider/hook | Caddy 配置验证通过；GET 返回 200；应用逐字节核对 HMAC 后返回 202 和 fixture-4。错误签名返回 401 |
| 请求工作台根、/ui/、/tasks/1、/internal/claim、其他回调子路径，以 DELETE 请求回调，或使用私有预览 Host | 三方客户端只能到达指定 Staging 回调 | 7 种请求均返回 404。应用日志仅出现前一步的 3 个回调。发送容器无法解析后端服务名 |
| 对照真实提供方的投递 ID | 在外部提供方和接收应用同时核对相同 ID 与实际投递结果 | 待验收。本次发送端是隔离容器内的测试客户端 |

### 专题 01 · Ruby Bundler 仓库初始化

PostgreSQL 18 测试 schema 为 `ruby_0a2dc902`。原生对照在隔离 Docker 中使用 Ruby 3.4.12 和 Bundler 2.6.9。对两版项目各执行 `bundle lock --local`，分别读取 `Bundler.definition.dependencies`、`Bundler::LockfileParser#dependencies` 及其完整 specs；工作台经正式 Worker 检查对应提交。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 登记项目，在任务 #1 展开 Gemfile 与 Gemfile.lock，归档初始化报告 | 各自显示来源 blob；工作台的直接依赖与两份原生结果相等 | 提交 `6b16a325f3e5e9308f31040e8618160f15e31a26` 的两份直接依赖均为 counter-first、json。原生 specs 包含的传递依赖 counter-helper 未混入报告 |
| 通过工作台生成 Management 归纳输入，检查其他清单中的多行 Ruby 表达式和无效锁文件 | 标明未解析来源，保留独立锁文件依据 | Management 取得包名及锁文件直接依赖标识，动态 Gemfile 标为未解析，无效锁文件标为格式错误。字符串中的伪声明、私有路径参数与注册表地址均未进入清单 |
| 改成 counter-next，在项目页检查新 HEAD 并归档；移开克隆、重启控制面，从 Chrome 依次打开两份报告 | 新任务读取新版本，旧回执保持原样，离线仍可核对 | 任务 #2 的 `84656de1c17d3c718b909eb7a71178e73c86ef7e` 对应 counter-next、json。离线两条任务的工作台和归档 JSON 分别恢复各自的原始结果 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 160 项通过，零跳过。Go vet、TypeScript 检查和 Web 生产构建通过。

### 专题 01 · Gradle 版本目录

在独立 Gradle 用户目录运行 Gradle 8.14.3、Java 21 的离线模型任务 `probeCatalog`，从 VersionCatalogsExtension 读取三个库别名和一个组合，并从 implementation 配置读取项目实际声明。ForgeOS 经正式 Worker、PostgreSQL 18 和 Chrome 完成同一提交的初始化。最终测试 schema 为 `gradle_bfe6ac64`。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 展开初版的 gradle/libs.versions.toml，逐项核对原生 libraryAliases 和 bundleAliases | 标明模块坐标，保留组合成员顺序及重复项 | 提交 `62374ec815863b73d973fb78c4723f1d968e2cb2` 中，core.api 对应 example.core:api，core.kit 按 util.lib、core.api、util.lib 排列，与原生模型一致。optional 存在于目录，在实际 implementation 配置中没有出现。指向未定义成员的另一份目录标为无效 |
| 从工作台归档并申请 Management 归纳，再修改目录，在项目页检查新 HEAD | 每个别名和组合能追溯到本次的 blob；后续提交独立归档 | Management 输入包含已核对的库和组合；第二次检查 `23c24c6150c1c4bb31addb3d2f10a824935a612c` 显示 example.core:next。原生模型、工作台与归档 JSON 一致 |
| 移开克隆并重启控制面，从两个任务回读清单和初始化报告 | 两版均从数据库还原 | Chrome 和归档分别显示 example.core:api 与 example.core:next，原报告未随新版本变化 |

本项开发期间，启用 PostgreSQL 18 和 Docker 的全量 Go 回归 161 项通过，零跳过。最终版本对别名中的横杠、下划线和点作同样的正规化，并以 Gradle 的实际版本引用及完整 localrepo 包测试复验。Go vet、TypeScript 检查和 Web 生产构建通过。

### 专题 01 · Gradle Groovy 与 Kotlin 构建脚本

使用 Gradle 8.14.3、Java 21 和离线构建的多项目样例。根项目使用 Groovy，子项目使用 Kotlin。自建模型任务 probeScripts 读取两者实际 configuration.dependencies，测试库 schema 为 `gradlescript_d7dcac48`。扫描由正式 ForgeOS 入口从 Git 对象完成。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 Chrome 登记并检查提交 `becd38184e9c271472f8292efdd46440cc4e2813`，展开两份构建脚本，将直接坐标与原生模型比对 | 两种 DSL 保留配置归属；版本目录引用可在同一提交中核对 | Groovy 的 compileOnly 为 example.tools:compile，testImplementation 为 example.test:fixture，implementation 引用 libs.bundles.core.kit。展开后的坐标集与原生模型一致。Kotlin 识别 BOM、project(:support)、库别名和带 isTransitive=false 配置块的直接声明 |
| 对照 Kotlin 的计算值和演示字符串，读取 Management 输入 | 无法从源代码确认的值不写入直接声明，字符串中的伪声明不产生依赖 | Kotlin 的 runtimeOnly 在原生模型为 example.dynamic:runtime；ForgeOS 记录一个未解析来源，未写入值或演示字符串内容。Management 从对应 blob 读到按配置的声明和未解析状态 |
| 修改 Kotlin 的 compileOnly 与目录指向；重新检查，归档两份报告，然后移开克隆、重启控制面 | 新旧快照都可从工作台独立回读 | 提交 `ce90667c482b2f1107bba59ddca961cc62695182` 中，compileOnly 改为 example.direct:updated，库别名对应 example.core:next。Chrome 和归档 JSON 分别恢复初版及新版结果 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 163 项通过，零跳过。Go vet、TypeScript 检查与 Web 生产构建通过。

### 专题 04、07 · 工作台查看运行中的原生 Harness 进程

Windows 正式 Worker、PostgreSQL 18、Chrome 与私有 beta 的两条真实 Issue。两次运行各从独立 schema 启动，在模型进程自然运行期间重启控制面，任务完成后通过候选验收，交付器等待必需 CI、合并并关闭来源 Issue。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 打开 Codex 执行的“最近的节点进程报告”，比对操作系统中的可执行文件，重启控制面并查询同一任务 | 执行仍在运行，节点 PID、原生 Thread 和实例保持一致 | Codex 0.169.0-alpha.11.1，实例 1，PID 37808，Thread `01a1e690-3aab-7be3-a88e-965cc29ccf34`。最终回执为相同身份、退出码 0。候选 `cc2e8e2b2725009acc7598146c8efa98a4d86222`，合并 `b2e756a8c738b79a188d8e0cc1926b7b40a0333d`。[PR #58](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/58) · [Issue 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/57#issuecomment-6931860523) |
| 以 Claude Code 复验同一操作，随后对照完整进程记录及交付回执 | 工作台能在进程退出前显示实际 Claude PID 和初始化事件确认的 Session | Claude Code 2.1.310，实例 1，PID 43664，Session `efadacc8-2f95-4555-ab76-7c681a523028`。重启后仍在运行，最终退出码 0。候选 `0a7b4ba401fc24802b5450616ad654207eb425d1`，合并 `6abb5f7fc4e75ecc198dcb0b8776be37516af903`。[PR #60](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/60) · [Issue 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/59#issuecomment-6931930995) |
| 以错误租约、过期租约或其他节点回报，重放旧实例、旧会话字段和退出前状态 | 只接受任务授权范围内的新观察；旧状态不得覆盖现值 | PostgreSQL 和 HTTP 回归分别核对冲突、越权、非模型任务以及相同退出回报的重试。两次真实交付均在 acceptance-check 和 forgeos/local-ci 成功后合并 |

全量 Go 回归启用 PostgreSQL 18 和 Docker，164 项通过，零跳过。增加节点权限断言后再运行了控制面节点测试。Go vet、TypeScript 检查与 Web 生产构建通过。本项验证控制面重启；Worker 自身重启后的在途会话恢复另行验收。

### 专题 07、13、16 C.3 · 运行中的工具调用

正式 Windows Worker、PostgreSQL 18、Chrome 和私有 beta 仓库。两个任务均从工作台对照原生进程，在执行中重启控制面，最终经固定候选验收、必需检查、合并和来源 Issue 关闭。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 打开 Codex 任务的进程报告，在原生进程退出前查看工具，再在最终回执展开该进程 | 已开始的调用可辨识，终态关联原调用 | 收到 8 条事件。3 次命令执行有 2 次完成、1 次失败；1 次文件修改完成。模型退出码 0。候选 `05e25448bf5457db2d0866b50903c0117d40b3d1`，合并 `587aed1fe547445029fb3e2000bd495830b656f3`。[PR #62](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/62) · [Issue #61 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/61#issuecomment-6932170930) |
| 在 Claude Code 任务执行时查看工具，再核对失败与并行工具的回报 | 每个结果对应发起它的调用，单次读取失败可与整个任务的结果区分 | 收到 20 条事件，含 Glob、Read 和 Write。一条读取失败，后续调用完成。候选 `5b9bef0e70a3e18f05064528d2be299da925a27f`，合并 `fd2950a3218f2a9b9fd044167d39197cef42dbec`。[PR #64](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/64) · [Issue #63 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/63#issuecomment-6932287848) |
| 对照浏览器运行中读取的样本、控制面保留的最后观察、最终回执与节点 SQLite | 样本应是完整序列的前缀，终态字段和顺序与节点相符 | 两个样本均为对应终态序列的前缀。两份最后观察、最终回执与节点记录逐字段相同。每个工具事件只有序号、调用号、类型、状态和节点时间。两个 PR 的 acceptance-check 与 forgeos/local-ci 均成功 |
| 分别写入 64 条带私有命令、输入、ID 或输出的模拟原生事件，重读节点与上报窗口；向真实 PostgreSQL 重放旧窗口和改写的调用记录 | 节点保留 64 条，回执只取后 32 条并保持编号，原文不进入回执，旧窗口或历史改写被拒绝 | 回归断言通过。启用 PostgreSQL 18、Docker 的全量 Go 回归 165 项通过，零跳过。Go vet、TypeScript 与 Web 生产构建通过 |

当前回执和工作台各显示每个进程最近 32 条事件。更早的工具记录可在所属节点的 SQLite 中查询。通用实时文本日志仍待实现。

### 专题 06、16 C.2 · 在任务树核对计划修订

用正式控制面、PostgreSQL 18 和 Chrome 打开此前已实际交付的 Goal 16 与 Goal 13。另从私有 beta 的 Issue #2 在隔离 schema `goal_revision_b6c6f303` 导入工作项，验证滚动草案尚未激活的状态。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 打开 Goal 16，从生效版往回逐版查看 | 五个版本可独立读取，里程碑和子项对应所选修订 | 第五版显示任务 17、18；第四版有 Cross-check links，任务 19 显示当前已从计划移除；第三版可进入任务 17。第一、二版的计划条目保留原文，无后续执行的链接 |
| 从第四版进入任务 19，再点击子任务的 Goal 链接；重启控制面并刷新 | 返回第四版并继续读取相同目标 | 浏览器回到 `#/tasks/16/plans/4`。重启后仍可读，向前和向后导航均成功。从第二版点“生效版”打开第五版 |
| 打开第 99 版，再返回最新版；打开另一个已归档的 Goal 13 | 缺失版本明确报错，归档版本可核对 Git 提交 | 第 99 版显示“找不到此计划修订”，可返回第五版。Goal 13 在页面显示归档 `35d9dc33e3ae606ed3e2c9f5388402a5d328c058`，与正式 API 一致 |
| 激活第一版后，用相同 key 提交改变目标的第二版，分别查看两版 | 草案展示新规格，原子项仍绑定已生效的第一版 | 第二版显示“尚未生效”和新目标，未关联现有执行；点击生效版，原目标及任务 2 的链接均可见 |

包含 PostgreSQL 18 和 Docker 的全量 Go 回归 165 项通过，零跳过。Go vet、TypeScript 与 Web 生产构建通过。

<a id="queue-performance"></a>

### 专题 14、16 A.10 · 实际任务量下的队列和事件存储

以 `9cadde2f94d16f5fb769d69b33950ae1183768ea` 构建 Windows 控制面与 Worker。逻辑复制实际验收库的 public schema 到本机独立 PostgreSQL 18.3，从同一快照创建两份数据库。起点均为 5 个项目、27 项任务、30 次执行、173 条任务事件。每个数据库启动 4 个正式 Worker，各有独立工作区、节点 SQLite 和一个执行名额。每个项目使用独立验收克隆。没有配置 GitHub 交付。

| 操作 | 预期 | 基线副本 | 复测副本 |
| --- | --- | --- | --- |
| 用 4 个并发 HTTP 客户端交替读前 30 项与任务 20 的 20 条历史事件；每个客户端预热后各读 50 次 | 任务总数一致，历史事件逐字段相同；取得两个入口各 200 个样本 | 列表 P50/P95/P99 为 2.93/5.17/50.28 ms，详情 6.76/36.78/78.80 ms | 列表 2.97/9.74/46.97 ms，详情 6.56/38.67/78.38 ms |
| 通过 Owner API 在 5 个项目轮流创建 27 个 Git HEAD 任务；核对领取、完成及各项目实际 HEAD | 每项执行一次，事件为创建、领取、完成，两个执行事件均指向同一执行 | 入队 HTTP P95 1.93 ms，排队 P95 2132.44 ms，领取至完成 P95 262.93 ms。整批 2.662 秒，10.14 项/秒 | 入队 HTTP P95 1.56 ms，排队 P95 2036.75 ms，领取至完成 P95 227.55 ms。整批 2.529 秒，10.68 项/秒 |
| 核对行数并在同一终态任务上复测读取 | 每库新增 27 次执行、81 条事件，原记录可回读 | 54 项任务、57 次执行、254 条事件。列表 P95 15.53 ms，详情 P95 48.28 ms | 行数相同。列表 P95 5.20 ms，详情 P95 38.36 ms |
| 从 PostgreSQL 对实际任务详情、列表、READY 候选查询运行 EXPLAIN ANALYZE | 分开记录规划与执行耗时 | 详情 30.06/1.16 ms，列表 0.62/0.63 ms，候选 3.35/0.11 ms | 详情 28.85/1.05 ms，列表 0.99/0.84 ms，候选 3.15/0.10 ms |

两轮全部读取和 54 项新增任务均通过校验，四个节点均有完成回执。整批时间从首个入队请求开始，到客户端观察到最后一个终态为止，包含 Git 进程、Worker 轮询与客户端查询间隔。SQL 单次 EXPLAIN 的规划耗时与并发 HTTP 百分位分别测量。项目文档未指定毫秒级 SLA，以上数值用于同负载复测。[复测入口](14-deployment-and-operations.md#测量队列与事件读取)

#### 四个并发客户端与四个节点的连接复用

基线探针提交为 `0aab874402a8f24fca7aa3a8d901324f5609e2a0`。在两套独立的 PostgreSQL 18.3 实例上，各按原配置、调整后、调整后、原配置的次序复验。每次均从同一逻辑快照另建数据库，使用相同的控制面源码、Worker、查询与 27 项真实工作。唯一代码差异为控制面最多保留的空闲连接数从 Go 默认的 2 调整为 8。预期为减少重复建立的数据库会话、缩短并发详情读取的尾部等待，并保留派发与事件结果。

| 实例与次序 | 最多空闲连接 | 新建 PostgreSQL 会话数 | 入队批次完成后，详情 HTTP P50 / P95 / P99（ms） |
| --- | ---: | ---: | --- |
| 一 · 第一次 | 2 | 36 | 6.61 / 37.53 / 78.60 |
| 一 · 第二次 | 8 | 1 | 7.67 / 9.19 / 9.70 |
| 一 · 第三次 | 8 | 0 | 6.74 / 8.31 / 8.80 |
| 一 · 第四次 | 2 | 33 | 7.31 / 38.27 / 81.97 |
| 二 · 第一次 | 2 | 33 | 6.39 / 35.34 / 78.93 |
| 二 · 第二次 | 8 | 0 | 6.18 / 7.84 / 8.64 |
| 二 · 第三次 | 8 | 0 | 6.48 / 9.32 / 9.64 |
| 二 · 第四次 | 2 | 36 | 7.24 / 12.93 / 83.17 |

每行包含 200 次完成批次后的详情读取。新会话数是四个节点登记后到服务停止并刷新统计之间，`pg_stat_database.sessions` 的增量。第一套实例部分测量与 Go 回归同时运行；第二套实例在回归结束后单独复测。总计 216 项新增任务全部完成，每项一次执行、三个有序事件，回执与项目 HEAD 一致。Go 回归启用 PostgreSQL 18 和 Docker，165 项通过、零跳过，Go vet 通过。

### 专题 01、03 · 人工处理事项的站内通知

正式 Windows Worker、控制面、PostgreSQL 18、Chrome 与私有 beta。验收 Issue #65 经过原生 Codex、候选验证、检查点答复与 GitHub 交付。另用独立节点挂载和已有多项目验收数据覆盖失败提醒及历史迁移。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从工作台打开新候选通知，标为已读；重复请求，重启控制面并刷新 | 待处理未读数从 1 到 0；候选仍待验收，首次已读时间保持不变 | 执行 1 产生通知事件 3，两次标记保留同一时间，重启后任务仍为 WAITING_ACCEPTANCE。Worker 令牌读取列表与标记均返回 401 |
| 对固定候选连续两次提交同一问题，从通知打开任务并回答 | 一个问题通知，答复后回到原候选；两种通知的阅读状态独立 | 待处理列表只含问题事件 4。Chrome 提交答复后问题退出待处理，原候选已读状态保持。批准后待处理列表为空，两条历史记录均可读 |
| 核对候选、必需检查、合并及 Issue 回执 | 通知不影响候选绑定和交付门禁 | 候选 `002e4dd35c530e8806c603e983949eb102e26453`，acceptance-check 与 forgeos/local-ci 成功，合并 `dc3818f109e86a377852387ddc3fb9ac54090cfa`，Issue 已关闭。[PR #66](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/66) · [Issue 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/65#issuecomment-6932967051) |
| Chrome 停留在任务列表，另一个正式 Worker 从缺少项目克隆的挂载执行 Git HEAD 检查 | 无需刷新即可看到待处理角标，通知能定位失败任务 | schema `notice_failure_e4761b03` 的任务 1、执行 1 失败。导航显示 1 条，点击后进入任务页，可见对应节点错误 |
| 升级已有验收库，切换全部记录，从旧通知打开计划原修订 | 保留既有任务与事件，历史计划通知能回到审核依据 | 28 条记录中有 19 条候选、5 条执行失败或失联、3 条计划及 1 条问题。待处理有 5 条。从历史事件 63 打开 Goal 16 的第一版，浏览器显示原 Management 审核会话及 revise 决定 |

PostgreSQL 回归还覆盖跨页、结果重报、并发标记和拒绝标记普通任务事件。启用 PostgreSQL 18 和 Docker 的全量 Go 回归 166 项通过，零跳过。Go vet、TypeScript 和 Web 生产构建通过。本节覆盖站内渠道，外部渠道另行验收。

### 专题 05、06、16 C.5 · 从工作台批准指定计划修订

使用正式控制面、PostgreSQL 18、Chrome，以及独立凭据的原生 Management Worker。在隔离 schema `approve_plan_chrome2` 导入私有 beta [Issue #19](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/19)。仓库固定在 `fd2950a3218f2a9b9fd044167d39197cef42dbec`。本场景验收截止于子项就绪；此前的 Goal 完整交付见本页阶段三记录。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 提交 D2 下限的第一版单子项目计划，从站内通知打开审核和修订树 | Management 给出建议后由 Owner 决定，展示原生审核依据 | 会话 `01a1e6cb-57c1-7231-8d05-5108a8010355` 返回 approve，有效等级 D2。通知打开 Goal 1 的修订 1，Chrome 显示同一审核会话及明确的批准按钮 |
| 在 Chrome 批准第一版，将已成功的 HTTP 响应在浏览器交付前丢弃 | 能从重新读取的任务状态确认是否生效，重复请求安全 | 服务端返回 200，客户端收到网络错误并重新读取。页面显示第一版生效；重复 POST 后仍只有子任务 2，计划通知退出待处理列表 |
| 当前仍生效第一版时打开第二版草案；另一 Owner 请求先发布第三版，再放行第二版的批准请求 | 旧版本不能覆盖新草案，返回可核对的冲突 | 返回 409，页面显示冲突。第二版成为历史修订，其批准按钮消失。第一版保持生效，子任务 ID 不变 |
| 打开第三版，显式批准，重启控制面并从计划树进入子任务 | 当前版本生效，仍可从同一子任务追溯版本 | 重启后第三版为当前生效版本，任务 2 为 READY，绑定修订 3，子任务页可返回该版计划 |

验收发现同一 Goal 内切换固定修订时可能要等下一次轮询才能看到新草案。工作台现按版本变化立即重读 Goal；新库重跑了上述完整流程。可复用的 Chrome 探针另在 `approve_plan_chrome1` 通过旧版冲突、丢失响应及重复激活验证。TypeScript 与 Web 生产构建通过。

### 专题 06、16 C.5 · Owner 起草及修改退回的计划

正式控制面、PostgreSQL 18 和原生 Management Worker 在独立 schema `plan_edit_run2` 登记私有 beta [Issue #19](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/19)，固定基线 `fd2950a3218f2a9b9fd044167d39197cef42dbec`。从两个独立 Chrome 会话运行 [修订探针](../scripts/probe_plan_revisions.py)。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| Owner 起草 D2 计划，把 Issue 指定的 HTML 页面错误地写为 README，另一会话打开站内通知 | 固定第一版并收到可追溯的退回依据 | Goal 1 的修订 1 收到 revise，审核会话 `01a1e6d3-40a5-7991-a6ac-b6688605784b`，通知定位到该版 |
| 两边同时编辑第一版；第一会话纠正文件、标题和段落，增删临时里程碑及工作项，再提交自依赖 | 循环依赖被拒，作者可以继续修改 | 服务端返回 400，表单显示 cycle；移除自依赖后可继续发布 |
| 在服务端成功发布第二版后拦截浏览器响应，以原内容重试。另一会话从旧版提交不同内容 | 同一修改仅发布一次；另一个会话保留本地输入 | 重试定位到修订 2。另一会话收到 409 并保留原文字。修订 1 完整内容和审核记录均保持一致 |
| 核对修订 2 的范围、验收条件及两条上下文引用，等待原生审核，再从工作台激活并重启控制面 | 审核和激活分开，子任务只绑定获准版本 | 会话 `01a1e6d3-83d4-7d10-9c96-3f01145b7cb9` 批准，最低等级 D2。激活前无子项，激活后仅任务 2 为 READY，绑定修订 2。重启后任务详情逐字段一致 |

TypeScript、Web 生产构建通过。本场景验收至子任务就绪，来源 Issue 保持原状态。

<a id="adr-evidence-008"></a>

### ADR-008、专题 06、13 · 从工作台归档固定修订

在独立 schema `plan_delivery_live1` 和新建的私有 beta 验收克隆运行正式控制面、Management Worker、执行 Worker 与 Chrome。Owner 为 [Issue #67](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/67) 起草两版计划，基线为 `dc3818f109e86a377852387ddc3fb9ac54090cfa`。原生审核会话 `01a1e6da-55a9-7912-826a-d62ce2f74009` 退回第一版，`01a1e6da-a138-72a1-983a-02ee4de2152c` 批准第二版。Owner 从工作台显式激活第二版。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在验收克隆预置与计划不符的第一版引用，从历史版点击归档 | 拒绝覆盖已有引用，页面能说明核对对象 | 返回 409，页面提示核对克隆、基线和引用，引用仍指向基线。控制面没有写入归档 SHA |
| 移除上述测试引用，拦截一次成功的历史版归档响应，点击重试；再切换到生效版归档 | 每一版有唯一 SHA，可按修订重试 | 第一版为 `f0539209f77392b3bf71f2f3ddcf96fc5083c238`，第二版为 `0fa8c2a5941e62a5fa048e06f1cfbb444a5ca494`。重复请求均返回各自原 SHA |
| 归档前同时准备暂存和未暂存修改，比较 HEAD、状态及两份 diff，再检查 Git 树与数据库正文 | 保留 Owner 编辑，归档只记录计划文件 | 两次归档前后四项一致。两个本地引用分别解析到页面上的 SHA，父提交均为固定基线。相对基线仅有 `.forgeos/plans/goal-1.json`，逐字段等于对应修订的 plan |
| 重启控制面，刷新第二版并切回第一版 | 每个固定页面展示自己持久化的回执 | 两版分别显示原归档 SHA，生效版仍为修订 2 |
| 正式 Worker 执行修订 2 的子任务，Chrome 检查候选页面和单文件差异，在任务页批准 | Git 归档不改变交付基线，真实分支保护通过后方可合并并关闭父 Issue | 候选 `41177262f5d826b72acf06ae638463714c1f1070` 只新增指定 HTML，唯一 h1 和段落逐字符合 Issue。首次必需检查按验收夹具失败，只重跑失败作业；acceptance-check 和 forgeos/local-ci 成功后合并为 `eed7fa9f83776301e04421e582b3688b464bfa77`，父子均为 DONE，Issue 关闭。[检查运行](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43986198166) · [PR #68](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/68) · [父项回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/67#issuecomment-6933532226) |

启用 PostgreSQL 和 Docker 的全量 Go 回归 166 项通过，零跳过。Go vet、TypeScript 与 Web 生产构建通过。

### 专题 08 · 项目节点读取工作项上下文和前置交付

在已有验收数据库启动正式控制面。为 alpha、beta 和本地 Git 项目配置三个独立的 Worker 身份，用 Owner 和对应节点的 HTTP 入口读取同一任务，再向 GitHub 查询已合并 PR。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 读取 alpha 任务 18，再打开其 Goal 16 和已移除项 19 | 区分子项的固定依据与 Goal 的最近修订 | 任务 18 绑定需求 1、计划 4，Goal 最新和生效修订均为 5；已移除项仍读到计划 4。前置项 17 的合并 SHA 为 `1dea711e585b05dc1d6aa4a79cc9cf1c96e565ab`，与 GitHub 一致。[alpha PR #5](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/5) |
| 用 beta 节点读取任务 15，并读取独立 Issue 任务 7；本地节点读取 Git HEAD 任务 2 | 每项仅含已有来源，独立任务没有前置项 | 任务 15 绑定需求 1、计划 1，前置项 14 的合并 SHA 为 `e45f9066d9865c64d0702d00320df0f39d5d63ee`。任务 7 有来源需求且计划为空；任务 2 的 Issue、需求、计划为空。两项依赖列表均为空。[beta PR #5](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/5) |
| 交换项目身份，用授权项目路径查询其他项目的任务，尝试无效 ID、未知令牌和 Owner 专用路径 | 请求在相应边界被拒绝 | 越权项目为 403，项目内不存在的任务为 404，无效 ID 为 400，未知令牌及节点访问 Owner 入口均为 401 |
| 重启控制面，分别重读两项依赖任务 | 固定资料和交付证据可恢复 | 两个项目的完整响应与重启前一致。Owner 与项目节点的响应也逐字段一致 |

数据库回归同时核对不同修订的子项及其前置合并记录。启用 PostgreSQL 和 Docker 的全量 Go 回归 166 项通过，零跳过；Go vet 通过。

### 专题 06、08 · 实现回合使用固定需求和前置交付证据

在新的私有 beta 克隆和独立 PostgreSQL schema `development_context_role1` 部署正式控制面、Worker 和 Chrome。基线 `eed7fa9f83776301e04421e582b3688b464bfa77`。来源 Issue 指定两个带随机值的标题，计划与子任务规格只引用父需求，未抄入标题。第二项还要引用第一项未来的合并。[Issue #69](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/69)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 激活两项有依赖的计划，运行首项 Codex，在 Chrome 批准候选 | 读取父需求中的标题，只写本项文件；第二项继续等待 | 会话 `01a1e6e7-8b69-7ae3-b9d4-714a0e146bbc` 的候选 `a421f3fb80d5acff20ea364d4c75e623345bfb53` 只新增 `docs/capacity-context.md`，全文是标题“Capacity ledger 80460926”。复验两项必需检查后合并为 `a4ecd0207e90ace7e6369f13f9b52f79c91d308d`，父 Issue 仍开放。[PR #70](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/70) |
| 重启控制面后领取第二项，核对与前置项交付的版本关系、页面标题和回执正文 | 以首项合并版本为基础，只生成第二个文件，填入真实 PR 地址和完整合并 SHA | 会话 `01a1e6ea-2908-7463-9b42-b4de2ef04f3e` 的候选 `b036e53e71709e2b80a5694c4fa0841adbc2183c` 只新增 `docs/queue-context-receipt.md`，标题是“Queue receipt 0b148783”，正文含 PR #70 的完整地址及合并 SHA。Chrome 已按该候选批准。[PR #71](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/71) |
| 在相同基线的新克隆用原生 Claude Code 领取第一项，在本地验收并重启控制面 | 独立 Harness 同样能取得父需求标题，只修改分配的文件 | Claude Code 2.1.310，会话 `a85fb186-1066-417e-a0cd-3970973fdf94`，候选 `0c60cecbf565aee445dbe31446d81a4339c38601`。文件名、唯一标题和验证回执均通过。这个隔离场景在 ACCEPTED 截止，重启后可回读同一回执 |

第二项 `acceptance-check` 与 `forgeos/local-ci` 成功，实际合并 SHA 为 `50f39ddabab401ca5161ca97aaabc9553add310e`。两项交付后，父 Goal 为 DONE，来源 Issue 已关闭。[父项回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/69#issuecomment-6933868151)

启用 PostgreSQL 与 Docker 的全量 Go 回归 167 项通过，零跳过；Go vet 通过。同一版本重新通过三个项目身份的上下文 API 与重启复测。

### 专题 08、ADR-018 · 检索 PR 合并与 Goal 完成记录

使用正式控制面、项目节点授权和已有的多项目 PostgreSQL 验收库，对照 GitHub 中两个私有仓库的记录。另在独立 schema `knowledge_history_run1` 运行 [beta Issue #72](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/72)，从新 Goal 观察历史查询随交付变化。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 分别用两个项目的节点查已知候选 SHA、合并 SHA、PR 及已完成 Goal | 两种 SHA 定位到同一份已确认的交付；能核对完成时的计划版本与时间 | alpha 子项 17 的 PR #5 对应确认事件 82，Goal 16 对应修订 5。beta 子项 14 的 PR #5 对应事件 45，Goal 13 对应修订 1。两个远端 PR 的 head 和 mergeCommit 均与检索字段一致。[alpha PR #5](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/5) · [beta PR #5](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/5) |
| 交换节点检索另一项目的合并 SHA，按父子任务分别筛选，再重启控制面 | 来源按项目和任务隔离；重启保持结果 | 在本项目查对方 SHA 均为空，使用越权项目路径返回 403。筛选子任务只返回该子 PR；以父任务号筛选合并为空。重启后两组结果逐字段一致 |
| 激活新 Goal，在子项等待验收时查询历史。工作台批准候选并等待实际 GitHub 交付，按两个 SHA 和 Goal 目标重复查询 | 验收前两类结果为空，交付后各有一条可追溯记录 | 候选 `8811d1e5d657e8cffc235a1aa1bc1315b26a069b` 与合并 `a609b572656568e697509c66d9d6085fe43e1fbb` 均定位到确认事件 9。父 Goal 1 完成修订 1，时间与完成事件一致。综合查询同时返回需求、合并和 Goal；控制面重启后逐字段一致。[PR #73](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/73) · [Issue 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/72#issuecomment-6933972241) |

PostgreSQL 回归另覆盖 PR 等待必需检查、已合并尚未完成交付、重复提交回执和已领取关闭租约但尚未完成的 Goal。启用 PostgreSQL 和 Docker 的全量 Go 回归 167 项通过，零跳过；Go vet 通过。

### 专题 08、ADR-018 · 固定资料修订的知识提取和独立审核

正式控制面、PostgreSQL、Chrome、独立令牌的 Curator 和 Management 进程。两种角色分别通过原生只读 Codex 会话工作。验收材料记录队列服务重启前后观察到的相同偏移量 274。资料 UTF-8 摘要为 `18843f38d8ef6496be02a0b3db87e65277919365c8573b27f3d4abbecdfaa429`。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| Chrome 在“项目接入 → 项目资料”导入第一版，先仅启动 Curator | Owner 可见待审核建议；节点只能读取已通过的项目经验 | schema `curation_four` 的会话 `01a1e6fa-1825-7b93-9cf8-904b9fef72eb` 提取一条建议，引句与原文逐字相符。节点列表为空。两种角色令牌互访领取入口均为 401，节点读取 Owner 回执为 401，越权项目为 403 |
| 重启控制面，再启动 Management，观察工作台并按原租约重报两个回执 | 独立 D1 审核通过后才可供本项目读取；重报保留时间和记录数 | Management 会话 `01a1e6fa-5a5e-7090-9e09-da0d78745a68` 给出 approve、D1。Owner 和项目节点读到相同的一条经验，包含来源版本和摘要。两次重报均为 204，回执逐字段一致 |
| 在 Chrome 编辑第二版，仅保留会议议程，等待 Curator，再重启并返回历史版 | 新版取代旧版；无事实时记录空结果，历史审核仍可核对 | 保存第二版后节点列表立即为空。原生提取返回空数组；没有生成审核任务。重启后第一版仍可在工作台查看原会话及审核依据 |
| 用旧控制面导入未获批准的定价提案，再升级数据库 51→52。用受控 Curator 回报先提交伪引句，再提交有真实引句但越过原意的建议 | 旧资料原样回读；伪引句被拒；真实引句不足以使商业提案自动生效 | 摘要和旧资料一致。无依据回报 409。旧 Management 领取协议没有取得提取建议；新能力参数可与旧控制面通信。真实审核会话 `01a1e6fb-13b0-75f1-a770-381fc3d634d0` 给出 D2、needs_owner，项目经验与正式决策列表都为空 |

启用 PostgreSQL 和 Docker 的 Go 全量回归 168 项通过，零跳过。Go vet、TypeScript 和 Web 生产构建通过。首次 Chrome 复验发现前端请求路径白名单缺少回执入口；补齐后已从工作台复跑完整场景。

固定资料修订的提取和审核对应提交 `2df857ecb236b45d48f64d8d1a4f367a6e1facf6`。

### 专题 08、ADR-018 · Owner 明确要求记住一条项目经验

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| Chrome 从项目列表进入项目资料，选择“记住一条项目经验”，记录演练 R14 重启前后偏移量均为 274。用浏览器拦截第一次成功写入后的响应，在原草稿重新提交 | 保留原文、来源和草稿标识，只生成一个修订和一条提取记录 | 两次保存后均为修订 1，提取回执逐字段一致。类型筛选查到一条来自 Owner log R14 的请求；另一个项目查到零条。正文摘要为 `40c66eb16c2f1a224bee7b9aaaa191d151be461e8954accabdb9929059c631cb` |
| 启动专用 Curator，重启控制面，再启动独立 Management | 审核前经验列表为空，D1 批准后仅本项目可读；保留两次会话 | schema `memory_three` 的原生会话 `01a1e702-cc3e-7383-a382-b9e6625160f1` 提取一条带原文引句的观察，`01a1e703-2903-7a42-a40e-68d3b105b188` 批准。Owner 和项目节点均读到同一条经验，另一项目的经验和本项目的正式决策均为空 |
| 将记忆修订为仅含议程，再次重启，并在工作台按类型查回第一版 | 当前列表撤下过时观察；两个版本保留各自依据 | 第二版提取为空，第一版仍显示原审核会话；重复回报原租约未增加记录 |
| 在版本 52 录入导入资料、项目档案、环境说明，再用新控制面升级至 53 并提交记忆请求 | 原资料、摘要和提取状态保持一致；新增类型可正常提交 | 三种旧资料及其提取回执逐字段一致，新请求为修订 1、待提取 |

启用 PostgreSQL 和 Docker 的全量 Go 回归 169 项通过，零跳过。Go vet、TypeScript 和 Web 生产构建通过。

Owner 记忆请求对应提交 `382aa91b0251f0cb4010364e2c36de2122779a20`。

### 专题 05、08、ADR-018 · Owner 处理有固定来源的知识建议

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 Chrome 提交 R18 记忆请求，原文明确请 Owner 决定是否将试点 A 的折扣上限设为 12%。用正式 Curator 和 Management 分别领取 | 按待确认提案摘录，审核分级；经验和正式规则均保持待决定状态 | schema `disposition_native` 的原生 Curator 会话 `01a1e70c-8983-79e3-90f7-b34348b22fef` 提取一条带原文引句的建议，原生 Management 会话 `01a1e70c-ca1c-7602-9e09-0ad927b1b406` 给出 D2、needs_owner。此时两份有效列表均为空。固定修订 SHA-256 为 `e6e4e924a5d987d6eda914eb7f548b3529111662ba02ad0b356f60e6d6d8b307` |
| 从建议起草决定，将上限改为 8%，限定为 R18 试点，保存后进入提案详情，单独确认 | 保存只提出决策；确认后节点读取 Owner 实际决定，仍能定位原提案 | 工作台先显示 #1 PROPOSED，确认后为 CONFIRMED；节点读到一项已确认决定，经验列表为空 |
| 先在版本 53 写入含两项定价提案的资料，受控提取按原文引用，原生 Management 会话 `01a1e70c-868b-7203-8b21-3ec7f0d7fe01` 判为 D2。升级至 54 后丢弃第一个提案的成功响应，从原表单重试 | 保留旧审核，重试只创建一项提案，查看和写入受项目与角色限制 | schema `disposition_three` 的旧回执字段及来源摘要保持一致，新增提案列表起初为空。重试后仅一项待确认决定。证据包括固定路径、建议序号、摘要及审核会话。节点越权项目读取 403，在本项目路径读取对方的决定为 404，节点写提案为 401 |
| 确认第一项，拒绝第二项，重启控制面，在第二项原建议上另提一份适用范围更小的草案 | 旧审核、拒绝历史及后续提案共同保留；仅已确认决定生效 | 回执显示建议 1 对应 #1 CONFIRMED，建议 2 对应 #3 REJECTED；另提后，第二项同时链接到历史拒绝和新 PROPOSED 提案。另一项目默认决策列表为空 |

启用 PostgreSQL 和 Docker 的全量 Go 回归 173 项（含子测试）通过，零跳过。Go vet、TypeScript 和 Web 生产构建通过。数据库回归另覆盖 D1 approve、D2 revise 禁止建立来源关联，D3 needs_owner 可提出待确认决定。

Owner 将审核建议提出为决策并单独确认或拒绝对应提交 `2359fe38c621b41eeff8922e2f8a7c399102021f`。

### 专题 06、08、阶段三 · Goal 子项引用项目决策

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 beta #74 的单项计划中同时引用仓库路径、本项目已确认决定（重复两次）、未确认提案和另一项目决定。把随机标题只写进已确认决定 | 项目节点和实现回合取得一条有效决定；生成结果符合其适用范围 | 原 Issue、子项规格和计划均不含标题。正式 Codex 只改 `docs/goal-decision-reference.md`。首轮 `171d6656e7451ae63991824fcaf8e3ad2252b375` 多写了决策句末的句号，Chrome 退回。第二轮会话 `01a1e716-35b8-7542-ad40-f4346b53b6f2` 生成精确标题“Pilot receipt 1634c4bfdd”，候选为 `c1f5ac79c016d066c5a3f077d3e337a9da83a0d3` |
| 候选完成后使该决定失效，查询当前任务上下文，再从 Chrome 展开第二次执行的领取记录并验收 | 当前上下文移除失效决定；已执行的候选仍有可核对的原文 | 当前列表为空，执行记录仍展示原始决定和适用条件。工作台按修正后的 SHA 批准；两个必需检查成功，PR #75 合并为 `77fb286e314f7e372b5d736de79702bcb6871c7e`，父 Issue 关闭。重启后执行依据逐字段一致。[子 PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/75) · [父 Issue 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/74#issuecomment-6934839179) |
| 在独立数据库和工作区为同一 Issue 设置另一个仅存于决定中的标题，使用 Claude Code 2.1.310 | Claude 同样依据本项绑定的决定创建唯一文件；确认后可追溯原决定 | 会话 `15295d1a-6141-4f0e-9bc4-f16fe148a9d2` 的候选 `9f293829059fbb7381444120f1918c439c414288` 通过文档验证和本地批准，正文精确匹配该决定。使决定失效后，重启仍可回读原快照。此组在独立控制面完成本地验收 |
| 用 54 版控制面派发有决策引用的子项，在未结束的租约上升级到 55。用原租约回报受控失败，重试并使决定失效 | 旧执行保持原始回执，新执行固定领取时的有效决定 | schema `ref_upgrade_one` 的旧执行 1 升级前后逐字段一致；升级后当前上下文可查到决定，新执行 2 保存快照。失效后当前列表为空，旧执行仍无快照，新执行仍有一条 |

启用 PostgreSQL 和 Docker 的全量 Go 回归 174 项（含子测试）通过，零跳过。Go vet、TypeScript 和 Web 生产构建通过。另有传输回归覆盖超过 2 MiB 的普通任务上下文及独立角色响应上限。

子项执行引用已确认项目决策对应提交 `ddbaae25eb110553bcc3d958bd268117ea57331d`。

### 专题 08、ADR-018 · 确认项目决策触发知识提取

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从 Chrome 项目决策页提出并确认 R31 决定。正文记录导入器重启前后偏移量 918，另含保留检查点的规定。重复确认，进入决策详情并先仅启动 Curator | 提案阶段没有任务；确认后只产生一个固定来源。审核前本项目经验列表为空 | schema `confirmed_source_two` 的确认事件 #2、正文摘要 `36e753b79fc7f21182c5f50e5f9e040cbc4dd3d82af52ce10fb0b347f4940b4a` 在重复确认前后一致。原生会话 `01a1e722-6f41-7162-bfe4-d3be98b58b19` 摘录一项观察，原文引句精确匹配。节点读取 Owner 审核回执为 401 |
| 启动独立 Management，再从工作台提出、确认替代决定，重启后打开原决定 | D1 审核通过后仅本项目可读。新提案不撤下旧经验，确认替代后旧版退出当前列表 | 审核会话 `01a1e722-cdd9-75f1-a96d-045fb6784d05` 批准。经验引用原决定 #1 和确认事件，另一项目无记录。替代生效后本项目经验为空，只有规则的 R32 决定提取为空，重启后仍能回读原会话 |
| 用 55 版控制面准备仍有效、曾确认后失效、被拒和待确认的决定，以及一条已有提取回执的旧资料，再升级至 56 | 仅曾确认的决定补入队列；旧回执不变；截止时间自然到达后撤下经验 | schema `confirmed_source_upgrade` 的原资料回执及决定列表逐字段一致，补入两项。未确认的两项回执均为 404，虚假引句返回 409。受控 D1 审核后仅有效源可见；到达 `2026-11-17T01:30:24.123456Z` 后经验为空，历史审核仍为 approve |

首次原生会话把不连续的原文拼成引句，控制面拒绝了回报。Worker 现在在提交前核对引句，可在原租约内重新提取一次。保留首轮数据库，等待原租约到期后由修订后的 Worker 接手同一任务；提取会话 `01a1e724-bc59-7fd1-9f61-431c60c9a206` 与审核会话 `01a1e725-10dd-7940-8308-bf17ba362685` 完成提取、批准及替代后回读。受控 CLI 回归同时覆盖第二次成功和两次均不匹配。启用 PostgreSQL 和 Docker 的全量 Go 回归 176 项（含子测试）通过，零跳过。Go vet、TypeScript 和 Web 生产构建通过。

决策确认触发提取、独立审核与失效后撤下经验对应提交 `4d1d737b36e2eb072bf0f718a20b89fe3128b60a`。

### 专题 06、08 · 未来里程碑尚未拆分时领取当前子项

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在独立 PostgreSQL schema `rolling_ref_one` 从 beta #19 导入任务；确认一个项目决定，发布并激活含“Now”和“Later”的计划。当前子项引用该决定，未来里程碑的工作项为 JSON `null` | 当前子项可读取计划依据并被授权节点领取 | 修复前，节点上下文 GET 和领取 POST 均为 500；事务回滚，子项 2 保持 READY。数据库回归复现“cannot extract elements from a scalar” |
| 保留同一数据库和计划修订，以新控制面重启，重复两个节点请求 | 原修订可直接派发，未来里程碑继续留在计划中 | 两个入口均为 200；执行 2 领取子项 2，执行中的决定与上下文接口返回值一致，均引用 `/projects/1/decisions/1`。原计划逐字段相同，仍只有一个子项，父 Goal 为 ACTIVE。本次用受控失败回报结束验收租约 |

启用 PostgreSQL 和 Docker 的全量 Go 回归 176 项（含子测试）通过，零跳过。Go vet 通过。

尚未拆分的未来里程碑与当前子项派发对应提交 `f21b9b6bf53f7df26d366f7e7861c7567628ff57`。

### 专题 08、ADR-018 · 确认 PR 合并触发知识提取

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从 beta #76 派发子项，用 Chrome 按实际文档差异批准候选，在 GitHub 必需检查等待期间检查合并历史和提取队列，再重跑失败的检查 | 合并前两处均为空；合并后回执固定到实际候选与合并 SHA | 实现会话 `01a1e72d-ada6-76c1-bbb1-c09e6b6cafc7` 的候选 `e4850732e1dfe95e4d647f147a77f4c409bdfdaa` 通过重跑，两处等待态检查符合预期。PR #77 合并为 `b9ffcad353ca1e8d37fd530a015653183a77db0a`，任务事件 #9 的提取来源摘要为 `6104db75bef5f477a200f9781022c54f867591a6c087c070eba06411f9fb3eaf`。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/77) · [CI](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43998067460) · [父 Issue 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/76#issuecomment-6935338036) |
| 从任务时间线打开合并回执，依次运行原生 Curator 和独立 Management，再重启控制面 | 提取引句可在验收来源定位；只有审核通过的项目经验可见 | 会话 `01a1e730-58c4-7fe0-9beb-0c9f67cd61ab` 提取一条，审核会话 `01a1e730-97e1-76d3-ae6b-940b80f9ed44` 给出 D1 approve。重启后仍是同一条回执和项目经验 |
| 用 56 版控制面读取 beta PR #73，再升级至 57。使用节点身份、错误任务和错误项目读取，提交伪引句和重复回报 | 历史合并只补一条，已有交付不变；私有回执和来源引句校验仍然有效 | 历史任务和交付逐字段一致，来源摘要为 `f00e7ee0f57ddf7341e7d0fa9e3a4963f65d3c98099091185e2f505df371e86f`。节点读取为 401，错误任务、项目或非合并事件为 404。伪引句为 409，空提取回放为 204，经验列表为空，队列无重复。[历史 PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/73) |

启用 PostgreSQL 和 Docker 的全量 Go 回归 176 项（含子测试）通过，零跳过。Go vet、TypeScript 和 Web 生产构建通过。

确认 PR 合并触发提取与独立审核对应提交 `44a77fc8ad52d801cabcd7d686a181b0f59e4fec`。

### 专题 08、ADR-018 · Goal 交付完成触发知识提取

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 beta #78 激活含前置依赖的两项计划，从 Chrome 逐项批准真实候选。第一项合并后读取父 Issue、任务事件与项目历史，再运行第二项 | 第一项交付时父 Goal 仍开放；第二项从已确认的前置合并出发 | #79 合并为 `278516b02ec274c6d2baffa4f3a587e6eb8a3804`，父 Issue 为 OPEN，完成事件与历史均为空。第二个候选的基线精确匹配该 SHA，正文引用前置 PR 和完整 SHA。#80 合并为 `161fec0ee0aa43ee30725d225e1d28e11c950372`；两个 PR 的 acceptance-check、forgeos/local-ci 均成功。[前置 PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/79) · [依赖 PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/80) |
| 父 Issue 关闭后从 Chrome 展开完成事件，运行 Curator 与独立 Management。重启控制面，回到父任务 | 只生成一份固定完成回执；经独立审核后可查到范围限定于本次交付的观察 | 事件 #19 引用计划修订 1、两个子项的候选与合并 SHA，正文摘要 `e123538ca659a7f682e9cd71588a85ddb20e2450a33688fefbb9b519d7c5ae37`。会话 `01a1e73f-99f8-7591-bd12-087e41615ff7` 摘录“Candidate base matched the confirmed merge of prerequisite ledger.”；会话 `01a1e740-36da-76d0-b236-c1dcb3ed5082` 给出 D1 approve。重启后经验和审核逐字段一致。[父 Issue 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/78#issuecomment-6935552049) |
| 用 57 版读取已完成的 beta #76，再升级到 58。检查原 PR 的 D1 回执，提交伪引句、空结果和重复回报 | 升级只补缺少的 Goal 回执，保留已有项目经验；访问与引句继续校验 | 旧版 Goal 回执为 404。升级后事件 #11 回读当时计划修订与 #77 交付，摘要 `c1049ab1e6bf84a6f539527c9601ec23c2c498529736065556bc71f9d83d5159`，原 PR 回执、父任务、经验和历史不变。节点读取为 401，错误项目和任务为 404，伪引句为 409，重复空提取返回 204；随后无待领任务。[旧 Goal](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/76) |

首次验收时，队列中第一个 PR 的原生提取触及五分钟超时，浏览器等待也到期。保留同一数据库，从父事件继续了已完成提取的审核，并在租约到期后用正式 Worker 处理剩余 PR，最终无待提取记录。启用 PostgreSQL 和 Docker 的全量 Go 回归 176 项（含子测试）通过，零跳过。Go vet、TypeScript 和 Web 生产构建通过。

Goal 交付完成触发提取及依赖证据审核对应提交 `6237e703bfe04607bc86bf29ad7d78b43cb97044`。

### 专题 08、ADR-018 · Git 仓库文档变化触发提取

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 58 版控制面登记真实 Git 仓库，依次在两个提交从工作台 API 派发 HEAD 检查、由正式 Worker 运行并归档清单 | 复现文档已变更而提取队列为空 | 检查 #1 在 `7ea1a118c0f96679cab7e5ffc72f9e29762263fb`，#2 在 `4c27a28441ce860397a4cb422ac68725ac4a6246`；README 的 blob 从 `be3a3ad8bad9a846ce37bfd05cef42bf9b290247` 变为 `429092230ee714d5e6bcba54735e6f74ff1f25db`。两次领取均为 204 |
| 保留这两份回执，升级到 59，在两个独立克隆记录第一个提交，再切换至第二个提交。在第一项目覆盖工作区文件并添加未跟踪文件。关闭角色配置，等待后台观察；随后并发两次手动同步 | 同一变更在各自项目归档一次；只读取提交对象；旧清单逐字段相同 | schema `doc_verify_6b1b016e` 两项目各有 README 修改和 `docs/restore.md` 新增两条记录。待提取文档 Git blob 为 `091ba3b3487b696ca9a26c9c181aba1aea51e590`，正文 SHA-256 为 `79e7d363f7a14f95d2bea4f38bacbb2cd77a8772db77800cc089c6e201b0afc8`；两个并发响应与挂牌记录一致。节点读变更和 Owner 回执均为 401，跨项目回执为 404 |
| 启动第一项目的 Curator，在 Chrome 的“项目资料 → 仓库文档变更”展开新增文档，再运行独立 Management | 审核前无项目经验；审核通过后可定位来源，引句与提交原文一致 | 会话 `01a1e74e-3e14-7af1-b66f-82632ee9e22c` 摘录本次演练重启后恢复到偏移量 418；会话 `01a1e74e-ab7a-78a0-b897-3b8c0970d483` 给出 D1 approve，回执为 `/projects/1/document-changes/4/curation` |
| 在下一次提交清空 README、删除 restore.md，从 Chrome 同步；重启控制面并将此测试克隆临时移离登记路径 | 旧经验撤下；删除和空内容可区分；离线仍可核对历史审核 | 提交 `900bfaa13d34d3582083e78f02f7c458eaa08d3d` 新增两项状态，项目经验归零。离线时回执和历史逐字段一致，Chrome 展示原审核会话，两项变更均未重复派发 |

启用 PostgreSQL 和 Docker 的全量 Go 回归 177 项（含子测试）通过，零跳过。Go vet、TypeScript 和 Web 生产构建通过。回归另覆盖并发扫描、非文本文件、超过 256 KiB 的文件和排除 vendor 目录。此次验收范围为两次实际观测到的本地提交；中间提交回放和大文档正文提取仍待补齐。

已提交仓库文档变化触发提取、独立审核及旧经验撤下对应提交 `8b2dbf115cffd423570c8ed8a7d23ba80de279c2`。

### 专题 08、阶段四恢复 · 回放停机期间的文档提交

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在旧控制面以 `fd552603f9182e57841bed693acf102fbc8eb909` 建立基线，停机后提交恢复记录，再提交撤回，重启同步 | 可追溯中间版本及撤回 | 旧版在 `doc_replay_30dd5007` 将扫描位置推进到 `f350255cd7d339a91f64659ff37c452bd30e1b9f`，变更列表为空。两端 README 字节相同 |
| 在带同一旧版基线的数据库启动新控制面，通过项目工作台读取历史，两次并发同步 | 中间版与撤回分别保留对应 SHA、前后 blob，同一提交不会重入队列 | schema `replay_verify_4e7f38da` 自动得到两条。中间提交 `82fa3ae54f7cd15f0d1a361815ca43457001764f`、blob `a7583dfc759d6778ebc8514a5792602e9a0cce5f` 与 Git 一致，末项引用该中间提交作为上一版本；并发回报和重启保留原记录 |
| 从 Chrome 展开历史版本，依次运行原生 Curator 与 Management，随后查看当前经验 | 已撤回的观察仍可核对审核，在当前经验中不可见 | Curator `01a1e753-580b-76b0-9c9d-676271aa8b9a` 摘录演练 Q73 恢复到偏移量 563；审核 `01a1e753-b446-7ea2-bad1-a92c70e1165b` 给出 D1 approve。来源摘要 `815505bd490924a0c7c96361f994adce24f00d971d5cb397d471f46b4b02f973`。当前正文只含标题，提取为空，项目经验仍为空 |
| 从共同祖先切换到分叉提交。另建真实合并提交，再用无原 Git 对象的独立克隆替换验收路径 | 分叉比较两份已观察的树，侧分支草稿不会被记作主线版本；缺失旧对象后仍能比对和读回旧正文 | 分叉 `cee982dd222a37337f80923052bc8c5e5e58f886` 只新增一条。另一库的合并 `715c1c534925d5c17dae3bb63ba71cd10d49b412` 以第一父提交为基线，只归档合入正文。替代克隆的首提交 `69946669ff62bace95cd66da09aec637036e39c7` 新增一次差异，历史回执逐字段相同 |

回放起点是数据库里最后一次已保存的扫描位置。旧版本已经跳过且未保存起点的历史不作补录。

启用 PostgreSQL 和 Docker 的全量 Go 回归 178 项（含子测试）通过，零跳过。Go vet 通过。集成测试跨 34 个离线提交，在第一批 32 个提交落库后重新创建 Store 并续扫，最终只有新增和删除两条变更。

停机期间回放中间文档提交、保留撤回证据对应提交 `07aeae4f74342d003ef4de05915e9d287492a5fa`。

### 专题 08、ADR-018 · 大文档分段提取与固定来源恢复

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在版本 59 提交一份 288,093 字节的 README，末尾记载 Q74 演练恢复到偏移量 672。在同一库用版本 60 启动，逐段读取，与 `git cat-file blob` 的输出按字节比较；并发同步两次 | 原变更编号保留，每段可定位、可独立领取，拼接完整还原提交原文 | `doc_large_62e21b65` 旧回执 #1 为 size_limit，领取为 204。升级后变为五段，仍为 #1。提交 `f9e9889b364de24df50c4cdfbfdb3caeb9b6ff12`，blob `ac3f8e4c13ef8476ffe57ed0e99ae7f05281baea`，拼接 SHA-256 为 `51198eb4c7603dd703ed4aa41725b7542d63a729f56ada579d507ca5d12d811a`。并发响应相同。缺段号或越界为 404，零为 400，节点读取为 401，跨项目为 404 |
| 从 Chrome 选择末段，运行原生 Curator 和独立 Management，随后替换整份文档并重启；将验收克隆移离登记路径，再次打开历史末段 | 可从实际段落提取，审核后仅对当前版本生效。源克隆离线时仍可读归档原文和审核 | 末段从字节 262137 起，段 SHA-256 为 `68452081bfef02b0ed22bda9abf0a685cd71d3915ae193479a42fdaa1d45bbcc`。会话 `01a1e75b-00c8-7bb3-ab58-52e633abb818` 提取带 672 的原句，会话 `01a1e75b-44dc-77e0-8274-4216561f67b1` 独立给出 D1 approve。经验引用 `/projects/1/document-changes/1/curation?part=5`。替换提交 `3bbd7d141012f26ad1ccd437711b2a603a150a49` 后当前经验归零，离线重启后同一回执逐字段相同 |
| 保留一个待处理的旧 blob，用不含该对象的验收克隆提交另一份大文档和一份末尾含 NUL 的文件，随后只补回旧对象并同步 | 缺对象的一项保持待处理，其他项可完成。部分无效的正文不留下可领取段落；对象恢复后同编号重试 | `doc_large_fdb0d0ec` 的旧 #1 等待，新 #2 按实际换行及长行切成六段，末段含 appendix，另一项为 non_text。取回旧对象后 #1 出现五段，#2 回执不变，HEAD 仍为 `63790922b4350a6e82bb57dd7d9d1e65bf4d1190` |
| 在有双项目、删除事件和两个既有 D1 回执的 59 版库上升级 | 原单段回执路径与内容保持兼容 | `doc_verify_6b1b016e` 的两项目清单、回执 #4 和 #3、经验列表逐字段一致 |

启用 PostgreSQL 和 Docker 的最终全量 Go 回归 180 项（含子测试）通过，零跳过。Go vet、TypeScript 和 Web 生产构建通过。开发中一次回归与缺对象处理的修改重叠，编译读到未同步的错误常量；最终回归从同一工作树完整重跑。分段测试覆盖跨 64 KiB 的多字节字符、CRLF、长行、空白及后段无效内容。

大文档分段提取、原文重组、审核及缺失对象恢复对应提交 `a5309b4442a97304e5ab723e28211fddbe881628`。

### 专题 05、08、ADR-005 · Owner 发布全局默认规则

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 60 版登记两个 Git 项目，将一条仅适用于第一个项目的观察存入其私有资料，再启动 61 版 | 新规则空间初始为空；旧资料保留原权限 | `global_rules_dc3cd528` 的旧全局入口为 404，升级后读取到 null；私有资料逐字段一致。第二项目节点访问第一项目资料为 403 |
| 打开两个独立 Chrome 页面基于修订 0 编辑不同默认值。丢弃第一份 PUT 的成功响应并从同一草稿重试，再用另一页提交 | 重报只发布一个版本；冲突草稿可核对当前版本并继续编辑 | 修订 1 SHA-256 为 `bcaf0822f76ded8837e22b77111ba6eb5b0da220ce8a335efc233644ea858572`，重试后回执完全一致。第二页显示发布冲突并保留原文字，读取当前版后由 Owner 保留草稿并发布为修订 2，摘要 `8338086ccccb2eed199d1c62c49ced82b428c895d8422d44adbc89f5297e2062` |
| 两个具名项目节点分别读取、跨项目读取、写入和指定历史修订；重启后浏览器回看第 1 版、返回第 2 版，清空并发布 | 各自项目获得相同当前版本；只有 Owner 可发布和回读任意历史。撤下后两项目同步读取空规则 | 同项目读取为 200，跨项目为 403，节点写入为 401；项目读取指定历史版为 400。旧发布重试仍返回第 1 版，当前保持第 2 版。撤下后为修订 3，两个节点均读到空正文。历史 1、2 和原私有资料不变，数据库恰有三份修订 |

本项验收覆盖发布、撤下、恢复和授权读取。原生模型执行中的默认值与覆盖顺序另行实测。

本项完整 Go 回归启用 PostgreSQL 和 Docker，181 项通过，零跳过。Go vet、TypeScript 和 Web 生产构建通过。

发布、冲突处理、撤下及两项目授权读取对应提交 `766d452a1b3f0054af6c6b8081ae93aff789de01`。

### 专题 05、08 · 规划与实现执行使用固定的全局规则

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 61 版发布默认值，用旧节点领取真实 Issue，再升级已有数据库，重报旧执行结果并重试 | 原回执保持一致。非空规则应阻止旧节点接单，撤下后恢复派发 | `rules_upgrade_f9b77462` 的执行 1 升级前后任务 JSON 一致。旧节点领取为 204，支持规则的节点领取执行 2 并取得修订 1。撤下后执行 2 仍引用修订 1，旧节点领取执行 3 且无规则字段 |
| 给规划器指定仅见于全局规则的里程碑名称，导入真实 Issue #81，核对只读 Codex 计划 | 原生计划使用已发布的名称，回执指向同一版本 | 会话 `01a1e76b-4aec-7243-b97f-7675a1cf6c4a` 的首个里程碑为 `Milestone 8bb0a8d811`，规则修订 1 的 SHA-256 为 `44276ef14e62c536d4c4fabc9d86998dc3224fe2d76cf1ca546a681e1b03043c` |
| 用 Owner 修订的计划给三份手册分配标题规则。在两份文档上引用项目决定，验收退回时再单独指定第三份标题 | 首次候选按全局、项目、项目取值；退回后按全局、项目、Owner 取值。其他文件不变，撤下规则并重启后两次回执仍可回看 | 项目配置选择原生 Claude。首个候选 `ffae17de60b464e38847e276962ff5d257e49a79`，退回后候选 `41368592f832837fb27221b23a9ac508cf29eefc`。逐文件核对固定 Git 内容，在 Chrome 回读撤下的正文并批准候选。两项必需检查通过，合并为 `83bd17558e73c1985df037ed94d763c32f936043`，父 Issue 已关闭。[PR #82](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/82) |
| 以原生 Codex 实现另一项真实 Issue，重复三级覆盖并检查节点能力及候选历史 | 三个标题逐项匹配，第二回合只改 Owner 指定的文件，工作台能核对规则来源 | 首个候选 `b44efdbbad943cb43676bc1ab52d881da75486dc`，退回后为 `b7314b01c46e13ae016b713bc01281bef5473cf5`。Chrome 节点页显示全局默认规则能力，任务页读到已撤下的正文。必需的 `forgeos/local-ci` 与 `acceptance-check` 均通过，合并为 `a7c1322275ad115b045b03a040fe20e4e768df2c`。[PR #84](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/84) |

两次验收均使用 beta 仓库的必需检查配置，刻意失败的首轮检查经重跑通过。首次规划的激活响应中断，重启后从 API 读到已生效的修订 2。首次启动的节点仅启用 Codex，子项因项目声明选择 Claude 而保持 READY；启用本机 Claude 后从原任务继续。启用 PostgreSQL 和 Docker 的全量 Go 回归 182 项通过，零跳过。Go vet、TypeScript 和 Web 生产构建通过。

规划与实现的固定规则、旧节点兼容和真实覆盖顺序对应提交 `3744ecc3498e25d8214558f13fedf0a7fc65e7d0`。

### 专题 05、08、10 · 独立 Reviewer 使用实现候选的规则依据

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从真实 beta #83 固定需求派生独立验收样本，原生 Codex 先生成三份文档，按 Owner 意见返工。发布新全局版并使项目决定失效，然后申请审查 | Reviewer 应能核对实现时用过的原始规则和返工意见 | 在 62 版复现缺口。开发执行 2 有规则和决策回执，实际 Reviewer 领取的需求没有三个标题中的任意一个。该次审查以可重试失败结束 |
| 升级相同数据库，重试已知候选。让旧 Reviewer、普通 Worker 和支持候选依据的独立 Reviewer 先后领取 | 历史失败回执保持原样。仅获准且理解候选依据的 Reviewer 可以领取，重试得到同一份数据 | `rules_review_context_one` 的旧 Reviewer 收到 204，普通 Worker 收到 401。兼容角色取得实现时的修订 1 和项目决定，Owner 意见准确关联退回执行 1 和当时的 SHA。已发布的修订 2 没有混入 |
| 启动原生 Claude Reviewer，工作台回看报告，启用必需审查后批准，再重启 | 合规候选通过独立审查，原始依据可回读 | 候选 `89c8971e6b8622717c6ea1b4b07f29b07fdd750c` 的实现会话为 `01a1e779-0b9f-76b2-ba5f-394c482e60b0`，审查会话为 `0f8ea58d-f205-4245-a4f6-993f091fbbe0`，未报告缺陷。Chrome 在报告内回读原规则、已失效决定及 Owner 意见。控制面重启后报告逐字段一致，任务保持 ACCEPTED |
| 在独立的 `worker review-candidate` 调用中，用同一真实 Git 候选，只替换输入样本的 Owner 标题要求 | 能据此发现第三份文档的标题冲突，引用准确位置 | 新会话 `064aaa2d-af24-4a8f-9eac-422cf128f2d8` 报告 High 阻塞发现，引用 `docs/rules-codex02-owner.md` 第 1 行 `# Review Owner 4c156e23`，控制逻辑核实证据与 Git 一致 |

独立样本在本地审查门禁结束，未另行派发 GitHub 交付；原始 Issue 的实际交付见 PR #84。本项覆盖有原生实现会话的候选。换基后仅运行 CI 的候选、其他角色上下文保持待验收。

启用 PostgreSQL 和 Docker 的全量 Go 回归 183 项通过，零跳过；Go vet、TypeScript 和 Web 生产构建通过。

独立 Reviewer 的候选规则、退回意见和工作台回执对应提交 `e566c170ecdb08caae37799dec46fa450f079f59`。

### 专题 08、10、12 · 换基重验后追溯原实现依据

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 导入 beta #85 并用原生 Codex 生成手册，发布规则修订 2，再把私有验收分支向前推进一个空提交，批准原候选并从交付入口请求重新验证 | 应关闭旧 PR，在新基础运行 CI。独立审查仍需使用实现回合采用的修订 1 | 原实现执行 1、会话 `01a1e780-85d1-70a2-bed1-08793171eeb5`、提交 `56af4bc27cdcc9cf4cbaad58c82a059d4e2ea8c3`。[PR #86](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/86) 已关闭且未合并。重验执行 2 的基础为 `c09f4b72e64e5e343cc43883cc90e466fec0a3fe`、候选为 `4db0d343719dc3ef65c29a4fb4fea5e6baaceb91`。63 版实测 CI 成功，Reviewer 领取缺少原规则 |
| 升级至 64 版，重试同一候选，先让旧 Reviewer 领取，再用兼容角色领取。启动独立原生审查，在 Chrome 核对来源并批准 | 旧角色等待；审查对照新候选及原实现规则，工作台展示两个提交的用途 | 旧角色返回 204。兼容角色得到原执行 1、原提交和修订 1 的摘要 `a501679eb474be62a742100d8b7146ad0c0a3759cba011c12cc54bcd32c865f5`。Claude 审查会话 `8ced1241-3502-4ff3-9bd4-0dcf90ee4f9c` 核对了重验后的候选并报告零发现。工作台读到了原执行、提交和正文 |
| 批准重验候选，核对 GitHub 检查、合并提交及父 Issue，再重启回看审查回执 | 同一候选通过独立门禁后完成交付，旧 PR 保持关闭 | `forgeos/local-ci`、`acceptance-check` 均通过，合并为 `29a6d6459253ffde9812deb0aa60e9fb6cc9564a`，Issue #85 已关闭。重启后审查报告和依据逐字段一致。[PR #87](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/87) |

实测使用验收仓库的独立分支 `codex/accept-review-refresh-25f6a1`。数据库回归另验证连续两次换基，最终报告仍指向原生实现，执行 CI 的节点没有领取模型规则。首次核对重启回执时，验收脚本的路径变量被 GitHub 检查详情覆盖；修正后使用同一已交付任务重跑，回执一致。

重验交付完成后，首次全量回归有 182 项通过、两项真实容器检查因 Docker Desktop 接口超时而失败。经 Owner 同意重启 Docker Desktop，核对先前运行的四个容器均已恢复。验收镜像摘要为 `sha256:c05ba4b73604069d376c4f41346b05374335b5ca0c46fb6dfede5a59f5196931`。原先失败的两项容器检查单独重跑均通过。

恢复后重新执行全量 Go 回归，PostgreSQL 和真实 Docker 均启用，184 项通过，零跳过。Go vet、TypeScript 和 Web 生产构建通过。

换基候选的规则追溯、独立审查和工作台回执对应提交 `feba4f4d699599f4cb91472facfbcdb1bd7bbdc2`。

### 专题 05、08、11 · 测试计划和覆盖审查使用同一候选规则

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从真实 beta #85 派发独立样本，生成候选后把全局规则改为修订 2，申请测试计划 | 角色应获得生成候选时采用的规则 | 在原版本复现缺口：执行 2 有修订 1 回执，Test Agent 领取的数据只有 basis、diff、diff_truncated 和 library，缺少规则及来源。原计划修订 1 留有失败回执 |
| 用同一数据库启动新版本，从任务入口请求修订 2，重复请求；让旧 Test Agent 和旧 Management 分别在对应阶段领取 | 重报返回同一版；旧角色等候，两个兼容角色拿到相同来源。旧计划可原样回看 | 两次创建响应相同。两类旧角色均返回 204。新修订的依据是候选 `e699b87538bb2765e843f92842c4b07b6aae627e` 所用的修订 1。独立重算整份依据的 SHA-256 为 `b4a1a8bc86ca4a50f98aefcfd537f8d522f1281587bfec5f77f3db7c02da8f01`。工作台可展开原文，在两个历史版本之间切换 |
| 启动正式 Test Agent 和 Management，根据计划在候选 Git 对象核对实际标题，记录用例结果并批准。重启后回读 | 生成和审查为不同会话；比较步骤使用规则中的准确标题，实测结果绑定本次候选与计划 | Test Agent 会话 `01a1e798-616e-7b12-8a39-b51835d43944` 生成 TC-001，Management 会话 `01a1e798-c07d-7f91-9062-129996a93590` 判定 covered。Git 标题与修订 1 一致，通过正式结果入口记为 passed。计划摘要 `6140c44d1bfdede7a426c570faec9f37ed9f2a7d263fc98551c88f19941efd89`。重启后两个计划修订及 ACCEPTED 状态均一致 |

样本在本地验收门禁结束，原始 Issue 的交付见 PR #87。第一次独立重算摘要时，验收脚本使用了 PostgreSQL Go 驱动专用的连接参数；改用 psycopg 的连接选项后，对同一已生成的计划修订完成核对。

本轮全量 Go 回归启用 PostgreSQL 和 Docker，186 项通过，零跳过。Go vet、TypeScript 和 Web 生产构建均通过。

候选规则进入测试计划与覆盖审核对应提交 `da40dd469296d019380fcf59a769d01025ff666a`。

### 专题 05、10、11 · 换基重验后的独立角色会话

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 读取 beta #85 的已交付执行，对照开发会话、重验执行和审查报告 | 换基须可追溯开发会话 | 原执行有原生会话，重验执行的 session_id 为空。旧版入库检查只读取重验执行。本次迁移让三个角色均追溯真实交付刷新记录，照常回读历史结果 |
| 在 beta #88 完成原生开发，推进独立目标分支，更新候选并重新跑 CI | 原 PR 关闭，新执行使用新基线，不再发起模型实现 | 原实现 `36fbbc71106133744e9855ea6cf0d7b45c521422`，开发会话 `01a1e7a1-9ce5-7d70-8451-9f9f5c26de41`。PR #89 关闭。基线 `6d7c03a2f1d9eff26ecd112710a8eac96c381543`，新候选 `c29858e39b76ba7f0e606694fa98a5c4d33c776c`，两候选文件树相同 |
| 对新候选启动三个独立角色，核对数据库、工作台、Git、远端交付并重启 | 三个角色均与开发会话独立，验收与合并对应新候选 | Reviewer `ee52327c-a5a7-4ead-a006-eb541780557e`、Test Agent `01a1e7a4-6adf-7211-8fff-1d14f27670a7`、Management `01a1e7a4-b61a-7e22-84ba-5fdec847c8e6`。Chrome 的两份回执都能展开原实现。Git 已实查新候选的单一 H1、变更文件和相同树。两项必需 CI 成功后合并为 `0df4e6b9c0883af1494709b86686557121bcef79`，Issue 已关闭；重启后回执相同。[Issue #88](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/88) · [PR #90](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/90) |

本次验收通过的是角色会话独立性。TC-001 第一步把原实现 SHA 写成待测候选，Management 仍给出 covered。实际核对使用了换基后的 Git 对象。用例文字和覆盖审核识别错误 SHA 的能力另列待修复；保留本次原始回执。首次克隆和首次创建 Issue 遇到 TLS 中断，核对远端记录后重试，只创建一个 Issue。测试脚本为首次审查补办正式请求，并给只读 Reviewer 配置登记仓库根目录后继续。

真实 PostgreSQL 与 Docker 全量回归 187 项通过，零跳过。用例覆盖无规则候选、连续两次换基、开发会话被拒、独立会话成功和相同回执重报。Go vet 通过。

换基后会话独立性检查对应提交 `85d9c5cc2b258daed6195e11a7c54b4be3e3f1e9`。

### 专题 11 · 核对用例指向的 Git 候选

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 读取 beta #88 原样归档的测试输入及计划，经隔离的本机 HTTP 回放入口交给正式 Management Worker 和原生模型 | 旧计划摘要相同；应指出 TC-001 错把原实现当作验收对象 | 摘要仍为 `19708c8be657f77271f362de21e8429b2d9b2777a99c4d29a7d9b24df7c76ac4`。会话 `01a1e7ab-c717-73e2-b181-6baa6ceb285a` 返回 changes_requested，准确指出前置条件和第一步把 `36fbbc71` 当作新候选 `c29858e3`。历史数据库只读 |
| 在 beta #91 推进目标分支，关闭原 PR #92，在同一新候选上请求计划和审核 | 使用新基线和新候选，覆盖审核后等待实际操作 | 修订 1 的目标正确，Management 因自己没有执行 Git 检查而要求澄清。修正覆盖审核的职责描述后申请修订 2。Test Agent 会话 `01a1e7ac-23af-7611-8aa6-5efe62766cae` 将前置条件和三步操作指向新候选 `3d369395c8c78c29395526d442c2d92372f01b5d`，比较基线 `57883b676d8d501f5073ce6bb2cd1246cef6dc95`。Management 会话 `01a1e7ac-6b01-7ef3-a3a8-8118edeb5f77` 判定 covered。两版回执均留存 |
| 按 TC-001 读取候选 blob，核对 H1、name-status 和完整 diff，通过项目节点入口记录并由 Owner 在 Chrome 批准 | 只有目标文档新增，准确标题来自原实现使用的规则修订 1；证据绑定当前候选 | `git show` 返回唯一一行 `# Frozen 6b16cbad`，`git diff --name-status` 只有 `A docs/review-refresh.md`。行号和完整 diff 已归档，计划摘要 `6f4bfc31c7c8c23acc6101e42798ff0f91d55e693aedf917591328bdf12f9237`，执行状态 passed |

PR #93 的 acceptance-check 与 forgeos/local-ci 均通过，合并 SHA `6b4040f9adf5d806ece6b8d982059087893179db`；Issue #91 已关闭。重启后计划和审查回执与重启前相同。最终版本使用真实 PostgreSQL 和 Docker 跑完全量 Go 回归，187 项通过、零跳过，Go vet 通过。[Issue #91](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/91) · [PR #93](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/93)

测试计划和覆盖审查区分原实现与待测候选，对应提交 `36ebbfd0f4b17f0c4e5e33fd2de386a5995e489f`。

### 专题 06、10、11 · 暂停后恢复 CI 的候选来源

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 beta #55 原验收库运行升级前控制面，对照已暂停的执行 1 和恢复后的执行 3 | 应能从相同候选找到实际产生代码的会话 | 两次均使用 `aae51b5ff79b882a489d52e9869bd11aa3e47c11`，原执行有模型会话，恢复执行只有 CI。旧查询对执行 3 返回 NULL |
| 升级同一库，从任务入口为执行 3 请求独立审查，在 Chrome 查看回执并重启 | 通过正式恢复记录找到原开发会话；已有两份运行结果原样保留 | 新查询返回执行 1、原生开发会话 `01a1e604-db7d-7882-9d40-5cccf796c57d` 和原候选。独立 Reviewer 会话 `9a961ecc-516a-491e-819f-2d358b537eff` 在同一候选完成审查。Chrome 可见该会话；重启前后审查回执、两个原执行结果一致。[历史 Issue #55](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/55) |
| 在 PostgreSQL 分别演练 Worker 回报中断、登记候选后失联并由 Owner 结算，再发布规则修订并恢复 CI | 冻结最初实现使用的会话与规则，即使缺少最终模型结果也可从登记回执追溯 | 相同回执重复成功，更改会话被拒。两条路径均找回同一原执行、原 SHA、原规则。该失联场景通过数据库集成测试覆盖；原生进程被强制中止仍按开放项管理 |

首轮全量回归因专用验收 PostgreSQL 写 WAL 时磁盘耗尽而中断，25 项报告连接失败。原验收库的升级、原生审查、Chrome 回读及控制面重启已在中断前完成。将回归切换到 D 盘新建的独立 PostgreSQL 18 实例，启用真实 Docker 从头运行，189 项通过，零跳过；Go vet 通过。原始验收库的物理恢复另行核对。

CI 恢复后的候选会话与规则追溯对应提交 `80ef8450e93df5ffee7980f59a44f5ddc01aa575`。

<a id="check-c-5"></a>

### 专题 05、06、08 · 跨项目审批中心

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在两个项目登记待确认决策、Goal 草案和待审核的脱敏副本，打开 Owner 接口和 Chrome 审批中心 | 跨项目显示可处理的操作，共享须经 Management 审核，列表只展示处理需要的字段 | 从真实 beta Issue #2 起草计划。共享审核前只见两类记录；原生 Management 会话 `01a1e7d0-f80b-7ee0-bea9-1cf22f243e34` 批准副本后页面显示三类。原资料的私有标识不在列表中。Worker 查询返回 401，错误筛选与游标返回 400 |
| 筛选计划，从固定链接打开修订 1，再由另一请求发布修订 2 | 老链接显示历史版本，不能批准过时修订；最新草案仍可处理 | Chrome 的修订 1 没有批准按钮，直接提交旧修订返回 409。从中心打开修订 2 并批准，Goal 的 active_revision 为 2 |
| 从中心打开项目 2 的决策并确认，打开项目 1 发给项目 2 的共享副本，检查摘要后发布；重启控制面 | 已处理或已失效的记录不再出现，原详情保存结果 | 决策 1 为 CONFIRMED。用其他摘要发布返回 409。按 `454147c7c97a3bfc99815de59d03cf922a94a66dc23db646d5093f07b779f201` 发布成功，接收方回读相同摘要。重启前后的审批列表均为空，处理结果一致 |
| 在 PostgreSQL 创建 32 个待确认提案和 1 个到期提案，在两页之间处理第 30 条 | 到期项不进入队列，翻页不随旧行处理而跳过其他记录 | 首页 30 项，游标 `decision.3`。处理决策 3 后按原游标仍读到 2 和 1，筛选共享为空。另从 HTTP 新建 31 项，Chrome 翻到末页后由另一请求处理前页末项，刷新后末页仍保留原记录；返回首页正好 30 项 |

验收只激活了隔离库的 Goal。[来源 Issue #2](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/2)

全量回归首次与历史库物理恢复并行时，GitHub 导入测试出现一次配置读取失败，Worker 测试包超过默认 10 分钟期限。单独重跑导入测试通过。在 D 盘独立 PostgreSQL 18 上按包顺序从头执行 `go test -p 1 -timeout 30m -count=1 -json ./...`，启用真实 Docker，190 项通过、零跳过。Go vet、TypeScript 与 Web 生产构建通过。

跨项目审批中心对应 ForgeOS 提交 `ec9ec1b32da0fb0352855efef19fef2844aa980b`。

### 实现回合提出业务问题并恢复

在两个独立的 PostgreSQL 18 schema 中，从 beta 仓库导入要求 Owner 选定手册标题的真实 Issue。分别启动原生 Codex 和 Claude Code 的正式 Worker，标题均在提问后由验收用的 Owner 身份给出。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 启动 Worker，读取任务、检查点和执行资源，再重启控制面 | 第一回合结束后进入 WAITING_INPUT，问题关联原会话和基础提交，可以腾出模型、CI 名额 | 两项执行均在检查点 0 提问。执行的资源类型为空，CI 候选与结果均为空；重启后同一问题仍在待答队列中。 |
| 在 Chrome 的“待答问题”填入精确标题并提交，查看工作区、Git 差异和本地验证 | 第二回合复用 Session 和 Worktree，一次执行生成一份仅包含所选标题的文件 | Codex 的 PID 3244、13124 共用 `01a1e7f1-c820-7de0-863e-e3529e3c9005`，候选 `17a4befff15d0f162ede927e43dea7e882618708` 的 H1 为 `Harbor 4788ed62`。Claude Code 的 PID 12176、24688 共用 `59cecac8-b5f0-44b8-ad11-6f42b8bb5023`，候选 `9ef05d9a987ea7a0ecad214fad8f1dbc1f05a25e` 的 H1 为 `Harbor e48c5217`。两者均只改动指定的文件，本地验证退出码为 0。 |
| 以 Owner 批准已核对的候选，重跑验收仓库刻意失败的 Actions 检查，查询 GitHub 与控制面 | 必需检查通过后才合并并关闭 Issue | 两个 PR 的 head 与相应候选相同。`acceptance-check` 与 `forgeos/local-ci` 均成功。Codex 合并为 `6d5e687e16312331d3bbc7984b6e8557bc58caea`，Claude Code 合并为 `e9f7f7b3e2e32744a621fcbd7049fa6568360e61`。两项任务 DONE、Issue CLOSED。[Codex PR #95](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/95) · [Claude PR #97](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/97) |
| 使用 PostgreSQL 重报提问和答复，尝试错误租约、改写问题、跳过恢复回合、切换原生会话或并行派发同项目任务 | 幂等回执保留一次决定，所有越过检查点的操作被拒绝 | 同一问题和答复可重报。Owner 取消、关闭源 Issue、租约过期并核对结束三条路径均保留历史问题，结算后下一项才能领取。 |

交付完成后分别重启控制面，在 Chrome 任务详情核对提问节点、提问和答复时间、基础提交、检查点及 Owner 原文。两项的待答队列均已清空。检查点 0、1 绑定同一 Session，检查点 2 没有待处理输入。两个执行均使用节点配额，三次资源申请依次为首轮实现、答复后实现和 CI，最终全部释放。

在独立 PostgreSQL 18 上启用真实 Docker，执行 `go test -p 1 -timeout 30m -count=1 -json ./...`，196 个测试及子测试通过，零跳过。Go vet 和 Web 生产构建通过。

原生实现回合提问和恢复对应 ForgeOS 提交 `eed7fde5ed1073822a58f0533a2f5270738ddfbc`。

### 阶段三 · 有依赖的两个子项、运行中答复、退回重做和父 Issue 关闭

从私有 beta 的 Issue #98 建立 D2 Goal，显式激活包含 index 和 reference 的计划。在同一项目的正式 Worker、真实 PostgreSQL、Docker、Chrome 和 GitHub 上完成以下操作。验收使用 Owner 测试身份。[父 Issue 与交付回写](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/98)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 激活计划，派发 index，按 SHA 批准第一份手册并读取父 Issue | 首项 READY，后项 BLOCKED。首个 PR 合并后父 Issue 保持开放 | index 只增加指定的单行 H1，候选 `b9eceb666819a91f8efe9915e8beef9b3cc71129`，合并为 `005e1861f46e375e1e3f48c84beba9e932767300`。GitHub 返回父 Issue OPEN。[首个子 PR #99](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/99) |
| 派发 reference，等待模型提问，重启控制面，在 Chrome 填入业务标题 | 使用前项合并版本作为基础。原会话继续完成候选 | 检查点 0 的基础 SHA 等于首项合并 SHA。执行 2 的 Codex 会话 `01a1e802-5b32-7573-a9fb-ea573b7f853d` 生成候选 `1c0ad3812afbc75ec44e1df252e513ae79492376`。Git 文件的 H1 为 Owner 指定的 `Field 1bb662cc`，包含到已交付索引的相对链接。本地验证通过。 |
| 在 Chrome 退回候选，要求保留标题和链接、补一句离线说明；重启并用新版本领取 | 旧 Worker 不能领取需要历史答复的返工。新执行保留答复来源和本次退回意见，以新会话修订 | 旧能力领取返回 204。执行 3 使用新会话 `01a1e805-066d-7683-82fb-b8e7ab2eb919`，只改动 reference 文件，保留批准的 H1 和链接，补入 `The guide is available offline.`。候选 `3ac077607ff55490526b48411aa2a8246e36ce68` 重新通过本地验证，业务问题总数仍为 1。Chrome 可查到沿用的原执行 2 和答复。 |
| 由独立 Reviewer 审查返工候选，再从工作台批准并检查收尾 | Reviewer 看到与实现执行相同的答复和退回意见。两个子项完成后父 Goal 才可关闭 | Claude Code 审查会话 `84fdca33-9fff-454e-b6a5-da995a15b986` 对同一候选返回零项发现。两个 PR 的 `acceptance-check` 与 `forgeos/local-ci` 均成功，第二项合并为 `323fd788d959674abbc3692900e47d7c06a542eb`，父 Goal 为 DONE，GitHub 父 Issue 为 CLOSED。[第二个子 PR #100](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/100) · [两个子项的关闭回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/98#issuecomment-6939813159) |

使用 PostgreSQL 18 与真实 Docker 运行全量 Go 回归，197 个测试及子测试通过，零跳过。另以 PostgreSQL 测试连续两次退回、验收时的答复、旧 Worker 能力及不同需求版本的隔离。Go vet 和 Web 生产构建通过。

双子项 Goal 的返工答复继承对应 ForgeOS 提交 `674a466b10c1f18bdeb8fc455ba39149623aa4a6`。

### 专题 14 · 验收数据库磁盘耗尽和回放期间主机重启

2026-11-17，本机历史验收库因数据卷写满而停止。确认源实例离线后，将 1,118,124 个文件复制到 D 盘保留原身份与配置；按同版 PostgreSQL 18 空库布局补回缺失的空目录。源库与恢复库各自保留 WAL，源库的 1,679 个 WAL 文件已逐文件按 SHA-256 核对。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从 D 盘副本启动实例。在首次回放期间 Windows 非正常重启后，再从相同副本启动 | PostgreSQL 自行从最后的检查点恢复，原证据可回读 | 13:49:26 到达 WAL `6/D4FFFF90`。13:51:04 完成恢复检查点，写入 13,924 个缓冲页，数据库可连接。随后显式执行 CHECKPOINT 成功。原验收路径现指向 D 盘活动实例，原离线副本另存。 |
| 在数据库对照归档回执，再启动控制面，查询原任务和 `code-review` | 状态、候选、原生审查会话与归档一致，已交付任务仍有原合并 SHA | #55 的执行 3 仍为 WAITING_ACCEPTANCE，候选 `aae51b5ff79b882a489d52e9869bd11aa3e47c11`，审查会话 `9a961ecc-516a-491e-819f-2d358b537eff`。#88 和 #91 的执行 2 均为 DONE，候选分别为 `c29858e39b76ba7f0e606694fa98a5c4d33c776c` 和 `3d369395c8c78c29395526d442c2d92372f01b5d`，审查会话、Management 计划摘要及合并 SHA 均与对应回执一致。 |

本次逐项核对覆盖上述三份归档。其他历史 schema 未逐表核对。

历史验收库 WAL 恢复与三份控制面回执核对对应 ForgeOS 提交 `134d507d18540d358e9a7af58a4bf93c7239b1da`。

### 仅验证候选退回后的原实现会话检查

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 PostgreSQL 创建候选，两次换基并复验，退回第二次换基候选，用相同原生会话提交新 SHA | 登记 CI 和成功回报均应拒绝复用被退回候选的实现会话 | 改动前，两处都接受了原 Session。改动后两处都返回冲突。新 Session 登记并完成同一份新候选成功。 |
| 将恢复后的 #55 schema 导出为独立快照，在另一个 PostgreSQL 18 实例还原，从 Chrome 退回只运行 CI 的执行 3 | 返工依据对应到产生候选的执行 1。新 Worktree 以被退回候选为起点，在新原生会话中修改，重新验证 | 快照 SHA-256 为 `4792c999c570aa885deab86e03c9f1025631bc3e2efef9f7028521a1092c33fb`。原 Session 为 `01a1e604-db7d-7882-9d40-5cccf796c57d`。执行 36 使用 `01a1e814-066d-7db2-93e7-ae8e3fcf0132`，只改动 `docs/queue-pause.md` 并补上指定句子；候选 `eefc6aba1827e5cee686218ebb677df888ecbcd6` 在真实 Docker 运行 `ci-run-2538649211` 中通过。 |
| 重启隔离控制面，再读任务、验收记录和候选身份 | 维护者可追溯旧候选、原会话和新验证。等待 Owner 验收 | 原候选 SHA 与原 Session 均命中退回记录，新候选与新 Session 通过检查。重启后状态为 WAITING_ACCEPTANCE；本次在还原副本上验证到此，未触发远端交付。 |

真实 PostgreSQL 18 与 Docker 全量回归共 197 个测试及子测试通过，零跳过。Go vet 通过。

上述仅验证候选退回后的会话门禁对应 ForgeOS 提交 `d10793fab91e7c4df74c793b5f3cac940c242c3c`。

### 专题 07 · 等待业务答复期间的工作区变化

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 导入 beta #101，让原生 Codex 就手册标题向 Owner 提问。在 Chrome 打开任务详情，核对暂存区与工作区的树 SHA。往该执行的工作区添加一个未跟踪的验收文件，再从问题队列答复 | Worker 在启动下一回合前识别变化，保留工作区供核对 | 执行 1 的两棵树均为 `c68e2773d836b722a40cd7a0f7205a0e5644c33e`。答复后状态为 FAILED，错误指出检查点 0 的工作区或暂存区发生变化。原文件仍在。仅一个已退出进程（PID 29988），无候选、CI 或交付记录。 |
| 从原任务重试，在新工作区收到业务问题，答复后继续执行并验收 | 沿用第二次执行自己的 Session，候选只包含指定标题，检查通过后合并并关闭来源 Issue | 执行 2 的进程 2948 和 35260 共用原生 Session `01a1e83f-147d-78b3-99d2-ab475b228a09`。候选 `e6f4426715cbbe934cc007da083a3a5b06b737e4` 只增加 `docs/owner-tree-checkpoint.md`，标题为 `Harbor b1a2b1a3`。Docker 验证通过，远端必需的 Actions 检查重跑通过，`forgeos/local-ci` 成功。PR 合并 SHA 为 `57b1405ca3807227a8e518f69554dcaf1c55d834`，Issue 已关闭。 |

[Issue #101](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/101) · [PR #102](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/102) · [Actions 两次运行](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44033567538)

本次 PostgreSQL 18 和 Docker 全量回归 198 个测试及子测试通过，零跳过。Go vet 和 Web 构建通过。Git 回归核对了暂存、删除、忽略、未跟踪、相同大小和修改时间的文件，计算前后真实 index 字节一致。此场景中 Worker 持续运行；节点重启后的会话恢复继续待验收。

检查点工作区变化检测对应 ForgeOS 提交 `870e1eab47685731f030ea13ec82b2c29c516234`。

### 专题 07、14 · 原 Worker 退出后接回等待答复的执行

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 beta #103 确认 Codex 提问、原生进程已有退出回执；停止该验收 Daemon，重启控制面。用相同身份、状态目录与工作区启动两个替代 Daemon，从 Chrome 回答 | 一个 Daemon 取得答复，接回原 Session，另一个退出本次恢复。后续资源申请从原序号继续 | 初测执行 1 的模型资源申请序号归零，控制面返回 409。修复后在同一工作项重试。执行 2 的原 Daemon PID 38328；PID 23252、31020 均读到检查点 0，只有一个取得答复。原生进程 29452、38032 共用 Session `01a1e853-3a70-78d1-bb5d-c902e6080b5d`。保存的活动预算剩余 1177.5 秒。 |
| 核对执行 2 的工作区、候选差异、Docker 验证和远端交付 | 同一执行交付指定标题。核对 PR、检查与关闭回执 | 仅增加 `docs/daemon-checkpoint-codex.md`，标题为 `Harbor b635abef`。候选 `751d15707345479a52d6c8695f8ec9802630060d` 的两项必需检查通过。合并为 `5462d1cd516f76eb1cedb9be5703962a2f47822b`，Issue 已关闭。[PR #104](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/104) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44036279084) |
| 在 beta #105 以 Claude Code 重复上述中断、两个替代进程并发恢复、工作台答复及交付 | 保留原执行、工作区、Session 和活动预算，只有一次恢复后的模型回合 | 执行 1 的原 Daemon PID 16056，替代进程 13644、2944。原生进程 26328、2592 共用 Session `3d5df7ef-8a39-428a-a5a8-001c5de399a9`，预算剩余 1190.1 秒。候选 `e1f6e792bce4af7af36339fb073e0763ea00a197` 仅增加指定文件并采用标题 `Harbor 4fed3e00`，两项检查通过，合并为 `3caa8099362d4c548cdd7dae03929d0e6a994762`，Issue 已关闭。[PR #106](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/106) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44038777485) |

两次成功运行均从本地 SQLite 核对了一次领取、已退出的模型进程、恢复完成后清除的待恢复正文。控制面两次返回 DONE。真实 PostgreSQL 18 和 Docker 全量回归 203 个测试及子测试通过，零跳过，Go vet 通过。运行中的原生进程被强制终止、整机重启后的模型恢复及跨 Session 交接仍待验收。

等待业务答复时 Daemon 退出后的恢复对应 ForgeOS 提交 `96641a09961d32e5233b42849e975d8c41a0c782`。

### 专题 05、16 B.3 · ACP 消息格式与检查点决策回读

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 用独立的 Draft 2020-12 校验器及日期时间检查校验文档中的决策请求，再由 Go 校验器验证五类消息以及错误版本、类型、日期、参与方、信任字段和幂等键 | 合法示例通过，错误输入被拒绝 | 两种校验器均通过文档示例。五种消息类型及拒绝用例的自动化回归通过。 |
| 从正式控制面的 Owner 消息接口读取 beta #101、#103、#105 的历史问题，将输出对照原任务和独立校验器 | 每个已答问题有独立请求与答复，作用域、发送人、答复、Git 依据与原始记录相符 | 三个来源分别读到 4、4、2 条消息。全部通过独立校验。前两条 Issue 各有两次实际执行，问题按执行区分。 |
| 重启三个隔离控制面，重读相同问题；使用 Worker 身份或将问题编号与错误任务组合查询 | 重启前后消息内容相同，未获授权或不存在的组合被拒绝 | 三次重启前后，按键排序的 JSON 消息摘要均相同。#101 为 `a47921de544b5adad43e4509cc1c0c39e2fa67ac9f53f02aa34b5fcdb5b00158`，#103 为 `c2bb819e4822c5a3fd04da12af326de56bc1bd557927bb8bc57e8bcef0373b95`，#105 为 `e5767b329d7d8613837c96c22ebf0ad879a6998a3df091879b1f0f06fc171c2e`。错误身份返回 401，错误任务返回 404。 |

本轮启用真实 PostgreSQL 与 Docker，全量回归 204 个测试及子测试通过，零跳过。UTC 输出定稿后，重新构建控制面复验了上述 10 条消息、数据库专项测试及 Go vet。其他操作的消息路由继续待验收。

ACP 1.0 格式及原生决策记录回读对应 ForgeOS 提交 `ca6aa459eba90d665ff92825c0a4f42633f7cf05`。

### 专题 05、07 · 原生 Worker 在线接收 ACP 决策答复

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 导入 beta #107，要求使用 Owner 指定的标题，并在下一行记录答复附带的请求和响应 ID。在原生 Codex 提问后重启控制面，从 Chrome 的待答队列答复 | 同一执行与原生会话接收 ACP 消息，校验作用域、双方身份和原问题，将答复和消息 ID 交给下一回合 | 执行 1、检查点 0 的进程 41568、42492 共用 Session `01a1e87b-354c-78c0-9264-0c8fca8e6bb7`。候选仅新增 `docs/owner-message-receipt.md`，标题为 `Harbor 3e2f28ca`，下方写入 `QUESTION-1-REQUEST QUESTION-1-RESPONSE`。两个 ID 与 Owner 接口逐字相同，关联 ID 为 `QUESTION-1`。 |
| 在 Docker 本地验证通过后批准候选，等待 GitHub 门禁，核对合并和 Issue 关闭 | 只有受验候选被交付，原任务可追溯到完成状态 | 候选 `feb0923510adeba740d070821e4ea2b9ff2b6c3d` 的两项检查成功，合并为 `ac57c1edad130ed06d96f97d51ab6b73a470df0c`。任务 DONE，Issue CLOSED。[PR #108](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/108) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44041896451) |
| 改变消息的项目、Goal、工作项、执行、序号、问题、Git 依据、参与方或关联信息；重读三个归档来源；检查版本协商 | 不匹配的答复被拒绝，旧回执保持相同。声明使用 1.0 的任务不能退回纯文本答复 | Worker 专项用例拒绝 14 种不匹配的消息。三个历史来源共 10 条消息，摘要与上一节相同。旧任务接受旧格式，1.0 任务拒绝缺少消息的数据。 |

本轮真实 PostgreSQL 与 Docker 全量回归 219 个测试及子测试通过，零跳过，Go vet 通过。新增的协议协商用例及消息绑定专项随后单独复验通过。

原生 Worker 在线接收 ACP 决策答复对应 ForgeOS 提交 `2acb08c87842feb3426eba8022eb785257470510`。

### 专题 05–06 · Management 审阅待答业务问题

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| beta #109 的 Codex 实现回合询问手册标题。启动项目专用的只读 Management 角色，在控制面重启后从 Chrome 打开待答队列 | Management 标注等级并引用冻结输入；临时标题缺乏最终授权时，问题继续等待 Owner | 审阅会话 `01a1e887-68ef-7972-a937-ffc97afc44f2` 从 `requirement:1:body` 引用等待 Owner 的原文，指出 Delta Bay 是临时标题。等级 D2，建议答复和页面输入框均为空。原生实现会话为 `01a1e887-1fc7-7d03-b95a-40e8cd7e567e`，审阅期间无候选。 |
| Owner 在 Chrome 选择 `Harbor 61cdd789`，完成候选验收并等待 GitHub | 实现采用 Owner 的决定，独立审阅留在问题历史中 | 实现进程 35116、43384 共用原 Session。候选 `f6bbd203c03606dc5d293cb513fc03d5a11451bd` 仅新增指定标题文件。两项必需检查通过，合并为 `7f23984672646e04c0776cdbc78cf91aeefd98e3`，Issue 关闭。[PR #110](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/110) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44043633680) |
| 在独立数据库还原 #55 快照，从候选验收接口提问不可逆生产操作所需的批准。让 Management 审阅，再从页面回答 | D3 留空建议。审阅和控制面重启都保留待答状态，Owner 答复后回到原候选 | 审阅会话 `01a1e889-db06-7941-afb8-0fd991b4f727` 引用缺少生产授权的原文。Chrome 显示 D3 和空输入框。Owner 明确未授权删除；执行 3 回到 WAITING_ACCEPTANCE，候选仍为 `aae51b5ff79b882a489d52e9869bd11aa3e47c11`，时间线有一次审阅事件。 |
| 对同一归档候选提出有明确文字依据的 D2 选项；Owner 选择与建议不同的标题。再用原实现 Session 提交新审阅，并在 Owner 已答后提交迟到结果 | 有证据时显示建议，原实现者不能充当独立审阅者。Owner 的最终决定单独留存 | Management 会话 `01a1e88b-dc1b-7bf3-8c4a-448fb1c8e9f7` 建议 `Recovery receipt`，引用本题提供的原文；Owner 选择 `Archive appendix`，两者均可回读。控制面拒绝执行 1 的原 Session `01a1e604-db7d-7882-9d40-5cccf796c57d` 代审执行 3，也拒绝 Owner 已答后的首份审阅，均为 409。 |

#55 的三次提问均在单独的恢复数据库内运行。真实 PostgreSQL 与 Docker 全量回归 221 个测试及子测试通过，零跳过；CI-only 原实现身份校验定稿后，重新通过上述归档实测、数据库专项测试与 Go vet。Web 构建通过。D1 问题的直接代答另行验收。

Management 审阅待答业务问题对应 ForgeOS 提交 `770717acaeb9995e8b2604c38726db93630cf54d`。

### 专题 05–06 · D1 查阅已确认的项目决策

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| Owner 在私有 beta 项目登记并确认决策 #1，限定其用于构建回执的标题；导入 Issue #111，要求实现者查阅此决策 | 控制面冻结提问的任务与检查点，重启后仍能交由该项目的独立 Management 审阅 | 执行 1、检查点 0、基础提交 7f239846。Codex 会话 01a1e896-8746-7ba2-9e99-0aaa9b0e64dd 主动查阅决策 #1。[Issue #111](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/111) |
| 启动 Management，直接从 ACP 和 Chrome 核对答复，继续原生实现 | 独立会话按 D1 引用完整原文和适用范围，Worker 依该记录恢复，无需 Owner 手工答题 | 管理会话 01a1e896-cd20-7953-abef-a3dfb0ad3e88 引用 decision:1。QUESTION-1-RESPONSE 的来源为 agent_generated，关联 QUESTION-1-REQUEST。Chrome 显示 Management 答复；两个实现进程 13644、21072 共用原生会话。仅新增 docs/confirmed-build-receipt.md，标题为 Build receipt 5241b7b0。审核上下文未将该答复计入 Owner 答复。 |
| 核对候选、按 SHA 验收，等待受保护分支检查、合并和 Issue 回写 | 两项检查通过后合并同一候选，关闭 Issue | 候选 c7a8b9da55632bd0ce19dd09186435b7677f80c7，必需检查 acceptance-check 和 forgeos/local-ci 通过，合并 bcae3339bd433ccab80b6a86a541f4d8f95396f4，Issue 已关闭。[PR #112](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/112) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44045978613) |
| 复制 #55 的历史验收库，从正式控制面升级 72 → 73，再新增一道验收问题并用 Owner 答复 | 保留原有三题的答复、身份与审核记录，第四题正常回到原候选 | 逐项比较原有记录一致。新问题 #4 由 Owner 答复，状态恢复 WAITING_ACCEPTANCE，候选仍为 aae51b5ff79b882a489d52e9869bd11aa3e47c11。 |

PostgreSQL 18 与 Docker 全量回归通过 226 个测试和子测试，零跳过。专项验证还覆盖同项目原文、领到任务后失效、跨项目编号、Owner 抢先回答，以及 ACP 来源与授权级别的区分。Go vet 与 Web 生产构建通过。

D1 查阅已确认项目决策对应 ForgeOS 提交 `36b88b4f7d5787517ff6aeb5cbd5def1180a3bf2`。

### 专题 05、08、10 · 审查和测试使用实现时查阅的决策

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| beta #113 要求实现者查阅决策 #1 后创建指定文档。生成候选，再由 Owner 将决策设为 EXPIRED，请求独立代码审查和候选测试计划 | 审查角色能按固定候选复核当时的约定，保留其适用范围与来源 | 原生实现会话 01a1e89f-3138-73c3-9b0d-348bee5465be，决策答复会话 01a1e89f-72fc-7ff0-8936-f0f46ec33402。Claude Reviewer 会话 d410fc2e-9e92-4c84-be19-3424a91857a0 在候选 b209b796408abfc3d1cf7ade6d77de09ff2770f3 未发现问题。任务页展示了实现回合所用的决策原文、适用范围和答复会话。[Issue #113](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/113) |
| 查看测试计划、覆盖审核和 PostgreSQL 固定来源 | 测试计划从决策取到预期标题，独立审核能与审查报告核对同一依据 | Test Agent 会话 01a1e8a1-03d4-7391-b842-1a5ce395a10c 将完整标题和指定路径写进 TC-001，覆盖审阅会话 01a1e8a1-5fc7-7083-a0f8-a6e821ab22b5 返回 covered。重新计算候选依据 SHA-256 得到 ece16246dc966386bdeb545b697d6ffa6ad18686cfb4425cd71a4e63770b7eb5，与计划回执一致。Chrome 在测试计划与审查报告中均展示相同原文和 Management 会话。 |
| 逐步执行 TC-001，核对 Git blob、全路径差异、ACP 时间和原生回合，再按候选 SHA 验收 | 仅新增指定文件，唯一 H1 符合历史决定，查阅和答复发生在首次编辑之前 | 唯一变化是 docs/reviewed-build-receipt.md，唯一 H1 为 Build receipt 8c594e93。提问时索引及工作树与基础树 5395ccd8ceffb2ceb3d842b88561e8b8f2763595 相同。原生恢复输入含 QUESTION-1-RESPONSE 和决策原文，随后调用 apply_patch，节点记录首次文件更改时间 1794904167。用例回执为 passed。 |
| 审批后等待 GitHub 必需检查 | 代码审查、测试计划和人工用例可追溯至同一交付 SHA | acceptance-check 与 forgeos/local-ci 通过，合并为 4183a330a41c8ee0544bc7bc5b55965073819338，Issue 已关闭。[PR #114](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/114) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44047812425) |

真实 PostgreSQL 与 Docker 回归通过 226 个测试及子测试，零跳过。Go vet 和 Web 生产构建通过。

候选审查与测试沿用实现时查阅的决策对应 ForgeOS 提交 `62f820902dfa167f4e51a39550adea4d4a52c08b`。

<a id="check-c-1"></a>

### 专题 16 C.1 · Owner 工作台总览

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在独立的 PostgreSQL 副本还原 beta #55，登记第二个项目、领取其 Git HEAD 任务、提出待确认决策，再对 #55 候选提出验收问题。从 Chrome 打开控制面根地址登录 | 首页汇总两个项目，卡片可直接进入相应队列 | 首页显示全部 3、执行中 1、待答 1、待验收 0。点击“执行中”只看到项目 34 的 T-38。待处理通知打开原项目 T-1 的问题，审批记录打开项目 34 的决策 #1。浏览器后退回到总览。 |
| 在另一客户端答复 #55 并确认决策，留在总览等待刷新 | 已处理的问题与审批离开队列，同一候选回到待验收 | 页面自动显示待答 0、待验收 1，项目 34 的待确认决策消失。点击待验收卡片进入 T-1 的验收队列。 |
| 将 Chrome 视口缩到 390 × 844；对总览的一次审批读取返回测试用 HTTP 503，再恢复真实服务并点击刷新 | 窄屏可进入任务列表，错误可见，重试恢复原始数据 | 页面无横向溢出。导航与返回正常；页面显示服务错误，重试成功后提示消失。其余流量均来自真实控制面。 |
| 对 3 项任务的归档库和 0 项任务的新库分别执行仓库内的浏览器探针 | 核对首页四项计数、四条导航、返回和窄屏布局 | 两次均通过；TypeScript 检查与 Vite 生产构建通过。 |

复验命令为 `python scripts/probe_overview.py http://127.0.0.1:8080`，使用当前控制面配套的 `FORGEOS_OWNER_TOKEN` 和 Chrome。

工作台总览对应 ForgeOS 提交 `30855aaeae2598917382cc1bb42b3a5c189aa283`。

### 专题 10、13、16 C.3 · 运行中查看声明式 CI 日志

以下操作使用私有 beta #55 的独立 PostgreSQL 还原库、本机原生 Codex、真实 Docker 和 Chrome。基础项目的 build 检查先输出一行文字，等待 65 秒，再运行 Go 构建。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从已归档候选 aae51b5ff79b882a489d52e9869bd11aa3e47c11 的正式验收入口退回修改。执行 36 进入构建时，从任务详情读取输出并与节点文件比较 | 在 CI 完成前显示当前检查、候选 SHA 和实际输出 | 候选 a608f2f30e09d114e0e1c99e1346da9982ddce27、ci-run-2770184003、检查 build。页面显示 waiting-for-ci-pause，带行尾换行共 21 字节，与节点当时的 build.log 逐字节相同。 |
| 保持 Docker 检查运行，重启控制面；等待正式 Worker 回报最终结果 | 重启保留同一样本，结算后回到带摘要的归档记录 | 重启后样本含首次观察时间，内容完全相同。format、unit、build 全部通过，任务回到 WAITING_ACCEPTANCE。数据库上的实时样本清空。build.log 的 SHA-256 为 62fde2cd4ebcf34475c2f2458b1a5ff5566f6f9567368c56ee91da7cbeeee5b2，与正式回执一致。 |
| 在另一份 #55 还原库退回候选，从任务详情暂停执行 36 的 build，再从暂停状态恢复 | 原执行结束，新执行重新验证同一候选，实时输出和最终归档都属于新运行 | 候选 f98d9143e97902fad0c4a1b217daa795dbda2708 保持不变。执行 36 的 ci-run-315086847 和执行 37 的 ci-run-2160220780 分别报告 21 字节，观察时间不同。恢复后的 format、unit、build 均通过，归档摘要与上行一致，任务回到 WAITING_ACCEPTANCE。 |

PostgreSQL 和 Docker 均启用时，Go 全包回归共 229 项测试及子测试通过，零跳过。Go vet 和 Web 生产构建通过。

声明式 CI 实时日志对应 ForgeOS 提交 `f374f06d3ade00e8460b7970072f25d3de9b1e4d`。

### 专题 07、16 A.1 · 原生命令审批拒绝与取消

在本机运行 `python scripts/probe_codex_app_server.py --bin <codex.exe> --approvals`，Codex CLI 版本为 0.169.0-alpha.11.1。每个场景在独立的只读 Thread 中请求运行临时目录里的同一标记脚本，审批策略为 untrusted。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 对 turn 01a1e8c7-3a4d-7bf0-9dce-a042483dc8ea 的原生命令审批请求 0 回复 decline | 命令未留下执行标记，模型可结束当前回合 | 工具项 exec-b2f1e209-cbe2-4773-be86-b8b06689c7fc 状态 declined；无标记文件，回合 completed。 |
| 对 turn 01a1e8c7-64a1-7b72-94c0-c6ebc8bc4886 的原生命令审批请求 1 回复 cancel | 命令未留下执行标记，当前回合被中断 | 工具项 exec-205923eb-1b9f-4297-8f50-a69e96c85762 状态 declined；无标记文件，回合 interrupted。 |
| 运行同一探针原有的启动、跨进程恢复和主动中断场景 | 第二个进程恢复原线程并读出随机标识，第三回合中断 | PID 38556、24492，线程 01a1e8c3-b64e-7810-babb-43b1bccf0dcc；三项断言通过。 |

一次额外诊断运行在收到首个审批请求后连接关闭，未取得第二项回执。随后从新线程复跑两个场景，通过了工具项终态、回合终态和文件系统三组断言。

<a id="check-a-3"></a>

### 专题 07、14 · 跨节点共享模型名额

两个 Windows Worker 使用同一份本机 Codex 登录，各有独立的节点身份、工作目录和 SQLite 日志；pool-alpha 只能领取 alpha 项目，pool-beta 只能领取 beta 项目。在 PostgreSQL 18 中配置 qa-codex 共享 1 个模型名额。下表的 HTTP 状态来自正式控制面的资源接口。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从 Owner API 配置两个节点，另用节点身份尝试改配置，再提交两组重叠成员 | 仅 Owner 可以提交；配置整体生效或整体保留 | Owner 得到 204，节点得到 401，重叠配置得到 409。Chrome 的执行节点页显示两个成员及占用 1 / 1。首次浏览器验收发现前端路径白名单缺少新接口，修复后用同一对 Issue 在新的隔离库复验。 |
| 同时派发两项目的真实 Issue；第一项运行时重启控制面 | 第二个节点等待，重启保留预算，首个节点释放后第二个节点可继续 | 执行 1 取得模型名额。执行 2 的模型申请收到 30 次 429。重启后仍为 1 / 1；执行 1 释放返回 204，随后执行 2 获得 204，执行 1 的 CI 也获得 204。两项本地验证通过后共享占用为 0。 |
| 验收两项候选，等待必需检查后回读 PR、合并 SHA 和 Issue | 两项均从各自项目交付，GitHub head 与验收的候选一致 | alpha #22 候选 c8be7c9f6578bfdc7b18b6e39dc28517eca29f6e，合并 fdd4caf85c2cb00ce93725649c85dcc16b6386dd；beta #115 候选 ded9b037b29e6edad15faf74d8f46860c54ee583，合并 679168aaab7bc6c401cc0f5994f7360c8d533f21。两仓的预设首次失败均在确认失败步骤后重跑，acceptance-check 成功，两条 Issue 关闭。[alpha PR #23](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/23) · [alpha Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/44055901297) · [beta PR #116](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/116) · [beta Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44055903938) |
| 经真实 Caddy 双向 TLS 网关查询预算，再在 PostgreSQL 测试中争抢、变更归属并让租约过期 | Owner 可查占用，节点内部地址保持隔离。旧占用须到回报或核实结束时释放 | 带客户端证书和 Owner 身份返回 200，无 Owner 身份返回 401，访问内部领取地址返回 404。数据库回归验证了旧执行计入预算、并发只放行一个模型、CI 可同时取得名额、换组后仍计入原组、租约过期保留占用，人工结束后等待节点获准。 |

启用 PostgreSQL 和 Docker 的 Go 全包回归有 230 项测试及子测试通过，零跳过；Go vet 与 Web 生产构建通过。此次两名 Worker 在同一 Windows 主机运行。Linux 专用服务身份、提供方真实账号限流及独立角色预算分别验收。

普通执行节点的跨节点共享账号名额配置与争用验收对应 ForgeOS 提交 `a04ebc75b41f514c867290fdc93c55c2e3e74d0f`。

### 专题 07、10、14 · 独立 Reviewer 与开发回合共用 Claude 名额

在私有 beta #55 的 PostgreSQL 还原库接入 alpha #24。两个项目在同一 Windows 账号下复用本机 Claude 登录；控制面分别鉴别普通 Worker 身份和项目 1 的 Reviewer 令牌。通过 Owner API 把 role-developer 与该 Reviewer 列入同一个名额为 1 的 Claude 预算。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 检查 #55 的既有候选，并通过 Owner 退回入口准备新的待审查候选 | 历史报告仍对应旧提交，新执行需要独立报告 | 首次探针把已有报告当成尚未领取的审查，断言失败。旧报告确实属于 aae51b5ff79b882a489d52e9869bd11aa3e47c11。按验收意见修订并通过 Docker 检查后，执行 37 的固定候选为 e9ffe79a2d4ae0a9c5474fc2c1c9f0ad62dcf3ce。 |
| 让 alpha #24 的普通 Claude 节点执行返工，同时请求审查 #55 的新候选 | 名额占满时 Reviewer 保持等待，不提前生成有效租约 | 执行 38 占用 1 / 1。支持租约截止时间的 Reviewer 领取返回 204，数据库上的新审查租约为空。未声明截止时间能力的旧客户端也未取得这项已配置预算的审查。 |
| 等开发节点释放名额，再让正式 Reviewer 领取；运行中重启控制面并打开 Chrome 的执行节点页 | 重启后审查仍占 1 / 1，固定候选报告成功回报后占用归零 | 审查租约截止于 17:47:01 +08:00；Chrome 显示开发节点与项目 1 的 Reviewer，共占 1 / 1。独立会话 7ca2e4f2-fde3-47d1-b2fa-e0d177770306 回报执行 37 的固定 SHA，零项发现。占用回到 0。 |
| 核对 alpha 的新增文档、返工内容和本地验证，批准执行 38 的候选 | 开发会话与审查会话各自独立，必需检查通过才交付 | 开发会话 22f3d062-d8db-4d6b-84aa-0081150e9c46，候选 0f268fee27df26e41d72ff2984e2c2d6b079b883。真实必需检查经预设失败重跑后成功，合并 ca6c58459a24aa9bcbaa12e1c6a0e8e48f9ed269，Issue #24 关闭。[alpha PR #25](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/25) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/44058056793) |
| 从数据库和网关复测既有安装 | 失败、失效和重复回执按租约释放；只有普通节点的旧预算仍可读取 | PostgreSQL 实测了在途审查首次计入、两个方向争用、失败释放、过期后重新领取、旧租约拒绝和成功回执重放。旧预算库升级后，经真实 Caddy 双向 TLS 返回原有节点预算字段，Owner 200、匿名 401、内部派发接口 404。 |

本项提交前的全包回归启用了 PostgreSQL 和 Docker，230 项 Go 测试及子测试通过，零跳过。Go vet 与 Web 生产构建通过。

独立 Reviewer 共享模型名额对应 ForgeOS 提交 `0c95c978d5f331b23eb714d9565be3876724c49c`。

### 专题 10、11、14 · 候选测试计划和覆盖审核共用 Codex 名额

在 beta #55 的隔离 PostgreSQL 18 还原库，通过 Owner 接口把项目 1 的候选 Test Agent 和 Management 覆盖审核列入同一个名额为 1 的预算。两种角色均通过正式 Worker 入口调用 Windows 上已登录的原生 Codex。固定候选为 `e9ffe79a2d4ae0a9c5474fc2c1c9f0ad62dcf3ce`，执行 37。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 申请修订 3，依次运行 Test Agent 和覆盖审核，在审核租约内重启控制面，并在 Chrome 打开执行节点页 | 两角色依次领取；重启保留占用，页面显示成员 | Test Agent 回报 204 后覆盖审核取得名额。Chrome 显示两个角色、已占 1 / 1。不支持租约截止时间的旧客户端领取返回 204 空响应。 |
| 由本机验收代理暂时对旧审核回报返回 503。在该回合尚未交回期间申请修订 4，再恢复回报通道 | 新版等待；旧结果只能释放名额，无法推进新版 | 修订 4 的 Test Agent 领取返回 204 空响应，数据库租约为空。恢复通道后修订 3 的审核回报收到 409。随后修订 4 的两个角色分别完成，回报均为 204。最终占用 0 / 1。 |
| 在 Chrome 打开任务的修订 4，核对会话与两项用例。从固定 Git 对象实查两份文档，经项目授权的 Runner 提交结果 | 独立覆盖报告和 Runner 实测均绑定同一份计划；基线到候选只包含 Owner 指定的文档变更 | Test Agent 会话 `01a1e8f1-3cb7-74b3-97ec-82d3e4cc8f7e`，覆盖审核会话 `01a1e8f1-b967-7053-a4f0-58775c8c42e5`，覆盖状态 covered。基线 `a6a1381648ea89096370f092337e1ee86aa2d745` 至候选只改了两份文件。queue-pause.md 第 1 行包含 `go test ./...`，ci-shared-review-receipt.md 第 1 行与 Owner 的 `# Shared review receipt` 逐字一致。两项 Runner 回报均为 200，回读计划 `fd26e006a8bb5d53bf58cd48b0705a0bb460037827db9eff2187e2669a150447` 的实测状态为 passed。 |
| 经 Caddy 双向 TLS 查询从版本 77 升级的既有预算；在 PostgreSQL 中复验在途任务及重复配置 | 旧配置仍可读取；同一项目角色不会分入两个预算 | Owner 200，匿名 401，内部领取地址 404。旧预算的空角色字段保持省略。在途任务首次配置时纳入占用；重复归属返回冲突。 |

提交前启用 PostgreSQL 和 Docker 的全包回归共 230 项 Go 测试及子测试通过，零跳过。Go vet 与 Web 生产构建通过。

候选测试计划和覆盖审核共享名额对应 ForgeOS 提交 `a6834639fec8ade64b984bd4ae6cf3264d1909d8`。

### 专题 07、16 · 原生命令单次批准后再次执行

在 Windows 使用本机已登录的 Codex CLI 0.169.0-alpha.11.1，运行 `python scripts/probe_codex_app_server.py --bin codex.exe --approvals`。四个回合使用同一只读 Thread `01a1e8fa-4b1d-7470-94ea-7c3ddf72dc9a`。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 请求 Python 打印本次随机数并答复 accept | 已批准的工具项运行成功，标准输出与预期逐字一致 | 请求 0，工具项 `exec-9e9486b8-cf9c-4903-a784-a9da08f97fd5`，状态 completed，输出 `542300488664`。 |
| 在下一回合重复同一命令，答复 decline | 再次产生审批，拒绝后该工具项结束 | 请求 1，工具项 `exec-29b10957-7c90-4968-b5c9-8b67aedd24dd`，状态 declined，回合 completed。 |
| 对会创建文件的脚本分别答复 decline 和 cancel | 均无执行标记；取消中断回合 | 请求 2、3 的工具项均为 declined，标记始终不存在。前一回合 completed，后一回合 interrupted。 |
| 运行探针默认入口，关闭进程后恢复原线程，再中断第三回合 | 新进程能读回首轮随机标识，中断有终态 | PID 47180、46436，原线程 `01a1e8fa-8a7b-7352-b0c9-e4e79061865e`，中断回合 `01a1e8fa-c3b4-7360-884c-f8c6648ae051`。 |

首次尝试批准读取临时目录的 Python 脚本，沙箱返回文件访问被拒绝，工具项 failed。改用只需标准输出的命令验证单次批准，保留原来的脚本验证拒绝和取消。正式 Worker 的 App Server 审批链路仍待验收。

原生命令单次批准探针对应 ForgeOS 提交 `10b29daaa10d4db97ad7423c22815f06485e59c1`。

### 专题 07、08、14 · 知识提取与审核共用模型名额

在独立 PostgreSQL 18 验收库登记本机 QA 项目。Windows 上的专用 Curator 和 Management Worker 使用各自项目令牌，分别调用本机已登录的 Codex。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从版本 78 的 Owner 接口尝试配置两个知识角色，升级后重试；提交第一份资料 | 可登记两个角色，共享一个名额；旧客户端在有预算时等待 | 原接口返回 400。升级至 79 后同一配置返回 204。旧 Curator 与旧 Management 均得到空的 204 领取响应；新角色完成第一份资料的提取并开始审核。 |
| 在第一份资料的审核租约内重启控制面，导入第二份资料，打开 Chrome 的执行节点页 | 重启保留占用；第二份资料等待，不产生提取租约 | Chrome 显示知识提取、知识审核共占 1 / 1。支持截止时间的 Curator 领取返回空的 204，第二份资料的提取租约为空。验收代理允许在观察窗口暂缓审核回报，随后恢复通道。 |
| 等待队列排空，读回两份材料的来源摘要、会话、经验列表及占用 | 四次真实模型调用各自留下回执，只有独立审核通过的经验才进入项目，名额归零 | 下表四个结果入口均返回 204。两个审核均为 D1 approve，两个资料键均在项目经验列表中，占用为 0 / 1。 |
| 经 Caddy 双向 TLS 读取既有普通节点预算，复验项目角色重复、过期与重放 | 旧预算可原样读取；角色重放和失效租约不会提前释放另一个回合 | 网关 Owner 200，匿名 401，内部领取 404，旧预算仍有两个节点且省略空角色字段。PostgreSQL 回归覆盖首次配置时的在途提取、重复归属、在审核期间重报提取结果、过期后让出名额、旧审核回报冲突和空提取。 |

| 资料键、修订 1 | 固定内容 SHA-256 | Curator 会话 | Management 会话 |
| --- | --- | --- | --- |
| queue-checkpoint-c7219e | `a927becb7162a94b21e15b2db239e789b5ddc6ee2ef4a50cbf77534c9b0272de` | `01a1e8ff-2c8b-7e02-9bd9-3620d7dcad9f` | `01a1e8ff-5a9b-7990-aab7-21df4ed733ce` |
| queue-replay-c7219e | `1f8033e9b78584beeacc9c23a7b5ed54685dad88c4257edf66766f4015258d2e` | `01a1e8ff-8cec-7a70-9b89-8c66be6931e5` | `01a1e8ff-bbdc-7b83-84a9-7d0c90f10155` |

首次全包回归指向已关闭的旧测试库端口，连接被拒绝。改用本次真实验收所用的独立 PostgreSQL 18 实例，在新的隔离 schema 中重跑全部包，启用 Docker 后 231 项 Go 测试及子测试通过，零跳过。Go vet 和 Web 生产构建通过。

知识提取和审核的共享名额对应 ForgeOS 提交 `4ae0fae3ff39c7cab0043c92fb3b17d59feda169`。

<a id="check-b-2"></a>

### 专题 07、16 · 从执行节点核对适配器的版本与操作

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 启动装有两种 CLI 的 Windows Worker，读取版本、Owner API，在 Chrome 打开执行节点并派发 Git HEAD | 版本与原生可执行文件一致，显示当前适配器能够执行的操作，节点能正常领取任务 | 基线节点只显示两个支持标志。更新后的节点 `adapter-qa-d68369` 显示 Codex `0.169.0-alpha.11.1`、Claude Code `2.1.310`；两项均报告 probe、start、resume、events。任务 2 完成并返回仓库 HEAD `65d40b2a22dcec366c5146ed5924c607b09f7cbd`。 |
| 重启控制面；用同一节点身份移除 Claude 配置、切换到旧 Worker，再将新 Worker 指向旧控制面 | 持久记录可读，停用的适配器及时消失，新旧节点均能登记 | 重启后完整报告一致。移除 Claude 后只剩 Codex。旧 Worker 的下一次心跳清除旧探测报告并保留原能力字段；旧控制面接受新 Worker 心跳。HTTP 回归另验证不一致或未知的操作返回 400，保留此前报告。 |
| 导入两个私有仓库中的文档 Issue；首次模型回合向 Owner 提问，在等待时重启控制面，经 Chrome 答复并检查两次进程回执 | 下一回合使用同一 Session；进程版本匹配心跳，事件和候选都能回查 | 下表两个执行各有两个不同的 PID，各自共用同一原生会话。Git 对象中仅指定文件发生变化，H1 与各自 Owner 答复逐字相同。 |

| 入口 | 会话、进程及工具事件条数 | 验收候选与交付 |
| --- | --- | --- |
| alpha #26 · Codex | `01a1e914-fe04-77d1-8d9c-91093ee75d2e`，PID 37992、43640，分别记录 0、8 条工具事件 | 候选 `d05878b7ebe296240ac796e6eaa06951a390004a`，合并 `e5fa4a03de6d6839118e5fa513db1c084298553d`。[PR #27](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/27) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/44066658220) |
| beta #117 · Claude Code | `d9afedc9-bea3-4bad-ac7a-03e2bd293a52`，PID 11044、36456，分别记录 12、2 条工具事件 | 候选 `ae076c7b347cd3e782af5d458ea6a163934c8370`，合并 `70f6e6136cdacd6b20ae7e515bae5d33a7433d64`。[PR #118](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/118) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44066462156) |

alpha 首次导入选择了 auto。原生会话恢复成功，但固定基线缺少 validation.yaml，候选 `97b9413e9a07fc6a5ad673e17e43073c248a2de7` 在本地得到 CONFIG_FAILURE。保留该记录后，在新的隔离 schema 用 alpha 的 docs 配置重新导入同一 Issue。两仓均核对了必需检查的预设首次失败步骤，重跑通过后合并。启用 PostgreSQL 18 和 Docker 的全包回归共 232 项 Go 测试及子测试通过，零跳过。Go vet 与 Web 生产构建通过。

执行节点的适配器版本和操作报告对应 ForgeOS 提交 `54122385ef11112ba1b00802d050172cf1d72858`。

### 专题 07、16 · 原生回合的终态失败与会话恢复

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 导入 alpha #28 和 beta #119。验收代理为两个执行各配置一个不存在的模型，启动真实 CLI，在 Chrome 打开任务 | 单次模型失败保留退出码和会话，无自动恢复，无候选 | 两个任务均为 FAILED，各只有一条进程回执。页面显示 `harness reported a failed turn: exit status 1`。Codex 原生 PID 28084，Claude 原生 PID 39332。 |
| Owner 重试。代理等待原生 CLI 正常退出，仅扣留新执行首个回合的成功终态，以 23 退出 | 同一新执行只恢复一次，复用原会话与 Worktree，正确完成本地验证 | 各有两条适配器回执，退出码依次为 23、0。四个底层 CLI 均正常退出。两仓均只新增 Issue 指定的 Markdown 文件。alpha 首次证据脚本因标题多一个句点停止；回读 Issue 原文和 Git 对象，确认句点紧随指定标题，按该仓实际标题复核后批准。 |
| 将 alpha #28 单独以只读规划模式导入，在首个回合之后扣留终态。打开进程记录，再重启控制面 | 保持只读和原工作区，形成一份计划，重启后结果一致 | 两个原生 PID 37852、40436 的会话均为 `01a1e920-c8e1-7430-a9b6-45fa995394f5`，两份运行上下文均为 read-only、approval never，同一目录无改动。任务进入 PLANNING，计划修订 1，重启后完整任务回读一致。 |
| 从两个实现任务分别批准固定候选，观察必需检查的首次预设失败，重跑失败的检查 | 重跑成功才合并；ForgeOS 的交付回执与 GitHub 的候选、合并和 Issue 状态相符 | 两个任务均为 DONE，两个 Issue 均已关闭，GitHub 的 acceptance-check 与 forgeos/local-ci 均成功。 |

| 入口 | 恢复会话、原生 PID | 验收候选与交付 |
| --- | --- | --- |
| alpha #28 · Codex | `01a1e91d-278b-71c3-b84f-1bb4b91f915d`，41568、24652 | 候选 `7bcdc825f79ed2d1c671e6aa9f3a3e3963ed93e1`，合并 `be0f75750c0d38b790a487f533e731ceea44d09e`。[PR #29](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/29) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/44068493265) |
| beta #119 · Claude Code | `1ae18d72-21ab-4448-ba03-3f0870f0dfda`，48976、38328 | 候选 `8b36eddf3802c4c96ea137b9232ceb174332120d`，合并 `e47bf8567555366738a3d5591831d7a8d96c13b6`。[PR #120](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/120) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44068122855) |

启用 PostgreSQL 18 和 Docker 的全包回归共 237 项 Go 测试及子测试通过，零跳过。回归包括两种适配器报告失败终态并以 0 或 23 退出的组合。Go vet 和 Web 生产构建通过。真实模型进程在执行途中被强制终止以及整机断电仍待验收。

原生回合终态分类与恢复对应 ForgeOS 提交 `69b884d6fd2d57bfcf04f65b7226829eeea298db`。

### 专题 06、07 · Owner 向下一回合排队

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从两个仓库导入只规定页面主标题的 Issue，启动 Windows Worker，在 Chrome 的首个模型回合提交随机副标题 | 指令绑定执行和当前检查点；输入与来源 Issue 评论分别记录 | 首次点击发现工作台的 API 路径校验未包含新入口，两个浏览器均未发出请求。修复后重新导入同一对 Issue，Chrome 在检查点 0 成功入队，两次均收到 204。 |
| 在入队之后重启控制面，重报原请求，再改写同一键的正文，用 Worker 身份重复请求 | 原请求幂等，改写被拒，只有 Owner 可以提交 | 两仓分别返回 204、409、401。通过真实 Caddy 双向 TLS 对 alpha 重报亦为 204，改写 409，匿名与 Worker 均为 401，内部领取路径为 404。 |
| 检查页面处理回执、模型会话、候选 Git 对象及本地验证 | 第二回合使用首轮会话并应用指令，候选包含 Owner 在首回合期间给出的副标题 | 两仓均有 3 个检查点、0 条评论，检查点 0 保存唯一指令，1 确认同一会话，2 为空。Chrome 显示“下一回合已确认处理”。两仓分别只新增一个文件，保留 H1，H2 与下表对应，验证退出码均为 0。 |
| 批准各自候选，核对必需 CI 的受控失败和重跑 | 通过后合并，交付回执与 GitHub 的 SHA、Issue 状态一致 | 两个 PR 均已合并。原 Issue 已关闭，acceptance-check 和 forgeos/local-ci 通过。 |

| 入口与实测 H2 | 会话、PID | 验收候选与交付 |
| --- | --- | --- |
| alpha #30 · `Harbor bef55d6b` | Codex `01a1e928-6244-7b43-adf6-39d606535636`，45272、47192 | 候选 `30607e45d9af0a8658f7049d77a8b9ee2168df92`，合并 `01ac03a0e52d3894593d3fefe7270393520a6ab4`。[PR #31](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/31) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/44069874104) |
| beta #121 · `Harbor 24c642ac` | Claude Code `48ff6486-aaf1-41b3-802e-01dfef8854ab`，43516、45988 | 候选 `022715f584056659b46a9caf4b7ec139050c06a8`，合并 `30731869de50e47a71d0cc19f8f3792c20e0ef25`。[PR #122](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/122) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44069914209) |

PostgreSQL 18 和 Docker 均启用的全包回归共 238 项 Go 测试及子测试通过，零跳过。数据库回归覆盖入队顺序、容量、旧 Worker、错误会话、跳过待处理指令的候选登记与结果上报、结束后重报。Go vet 和 Web 生产构建通过。

Owner 下一回合指令对应 ForgeOS 提交 `2c13ca8adf9218e748962e136836b4817337f906`。

### 专题 06、08、10 · 从实现检查点追溯候选审查与测试依据

ForgeOS 实现提交 `ba191fd42ed5f0313bbc464e0fd8ba4a291652d8`。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在版本 80 回查 alpha #30 和 beta #121，再用 beta #123 实现一项新需求并保存第一版测试计划 | 能分辨实现回执与当时各角色真正取得的依据 | 两个旧执行均有已处理的 Owner 指令，旧候选依据中均未包含。#123 首回合从 Chrome 指定 H2 为 `Harbor f76f3df2`；旧版测试计划保存时也没有该依据和摘要。 |
| 升级控制面后，向同一候选请求独立代码审查和第二版测试计划，再重报创建请求 | 保留修订 1；新修订和审查取得同一实现检查点的指令 | 两份新回执列出检查点 1、实现会话和完整指令。Reviewer 的报告明确核对了指定 H2；Test Agent 将请求键和指定文字写进 TC-001。重报仍返回修订 2。|
| Management 审核修订 2；重启控制面，在 Chrome 切换审查报告及两版测试计划 | 独立角色核对同一固定候选，重启后仍能分辨历史修订 | 修订 2 获得 covered；审查和计划依据分别在页面展示相同的指令及实现会话。修订 1 仍没有新增的依据。三个回执在重启前后相同。用 PostgreSQL 保存的原文重新计算的依据 SHA-256 为 `ac72f4586bdf787674c55fd4464921b021180a7f8630f07fb49a878bb248ff0d`，与修订 2 和测试计划中的值相同。 |
| 以项目限定的 Runner 身份从两个固定 Git 提交执行用例，批准候选，再从 GitHub 核对交付 | 文档只新增一个文件，唯一 H1 之下的 H2 与 Owner 原文一致；必需 CI 成功后合并并关闭 Issue | TC-001 回读指定文件并核对两个标题的行号，TC-002 核对完整路径列表和差异，两项均通过。候选 `24a95639acc4641ebf3cdc7421c5b8a03232e373`，合并 `d0a337b859ecf72271d2641787e5d2e3f5dd3887`。预设首次 CI 失败后重跑通过，任务 DONE，Issue 已关闭。[PR #124](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/124) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44071377188) |

| 回合 | 独立原生会话 |
| --- | --- |
| Claude 实现 | `da3aaacc-cc92-4039-9426-03b00577ddc3` |
| Claude Reviewer | `a3980a7b-e4cd-455b-aee1-decaa390a521` |
| Codex Test Agent | `01a1e92f-b990-7cd2-b0c6-849ba980e460` |
| Codex Management 覆盖审核 | `01a1e930-269f-7753-b531-c19f1d49da80` |

开启 PostgreSQL 18 和 Docker 的全包回归共 238 项 Go 测试及子测试通过，零跳过。数据库回归另核对一批 8 条指令的顺序、处理检查点和会话，Go vet 与 Web 生产构建通过。

### 专题 07、14、16 A.3 · 两个 Goal 计划争用同一 Codex 名额

ForgeOS 实现提交 `4eaf44a273aa9b2bec2f9ea0ab126b714921d69c`。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在版本 81 从 Owner API 导入 beta #2 和 #19 并提交两份 D2 草案，配置项目 1 的 `plan_review` 预算，再升级至版本 82 重试配置 | 升级后两份在库草案可使用一个共享名额 | 升级前配置返回 409，升级后返回 204，初始占用 0。只支持旧预算截止时间的领取请求返回 204，没有领取草案。 |
| 启动第一台 Management Worker，观察原生 Codex 进程，重启控制面并启动第二台 Worker。通过 Owner 登录 Chrome 工作台查看执行节点 | 重启后已有审核仍占 1/1，其他 Worker 等待，第二份草案尚无租约 | 工作台显示“planning · codex · 已占 1 / 1 · 项目 1 的Goal 计划审核”。第二台 Worker 和额外一次兼容领取都返回 204；数据库中任务 2 的审核租约为空。 |
| 允许第一份真实审核回报，等两份回报入库后重启控制面 | 名额可再次领取；两份独立审核可追溯，D2 草案等候修订 | 第一台 Worker 先后领取任务 1、2；两次领取的截止时间分别为 19:35:17、19:35:32（UTC+8）。beta #2 原生会话 `01a1e944-bbe2-70f0-9eb9-c4070008101f`，beta #19 原生会话 `01a1e944-f47c-7de2-aed6-90e79f1880df`，两份均为 D2、revise。占用归零，重启后两份草案与审核回执仍可查。 |

在 PostgreSQL 18 和 Docker 环境运行全包回归，238 项 Go 测试及子测试通过，零跳过。数据库测试另覆盖配置时纳入在途审核、旧草案回报 409、过期重领、跨项目回报和重复回报。Go vet、Web 生产构建通过。

### 专题 07、08、14、16 A.3 · 撤销正在审核的跨项目资料副本

ForgeOS 实现提交 `e87c653f129db9f0dee40256c93fadabdbabab9a`。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在版本 82 的控制面登记 beta 和 alpha 克隆。在来源项目保存资料并从固定修订 1 创建两份脱敏副本；配置一个 `share_review` 名额，升级后重试 | 原草稿可沿用；启用预算后只有声明支持该租约的 Worker 能领取 | 版本 82 返回 409。版本 83 配置返回 204，初始占用 0。只声明支持计划审核截止时间的客户端请求共享审核收到 204，接收方列表为空。 |
| 正式 Management Worker 启动原生 Codex 审核副本 1 后，Owner 撤销副本，重启控制面，启动另一台 Worker，并从 Chrome 查看执行节点 | 审核仍在用账号；副本 2 等待，可凭原租约交回已撤销的工作 | 页面显示“项目 1 的共享资料审核”，占用 1/1。第二台 Worker 空领取，副本 2 的租约为空。副本 1 原生会话 `01a1e949-a657-7da3-83c9-a30d911435ea` 回报 409；重启后读到撤销时间，审核栏为空。 |
| 等副本 2 独立审核完成，重启控制面，查看接收方列表，再用 Owner 身份发布副本 2 | 名额归零；接收方只取得已发布的脱敏副本 | 会话 `01a1e949-fad2-7ba2-9aa2-ff1cfaab5d28` 返回 204，D3 审核通过。发布前列表为空，发布后只有副本 2，摘要 `dc8bcd8740ebdc0ec869a59a6bdd10e086f80279b32c627e47893a725a941aac` 与来源项目一致，内容没有私有资料标记。 |

PostgreSQL 18 和 Docker 全包回归 238 项测试及子测试通过，零跳过。数据库测试还核对为已有回合配置预算、错误租约不能提前释放、审核回执幂等和接收方授权。Go vet 和 Web 生产构建通过。

### 专题 07、08、14、16 A.3 · 固定提交的仓库分析共用 Codex 名额

ForgeOS 实现提交 `0fa8e68a48790ff8cc7656af0ed1f9a8a68962e4`。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在版本 83 登记 Git 仓库。分别在 README 提到 North Station、South Station 的两次提交上，从项目检查入口让普通 Worker 读取 HEAD，保存清单并申请归纳 | 两份资料均固定到检查时的提交。升级至 84 后可以给原有任务配置一个名额 | 两项均进入 READY。配置 `repository_analysis` 的结果从 409 变为 204。只声明旧角色截止时间的客户端不能领取。 |
| 启动 Management Worker 及原生 Codex，重启控制面，启动第二台 Management Worker，从 Chrome 打开执行节点 | 占用继续显示 1/1，第二份报告尚未开始 | Chrome 显示来源项目与“仓库分析”。第二台 Worker 和一次独立领取均收到 204；第二份任务在数据库中尚无租约。 |
| 收取两份报告，按各自 HEAD 重新计算 README blob；重启控制面再回读 | 释放后可领取下一份，两份原生报告引用各自的文件 | 两份均 DONE，各有两条事实，分别引用 North Station 或 South Station，均列出 `go test ./...`。API 中的 blob 与 Git 一致，重启后两个会话仍可查，占用为零。 |

| 检查任务 | Git 提交 | README blob | 原生分析会话 |
| --- | --- | --- | --- |
| 1 | `23b5f39b97ae34822d7a3c6194b5d6f12534d5cb` | `0e5d1778de236ced7d2ec46086a797efe56717d9` | `01a1e94d-cff6-7c91-8acd-29d05bf84698` |
| 2 | `40b565becfb25e31c767c88bb46bbca70a47385b` | `c60f743bdf0d1df71103f5b16f61cea65b13709f` | `01a1e94d-fbc7-7521-b5d6-eae01e7a20b4` |

PostgreSQL 18 和 Docker 全包回归 238 项测试及子测试通过，零跳过。数据库回归另验证在途配置、过期与重领、旧租约回报、失败后重试和成功回报重放。Go vet 和 Web 生产构建通过。

### 专题 07、11、14、16 A.3 · 固定预览上的两条观察争用反馈分类名额

ForgeOS 实现提交 `a410eb5dc16c536db67818b2b2afc84a9dff2b07`。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 将私有 beta #125 导入控制面，让正式 Claude Worker 实现预览标题的检查说明。按候选 SHA 启动预览，用 Chrome 读取标题和 H1，再通过反馈入口分别登记两条观察 | 工作区只增加指定检查说明；两次独立观察均与同一候选关联 | 实现会话 `3bc7da32-5437-4772-9d0b-f7c4c4c224c0`，候选 `5da50fd7832b706d086d5afbeaff8a932934b4ce`，唯一变更为 `docs/queue-preview/feedback-check.md`。Chrome 核对标签页标题、可见 H1 均为 Queue handbook。反馈 1 和 2 已登记。 |
| 在版本 84 配置来源项目的 `feedback_classification` 名额；升级至 85 重试。启动原生分类回合后重启控制面，再让第二台 Management Worker 领取 | 升级后配置生效；第一份回报之前，第二份不取得租约 | 预算配置从 409 变为 204。旧能力领取返回 204。Chrome 显示项目 1 的预览反馈分类占用 1/1；第二台 Worker 及独立领取均返回 204，反馈 2 的租约为空。 |
| 两次回报后重启，从反馈历史回查每条证据及任务的验收状态 | 两条分类各自引用观察原文，独立于实现会话；名额归零，候选等待 Owner 验收 | 原生 Codex 会话分别为 `01a1e958-8d77-75a2-9af9-1e256acab17f` 和 `01a1e958-b957-7f72-8209-03f2313e608c`。两条均为 acceptance，证据都是对应观察的原文，均无 Owner 决定，任务仍为 WAITING_ACCEPTANCE，账号占用为 0。 |

PostgreSQL 18 和 Docker 全包回归包含预算生效时的在途分类、过期重领、证据校验、错误项目、重复回报及事件幂等。238 项 Go 测试及子测试通过，零跳过。Go vet 和 Web 生产构建通过。

### 专题 05、07、14、16 A.3 · Owner 先答复时的业务问题建议与交付

ForgeOS 实现提交 `ff584935cede64bfa9cb1d6aa9006eb27f44fe0f`。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 对 beta #125 的已分类预览观察，Owner 在候选验收时询问是否检查浏览器标题。用版本 85 配置一个 `question_advice` 名额，升级至版本 86 重试 | 沿用同一问题和候选，支持截止时间的 Management Worker 才可领取 | 配置从 409 变为 204，旧客户端得到空领取，任务等待回答。第一台 Worker 开始原生建议回合。 |
| 首个回合运行中，Owner 明确回答应检查标签页标题，再询问是否应检查可见 H1。重启控制面，启动第二台 Worker | Owner 决定立即生效；第一回合返回前，第二条问题等待账号名额 | Chrome 可见占用 1/1；第二台 Worker 空领取，问题 2 尚无租约。原生会话 `01a1e95d-c58a-7bf2-b1fb-813db57bafe8` 对问题 1 回报 409。重启回读 Owner 答案仍为原文，问题 1 没有写入过时建议。 |
| 问题 2 领取、回报后重启，读取建议及领取时的来源，再从 Owner 入口回答 | D2 建议引用可核对的固定来源，等待 Owner 作决定；两回合结束后占用归零 | 会话 `01a1e95d-f2c3-7b23-addb-cb540879b1b0` 回报 204，引用 `requirement:1:body` 中同时要求标签页标题和可见 H1 的原句。回读时状态 WAITING_INPUT，占用 0。Owner 确认两者均须满足后回到 WAITING_ACCEPTANCE。 |
| 以同一 SHA 核对 Markdown 标题、唯一变更路径和验证回执，Owner 同时引用反馈 1、2 批准。跟踪真实 PR、必需检查和来源 Issue | 决定绑定相同候选和两条观察，检查通过后交付并关闭 Issue | 候选 `5da50fd7832b706d086d5afbeaff8a932934b4ce`，两条观察均关联批准回执。预设的首次 CI 失败后重跑，`acceptance-check` 与 `forgeos/local-ci` 均成功。PR #126 已合并，Merge SHA `2824245d2c7270bfc1aecbebeecbecaa713def67`，任务 DONE、Issue #125 已关闭。[Issue #125](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/125) · [PR #126](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/126) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44078720644) |

全包 Go 回归在 PostgreSQL 18 和 Docker 上通过 238 项测试及子测试，零跳过。数据库覆盖在途配置、过期重领、D1 决策查阅、错误项目、D3 下限及结果重放。Go vet 和 Web 生产构建通过。

<a id="adr-evidence-009"></a>

### 专题 07、14、16 A.4 · Linux 服务身份的原生 CLI 探测和节点重启

ForgeOS 验收提交 `c18af6d27c6e054d50a79959e43ed28d71963f66`。

在 WSL Ubuntu 上使用仓库的 Worker systemd unit，仅将可执行文件和环境文件路径改到本次验收的运行目录。控制面运行于 Windows，使用 PostgreSQL 18。控制面和 Linux Worker 的项目根目录不同；两侧仓库来自同一固定 Git 提交。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 用专用用户安装并执行 Linux 原生 CLI，回读 HOME、权限与版本 | 服务身份有独立 HOME，CLI 在 unit 的 PATH 中可用 | 用户 `forgeos-worker`，HOME 为 `/var/lib/forgeos-worker`，权限 0700，Node 22.23.5，Codex 0.169.0-alpha.11.1，Claude Code 2.1.310。 |
| 启动正式 Worker，通过 Owner API 回读心跳，再派发一次项目 HEAD 检查 | 控制面取得两个适配器真实探测的版本与操作，Worktree 归 Worker 所有 | 节点 `linux-native-32807572` 报告上述两个版本，两者均声明 probe、start、resume、events。任务 1 为 DONE，Worktree 属主为服务用户。 |
| 重启该 systemd unit，再从项目入口重复检查 | 新进程使用同一 HOME，登记相同能力并能完成新任务 | PID 从 2681 变为 2787；任务 2 同样为 DONE，两次都返回 `0cc6087ae6cda0df01832cbf62328d0d1d6aef39`，执行目录分别为 execution-1、execution-2。 |
| 在同一专用 HOME 中运行两个 CLI 的登录状态命令 | 要验收真实模型和代理请求，须有已登录的账号 | 两者均返回未登录。登录复用、经代理发出真实模型请求和服务重启后的会话恢复保持待验收。临时验收 unit 已停止，专用用户及 CLI 保留供登录。 |

### 专题 07、16 A.2/B.4 · 在真实 Claude 工具调用后终止原生进程

ForgeOS 验收提交 `332568f564b9fef65703507dc8cdf6426a730058`。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在私有 beta #127 由正式 Worker 实现预览恢复检查说明。等节点 SQLite 记下原生会话与已完成的工具事件，然后核对 PID、可执行文件、父 Worker 和当前工作区，再终止本次任务的模型进程 | 真实原生进程在完成回合前退出，已有证据保留 | Claude Code 2.1.310 已完成 Glob。PID 42168 的进程退出码为 15，原生会话为 `f7bbbbd6-eac1-45ea-aa4d-18de5ce17f33`。 |
| 保持 Worker 运行，回读后续原生进程、执行历史、工作区及候选 | 另一进程使用同一会话完成剩余工作，产生一个执行和一个可验收候选 | PID 39988 恢复同一会话并以 0 退出。任务只有执行 1，Worktree 未变。候选 `ed272c237aa9e25ebc53ea35a8a8e3a93c889ce3` 通过本地验证，唯一变更为所要求的 Markdown 文件。核对了两个来源路径、唯一 H1 和 Queue handbook 标题。 |
| Owner 批准固定 SHA，跟踪 GitHub 检查、合并及 Issue 回写；重启控制面，用 Chrome 展开任务的 Harness 进程记录 | 交付保持按 SHA 绑定，事后能区分原进程与恢复进程 | 首次必需检查按演练设置失败，再次运行通过。PR #128 合并为 `510aea6eb12c027d12372e64cd53a09d212e5f9c`，任务 DONE，Issue 已关闭。Chrome 同时显示两个 PID 和相同的两个会话字段。[PR #128](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/128) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44080486657) |

运行 PostgreSQL 18 和 Docker 的全包回归，238 项 Go 测试及子测试通过，零跳过。此次复验复用了现有恢复实现 `69b884d`。

### 专题 07、16 B.4 · 在真实 Codex 命令执行后终止原生进程

ForgeOS 验收提交 `9da6a8e35745b4455f3aa5f9a96d9c0a3f860378`。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 导入 beta #129，使用正式 Codex Worker。等待本机日志登记原生 Thread 与一次已完成的 command_execution，核对可执行文件、当前 Worktree 和所属 Worker 后终止该进程 | 第一次进程能与任务关联，未完成的回合以非零退出码结束 | 进程实例 1，PID 45188，退出码 15，Thread `01a1e96c-f791-7bd3-b939-401b2d0afefe`。 |
| 等待 Worker 核对租约并恢复。对比进程、实际 Git 差异和本地验证 | 同一执行、工作区和 Thread 继续完成工作；最终交付使用恢复后的候选 | PID 45340 沿用该 Thread，以 0 退出。只有执行 1。Git 差异只有检查说明，核对了唯一 H1、两个来源路径、预期标题和成功的文档验证。候选 `f18aadce03fcff29c40c13a483b44545da1eaebd`。 |
| Owner 批准，跟踪必需检查；重启控制面，从 Chrome 打开任务的进程记录 | 合并、Issue 回写和两次进程的会话记录均可复查 | 必需 CI 首轮预设失败、重跑通过。PR #130 合并为 `2db282232add7f7571f5ca60473573d16f83de10`，Issue #129 已关闭，任务 DONE。Chrome 中两个 PID 及两个相同的会话字段均可见。[PR #130](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/130) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44081314568) |

### 专题 06、07、16 B.4 · 只读 Planner 原生进程终止后的草案与沙箱

ForgeOS 验收提交 `11bed5a4d07f536a0cf8a340f1d83174a0438a98`。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在隔离数据库中以 `mode=plan` 导入已存在的 beta #19。等真实 Codex 完成一次命令，再按执行号、原生 PID、父 Worker、可执行文件和 Worktree 核对并终止 Planner 进程 | 首次进程非零退出，Worker 可凭已确认的 Thread 重试只读规划 | PID 13332 退出码 15，恢复进程 PID 47096 退出码 0。一次执行的两次进程均为 Thread `01a1e975-4287-70f3-bcb3-bf31f2f5cfef`。 |
| 从原生 Thread 的两个 turn_context 读取实际沙箱、审批、网络和 cwd，并在 Worktree 核对 HEAD 及 Git 状态 | 恢复回合继续只读，使用同一工作区和原始版本 | 两回合均为 read-only、网络禁用、审批 never，cwd 均为该执行的 Worktree。HEAD 保持 `2db282232add7f7571f5ca60473573d16f83de10`，Git 工作区干净。 |
| 从任务 API 读取草案和执行结果，重启控制面并在 Chrome 展开进程记录 | 仅一份计划修订；重启可回读完整草案及两次进程 | 状态 PLANNING，修订 1 包含两个里程碑，草案等于执行回报中的计划，重启前后完整任务响应一致。Chrome 展示两个 PID 及同一 Thread。此次验收终点为计划草案，来源 Issue 仍开放。 |

### 专题 07、12、16 · 关闭来源 Issue 中断运行中的原生 Codex

ForgeOS 验收提交 `0ad5bdc144a78af0ec0c3958237d93c1e2faa312`。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在私有 beta #131 指定 120 秒的标题稳定性观察。正式 Worker 启动原生 Codex，读取页面后执行前台等待命令；此时提交同项目的 HEAD 检查 | 可确认是真实模型及其工具在工作，下一任务等待 | 执行 1 的 Codex PID 为 36468，Thread 为 `01a1e979-3667-74e2-a4b5-ac013235ea6a`。已定位其运行中等待命令的子进程 11996，任务 2 为 READY。 |
| 从 GitHub 将 Issue 关闭为 not planned，重启控制面并启用项目轮询，从 API 观察登记和结算 | 先记录来源关闭，随后停止本次执行并释放项目名额；取消的候选不能交付 | 记录来源关闭时任务仍为 RUNNING。随后任务 CANCELLED，只有一次执行，退出码 1，回执原因是来源 GitHub Issue 已关闭。原生进程及已观察到的工具子进程均已退出。任务没有候选 SHA 或交付记录。[Issue #131](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/131) |
| 等同项目下一任务完成，再重启控制面回查 | 归还名额后新任务可用独立工作区执行，原任务继续保持取消 | 任务 2 为 DONE，返回仓库 HEAD `2db282232add7f7571f5ca60473573d16f83de10`。原任务重启后仍为 CANCELLED，事件依次为 issue_imported、execution_claimed、source_cancel_requested、source_issue_cancelled。 |

### 专题 06、07、16 · Owner 从 Chrome 取消运行中的模型和工具

ForgeOS 验收提交 `b5c6217c65e59613b4112e191afd4d0f4b4f351c`。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 导入 beta #132，由正式 Worker 运行 120 秒标题观察。在工具等待期间提交同项目 HEAD 任务，从 Chrome 任务页点击“取消任务 · 执行 #1” | 取消绑定当前执行，后续任务在名额释放前等待 | 任务 2 初始为 READY。原生 Codex PID 17088，Thread 为 `01a1e97a-ff5c-7dd0-ae1e-b0ed113f550c`，工具子进程 PID 32240。 |
| 等待取消回执，核对操作系统进程、任务及 GitHub | 模型和在途工具均退出，任务取消，保留来源 Issue | 两个 PID 均已退出。任务 CANCELLED，仅一次执行，回执原因是 Owner 请求停止。无候选和交付，Issue 回读为 OPEN。[Issue #132](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/132) |
| 等待任务 2，再重启控制面查询原任务 | 下一任务取得名额，取消状态持久化 | 任务 2 为 DONE，返回 HEAD `2db282232add7f7571f5ca60473573d16f83de10`。原任务仍为 CANCELLED，事件依次为 issue_imported、execution_claimed、owner_cancel_requested、owner_cancelled。 |

<a id="check-b-1"></a>

### 专题 05、07、16 B.1 · 普通 Worker 角色配置 v1

ForgeOS 实现及验收提交 `5f8a4ebbcd08d082ffe7bbba030a851b25fe91d0`。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 启动已安装 Codex 与 Claude 的普通 Worker，在 Chrome 打开“执行节点”，读取同一节点的 API。用 beta #19 启动 Planner，核对本次 Worktree 中运行的原生进程参数 | UI、API 与 Planner 实际使用的权限及沙箱一致 | 页面显示三个角色配置。Codex Planner PID 25040 使用 read-only 和 never，任务进入 PLANNING。控制面重启后配置相同；同一节点换成旧 Worker 后，旧心跳撤下配置。 |
| 在两个隔离库分别从 beta #133 派发 Claude 和 Codex 实现。核对其运行参数，再检查本地验证、候选差异和文档内容 | 两种角色仅使用声明的权限；均生成指定文档 | Claude PID 40564 使用 acceptEdits，工具为 Read、Glob、Grep、Edit、Write，候选 `9e360fa515202b0e3f88acbea23fc69528cd659f`。Codex PID 21508 使用 workspace-write 和 never，候选 `02fd6ab22f16737bda508b5bf5f61385e77610c3`。两份候选各只新增一个文件，唯一 H1、来源路径、页面标题及本地验证均通过。 |
| 批准 Codex 的固定候选，等待必需 CI 并回读 GitHub；重启控制面 | 交付使用批准的 SHA，节点角色配置可回读 | 必需检查按验收工作流首轮失败、重跑通过。PR #134 已合并为 `3839cde3cb4514141b02cedc766ee68900741dd3`，Issue #133 已关闭，任务 DONE。控制面重启后三种配置与原心跳相同。[PR #134](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/134) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44086353069) |

先前以已有 #19 测试 Claude 实现，回报“相对基础提交没有变更”，该次没有生成候选。上表的实现验收使用明确要求新增文件的 #133。带 PostgreSQL 18 和 Docker 的回归通过 239 项 Go 测试及子测试，零跳过。`go vet ./...` 与 Web 构建通过。

### 专题 06、07、16 · Ubuntu Worker 取消同一进程组的工具

ForgeOS 修复及验收提交 `75391e5294f49cf609008663fd665c2a4be77bfa`。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 用 Ubuntu 的非 root systemd Worker 导入私有 beta #132。在工具进入有时限的等待后，从 Chrome 取消任务，再核对 Linux 进程和下一任务 | 回报 CANCELLED 时，等待工具也应停止 | 修复前，适配器 PID 3871，工具 PID 3872，进程组 3773。任务已取消，工具仍处于睡眠态 S。下一任务已完成。 |
| 使用修复后的 Worker 重跑同一路径，按父 PID 和进程组核对，再从控制面查询后续 HEAD 任务 | 取消仅作用于本回合，Worker 继续派发 | 适配器 PID 4144，工具 PID 4145，进程组 4144。取消回执出现时该工具的 /proc 记录已消失。任务没有候选或交付；下一任务完成，返回 HEAD `3839cde3cb4514141b02cedc766ee68900741dd3`。 |
| 以专用 Linux 用户分别从两种适配器的回合入口取消，检查真实子进程 | 相同的中断规则适用于两种适配器 | 修复前 Codex 与 Claude 两例均失败；修复后连续三轮共六例均通过。 |
| 在 Windows 用真实 Codex 和 beta #132 从 Chrome 复跑同一取消入口 | 原生模型与前台工具退出、后续任务可运行 | PID 39984 与工具 PID 14628 均已退出，任务 CANCELLED，后续 HEAD 任务 DONE，重启后仍能回查取消事件。 |

systemd、Worker、GitHub 来源、Chrome 操作及 Linux 工具进程均为真实环境。Linux 回合使用有时限的本地测试程序，以便分别复现适配器退出和工具继续运行。专用账号登录供应方后的实测仍待验收。

Windows 的 PostgreSQL 18 和 Docker 全包回归通过 239 项 Go 测试及子测试，零跳过。`go vet ./...` 通过。

<a id="check-b-3"></a>

### 专题 05、06、16 B.3 · ACP 将 Owner 指令送入下一原生回合

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在 alpha #32 和 beta #135 的实现回合运行期间从 Chrome 排入不同的副标题，重启控制面，再次提交原请求和冲突请求 | 原请求可安全重放，变更同键正文被拒；接收节点只处理绑定本次执行的指令 | 原请求 204，冲突 409，节点调用 Owner 写入口 401。两任务的检查点 1 各有一条 COMMAND，检查点 0 和 2 均为空。跨任务或不存在的检查点返回 404，节点读 Owner 接口为 401。 |
| 从工作台打开“查看指令消息”，对比原生会话输入、候选文件、会话与检查点；重启后再查 | 消息 ID、收件人、指令正文和实际生效的副标题一致，同一会话继续执行 | Codex 收到 `EXEC-1-CHECKPOINT-1-COMMAND-17e9c721-21f3-4122-bd99-d5ada17a6066`，候选中有 H2 “Harbor 4adbd1c6”。Claude 收到 `EXEC-1-CHECKPOINT-1-COMMAND-1a591f37-95f4-459c-a28a-712b0bd65f31`，候选中有 H2 “Harbor a1b64757”。两份原生输入均包含该次消息 ID 与指令；浏览器编号与其一致。两个节点分别为 question-codex、question-claude，两次重启回读均一致。 |
| 批准各自的固定候选，核对必需 CI 和交付，再用新接口回读两个早期任务 | 两个项目独立合并；历史队列的原文和目标继续可查 | alpha 候选 `94b69e2bab668617f5b4a3d9375e86aa514c3721` 合并为 `1d5829fb2f05ef95575cc153950a2c30476edc18`；beta 候选 `fb127e1304de3523cbff8e30ef3d7ab8a4c91e9f` 合并为 `11b38381531c3fdb2eaae43b195abcf7875d3e73`。两项必需 CI 重跑通过，Issue 均关闭。早期 alpha #30、beta #121 的请求原文和节点均可回读。[alpha PR #33](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/33) · [alpha CI](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/44088504983) · [beta PR #136](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/136) · [beta CI](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44088725108) |

PostgreSQL 18、Docker 回归通过 240 项 Go 测试及子测试，零跳过。已验证篡改来源、收件人、Goal 或正文以及遗漏消息会被拒绝。Go vet 与生产 Web 构建通过。

上一项 ACP 下一回合指令对应 ForgeOS 提交 `ae0a57a2b178f8a3b9a4c2c4cca3fcdf5b7bc1cb`。

### 专题 06、07、16 · 暂停实现回合并在原会话继续

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在 Chrome 打开 beta #137 对应的任务。等 Worker 写入 Markdown 草稿、执行前台 PowerShell 等待时请求暂停 | 只有 Owner 可暂停当前实例，模型与工具退出，保留未提交的草稿 | 错实例 409，Worker 身份 401，Owner 请求及重放均为 204。21:28:55 请求，21:28:56 进入 PAUSED。Codex PID 10200 和工具 PID 4840 均退出。索引树 `c793aa68f073c912dfcb0f31ced9d5c52c64c6cc`，含草稿的工作区树 `fbf39c309e5e8df6385bfdcabcb17b5671cbf860`。 |
| 重启控制面，用同节点规划 alpha #2，再从任务页恢复 beta | 暂停记录可回读；第二个项目可用唯一模型名额；原任务继续之前的草稿 | 重启前后暂停记录相同。节点与共享模型占用均降为 0，规划任务占用 1 并完成第 1 版草案。恢复及重放均为 204。执行仍为 1，Worktree 及 Thread `01a1e9b1-de30-7c43-a383-42b625c443c4` 相同，恢复后 PID 47740 正常退出。三类暂停与恢复事件各一条。 |
| 核对文档再批准固定候选，等待必需 CI、合并和关闭 | 只变更指定的 Markdown 文件，保留原 H1，补上实际页面标题和来源路径，按原门禁交付 | 文档检查退出码 0。候选 `d73c1e3a595984a9e42740e9f7f88a302fc5cb6a`，必需 CI 按验收仓库约定重跑成功，合并 `d15de1fe46e5c59966f52100acd9d9bc81726684`，#137 已关闭，任务 DONE。[PR #138](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/138) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44093499303) |

首轮浏览器验收发现前端请求路径校验拦截了暂停操作。修复后通过上述浏览器流程。PostgreSQL 18、Docker 全量 Go 回归共 241 项通过、零跳过；补测旧进程回报重放，下一资源阶段保持可用。Go vet、TypeScript 检查和生产 Web 构建通过。

ForgeOS 实现回合暂停提交 `b2e52c6db286af42aa212a9835cbe0436ac6f6a9`。

### 专题 06、07、16 · 从工作台暂停只读 Planner

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在独立库导入 beta #19 做只读规划。等真实 Codex 完成一次工具调用后，在 Chrome 暂停，重启控制面再恢复 | 释放名额、保留检查点，两个回合同为只读 | 节点的模型占用降为 0。索引和工作区树均为 `0d8951c98cc5d29ebdcc31be1eec49fdcad966a9`。两次进程 PID 30004、6096 使用同一 Thread `01a1e9c9-b187-7fd1-8add-778010613132`。两份原生 turn_context 均为 read-only、禁网、审批 never，cwd 均为原 Worktree。 |
| 读取计划和进程记录，再次重启控制面并从 Chrome 展开结果 | 只有一个执行和一版草案，仓库保持干净 | 任务处于 PLANNING，修订 1 含两个里程碑。执行 1 的回报与草案相同，原始 HEAD `d15de1fe46e5c59966f52100acd9d9bc81726684` 和空 Git 状态已核对。重启前后任务一致。[来源 Issue #19](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/19) |
| 另一次暂停后在原 Worktree 新建文件，再从 Chrome 恢复 | 保留现场，在新模型进程启动前拒绝执行 | 任务 FAILED，报告检查点 0 的工作区或索引已改变。仅有原 PID 47112，保留新增文件，无草案及候选。 |

第一轮在 Thread 刚登记时便暂停，原生记录仅含一份 turn_context；权限验证采用上表中完成一次只读操作后的复跑。PostgreSQL 18 和 Docker 的 242 项 Go 测试及子测试全通过，零跳过。旧、新 Planner Worker 的能力协商用例和 Go vet 均通过。

只读 Planner 暂停实现及验收提交 `b2aa5ce0c8499758dc4d404cb7b116873750b0cf`。

### 专题 06、07、16 · 从 Chrome 暂停和恢复真实 Claude Code

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 从私有 beta #139 派发三个文档来源的交接说明。Worker 完成 Glob 工具调用后，在 Chrome 暂停 Claude，重启控制面，再选择恢复 | 释放模型名额，保留会话、工作区和执行，只产生一次恢复事件 | 错实例 409，节点身份 401；两次 Owner 操作及重放均为 204。节点及共享名额降至 0。首个 PID 39748 退出，恢复后 PID 4840 正常退出。执行均为 1，Session 均为 `ae892d7f-2529-498b-af21-c4446c931e8f`。 |
| 读取固定候选的 Git 差异和本地验证，批准并跟踪必需 CI 与交付 | 只增加指定的交接文档，核对唯一 H1、三条实际路径、页面标题和原 Issue 的交付回执 | 候选 `8b4977635c658f01847d0ee5eb3a3074d2fe230f` 仅新增 planning-handoff.md，三条来源路径和 Queue handbook 标题均存在。文档验证退出码 0。必需 CI 第二次成功，合并 `d242f11ae8e07fd64a4ff32a9301d418a5844904`，任务 DONE，#139 已关闭。[PR #140](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/140) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44098005667) |

此次真实 Claude 验收复用了 `b2e52c6db286af42aa212a9835cbe0436ac6f6a9` 的实现回合暂停路径。

Claude 暂停至 GitHub 交付的验收提交为 `402936239a5c25e1412c93239878bb385fde5746`。

### 专题 06、07 · 从任务详情核对多次模型暂停记录

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在 beta #19 的同一只读规划执行中，从 Chrome 暂停和恢复两次，每次暂停后都重启控制面 | 每次检查点独立保留，迟到的更新不覆盖前一次 | 实例 1 在 21:59:21 暂停，21:59:22 请求恢复；实例 2 在 21:59:26 暂停，21:59:27 请求恢复。三个进程 PID 依次为 13108、26420、39640，同一 Thread `01a1e9ce-14af-7f80-942d-b64e5878b3a9`，任务回到 PLANNING。 |
| 重启后从 Chrome 展开“模型暂停记录（2）”，再在已交付的 Codex #137 和 Claude #139 库中打开原任务 | 两次请求及两组工作区证据按时间顺序可见，历史交付保留其实际记录 | 规划任务显示两次会话和四个树值，均为 `157b6d627d8053bbc76c45f88abd4fc2a5630650`，三种生命周期事件各两条。两个已交付任务各显示一条记录，Session、工作区树和索引树均与原验收回执一致。 |

PostgreSQL 集成测试覆盖首次暂停已恢复、第二次只发出请求时的历史顺序和当前实例。Go vet 和 Web 生产构建通过。

任务详情展示历次模型暂停的提交为 `b40b109da1f187e2e81481d16c4f58c1dedf239f`，该版本的全包回归 242 项通过，零跳过。

### 专题 06、07、16 · 暂停只读规划时 Worker 进程退出

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在 beta #19 的只读回合从 Chrome 暂停，强制结束 Worker，保留控制面与原节点状态目录，启动替代进程后恢复 | 替代进程应能从原检查点继续 | 修复前的 Worker 47392 退出后，43388 没有接回规划；恢复请求后观察 12 秒，任务仍为 RUNNING，模型名额为 0，无新进程或计划。 |
| 在修复版再次暂停，重启控制面，结束 Worker 24424，同时启动使用原日志的两个 Worker，并确认它们都读到执行 1 的检查点 0，再从 Chrome 恢复 | 两个进程争用时只能产生一份草案；恢复回合继续只读 | 替代 Worker 46716、38216 均进入恢复，只有一方完成。两个原生 PID 43748、35156 的 Thread 均为 `01a1e9d9-c0ed-7502-8040-49c233703f5b`。两份原生上下文均为 read-only、禁网、审批 never、原 Worktree。任务 PLANNING，修订 1、两个里程碑，重启后逐字段一致。[来源 Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/19) |
| 在另一隔离库重复暂停并结束 Worker 44812，等待两个替代进程；排入同项目检查，再从任务页取消规划 | 不重启模型、不生成草案，归还项目名额并执行排队任务 | 任务 CANCELLED，只有原生 PID 46004，一条取消事件。检查任务 2 从 READY 到 DONE，返回项目 HEAD `d242f11ae8e07fd64a4ff32a9301d418a5844904`。重启控制面后取消记录相同。 |

节点回归验证已完成草案与暂停请求相遇时，重启可回读原草案。PostgreSQL 测试覆盖同一领取者重报、另一个进程竞争和缺少领取身份。PostgreSQL 18 与 Docker 全包回归 243 项通过、零跳过，Go vet 通过。此次强制中断针对暂停期间的 Worker 进程；主机重启、领取恢复后再次中断、写入型实现的暂停接回继续待验。

暂停的只读规划在 Worker 重启后接回，对应 ForgeOS 提交 `707765d223c05ce8c98eb8aa01aae4725c71b692`。

### 专题 06、07、16 · 暂停写入任务时 Worker 进程退出

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在 beta #141 写入首行草稿并启动限时 PowerShell 等待后，从 Chrome 暂停。结束原 Worker，启动替代进程再恢复 | 替代 Worker 应从原检查点继续 | 修复前 Worker 40916 退出，40496 接管节点后观察 10 秒，模型占用为 0，任务仍为 RUNNING。 |
| 在修复版重复相同操作，重启控制面、结束 Worker 43568，启动使用同一状态目录的进程 45756 和 24700，再从 Chrome 恢复 | 只有一个进程接回，保留草稿、工作区、会话和执行身份 | 首次 Codex PID 9744 和工具 PID 33276 均已退出。两个替代 Worker 都读到检查点 0。恢复后只有原生 PID 40428，执行仍为 1，Thread 仍为 `01a1e9ea-2033-7aa3-9650-341f1ee5cc90`。未提交工作区树为 `4502e86dbf9c54108aaf6165529a8ca10666829b`，首行草稿字节相同。 |
| 核对候选、批准和交付；在独立运行中于恢复前修改草稿 | 正常候选只含要求的文件，修改现场时停止 | 候选 `b7c7d41197e035293494ee6d929e5cc776eb9271` 只新增 daemon-check.md，唯一 H1、页面标题和来源路径正确。文档验证和必需 CI 通过，合并 `a20b40671bdfc36b376b7e14663937eb55aab3dc`，任务 DONE，Issue 关闭。改动草稿的一轮为 FAILED，只有原进程，保留外部修改，无候选或交付。[PR #142](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/142) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44104150073) |

PostgreSQL 18 与 Docker 的 244 项 Go 测试及子测试全部通过，零跳过；Go vet 通过。节点回归覆盖已完成模型回合产生的业务问题，恢复后沿用原序号、问题、依据和 Session。

写入任务在 Worker 重启后接回原回合的 ForgeOS 提交为 `58a5e5fa730eec1c3e60d97145827df98970ded0`。

### 专题 06、07、16 · Claude 在后续检查点接回同一条 Owner 指令

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 从 Chrome 向 beta #143 的首轮排入指令，要求在新增阅读指南中加入指定 H2。等第二个原生进程完成 Read 后暂停、重启控制面、结束 Worker 4380，启动 Worker 21076 | 节点应接回本次实际检查点 1，首轮已写入的草稿和第二轮的指令继续生效 | 原生进程依次为 4148、45988、45196。Worker 日志确认接回检查点 1。三个进程的 Session 均为 `fe3e624a-2b57-4617-a7d9-86319d8d48a4`。恢复前后两条原生用户输入都包含本次 Owner 指令，后者含恢复说明。 |
| 核对检查点、候选及 GitHub 交付 | 同一指令只登记一次；最终文档只生成一个指定 H2，通过原有门禁 | 检查点 0、1 均确认原 Session，2 无 Session。候选 `5b36b55332a1996f0af3ac3e9f9b0ac6bfa5092b` 仅新增 restart-reading.md，唯一 H1、三个来源及页面标题正确，H2 “Handoff 9b456efb” 只出现一次。必需 CI 第二次通过，合并 `31e6fff90796c6766f2007fd98b76ba6fe7dbda1`，任务 DONE，Issue 关闭。[PR #144](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/144) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44104899278) |

Claude 后续检查点恢复验收提交为 `42138283560573ad02e873f917384e091ea3883e`。

### 专题 07、16 B.4 · 从真实客户端识别丢失的会话

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在 beta #19 的 Codex 只读规划期间从 Chrome 暂停，替代 Worker 使用空的 CODEX_HOME 和原节点日志，恢复原 Thread | 客户端应明确指出所请求 Thread 不在当前会话目录；任务页保留具体原因 | 原 PID 13344，Thread `01a1e9f3-5a29-71b1-8fb3-57fa338a74e5`。恢复 PID 39108 在确认 Thread 前以 1 退出，Codex 0.169.0-alpha.11.1 返回指向该 ID 的 no rollout found 错误。状态 FAILED，无草案或候选。 |
| 在独立库对 Claude 实现回合重复上述操作，替代 Worker 使用空的 CLAUDE_CONFIG_DIR。重启控制面再从 Chrome 回查 | 仅与所请求 Session 匹配的缺失回执能触发相同诊断 | 原 PID 47740、恢复 PID 48252，Session `379fe16a-90ee-45b1-aaee-0abac0fd5d8e`。Claude Code 2.1.310 的 error_during_execution 指出了相同 ID，退出码 1。两次任务回读均为 native harness could not find the requested session，无交付。 |

使用空目录模拟替代节点缺少原会话，未改动用户的原生会话文件。新增回归覆盖不同 ID、未请求恢复、错误类别不匹配以及已确认 Session 后的普通模型失败。此次验收终点为区分失败原因；创建新会话的交接另行验证。

原生缺失诊断提交为 `2e413fbfd924d0fea9bf182c64b7c2734b72db9c`。该版在 PostgreSQL 18 和 Docker 上通过 245 项 Go 测试及子测试，零跳过。

### 专题 06、07、16 B.4 · 只读 Planner 在原 Thread 缺失后交接

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 对 beta #19 启动真实 Codex 规划，首次调用使用 `--ephemeral`。在 Chrome 暂停后替换 Worker，使用原登录和正常客户端恢复 | 原 Thread 未持久化；先尝试原 Thread，只有明确缺失才可交接 | 首次原生 PID 3512，Thread `01a1ea05-94b2-7ec3-b746-d970197e231d`。Worker 从 42272 换为 36080。恢复进程 30844 在确认 Thread 前以 1 退出。 |
| 核对下一进程的原生输入、权限和实际工作区，等待草案 | 新 Thread 沿用同一执行、固定提交、Issue 和权限，只产生一版计划 | PID 47084 在新 Thread `01a1ea05-b471-7391-84d4-53f625d67939` 成功结束。原生输入含原 Thread、Issue 标题和基线 `31e6fff90796c6766f2007fd98b76ba6fe7dbda1`。实际上下文为原目录、只读、禁网、无需交互审批。Git 状态干净，一个执行产出计划修订 1，含一个里程碑，进入 PLANNING 等待审核。 |
| 重启控制面并从 Chrome 展开任务的 Harness 进程记录 | 原进程、恢复失败和新进程均可回查，并显示交接来源 | 页面保留三条记录；新进程的“交接来源会话”与首次 Thread 一致。[验收 Issue #19](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/19) |

PostgreSQL 18 和 Docker 上全量 Go 回归通过 251 项测试及子测试，零跳过。Go vet 和 Web 生产构建通过。回归还核对了租约失效、工作区有外部编辑、旧控制面以及普通模型报错时的处理。

只读 Planner 跨 Thread 交接的 ForgeOS 提交为 `331fe16ef9fc012b2ca5b9b1b654685d77a7a7c8`。

### 专题 06、07、16 B.4 · Codex 写入首轮从暂停草稿交接

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| beta #145 首次启动 Codex 时使用 `--ephemeral`。等目标 Markdown 出现唯一 H1，从 Chrome 暂停、替换 Worker 后恢复 | 原 Thread 缺失时仍可核对并接回已写草稿 | 修复前，原生 PID 12212 退出，恢复 PID 20280 报告原 Thread 缺失，任务 FAILED。原草稿保留，无候选。 |
| 修复后重跑相同 Issue 与基线，检查暂停时的 Git 树，再检查新 Thread 的真实输入与权限 | 新回合应取得原执行的范围和草稿，沿用原工作区；重复写入或草稿丢失都会导致验收失败 | Worker 5972 替换为 8740。原生 PID 42836、缺失恢复 PID 11992、新回合 PID 18704。Thread 从 `01a1ea12-6c23-7061-ac99-e34461a1c345` 交接到 `01a1ea12-da1f-7b21-85fe-dacc72192800`。新输入含原 Thread、基线、Issue 标题、暂存区树 `c57be9b059492be5192629be68ea4ad94a7340fb` 和工作区树 `5dc16eda8c331dafd118f54ae1bc4cea1483bfed`。原生权限为原工作区可写、禁网。 |
| 核对候选并批准，通过必需 CI 后回查 GitHub | 一个执行只交付一个目标文件，文档保留唯一 H1、正确页面标题及来源 | 候选 `046f6655367c85601b6ad77ffc74986576f70ffb` 只新增 thread-handoff.md。文档验证及必需 CI 通过，合并 `8f59f93b900ef16968fc1a5498f73a85cb7ae0e5`，任务 DONE、Issue 已关闭。[PR #146](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/146) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44112298300) |
| beta #147 在首轮暂停前从 Chrome 排入一条 Owner 指令。交接后读取原生回合输入、检查点和提交内容，再批准候选 | 当前回合接回草稿，下一回合收到排队指令一次 | 原生 PID 46868，缺失尝试 28256，交接 PID 37156，处理指令的 PID 47840。原 Thread `01a1ea16-8290-7341-999f-f69e44611f3a`，新 Thread `01a1ea16-e7e2-7b62-9262-d250617fe551`。检查点 0、1 确认新 Thread，2 尚无会话。原生输入中解码出一次完整指令，候选 `a68f7a46941a8a68df155af4d0d5c02086cdbf03` 中只有一个指定 H2 “Review 71ccb32b”。必需 CI 通过，合并 `b3036679221a1079785e7085d3de1a209e7d8e69`，任务 DONE、Issue 已关闭。[PR #148](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/148) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44113316900) |

最终版本在 PostgreSQL 18 和 Docker 上通过 252 项 Go 测试及子测试，零跳过，Go vet 通过。回归另验了已暂停草稿被外部改动、来源 Session 无关、来源进程实例不匹配时拒绝交接。

Codex 首轮写入从暂停草稿交接的 ForgeOS 提交为 `4916e84dfb5779f90ff5c7e4ea2ba74b92231bae`。

### 专题 06、07、16 B.4 · Claude Code 首轮写入从暂停草稿交接

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| beta #149 的首次 Claude Code 2.1.310 调用使用 `--no-session-persistence`。从 Chrome 排入一条 Owner 指令，在只写入 H1 时暂停，重启控制面并更换 Worker，再从任务页恢复 | 保留已写草稿。正式客户端先尝试原 Session，确认缺失后按原权限从新 Session 接回 | 初始原生 PID 35900，原 Session `f85c8b3a-8b37-41c3-850c-b036a829d97f`。Worker 从 43268 换为 25328。恢复 PID 29512 以 1 退出；新 Session `deaa5e3d-b0fe-4f3e-a518-e648e234acdd` 的 PID 为 25868。新进程在相同目录使用 restricted、safe-mode、acceptEdits 和 Read/Glob/Grep/Edit/Write，原生输入含原 Session、基线和两棵暂停时的 Git 树。 |
| 待下一回合完成，核对检查点、原生输入、候选文件和来源 HTML | Owner 指令只排入及执行一次。标题、引用和内容应逐项与来源一致 | 检查点 0、1 均确认新 Session。PID 35748 处理下一回合，原生输入包含一次完整指令，文件也只含一个指定 H2 “Review c7516795”。首次候选 `13d672d59bdc7e786133bad25b624d6e9065016a` 把页面标题写成 Queue preview handbook，还列出了 HTML 中不存在的链接。文档验证退出码为 0，内容验收退回该候选。 |
| 在退回意见中列出来源标题、等待容量正文和多写的链接。核对第二次执行产生的提交和实际 GitHub 交付，重启控制面后从 Chrome 展开第一次执行 | 修订保留同一个 H2、三个来源和原文件范围。已退回的候选及其会话交接仍可回查 | 新候选 `882901b46c2444086433bd95550ca9eae8a9cdc8` 只新增 session-review.md，标题为 Queue handbook，等待容量说明正确，多写的链接已删除。必需 CI 通过，合并 `bdcada9f771b660f5f815931e47dbf936310ff7e`，任务 DONE、Issue 已关闭。Chrome 可展开原执行的四条进程记录并回读交接来源。[PR #150](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/150) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44115294612) |

PostgreSQL 18 和 Docker 上的全量 Go 回归通过 253 项测试及子测试，零跳过；Go vet 通过。新增回归验证 Claude 跨会话必须有完整的来源和失败恢复记录，交接后的普通回合必须延续新 Session。

Claude 首轮写入从暂停草稿交接的 ForgeOS 提交为 `de6868b317fc1e2678e0e639640a04b1d4ba1251`。

### 专题 06、07、13、16 B.4 · Claude 检查点 2 交接已处理的指令和答复

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| beta #151 从首轮排入指令，经问题页回答 H1，再排入第二条指令。在检查点 2 从 Chrome 暂停并替换 Worker。仅对原 Session 的恢复调用，将真实 Claude CLI 指向一个没有该会话的目录 | 原会话缺失时，任务应保留此前的草稿和答复，继续完成当前指令 | 修复前，恢复 PID 21064 返回原 Session 缺失，任务 FAILED，无候选。验收只改变指定恢复调用的会话目录。首次复测中模型未按结构化格式提问，该轮未计入通过；随后在测试 Issue 中明确了问题格式。 |
| 用修复版复验，先通过 GitHub 轮询收录评论，再重复上述操作。检查新会话的原生输入和三个已确认检查点 | 已完成回合的讨论、指令和答复均可辨认，当前回合仍使用暂停时的指令 | Worker 47360 更换为 41476。检查点 2 原进程 21988，缺失尝试 17140，新进程 20100。新 Session `f7efaeec-b192-4c1c-8d9b-615a890afa57` 的原生输入含检查点 0 的评论及 GitHub 来源链接、检查点 1 的完整指令和业务答复、原请求和答复的消息 ID。检查点 0、1 的 Session 均为 `be194d37-00d4-4b0a-88ac-3cf303d6bf42`。当前指令、基线和两棵 Git 树均匹配。[来源评论](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/151#issuecomment-6953126040) |
| 核对候选规则记录、Git 内容、批准与交付，重启控制面后从 Chrome 回读 | 两条指令应分别关联到处理它们的 Session；保留 H1 和两个唯一 H2，只交付目标文件一次 | 候选 `64aa63f3551ce81bfdfb0107f2f5f1c2990bef9a` 仅新增 checkpoint-review.md。H1 为 Owner 指定的 “Queue a98bb5f8”；H2 “Sources 08b56c34” 与 “Review d06515ab” 各一次，页面标题和两条来源路径正确。候选规则记录分别关联检查点 1 原 Session、检查点 2 新 Session。必需 CI 通过，合并 `29b3e8caffabf9b89206485d5157ff08c8e62ce7`，任务 DONE、Issue 关闭。任务页保留五条原生进程记录。[PR #152](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/152) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44119628210) |
| 对已有 beta #149 数据的 PostgreSQL 18 启动新控制面，将模式从 88 升至 89 | 已有交付、退回记录和审核依据仍可回读 | 升级后的任务 API 完整响应和两次执行的候选规则快照均与升级前一致。 |

PostgreSQL 18 和 Docker 上全量 Go 回归通过 255 项测试及子测试，零跳过，Go vet 通过。回归覆盖同一检查点的两次连续交接、重复上报、来源进程不匹配、缺失最终回执，以及 Management 答复的原消息身份、检查点和决定适用范围。实际 Management 回合与后续 Codex 检查点的跨会话交接另列待验。

写入任务恢复较早检查点的讨论、指令与答复的 ForgeOS 提交为 `076dda306f870e8b41919e2a48cdd2406b0e4566`。

### 专题 05、06、07、13、16 B.4 · Codex 检查点 2 交接独立 Management 的 D1 答复

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 2026-11-18 在 beta #153 确认项目决定 1，适用范围限定为 decision-review.md 的标题。Codex 在检查点 0 请求查询；从 Chrome 分两批排入指令，并由独立 Management 进程查证决定 | Management 与实现 Agent 各有原生身份。被引用的决定应保留原文、适用范围和回答方 | Management Session `01a1ea3d-7a91-79b0-a69d-50ed5814c789` 返回 D1 答复，`answered_by=management`。初始实现 Thread 为 `01a1ea3d-4ea5-7111-b1a0-ff2cafa8b70e`。草稿保留决定指定的 H1 和第一条指令的 H2。 |
| 在检查点 2 暂停并重启控制面、替换 Worker。只为恢复旧 ID 的真实 CLI 调用提供空会话目录，读取新 Thread 的原生输入和候选审核依据 | 新 Thread 应能区分 Management 引用和 Owner 指令，沿用项目范围与原沙箱 | Worker 从 41552 换为 448。原进程 23020、缺失尝试 48068、交接进程 43544。新 Thread `01a1ea3e-3df7-70e2-848c-ffb267d0db5d` 与前两种身份都不同；原生上下文为同一工作区、workspace-write、禁网。原生输入和候选规则记录都含同一 Management Session、决定 ID、原文及适用范围。两条指令分别关联到实现方的旧、新 Thread。 |
| 核对候选，批准后回查必需 CI、合并、Issue 和任务页历史 | 一个执行只交付指定文件，保留决定和两条指令的结果 | 候选 `0d336241dac4bea9a78c3016a484ac8f158aa4da` 仅新增 decision-review.md。唯一 H1 为 “Queue e8ed3446”，两个 H2 各一次，引用的页面和交接标题正确。必需 CI 通过，合并 `3a47d7c57010dc678410e357ca586083f2168e1f`，任务 DONE、Issue 已关闭。重启后 Chrome 仍可展开五条进程记录。[PR #154](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/154) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44121359700) |

上述 Codex 检查点 2 与独立 Management 答复的 ForgeOS 验收提交为 `9b8a480d31a83ecb62f27e479bbe6ac5962a04e4`。

<a id="adr-evidence-010"></a>

### 专题 06、07、13、16 B.4 · 等待 Owner 答复期间原生会话丢失

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 2026-11-18 在 beta #155 用真实 Claude 的 `--no-session-persistence` 写入来源路径并提出业务问题。进入 WAITING_INPUT 后终止 Worker，重启控制面，以同配置启动替代 Worker，在 Chrome 问题页提交标题 | 替代 Worker 应先恢复原会话，确认缺失后保留草稿和答复完成原执行 | 修复前原会话 `7a008d43-5808-4d92-b422-1174476d5c3f` 缺失，执行 FAILED，无候选。复测提问、失败的恢复和新会话的进程号分别为 45772、7716、26100，退出码为 0、1、0。新 Session `9c672fc6-b6f0-48f5-9612-9331ea84833a` 生成候选 `0cc693d5686c4d8512493a40c76404c1401843e5`，仅创建指定文件，保留来源路径、精确页面标题及 Owner 指定的唯一 H1 “Queue 366db49e13”。 |
| 在 beta #157 使用 Codex 的 `--ephemeral` 重复业务等待、替换 Worker 和答复，再核对原工作区、两次会话、问题领取和任务历史 | 同一 Execution 只提问一次，无需 Owner 暂停或再次答复 | 原 Thread `01a1ea4c-8475-7051-bac9-1076b45cc06f`，新 Thread `01a1ea4d-0a41-7700-9b49-c4d1510f5691`，进程号 3464、45256、22868。候选 `2540e62c3634d547c697ae2d8a1fe7866baa4b4d` 保留来源、页面标题及指定 H1 “Queue 1f74eba4e0”。两个案例均只有一次执行、一个已领取的答复，暂停记录为零，检查点依次记录旧会话、新会话和空的下一检查点。 |
| 对两个候选核对 Git 内容与验证结果，批准交付。确认必需检查的预设失败步骤，在原候选重跑；合并后重启控制面并从 Chrome 展开任务详情 | 候选、必需 CI、合并和关闭的来源 Issue 可互相核对；任务页保留三条进程记录 | 两次均为 DONE，必需检查成功，Issue 关闭。#155 合并为 `3c126fed3d98725516233cc8dd905d7f52aac4e4`，#157 合并为 `83243d3a7cf36371098362d52e5263a45f10ff0b`。[Claude PR #156](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/156) · [CI](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44123738162) · [Codex PR #158](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/158) · [CI](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44124443875) |
| 将保存 beta #151、#153 的两个 PostgreSQL 18 schema 从 89 升级至 90，逐一读取原任务、候选审核依据及跨会话关系 | 历史任务和审核记录应能原样读取，原暂停交接仍可追溯 | 两个任务的 HTTP 详情、数据库审核快照及旧会话至新会话的查询结果分别与升级前相同。 |

PostgreSQL 18 和 Docker 环境中，全包 Go 测试及子测试 256 项通过，零跳过，`go vet ./...` 通过。新增回归覆盖未领取的答复、来源进程或退出码不匹配、重复确认、分叉会话及缺少最终进程回执。

上述业务等待恢复的 ForgeOS 提交为 `c1b48476423a6488debb4dfe431aaf1d0ad2279e`。

### 专题 06、07、13、16 B.4 · 未主动暂停的写入进程异常退出且会话丢失

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| beta #159 使用 Claude 的 `--no-session-persistence`，等目标文件出现指定 H1 后直接终止原生子进程 | Worker 先尝试恢复原 Session；若该 Session 缺失，应保持执行与草稿并完成交付 | 修复前原生 PID 6768 被终止，原 Session `4497e087-5a54-407f-834e-e6157df43fe5` 恢复失败，任务 FAILED。复测终止 PID 40312 后，三条进程记录为 41820、34316、14768。新 Session `9cef4dbd-91b5-4a7f-8e3d-065038ef0ebe` 保留了唯一 H1。逐项对照目标文件、HTML 标题、等待容量原句、两份交接文档及 Review 71ccb32b，候选 `baa2e626bffbca8124d4c677a4787dcd033f1446` 仅更改指定文件。 |
| beta #161 让 Codex 在草稿写入一次性 UUID，记录其值并直接终止原生 PID 21856。仅对恢复原 Thread 的 CLI 调用使用空会话目录。检查替代 Thread 的原生输入和工作区 | 新 Thread 应取得源进程回执、工具事件和两棵 Git 树。已有 UUID 只应生成一次 | 进程 42516、21508、27040，原 Thread `01a1ea59-2055-75a3-8001-35378c99e19d`，新 Thread `01a1ea59-a3ef-7763-b343-60dcb2f4e259`。新原生输入中的源进程回执与终态记录逐字段相等，运行于同一工作区、workspace-write、禁网。候选 `abff8721144297ed48b69d98901280510838b5ea` 保留原 UUID `a5bf7f29-f476-468e-8399-0ccdf0758d66`，出现一次，来源、标题和等待容量原句均正确。 |
| 核对两次任务的数据库交接、候选、CI 与合并。在控制面重启后从 Chrome 展开进程历史 | 无需暂停或提出业务问题；同一执行的原进程、缺失尝试及新进程都应可回查 | 两次均只有一次执行，无暂停、无业务问题，崩溃与缺失回执匹配最终历史。两个必需检查在验证预设失败步骤后于原 SHA 重跑通过，两个 Issue 都已关闭，任务均为 DONE。#159 合并 `6114d2205497635a8f25348f3df0bb8057048817`，#161 合并 `959ab0e9e3f25f38a4f05d4c22851c6865e8a8de`。[Claude PR #160](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/160) · [CI](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44126356582) · [Codex PR #162](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/162) · [CI](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44127225352) |
| 将真实任务 #151、#155、#157 的 PostgreSQL 18 schema 从 90 升到 91，对比任务详情、审核依据和会话关系 | 主动暂停与等待答复两类历史记录均应保持可读 | 三个任务的 HTTP 详情、审核快照与交接查询逐项相同。PostgreSQL 18 和 Docker 全包 Go 回归 257 项通过、零跳过，`go vet ./...` 通过。 |

上述未主动暂停的写入进程恢复对应 ForgeOS 提交 `a77cf1c24f6725bb3685d82ea83eb43ef713ceaa`。

### 专题 08、13 · 项目资料的混合检索与固定修订

在 PostgreSQL 17.11、pgvector 0.8.6 上启动正式控制面，用 Chrome 工作台及仅授权项目 1 的节点身份访问。推理服务为本机 CPU 容器 Hugging Face Text Embeddings Inference 1.8，镜像摘要 `sha256:8de25e75ce39617f17f2f6c77d60a4f75b65e779ed5005420eb1400072a15c1c`。验收权重为 `sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2` 修订 `e8f8c211226b894fcb81acc59f3b34ba3efd5f42`，每个向量 384 维。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在数据库版本 91 登记两个项目。向项目 1 导入英文断电恢复手册和午餐菜单，向项目 2 导入私有发电机资料。在 Chrome 搜索“断电后怎么办” | 能否用不同语言找到手册可与关键词基线对比 | 旧入口返回零条，搜索 `backup` 能找到手册。升级到 92 并配置本机模型后，同一中文查询首项为项目 1 的 `manual@1`，正文位置 `[0,40)`，SHA-256 为 `888c2a492f2faba5789c7a4ceb387a937bfc46796c37473a7b445d15749e9c70`，与原修订逐字一致。 |
| 保留搜索结果，在另一次写入中发布手册修订 2，再从 Chrome 打开原结果及“查看最新版” | 点击命中项能回到引用的版本，也能单独读取最新版 | 原结果仍显示修订 1 的断电恢复原文，“查看最新版”显示修订 2 的钢琴手册。按类型重新检索只返回最新修订。 |
| 新增另一种资料类型和一份长文。项目 1 的节点带 `kind=material` 搜索，再请求项目 2 | 模型和数据库均只处理授权类型及项目；引用应定位到命中的正文片段 | 节点首项为 `long-guide@1`，来源 `Incident handbook p. 4`，片段 `[480,572)`，原文 SHA-256 为 `49949511a8c4e8dda8ed1e4e1afb0b2aef88380dc5de52bfa96eb31df7238594`。跨项目返回 403。检查 11 个已保存的片段，项目 2 和被类型筛除的 `unread` 均无向量。 |
| 重启控制面，重复节点查询；再对一份未建索引的长资料同时发起两个请求，复跑双浏览器编辑探针 | 重启后可复用旧索引，并发查询保持同一结果。编辑和历史回读仍可使用 | 两次结果及缓存行的 PostgreSQL 事务标识完全一致。两个并发请求得到相同结果，新资料共 10 个不同起点的片段。长中文查询通过；两份 Chrome 会话验证同一资料修订 1–3 的冲突处理与历史读取。 |
| 在未配置推理服务的 PostgreSQL 18 升级既有 #151、#155、#159、#161 四个任务 | 可继续读取原有任务、候选审核及三类会话交接 | 四份任务 HTTP 详情、审核快照及交接查询与升级前逐项相同。 |

启用 PostgreSQL 18 和 Docker 的全包回归有 264 项 Go 测试及子测试通过，零跳过。`go vet ./...`、TypeScript 类型检查和 Web 生产构建通过。此项覆盖项目资料，其他知识来源的语义检索和角色上下文仍待验收。

上述项目资料检索对应 ForgeOS 提交 `ce6b5a25c81f61166dc10b61a37ce6fd5400f8df`。

### 专题 08、13 · 从检索结果复核已审核项目经验

使用上述 PostgreSQL 17.11、pgvector 0.8.6 和固定权重多语言模型。两份英文记录分别经原生 Codex Curator 提取、独立 Management 会话批准为 D1。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在 schema 92 搜索“断电后该怎样恢复”，升级到 93，分别在关键词模式和混合模式重试 | 无查询的浏览及审核回执保持兼容。混合检索能跨语言找到相关经验 | 升级前带查询仍按 ID 返回字体、断电两项。升级后旧浏览和两个回执逐字段相同。仅关键词时中文查询为空、`checksum` 命中断电记录。启用模型后同一中文查询在 Chrome 和授权节点均将 `power@1` 排首位。 |
| 在 Chrome 展开断电记录的审核回执，核对事实、引句和固定来源 | 引句与原修订标记位置逐字相同，并能区分提取和审核会话 | 事实 1 的 `[0,135)` 与原文一致，来源 SHA-256 为 `de02416f0a91814a63efe3a8e624f237bf5f48f415feeac4c0f7e5320f6f6439`。提取会话 `01a1ea6f-e72d-7080-a4e7-afce9eb08580`，审核会话 `01a1ea70-122b-7ee0-aa99-ac2ab3b9e790`。首次浏览器测试发现前端路径校验拦截了请求，修正后已从页面复测。 |
| 在项目 2 导入 UPS 演练记录，由该项目的原生提取与审核角色批准。先以项目 1 节点搜索，再以 Owner 搜索项目 2 | 仅编码授权项目的事实。两边都建好索引后仍应隔离 | 首次节点查询后只有项目 1 有向量。Owner 随后搜到项目 2 的 `battery@1`，正文 SHA-256 为 `5aba415c320170fc3c936177246ed7d464a6b4688e21aa59a8198d86e3baf818`。两项目均有缓存后，项目 1 节点仍得到原结果，对项目 2 返回 403。 |
| 重启后重复查询，再发布断电资料修订 2 | 向量可复用，旧回执可读，旧经验在当前搜索及浏览中退出 | 重启前后两条 384 维向量的事务标识及搜索结果相同。新修订发布后旧事实立即退出两种入口，历史回执逐字段相同。 |

启用 PostgreSQL 18 和 Docker 的全包 Go 回归 264 项通过、零跳过。确认决策的回归另核对待审核、已批准及被替代三种状态下的搜索结果。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

上述已审核项目经验检索对应 ForgeOS 提交 `cdb8f522f072de5ea76fae7bc73db0f80e73dc31`。

### 专题 08 · 节点按提交检索工程文档

在正式控制面登记的 alpha、beta 私有仓库中，通过 Owner 和仅获准访问 beta 的节点调用 HTTP 接口，再用独立 Git 命令按提交读取文件。原控制面对该节点的文档查询返回 404。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在 beta 默认 HEAD 搜索 `queue PREVIEW session review`，分别以 Owner、授权节点和显式提交查询 | 三种调用应返回同一固定提交。可根据路径和字节位置精确还原引文 | 三份响应一致，时间与 Git 提交时间一致。PR #160 合并提交 `6114d2205497635a8f25348f3df0bb8057048817` 的 `docs/queue-preview/crash-review.md` 命中标题，blob 为 `b755686df78c1a2758e9c17187653435e2798262`，区间为 `[2,30)`。引文及前后文与 Git 原始字节一致。[beta PR #160](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/160) |
| 指定同一项目的上一个提交，跨项目重试相同的查询；再传入另一项目的 SHA 与分支表达式 | 结果应对应所选项目和提交，越权请求应被拒绝 | 前一提交 `83243d3a7cf36371098362d52e5263a45f10ff0b` 仅命中已有的 `session-review.md`。Owner 可查 alpha 的 `1d5829fb2f05ef95575cc153950a2c30476edc18`，beta 节点访问 alpha 返回 403。用 alpha SHA 查 beta 返回 422，用 `HEAD~1` 返回 400。 |
| 在独立本地克隆提交跨越 64 KiB 的多字节文本和 32 份命中文档，再修改工作树并推进 HEAD | 字节位置应跨段连续，数量应有上限。提交前读原版本，提交后仍能按 SHA 回查 | 在 `2155d33d262eb2822bef3517f2e4622cd195267b` 的 blob `cb21587b92bd400af726316c9cdfb29b102873d0` 命中 `K界测量` 的 `[65535,65545)`。列表按路径返回前 30 份并报告还有结果。工作区草稿无法命中；提交 `0b59f7cd3ee4cd0865643fc6810df339bcc46ee7` 后新旧版本分别返回各自正文。 |

PostgreSQL 18 和 Docker 全包回归共 265 项 Go 测试及子测试通过，零跳过。`go vet ./...` 通过。仓库文档入口的回归还覆盖含 NUL 的二进制文件、符号链接及无效对象类型。

上述固定 Git 版本的工程文档检索对应 ForgeOS 提交 `9c5b31158fa99861443c4b0b32ad708cc62f6a94`。

### 专题 08、14 · 通过双向 TLS 网关管理全局规则

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 使用实际 Caddy 容器、服务端证书及客户端证书，分别经直连和工作台网关读取全局规则 | 同一 Owner 应能从两处读到同一版 | 修复前直连返回 200，网关的项目列表返回 200，网关的规则入口返回 404。给工作台增加精确路径后，直连和网关回执一致。 |
| 在 Chrome 通过 HTTPS 工作台发布一版规则、撤下规则并打开上一版，再从项目读取入口核对 | 发布修订可见，撤下后仍能复核旧版 | 修订 2 的正文 SHA-256 为 `fcaa8bbff01aded9ce7be1e007d3381335b0c3d05730a4064fbd12cc792dd239`。项目读到同一回执；撤下后得到修订 3。在页面打开上一版仍能读取原文。 |
| 分别省去客户端证书、Owner 令牌，并尝试从工作台域名访问内部节点及凭据重载路径 | 网关与控制面应分别执行各自的访问限制 | 无客户端证书时 TLS 失败，有证书无令牌时规则接口返回 401。内部节点读取和节点凭据重载路径均返回 404。 |

浏览器会话通过本机代理定位验收域名，使用临时客户端证书并忽略临时服务端 CA 的浏览器信任错误。独立 HTTPS 探针以该 CA 校验了服务端证书。外部 tailnet 设备访问仍待验收。

上述工作台网关的全局规则路由对应 ForgeOS 提交 `020a716e9ad627973f9188c732a6e54abcf235e9`。

### 专题 07、08、16 B.1 · 独立知识角色的会话权限配置

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 导入演练记录，让独立的 Curator 和 Management 完成提取、审核，分别观察其 Codex 进程参数与正式回执 | 两个不同的角色会话可以分别核对沙箱、审批和禁用功能 | 原版 PID 33348、45004 均使用只读沙箱，回执只记录会话。本次资料 `profile-77bddb17@1` 的 PID 42740、44260 分别对应会话 `01a1ea8c-7740-7f32-be04-de4452a4f898`、`01a1ea8c-9fbc-7d91-82c8-bb313c2f195b`。两个回执的 `read-only`、`never` 和有序的 13 项禁用功能逐一等于各自原生参数。 |
| 重启控制面，在 Chrome 从项目资料打开修订及“知识提取与审核”，展开两个角色配置，同时重读旧版资料的回执 | 可从各自会话查看原权限；旧版继承原记录 | 两个展开项均可见沙箱、审批、shell 工具及图像生成功能的禁用设置。新旧资料各自的完整 HTTP 回执与重启前逐字段相同。 |
| 交叉运行新 Worker 与旧控制面、旧 Worker 与新控制面 | 两种升级顺序均可完成提取和独立审核，不为未采集的回合生成配置 | 两轮各产生一对独立的原生会话，均获批准，两轮的旧格式回执均未携带权限配置。数据库回归验证缺失禁用项的配置被拒绝，幂等重报可复用相同回执，重报不能移除原配置。 |

PostgreSQL 18 和 Docker 全包 Go 回归有 265 项测试及子测试通过、零跳过。Go 静态检查、TypeScript 及 Web 生产构建通过。

上述独立知识角色会话配置对应 ForgeOS 提交 `32ca1e05e211d012316cd2eb3c8ab7496733c5ba`。

### 专题 07、10、16 B.1 · 独立代码审查的原生权限

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 从 beta #163 派发 Codex 实现，用旧版 `review-candidate` 审查真实候选，再让正式 Reviewer 领取同一候选 | 审查独立于实现。新报告应能核对实际审批与工具范围 | 实现 Thread `01a1ea90-8f00-7891-ac2a-a253fb7a6cd7`。旧审查会话 `e4b75b5c-9322-4643-bb30-0a03bb9cf424` 没有配置字段。新 Reviewer 的原生 Claude PID 46732 使用 `--permission-mode dontAsk --tools "" --strict-mcp-config`；正式会话 `ef0f882f-6c77-4a7a-a490-fea79495f6f6` 的回执记录 v1、`dontAsk` 和显式空工具列表。 |
| 重启控制面，Chrome 打开候选审查报告并展开权限配置，核对 HTML 来源、标题及等待容量原句后通过验收 | UI 应与原报告一致；只有经过审查的候选才能交付 | 重启前后报告逐字段一致，页面可见原生会话、`dontAsk` 和“可用工具 · 无”。候选 `d689d8e011d2c9e1e49e93cfa632d31b132c2228` 仅改动指定指南。Actions 首轮命中预设失败步骤、同一 SHA 重跑通过，`acceptance-check` 和 `forgeos/local-ci` 均成功。PR #164 合并为 `a5bcb4f72940bd864fab43abdfd9458716c853cc`，Issue #163 已关闭，任务为 DONE。[审查后的 PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/164) · [Actions 运行](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44139074764) |
| 从进入审查前的真实 PostgreSQL 快照分别启动新旧控制面及相反版本的 Reviewer；另恢复 beta #55 的旧审查快照 | 混合版本应能完成审查；既有报告不应推断权限字段 | 两个交叉组合的真实 Claude 审查均成功，保留旧格式。#55 执行 3 的旧报告、候选依据及会话 `9a961ecc-516a-491e-819f-2d358b537eff` 在升级前后逐字段相同。 |

PostgreSQL 18 与 Docker 的全包 Go 回归有 265 项测试及子测试通过，零跳过。报告验证覆盖缺失工具列表及扩大工具范围的错误配置。Go 静态检查、TypeScript 检查及生产构建通过。

上述独立代码审查的会话权限对应 ForgeOS 提交 `c7577c34f7222c6d5666795327c0a5676a40b62b`。

### 专题 07、08、09、16 B.1 · 源项目回读共享副本的审核权限

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 项目 2 为项目 1 起草数据库恢复经验的脱敏副本，由原生 Management 审核。对被退回的副本尝试发布 | 工作台应显示实际审核配置。退回的内容不能发布 | 旧版副本 1 已审批但只有会话 ID。副本 2 的标题含疑似内部编号，Management 要求修订，源项目仍能读取配置，按该副本摘要发布返回 409。副本 3 改用通用标题后获批。其 Codex PID 41708、会话 `01a1ea9b-f757-7240-afaa-349736a9bc67` 的 `read-only`、`never` 和 13 个禁用功能均与归档字段相同。 |
| 重启控制面，用 Chrome 打开源项目 D3 审核依据、展开权限配置、发布再撤销，同时以接收项目节点身份读取 | 独立审核通过后须等 Owner 发布；接收方只读取获批副本 | 发布前没有副本 3。发布后节点仅收到 ID、标题、脱敏正文、摘要、发布时间，摘要为 `3ae224e8e85931eedb3d2446ab108a0e011be29c32c3afc3dbb8d774f1475d70`。撤销后节点列表恢复原样；源项目仍可查看原审核及权限。 |
| 双向交叉运行新旧 Worker 和控制面，重读升级前的副本 1 | 混合版本能审核，旧记录保留原内容 | 副本 4、5 各获一份原生审核回执，均沿用旧格式。副本 1 在重启和升级前后逐字段相同。 |

PostgreSQL 18 和 Docker 全包 Go 回归 265 项通过、零跳过。Go 静态检查、TypeScript 及 Web 生产构建通过。

上述共享副本审核权限对应 ForgeOS 提交 `1e768608d4600123bd0a40670d359d27a81005aa`。

### 专题 06、16 B.1 · 固定 Goal 计划修订的独立审核权限

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 从 beta #165 请求原生 Planner，设置 D2 下限后由正式 Management 审核修订 1 | 两个角色有独立会话，审核配置来自实际原生进程；批准后等待 Owner | Planner Thread `01a1ea9d-558d-7ab1-8be6-054fabf2631e`。Management PID 6808、会话 `01a1eaa0-3167-78c2-b9cb-fc08cc4d2bf9` 的 `read-only`、`never` 和 13 个禁用功能与回执相同。审核结果 approve、有效等级 D2，Owner 操作前保持 PLANNING。 |
| 重启控制面，从 Chrome 打开修订 1 的审核配置，再批准并激活 | 固定审核可回读，只有首个子项可以启动 | 重启前后计划与审核逐字段一致。页面显示原生会话和权限。Owner 激活后首项 READY、第二项 BLOCKED。 |
| 对照 HTML 来源验收标题指南，通过 Chrome 验收后等待正式交付 | 只提交指定文件，两个必需检查通过后合并 | 候选 `10f7f7dc20861ce83d64e8539c795adbb35a87de`，PR #166 合并为 `a647a7f43ac65b24b995cf6faa694bc44d537a60`。Actions 44141627564 首轮命中预设失败步骤，同一 SHA 重跑通过，`acceptance-check` 与 `forgeos/local-ci` 均成功。[PR #166](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/166) |
| 对照容量原句及第一份指南的相对链接验收依赖子项，等待父项收敛 | 第二项以首项合并 SHA 为基础，全部交付后关闭父 Issue | 第二项 base 为 `a647a7f43ac65b24b995cf6faa694bc44d537a60`，候选 `40ab0fb34550701f82b3d464b56f81f42cfb0334` 仅改容量指南。两个必需检查成功，PR #167 合并为 `e1dc8569937471aed1059ff422469fb949e4014b`。Actions 44142162255 的预设失败亦在原 SHA 重跑通过。父 Goal DONE、Issue #165 CLOSED，修订 1 仍关联原 Management 会话。[PR #167](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/167) |
| 恢复审核前真实 PostgreSQL 快照，分别运行旧控制面与新 Worker、新控制面与旧 Worker | 双向交叉升级能审核，旧版格式保留 | 两个原生审核会话 `01a1eaa0-b8ed-7030-8987-3fb664bfc1e0`、`01a1eaa0-fefe-7b12-9910-5cf146d4d203` 均返回可接受的旧格式并保留 D2 下限。 |

PostgreSQL 18 与 Docker 全包 Go 回归 265 项测试及子测试通过，零跳过。Go 静态检查、TypeScript 检查及生产构建通过。

上述 Goal 修订审核权限对应 ForgeOS 提交 `9c772dd4d784e88cf0f023e51d83055991c73322`。

### 专题 11、16 B.1 · 候选测试计划与覆盖审核的原生权限

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 恢复 beta #163 进入审查前的真实 PostgreSQL 快照，从候选测试计划入口提交两条补充标准，运行正式 Test Agent 和 Management | 两份会话独立于实现，固定原候选及比较基线，归档各自实际权限 | 候选 `d689d8e011d2c9e1e49e93cfa632d31b132c2228`，基线 `959ab0e9e3f25f38a4f05d4c22851c6865e8a8de`。Test Agent PID 38308、会话 `01a1eaa8-01cd-7cf1-b167-e88aa5bb370c`；Management PID 46220、会话 `01a1eaa8-6ee3-7b52-aca3-ffea34517a9d`。两份配置的 `read-only`、`never`、13 个禁用功能均与原生进程参数相同。三个标准生成三个用例，独立覆盖结论为 covered。 |
| 重启控制面，在 Chrome 展开候选测试计划及两个角色的权限配置 | 固定回执可重复读取，计划摘要仍指向生成时的计划 | 重启前后回执逐字段一致。页面显示两份角色配置，覆盖回执的计划摘要为 `b2c71462764908eee9605930c0290ec2e61a4319579b8e4117d645f8cd3865c3`。本轮用例运行状态为 pending。 |
| 旧版先完成真实计划和覆盖审核，再对同库应用迁移 094 并用 Chrome 回读 | 原用例、会话、覆盖与计划摘要保留，不补造历史权限 | 原 Test Agent 会话 `01a1eaa6-1716-71e3-a999-c6eda37d0168`、Management 会话 `01a1eaa6-86f3-7b30-922f-6332049f2fbd` 的完整回执在升级前后逐字段一致。旧页面记录保留原摘要和会话字段。 |
| 从同一候选快照分别运行旧 Worker／新控制面、新 Worker／旧控制面 | 两个角色按领取结果协商回报格式，继续生成及审核 | 两个组合均经实际 Codex 会话返回 covered，保留旧版回报格式，无权限字段。 |

PostgreSQL 18 和 Docker 全包 Go 回归 265 项测试及子测试通过，零跳过。Go 静态检查、TypeScript 检查和生产构建通过。回归覆盖错误权限配置、结果重报时删除或补写权限、原计划摘要及旧记录回读。

上述候选测试计划及覆盖审核权限对应 ForgeOS 提交 `5bb9f228b2683c007eade1314ee45f99669cd3a5`。

### 专题 01、16 B.1 · 固定仓库归纳报告的原生权限

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 为 beta 克隆执行正式 HEAD 检查，在 Chrome 读取仓库清单并点击“归纳入口文档”，启动 Management Worker | 独立原生归纳保存来源证据与本次权限配置 | 固定提交 `959ab0e9e3f25f38a4f05d4c22851c6865e8a8de`。Codex PID 35152、会话 `01a1eaaf-ce0c-7c82-af46-f45acd5241d7` 的 `read-only`、`never`、13 个禁用功能与归档配置一致。返回项目用途及技术栈两项事实。 |
| 核对来源 blob，重启控制面，从 Chrome 再次读取清单并展开权限配置 | 报告、原生会话与引用来源保持固定 | 完整回执逐字段一致。README blob 为 `f05a7b812459e0e8f0e92a28aed6a97b42e5fb27`，其直接引文匹配 Git 原文。go.mod blob 为 `905ad325c90af8e1c89c181f164b6afa9c1a2f72`。两项均与固定提交树匹配，页面可见原生权限配置。 |
| 旧版先完成归纳，再升级控制面，从 Chrome 回读 | 旧报告保留原内容及会话 | 会话 `01a1eaaf-ce14-70a3-a8a7-d4014c94b7b3` 的完整报告、两项来源和原字段在升级前后逐字段一致。 |
| 新旧控制面和 Worker 双向交叉运行 | 两种组合都可接收归纳回报 | 真实会话 `01a1eaaf-ce14-7393-bc05-c8553f3a2a62`、`01a1eaaf-ce0d-7952-9ae1-ffa5ba22a635` 均生成有效报告，使用旧版字段格式。 |

PostgreSQL 18 和 Docker 全包 Go 回归 265 项测试及子测试通过，零跳过。Go 静态检查、TypeScript 和生产构建通过。临时移除权限配置校验后，针对性回归检出非法配置；恢复校验后通过。重报不能删除已归档权限。

上述仓库归纳报告权限对应 ForgeOS 提交 `753f1c2def0917e789df5a8270d3a9aa75b0f654`。

### 专题 11、16 B.1 · 预览反馈分类的原生权限

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 从 beta #163 的待验收候选快照启动静态预览，Chrome 核对页签标题及 H1，再从预览反馈表单提交观察 | 反馈绑定实际候选，Management 归档分类、原文依据和原生配置 | 候选 `d689d8e011d2c9e1e49e93cfa632d31b132c2228` 的页签及 H1 均为 Queue handbook。原生 PID 5572、会话 `01a1eab4-88e4-7580-801a-0f919ec9adf9` 返回 acceptance，引用匹配已提交观察。配置的 `read-only`、`never`、13 个禁用功能均匹配实际参数。 |
| 重启控制面，从 Chrome 展开这条观察的分类依据及权限 | 分类回执可回读，候选继续等待正式验收 | 反馈记录逐字段一致，页面显示原生配置及会话。任务仍为 WAITING_ACCEPTANCE。 |
| 旧版先分类，再升级并从 Chrome 回读；另双向交叉运行新旧控制面和 Worker | 保留历史字段，混合版本能完成分类 | 旧会话 `01a1eab3-794d-7b30-9fd7-6018e6409a3c` 的记录升级前后相同。交叉组合会话 `01a1eab4-881a-7cd3-9640-f59f87119652`、`01a1eab4-88e4-7ff1-96d1-0a569c6741b1` 均成功分类，使用旧格式。 |

PostgreSQL 18 与 Docker 的全包 Go 回归 265 项测试及子测试通过，零跳过。Go 静态检查、TypeScript 及生产构建通过。错误原生权限和重报删除权限均被拒绝。临时移除配置校验后，回归检出错误；恢复后复测通过。

上述反馈分类权限对应 ForgeOS 提交 `88f48a99667cebe54457f78a5fef29b7a2ab8e38`。

### 专题 05、16 B.1 · 业务问题建议的原生权限

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 从 beta #163 待验收候选快照提交“验收时是否比较浏览器页签与指南标题”的业务问题，由正式 Management 审阅 | 建议引用固定来源并归档原生配置，问题等待 Owner 决定 | 原生 PID 13976、会话 `01a1eab8-58b2-7132-8368-a19b0ea1d4fb` 返回 D2。引用来自 `requirement:1:body`，摘录逐字匹配冻结需求。归档的 `read-only`、`never` 及 13 个禁用功能与进程参数相同。问题未答，任务为 WAITING_INPUT。 |
| 重启后从 Chrome 展开权限配置，提交 Owner 答复，再重启 | 原建议保持固定；Owner 操作后才能继续候选验收 | 页面显示两类原生权限及禁用功能。Owner 确认按固定 HTML 比较标题后，任务回到 WAITING_ACCEPTANCE，答复身份为 owner。再次重启后，问题、原建议及答复逐字段一致。 |
| 旧版先生成建议，再升级回读；双向运行新旧 Worker 和控制面 | 保留旧建议内容，两个交叉组合仍能审阅 | 历史会话 `01a1eab8-57c6-72f1-9f42-8714ffc55de5` 的建议在新控制面与 Chrome 中逐字段一致。混合版本会话 `01a1eab8-58b4-7463-b3e4-1d93940c8082`、`01a1eab8-57c6-7773-87b3-a08431110241` 均返回 D2，使用旧格式。 |

PostgreSQL 18 和 Docker 全包 Go 回归 265 项测试及子测试通过，零跳过。Go 静态检查、TypeScript 及生产构建通过。临时移除配置校验后，针对性回归检出非法权限；恢复后通过。重放已回答问题的回执不能删除原权限。

上述业务问题建议权限对应 ForgeOS 提交 `aa645c211c08250cdab9caae9066158bd020076e`。

### 专题 07、16 A.1 · 正式 Worker 使用 App Server 实现、规划与恢复

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 设置普通 Worker 的 `FORGEOS_CODEX_TRANSPORT=app-server`，从真实 beta #168 派发 Codex 实现 | 原生进程使用 stdio，权限及当前工作目录经回执核对，候选进入实际验收流程 | 版本 `0.169.0-alpha.11.1`、PID 45188、Thread `01a1eac2-7460-7393-bf9e-9a64e0ef2b0d`。首候选 `5b63d89118a4e8aca5c515cd6d36b209f831d685` 只新增指定来源指南。返回 workspaceWrite、never、禁网和受限可写目录，通过本地检查后为 WAITING_ACCEPTANCE。 |
| Chrome 退回候选，要求补充指定句子；在新返工执行中终止正在运行的 App Server | 返工使用独立执行。该执行恢复时沿用崩溃前 Thread 与工作区，保留进程及工具活动 | 执行 2 的 PID 27940 被终止，退出码 15。Worker 经恢复校验后启动 PID 48244，继续 Thread `01a1eac4-b542-7331-9d12-c249efb59ae7` 并成功退出。两进程均有归档的命令工具事件。新候选 `039ccf9b6bee303a9d233183b95a03f7fa721840` 包含来源原句及指定补充句，只改一份指南，按空白分词为 38 词。 |
| Chrome 核对并通过返工候选，等待正式 GitHub 交付 | 精确候选通过必需检查、合并并关闭 Issue | Actions 44148894325 首轮命中预设失败步骤，同一 SHA 重跑成功；`acceptance-check` 和 `forgeos/local-ci` 均为 SUCCESS。PR #169 合并为 `c34cca80bf85d8ef17db0d98e01131701ed2e2d8`，Issue #168 CLOSED、工作项 DONE。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/169) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44148894325) |
| 在独立验收库对同一 Issue 启动 App Server 只读 Planner，从 Chrome 打开计划修订 1 | 只读规划输出符合计划契约，正式归档后等待审核 | Thread `01a1eac6-2bb3-7a11-890f-a3aa19597213` 生成两项工作内容及验收标准，任务处于 PLANNING。归档计划目标在 Chrome 可见，仓库保持干净。 |
| 增加返回工作目录校验后，直接调用同一 Go 适配器执行两次真实结构化回合；另重启正式 Worker 检查启动探测 | 能核对实际目录与原生权限，新进程恢复原 Thread 并保存结构化输出 | Thread `01a1eacc-b88a-7d10-9f71-0e14a097bb6b` 两回合均成功，第二回合沿用第一回合的状态。新版 Worker 正常启动并维持可领取状态。 |

针对性回归拒绝错误 Thread、缺失 Turn、错误工作目录、扩大的沙箱、网络或可写目录、改变审批模式及超出角色权限的原生请求；缺失终态与失败终态分别进入恢复和失败路径。临时移除 Thread 匹配条件时，测试检出错误，恢复条件后通过。

本项已验证实现、只读规划及进程中断后的恢复。正式原生审批、运行中输入、`turn/interrupt` 和其他版本兼容性继续待验收。协议字段对照安装版本生成的 JSON schema 和官方 Thread 参数定义。[官方定义](https://github.com/openai/codex/blob/main/codex-rs/app-server-protocol/schema/json/v2/ThreadStartParams.json)

本项最终 PostgreSQL 18 与 Docker 全包 Go 回归 279 项测试及子测试通过，零失败、零跳过；Go 静态检查通过。

上述 App Server 实现、规划与恢复对应 ForgeOS 提交 `74e856fd9319af075704668556c5b5560c11d8d6`。

<a id="check-a-1"></a>

### 专题 07、16 A.1/B.2 · 正式 App Server 原生中断

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在真实 beta #170 已写草稿、工具执行 120 秒观察等待时，从 Chrome 暂停模型 | 发送当前 Turn 的原生中断，退出工具及适配器，保存草稿并释放模型名额 | PID 22380 和工具 PID 5332 均退出。Thread `01a1ead5-e7c1-7661-8ea0-dda92149178e` 的 Turn `01a1ead5-eba6-7640-a671-18c6b9cbfecd` 原生记录为 turn_aborted/interrupted，进程退出码为 0。暂停请求到检查点约 1.83 秒，索引树与含草稿的工作树分别保留。模型池及节点保留名额均为 0。 |
| 重启控制面，Chrome 恢复模型回合，核对候选并通过验收 | 在同一执行、Thread 和 Worktree 完成文档，再经必需 CI 交付 | 新 PID 38196 沿用原 Thread，保留指定 H1 并补全 Queue handbook 与 HTML 来源路径。候选 `29d810254aeb33672b2044f215b62c2e0f34ec25` 仅新增指定文件。Actions 44152156126 首轮仅预设失败步骤失败，同一 SHA 重跑成功；PR #171 合并为 `787ba9f27d6d2dbd854834c2b79786ac826a4e8b`，Issue #170 CLOSED、任务 DONE。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/171) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44152156126) |
| 对 beta #19 的独立只读 Planner，在实际工具完成后从 Chrome 暂停，重启控制面并恢复 | 原生中断后保持工作区干净，在原 Thread 产出正式草案 | PID 38352 以 0 退出；Thread `01a1ead9-ab5d-7fc2-bbae-75489f6a23d5` 记录 interrupted。PID 40248 恢复原 Thread，归档修订 1、两个里程碑并进入 PLANNING。两个回合均为 never、read-only、禁网且工作目录一致。重启后任务及进程记录一致，Chrome 可展开两个进程。 |
| beta #172 的工具等待期间，从 Chrome 取消任务，再派发 git_head | 中断当前回合，不能产生候选或交付；原 Issue 继续开放，释放的资源可复用 | Thread `01a1ead9-620a-7a02-a31a-69e713b98ee1` 的 Turn `01a1ead9-6670-7430-806c-fd4a0f08e4b6` 记录 turn_aborted/interrupted。原生 PID 46264 以 0 退出，工具 PID 27928 退出。任务 CANCELLED，无候选、无 PR；后续任务 2 DONE，模型保留名额为 0，Issue #172 OPEN。 |
| Chrome 查看节点能力，再交叉运行新旧控制面和 Worker，等新心跳覆盖旧记录 | 只有新组合的 App Server 显示原生中断，混合版本仍能工作 | 页面显示“中断回合”。新 Worker 配旧控制面可持续运行，旧控制面忽略附加声明。切回旧 Worker 后，新控制面撤下 interrupt。已取消任务状态保持。 |

针对性回归覆盖正常中断、完成与取消同时发生、原生拒绝、无响应超时，以及尚未收到 turn/started 时终止进程；所有取消场景均禁止输出。把请求的 Turn ID 临时改错，回归立即检出未中断目标回合，恢复代码后通过。原生协议的参数为 Thread ID 与 Turn ID。[官方定义](https://github.com/openai/codex/blob/main/codex-rs/app-server-protocol/schema/json/v2/TurnInterruptParams.json)

正式原生审批、运行中的输入、Claude 原生中断及其他 Codex 版本继续待验收；这次没有扩大角色权限。

本项最终 PostgreSQL 18 与 Docker 全包 Go 回归 285 项测试及子测试通过，零失败、零跳过。Go 静态检查、TypeScript 及生产构建通过。Linux 容器中的 App Server 针对性竞态检查通过。

上述 App Server 原生中断对应 ForgeOS 提交 `af79a557af1fe91f328aeed8b18a2412b3d9fd89`。

### 专题 05、08、16 C.7 · Management 计划审核的固定规则依据

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在真实 beta #173 从 Chrome 起草两项有依赖的计划；第一项采用 Issue 明确指定标题，第二项故意采用与项目决策冲突的全局默认标题 | Management 按 Owner、项目、全局的优先级审核，来源绑定到此次领取 | 原生会话 `01a1eae5-e09a-7823-a9d8-bb0d2604d5a7` 退回修订 1，指出第二项标题不符合决策 1。review_context 固定全局修订 1、决策 1 正文、适用范围和引用它的 capacity/title 工作项。 |
| 领取后更新全局规则并使决策 1 失效，再等待审核结束、重启并从 Chrome 打开修订 1 | 正在审核的输入和历史证据保持领取时版本 | 完成后的快照逐字段不变；页面仍显示全局修订 1 的 Generic queue guide 及原项目规则，不混入修订 2 的 Later default guide。原生输入 JSON 与归档快照一致。 |
| 确认新的项目决策 2，Chrome 提交修订，原生 Management 审核后由 Owner 批准 | Owner 明确标题优先于项目约定，项目标题优先于全局默认；D2 仍需 Owner 激活 | 修订 2 的会话 `01a1eae6-28de-7600-a6e7-a1170628b3a7` 正确识别两处标题优先级，但要求重复写明前项先合并。补充中台对 dependencies 的既有执行合同后，同一计划的修订 3 在会话 `01a1eae8-686a-75f1-bdc7-bdb67cbb46f5` 获 D2 approve。两版均固定全局修订 2 和决策 2。Chrome 批准后，title 为 READY、capacity 为 BLOCKED。三次原生输入均逐字段匹配各自快照，权限为 never、read-only、禁网。 |
| 正式 Worker 实现 title；Chrome 核对标题、HTML 引文及单文件差异并验收 | 使用 Owner 指定 H1，必需 CI 通过后合并，父 Issue 继续开放 | 候选 `89e8cc3058f7be57e67a0824cf47ffdacfe0b398` 仅新增 authority-title.md，H1 为 Queue preview Owner title。PR #174 合并为 `2b3943401276e7f3f80280fcd50fbe8e090af64c`，父 Issue #173 OPEN。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/174) · [Actions 44155648615](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44155648615) |
| 依赖项获派后实现 capacity；Chrome 验收并等待整个 Goal 交付 | 使用项目约定标题，以前项合并为基础，全部必需项交付后关闭父 Issue | 候选 `6672e00ed7af8706c8b952d06444624252454c2f` 的基础为上述合并 SHA，只新增 authority-capacity.md，H1 为 Queue project guide；等待容量原句匹配 HTML，链接可定位已交付首项。PR #175 合并为 `34802ac54ff9a3a1b5de0980d3ecfee28d4172d4`，父 Goal DONE、Issue #173 CLOSED。两个候选均核对全局修订 2、决策 2；必需 Actions 的预设失败均在同一 SHA 重跑成功，本地检查也为 SUCCESS。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/175) · [Actions 44156183628](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44156183628) |
| 旧版先完成计划审核，再升级迁移 095；另分别运行新控制面/旧 Worker、旧控制面/新 Worker | 历史记录保持原状，混合版本不记录未派发的上下文 | 历史会话 `01a1eae6-c773-74a3-9dc8-e96c8d219270` 的整份计划回执升级前后相同，Chrome 可回读且无补造快照。两个交叉组合分别以 `01a1eae7-1043-7002-ae02-206eb34d25b8`、`01a1eae6-e6ec-7223-aaac-ae1bacffe990` 完成原生审核，保持原输入格式。 |

PostgreSQL 18 与 Docker 全包 Go 回归 287 项测试及子测试通过，零失败、零跳过。随后对引用工作项的稳定排序再次运行相关数据库和 Worker 测试，通过；Go 静态检查、TypeScript 和生产构建通过。回归核对跨项目、未确认、失效和未引用决策的排除、重复引用合并、历史固定以及租约重领后的新快照。临时移除项目边界时，测试检出跨项目内容，恢复条件后通过。

本项覆盖计划审核；其他角色的上下文和整条路线的最终回归仍待完成。


上述 Management 计划审核固定规则依据对应 ForgeOS 提交 `20b8067c71a9a24ae374bee66bc7fed13a152bdb`。

### 专题 05、08、16 C.7 · 业务问题建议的固定执行依据

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 从真实 beta #176 派发单文件指南，在 Codex 工具观察等待期间用 Chrome 排队指定页脚的 Owner 指令；第一轮只询问标题 | 建议使用执行开始时的规则，不能把未处理指令当成已执行依据 | 问题 1 的原生建议会话 `01a1eafb-60e0-7890-b396-e439b735db46` 引用固定需求，D2 继续等待 Owner。快照含全局修订 1，无 owner_instructions。 |
| 发布全局修订 2，再从 Chrome 回答标题。实现回合处理排队指令后提出第二个页脚问题 | 第二轮建议取得前一轮 Owner 答复、已处理指令和原全局版本，按 Owner 优先级给出建议 | 会话 `01a1eafb-c244-7af3-8fa6-15847c582673` 引用 `owner-command:1:7e3ccd18-f6dd-41ac-9a26-2492be84ccd3`，建议采用 Owner queued footer。上下文包含指定 H1 的前一答复、检查点 1 已处理指令和全局修订 1；不混入 Later fallback footer。两次原生 prompt 的上下文均逐字段匹配归档，权限均为 never、read-only、禁网。 |
| 重启控制面，在 Chrome 待答页展开执行依据并回答；完成后打开任务历史 | 原建议保持不变，由 Owner 答复后继续执行；历史页可回读两次不同依据 | 两题分别答复后，原生实现会话 `01a1eafa-7576-7c82-939b-91b8ee005fbc` 生成候选 `03ec04ff4b630b1ff99d2a57fc77462181b7e373`，只新增 advice-authority.md。唯一 H1 为 Queue Owner advice，页脚为 Owner queued footer，同时保留 Queue handbook 和 HTML 来源路径。本地文档检查通过。首次历史页断言早于加载完成，补上页面等待后从同一候选复验通过。 |
| Chrome 核对单文件内容并通过候选验收，等待正式交付 | 必需 CI 通过，合并精确候选并关闭 Issue | Actions 44159844163 首轮仅命中预设失败步骤，同一 SHA 重跑成功；acceptance-check 和 forgeos/local-ci 均 SUCCESS。PR #177 合并为 `fff9cd97784673a56dc215fbe7663a3ceca9b038`，Issue #176 CLOSED、任务 DONE。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/177) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44159844163) |
| 从 beta #163 的旧候选快照运行历史升级、新旧控制面和 Worker 双向组合，并在旧候选生成后发布全局规则 | 历史及混合版本不补造未使用的上下文；新节点不能给旧候选套上后来发布的规则 | 历史会话 `01a1eafb-9ccd-7a91-9775-830ce15702fb` 升级后建议逐字段不变。旧控制面/新 Worker 会话 `01a1eafb-e172-7bd1-b8c0-b8b441d04654`、新控制面/旧 Worker 会话 `01a1eafc-2073-7a20-a15d-4fc6a8305668` 均无新 context 字段。新组合会话 `01a1eafc-60dd-7a83-ae80-67b0e6b60d84` 的上下文未附加后来发布的全局规则。四种组合均从重启后的 Chrome 核对原回执。 |

数据库回归覆盖继承的 Owner 答复、当前执行的先前答复、验收退回意见、已处理指令的检查点边界、候选重复换基后的原实现规则、过期重领的旧格式、拒绝伪造依据和回答后的幂等回报。D1 的当前有效性、项目隔离及 D3 约束继续通过。临时删除检查点上限时，新增回归检出未来指令；恢复原条件后针对性复测通过。

本项 PostgreSQL 18 与 Docker 全包 Go 回归 289 项测试及子测试通过，零失败、零跳过；Go 静态检查、TypeScript 和生产构建通过。预览反馈分类等尚未覆盖的角色上下文及最终全路线端到端仍待补齐或验收，当前 13–23 个工作包的粗估口径保持不变。


上述业务问题建议固定执行依据对应 ForgeOS 提交 `fc79842f7d9af673044d7a99e622424d340ca321`。

### 专题 05、08、11、16 C.7 · 预览反馈分类的固定候选依据

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 从 beta #176 已交付候选开启真实静态预览，用 Chrome 查看 Queue handbook 并读取预览中的 advice-authority.md；登记核对标题和页脚的观察 | 根据该候选原需求及 Owner 答复分类，不能使用后来发布的规则 | 反馈 1 的原生会话 `01a1eb07-0471-79a3-abb9-69ec1b1f29e4` 分为 acceptance。来源固定候选 `03ec04ff4b630b1ff99d2a57fc77462181b7e373`、需求修订 1、全局修订 1、已处理指令及两轮 Owner 答复。页面实际取得同一候选响应头与文件正文。 |
| 在同一指南登记“改用 Global fallback footer，替代此前 Owner 选定的 Owner queued footer” | 识别为修改既定业务选择，分类本身不能修改候选或正式要求 | 反馈 2 的会话 `01a1eb07-2f96-7d73-a6fe-d33e3186861d` 分为 requirement_change，引用实际观察；上下文仍为原执行版本。两份分类的原生输入逐字段匹配归档快照，权限为 never、read-only、禁网。 |
| 重启控制面，从 Chrome 展开两条反馈的候选依据和规则依据 | 可回读准确候选与原规则，原交付保持不变 | 两份分类与重启前逐字段一致，页面显示全局修订 1 和 Owner 页脚，没有混入 Later fallback footer。原任务仍 DONE、候选不变；原交付为 [PR #177](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/177)。 |
| 在 beta #173 的 title 子项已交付候选开启真实预览，读取 authority-title.md 并登记符合明确标题要求的观察 | 根据子项绑定的计划范围审核，不能用项目默认标题覆盖 Owner 明确标题 | 会话 `01a1eb0c-0571-7d30-a156-8164ec2b65ec` 返回 acceptance。固定父需求修订 1、计划修订 3 的 title 工作项、全局修订 2 和项目决策 2；候选 `89e8cc3058f7be57e67a0824cf47ffdacfe0b398` 未变。原生输入匹配快照。Chrome 重启后可见 Goal、计划版、范围及规则。原交付为 [PR #174](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/174)。 |
| 从 beta #163 待验收快照验证历史升级和双向新旧角色组合 | 历史回执保持不变，旧角色不补造新字段，候选仍等 Owner 验收 | 历史会话 `01a1eb07-b6db-7cf1-a06c-7a368d0f6092`、旧控制面/新 Worker `01a1eb07-fb39-7850-842a-5f8b515ddb1d`、新控制面/旧 Worker `01a1eb08-3b16-7241-9270-92f553143f16` 均可由升级后的 Chrome 回读，无 context 字段。 |
| 在同一旧候选用新组合分类只核对既有页面标题、未核对本次指南的观察 | 依冻结需求指出证据范围不足 | 新会话 `01a1eb08-7cd9-7f10-91a0-3ae4d523cc46` 返回 blocked，理由是需求交付对象为 reviewer guide，页面标题观察未证明该指南合规。验收脚本原先沿用旧版 acceptance 断言，核对冻结需求后改为验证这一证据不足结论；没有更改模型回执。原生输入与快照一致，Chrome 可回读，候选仍 WAITING_ACCEPTANCE。 |

数据库回归验证需求新版、全局新版不混入旧候选、Goal 绑定范围、过期租约由旧角色领取时清除未使用上下文、拒绝伪造快照、重复回报不重复事件，以及重复换基后保留原实现规则并拒绝原实现会话分类。临时删除原实现身份检查时，回归检出越权代审；恢复后针对性复测通过。

本项 PostgreSQL 18 与 Docker 全包 Go 回归 291 项测试及子测试通过，零失败、零跳过；Go 静态检查、TypeScript 和生产构建通过。

初次还原的 beta #55 固定候选不含 index.html，正式预览入口按合同返回 422，未伪造页面或预览记录。仅重跑 CI 及换基候选的完整预览分类现已在 beta #178 实测，详见下一节。其余全路线工作及最终回归继续保留，粗估 13–23 个工作包不变。

上述固定候选依据实现对应 ForgeOS 提交 `478b715bf86d6bf1cb03d9fa3b4c57dfd145604a`。


### 专题 08、11、12、16 · CI 恢复与真实换基后的预览依据

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 从 beta #178 派发单文件 HTML；在实际 Docker 验证进程 PID 14904 运行时，通过 Chrome 暂停 CI，重启控制面后恢复 | 只重跑验证，沿用原候选，不重新调用实现模型 | 原实现执行 1、会话 `01a1eb0f-a87c-74e2-b25f-a56f47ad1d5c` 生成 `f0705aa31192e7faa591d2409f51b98385b105ad`。执行 2 无模型会话，真实 Docker go-test 通过，候选不变。 |
| 在恢复后的候选开启静态预览，用 Chrome 核对 title、唯一 H1 和段落，再登记观察；全局规则此时已从修订 1 更新为 2 | 分类沿用原实现依据，不能由原实现会话代审 | 页面标题及唯一 H1 为 Queue CI evidence，段落逐字匹配 Issue。用原实现会话回报返回 409；隔离验收库将该次人工探测租约设为到期后，正式 Management Worker 重新领取，独立原生会话 `01a1eb13-a499-7f83-8a7b-61b6569df8e5` 返回 acceptance。上下文是预览执行 2、原实现执行 1、原需求及全局修订 1。Chrome 重启回读并勾选该反馈完成验收。 |
| 独立交付 beta #180，让真实目标分支前进，同时保持原 PR #179 开放 | 新基线来自正常受保护交付；旧候选不能越过目标更新直接合并 | 直接推进空提交曾被分支保护拒绝，未修改目标。随后原生 Worker 完成单文件 refresh-note.md，文档验证及两项必需检查通过。[PR #181](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/181) 合并为 `ba4254b4c832e00c126e17fecbd1ab1985116728`，[Actions 44164247128](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44164247128) 重跑通过，Issue #180 关闭。验收分支保留两项必需检查并启用 strict；旧 PR 的预设失败检查重跑成功后，GitHub 显示 BEHIND。 |
| Chrome 点击“在最新目标版本上重新验证” | 读取目标分支当前 ref，应用已批准补丁，关闭旧 PR，排队新验证 | 首次真实运行发现 GitHub PR 的 base.sha 仍为 `fff9cd97784673a56dc215fbe7663a3ceca9b038`，原代码返回 409。修复后按实际分支 ref 换基，关闭 [PR #179](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/179)，未发生合并。执行 4 无模型会话，Docker go-test 成功，产生新候选 `fd2d16c252b6acac10ef14c43c9638689b23f863`，唯一父提交是上述新基线；相对基线只新增原 HTML，正文逐字不变。 |
| 在新候选重新开启预览并分类，重启后重新勾选反馈验收 | 新候选有自己的分类与批准，依据追溯到旧实现，不混入全局新版 | 会话 `01a1eb1d-dad7-7d81-86ca-37fc35c5e011` 返回 acceptance。候选为执行 4 和新 SHA；规则为原实现执行 1、提交 `f0705aa31192e7faa591d2409f51b98385b105ad` 及全局修订 1。原实现会话代审仍返回 409。两次原生 prompt 的 context 均逐字段匹配归档，权限均为 never、read-only、禁网。 |
| 等待新候选必需 CI、合并与 Issue 回写，最后重启工作台 | 只交付重新验收的新 SHA；两次历史依据与验收引用继续可读 | [Actions 44165869662](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44165869662) 首轮仅预设步骤失败，同一 SHA 重跑成功。acceptance-check、forgeos/local-ci 均 SUCCESS。[PR #182](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/182) 合并为 `a68e1c2fb11ed1f0b46d88474098fb26d5fe6e9c`，Issue #178 CLOSED、任务 DONE。重启后 Chrome 可回读两版预览的当前候选、原实现提交及全局修订 1；两条反馈均关联各自的 Owner approve。 |

新增控制面回归覆盖真实 Git 仓库和 PostgreSQL：PR base SHA 保持旧值、目标 ref 身份不符、目标未前进、关闭旧 PR 期间目标再次前进、从已关闭 PR 重试及重放无远端副作用。最初测试夹具把普通 Issue 的 pull_request 序列化为 null，被严格身份校验拒绝；改为与实际普通 Issue 一样省略该字段后通过。临时恢复使用 PR base SHA 的错误实现时，新增测试失败，恢复修复后复测。

本项 PostgreSQL 18 与 Docker 全包回归 292 项测试及子测试通过，零失败、零跳过；Go 静态检查及 git diff --check 通过。本次未改 Web 代码，沿用上一实现提交的 TypeScript 与生产构建结果。全路线最终回归尚未执行，外部设备、受限 GitHub 身份、独立审查者及其余已列缺口继续待验收。

上述实际换基修复对应 ForgeOS 提交 `51a440d38d10221003094a16793e5a560509dcc0`。

### 专题 07、13、16 C.3 · 模型公开消息的运行中查看与归档

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在 beta #183 的 Codex CLI 原生回合输出公开进度和合成凭据行；在有界本地等待期间打开 Chrome，再重启控制面 | 原生进程仍在运行时显示脱敏消息，身份和序号可追溯；重启保留 | PID 35292、会话 `01a1eb29-7ab8-7723-a416-d4627ad37d44` 的公开消息从原生记录核对。Chrome 显示 ForgeOS public progress cli，合成凭据行显示 [redacted]；SQLite 与最终进程记录逐字段一致。结束时记录 2 段，候选 `546995460bd0d730cb0b0464adb9b1ad72349bf3` 通过文档验证及页面验收。 |
| 等待 Codex CLI 候选的受保护交付，结束后重启工作台回读 | 必需检查成功，精确候选合并并关闭 Issue，历史输出不变 | [PR #184](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/184) 合并为 `ddcdc8e76ba77e603d520bb4115f609c55612fa8`。[Actions 44168310584](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44168310584) 首次仅预设步骤失败，同一 SHA 重跑成功；acceptance-check 和 forgeos/local-ci 均 SUCCESS。任务 DONE、Issue CLOSED，重启后 Chrome 可回读原输出。 |
| 在 beta #185 的 Codex App Server 原生回合输出公开进度和合成凭据行；在有界本地等待期间打开 Chrome，再重启控制面 | 原生进程仍在运行时显示脱敏消息，身份和序号可追溯；重启保留 | PID 38080、会话 `01a1eb2d-5a5b-7242-a414-88d80ccf83d3` 的公开消息从原生记录核对。Chrome 显示 ForgeOS public progress appserver，合成凭据行显示 [redacted]；SQLite 与最终进程记录逐字段一致。结束时记录 2 段，候选 `a8e56ace64626921d814233d986a20bc9dbcfcc8` 通过文档验证及页面验收。 |
| 等待 Codex App Server 候选的受保护交付，结束后重启工作台回读 | 必需检查成功，精确候选合并并关闭 Issue，历史输出不变 | [PR #186](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/186) 合并为 `ed7cf6e2c6b5d099793ac3ddb5dd71b1d234bfed`。[Actions 44169059857](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44169059857) 首次仅预设步骤失败，同一 SHA 重跑成功；acceptance-check 和 forgeos/local-ci 均 SUCCESS。任务 DONE、Issue CLOSED，重启后 Chrome 可回读原输出。 |
| 在 beta #187 的 Claude Code 原生回合输出公开进度和合成凭据行；在有界本地等待期间打开 Chrome，再重启控制面 | 原生进程仍在运行时显示脱敏消息，身份和序号可追溯；重启保留 | 需求修订 2、执行 2。PID 21072、会话 `2e2ca36c-ced9-4cac-8136-e292b562dcef` 的公开消息从原生记录核对。Chrome 显示 ForgeOS public progress claude，合成凭据行显示 [redacted]；SQLite 与最终进程记录逐字段一致。结束时记录 4 段，候选 `740d521f19444c956443d3f928cba1f36a842e43` 通过文档验证及页面验收。 |
| 等待 Claude Code 候选的受保护交付，结束后重启工作台回读 | 必需检查成功，精确候选合并并关闭 Issue，历史输出不变 | [PR #188](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/188) 合并为 `e1b0a41f1fe9fc0125ba69e749d090477a825805`。[Actions 44171222556](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44171222556) 首次仅预设步骤失败，同一 SHA 重跑成功；acceptance-check 和 forgeos/local-ci 均 SUCCESS。任务 DONE、Issue CLOSED，重启后 Chrome 可回读原输出。 |
| 在 beta #189 的 Codex CLI（最终回执连续性复验） 原生回合输出公开进度和合成凭据行；在有界本地等待期间打开 Chrome，再重启控制面 | 原生进程仍在运行时显示脱敏消息，身份和序号可追溯；重启保留 | PID 35332、会话 `01a1eb3e-9c88-73e0-9938-9ca5766db3a6` 的公开消息从原生记录核对。Chrome 显示 ForgeOS public progress clifinal，合成凭据行显示 [redacted]；SQLite 与最终进程记录逐字段一致。结束时记录 2 段，候选 `e4bd4eece8fefb607da70b8709a7a98906572e44` 通过文档验证及页面验收。 |
| 等待 Codex CLI（最终回执连续性复验） 候选的受保护交付，结束后重启工作台回读 | 必需检查成功，精确候选合并并关闭 Issue，历史输出不变 | [PR #190](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/190) 合并为 `64fd9bbbadcc05b50ca5ae387260b80b77f0b468`。[Actions 44172358355](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/44172358355) 首次仅预设步骤失败，同一 SHA 重跑成功；acceptance-check 和 forgeos/local-ci 均 SUCCESS。任务 DONE、Issue CLOSED，重启后 Chrome 可回读原输出。 |
| 用 旧控制面／新 Worker 只读规划 beta #2，再重启并打开任务历史 | 保留协议实际收集的数据，规划结果和输出可分别追溯 | 会话 `01a1eb2f-f180-7282-b14a-38b0291654f8` 生成计划修订 1。原生规划成功，未采集输出时不新增该字段；重启后 Chrome 不显示虚构消息。 |
| 用 新控制面／旧 Worker 只读规划 beta #2，再重启并打开任务历史 | 保留协议实际收集的数据，规划结果和输出可分别追溯 | 会话 `01a1eb30-d867-7ec1-a744-1487a279823f` 生成计划修订 1。原生规划成功，未采集输出时不新增该字段；重启后 Chrome 不显示虚构消息。 |
| 用 新控制面／新 Worker 只读规划 beta #2，再重启并打开任务历史 | 保留协议实际收集的数据，规划结果和输出可分别追溯 | 会话 `01a1eb31-fe7b-7a30-bba5-8b3449890ad2` 生成计划修订 1。归档公开输出，SQLite 与进程记录一致，Chrome 可展开回读。 |


Claude 初轮把“公开进度消息”理解成对外消息发送，拒绝该部分并继续编辑；这次未计为运行中验收通过。验收脚本退出时中断了尚未回报的 Worker，原生进程已退出但执行仍 RUNNING。核对节点记录后，在隔离验收库加速该租约到期，使用正式 Owner abandon 入口保留失败执行；随后真实修订 GitHub Issue，明确为自身响应文本与本地有界等待，通过需求采用入口切换到修订 2。新执行完成上表全部检查，未伪造模型消息或完成回执。重试时按执行编号选择对应节点日志，保留首轮日志。

数据库回归覆盖输出序号回退、同序号改写、退出后改写、过期租约及最终回执夹带执行租约的拒绝，并验证最终回执不得删除或同序号改写已上报输出，以及合法最终输出的幂等保存。beta #189 另行从运行中上报、控制面重启到最终回执和真实交付复验该连续性检查。节点测试覆盖三种适配器的公开消息、排除思考块和工具参数、敏感环境值、凭据轮换、跨消息私钥遮蔽、UTF-8 窗口及 SQLite 重开。临时关闭采集时，三种适配器回归均失败；恢复后复测通过。

PostgreSQL 18 与 Docker 全包回归 298 项测试及子测试通过，零失败、零跳过；Go 静态检查、TypeScript 与生产构建通过。模型公开消息仅完成实时输出工作包的一部分，工具正文、内置验证与启动阶段继续待补齐。13–23 个剩余工作包的粗估口径不变，全路线最终验收及已列外部条件仍待完成。

上述模型公开消息对应 ForgeOS 提交 4e2e02e6df774ed770b586916934422a7f1a3795。

### 专题 10、16 C.3 · 内置 go-test 运行输出

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 从 alpha #34 导入 go-test 任务，由原生 Codex 添加两个测试包和模块声明 | 不存在 validation.yaml 时保留内置验证，固定候选进入隔离 Docker | API 的 validation 为 go-test。候选为 20078d3e0cde0023c694fe4e7e83c94b64a7fd26，差异仅含三个要求文件。原生 Go 测试容器 f515169813e8 使用只读根文件系统、只读候选挂载和 network=none。 |
| 快包完成、45 秒观察包仍在运行时打开 Chrome，重启控制面后刷新 | 展示实际已写出的输出，候选和运行身份不变 | 页面“实时 CI 日志 · go-test”显示 progressquick 的真实输出，运行 ci-run-1、候选 20078d3e0cde0023c694fe4e7e83c94b64a7fd26 与该容器的 Git HEAD 相符。重启后回读相同输出。 |
| 等待完整测试结束，从页面验收，再完成受保护交付 | 两包通过，实时样本清除，日志留存，精确候选合并并关闭 Issue | 节点相邻 .go-test.log 含两个包的成功输出。候选通过页面验收，[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/35) 合并为 839b68d77e881789b765c0c9def792c2c287a9e9。Actions 44174101132 首次仅预设失败步骤失败，同一 SHA 重跑通过；acceptance-check 与 forgeos/local-ci 均 SUCCESS。任务 DONE、Issue CLOSED，重启后不再显示过期实时样本。 |

先前 beta #191 因已有 validation.yaml，按既有权限规则升级为 auto/V2，真实声明检查通过并停在 WAITING_ACCEPTANCE。这次夹具没有进入内置 go-test，不计为本项验收证据；保留其候选和运行记录，改用 alpha 完成本项。未修改或降低 beta 的声明式验证规则。

新增真实 Docker 回归让一个包输出合成凭据失败诊断、另一个包继续运行，断言完整检查结束前收到脱敏消息，最终失败结果不变。PostgreSQL 回归对声明式与内置配置同时验证候选、租约、检查顺序、重放、过期及终态清除。301 项测试及子测试通过，零失败、零跳过；Go 静态检查和 git diff --check 通过。没有修改 Web，使用上一提交已验证的任务页面构建。

工具正文、启动阶段以及独立角色的实时输出仍待补齐；内置验证原始日志仍在节点本地。本项不替代全路线最终回归，外部设备、受限身份和独立审查者仍保持待验收。

上述内置 Go 验证实时输出对应 ForgeOS 提交 a4ebc74863e359520a038f7d2ac65bd86dee2c00。

### 专题 07、13、16 C.3 · 完成工具的输出窗口

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| beta #192 由 Codex CLI 先读取 README，再写入并读取一个合成脱敏夹具 | 运行中能看到实际工具结果，绑定调用和原生会话；重启可回读 | PID 2740、Session 01a1eb51-fd48-75e2-afa3-ca0cede3d2a4 的活动工具结果已从 Chrome 核对并重启回读。最终输出为 command_execution 调用 #5、事件 #6；原生结果含合成凭据行，SQLite 和最终窗口均显示 [redacted]，二者逐字段相符。 |
| 核对单文件候选，在页面验收并等待受保护交付 | 精确候选通过检查、合并、关闭 Issue；重启后可回读 | 候选 ff213ed342df405bdde43169d534741e34fffc23 经 [PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/193) 合并为 882223248bed1d6f80c9c81266a350ec91a9e6eb。Actions 44175896450 的预设失败步骤在同一 SHA 重跑通过，acceptance-check 和 forgeos/local-ci 均 SUCCESS；任务 DONE、Issue CLOSED，Chrome 历史窗口保留。 |
| beta #194 由 Codex App Server 先读取 README，再写入并读取一个合成脱敏夹具 | 运行中能看到实际工具结果，绑定调用和原生会话；重启可回读 | PID 40240、Session 01a1eb54-e5be-7ce3-ba04-15e282caa19a 的活动工具结果已从 Chrome 核对并重启回读。最终输出为 command_execution 调用 #7、事件 #8；原生结果含合成凭据行，SQLite 和最终窗口均显示 [redacted]，二者逐字段相符。 |
| 核对单文件候选，在页面验收并等待受保护交付 | 精确候选通过检查、合并、关闭 Issue；重启后可回读 | 候选 f7b4006c4de57e5b3d8571c51684554217aa299d 经 [PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/195) 合并为 74177aadbe86fa8c3991135d7cf9c385b0177d21。Actions 44176490839 的预设失败步骤在同一 SHA 重跑通过，acceptance-check 和 forgeos/local-ci 均 SUCCESS；任务 DONE、Issue CLOSED，Chrome 历史窗口保留。 |
| beta #196 由 Claude Code 先读取 README，再写入并读取一个合成脱敏夹具 | 运行中能看到实际工具结果，绑定调用和原生会话；重启可回读 | PID 38944、Session 35cc78fd-4a4e-4b64-9978-90eb2f791ebe 的活动工具结果已从 Chrome 核对并重启回读。最终输出为 Read 调用 #5、事件 #6；原生结果含合成凭据行，SQLite 和最终窗口均显示 [redacted]，二者逐字段相符。 |
| 核对单文件候选，在页面验收并等待受保护交付 | 精确候选通过检查、合并、关闭 Issue；重启后可回读 | 候选 759108368389bbb91a52756c10c05db35b722a11 经 [PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/197) 合并为 34c0f5eedb90c9f96d488552cf29c9fa2bebda1a。Actions 44176964519 的预设失败步骤在同一 SHA 重跑通过，acceptance-check 和 forgeos/local-ci 均 SUCCESS；任务 DONE、Issue CLOSED，Chrome 历史窗口保留。 |
| beta #198 由 Claude 分段私钥夹具 先读取 README，再写入并读取一个合成脱敏夹具 | 运行中能看到实际工具结果，绑定调用和原生会话；重启可回读 | PID 33476、Session eaa6bd50-bdab-42bb-8bcc-67055c5bcad2 的活动工具结果已从 Chrome 核对并重启回读。最终输出为 Read 调用 #11、事件 #12；原生结果含合成凭据行，SQLite 和最终窗口均显示 [redacted]，二者逐字段相符。 |
| 核对单文件候选，在页面验收并等待受保护交付 | 精确候选通过检查、合并、关闭 Issue；重启后可回读 | 候选 fa45483beab53b1e0552c8dbbeb9bcdbe888fded 经 [PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/199) 合并为 be4987c02c047e2b2c8b2dc1fd4a589edbd8caef。Actions 44177961738 的预设失败步骤在同一 SHA 重跑通过，acceptance-check 和 forgeos/local-ci 均 SUCCESS；任务 DONE、Issue CLOSED，Chrome 历史窗口保留。 |
| 旧控制面／新 Worker 对 beta #2 进行只读规划，再重启工作台 | 保留实际协议收集的字段和原生计划 | 会话 01a1eb53-da47-7500-bf6e-43738b11997a 产出计划修订 1；没有声明或采集工具正文时保持缺省，浏览器不补造记录。 |
| 新控制面／旧 Worker 对 beta #2 进行只读规划，再重启工作台 | 保留实际协议收集的字段和原生计划 | 会话 01a1eb55-069d-7aa3-82d8-78c3ff48ea85 产出计划修订 1；没有声明或采集工具正文时保持缺省，浏览器不补造记录。 |
| 新控制面／新 Worker 对 beta #2 进行只读规划，再重启工作台 | 保留实际协议收集的字段和原生计划 | 会话 01a1eb56-3403-7542-b2ac-372c8e38d837 产出计划修订 1；保留脱敏工具窗口，节点 SQLite 与浏览器历史已核对。 |
| 使用最终持久化实现重新执行只读规划并重启回读 | 新的状态表不影响原生工具记录和计划 | 会话 01a1eb60-4d36-7f43-82c5-d7d2623eff92 生成计划修订 1，工具窗口和浏览器历史一致。 |

回归验证三适配器工具正文、排除参数和图片、失败调用输出、敏感环境值、节点凭据轮换、跨工具调用的分段私钥、UTF-8 窗口、事件滚动及 SQLite 重开。控制面测试覆盖工具和调用身份、序号回退、同序号修改、执行租约泄露及最终回执删除／覆盖拒绝。原 16 KiB HTTP 上限不能容纳合法窗口的 JSON 转义结果；新增真实路由测试用两个 8 KiB HTML 字符窗口验证成功上报、幂等重放、改写拒绝和超限拒绝。

PostgreSQL 18 与 Docker 全包 307 项测试及子测试通过，零失败、零跳过；复查发现最初实现会在新调用时清除私钥状态，新增分段读取回归先失败；移除该重置后复测和全包回归通过。beta #198 又从真实 Claude 的两次 Read 分段返回核对：第一段有开始标记且无结束标记，第二段含结束标记；节点与最终输出均遮蔽第二段合成密钥。Go 静态检查、TypeScript 和生产构建通过。仅最近完成且有正文的工具结果进入本窗口；工具执行中的增量输出、Codex MCP 等结构化工具结果、完整工具归档、启动阶段及其他独立角色输出仍待补齐。SQLite 重开及替换脱敏器后的跨段遮蔽已回归；Codex 原生进程在两段读取之间重启的完整场景见后续记录；App Server 和 Claude 的恢复场景见后续记录。全路线最终验收和已列外部条件继续待验收。

上述工具输出实现对应 ForgeOS 提交 dbbc3d9e75e5ed8ce6a36ec049bd52b44c18d4d7。

### 专题 07、13 · 原生进程替换后的工具脱敏状态

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| beta #200 的原生 Codex 创建九行合成夹具，读取前五行，再执行有界观察等待 | 私钥开始标记之后的正文遮蔽，节点保存未结束状态 | 原生 PID 39888、Thread 01a1eb64-a38d-7f83-ab2e-1f27007d5824 完成第一段读取；SQLite 的 tool 私钥状态为 true，公开窗口没有合成密钥正文。 |
| 从 Chrome 暂停模型，退出 Worker，重启控制面和 Worker，再从页面恢复 | 同一执行、工作区和 Thread 恢复，替代进程继承脱敏状态 | Worker 35244 退出后由 44504 接回，原生 PID 从 39888 变为 33744，执行仍为 #1。恢复前 SQLite 状态仍为 true。 |
| 核对恢复后实际原生工具结果和节点窗口 | 第二段无需再次包含开始标记，也持续遮蔽到结束标记 | 按替代进程启动时间筛选原生 item_completed：后续结果没有 BEGIN PRIVATE KEY，却包含 fake-key-second 与结束标记。最终窗口中该合成正文被 [redacted] 替换，结束标记后普通文本保留，SQLite 状态变为 false。 |
| 验收精确候选并等待交付，重启后回读两个进程 | 候选通过检查、合并并关闭 Issue，历史保留 | 候选 149e6c58d2d0eea8ed75dd3dbe88d04277fb4e68 由 [PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/201) 合并为 ad201df73c0828524c8f19f35d646ba48d506233；Actions 44179576877 预设失败后同 SHA 重跑成功，两项检查 SUCCESS。任务 DONE、Issue CLOSED，Chrome 回读同一份进程输出。 |

本项未修改实现，沿用 dbbc3d9 的 307 项 PostgreSQL/Docker 回归、静态检查及 Web 构建。仅新增真实入口验收和文档。App Server 和 Claude 的后续恢复实测见下文；增量输出、完整工具归档，以及全路线剩余项目继续待验收；未把 Worker 重启等同于整机重启。

上述 Codex CLI 恢复验收对应 ForgeOS 提交 00379260780c5b9df2aea10336f4c545460f0c90。

### 专题 07、13 · App Server 与 Claude 恢复后的工具脱敏

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| beta #202 的 App Server 读取合成私钥前五行，在有界等待期间从 Chrome 暂停，重启控制面和 Worker，再恢复 | 原执行和 Thread 继续；新进程只读后半段时仍遮蔽 | 执行 #1、Thread 01a1eb68-e3c3-7603-a12d-0a2deb5e2513 保持不变，原生 PID 40728 → 42796，Worker 39052 → 49128。替代进程的原生结果无开始标记、含合成密钥尾段与结束标记；SQLite 恢复遮蔽状态，公开窗口没有密钥正文，状态在结束标记后关闭。 |
| 在 Chrome 验收 #202 的单文件候选，再等待受保护交付与重启回读 | 固定候选检查成功并交付；两个进程的输出保留 | 候选 6fe950f8c71fc78619ff0c8b0551b48e4bd4ec11 经 [PR #203](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/203) 合并为 6f165a18dc607fb5096b3c0c4f6dbeb2b0bd3f65。Actions 44180356441 预设失败后同 SHA 重跑成功；acceptance-check 与 forgeos/local-ci 均 SUCCESS。任务 DONE、Issue CLOSED，重启后历史一致。 |
| beta #204 的原生 Claude 写入九行夹具、只读前五行，提出是否加脚注的业务问题；在 WAITING_INPUT 重启控制面和 Worker，Chrome 答复不加脚注且仅续读后四行 | 检查点、Owner 答复、工作区及 Session 连续；替代进程继承遮蔽状态 | 执行 #1、Session c862106e-b547-4edd-88a0-455a1d975f4c 不变，原生 PID 22560 → 34260，Worker 35516 → 12044。替代进程唯一 Read 结果无开始标记、含合成密钥尾段和结束标记；公开窗口遮蔽该正文，普通尾部文本保留，SQLite 私钥状态由 true 转 false。 |
| 核对 Claude 精确候选，在 Chrome 验收并完成交付，再重启回读 | 九行夹具未被答复后的回合修改，历史与最终回执一致 | 候选 ac031e6ae6bfff5ed8f98092356e5e02b574027f 经 [PR #205](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/205) 合并为 b8cc1907c16022e45e348557c9e989d532ea00d9。Actions 44181570854 预设失败后同 SHA 重跑成功，两项必需检查 SUCCESS；任务 DONE、Issue CLOSED，Chrome 重启回读保留两个 PID 的输出。 |

Claude 的首次验收脚本误把业务答复的成功状态码写成 204，接口实际按合同返回 200；答复已成功入库。核对数据库、节点日志和原 API 地址的 SQLite 后，修正脚本并从原执行继续，没有重复导入 Issue、重复提交答复或创建新执行。原生首回合一次 Write 失败后重试成功，失败事件仍在历史内。

本项仅增加真实验收证据和文档；实现沿用 dbbc3d9，307 项 PostgreSQL/Docker 回归、静态检查及 Web 构建结果仍适用。分别核对原生会话文件中替代进程启动后的工具结果、SQLite 状态、Chrome 页面和最终回执。本地证据目录为 tool_output_appserverresume、tool_output_clauderecover，均记录 done、restarted_done、native_replacement_tail_verified 为 true。

Claude 本项是业务问题检查点恢复，不代表活动工具强制中断或整机重启已经验收。增量输出、完整归档、其余角色输出及全路线最终验收仍待完成；外部设备、受限身份和独立审查者继续待验收。

上述 App Server 与 Claude 恢复验收对应 ForgeOS 提交 80ecc0cc7c2b1300a82c38f761f7d6f89a8714f5。

<a id="app-command-delta"></a>

### 专题 07、13、16 C.3 · App Server 命令增量输出

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 先从本机原生 App Server 探测一次命令：打印首行、等待 12 秒、打印尾行 | 原生协议确实在命令完成前发送文本 | item/commandExecution/outputDelta 在 8.64 秒收到首行，20.82 秒才收到 item/completed。最初直接探测继承了宿主环境而无法创建执行进程；改为与正式 Worker 相同的环境白名单后跑通。 |
| beta #207 写入四行合成夹具，再在单一前台命令中分块输出合成凭据和私钥，中间等待 45 秒；从 Chrome 查看并重启控制面 | 命令仍运行时看到安全片段，调用身份与窗口连续 | 原生 PID 38132、Thread 01a1eb7a-df6c-7611-9fe9-e89af4df017c；页面和 API 的调用 #524、事件 #526 带 partial=true，含 FIRST 与 [redacted]。重启后可回读。按原生 completed_at_ms 核对，首次页面观察早于命令完成 45.61 秒。 |
| 命令完成后核对原生结果、节点窗口、最终回执，在 Chrome 验收并等待交付 | 原生包含真实合成样本，公开记录没有样本正文；收到完成事件后不再标为片段 | 最终调用仍为 #524、事件 #532，partial 缺省；FIRST 与 LAST 保留，分块 password 及 fake-stream 私钥正文被遮蔽，SQLite 与最终窗口一致。候选 f7b801048e93b371913b681ab3fcf5e00f4e0736 经 [PR #208](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/208) 合并为 207398f83e11403ca76c18918f7a60cb905fea74。Actions 44183858294 预设失败后同 SHA 重跑成功，两项必需检查 SUCCESS；任务 DONE、Issue CLOSED，重启可回读。 |
| beta #209 在同一命令中读取合成私钥前五行并等待 120 秒；页面确认未完成片段后暂停，重启控制面与 Worker，再恢复 | 已记录片段保留为未完成，替代进程只读后半段时继承遮蔽状态 | Thread 01a1eb7d-5576-75d1-ab87-eaadc8980e33、执行 #1 与工作区不变；Worker 32440 → 35308，原生 PID 41856 → 38124。旧进程的 tool_output.partial=true 保留，后续原生结果没有开始标记却包含密钥尾段和结束标记，公开窗口持续遮蔽；SQLite 状态在结束标记后关闭。 |
| #209 初次验收后，目标因 #207 交付而前进；从页面重新应用差异、验证并验收新 SHA | 严格分支保护阻止旧候选直接合并，新候选重新通过验证与验收 | 原候选 7a19ee165d68e946ce725afb52c0ce62f6945009、[PR #210](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/210) 即使 Actions 44184252347 重跑成功仍为 BEHIND。页面刷新关闭旧 PR，保留原执行，执行 #2 在新基线生成 8a86b8b46f3b8b67893a00fd56bedb3b4b1e4f34，单文件差异与九行夹具相符，验证通过后重新在 Chrome 验收。 |
| 等待刷新候选交付，再重启页面查看原执行的两个进程 | 精确新候选交付，原暂停与替代进程证据不被新验证执行覆盖 | [PR #211](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/211) 合并为 0375dbcbce17814ca4b475229d20e1f44d488f08；Actions 44185263293 预设失败后同 SHA 重跑成功，两项必需检查 SUCCESS。任务 DONE、Issue CLOSED；重启后展开原执行，未完成片段和替代进程的完成输出仍可见。 |
| 分别运行旧控制面／新 Worker、新控制面／旧 Worker，以及最终代码的只读 Planner | 旧协议不收到新状态；新 Planner 可采集增量，重启后历史正确 | 原生 Thread 分别为 01a1eb79-96c7-73c2-badb-78ce02552bb6、01a1eb7a-d873-7450-ba49-ad087c4164ce、01a1eb7d-7325-75d1-94cd-49f533021b00，均得到计划修订 1。两种混合版本只记录完成结果，没有 output 事件；新 Planner 的 SQLite 存在 output 事件。Chrome 重启回读均通过。 |

首个夹具 beta #206 使用 [Console]::Write，被原生 PowerShell 受限语言模式拒绝；其错误输出不算成功的增量验收。改用允许的 Write-Output／Write-Host 在 #207 复验，没有放宽 Sandbox。#206 的原生进程已退出，验收脚本停止时尚未回报正式结果；请求取消后，节点没有可恢复的安全检查点。原租约于 2026-11-18 06:11:31 +08:00 到期后，再次核对 SQLite 的进程退出记录及本机 PID，经 Owner abandon 入口结算，执行 #1 的 abandoned_at 为 2026-11-18T06:11:46.216058+08:00，任务 CANCELLED。未验收、未交付，没有记为 DONE。

刷新验收脚本首次尝试展开旧进程详情时，外层历史执行仍处于折叠状态，浏览器等待超时。修正为先展开原执行，再读取进程详情；沿用已经验证的新候选，没有重复刷新或创建第三次执行。

回归覆盖完整行前不发布、跨块凭据与私钥、凭据状态继承、交错调用与延迟尾段、64 KiB 未完成行上限、8 KiB UTF-8 窗口、SQLite 重开、未完成开始标记后的替代进程遮蔽、无 opt-in 的旧协议，以及真正的子进程在命令完成前报告和拒绝异 Thread／Turn。PostgreSQL 回归验证 partial 与 output 事件绑定、拒绝冒充完成、最终输出连续性和过期租约。最终 PostgreSQL 18／Docker 全包 312 项测试及子测试通过，零失败、零跳过；Go 静态检查、TypeScript 和生产构建通过。

本地证据在 tool_stream_v2_stream、tool_output_streamresume、tool_stream_compat_oldcontrol、tool_stream_compat_oldworker、tool_stream_final_compat_planner；前两项均有原生会话核对、DONE 和重启回读标记。增量窗口按完整行刷新，不是字节级直播；中断时未换行的原始片段不留存，超长行及无法确认边界的恢复会保守遮蔽到明确结束标记。Codex CLI 和 Claude 的其他增量接口、结构化工具结果、完整归档、启动阶段与独立角色输出仍待补齐；全路线最终验收及已列外部条件继续待验收。13–23 个剩余工作包的粗估口径不变。

上述 App Server 命令增量输出实现和验收对应 ForgeOS 提交 9cc8d28add7cc42cdcb8d206c78bd949b14e4cdd。#206 的取消核销只更新验收证据，未改动运行实现。

### 专题 07、13、16 C.3 · 本地环境启动日志

2026-11-18 在真实 PostgreSQL 18、Docker Linux 引擎和 Chrome 中验收。节点 startup-output-node，来源 HEAD 任务 #3，固定提交 d9488baed8d73a3af76f7e6370b2f94bf734359e。镜像 golang:1.26.9-bookworm，实际 ID sha256:d9c68c2c51161e12fd77e4c6320687c9cd86e1af1e3ad6e6cd63ff970641453c。Integrated 在项目隔离网络连接真实 PostgreSQL 16.15，核对数据库日志中的 SELECT marker FROM forgeos_readiness。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 从 HEAD 清单点击 local-mock 启动检查，运行中查看日志并重启控制面 | 就绪前可读安全输出，重启不回退，完成后保留回执 | 任务 #4 首次窗口 33 字节，含 FIRST 与 [redacted]；HTTP 204、DONE，最终 132 字节，页面最终日志与本地文件一致。 |
| 从同一清单启动 local-integrated | 真实依赖就绪后成功，日志保留 | 任务 #5 实际查询数据库，HTTP 204、DONE，132 字节。 |
| 停止依赖数据库，再从页面重新检查 | 失败且保留诊断，不记为成功 | 任务 #6 HTTP 503、readiness deadline exceeded、FAILED，398 字节包含 Database not ready。 |
| 恢复数据库，再次从页面检查 | 新任务成功，失败历史不覆盖 | 任务 #7 HTTP 204、DONE，132 字节；原 #6 仍为失败。 |
| Mock 输出首行后从 Chrome 取消 | 记录取消，保留已观察日志并清理容器 | 任务 #8 CANCELLED，66 字节；Owner 取消接口 204，回执 context canceled。 |
| 新 HEAD #9 将脚本改成输出 stderr 后立即 exit 7，再启动 | 不丢失瞬时失败输出 | 提交 246ceee8f9e234c65a046410a5357c10a423104f 的任务 #10 FAILED，45 字节包含 immediate failure，回执 service exited before readiness。 |
| 重启控制面，从页面回读六项终态 | 正式日志保留，过期实时面板清除 | 六项均通过；output 与 run.json、startup.log 的脱敏内容和字节数一致，container_removed 均为 true，没有合成密码或私钥正文。 |
| 分别运行旧控制面／新 Worker、新控制面／旧 Worker并重启回读 | 未支持日志的组合保持兼容，不补造历史 | 两套独立库的任务 #2 均 DONE，无 output、无 startup.log，页面重启回读无虚构日志。 |
| 用最终 Worker 的 verify-environment 运行成功及立即失败脚本 | CLI、节点文件和 JSON 回执一致 | environment-run-415403625 成功、132 字节；environment-run-2926815011 失败、45 字节。两项 stdout JSON 与 run.json 一致，敏感样本遮蔽，容器均清理。 |

首个夹具在 Windows 写入 CRLF shell 脚本，固定提交 f2fc535c54ec523bfd9c5a8c71f157431563db09 的来源 #1／启动 #2 因 Linux 路径含回车失败。该失败保留，控制字符行被遮蔽；修正为 LF 后新建 HEAD 检查，没有改写旧清单。首次重复 HEAD 请求按幂等约定返回旧任务，验收脚本随后沿用正式页面的 retry_task_id 创建 #3。

回归覆盖完整行前不落盘、分块凭据及私钥、凭据轮换、超长行、无换行尾部、脱敏器后续失败时保留已存前缀、真实 Docker 正常／立即退出／取消，以及租约、CI 名额、固定提交、环境和运行身份绑定。拒绝同字节数改写与终态删除、回退已观察输出。最终 PostgreSQL 18／Docker 全包 317 项测试及子测试通过，零失败、零跳过；Go 静态检查、TypeScript 和生产构建通过。

本地证据目录 startup_output_live（proof.json、cli-proof.json）、startup_output_compat_oldcontrol、startup_output_compat_oldworker，回归报告 startup-output-final-regression.jsonl。Docker 回归需包含 curl 和 timeout 的 Linux 镜像，当前默认 golang:1.26.9-bookworm，可用 FORGEOS_STARTUP_IMAGE 指定预备镜像。启动日志为有界脱敏记录，尚未提供完整中央归档或到期清理。其他适配器增量、结构化工具结果、独立角色输出及全路线最终回归仍待完成；已列外部条件继续待验收。

上述启动日志实现与验收对应 ForgeOS 提交 0497b7d0c94862d0f44a3401101ef12ffdeb1fc9。

### 专题 07、13、16 C.3 · 独立 Management 计划审核进程

2026-11-18 从私有 beta #2 的只读讨论接入三个独立验收库，在 Chrome 起草计划并启动真实 Management。这里验证审核及历史，不向讨论 Issue 提交代码或合并。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在任务 #1 的页面发布计划修订 1，启动独立 Management 并查看进程 | 只读权限不变，审核结束前可定位原生进程 | 节点 qa-management-current、PID 22896、Codex 0.169.0-alpha.11.1、Thread 01a1eba6-5794-7bc2-aff9-a126f300c6bc。核对原生参数 read-only／never 和父进程，Chrome 显示同一身份。 |
| 原生审核仍运行时重启控制面，再等待结论 | 已观察进程保留，最终输出和审核会话一致 | 控制面重启回读原 Thread。退出码 0，公开输出与 SQLite 完全相同；结论 D2／revise，正确指出 README 修改超出原 Issue 的 Discussion only。 |
| 分别运行旧控制面／新 Worker、新控制面／旧 Worker | 保留原审核，不补造缺省日志 | Thread 01a1eba6-9ae8-7860-af23-115260d2b632、01a1eba6-bcfe-73a0-b727-93d514069f75 均获得 revise；两种组合没有 review_processes，重启后页面仅显示原有审核回执。 |
| 从页面发布修订 2，核对专属原生子进程后直接终止 | 保存失败退出，不生成成功审核 | PID 44936、Thread 01a1eba8-addf-76b3-b563-5eb33f8cac1f 退出码 15；页面保留退出和会话，修订 2 无 review。 |
| 从页面发布修订 3，由同一 Management 守护进程继续 | 新修订有独立会话，失败历史不覆盖 | PID 40148、Thread 01a1eba8-b773-72e3-9b38-c01667e7b63b 成功退出，得到 D2／revise；修订 2 的失败记录保持相同。 |
| 再重启控制面，分别打开三个修订 | 历史归属正确，公开消息可回读 | Chrome 分别展开三个进程并核对 Thread、退出及实际存在的输出；API 的各版 review_processes 与原记录相同。 |

验收脚本起初对表单文本作了不正确的精确匹配，发布前定位超时；修正为实际文本框标签。发布成功后首次断言误写 201，接口合同为 200，核对已存修订后继续原任务。重启回读脚本曾在已登录标签页寻找登录框，改用新标签页后复验，没有重复模型审核或重复发布。

PostgreSQL 18／Docker 全包 322 项测试及子测试通过，零失败、零跳过；随后新增的 Management HTTP 角色鉴权、未知字段、多 JSON 及超限正文专项通过。测试包含真实测试子进程的运行中消息、成功及非零退出、脱敏、旧组件无采集，以及 PostgreSQL 项目／修订／租约绑定、重放、改写拒绝、会话一致性、过期重领保留两次尝试。Go 静态检查、TypeScript 和生产构建通过。证据目录 management_process_current、management_process_oldcontrol、management_process_oldworker；回归报告 management-process-regression.jsonl。

本项覆盖独立计划审核的进程和公开消息观察，不等同于其 Daemon 崩溃恢复、完整会话归档或其他独立角色已验收。原生 MCP 服务在正式 Worker 中仍被禁用，结构化 MCP 结果的真实验收依赖服务授权接入；没有为取证放宽现有工具权限。全路线其他工作及已列外部条件继续待完成。

上述独立 Management 计划审核实现与验收对应 ForgeOS 提交 492f1d9991ba4f720be0684b8b34a8e038dba494。

### 专题 07、08、13、16 C.3 · 共享资料审核进程

2026-11-18 从 Chrome 的固定资料修订起草两份脱敏副本，在专用来源项目运行真实 Management。接收方节点只授权项目 #2，来源项目为 #1。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 页面起草副本 #1，等待审核进程，核对原生参数并在回合中重启控制面 | 只读权限不变，运行观察和最终回执连续 | qa-share-current、PID 13540、Thread 01a1ebae-c3fd-7a90-afca-465e9c727964，read-only／never，Codex 0.169.0-alpha.11.1。重启保留原 Thread；成功退出、D3／approve，公开输出与 SQLite 相同。 |
| 在来源页发布该副本，用只授权接收项目的节点读取，再从来源页撤销 | 只发布审核过的副本，私有进程不随之披露 | 接收方仅获得 id、title、content、digest、published_at；无原资料中的客户标识及原生会话。撤销后接收列表为空，来源审核与进程保留。 |
| 创建副本 #2，原生回合仍运行时从 Chrome 撤销 | 已占名额保留到实际结束；不发布已撤销内容 | PID 47168、Thread 01a1ebb0-328d-7862-a56e-7ed7249670b5；撤销前后 reserved 均为 1。退出码 0、退出回报保留，完成回报后名额归零，副本仍 revoked、无 review，接收方为空。 |
| 重启后打开两份副本及来源列表，使用接收方节点访问来源记录 | 来源历史可读，接收方被隔离 | 两份页面进程与 API 一致；来源列表含对应历史。接收方读取来源回执及列表为 401，读取未授权项目内部经验为 403。 |
| 旧控制面／新 Worker、新控制面／旧 Worker分别审核并重启回读 | 兼容旧协议，不补造进程 | Thread 01a1ebb0-8138-7ac3-80ee-7e42ea54bccb、01a1ebb0-7ef8-7903-b890-39ffcd1a1a0b 均完成审核，页面没有虚构进程，原审核保持可读。 |
| 将上一项真实计划审核库从迁移 98 升至 99，并从 Chrome 回读修订 1–3 | 共享存储扩展不破坏已有计划进程 | 原三条 ID、节点、完整进程 JSON 均与迁移前相同；两个成功回合和退出码 15 的失败回合都可展开，原生会话与实际输出保持一致。 |

首次接收方请求使用旧式无项目读取授权的节点令牌，按规则返回 403；为验收配置只授权项目 #2 的具名节点后，从已发布副本继续，没有重复审核或重新创建副本。新增回归的首次失败来自测试解码结构遗漏 review_processes 的 JSON 标记，修正测试后重跑。

最终 PostgreSQL 18／真实 Docker 全包 326 项测试及子测试通过，零失败、零跳过；报告为 share-process-final-regression.jsonl。回归覆盖计划与副本目标互斥、跨项目与租约拒绝、会话和退出绑定、改写及重复回报、接收方字段隔离、撤销后实际退出、名额归还以及过期重领保留两次尝试。Go 静态检查、TypeScript 和生产构建通过；真实原生回合、源端浏览器、接收方授权入口、SQLite、控制面重启及旧库迁移均已核对。证据位于 share_process_current、share_process_oldcontrol、share_process_oldworker，以及 management_process_current/share-migration-proof.json。

采集仍为有界公开消息窗口；共享审核的 Daemon 崩溃补报与原会话恢复、其他独立角色及完整归档仍待补齐。既有外部条件及全路线最终验收保持待验收。

上述共享资料审核实现与验收对应 ForgeOS 提交 8adf7eaf2c806dc86a3ea5d70d6967651dc9247f。

### 专题 07、13、16 C.3 · 仓库分析进程与固定来源重试

2026-11-18 在真实 PostgreSQL 18、Chrome 和独立 Codex Management 中验收。节点 qa-analysis-current；先由普通 Worker 完成本地仓库 HEAD 任务 #1，再从页面读取清单并点击“归纳入口文档”。固定提交 2c2f70f73a282218512a02dd2031cd6102bcaa72。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 页面派发归纳，核对原生子进程，再终止该进程 | 保留实际失败，不把退出当作报告成功 | PID 40788、Thread 01a1ebbb-1870-74a3-8431-9669c5c481e4，Codex 0.169.0-alpha.11.1，read-only／never；退出码 15，状态 FAILED、agent_run_failed，无 analysis。 |
| 将原克隆推进到新提交，再从 Chrome 点击“重新归纳” | 沿用原清单来源，新增尝试，不覆盖失败 | 克隆新 HEAD 82c9e0d25e4833772f269495a1cb7abae2ffdc75 的替换标记未进入报告；第二次为 PID 1192、Thread 01a1ebbb-32d6-7c20-add9-48f590dd2ed3。 |
| 第二次回合仍运行时重启控制面 | 原观察保留，结果与退出会话一致 | 重启后原 Thread 可查，随后退出码 0、DONE；用途、Go 技术栈、清单、启动及测试命令分别核对归档原文或清单摘要，来源 blob 与原提交一致。 |
| 比对节点 SQLite、API 和 Chrome，再重启回读 | 两次进程都保留，成功公开消息一致 | 失败记录完全相同；成功窗口与 SQLite 相同，页面展开可读；重启后 analysis 和 processes 保持相同。 |
| 旧控制面／新 Worker、新控制面／旧 Worker分别从页面派发并重启回读 | 原分析协议兼容，不补造进程 | Thread 01a1ebbb-a784-7e53-90af-a700776df1b2、01a1ebbb-a6e8-7601-8ade-d3c55362711c 均完成真实分析；页面可读来源与结果，无虚构进程。 |
| 将真实计划审核与共享审核库从迁移 99 升至 100 | 既有历史完整可读 | 三版计划和两份副本的 ID、节点及完整进程 JSON 均未改变，Chrome 分别展开原会话和实际存在的输出。 |

首次验收断言把包清单摘要也当作 Git 原文，错误拒绝了 go.mod 清单名称这一合法来源；按 basis 分别核对摘要与原文，并核对所有 blob 后通过。重启回读脚本首次遗漏“读取仓库清单”的页面操作，补齐后重读已有结果，没有重复派发模型回合。

PostgreSQL 18／真实 Docker 全包 331 项测试及子测试通过，零失败、零跳过，报告 analysis-process-regression.jsonl。回归覆盖实际测试子进程的成功、非零退出、无效引文、旧协议，以及项目／租约／目标互斥、失败前必须退出、成功会话一致和过期／失败／重试三次历史。Go 静态检查、TypeScript 和生产构建通过。真实验收证据目录 analysis_process_current、analysis_process_oldcontrol、analysis_process_oldworker；旧库迁移证据为 management_process_current/analysis-migration-proof.json 和 share_process_current/analysis-migration-proof.json。

本项保存有界公开消息及进程历史。初始化报告快照仍保存分析结果，进程历史从来源任务回读；完整会话归档、独立角色 Daemon 崩溃补报和恢复、其他角色采集、全路线最终验收及已列外部条件继续待完成。剩余 13–23 个工作包的粗估口径不因本运行时子项而缩减。

上述仓库分析进程与固定来源重试对应 ForgeOS 提交 670e84fef48a9911c5fb61519fb0c2ac343ba616。

### 专题 05、07、13、16 C.3 · 业务问题建议进程

2026-11-18 在真实 PostgreSQL 18 和 Chrome 中，使用独立数据库还原既有真实候选 d689d8e011d2c9e1e49e93cfa632d31b132c2228 的待验收状态。通过 Owner API 在任务 #1 提出问题，再运行独立原生 Management 并从页面答复；本轮未修改或重复交付该候选。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 问题 #1 派发后核对原生进程，在回合中重启控制面 | 进程属于独立建议回合，最近观察可续接 | qa-advice-current、PID 38568、Thread 01a1ebc3-36bd-77d1-85da-f8d62f95e70c；read-only／never，Codex 0.169.0-alpha.11.1。重启后同一进程完成，退出码 0。 |
| 读取 D2 建议、节点 SQLite 和待答页，再从 Chrome 答复 | 来源有效，进程、建议及人工答复分别保留 | 建议引用 requirement:1:body 中 Document the exact HTML title from docs/queue-preview/index.html；原生会话不同于实现会话，公开消息与 SQLite 完全一致；待答页和任务页可展开同一进程，Owner 答复后建议原样保留。 |
| 问题 #2 的原生回合仍运行时从 Chrome 提交 Owner 答复 | 实际退出可回报，迟到建议不能覆盖 Owner | PID 43816、Thread 01a1ebc3-6c26-7f13-8cca-3fbeb1c39e20，实际退出码 0；无 management_advice，Owner 答复保持，完成回报后账号占用归零。 |
| 问题 #3 确认原生子进程后直接终止，再从页面答复 | 失败不生成建议，实际失败和人工处理都可追溯 | PID 37696、Thread 01a1ebc4-3d63-71b1-a8db-9e1d7c15ead5，退出码 15；没有建议，Owner 答复成功。重启后失败进程和答复均相同。 |
| 重启控制面回读问题 #1、#2 | 所有独立会话与各题答复归属不变 | Chrome 展开两条成功退出，核对原生会话和实际存在的公开输出；API 完整问题记录与重启前一致。 |
| 旧控制面／新 Worker、新控制面／旧 Worker分别运行并从 Chrome 答复 | 保持旧建议协议，不伪造历史进程 | Thread 01a1ebc3-b9db-7aa1-b5e0-9a7eb283b3a0、01a1ebc3-b900-7cb3-b14d-e5b8e2ca6d01 均获得真实 D2 建议，答复独立，重启可读且无虚构进程。 |
| 将上一项真实仓库分析库从迁移 100 升至 101 | 新问题关联不损伤分析尝试 | 两条进程的 ID、节点及完整 JSON 原样保留；Chrome 可展开退出码 15 的失败与退出码 0 的重试，分析结果完全相同。 |

PostgreSQL 18／真实 Docker 全包 335 项测试及子测试通过，零失败、零跳过，报告 advice-process-regression.jsonl。回归包含实际测试子进程的成功／非零退出、旧协议，以及项目和目标互斥、过期重领历史、建议前退出和会话校验、待答页历史、D1 引用决定及 Owner 先答复时不能提前归还活动名额。Go 静态检查、TypeScript 和生产构建通过。证据目录 advice_process_current、advice_process_oldcontrol、advice_process_oldworker；迁移证据 analysis_process_current/advice-migration-proof.json。

本次真实原生进程验收覆盖 D2 和 Owner 答复，D1 的新增进程门禁由数据库回归验证；原有 D1 在线引用验收见前文。异常回合没有建议完成回报，名额仍按原租约期限回收，不宣称已实现独立 Daemon 的失败补报或原会话恢复。采集为有界公开消息，其他独立角色、完整归档、全路线最终验收及已列外部条件保持待完成。

上述业务问题建议进程实现与验收对应 ForgeOS 提交 0c5d47a2cd1113ffd0342e3fac0f7323c0427942。

### 专题 07、11、13、16 C.3 · 预览反馈分类进程

2026-11-18 在独立 PostgreSQL 18 库还原既有真实候选 d689d8e011d2c9e1e49e93cfa632d31b132c2228 的待验收状态，从真实静态预览检查页面和指南，再经 Chrome 提交反馈。该候选本轮未重复交付；任务保持 WAITING_ACCEPTANCE。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 在真实预览核对标题与 H1，提交反馈 #1，观察原生分类进程并在回合中重启控制面 | 反馈、候选和进程可关联，重启保留原观察 | qa-feedback-current、PID 26448、Thread 01a1ebc9-133a-7193-ac43-ba28304874d9，read-only／never，Codex 0.169.0-alpha.11.1；退出码 0，窗口与节点 SQLite 完全相同。 |
| 回读首条分类及固定需求 | 分类引用实际观察，不把局部观察误当完整候选验收 | 结果 blocked：只观察了预览页，未证明需求中的新增指南满足内容与长度要求。引用为观察原文，固定候选及需求修订可回读，任务仍待验收。 |
| 从同一预览实际读取 reviewer-sources.md，核对固定 SHA、H1、HTML 标题、容量句、来源路径及少于 150 词，再提交反馈 #2 | 新观察覆盖指南要求，保留与首条不同的反馈 | 浏览器取回 HTTP 200 和同一 X-ForgeOS-Commit，所有内容断言通过；以 /reviewer-sources.md 路径提交完整观察。 |
| 确认反馈 #2 的原生子进程后终止，等待原租约自然到期 | 不生成失败分类；到期后同一反馈由新会话重领 | PID 11480、Thread 01a1ebca-ecd9-7160-9979-0d47afbfa742 退出码 15。未更改数据库截止时间，也未手动派发；六分钟租约到期后 PID 35256、Thread 01a1ebd0-6c7c-7cc3-869b-9e7820ece187 完成，退出码 0、acceptance，名额归零。 |
| 重启后从 Chrome 展开两条反馈的全部进程 | 失败与重试不相互覆盖，分类会话与成功退出一致 | 三条进程身份与原回执一致；第二条反馈保留实际失败和新会话成功，输出与 SQLite／API／页面相同；分类引句均属于原观察。 |
| 旧控制面／新 Worker、新控制面／旧 Worker完成分类并重启回读 | 旧协议兼容，不补造原生历史 | Thread 01a1ebc9-5424-7cb1-b4fc-9a4f5b009dbf、01a1ebc9-5419-7052-b356-cb70dd40486d 均完成真实分类；页面保留分类及固定候选，无虚构进程。 |
| 将真实问题建议库从迁移 101 升至 102 | 反馈关联不破坏已有问题历史 | 三条问题进程的 ID、节点和完整 JSON 未改变，Chrome 可展开两条成功、一条失败；原建议及三条 Owner 答复保持。 |

首轮脚本预设标题观察一定属于 acceptance，实际固定需求是新增指南，原生分类合理返回 blocked；核对原需求和理由后保留该结论。补充的指南观察从真实页面读取后另行提交，没有改写首条反馈或重跑已完成分类。脚本续跑时曾错误替换预览的能力主机名，得到 404；恢复原主机和绑定端口后读取成功，没有放宽预览鉴权。

PostgreSQL 18／真实 Docker 全包 339 项测试及子测试通过，零失败、零跳过，报告 feedback-process-regression.jsonl。回归覆盖进程退出先于分类结果、非零退出无分类、旧协议，以及目标互斥、负编号和多余修订拒绝、项目与租约绑定、失败／重领历史、实际退出及会话匹配、重复分类和原文校验。Go 静态检查、TypeScript 和生产构建通过。证据目录 feedback_process_current、feedback_process_oldcontrol、feedback_process_oldworker；迁移证据 advice_process_current/feedback-migration-proof.json。

本项覆盖反馈分类的有界公开消息和进程历史。自然到期重领会创建新会话，不等同于独立 Daemon 崩溃恢复；完整会话归档、其他独立角色、全路线最终验收及已列外部条件仍待完成。

上述预览反馈分类进程实现与验收对应 ForgeOS 提交 e9bae2aafb81e9e95b9c4dcc4c7c58d538a9caa2。

### 专题 07、08、13、16 C.3 · Curator 提取与 Management 知识审核进程

2026-11-18 从 Chrome 导入 queue-lab 修订 1，内容是一次恢复演练前后核对 offset 274 的观察。在独立 PostgreSQL 18 库运行真实 Curator 和 Management，使用相同节点名 qa-knowledge-current 及同一私有状态目录，分别持有角色凭据。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 页面导入资料并展开知识提取，运行 Curator，回合中重启控制面 | 只读身份和提取进程保留，原文引句有效 | PID 46944、Thread 01a1ebd7-273d-7f83-8eb1-a05b2b1db9c6，Codex 0.169.0-alpha.11.1，read-only／never；退出码 0，提取引句均来自该修订，未经审核前经验列表为空。 |
| 在同一节点和状态目录运行 Management，回合中再次重启控制面 | 审核与提取独立，公开消息分别保存 | PID 41768、Thread 01a1ebd7-500f-7c40-8e62-607e9cfcf7ea，退出码 0、D1／approve；两个原生会话不同，两个 SQLite 文件分别保存各角色公开窗口，并与 API 相同。 |
| 回读当前项目经验 | 只有获准事实进入经验，进程不随之发布 | 获得同一来源的事实；返回内容不包含两个原生会话及进程历史字段。 |
| 从 Chrome 编辑修订 2，只声明议程且没有观察，运行 Curator | 空提取不触发审核，新修订不沿用旧批准 | PID 43156、Thread 01a1ebd7-803a-7f40-85fc-2c7d8d8bdef5，退出码 0、facts 为空、无 review；当前项目经验为空，修订 1 的提取和审核仍保留。 |
| 从 Chrome 保存修订 3，确认 Curator 原生子进程后终止 | 不生成提取或审核，保存真实失败 | PID 26960、Thread 01a1ebd7-a548-7862-b8a5-1c2088fca526，退出码 15，无 extraction／review。该修订仍待提取，本次未启动新的重领回合。 |
| 重启后从页面分别打开三个修订 | 来源、角色及实际退出归属不变 | 四条原生进程均可展开，原始记录与 API 相同，存在的公开消息可回读。 |
| 角色令牌互访进程端点、伪装另一角色、读取 Owner 历史 | 角色隔离，来源历史不由角色令牌公开 | 跨令牌端点为 401；本角色令牌报告另一角色为 403；两个角色读取 Owner 来源历史均为 401。 |
| 旧控制面／新 Worker、新控制面／旧 Worker完成两阶段并重启回读 | 保持原协议，不补造进程 | 提取／审核会话分别为 01a1ebd7-863d-7d40-abb5-b31eae6b0ea9 / 01a1ebd7-af8d-7912-bb14-9e2aedd36db2；01a1ebd7-84b6-7622-9f3e-bf9cfcc82c93 / 01a1ebd7-ac16-7510-a782-974fe2f44c3c。两组均 D1 approve，原回执可读，无虚构进程。 |
| 将真实反馈库从迁移 102 升至 103 | 多角色、多回合扩展不损坏已有单回合历史 | 三条反馈进程的 ID、节点及完整 JSON 不变；Chrome 回读首条分类和第二条反馈的失败／自然到期重试。 |

同租约引文修复通过实际测试子进程和 PostgreSQL 回归验证：第一次非连续引句被拒绝，第二次原生尝试另有 Session 和更大的 instance；未确认第一次成功退出、跨节点、重复会话或第三次尝试均不能替代合法过程。最终提取匹配最后一次成功回合，已结束的第一回合不能覆盖结果。本轮真实模型没有主动触发两次引文修复，不把测试夹具记作该分支的真实模型证据。

PostgreSQL 18／真实 Docker 全包 346 项测试及子测试通过，零失败、零跳过，报告 curation-process-regression.jsonl。另覆盖角色路由、项目／来源／租约绑定、先退出后结果、提取与审核隔离、失败及过期重领历史、结果重放和旧组件兼容。Go 静态检查、TypeScript 和生产构建通过。证据目录 curation_process_current、curation_process_oldcontrol、curation_process_oldworker；迁移证据 feedback_process_current/curation-migration-proof.json。

本项保存有界公开消息及各次原生进程。失败修订 3 保持待提取；独立 Daemon 崩溃补报、原会话恢复、完整归档、其余独立角色和全路线最终验收继续待完成，已列外部条件保持待验收。

上述知识提取与审核进程实现和验收对应 ForgeOS 提交 48177987e22586eb3f6feed3828d18791f0640d2。

### 专题 07、10、13、16 C.3 · 候选 Test Agent 与 Management 覆盖审核进程

2026-11-18 在隔离 PostgreSQL 18 库还原 beta #163 的真实候选 d689d8e011d2c9e1e49e93cfa632d31b132c2228，从 Chrome 请求测试计划。未再次写入 GitHub 或交付候选。原生 Codex 0.169.0-alpha.11.1，两个独立角色共用一个模型名额及相同节点名，使用各自 API 凭据和独立 SQLite。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 修订 1 的 Test Agent 运行中，从 Chrome 请求修订 2 | 旧回合继续计入名额，实际退出保留，但不发布过时计划 | PID 34156、Thread 01a1ebe2-73ec-70c3-90cf-2f1778587d2f、退出码 0；替代前后 reserved 均为 1；修订 1 的 plan 仍为空，后续修订才取得生成结果。 |
| 完成修订 2 的生成和覆盖审核，审核中重启控制面 | 独立原生身份、固定候选和计划摘要不变 | Test Agent：PID 27116、Thread 01a1ebe2-bde9-7841-a1bb-52ad0c966fdf、退出码 0；Management：PID 41904、Thread 01a1ebe3-0b40-7b01-be64-5bd9eb7092c7、退出码 0。运行中 Chrome 自动显示进程，原生参数为 read-only／never；覆盖结果 covered，两角色公开消息与各自 SQLite 一致。 |
| 从 Chrome 请求修订 3，Test Agent 运行中重启控制面后终止已确认的子进程 | 保存实际失败，不生成计划或覆盖结论，归还名额 | PID 43324、Thread 01a1ebe3-46ca-7453-a162-2e0f04f73540、退出码 15；状态 plan_failed，plan／coverage 均为空，reserved 为 0。 |
| 请求修订 4，生成完成后终止 Management 原生子进程 | 保留成功计划，覆盖审核失败可追溯 | Test Agent：PID 13248、Thread 01a1ebe3-6eb2-7072-abd2-f929b8885ce7、退出码 0；Management：PID 47920、Thread 01a1ebe3-bc5c-7071-9a4f-d6e6fef9dbbb、退出码 15；状态 review_failed，无 coverage，reserved 为 0。 |
| 重启后从 Chrome 回读四个修订 | 成功、被替代及两角色失败历史分别保留 | 六条原生进程的 API 记录完全相同，页面可展开会话及存在的公开消息。 |
| 角色令牌互访进程端点、伪装另一角色、读取 Owner 历史 | 角色与历史权限隔离 | 跨端点 401，本角色伪装另一角色 403，两个角色读取 Owner 测试计划历史均 401。 |
| 旧控制面／新 Worker 与新控制面／旧 Worker | 原有计划、覆盖结果和历史仍可用，不补造进程 | 两组均生成 covered 结果，重启后可从 Chrome 回读；进程数组为空。 |
| 知识提取库从迁移 103 升至 104 | 新增候选来源不破坏旧角色历史 | 四条既有提取／审核记录的 ID、节点及完整 JSON 不变，Chrome 回读成功、空提取和失败三版来源。 |

旧控制面组合的 Test Agent／Management 会话为 01a1ebe2-aca9-7c13-a9fe-e3ce85d322aa / 01a1ebe3-036b-7201-b030-3e32176b7db7；旧 Worker 组合为 01a1ebe2-acbe-7210-b489-23db7cd258ba / 01a1ebe3-0505-7021-95fc-1764208a32b6。

PostgreSQL 18／真实 Docker 全包 355 项测试及子测试通过，零失败、零跳过，报告 test-process-regression.jsonl。新增专项覆盖运行中失败回报不得释放名额、旧修订实际退出、项目／角色／租约／目标互斥、结果会话匹配、幂等回报、独立角色进程退出先于结果以及旧协议；Go 静态检查、TypeScript 和生产构建通过。证据目录 test_process_current、test_process_oldcontrol、test_process_oldworker；升级证据 curation_process_current/test-process-migration-proof.json。

本项验收进程观察及结果绑定；还原候选保持 WAITING_ACCEPTANCE，生成用例未执行，case_results 为空，不将 covered 当作候选通过。失败修订保留失败状态，被替代修订不补造计划。Reviewer 实时进程、独立 Daemon 崩溃补报和原会话恢复、完整归档及全路线最终验收仍待完成；已列外部条件保持待验收。

上述候选测试计划与覆盖审核进程实现和验收对应 ForgeOS 提交 64b281a2019916f9b012e2b3b2bc7299a5db04fb。

### 专题 07、10、13、16 C.3 · 独立 Claude Reviewer 进程

2026-11-18 在隔离 PostgreSQL 18 库还原 beta #163 的真实候选 d689d8e011d2c9e1e49e93cfa632d31b132c2228，从 Chrome 请求独立代码审查。使用 Claude Code 2.1.310 的原生事件流；本轮没有再次写入 GitHub 或交付候选。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 从任务请求审查，确认原生子进程和 Session 后终止 | 保存实际退出，不生成审查报告，归还并发名额 | PID 36716、Session d18702d9-9045-48d9-85aa-2f270224a9b3、退出码 15；report 为空、failed_at 已记录，reserved 为 0。此回合尚未产生公开文本，页面不补造消息。 |
| Chrome 点击重试，读取运行中原生进程，回合中重启控制面 | 新尝试绑定同一候选，旧失败不被覆盖 | PID 45088、Session c7e3be7b-6e46-4d49-92f6-0356e0aadc34；运行中页面可见，重启后仍回读同一会话。原生参数 stream-json／verbose、dontAsk、tools 空列表，禁扩展和浏览器。 |
| 原生重试完成并核对公开消息 | 真实退出先于成功报告，消息与节点日志一致 | 退出码 0；原候选及基线不变，报告无发现，与实现会话不同。SQLite、API 和 Chrome 的公开消息完全一致，reserved 为 0。 |
| 再次重启后打开审查记录 | 失败与成功原生身份、配置和消息可回读 | 两条进程 JSON 不变，页面可展开两次 Session、存在的公开消息及无工具权限配置。 |
| 角色令牌互访、伪造另一个角色目标、读取 Owner 历史 | 角色和项目历史权限隔离 | 跨端点 401；Management 冒充 Reviewer、Reviewer 报告计划审核目标均 403；两个角色读取 Owner 审查记录均 401。 |
| 旧控制面／新 Worker、新控制面／旧 Worker | 原最终 JSON 审查继续可用，不补造进程 | 两组原生审查均完成，无发现，权限配置相同，重启后 Chrome 可回读报告，进程列表为空。 |
| 测试计划库从迁移 104 升至 105 | 新增 Reviewer 来源不损坏已有多角色历史 | 六条计划生成／覆盖审核记录的 ID、节点和完整 JSON 不变；Chrome 回读成功、被替代及两角色失败四版记录。 |

旧控制面组合的原生 Session 为 02e25de9-a535-49b1-81c7-98588ea29d90；旧 Worker 组合为 a084a101-3457-4d17-82e3-5d6ee5b649c2。真实候选保持 WAITING_ACCEPTANCE；审查无发现不等同于 Owner 验收或重新交付。

专项测试覆盖原生成功、旧协议、非零退出、会话不匹配、缺少终态、缺少结构化结果、错误终态和超大结果。初次 Windows 测试复现上报取消时 SQLite 文件仍由测试 Worker 持有；让短暂本地读取先关闭语句，再结束上报器后，八种场景连续三轮通过且临时日志正常释放。数据库回归覆盖项目／角色／租约／来源互斥、运行中不得失败或成功回报、非零退出不得产生成功报告、重试历史与结果幂等。原有其他角色继续使用共享上报器。

PostgreSQL 18／真实 Docker 全包 367 项测试及子测试通过，零失败、零跳过，报告 review-process-regression.jsonl。Go 静态检查、TypeScript 和生产构建通过。证据目录 review_process_current、review_process_oldcontrol、review_process_oldworker；升级证据 test_process_current/review-process-migration-proof.json。Chrome 脚本在首次失败后的刷新中曾提前选择未出现的按钮而超时；修正等待条件后从同一失败记录重试，没有重做或抹除失败证据。

本项覆盖 Reviewer 有界公开消息、原生进程与报告绑定。用户点击失败重试创建新会话，不能算作独立 Daemon 崩溃补报或原 Session 恢复；这些恢复能力、完整归档和全路线最终验收仍待完成，已列外部条件保持待验收。


上述独立 Reviewer 进程实现与验收对应 ForgeOS 提交 ebb0f475922ab46ed962f7ed733a08a2cb94fe52。

### 专题 07、13、16 C.3 · 独立角色的已完成结果补报

2026-11-18 在隔离 PostgreSQL 18 库还原 beta #163 的真实候选，从 Chrome 导入资料、生成测试计划及请求独立审查。使用正式四类角色 Daemon、原生 Codex 0.169.0-alpha.11.1／Claude Code 2.1.310 和各自凭据；本地代理只在指定最终结果端点注入故障。所有模型均先实际退出，再确认节点 SQLite 已保存原请求，随后结束该 Daemon；重启控制面和同一节点身份，核对原结果的补报。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| Curator 提取完成后阻断首次最终结果，结束 Daemon | 未收到的提取仍可补报，不重新提取 | 控制面原先 extraction 为空；重启后原提取获 204。 |
| Management 知识审核已获控制面接收，但代理丢弃确认，再结束 Daemon | 重新发送相同结果保持幂等，不产生第二次审核 | 重启补报获 204；补报前后原始审核回执完全相同。 |
| Test Agent 完成后阻断结果，从 Chrome 创建修订 2，再重启 | 过时结果不得覆盖新修订，保留可排查记录 | 原修订结果获 409；两版 plan 均为空，私有日志保留删除 lease 字段的冲突结果。 |
| Reviewer 完成后阻断最终结果，再重启 | 已完成原生审查可补报，不重复运行 Claude | 控制面原先 report 为空；重启后原报告获 204。 |
| 再次重启，从 Chrome 回读资料知识提取、旧版测试计划与审查记录 | 成功与冲突来源仍关联原生身份 | 四类原生会话均可展开；任务仍为 WAITING_ACCEPTANCE，没有再次 GitHub 交付。 |

| 原生角色 | 已退出进程 | 补报核对 |
| --- | --- | --- |
| Curator | PID 36428，Thread 01a1ebf5-d1f9-7350-a35a-3e7a86f0bbf7，退出码 0 | 204；补报先于新领取，进程历史不变，没有再次启动模型。 |
| Management 知识审核 | PID 29856，Thread 01a1ebf6-d0fb-7262-9528-08aa0f208a3c，退出码 0 | 204；补报先于新领取，进程历史不变，没有再次启动模型。 |
| Test Agent | PID 34212，Thread 01a1ebfd-1334-7f30-9f66-a39f2dcab8ec，退出码 0 | 409；补报先于新领取，进程历史不变，没有再次启动模型。 |
| Reviewer | PID 26268，Session a2543be3-c2c8-4398-8956-e82318fa1b0e，退出码 0 | 204；补报先于新领取，进程历史不变，没有再次启动模型。 |

九个结果端点分别覆盖断连、已接收但确认丢失、409 冲突及 401 鉴权失败共 36 个子场景，验证原文重报、角色／节点隔离、不同正文不得覆盖相同租约、确认后不重复发送以及清除待发租约。另验证已取消上下文仍保存已准备完成的结果、拒绝非结果端点及旧 SQLite 新增补报表后保留已有原生进程。

PostgreSQL 18／真实 Docker 全包 405 项测试及子测试通过，零失败、零跳过，报告 role-receipt-regression.jsonl。之后新增的旧 SQLite 升级专项单独通过；Go 静态检查通过。本项没有修改前端。真实运行证据 role_receipt_current/proof.json，故障注入及 Chrome 入口脚本 role-receipt-live.py。

首次夹具误把控制台辅助进程当作模型进程而停止，修正为识别原生回合后，从同一已保存的 Curator 结果继续。另一次代理读响应失败时，控制面已领取 Test Agent 租约，但节点未收到完整领取响应，尚未启动模型。观察脚本超时退出后，保留原数据库与节点日志，等待原租约自然到期，再从同一待处理修订继续；没有修改租约期限。最后一轮浏览器检查补上展开进程详情后，从既有记录通过；未重新运行模型。

本项仅保证已写入最终补报表的结果可恢复。领取响应丢失、仍在运行的原生回合、结构化输出尚未入队、退出观察尚未获确认等窗口仍待补齐；不将补报等同于原 Session 恢复。完整输出归档、全路线最终回归及外部设备、受限 GitHub 身份等条件继续待验收。


### 专题 07、16 · 独立角色领取的大响应完整传输

2026-11-18 真实退出补报验收中，Windows Python HTTP 代理在读取 Test Agent 领取响应时出现连接重置；PostgreSQL 已写入六分钟租约，节点没有收到作业，未启动模型。两次原租约自然到期前后均观察到该问题，没有编辑租约期限。检查发现 Test Agent、Curator 领取处理器没有读取 POST 请求体。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 用本机 Go HTTP 服务与 Python urllib、Connection: close 重现未读请求体的大响应 | 区分数据库与 HTTP 连接问题 | 服务不读取请求体时，三个 12,000 字节响应均遭 WinError 10054；先读取同一请求体后，三次均完整读到 12,000 字节。 |
| 在正式领取处理器先按既有 JSON 边界读取空对象 | 返回响应前不遗留请求正文，保留空正文兼容性 | PostgreSQL 专项核对 Curator、Test Agent 的空正文、空对象及尾随空白；多对象、损坏 JSON、未知字段及超限正文返回 400。 |
| 原 Test Agent 租约自然到期后换用修复后的控制面 | 经同一代理完整领取原修订，不改数据库期限 | 同一资料源 10,817 字节；原生 Codex PID 42216、Thread 01a1ec0e-cf7a-7ab1-9907-f41b60e8c537 已启动并退出码 0，随后继续退出补报验收。 |
| 从正式 Curator API 领取三份较大固定资料 | Python 可完整读取合法 JSON | 三个响应各 14,922 字节，来源 ID 2、3、4。只验证传输，未调用模型提取。 |

直接连接旧 Curator 的一次较大响应未重现连接重置，不将它记为失败证据。故障证据来自原 Test Agent 代理和最小 HTTP 探针。现有 Reviewer、Management 已读取其请求体，无需改动。新请求边界及已有测试计划四种角色／版本组合回归通过；Go 静态检查通过。证据为 claim_body_current/proof.json、claim-body-live.py、unread-body-probe.py 和 role_exit_current 中的原生 Test Agent 记录。

此修复避免本次未读请求体造成的传输中断，不保证网络中断后的领取幂等恢复；该能力继续待补齐。


上述独立角色最终结果补报实现与验收对应 ForgeOS 提交 7f8be04aedaaa1a5f975b51f8c81d6644e388ef1。

上述领取请求体修复对应 ForgeOS 提交 24649d95381a099df40fdea599d59baab04ddb62。

### 专题 07、13、16 C.3 · 退出观察先于最终结果补报

2026-11-18 在独立 PostgreSQL 18 库还原 beta #163 候选，从 Chrome 导入资料、生成测试计划及请求独立审查。正式四类角色使用各自凭据、原生 Codex 0.169.0-alpha.11.1／Claude Code 2.1.310；代理只阻断带实际退出字段的进程报告，运行中的观察正常通过。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| Curator 和 Reviewer 已完成原生回合，代理阻断退出报告 | 完成结果不被网络错误丢弃，也不越过退出门禁发送 | 节点同时保留退出观察和原结果；未发送最终结果。结束 Daemon、重启控制面和同身份 Daemon 后，退出与结果依次获 204。 |
| Management 退出已被控制面接收，代理丢弃确认 | 原成功回合仍保存，重报退出保持幂等 | 退出确认丢失时 review 仍为空；重启后重报同一退出，再提交原审核，均为 204。 |
| Test Agent 退出报告被阻断，Owner 创建修订 2 | 先保留真实退出，再拒绝过时结果 | 重启后旧回合退出获 204，结果获 409；两版 plan 均为空，原进程保留，没有覆盖新版。 |
| 再次重启，从 Chrome 展开资料、旧版计划和审查记录 | 原生身份及结果归属保持一致 | 四类会话均可回读，任务保持 WAITING_ACCEPTANCE；没有再次写入 GitHub 或交付候选。 |
| 使用新 Worker 启动上一项验收留下的四份 SQLite | 新增退出队列，不改变已有进程和结果 | 四条原生进程、四条已结算结果完全相同，未重发已结算结果；新退出队列为空。 |

| 原生角色 | 原生退出证据 | 恢复顺序 |
| --- | --- | --- |
| Curator | PID 46032，会话 01a1ec03-498e-74a1-a299-5e70807cdbf5，退出码 0 | 退出 204，结果 204；首个补报请求为退出观察，第二个为原结果，无新模型进程。 |
| Management 知识审核 | PID 25620，会话 01a1ec03-8a42-7282-84be-7e99823ae66b，退出码 0 | 退出 204，结果 204；首个补报请求为退出观察，第二个为原结果，无新模型进程。 |
| Test Agent | PID 42216，会话 01a1ec0e-cf7a-7ab1-9907-f41b60e8c537，退出码 0 | 退出 204，结果 409；首个补报请求为退出观察，第二个为原结果，无新模型进程。 |
| Reviewer | PID 48056，会话 2a5d7e46-9557-4e32-aad7-3040706a881b，退出码 0 | 退出 204，结果 204；首个补报请求为退出观察，第二个为原结果，无新模型进程。 |

真实退出与最终结果在本地均已持久化后才结束 Daemon。恢复前后原生进程列表相同；所有已确认的退出待发正文均被清空。测试计划被替代时仅其最终结果进入去租约的冲突归档，实际退出正常保留。

PostgreSQL 18／真实 Docker 全包 415 项测试及子测试通过，零失败、零跳过，报告 role-exit-regression.jsonl；Go 静态检查通过。专项实测子进程验证四类日志身份在退出报告 503 时保留成功输出、先补退出再补结果及重报正文不变；另验证确认丢失、401 保留待发记录、409 冲突脱租约和按 instance 顺序重放。测试中 Reviewer 的通用队列使用 Codex 测试子进程，真实 Claude 身份由本节原生验收覆盖。最后补强测试查询的错误检查后，相关专项再次通过；期间新增的领取请求体修复另通过真实 PostgreSQL 专项及既有测试计划组合回归。

证据目录 role_exit_current/proof.json；脚本 role-exit-live.py。旧日志升级证据 role_receipt_current/exit-upgrade-proof.json。升级脚本首次未配置 Reviewer 所需仓库根目录，启动校验拒绝；补齐已有目录后通过。真实 Test Agent 两次领取响应丢失，均未启动模型；先保持原 Worker 等待自然到期，复现后定位到未读请求体导致的连接重置。确认没有原生回合后受控结束验收进程，换用领取请求体修复后的控制面，保留原数据库、日志及租约期限；到期后继续同一修订并完成验收。

仍未覆盖 Daemon 在最终结果持久化前退出、领取响应丢失的直接恢复、运行中原生会话恢复、完整输出归档及全路线最终端到端验收；外部条件继续保持待验收。


上述退出观察补报实现与验收对应 ForgeOS 提交 8ffaec8759dc7428862157b293fe715294f49cdd。

### 专题 07、13、16 C.3 · 独立角色领取响应重放

2026-11-18 在隔离 PostgreSQL 18 库还原 beta #163，从 Chrome 导入 claim-lab 资料、生成测试计划及请求独立审查。原生 Codex 0.169.0-alpha.11.1／Claude Code 2.1.310；四类角色各用自己的凭据及节点日志。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 代理让控制面完成领取并保存响应，向节点返回 503 | 已分配作业不丢失，节点没有提前运行模型 | 节点保留相同请求编号；SQLite 尚无原生进程，进程树也没有模型子进程。 |
| 结束该 Daemon，重启控制面后启动同身份 Daemon | 不等六分钟重领，直接拿回原分配 | 首个领取仍使用原路径、正文和编号；控制面回显相同编号及逐字节相同的响应，包括原租约、截止时间和能力字段。 |
| 四类角色运行至完成 | 同一分配只运行一次，仍受原期限限制 | 各有一个原生进程，实际退出码 0，在原租约期限内获得最终结果。 |
| 再次重启，从 Chrome 回读知识提取、测试计划及独立审查 | 原生身份与固定来源保持关联 | 四类会话均可展开；任务仍为 WAITING_ACCEPTANCE，没有再次 GitHub 交付，未把测试计划或审查报告当成 Owner 验收。 |
| 将上一项真实验收库从 105 升至 106 | 新领取表不破坏已有记录 | 四条进程 JSON 完全相同，知识回执、被替代的测试计划和 Reviewer 报告均与升级前相同；Chrome 回读通过，新领取表为空。 |

| 原生角色 | 真实进程 | 领取恢复结果 |
| --- | --- | --- |
| Curator | PID 46896，会话 01a1ec19-210d-7351-a97d-5b4889a00d23，退出码 0 | 领取响应逐字节相同、原租约和截止时间不变；一个原生进程，在原期限内完成。 |
| Management 知识审核 | PID 26920，会话 01a1ec1b-1dae-7000-a793-6c43873494a4，退出码 0 | 领取响应逐字节相同、原租约和截止时间不变；一个原生进程，在原期限内完成。 |
| Test Agent | PID 4992，会话 01a1ec1b-4d3e-7b11-96fb-6eca2405af2a，退出码 0 | 领取响应逐字节相同、原租约和截止时间不变；一个原生进程，在原期限内完成。 |
| Reviewer | PID 26452，会话 7a313d37-3703-4de5-b787-6634d3225a2c，退出码 0 | 领取响应逐字节相同、原租约和截止时间不变；一个原生进程，在原期限内完成。 |

数据库专项验证同一请求六个并发调用只分配一次、控制面 Store 重建后回放相同字节、请求正文变化拒绝、分配后注入错误时租约和回执一起回滚、空轮询之后仍可领取新资料，以及 Management 在多个来源中选择作业。并发检查曾复现空候选占着模型预算锁；改为候选保存点后，第二个事务的 FOR UPDATE NOWAIT 能立即取得该锁。

节点专项覆盖四类角色的 503、确认丢失、截断响应和旧控制面无回显，确认重启后沿用原请求而不是新版能力声明；两个共享日志的 Daemon 并发收到同一分配时仅一个消费成功。正式 HTTP 入口另核对能力字段快照、正文和路径冲突、节点隔离以及错误角色令牌 401。

PostgreSQL 18／真实 Docker 全包 438 项测试及子测试通过，零失败、零跳过，报告 role-claim-final-regression.jsonl；Go 静态检查通过。使用正式新 Worker 启动上一项的四份真实 SQLite，原生进程、已结算退出和最终结果完全相同，新增领取请求表正常，未重报旧结果；证据 role_exit_current/claim-node-upgrade-proof.json。

真实证据 role_claim_current/proof.json；故障注入及 Chrome 入口脚本 role-claim-live.py。迁移证据 role_exit_current/claim-migration-proof.json。首个脚本在 Curator 完成后把下一次空闲轮询误判为原请求未清除；改为核对原请求编号后，从同一已完成结果恢复验证，未再次启动 Curator。受控结束 Daemon 时，代理记录的空闲连接关闭不影响已确认的业务回执。

尚未实现领取确认后、最终结果持久化前的 Daemon 恢复或独立角色的原 Session 恢复；完整归档、全路线最终回归及外部条件继续待完成。旧组件的原有协议由回归覆盖，本节的真实响应丢失恢复要求两端均支持请求编号。

上述领取响应重放实现与验收对应 ForgeOS 提交 a34944823569b22a7c35444b98a8e054b88690fc。

### 专题 07、13、16 C.3 · 模型启动前的独立角色作业恢复

2026-11-18 在隔离 PostgreSQL 18 库还原 beta #163，从实际 Chrome 入口导入 assignment-lab 资料、生成测试计划并请求独立审查。使用正式 Daemon、Codex 0.169.0-alpha.11.1 与 Claude Code 2.1.310。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 领取完整响应后，在模型版本探测处暂停，核对日志后结束 Daemon | 已确认领取的作业在本地保留，模型尚未启动 | 四类角色均有逐字节相同的冻结响应，原请求已移除，started=0，原生进程表为空。仅版本探测使用暂停包装程序，未执行模型回合。 |
| 结束已核实属于该 Daemon 的版本探测进程，重启控制面及原身份 Daemon | 直接使用本地原作业，不重新领取 | 恢复进程使用真实原生可执行文件；首个请求为进程报告，没有新领取；启动标记写为 1。各回合在原截止时间前完成。 |
| 原生进程运行时启动第二个同身份 Daemon | 拒绝重复运行 | 四次均非零退出并记录节点锁错误；每份角色日志仅有一个原生模型进程。 |
| 再次重启控制面，从 Chrome 展开四类记录 | 固定来源与原生会话可回读 | 四条记录均可展开，任务保持 WAITING_ACCEPTANCE；没有执行测试计划中的用例，也没有再次交付 GitHub 候选。 |
| 用新 Worker 打开上一项的四份实际 SQLite | 新表不改历史，不重跑已完成模型 | 原进程、已结算退出和最终结果逐项相同；role_assignment 表已创建，未重发旧结果。 |

| 原生角色 | 真实进程 | 结果 |
| --- | --- | --- |
| Curator | PID 45224，会话 01a1ec2c-ecc6-74c0-8320-35ce9b61c43f | 一个进程，退出码 0；实际退出与最终结果均获 204。 |
| Management 知识审核 | PID 49120，会话 01a1ec2d-1f3b-7ee0-9dcf-db79c59129ed | 一个进程，退出码 0；实际退出与最终结果均获 204。 |
| Test Agent | PID 40952，会话 01a1ec2d-5a03-7ce0-9f22-9c65b99bfb5a | 一个进程，退出码 0；实际退出与最终结果均获 204。 |
| Reviewer | PID 45596，会话 f1ea39c7-1c92-45bb-946d-595b1406ea23 | 一个进程，退出码 0；实际退出与最终结果均获 204。 |

PostgreSQL 18／真实 Docker 全包 445 项测试及子测试通过，零失败、零跳过；Go 静态检查通过。专项核对四类冻结作业的无网络恢复、启动后换 Daemon 明确拒绝、已结算作业不重跑，以及同身份锁排他、不同角色不互锁和退出后可重新取得锁。将 Linux 测试程序交叉编译后，在 Ubuntu 的非 root 用户（UID 1000）实际执行角色专项，73 项测试及子测试通过；包含上述恢复和锁测试，不能据此宣称 Linux 服务账号的模型登录已验收。

证据为 role_assignment_current/proof.json、role-assignment-live.py、assignment-gates.py、assignment-evidence-check.py；旧日志升级为 role_claim_current/assignment-node-upgrade-proof.json，Linux 输出为 assignment-linux.log，全包报告为 role-assignment-regression.jsonl。受控结束 Daemon 时代理的空闲连接被关闭，业务退出与结果已逐项核对为 204。最初直接通过 PowerShell 传递 Linux 测试参数被拆分，改用 Python 参数数组后实际执行通过。

仍待补齐已启动回合的原生 Session 恢复、最终结果入队前的恢复、完整归档及全路线最终端到端验收。已有启动标记的跨 Daemon 作业保留日志并明确停止，不将其记为自动恢复。外部设备、受限 GitHub 身份及独立审查者继续待验收；总工作包估算不因本次细分项单独扣减。

上述模型启动前的作业恢复实现与验收对应 ForgeOS 提交 9564a1adb1869d4a51494328509dc59a948c387d。

### 专题 07、13、16 C.3 · 已完成回合在最终结果入队前的恢复

2026-11-18 在独立 PostgreSQL 18 库还原 beta #163，经 Chrome 导入 completed-lab 资料、生成测试计划及请求独立审查。正式四类角色分别调用本机原生 Codex 0.169.0-alpha.11.1／Claude Code 2.1.310。代理阻断实际退出报告，节点已有成功回合快照、尚无最终结果记录时结束 Daemon。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 核对成功快照及实际退出，在最终结果入队前结束 Daemon | 完成输出在私有日志保留，最终回执确实尚未保存 | 四类角色各有一个成功进程、一个回合快照和待发退出；role_receipts 尚无该结果。 |
| 重启控制面及同身份 Daemon | 先补退出，再使用原输出提交业务结果，不调用新模型 | Curator、Reviewer 的退出与结果均为 204；Management 的退出第一次已被接收但确认丢失，重报幂等，原审核结果随后获 204。 |
| Test Agent 退出后从 Chrome 建立修订 2，再恢复旧作业 | 保留原输出，不能覆盖新计划 | 原退出获 204、原结果获 409；两个修订均未被该旧输出覆盖。冲突归档中的用例正文与保存草案相同，仅按既有 Bind 逻辑补编号及 pending 状态。 |
| 再次重启控制面，从 Chrome 展开资料、旧计划及审查历史 | 原会话与固定来源仍可追溯 | 四类原生会话均可回读；任务保持 WAITING_ACCEPTANCE，无额外 GitHub 交付。 |
| 使用新 Worker 打开上一项四份真实 SQLite | 新表不改变已有历史或重跑模型 | 原进程、已结算退出与最终结果逐项相同；新增回合表正常，未重发旧结果。 |
| 核对完成恢复后的节点日志 | 临时输出随已结算作业清理，审计记录保留 | 四份日志的 role_assignment 和 role_turn_results 均为空；原进程及 204／409 回执保留。 |

| 原生角色 | 原生证据 | 恢复结果 |
| --- | --- | --- |
| Curator | PID 39024，会话 01a1ec36-7b58-7ef3-be4f-bc98ae4c5d50，退出码 0 | 退出 204；最终结果 204；没有新模型进程。 |
| Management 知识审核 | PID 46988，会话 01a1ec36-b5eb-7051-810c-2747cee4fcc5，退出码 0 | 退出 204；最终结果 204；没有新模型进程。 |
| Test Agent | PID 40680，会话 01a1ec36-ebd4-7160-86dd-eda1a27ac39f，退出码 0 | 退出 204；最终结果 409；没有新模型进程。 |
| Reviewer | PID 47344，会话 783d5465-0395-438d-ba9c-d694e67730ad，退出码 0 | 退出 204；最终结果 204；没有新模型进程。 |

专项用实际测试子进程验证四类日志的“成功快照已保存、最终回执未入队”状态，并在替代 Daemon 配置不存在的模型路径后复用原输出，确认没有新模型进程。事务故障注入使快照更新失败，退出补报与完成快照一起回滚，实际进程退出历史仍保留，重启不能误判为成功。Curator 首回合已完成但第二回合未完成时继续阻断；重复启动同一回合曾复现覆盖快照，改为唯一键拒绝后复测通过。

真实证据 role_completed_current/proof.json、role-completed-live.py、completed-proof-check.py；升级证据 role_assignment_current/completed-node-upgrade-proof.json。测试计划额外核对最初把模型草案与完成绑定的计划直接比较，因正常添加用例 ID 和 pending 状态而失败；按既有 Bind 合同分别核对新增字段及原正文后通过，未重跑原生模型。

PostgreSQL 18／真实 Docker 最终全包 452 项测试及子测试通过，零失败、零跳过，报告 role-completed-final-regression.jsonl；Go 静态检查通过。Ubuntu 非 root 身份实际执行角色恢复、结果补报及本次完成快照专项，80 项测试及子测试通过，报告 completed-linux.log。旧协议继续由回归覆盖，但没有进程采集的旧控制面不提供本项完成快照保证。仍未覆盖原生模型运行中的 Daemon 恢复、输出尚未保存的中断、原 Session 恢复、完整归档与全路线最终验收。外部条件继续保持待验收，本项不单独扣减总工作包估算。

上述已完成回合恢复实现与验收对应 ForgeOS 提交 4ba64cf1c35b1fd4f7f3a89a849948f64b38982b。

### 专题 07、13、16 C.3 · 独立角色原生恢复及缺失会话交接

2026-11-18 在独立 PostgreSQL 18 库还原 beta #163，通过 Chrome 导入资料、生成候选测试计划并请求 Reviewer。使用正式角色 Daemon、本机 Codex 0.169.0-alpha.11.1 和 Claude Code 2.1.310。验收固定候选 d689d8e011d2c9e1e49e93cfa632d31b132c2228，不重新交付 GitHub。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 原生 Session 已记录时暂停本次原生进程，结束 Daemon，再启动同身份 Daemon | 原进程仍活着时不能重复模型 | 四类角色均明确阻断；核实并结束本次原生进程后才继续。 |
| 重启控制面及 Daemon | 原 Session、工作目录、提示及合同不变，保持原期限 | 四类角色均恢复同一 Session，实际退出及业务结果获 204；未取得原退出码的实例记为 -1。 |
| 原生恢复时 CLI 报 Session 不存在 | 保留失败实例，以冻结输入创建有来源的新会话 | Codex Curator、Claude Reviewer 均保留原实例、失败恢复实例（退出 1）和新实例（退出 0），handoff_from 指向原 Session。 |
| 再次重启，从 Chrome 展开历史 | 能解释每次失败、恢复及交接 | 四类会话可回读；两种交接显示失败标签、恢复实例与交接来源。任务仍为 WAITING_ACCEPTANCE。 |
| 升级已有 PostgreSQL 106 和四份真实节点 SQLite | 保留原业务及进程历史，不重跑模型 | 数据库到 108，原四条进程 ID 与业务 JSON 相同；节点原退出及结果相同，未重报已结算结果。 |

| 角色／场景 | 原生进程与 Session |
| --- | --- |
| Curator 原会话恢复 | PID 11548 → 2476；01a1ec47-467e-72b0-b20b-9ccfe6f74ecd。 |
| Management 原会话恢复 | PID 46972 → 46592；01a1ec4a-3e0c-7230-98f1-ee93d2fe0db0。 |
| Test Agent 修订 2 原会话恢复 | PID 45268 → 39144；01a1ec54-1233-7970-8032-4089c8b2150e。 |
| Reviewer 原会话恢复 | PID 47700 → 41912；f48ccf5b-6235-41cb-8796-cf74e8cde4f5。 |
| Codex 缺失会话交接 | PID 29164 → 29976（失败）→ 34720；01a1ec57-bea3-78f3-a461-7c9727a7a1d3 → 01a1ec57-d600-7bc1-881e-19e1b3d02897。 |
| Claude 缺失会话交接 | PID 25524 → 48028（失败）→ 44812；b2fee81a-e9ee-4489-a622-123eecbe52c1 → e403d691-7250-46ab-bf51-2ccbeec80fcb。 |

首次实测发现迁移文件未注册，退出报告得到 503；补齐注册后原作业在原期限内恢复。测试计划修订 1 的 Session 在 CLI 落盘前中断，恢复失败；该失败保留，补齐缺失会话交接后用修订 2 验收原会话恢复，另用两种真实 Harness 验收交接。Claude 交接仅临时隐藏本次验收自己的会话文件，结束后已恢复；Codex 的原 Session 本身未持久化，无需隐藏文件。

证据：role_native_current/proof.json、role_native_handoff_current/proof.json，操作脚本 role-native-live.py、role-native-handoff-live.py；升级证据 role_completed_current/native-migration-proof.json 和 native-node-upgrade-proof.json。私有输入与租约不复制到本仓库。

本次 Windows Worker 受影响回归 107 项、PostgreSQL 进程／领取与会话交接回归 16 项、Linux 非 root 原生恢复专项 17 项测试及子测试通过，均零失败、零跳过。报告分别为 role-native-worker-tests.log、role-native-pg-tests.log、role-native-handoff-pg-test.log 和 native-role-linux.log。Go 静态分析、前端 TypeScript／Vite 构建和 git diff 检查通过。上一阶段全包为 452 项通过，本次不冒充已重跑全量。全路线最终回归及端到端验收仍待完成。

尚未覆盖未记录 PID／Session 的启动间隙、接回仍存活 CLI 的事件流、完整归档及到期清理；外部设备私网、受限 GitHub 身份、第二位审查者和 Linux 服务账号模型登录继续待验收。固定输入和工作目录目前在私有节点目录中保留。

上述独立角色原生恢复与交接实现、验收对应 ForgeOS 提交 df323a9cea263409c5dbffecef10a5985e9f2585。

<a id="check-b-9"></a>

### 专题 07、13、14、16 C.3 · 节点进程日志归档、导出和保留

2026-11-18 从正式 Worker、真实原生 CLI、Chrome 工作台和 GitHub 交付验证节点公开消息与工具文本归档。记录保存在节点私有 SQLite，正式 process-logs CLI 负责列出、导出、校验和清理，不把页面窗口当作完整输出。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 原生工具输出 700 行编号文本及首尾标记，再读取含合成敏感字段的文档 | 超过 8 KiB 仍可回查开头、末尾，敏感行不落盘 | 已完成适配器证据在下表；正文包含 0000–0699 和两端标记，敏感行仅保留 redacted。 |
| 原生完成后重启控制面，从 Chrome 核对候选并交付，再用节点 CLI 导出 | 页面快照、候选和归档相互对应 | 导出含同一 PID／Session 与执行编号，末行 verified=true，未达到容量，摘要可复验。 |
| 四类独立角色完成后中断结果发送，重启 Daemon | 原公开消息归档保留，不重新运行模型 | Curator、Management、Test Agent、Reviewer 各一份真实原生归档可验证；过时计划结果仍为 409，其日志保留。 |
| 对四份真实角色日志的隔离副本推进保留时间 | 最近确认、pending 和在途作业受保护；到期仅清正文 | 只修改旧退出时间时清理数为 0；确认时间也超过 30 天后各清理 1 份，摘要和业务进程不变，重复清理为 0。 |
| 从清理前 SQLite 快照恢复并再次清理 | 旧备份不能永久绕过期限 | 四份均再次清理 1 份；已到期 export 返回非零，未输出正文。 |
| 在 Ubuntu UID 1000 下运行正式 Linux 二进制 | 无需模型登录即可列出、导出及清理自己的日志 | 私有目录中导出摘要一致，清理后保留摘要并拒绝导出正文。 |

四类独立角色原生 Session：Curator 01a1ec65-134e-7333-837f-6f1fecd7664f，Management 01a1ec65-5656-7470-8fca-4b953cac2e23，Test Agent 01a1ec65-938f-7502-9cd0-9e45d65c5c67，Reviewer 11c0c519-78d4-4a7d-ba50-c2f82622a164。证据 role_archive_current/proof.json、process-archive-roles-live.py、process_archive_retention/proof.json、archive-retention-live.py 和 process-archive-linux-cli-proof.json。

容量上限、跨块脱敏、事务故障同时回滚归档／窗口／脱敏状态、记录损坏拒绝验证、暂停保护、在途角色保护、过期幂等及旧进程无归档通过专项。删除失败时，到期标记和正文一起回滚；后台维护正常退出。中央全文下载、其他结构化结果及冻结角色输入／工作目录清理仍待补齐；全路线最终端到端验收和既有外部条件继续待验收。


| 原生适配器 | 归档证据 | 真实交付 |
| --- | --- | --- |
| Codex CLI | PID 23216；Session 01a1ec65-3214-7be3-9c12-e3abc880d428；12 条，14,774 字节。 | beta #212，候选 b68cbfe393bc3708bfa4c4f6c8f0988423e7ea85；[PR #213](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/213)，合并 4cdd6c604540e30a91fc46c9af9fc3488bb394d0。 |
| Codex App Server | PID 26988；Thread 01a1ec6e-afb8-7303-afdd-d122000b12b8；734 条，111,100 字节，含命令增量。 | beta #216，候选 8c010ae8395634925bdfa76702f215827fd0eda6；[PR #217](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/217)，合并 9919147f99241cb0017480c30151241e8f20282b。 |
| Claude Code Read | PID 43020；Session 4693c936-7ae7-4512-94f0-6952174e6dc7；14 条，18,889 字节。 | beta #218，候选 e32d32a99e27b908cb7f4d84add448740e5c2a44；[PR #219](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/219)，合并 9cdf1e06d7490439490209736cfc42594adbf07f。 |

三项均由 Chrome 按固定候选验收，真实必需检查先失败、按验收工作流重跑成功后才合并，forgeos/local-ci 同时为成功，Issue 关闭；重启后可回读原进程与输出。证据 process_archive_cli/proof.json、process_archive_appserver/proof.json、process_archive_long_claude/proof.json，以及各目录的 process-1.jsonl。导出摘要分别为 c8a67ef58fcd83a64449f84f12902e4e0ce02fd24b56ce3eaea4f7d4e22bd96b、a777bb850deba1e331a55477448f6386da3c65e7f91c5c46151ce4306618d08e、56b1849f0342fbcf3a8bb9575cd9f664ce45952efbd8975dcdefd8d9292ab306。

Claude 首个 beta #214 场景完成了允许的 Read/Write 及 PR #215 交付，但权限集合没有 Shell，未执行长输出命令，因此未记为长日志验收通过。保持原权限，改用 #218 的 706 行文档及完整 Read 取得长输出。前一个场景记录保留在 process_archive_claude/proof.json。

本阶段 PostgreSQL 18／真实 Docker 全包 463 项测试及子测试通过，零失败、零跳过，报告 process-archive-regression.jsonl。随后补入的后台维护及清理失败回滚以受影响专项复验；最终 Windows 受影响专项 30 项测试及子测试通过，零失败、零跳过（process-archive-tests.log）。Linux 非 root 专项 14 项通过，零失败、零跳过（process-archive-linux.log），另以正式 Linux CLI 实测真实日志导出和到期拒绝。Go 静态检查及 git diff 检查通过；本次没有修改前端代码，全路线最终回归仍待完成。

上述节点日志归档、导出与保留实现及验收对应 ForgeOS 提交 772840a7b2e3ae0a78ac86e34a3a721c8a71d29a。

### 专题 07、13、16 C.3 · Codex 文件改动结构化回执

本项使用既有工具输出窗口和节点归档保存原生 apply_patch 的结果：CLI 记录实际改动清单，App Server 另记录原生补丁。适配器测试以实际测试子进程核对完成与失败回执、调用编号、脱敏、归档，以及未知类型不伪造输出。正式权限配置保持不变。2026-11-18 已从正式 Worker、Chrome、节点导出 CLI 和真实 GitHub 完成以下验收。

最初的 CLI 验收脚本将返回的 Windows 绝对路径与斜线分隔的相对路径直接比较；窗口已经显示，但断言失败。脚本清理又在模型退出后、Worker 最终结果保存前结束了 Daemon，留下原进程退出 0 和未结算执行，不能记为恢复或交付成功。原 file_change_output_cli 库及日志保留，重新启动时因控制面地址变化也未接回旧作业。修复路径判据，并使脚本在清理前等待业务结果；隔离 v2 验收库复用仍开放的同一 QA Issue，重新核对实际模型、运行中 Chrome 和重启回读。没有改写原进程或伪造最终结果。该普通 Worker 中断窗口继续属于未完成的恢复范围。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 原生 apply_patch 添加文档，后续工具保持运行 20 秒 | 运行中即可查看实际回执 | 两种传输均从 Chrome 看到 file_change；CLI 是实际清单，App Server 含原生补丁。 |
| 补丁含合成 password 字段 | 敏感行不进入公开窗口和归档 | App Server 仅显示 [redacted]，CLI 未伪造不存在的补丁。 |
| 模型运行中重启控制面 | 原进程继续，回执可读 | 两种传输均回读原 PID／Session，未重跑模型。 |
| 节点 CLI 导出、Chrome 验收、通过必需检查并交付，再重启 | 归档可验证，交付及历史一致 | 两份导出 verified=true；真实 CI 首次失败后重跑成功，PR 合并、Issue 关闭，重启可读原回执。 |

| 原生传输 | 进程和归档 | 真实交付 |
| --- | --- | --- |
| cli | PID 42248；Session 01a1ec7f-8825-7cc3-8138-ce9933592b10；6 条，1116 字节；摘要 daf16ea1d55983abaef16a2291fe721224cb5f88f05d2a646218ce54944ab513。 | beta #220；候选 e10b5e9bff44985b4314b955c54cf14f16c8833c；[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/221)；合并 d8ddd6551685fad9a5083ca2b075d681a3310769。 |
| appserver | PID 9756；Session 01a1ec82-ef7e-7271-a7ff-e67235812c52；5 条，1022 字节；摘要 a6a4d2d4235ca1bed0c7bb8261424ef4af27e2723be1db9ddc0095a6244b0d5f。 | beta #222；候选 0b1a736e715f73328d58a17047daf46b6cc9f5d6；[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/223)；合并 f876ae7b9c0f3a439e18afd2c0a7349357d9b412。 |

证据 file_change_output_v2_cli/proof.json、file_change_output_v2_appserver/proof.json 及各目录 process.jsonl；操作脚本 file-change-output-v2-live.py。Windows 受影响专项 28 项、Ubuntu 非 root 专项 9 项测试及子测试通过，零失败、零跳过；报告 file-change-tests.log 和 file-change-linux.log。完成、失败、拒绝、禁用正文采集和转义凭据均有覆盖。Go 静态检查及 git diff 检查通过。本项没有改动前端或数据库合同，未重复上一阶段全包回归；全路线最终回归仍待完成。

上述 Codex 文件改动回执实现与验收对应 ForgeOS 提交 e07cb31c2e6122312881f5416abbee4ac4c18549。

### 专题 07、13、14、16 B.4 · 普通 Worker 候选结算恢复

2026-11-18 使用正式 Worker、真实原生模型、PostgreSQL 18、Chrome 和两个重启检查点验收。验收代理在原生模型退出后阻断 CI 候选登记，节点已经保存候选；核对没有存活原生子进程后仅终止本次 Daemon。随后在本次 SQLite 注入最终回执写入故障，验证已完成 CI 的快照能由另一 Daemon 复用。故障均只针对隔离验收环境。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 模型正常退出后阻断 CI 登记，另起相同身份的 Daemon | 已保存候选，不能并发消费 | Codex、Claude 各保留一个退出 0 的原生进程，第二个 Daemon 被 Execution 文件锁拒绝。 |
| 结束原 Daemon，重启控制面及 Worker | 在有效租约内接回同一候选，不再运行模型 | 原执行、Worktree、Session、PID 与 SHA 不变；对固定候选取得 CI 名额。 |
| 让实际 Docker 验证完成，但注入最终结果写入故障，再次重启 | 复用已保存验证结果，不重复 CI | 两种 Harness 各仅领取一次 CI；三个声明检查 format、unit、build 成功，原 Run ID 和结果被复用。 |
| 清除故障，从 Chrome 按 SHA 验收并完成 GitHub 交付，再重启查询 | 原结果完整，必需检查通过后合并并关闭 Issue | 两项均已通过；节点 candidate_finalizations 清空，原进程及日志仍保留。 |

| Harness | 原生进程与候选 | 交付证据 |
| --- | --- | --- |
| codex | PID 27028；Session 01a1ec8e-7ca6-7523-b23d-1a32dcd19a83；候选 8c33cb1aef02c51430f64cea292426fbc25e949a；原 CI ci-run-3032207461。 | beta #224；[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/225)；合并 ba8ff0bd8ac5b165274442b93b2bdc026098c6d9。 |
| claude | PID 7392；Session 03553cd8-1886-4ad1-afc5-09e420e68169；候选 759ee27df8190c583a803715bb1378d02299a1f3；原 CI ci-run-2021937400。 | beta #226；[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/227)；合并 12e5996c8d8ce0152f1efaa1b00a8855cf3904ee。 |

证据 candidate_recovery_codex/proof.json、candidate_recovery_claude/proof.json；操作脚本 candidate-recovery-live.py。首次 Codex 验收脚本把 Result 的 JSON 字段 commit 错写为 Commit 而停止；原候选检查点未丢失，修正断言后保持原 API origin 和节点日志继续，未重跑模型、未另建执行。测试代理在结束连接时的 Windows 10053／10054 日志属于故障注入，未据此修改业务结果。

整个 Worker 包回归 268 项测试及子测试通过、零失败；4 项依赖 Docker 开关的测试在该命令中跳过，并未记为通过。本次两项真实交付另已实际运行 Docker 验证。随后补入的验证失败保存、最终入队失败、Owner 取消及来源关闭通过最终 Windows 专项 13 项；Ubuntu 非 root 恢复／锁／既有问题与暂停专项 20 项、PostgreSQL 资源名额回归 2 项均通过，零失败、零跳过。报告 candidate-worker-tests.jsonl、candidate-final-tests.log、candidate-linux-tests.log、candidate-pg-tests.log；Go 静态检查和 git diff 检查通过。没有修改控制面数据库合同或前端。

本项未将旧 file_change_output_cli 失联执行反推为已恢复。普通 Daemon 在候选检查点尚未保存时的崩溃、运行中原生流接回、整机重启及全路线最终回归仍待完成；现有外部验收条件保持待验收。

上述普通 Worker 候选结算恢复对应 ForgeOS 提交 `2ea9a0f5ab1666d48c42ec1f7d4c29a9ca8621a0`。

### 专题 07、14 · 从任务页核对并结束过期执行

2026-11-18 用原 file_change_output_cli 库和工作区复验。旧执行 1 的租约自然过期，节点日志保留了 PID 22380 的正常退出，最终结果为空。另一次独立执行已完成 [PR #221](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/221) 并关闭 Issue #220，故本次处置无需重新派发。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 核对原 Worker、工作区、GitHub 状态 | 能判断是否已有交付或未保存的修改 | 原 Worker 已停，工作区 HEAD 为 9cdf1e06，仍暂存 docs/file-change-cli.md。另一次执行已合并为 d8ddd6551685fad9a5083ca2b075d681a3310769。 |
| 从 Chrome 打开任务并撤回核对勾选，用 Worker 身份请求结束 | 勾选前无法提交，Worker 无权结束 | 按钮禁用，任务仍为 RUNNING，Worker 请求返回 401。 |
| Owner 核对后勾选并结束，再次提交同一请求 | 只结束一次，保留证据 | 两次返回 204，任务为 FAILED，原执行仍无最终结果，只有一条 execution_abandoned 事件。 |
| 重启控制面，再次从 Chrome 回读 | 处置结果和原工作区持久保留 | 页面显示失败，原退出记录、HEAD、暂存文件和人工结束时间一致，仍只有一次执行。 |

首次页面实测发现浏览器请求白名单遗漏 abandon，点击未送出请求。补入执行编号受限的路径后，用同一旧执行完成以上验证。证据为 file_change_output_cli/abandon-proof.json 和 abandon-ui-live.py。TypeScript 编译、Vite 构建与 diff 检查通过。旧执行按人工结束失联记录，候选检查点落盘之前的自动恢复仍待补齐。

上述工作台失联执行处置对应 ForgeOS 提交 `c4b68dfd13ffefd1cde3f03c3887ba186bededb7`。

### 专题 07 · 候选恢复锁内复核

节点发现候选后，原 Daemon 可能在替代 Daemon 获取锁之前完成 CI 或把最终回执入队。以提交 2ea9a0f 的 journal.go 加载相同测试，原实现实际申请了两次 CI。修复在持有同一执行的锁后重新读取节点 SQLite。验证覆盖两种时序：先前的拥有者已保存验证结果时仅补报，已结算时直接跳过。固定候选的 CI 名额只申请一次，结果只发送一次。操作、回执使用真实 Git、SQLite 和本机 HTTP 测试控制面，专项入口为 `go test ./cmd/worker -run TestCandidateFinalizationRecovery/receipt-failed -count=1 -v`。此前的原生端到端证据见 PR #225 和 #227。

报告 candidate-lock-before.log 保留了旧版失败。修复后的 Windows 候选／锁专项和 Ubuntu 恢复专项均通过。随后在独立 PostgreSQL 18 和真实 Docker 下执行 `go test -count=1 -json ./...`，487 项测试及子测试通过，零失败、零跳过，报告 candidate-lock-stage-regression.jsonl。此为候选恢复阶段回归。

上述锁内复核及阶段回归对应 ForgeOS 提交 `1e3a1b5444405e62043ece8bff908a69316c1853`。

### 专题 02、03 · 项目外部通知通道与持久重试

2026-11-18 在 Ubuntu UID 1000 运行正式控制面二进制，通过 HTTP 登记两个 Git 项目，各创建、领取一项任务并回报失败。在实际控制面配置中仅为项目 1 启用 Telegram 与飞书，使用专用测试凭据和本机 TLS 接收端。验收代理仅接收这两个固定主机的 CONNECT，由本机证书完成 TLS 握手。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 通过控制面产生两个项目的失败事件 | 两个通道仅获得已配置项目的事项 | 两端都收到项目 1、任务 1、failure 和通知 3，未传输任务错误正文。飞书文本及签名校验通过。 |
| Telegram 首次返回 429 并指定等待 18 秒，飞书首次 HTTP 200 但业务码非零，然后重启控制面 | 保留失败尝试，到期后由新进程重试 | 两端各收到第二次请求，Telegram 等待至少 18 秒，飞书按 15 秒退避。两端分别返回消息编号和业务码 0。 |
| 成功后再重启，查站内通知和任务 | 避免再次发送，保留站内处理状态 | 两端请求仍各为 2 次，失败任务状态保持 FAILED，站内未读仍为 2。 |
| 两个发送者并发领取，模拟预留过期后让旧发送者确认，再将站内事项标记已读 | 只有一个发送者取得同一次尝试，旧确认无效，已读事项停止重试 | PostgreSQL 专项通过，旧确认得到冲突。 |

正式入口证据 external-notice-proof.json，操作脚本 external-notice-linux.py、external-notice-qa.py。提供方回执、无效配置及 PostgreSQL 通知／领取专项通过，go vet 通过。当前环境没有供本项目发送验收消息的 Telegram 测试聊天或飞书测试群。Owner 于 2026-11-18 确认真实投递保持待验收。远端账号收件与最终全路线验收均未计为通过。

正式 Windows 控制面将原失联处置验收库从 108 升级至 109。升级前后均有 3 条任务事件、1 条通知、1 次人工结束。任务页 API 仍返回 FAILED，未配置外部通道时投递记录为 0。证据 notice-upgrade-proof.json。

上述外部通知通道实现、本机 TLS 验收与数据库升级对应 ForgeOS 提交 `656375c8e668678ca08f105ee02d917f1b8a061e`。

<a id="adr-evidence-017"></a>

### 专题 12、ADR-017 · 失败与取消的 Issue 回写

2026-11-18 通过正式控制面在私有 beta 项目导入 Issue #228。验收客户端使用 Worker 的 HTTP 领取与回报接口提交受控失败，以验证失败处理路径。退出码为 9，原始错误含专用隐私标记。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 领取任务、报告执行失败 | Issue 保持开放，GitHub 反馈具体尝试及失败原因 | 执行 1 的评论包含退出码 9、任务和事件编号，原始错误未出现在评论。 |
| GitHub 已保存评论时，通过数据库故障注入阻止本地确认。重启并等待真实领取期限届满 | 重领后先核对远端评论，同一失败只有一条 | 在本地未确认时已读到远端评论，重新启动后确认原评论，数量仍为 1。 |
| Owner 重试，领取新执行，请求取消并让 Worker 回报停止 | 独立事件反馈 Owner 取消，Issue 保持开放 | 执行 2 留下第二条评论，任务 CANCELLED，远端 Issue 为 OPEN。 |
| 再次重启，通过 Chrome 回读任务时间线 | 两次尝试和回执分别展示 | 原失败事件和 Owner 取消事件各有确认时间，两条链接均指向来源 Issue。 |

GitHub 原始证据为[失败回写](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/228#issuecomment-6968072667)和[取消回写](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/228#issuecomment-6968106519)。操作脚本 issue-outcome-live.py、outcome-browser.py，证据 outcome_feedback_live/proof.json。PostgreSQL 专项验证重复 Worker 回报、项目隔离、重领、旧租约确认冲突与事件回读。来源关闭、Owner 取消、迁移、失败重试及既有远端交付门禁回归通过。前端生产构建、go vet、git diff 检查通过。

<a id="adr-evidence-006"></a>

### ADR-006 · 已有失败执行的简单任务升级为 Goal

2026-11-18，正式控制面与 PostgreSQL 18 从 beta 私库导入 [Issue #229](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/229)。首次领取与回报使用 Worker HTTP 入口，规划和激活使用 Chrome 工作台。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 领取简单任务 1 的执行 1，在执行中提交有效计划 | 保护活动租约，保留原任务 | HTTP 409，原执行仍可回报。 |
| Worker 回报退出码 3；待失败反馈抵达 Issue，在任务页起草带决策引用的 Goal | 在安全检查点允许升级，沿用原任务 ID | Chrome 发布修订 1 返回 200。归档得到 Git 提交 `c7ca95b43d585ba1c5b30dcadafef0a01db3fc6f`。 |
| 在 Chrome 批准并激活修订，重启控制面后读取父子两项 | 保留原执行及来源事件，生成可派发子项 | Goal 仍为任务 1，生效修订 1。工作项 2 的 goal_id 为 1。原执行 1 逐字段相同，导入、领取及失败三个原始事件均存在。 |

操作记录 adr-goal-live.py、adr-publish-live.py；回执 adr_decision_goal/proof.json。对应 ForgeOS 实现 `5d30bcf451d838fae10d684d5c6bd718f5a4f73a`、`94b6b0845c7bbb370701110c1ab2f841fffdc953`。同一实现版本启用 PostgreSQL 18 与 Docker 的全量 Go 回归得到 498 项通过、零跳过、零失败（issue-outcomes-stage-regression.jsonl）。

<a id="adr-evidence-004"></a>

### ADR-004 · 将 Owner 确认的工程决策写入项目仓库

2026-11-18，使用私有 beta 仓库的 [Issue #229](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/229) 和正式 Worker。操作脚本 adr-goal-live.py、adr-publish-live.py、adr-revert-live.py；证据 adr_decision_goal/proof.json。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| Owner 确认队列预览必须记录完整来源 SHA 的工程决策，把决策引用加入 Goal 子项 | Worker 获得来源、原文与适用条件 | 工作项 2 的执行 2 保存决策快照。原生 Codex 将原文、适用条件和 `/projects/1/decisions/2` 写入 `docs/adr/queue-preview-source.md`。 |
| 从 Chrome 核对候选，批准后等待远端检查 | 只改该文档，仓库门禁通过后才合并 | 固定候选 `7b5484c22dee55f942628c1426ae95aa48553aee` 的文档验证退出 0。Actions 预设失败项重跑后成功，两项必需检查通过，[PR #230](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/230) 合并为 `aa59e90be074a63476136edc119afc802fc4ed76`。 |
| 从登记克隆按合并提交查询工程文档，并回读项目决策 | 工程文件和运营索引可分别复核 | API、GitHub 和本地 Git 返回同一 blob `9fda8b0a63058b2b0dd8fd22d3a5e70efa06849a`。文档含确认原文和适用条件，中台决定仍为 CONFIRMED。 |

在同一项目中复用计划引用和已有交付流程。主要实现为 `ddbaae25eb110553bcc3d958bd268117ea57331d`。

<a id="adr-evidence-007"></a>

### ADR-007 · 回退项目 Git 内容后核对运行历史

2026-11-18，从同一控制面读取已交付的父 Goal 1、子项 2、计划修订 1 和已确认决策 1。子项关联原生执行、验证、Owner 验收、[PR #230](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/230) 和 Merge SHA。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在干净的登记克隆，以合并提交单独建立本地验证分支，执行 git revert | 工程文档从当前树撤下，仍可按原提交查回 | 从 `aa59e90be074a63476136edc119afc802fc4ed76` 回退得到 `952b92546b5009fa809cc0b2e380cb1e3b57ab4c`，差异仅删除该 ADR。当前 HEAD 的文档查询没有该文件；按原 SHA 的响应逐字段相同。远端目标分支保留已交付版本。 |
| 回退前后读取四份完整记录，再重启控制面，用 Chrome 查看子项 | 运行历史独立保留，仍可核对原候选和远端合并 | 三次回读相同，排序 JSON 的 SHA-256 均为 `446a7f9c47df5bb3af4b6fb7a9d7029373eecd2ddb68792aac37c415215c6a21`。任务页显示原候选 `7b5484c22dee55f942628c1426ae95aa48553aee` 和原 Merge SHA。 |

证据 adr_decision_goal/proof.json，操作脚本 adr-revert-live.py。阶段三父项关闭实现为 `544744520aeb6ea13724c7aaf4166ebf0a5c09b0`，运行历史与候选证据的提交见 ADR 对照表。

<a id="check-b-4"></a>

### 专题 07、13、16 B.4 · 从尚无候选 SHA 的已完成草稿恢复

2026-11-18，在正式控制面与独立 Worker 中让 Codex 完成私有 beta #233 的文档修改。最后一个评论／指令检查点处理结束后，通过本次测试专用的代理暂停续约，在候选尚未创建时结束 Daemon。生产实现通过节点 SQLite 保存租约、原生会话、当前 Git HEAD、索引树和完整工作树。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 终止节点，检查进程、工作区和节点记录 | 原生回合已结束，真实索引保持原样，尚无候选 SHA | 执行 1，进程 29508 已退出 0。HEAD `b3d0b5dca465411606df56d49aeaac9ed2e8fbad`，索引 `9bd90e60a844526cbb5af46b1501760b636bd00a`，草稿树 `26962decb9aef90dff92e50c169c8963a32bf7c8`，SQLite 中没有候选 SHA。 |
| 重启控制面，向专用 SQLite 注入一次候选 SHA 保存故障，再结束替代 Worker | 先保存 SHA 再更新索引和 HEAD；失败可重试 | 原生进程仍只有一个，HEAD、索引和草稿凭据保持原值。撤除故障后新 Worker 核对租约与草稿，从原树产生候选 `def592e21ed781b89f23556da2101c0e6ee5a365`，本地验证只领取一次。 |
| 从 Chrome 审阅并批准候选，等待目标分支门禁，再重启查询 | 仅交付原草稿；原执行和会话可追溯 | 两项 GitHub 必需检查成功，[beta PR #234](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/234) 合并为 `2d182947f93b2799232110afbfe5403b8138ae63`，来源 Issue 关闭。重启后页面仍显示同一候选。 |
| Windows、Ubuntu UID 1000 分别重开 SQLite，模拟已暂存、索引有缓存标记、索引被单独修改及工作树变动 | 原内容可恢复，外部改动阻止结算，兼容原先已有 SHA 的记录 | 专项通过。新建候选的 Git 树与保存的真实草稿相同。索引或文件被更改时零次验证、零次结果回报，保留记录供核对。 |

开发期间另在两个私库使用同一套任务及 Chrome 验收入口，原生 Codex 完成 [beta PR #232](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/232)，原生 Claude Code 完成 [alpha PR #37](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/37)。最终实现的 SHA 保存故障通过 beta #233 实测。启用 PostgreSQL 18 与真实 Docker 的全量 Go 回归 503 项通过，零失败、零跳过。go vet、TypeScript 与前端生产构建通过。操作脚本 draft-final-live.py，证据 draft_final_recovery_codex/proof.json。原生回合退出至最终输入检查点和草稿首次持久化之间中断，仍待验收。

<a id="completed-writer-input"></a>

### 专题 07、13、16 B.4 · 草稿恢复后继续处理排队指令

2026-11-18，Windows 原生 Codex、正式控制面与 Worker、节点 SQLite、Chrome 及私有 beta 仓库。使用测试代理暂停 Worker 发往当前检查点的确认请求，在控制面仍持有执行租约时终止本次验收的 Daemon。

| 操作 | 预期结果 | 实际结果 |
| --- | --- | --- |
| 导入 beta #237，在模型名额释放前经 Chrome 将一个新标题排入下一回合。首个模型退出后，中断 Worker。 | 原草稿和待处理指令均可找回，尚无候选。 | 进程 13936 退出 0。SQLite 有检查点 0、原 HEAD、索引及草稿树 `ac3af4739a80421f14ca21ca13c821c2bdee75f6`，没有候选。控制面接受了排队指令。 |
| 重启控制面和 Worker，第二回合退出后，在确认检查点 1 前再次中断。 | 新进程沿用同一 Session，在原文后只追加一次指定标题。 | 进程 5920 退出 0，两个进程的 Session 均为 `01a1ed52-3998-7992-a643-d0428125f7d1`。原文完整，标题 `## Receipt 237` 出现一次。第二份草稿树为 `e74d8e68882f6bc03d07a31090a1afe320eb7367`。 |
| 再次重启、等待本地验证，从 Chrome 批准，核对 GitHub 门禁和来源 Issue。 | 从第二份草稿生成候选，保留两条进程记录，只领取一次 CI。 | 候选 `e82061193ae3e3b7ddd476e9863fb92b9cfbb42c` 的树与第二份草稿相同，仅修改约定文档。两项门禁通过，合并提交 `34c03d512b6018110659bae8742b91286884e5f0`。[来源 #237](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/237) 已关闭，[PR #238](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/238) 已合并。 |
| Windows 和 Ubuntu 非 root 用户执行专项测试 | 正常评论与指令均能继续，空队列可结算；索引、文件、HEAD 或版本变化时保留现场；取消和来源关闭可结算。 | `TestCompletedWriterCheckpoint` 的 10 个场景在两个系统上全部通过。未确认下一检查点时保留本地记录。 |

最早的中断点在完成回合记录已写入 SQLite 之后。原生进程退出到首份记录写入，以及上一轮记录撤销到下一轮保存之间的中断仍待验收。操作脚本为 `completed-turn-live.py`，节点证据为 `completed_turn_queue_codex/proof.json`。

#### Owner 在检查点确认期间取消

在[私库 beta #235](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/235) 的独立执行中，让 Codex 完成草稿，测试代理暂缓当前检查点确认。从 Chrome 点击“取消任务”后恢复请求。预期 Worker 确认取消并提交停止回执，保持原草稿，CI 和交付均不启动。

实际执行 1 的原生进程 40880 退出 0，Session 为 `01a1ed65-d974-7320-8658-2d8af04aab05`。控制面接受取消后，检查点返回 409，随后的租约查询返回 423。Worker 回报停止原因，节点清除恢复记录；任务为 `CANCELLED`，没有候选、CI 领取或 PR。重启控制面后状态一致，专用取消用例的来源 Issue 仍打开。

修复前，Owner 取消与来源关闭两个竞态测试都把执行留在待恢复状态。修复后，Windows 与非 root Ubuntu 的 12 个专项场景全部通过。PostgreSQL 18 和真实 Docker 的全量 Go 回归共 526 项测试及子测试通过，零失败、零跳过。来源关闭的本次竞态由专项测试覆盖。节点证据为 `completed_turn_cancel_codex/proof.json`。

<a id="role-recovery-retention"></a>

### 专题 07、13、14、16 B.9 · 独立角色的恢复资料到期清理

2026-11-18，从此前实际执行和恢复的四类角色取四份节点 SQLite 一致性备份与五个工作目录。在独立副本中将已结算记录回拨至 2001 年以跨越一天的验收期限，保留一条近期业务结果及一份还在登记的 Reviewer 分配。原始数据库和原目录单独核对。测试使用生产 Worker 命令、Windows 和非 root Ubuntu；另在 PostgreSQL 18 上启动正式控制面和 Reviewer。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 Windows 和 Ubuntu 对四份副本分别运行 prune-role-recovery，再次执行同一命令 | 清除到期输入、合同、输出和空目录。近期、在途内容和业务证据可继续回读 | 两个系统各清理 Curator、Management 和 Test Agent 各一回合，保留近期 Test Agent 和已登记 Reviewer。重复执行均为 0。原进程、原生交接、业务及进程回执逐字段相同。 |
| 在真实 Reviewer 启动时使用尚有原分配的日志；下一次启动前在过期工作目录加入未登记文件 | 分配释放前保留现场；异常目录可查且不影响领取 | 首次扫描保留目录，随后原分配正常释放。第二次启动保留额外文件且没有清理回执，两次均从正式控制面收到 204。 |
| 移走测试额外文件并重启；将清理前 SQLite 快照和工作目录实际还原到同一绝对路径，再启动 Reviewer | 自动清理，已恢复的过期内容按相同策略再次到期 | 两次都清除原目录及冻结输入，保存一条回合清理回执，原两条进程记录完整。四轮真实领取均返回 204。 |
| 验证暂存的角色回合、待发或近期进程回执、仍运行的进程，以及跨目录路径或链接 | 保留尚需恢复的内容，避免操作其他路径 | 节点专项通过。额外文件会在删除任何已知文件之前被发现，移除障碍后重试成功。 |

启用 PostgreSQL 18 与 Docker 的全量 Go 回归共 505 项通过，零失败、零跳过。证据保存在 role_recovery_retention 的 win32.json、linux.json、daemon.json。操作脚本为 role-retention-live.py 和 role-retention-daemon.py。[运行命令与恢复步骤](14-deployment-and-operations.md#role-recovery-retention)

<a id="approval-source-levels"></a>

### 专题 05、16 C.5 · 审批中心的来源审核和有效等级

2026-11-18，在独立 PostgreSQL 18 验收库登记两个项目。使用原生 Codex Curator 提取退款期限变更提议，Management 在另一会话给出 D2、请 Owner 决定。同时审核一份只包含恢复检查方法的跨项目共享副本。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从 Chrome 登录审批中心，筛选项目决策，对照两类提案和固定资料的审核回执 | 来源知识的审核可追溯，手工提案只显示已有依据 | 来源提案显示“来源知识审核 · D2 · 请 Owner 决定”。另一项目的手工提案无审核标签。Owner 在原提案页确认。 |
| 在审批中心打开待发布副本，再由 Owner 发布 | 列表显示生效 D3；发布针对同一副本 | Chrome 列表可见 D3，发布后接收项目回读同一 SHA-256。 |
| 用普通节点身份读取审批列表；控制面重启后再以 Owner 身份打开队列 | 节点无法读 Owner 队列；已处理项退出 | 节点收到 401。列表中没有资料的内部标记或原生会话字段。重启后仅手工提案仍待确认，已确认决策和已发布副本均可回读。 |

脚本 approval-policy-live.py，证据 approval_policy_a/result.json 与 decision-list.png。来源和共享由独立角色在生产入口审核，审批页使用生产构建。

<a id="curator-result-route"></a>

### 专题 05、16 B.3 · Curator 向 Management 交付 RESULT

2026-11-18，在三套独立 PostgreSQL 18 项目中导入同一份重启观察，分别运行新控制面与新 Worker、新控制面与旧 Worker、旧控制面与新 Worker。每组都使用真实 Curator 和 Management 原生 Codex 会话，另从 Chrome 查看审核回执。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 使用当前版本提取资料并领取审核，对照控制面领取回执、节点冻结提示词和来源正文 | RESULT 携带原事实与逐字引用；发送与接收角色、项目和来源固定 | 节点提示词里的整条消息与领取回执相同。消息为 CURATION-1-RESULT，原生提取会话、固定正文 SHA-256 与全部事实匹配；时间为原提取回执时间。Management 在不同会话审核通过。 |
| 分别换用旧 Worker 或旧控制面，重复两角色作业 | 两种次序均能交付原始提取并完成审核 | 两组的原生提示词都与旧格式提取一致，各有两份退出码 0 的独立进程记录；Chrome 可回读审核会话。 |
| 各组重启控制面，以原节点、原键和原请求再次领取 | 原请求返回原响应，不新建审核 | 三组均逐字节重放相同响应。来源与审核回执保持一致。 |
| 在测试中改变项目、知识记录、参与角色、信任级别、关联范围、消息类型、来源绑定或引用 | Management 在启动原生审核前拒绝无效消息 | 接收端专项通过。携带新旧两套提取内容的歧义输入也被拒绝。 |

脚本 curation-acp-live.py，三组证据为 curator_acp_current、curator_acp_oldworker、curator_acp_oldcontrol 中的 result.json。

<a id="curation-review-event"></a>

### 专题 05、16 B.3 · Owner 从工作台接收 Management 知识审核事件

2026-11-18，在独立 PostgreSQL 18 控制面登记两个项目，导入重启检查记录，启动正式 Curator 和 Management 完成原生审核。在生产构建的 Chrome 工作台对照审核前后、控制面重启以及旧版控制面。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 审核前读取资料回执，审核完成后展开“审核事件回执” | 仅完成审核后出现 EVENT，并绑定原审核 | 浏览器显示 CURATION-1-REVIEWED、CURATION-1 和 management → owner。事件时间等于审核落库时间，来源 SHA-256、等级、结论及原生会话与原回执一致。 |
| 普通节点读取原项目回执，Owner 以另一项目编号读取同名资料 | 保持身份与项目隔离 | 普通节点收到 401；另一项目返回 404。 |
| 切到旧控制面查看同一资料，再重启当前控制面 | 旧接口仍可阅读审核；恢复当前版本后收到同一事件 | Chrome 在旧版显示原审核，当前版本的完整事件逐字段匹配重启前记录。 |

专项用正式 AgentMessage Schema 校验实际 PostgreSQL 生成的事件。脚本 curation-event-live.py，证据 curation_owner_event_a/result.json、owner-event.png。

<a id="protocol-route-matrix"></a>

### 专题 16 B.3 · 五类消息的正式路由复验

2026-11-18，使用当前控制面和四组真实作业记录，从原检查点的 Owner 消息查询、原 Management 节点的幂等领取请求、以及 Owner 的资料审核回执读取 10 条消息。按仓库 AgentMessage 1.0 的 Draft 2020-12 Schema 验证完整对象和日期格式，逐条核对发送方、接收方、业务范围、信任等级和幂等键。每组均重启控制面后再读一次。

| 类型与场景 | 预期 | 实际消息编号 | 完整消息 SHA-256 |
| --- | --- | --- | --- |
| REQUEST · Owner 答复 | 绑定提问检查点 | `QUESTION-1-REQUEST` | `a0ec6333427e5a8a210e866a03748954e3942d98cc4fd07e679c3b9164197f43` |
| RESPONSE · Owner 答复 | 关联原请求，返回提问节点 | `QUESTION-1-RESPONSE` | `2881bf4ad7761e6c84a7e8d275b7b5c128d8cdc3cc000c3585a5250ada8cd647` |
| COMMAND · Owner 答复 | 由 Owner 排入原执行的下一检查点 | `EXEC-1-CHECKPOINT-1-COMMAND-676ec8c3-b285-47b3-847c-994d26179047` | `198734bdad1f360278db01810d87d3602cfb9caa868870ebe8c9c7069ba93fc8` |
| COMMAND · Owner 答复 | 由 Owner 排入原执行的下一检查点 | `EXEC-1-CHECKPOINT-2-COMMAND-4d086944-0817-4cdb-b1c9-8332a7123c2a` | `04ed590e9bdd144e16b92447e09fc626861da24a2b3567f43ec06ffca0248e39` |
| REQUEST · Management 查阅 | 绑定提问检查点 | `QUESTION-1-REQUEST` | `321ad7eb4a2fc07d6831c5c5746a9edce4f6fadde094c8cb9ba933e6d42aa2e1` |
| RESPONSE · Management 查阅 | 关联原请求，返回提问节点 | `QUESTION-1-RESPONSE` | `d659a5f19a26e7b73ca7dd7792038f93872e3000c23044298555df498ba90125` |
| COMMAND · Management 查阅 | 由 Owner 排入原执行的下一检查点 | `EXEC-1-CHECKPOINT-1-COMMAND-b3af4f7a-59fe-4e59-96b9-87815271ef54` | `6d815169c13961c842e19fe1286fc9ddfb1bd0bada1c47e6e941d0c1ac91f91e` |
| COMMAND · Management 查阅 | 由 Owner 排入原执行的下一检查点 | `EXEC-1-CHECKPOINT-2-COMMAND-8dc7d7a2-9e1d-4105-8fa9-b0ccb168d0ca` | `e83366a2477a5f51b0f2d39dc4bfa797ad1d602716daf9c224fa0b6ddc7e7b5a` |
| RESULT · 角色审核 | 同项目 Management 接收原 Curator 事实 | `CURATION-1-RESULT` | `f3745b9c1c27442caff46e26e0a77bd3249e892289f11ce25211449d7a85231d` |
| EVENT · 知识工作台 | Owner 读取已完成的知识审核 | `CURATION-1-REVIEWED` | `970008374116e8c090f0e730bc9738e1d09988b17f95539bf54823e8ed1a8288` |

两种 REQUEST/RESPONSE 均核对了原请求 ID。四个 COMMAND 各绑定自己的接收检查点。RESULT 与 EVENT 共享知识记录的关联编号，分别保留提取与审核的业务时间。所有 10 条均通过 Schema 并在重启后逐字段一致。普通 Worker 令牌访问对应的 Owner 或 Management 接口均收到 401。摘要以 UTF-8 对键排序的 JSON 计算（Python json.dumps，sort_keys=True，ensure_ascii=False，默认分隔符）。

原始端到端证据包括 [Owner 指令](#check-b-3)、[角色结果及新旧版本的真实组合](#curator-result-route)、[Owner 在 Chrome 查看审核事件](#curation-review-event)。实现与验收提交见[专题 16 的原始事项对照](16-open-decisions.md#requirement-status)。此次复验脚本 protocol-type-matrix.py，每组 proof.json 保留消息身份、幂等关联及完整消息摘要。

<a id="approval-questions"></a>

### 专题 05、16 C.5 · 从审批中心处理原任务的 D2 和 D3 业务问题

2026-11-18，将真实候选的审核前快照还原到独立 PostgreSQL 18 数据库。从正式接口先后对同一候选提出标题用词、不可逆删除的审批规则两项问题。第二项只要求判断政策；Owner 在页面选择拒绝。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在原候选提出问题，启动独立 Management，分别打开审批中心的全部操作与“业务决定” | 完成风险分级后，两种筛选都可转到该问题；传统请求保留旧类别 | 原生会话分别给出 D2 和 D3。分级前问题未出现在本队列，D3 的建议答复为空。普通节点请求返回 401；原接口不带新参数时返回原有类别。 |
| 在 Chrome 分别打开问题 #1 和 #2，查看审核依据后提交答复 | 每个详情入口只处理指定问题；答复后从队列移除 | 两次均只出现一个答复框。D2 记录 Owner 的标题用词，D3 记录 Owner 拒绝变更，原候选恢复待验收。以边界游标读取没有重复项。 |
| 重启控制面，再次进入两条原链接，同时尝试不存在的编号和超出安全整数范围的编号 | 历史答复可独立回读；无效链接不能提交答复 | 两条链接各显示原问题的 Owner 答复，审批队列为空。未知编号提示本任务无此问题，超大编号进入不存在的页面。 |

脚本 question-approval-live.py，证据 question_approval_a/proof.json 及 D2.png、D3.png。PostgreSQL 专项验证 D3 项保留至 Owner 答复后移除。

<a id="native-steer-probe"></a>

### 专题 07、16 A.1/B.2 · 真实 Codex App Server 运行中输入

2026-11-18，Windows，codex-cli 0.169.0-alpha.11.1。执行 python scripts/probe_codex_app_server.py --bin codex.exe --steer。初始 Thread 为只读、从不申请授权，提示要求用文字回答。这是原生协议验收；后续[工作台实测](#live-workbench-turn-input)记录正式 Worker、故障重试和候选验证。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 turn/started 后指定另一回合编号发送，再用正确编号提交一条随机标识 | 错误回合被拒绝，正确回合返回自己的编号 | 错误为 -32600。回合 01a1ed01-e3b1-7db3-875b-e9c77dae792a 的最终回复为 FORGE-cfdb8c1c32facc3c，标识仅在追加输入中出现。 |
| 退出首个进程，以第二个 App Server 读取同一 Thread 的原生历史 | 恢复同一回合、同一客户端消息及回复 | 输入项 01a1ed01-f8a6-7030-a5a2-23ead09bdfe8，clientId 为 55b1dc08-e61e-462a-8d63-36506121aa82。在原回合只出现一次。 |
| 在另一个独立只读回合，对同一正确回合连续使用相同的客户端编号投递同一文本两次 | 查明客户端编号能否防止重复执行 | 两次调用均成功。重开历史得到两条同 clientId 的用户输入，证明该版本需要应用侧处理响应丢失和重复发送。 |
| 重新执行默认生命周期探针和 --approvals | 启动、恢复、中断、批准、拒绝及取消可重复运行 | 两种模式均成功；批准的临时命令输出与随机数匹配，两次拒绝和一次取消均有原生回执。 |

证据为 native-steer-final-0.json、native-steer-final-1.json、native-steer-final-2.json。重复投递的独立实验脚本为 native-steer-repeat.py。

另用真实只读回合测试丢失发送回执后的核对。仅发送一次追加输入，客户端忽略对应 RPC 响应，转用 thread/read 按原 Thread、Turn、clientId 和完整输入正文查找。预期是从原生历史确认唯一输入。实际在同一运行中回合的前两次读取分别找到 0 条和 1 条；回合结束后及新开 App Server 均读到同一条。首次查不到不能作为重发依据。Turn 为 01a1ed13-b341-7bb0-bb2e-1a196e59a339，输入项为 01a1ed13-c8c5-7552-a9d8-e40f1c36f5af，记录为 steer-reconciliation-live.json。

<a id="native-interrupt-fallback"></a>

### 专题 07、16 A.1/B.2 · App Server 中断的退出码与强制终止顺序

2026-11-18，审批中心变更的全量回归暴露一次已有中断测试失败。模拟服务器拒绝中断，随后收到管道 EOF 并先于强制终止以 0 退出。节点将管道关闭移至强制终止之后。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| Windows、非 root Ubuntu 各运行 40 次拒绝中断专项，核对真实子进程的 SQLite 退出码 | 拒绝中断的子进程每次通过强制终止退出 | 两个系统共 80 次通过，未再记录退出码 0。 |
| 用正式适配器启动本机已登录的 Codex，分别在 Thread 建立前和工具运行中取消 | 建立前强制终止；正在运行的回合先使用原生中断 | 真实进程分别记录退出码 1 和 0。后者保留原生会话 01a1ed05-4546-7583-8589-f2415bd921be。 |
| 使用正式控制面和 Worker，从 Chrome 取消正在运行的实现回合；再派发本机连通性任务 | 工作项取消、原回合正常中断、资源可复用；来源 Issue 保持打开 | [beta Issue #235](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/235) 的 Codex PID 35464 和工具子进程均退出。原会话退出码为 0，无候选、PR 或合并。随后本机任务完成，节点模型占用归零。 |

操作记录为 interrupt-fallback-worker.py、interrupt_fallback_real/proof.json、interrupt-fallback-native.log 及 interrupt-rejection-repeat.jsonl。

<a id="worker-app-turn"></a>

### 专题 07、16 A.1/B.4 · 运行中和结束后核对同一原生 Turn

2026-11-18，复用 [beta Issue #235](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/235)，分别在隔离库启动正式控制面、Worker 和真实 Codex App Server。在 Chrome 取消执行，核对原生历史、节点 SQLite 和控制面回执。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在工具仍运行时展开任务执行记录，通过 Chrome 取消；随后重启控制面 | 运行中可见真实回合，结算和重启保留同一编号 | 最终复验的进程 44860、Thread 01a1ed0c-bb60-70e2-8ed3-c4d3d070691d 和 Turn 01a1ed0c-c2c0-7901-ac5c-1740fe2eefd2 在各入口一致；真实 Thread 历史中正好有一个匹配 Turn，状态 interrupted。 |
| 新控制面配旧 Worker、旧控制面配新 Worker，分别处理同一来源 Issue | 已确认的原生会话继续显示；缺少 Turn 采集协议时不填编号 | 两种组合均可取消并释放节点，API 与页面均未补造 Turn ID。后续连通性任务完成。 |
| 对同一个已观测进程尝试省略或替换已登记 Turn，另测错误格式与携带执行凭据的编号 | 拒绝不一致报告，保留原始回合 | PostgreSQL 专项通过；Worker 的实际协议子进程在运行中和结算时读到相同的 Turn。 |

程序 app-turn-live.py、app-turn-final-live.py，证据 app_turn_final_current、app_turn_id_oldworker、app_turn_id_oldcontrol 下的 proof.json。工作台向运行中原生回合发送 Owner 输入另行验收。

此版使用空 PostgreSQL 18 数据库和真实 Docker 运行全量 Go 测试。worker-turn-final-regression.jsonl 中有 9 个测试包通过、3 个包没有测试；506 项具名测试和子测试全部通过。go vet、TypeScript 检查与工作台生产构建通过。全路线最终端到端验收仍待功能补齐后执行。

<a id="archive-verify-before-export"></a>

### 专题 07、16 C.3 · 进程归档校验失败时的导出

2026-11-18，使用 beta #235 的正式 Worker 私有 SQLite 快照，执行 worker process-logs export。仅在独立备份上替换最后一条工具记录的正文，保留原摘要。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 导出损坏的进程 1 归档，分别读取退出码和标准输出 | 命令返回失败，任何正文均不进入管道或重定向文件 | 修复前已输出 97,744 字节，其中有替换的文本。修复后返回失败，输出 0 字节。 |
| 导出未改动的真实日志，按字节与旧版输出比较 | 顺序、正文及验证末行完全一致 | 两版均成功，531 条记录，完整输出均为 97,855 字节，逐字节相同。 |

专项测试还覆盖首条被更改、归档截断、跨块脱敏以及到期清理。操作脚本为 archive-before-export.py，运行记录为 comparison.json。工作台中央全文调阅仍待完成。

<a id="decision-search"></a>

### 专题 08、16 C.7 · 在工作台及项目节点检索决策

2026-11-18，先用旧版正式控制面在两个项目登记四条决策，再升级同一数据库。使用 Chrome、PostgreSQL 17、pgvector 0.8.6 及本机多语言 MiniLM 编码服务。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 不配置嵌入服务，比较升级前后列表，并按 backup 和中文问题检索 | 浏览保留原记录，关键词可独立使用 | 两次列表逐字段相同。backup 找到项目 1 的决策 #1，中文问题没有字面命中。 |
| 启用向量模型，在 Chrome 查询“断电后该怎样恢复”，打开首项的证据与历史 | 英文恢复决定排在第一，原记录和引用可以核对 | 首项是 #1“Restore the backup after a power outage.”，引用 manual/power。项目节点得到同序结果。另一个同项目结果排在其后。 |
| 先用受限节点查询另一项目，再以正确身份检索并重启 | 越权请求在编码前拒绝；默认范围只编码本项目有效决定，重启复用缓存 | 跨项目 403，无凭据授权的请求 401。项目 1 的 #1、#2 各有一个 384 维向量，重启后结果及两行缓存的事务号一致。 |
| 将 #1 标记失效，分别查询默认、已失效和待确认状态 | 默认结果撤下 #1，历史可按状态检索，私有项目数据始终隔离 | 已失效查询找到 #1，待确认查询找到 #3，项目 2 的 #4 始终未写入缓存。另有 PostgreSQL 测试在编码期间使决定失效，验证不会留下越过状态筛选的缓存。 |

操作脚本 decision-search-live.py，证据 forgeos_decisions_604a056e/proof.json 与 rank.png。此项覆盖项目决策检索；其他角色上下文仍按专题 16 保留为部分验收。

本项实现为 `e6fb688acf9d5626455b8c5bc1d182319fa7c97e`。包含新增并发状态测试的全量回归使用空 PostgreSQL 18 和真实 Docker，507 项具名测试和子测试通过，零失败、零跳过，另有 3 个无测试包。go vet、TypeScript 与前端生产构建通过。

<a id="claude-stream-interrupt"></a>

### 专题 07、16 B.2 · 在 Claude Code 正在输出文本时中断

2026-11-18，Windows 原生 Claude Code 2.1.310，运行 python scripts/probe_claude.py --bin <原生程序> --interrupt。探针使用独立临时目录、受限模式和空工具列表。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 收到当前会话首段文本后发送 interrupt，在进程仍受探针控制时等待回执和结果 | 请求编号匹配，原回合报告中断并退出 | PID 47332、Session 08063236-867c-427c-8a58-bfe064b4c683；请求 e568b96e-46d9-44b5-81ab-2d98f0ff6563 得到 success，终态 aborted_streaming，退出码 1。另一独立回合观察到同样的顺序和退出码。 |
| 再跑原有问答、拒绝写入及恢复模式 | 回答 Green，拒绝写入临时文件，第二进程恢复同一 Session 并读回标识和颜色 | 首次复验有一轮请求其他写入目标，探针拒绝。提示改为明确临时文件绝对路径后，进程 4932、42120 完成同一会话，未写入文件。 |

证据 claude-interrupt-final.json、claude-native-interrupt.json、claude-original-regression-final.json。后续正式节点中断见[工作台实测](#claude-worker-interrupt)。

<a id="claude-worker-interrupt"></a>

### 专题 07、16 B.2 · 从工作台中断真实 Claude 实现任务

2026-11-18，在隔离数据库启动正式控制面、普通 Worker 和本机 Claude Code 2.1.310。使用 Chrome 的任务页，分别验证取消和暂停。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在真实 [beta Issue #235](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/235) 的活动 Session 上点击取消，随后派发连通性任务并重启控制面 | 收到该进程的原生回执、结束工作项、归还名额，重启后可核对 | PID 38140、Session 91b09608-23e0-46a6-a192-f9e2a3e1ffdc。节点收到成功中断响应和终态，进程退出 1。任务取消，无候选及 PR，下一任务完成。 |
| 新控制面配旧 Worker 再从 Chrome 取消；旧控制面配新 Worker 复核能力协商 | 两端继续派发，旧协议维持原操作列表 | 旧 Worker 的 PID 41052 退出、取消结算、下一任务完成且重启可回读。旧控制面接受新节点心跳与执行，未展示新增能力；该次模型已返回余额不足，未取得活动回合中断回执。 |
| [beta Issue #236](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/236) 从 Chrome 暂停真实 Claude，重启控制面后恢复 | 确认原生中断，在原 Session 继续并交付 | 暂停和原生中断通过。PID 25628 退出 1，已保存树及 Session bf39454b-b476-4db7-8d50-5219919633c2。恢复 PID 2264 使用相同 Session，但模型服务返回 HTTP 403 并明确提示账号余额不足。执行失败，未产生候选。恢复后生成、验证和交付保持待验收。 |
| Windows 与 Ubuntu 非 root 各重复两次协议子进程测试 | 正常回合结束；中断回执在终态前后到达均可退出；错误回执及静默子进程强制退出 | 两个平台均通过全部五种场景。正式角色原有的工具集与工作目录权限继续由原配置提供。 |

现场程序 claude-interrupt-worker-live.py、claude-pause-delivery.py，证据 claude_native_stream_current/proof.json、claude_native_stream_oldworker/proof.json、claude_stream_pause_delivery/proof.json 和对应的私有节点日志。

在实现提交 `2d991f2c8b8e961ae07b1a66a302eab8c19efedd` 上从空 PostgreSQL 18 和实际 Docker 重新运行全量 Go 回归，513 项具名测试和子测试全部通过，9 个测试包通过，另有 3 个包无测试。报告为 claude-stream-committed-regression.jsonl。用 GitHub CLI 回读，#235 与 #236 均为 OPEN。四阶段最终端到端复验仍待其余功能完成。
