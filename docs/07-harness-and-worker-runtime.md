# 07 — Harness、Worker Daemon 与会话运行时

## 1. 基本原则

- 每台本地执行服务器部署一个 Worker Daemon。
- Daemon 使用用户已登录 Harness 的非 root Linux 用户运行。
- Codex、Claude Code 在该用户下分别登录一次，任务复用现有登录状态。
- 不为每个项目/Execution 创建独立 Harness 账号。
- 每个写入型 Work Item 使用独立 Git Worktree。
- Session、Process、Execution 分开管理。
- 容器不是默认必选项；按任务风险和隔离需求启用。

## 2. Worker Daemon 模块

- Worker Registry；
- Job Receiver；
- Workspace Manager；
- Harness Manager；
- Process Supervisor；
- Event Collector；
- Resource Manager；
- Recovery Manager。

### 在工作台查看 Worker

执行节点启动时先向 `POST /internal/workers/heartbeat` 登记。节点在领取、执行和回报任务期间每 15 秒报告身份、操作系统、架构、逻辑 CPU 数、执行名额及启动时探测的能力。使用节点授权文件时，令牌只能为自己的节点 ID 登记。首次登记与最近心跳都按控制面数据库时间记录。超过 45 秒未收到心跳，工作台显示“心跳超时”；未结束的执行仍占用名额。

调度器对已登记节点取最近心跳与本次领取所声明能力的交集。节点重启或能力改变时先上报新心跳；一次领取可以主动缩小可承接任务的范围。仅对尚未登记的旧版 Worker 沿用领取请求中的能力声明。

Owner 在工作台点击“执行节点”，也可调用 `GET /workers`。列表显示节点最近一次报告和从执行记录计算的已占名额，服务重启后可复查。节点恢复时更新原记录。节点令牌仅用于内部操作，私网网关只代理列表的 Owner 入口。

执行记录中的工作区路径属于领取任务的节点。控制面接受 POSIX 根目录、Windows 盘符绝对路径和 UNC 共享路径，并按节点路径格式生成 execution 目录。Worker 执行前仍会在本机核对工作区及其授权的项目根目录。

普通执行和独立代码审查的领取响应保留登记时的仓库绝对路径，并附带 `repo_relative_path`，如 `team/beta`。当登记路径已在节点允许的根目录内且存在时，节点继续使用原路径。挂载位置不同时，节点在自己的 `FORGEOS_REPO_ROOT` 下按同一相对路径定位项目。解析后仍核对工作树顶层和 Git 管理目录的授权边界。

## 3. Harness Adapter

建议统一基础能力：
- Probe；
- Start；
- Resume；
- SendInput；
- Interrupt；
- Respond；
- Events；
- Status；
- Close。

可选能力：
- Steer；
- Fork；
- Native Goal；
- Usage Reporting；
- Queue Message。

Adapter 应报告真实能力，不支持的能力不得伪装支持。

普通 Worker 在启动时读取已配置 CLI 的帮助和版本。Codex 实现适配器检查隔离配置、工作区沙箱、JSON 事件以及按 Session ID 恢复的子命令；Claude Code 检查受限启动、工具范围、流式事件和恢复参数。只读 Codex 角色沿用结构化输出探测。探测依赖显式配置的可执行文件时，缺少必需能力会使启动失败。

Owner 在“执行节点”页查看每个适配器的版本和操作，API 字段为 `GET /workers` 的 `capabilities.adapters`。两种普通执行适配器当前报告 `probe`（检查 CLI）、`start`（启动回合）、`resume`（使用原会话发起新回合）、`events`（读取原生事件）。节点重新启动后，下一次心跳替换整份报告。老节点继续上报原有的任务能力字段。

启用 App Server 的 Codex 节点另报 `interrupt`，页面显示“中断回合”。该声明通过附加心跳查询参数传递，旧控制面忽略它并继续接收原有操作。切回 CLI 后，下一次心跳撤下中断能力。

在节点页可按适配器核对普通 Worker 启用的角色。API 在每个适配器的 `profiles` 下按 `implementation`、`planner` 保存最近心跳。版本 1 的 `version` 是整数，`permission` 是原生权限参数，Codex 另报 `sandbox`，Claude 另报 `tools`。Codex 实现使用 workspace-write，规划使用 read-only，两者审批均为 never。Claude 实现使用 acceptEdits，只启用 Read、Glob、Grep、Edit、Write。未探测到的角色不出现。旧节点的心跳会撤下上次报告的配置。

