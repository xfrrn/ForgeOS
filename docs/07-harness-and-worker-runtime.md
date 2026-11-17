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

在节点页可按适配器核对普通 Worker 启用的角色。API 在每个适配器的 `profiles` 下按 `implementation`、`planner` 保存最近心跳。版本 1 的 `version` 是整数，`permission` 是原生权限参数，Codex 另报 `sandbox`，Claude 另报 `tools`。Codex 实现使用 workspace-write，规划使用 read-only，两者审批均为 never。Claude 实现使用 acceptEdits，只启用 Read、Glob、Grep、Edit、Write。未探测到的角色不出现。旧节点的心跳会撤下上次报告的配置。

两种 CLI 通过同一个 Go 接口提供身份、版本和单回合执行。Codex 报告 `turn.failed`，或 Claude 报告失败的 `result` 时，Worker 保留模型失败及进程退出码，结束本次执行。Owner 可从失败任务发起新执行。若进程异常退出且缺少终态，适配器已确认可恢复的原生会话，Worker 会复核租约、Worktree 和起始提交，再从原会话重试一次。只读 Planner 还会检查工作树干净并清除上一次的临时输出。

## 4. Codex 接入建议

优先评估 Codex App Server，利用其原生 Thread、Turn、事件流、恢复、中断和审批机制。具体协议版本、方法名称和能力必须以实际安装版本的官方文档和探测结果为准。

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

Codex 写入回合与只读 Planner 从 CLI 的 `thread.started` 获取 Thread ID。两者共用进程登记与退出回执，保留原生版本号、PID 和每轮实际会话。管理与审查角色的独立调用仍沿用各自的回执。

已确认原生 Thread 的 Codex 进程异常退出、且事件流中没有终态或明确报错时，Worker 重新确认租约、工作区身份和起始提交，用相同权限执行一次 `codex exec resume <thread>`。写入回合保留未提交的编辑。只读规划还要求工作区干净，并清除上次生成的临时输出，重新验证恢复回合生成的结构化计划。

在任务详情的执行记录查看“最近的节点进程报告”。正在运行的页面每 8 秒读取最新任务；也可手动刷新。工作台显示节点、实例号、PID、Harness 版本、已确认的原生 Session、进程开始时间及控制面收到报告的时间。报告反映节点上报时的状态。失联时保留最近一次观察和原执行的租约状态。收到最终结果后，页面显示完整的 Harness 进程记录。

具备实时上报能力的控制面在模型任务的 claim 响应中返回 `report_live_process`。Worker 在 SQLite 记录启动、原生会话和退出后通知上报循环，并每 15 秒重读最近进程。通过 `POST /internal/executions/{id}/process` 发送执行租约令牌和 process 对象。节点需要对应项目与执行权限；控制面核对租约、任务指定的 Harness、进程身份与时间顺序，拒绝过期租约和旧状态重放。上报失败时，下一次从本地日志重读。此请求独立于租约续约和最终回执。

控制面在普通模型任务中通过 `record_tool_activity` 请求采集工具事件。在“最近的节点进程报告”查看工具名、调用编号、节点时间和开始、完成或失败状态。调用编号取第一次观察到该调用的事件序号，同一进程中的并行调用可分别追踪。只有终态事件的工具也会显示。执行结束后，在“Harness 进程记录”展开对应进程继续查看。每个进程显示最近 32 条事件，窗口移过开头时显示事件总数。Worker 本地 SQLite 保存该进程从开始采集起的全部工具事件。

Codex 从 `item.started` 和 `item.completed` 识别命令执行、文件修改等已知类型。Claude Code 从 `tool_use` 和同一调用的 `tool_result` 记录已授权工具。事件字段只含序号、调用编号、工具类型、状态和时间；命令、参数、路径、正文与工具输出均不进入此记录。旧版 Worker 留下的进程记录可继续读取。

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

- Harness 进程崩溃：尝试恢复原 Session；
- Daemon 重启：恢复执行记录、Worktree 和事件；
- 服务器重启：检查工作区、租约、Session；
- Session 不可恢复：创建 Handoff Context；
- 对迁移、部署等非幂等操作，恢复前必须检查实际副作用。