两种 CLI 通过同一个 Go 接口提供身份、版本和单回合执行。Codex 报告 `turn.failed`，或 Claude 报告失败的 `result` 时，Worker 保留模型失败及进程退出码，结束本次执行。Owner 可从失败任务发起新执行。若进程异常退出且缺少终态，适配器已确认可恢复的原生会话，Worker 会复核租约、Worktree 和起始提交，再从原会话重试一次。只读 Planner 还会检查工作树干净并清除上一次的临时输出。

## 4. Codex 接入建议

优先评估 Codex App Server，利用其原生 Thread、Turn、事件流、恢复、中断和审批机制。具体协议版本、方法名称和能力必须以实际安装版本的官方文档和探测结果为准。

普通 Worker 可设置 `FORGEOS_CODEX_TRANSPORT=app-server`，通过本地 stdio 执行 Codex 实现任务和只读 Planner。启动时检查可执行文件支持的参数。每回合核对返回的工作目录、Thread、沙箱、网络限制、可写目录和审批模式，再发送任务。事件必须属于当前 Thread 与 Turn，结构化计划只在该回合正式完成后保存。进程、会话及工具活动沿用任务的执行记录。未设置该变量时使用 `exec`。

App Server 使用当前 Harness 身份的登录状态，按角色固定 `never` 审批和沙箱。进程禁用 apps、plugins、hooks、MCP 和网页搜索，设置本次基础指令与空开发者指令，关闭自动载入项目指令。beta #168 已从正式 Worker 完成实现、Owner 退回、返工期间原生进程被终止后的同 Thread 恢复，以及 PR #169 的必需 CI、合并和 Issue 关闭。只读 Planner 已返回固定计划并在 Chrome 回读。该接入已验证版本为 `0.169.0-alpha.11.1`；正式流程中的原生审批和运行中输入仍待验收。[逐项证据](acceptance.md)

工作台暂停或取消 App Server 回合时，适配器在已核对 `turn/started` 后向当前 Thread 和 Turn 发送 `turn/interrupt`。收到匹配终态后关闭进程，沿用原来的暂停检查点或取消结算。尚未确认活动回合、请求失败或五秒内未退出时，使用进程终止兜底；取消后的输出不会成为候选。beta #170 已从 Chrome 暂停、重启控制面、恢复原 Thread，并经 PR #171 完成交付；只读 Planner 同样恢复到原 Thread 产出草案。beta #172 从 Chrome 取消，工具子进程退出，后续任务取得已释放资源。三次原生会话均记录了 `turn_aborted`、原因 `interrupted`，正常中断进程的退出码均为 0。

在已登录的 Harness 用户下执行 `python scripts/probe_codex_app_server.py --bin <原生可执行文件>`，通过本地 stdio 启动两个独立 App Server。探测核对服务返回的只读 sandbox，完成首轮后关闭进程，在第二个进程恢复原 Thread 并读取随机标识，再中断第三轮。以返回的 Turn ID 和 `turn/completed` 状态核对中断。当前实测版本要求先接收 `turn/started` 通知，再调用 `turn/interrupt`。可用 `codex app-server generate-json-schema --out <目录>` 对照安装版本的字段。[App Server 协议说明](https://developers.openai.com/codex/app-server/)

用同一命令加 `--approvals` 实测命令审批。探针在只读 Thread 中用 `accept` 批准一次只输出随机数的 Python 命令，核对工具项状态和标准输出，然后在同一 Thread 再次提交相同命令，核对新的审批请求并拒绝。接下来分别拒绝和取消临时目录中的标记脚本，检查标记文件始终不存在、拒绝后回合完成、取消后回合中断。每次审批均比对工作目录、命令、Thread、Turn 和工具项编号。输出保留原生回执。

## 5. Claude Code 接入建议

第一版可评估 Headless CLI + stream-json + Session Resume；复杂交互可后续评估 Agent SDK。必须通过真实环境验证会话恢复、问题交互和权限事件。

在已登录的 Harness 用户下运行 `python scripts/probe_claude.py --bin <原生可执行文件>`。探测会发起两次真实模型调用。第一次在临时目录启动会话，接收并回答 AskUserQuestion，拒绝 Write 权限。第二次启动新的 CLI 进程，按 Session ID 恢复，要求返回第一次消息中的随机标识和选择的答案。脚本核对答复、权限请求的目标路径和文件实际状态后输出版本、Session 和两个 PID。Windows 上将 `--bin` 指向原生 `.exe`。

### 将实现任务交给 Claude Code

在 Worker 专用账号的环境中设置 `FORGEOS_CLAUDE_BIN` 为本机 Claude Code 可执行文件；Windows 使用原生 `.exe`。Worker 启动时检查版本、`--restricted` 和 `--safe-mode`。通过探测的节点才会声明 `supports_claude`。模型认证沿用服务账号的登录或专用用户设置。用户设置位于该账号的 Claude 配置目录，受限模式下由 Worker 显式传给本机 CLI。[Claude Code 设置来源](https://code.claude.com/docs/en/settings)

Owner 可在导入 Issue 时提交 `{"validation":"auto","harness":"claude"}`，也可在预览反馈的独立工作项表单选择 Claude Code。显式选择优先于项目默认。新任务将 Harness 选择与 Issue 快照一起冻结，重复导入复用原记录。Claude 节点执行实现与退回修订，计划任务继续由具备只读规划能力的 Codex 节点领取。既有的仅验证任务沿用原候选。

项目可在仓库根目录提交版本 1 的 `project.yaml`，在 `execution.default_harness` 填写 `claude` 或 `codex`。手工导入和带 `forgeos-agent` 标签的自动派发均读取登记克隆的固定基础提交。没有声明时默认 Codex。任务详情内的“项目执行配置”显示实现 Harness 与文件的 Git blob。工作区未提交的改动不会改变配置来源。规划任务使用项目默认值为后续写入型子项选择 Harness。[项目配置示例](../schemas/project.yaml.example)

执行时 CLI 使用文件读取、搜索和编辑工具，限制在工作树内；Worker 禁用扩展自定义、MCP 和浏览器集成，并传入选定的系统和模型环境变量。CLI 结果中的原生会话编号、实际版本和候选 SHA 在任务详情可查看。提交、验证、验收与 GitHub 交付沿用原有工作项流程。

## 6. Session 与 Process

- Session 是 Harness 原生持续上下文。
- Process 是 OS 进程实例。
- Execution 是中台一次工作尝试。
- 一个 Execution 可以经历多个 Process Instance。
- 普通等待、回答和进程重启不自动创建新 Execution。
- Session 丢失时用 Handoff Context 创建新 Session，并保留关联。

只读 Planner 进入主动暂停前，Worker 将冻结的讨论、Thread、工作区身份、Git 树、剩余执行时间和资源阶段写入节点日志。原生规划恰好完成时也保存已验证的草案。写入型任务使用同一节点日志和恢复领取机制，另保存原检查点的输入及尚未提交的业务问题。当实现回合已经完成时，恢复后继续处理该回合的结果。原租约有效且节点身份和工作目录一致时，重启的 Worker 等待 Owner 恢复，核对版本和工作区后继续。并发接回时，控制面只允许一个进程领取本次恢复；Owner 在等待期间取消则直接结算任务。

Codex 写入回合与只读 Planner 从 CLI 的 `thread.started` 获取 Thread ID。两者共用进程登记与退出回执，保留原生版本号、PID 和每轮实际会话。管理与审查角色的独立调用仍沿用各自的回执。

已确认原生 Thread 的 Codex 进程异常退出、且事件流中没有终态或明确报错时，Worker 重新确认租约、工作区身份和起始提交，用相同权限执行一次 `codex exec resume <thread>`。写入回合保留未提交的编辑。只读规划还要求工作区干净，并清除上次生成的临时输出，重新验证恢复回合生成的结构化计划。

在任务详情的执行记录查看“最近的节点进程报告”。正在运行的页面每 8 秒读取最新任务；也可手动刷新。工作台显示节点、实例号、PID、Harness 版本、已确认的原生 Session、进程开始时间及控制面收到报告的时间。报告反映节点上报时的状态。失联时保留最近一次观察和原执行的租约状态。收到最终结果后，页面显示完整的 Harness 进程记录。

具备实时上报能力的控制面在模型任务的 claim 响应中返回 `report_live_process`。Worker 在 SQLite 记录启动、原生会话和退出后通知上报循环，并每 15 秒重读最近进程。通过 `POST /internal/executions/{id}/process` 发送执行租约令牌和 process 对象。节点需要对应项目与执行权限；控制面核对租约、任务指定的 Harness、进程身份与时间顺序，拒绝过期租约和旧状态重放。上报失败时，下一次从本地日志重读。此请求独立于租约续约和最终回执。

控制面在普通模型任务中通过 `record_tool_activity` 请求采集工具事件。在“最近的节点进程报告”查看工具名、调用编号、节点时间和开始、完成或失败状态。调用编号取第一次观察到该调用的事件序号，同一进程中的并行调用可分别追踪。只有终态事件的工具也会显示。执行结束后，在“Harness 进程记录”展开对应进程继续查看。每个进程显示最近 32 条事件，窗口移过开头时显示事件总数。Worker 本地 SQLite 保存该进程从开始采集起的全部工具事件。

Codex 从 `item.started` 和 `item.completed` 识别命令执行、文件修改等已知类型。Claude Code 从 `tool_use` 和同一调用的 `tool_result` 记录已授权工具。事件字段只含序号、调用编号、工具类型、状态和时间；命令、参数、路径、正文与工具输出均不进入此记录。旧版 Worker 留下的进程记录可继续读取。

普通实现与只读规划的领取响应通过 `record_model_output` 开启公开消息记录。Codex CLI 的已完成 agent_message、App Server 的已完成 agentMessage 和 Claude 的 assistant 文本块进入同一份进程输出；思考块、工具参数及用户输入不进入这份记录。每次完整消息到达后更新，节点在写入 SQLite 前遮蔽可识别的敏感行、执行租约、当前及已观察的轮换节点凭据，以及敏感环境变量的值。每个进程仅保留最近 8 KiB 和递增序号。

运行中从任务详情“最近的节点进程报告 → 模型公开消息”查看；结束后从“Harness 进程记录”回读。输出沿用进程上报的项目权限、租约与身份检查，同一实例拒绝旧序号、同序号改写和退出后改写；最终回执也不得删除或回退已上报的公开消息。控制面重启后可回读，最终记录随原结果缓冲、重报并归档。旧控制面未声明采集时，新 Worker 沿用原协议；旧 Worker 及历史记录不补造输出。工具正文、内置验证和启动阶段等其他实时输出继续单列验收。

正式 Worker 通过 Claude Code 的 `stream-json` 读取原生初始化和终态事件。SQLite 的 `harness_processes` 为每个启动的适配器进程记录本地实例号、PID、探测到的版本、开始时间、CLI 确认的 Session、退出时间与退出码。使用原生可执行文件时，该 PID 即 Claude 进程的 PID。通过租约摘要关联 `attempts` 下的同一次执行。

若已确认 Session 的进程以非零状态退出，且没有收到终态，Worker 等待旧进程退出，向控制面确认原租约，核对原工作区、分离 HEAD、Git 目录和起始提交后，使用相同工具权限与 `--resume <session>` 恢复一次。恢复提示要求先检查现有编辑，再完成剩余工作。已报告的模型失败、身份不符、失去租约、变更起始提交或取消中的任务保留失败状态。一个执行无需为这次恢复创建新的 Worktree。


## 7. Checkpoint

至少保存：
- Execution ID；
- Harness 类型、版本；
- Native Session ID；
- Worktree ID、路径、分支；
- Base Commit；
- Requirement Revision；
- Plan Revision；
- 最后事件序号；
- Pending Request；
- 当前状态；
- 是否可恢复。

## 8. Workspace

每次写入型 Execution 记录实际使用的 Worktree、分支、Base Commit、路径、Diff 状态和生命周期，确保不同写入型 Work Item 的改动彼此隔离。同一 Work Item 重试时是否复用之前的 Worktree，见[待细化事项 B.5](16-open-decisions.md)。

审查任务使用固定 Commit 的只读工作区。测试任务使用固定版本的测试工作区。

## 9. 进程和资源

- Daemon 由 systemd 运行；
- 不以 root 执行 Agent；
- 显式设置 HOME、PATH、代理和工作目录；
- PID 不是 Session 身份；
- 资源不足时排队；
- 进程退出不等于任务完成；
- 不允许因为租约丢失而盲目重复执行。

## 10. 恢复策略

Linux Worker 为原生回合创建独立进程组。回合被取消、事件解析中断或启动后的登记失败时，Worker 终止该组，包括仍在组内运行的工具。

运行中每个执行凭自己的租约令牌独立续约。有效期内的短暂断线可以重试；永久拒绝或超过已确认的截止点会取消该执行的上下文。控制面保存原执行与工作区，由 Owner 核对处置。具名节点可在原租约续约和 CI 上传期间轮换身份令牌。使用节点令牌文件，并按[重叠授权、切换、撤销的顺序](local-development.md#为多个-worker-分配项目)操作。[租约接口与时间窗口](local-development.md)

Worker 已支持执行结果断线缓冲和重启补报。查询本机领取记录、未确认结果和冲突回执的方法见[Worker 执行恢复日志](13-data-storage-and-audit.md#worker-执行恢复日志)。

实现回合等待业务答复时，Worker 随问题登记暂存区和工作区两棵 Git 树的 SHA。工作区树包含 Git 会暂存的新增、修改和删除。Owner 可从任务详情核对两者。答复后，Worker 在原租约和工作区中重新计算这两个 SHA；任一变化都会使该次执行失败，保留工作区供核对。Owner 可从失败执行重试，取得新的工作区。

若 Daemon 在这个检查点退出，在租约到期前以相同配置启动 Worker，可接回原执行。接回时沿用剩余执行时间与资源申请序号，核对 Harness 版本和 Git 工作区。若同时启动两个替代进程，控制面只允许一个取得答复并恢复原会话。已领取答复的进程在启动下一回合时中断，或租约已过期，须核对现场后再处置。

接回业务问题后，Worker 用原 Session 处理答复。若原生客户端明确报告该 ID 缺失，Worker 在同一工作区再次核对执行租约、Git 目录、起始提交及提问时保存的两棵树，再将已确认的检查点输入和当前答复交给新会话。控制面把这次交接关联到已领取的答复、提问进程和下一检查点。任务详情保留提问进程、恢复失败的进程和接手的进程。

- Harness 进程崩溃：尝试恢复原 Session；
- Daemon 重启：恢复执行记录、Worktree 和事件；
- 服务器重启：检查工作区、租约、Session；
- Session 不可恢复：创建 Handoff Context；
- 对迁移、部署等非幂等操作，恢复前必须检查实际副作用。

写入进程在回合中异常退出时，Worker 在尝试恢复原会话前记录暂存区和工作区两棵树。如果原生恢复命令随后确认会话缺失，控制面在仍有效的模型资源与执行租约内登记两次退出回执。Worker 再次核对两棵树，将原进程最后的工具事件、草稿位置及冻结输入交给新的写入会话。终态进程历史必须能逐项匹配这些回执。

Codex 0.169.0-alpha.11.1 和 Claude Code 2.1.310 均已实测识别原生会话缺失。Worker 只在恢复命令退出码为 1、原生客户端尚未确认会话，而且缺失回执包含本次请求的确切 ID 时记录该原因。任务详情可区分会话缺失与普通回合失败。

只读 Codex Planner 确认原 Thread 缺失后，Worker 再次确认执行租约、原工作区、固定提交和干净的 Git 状态，清除上次规划的临时输出，在同一工作区以只读、禁网权限启动新 Thread。新输入保留原规划请求和讨论，并写明来源 Thread、执行编号和固定提交。任务详情的进程记录可回查两个 Thread 及中间失败的恢复尝试。

Codex 或 Claude Code 实现任务在首个检查点主动暂停后，也可从缺失的原生会话交接。Worker 在恢复命令确认缺失后复核租约、Git 目录、起始提交、暂存区树和工作区树。新会话取得这些树的标识和冻结的任务输入，在原工作区检查现有草稿后继续。控制面将来源进程与同一执行中 Owner 已恢复的暂停记录核对。

在后续检查点，支持交接的 Worker 会按序重读已确认回合的冻结讨论、Owner 指令和已答复的业务问题，并分别标明当时的会话、指令和答复来源。当前回合仍使用暂停时冻结的输入。控制面在同一事务内登记暂停来源、新会话及接收检查点，确认该回合；候选审核仍可回查每条指令实际进入的会话。beta #151 从 Claude 检查点 2 核对了早期的 Owner 答复。beta #153 从 Codex 检查点 2 核对了独立 Management 的 D1 答复、原决定和适用范围。
