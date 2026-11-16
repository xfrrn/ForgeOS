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

## 4. Codex 接入建议

优先评估 Codex App Server，利用其原生 Thread、Turn、事件流、恢复、中断和审批机制。具体协议版本、方法名称和能力必须以实际安装版本的官方文档和探测结果为准。

在已登录的 Harness 用户下执行 `python scripts/probe_codex_app_server.py --bin <原生可执行文件>`，通过本地 stdio 启动两个独立 App Server。探测核对服务返回的只读 sandbox，完成首轮后关闭进程，在第二个进程恢复原 Thread 并读取随机标识，再中断第三轮。以返回的 Turn ID 和 `turn/completed` 状态核对中断。当前实测版本要求先接收 `turn/started` 通知，再调用 `turn/interrupt`。可用 `codex app-server generate-json-schema --out <目录>` 对照安装版本的字段。[App Server 协议说明](https://developers.openai.com/codex/app-server/)

## 5. Claude Code 接入建议

第一版可评估 Headless CLI + stream-json + Session Resume；复杂交互可后续评估 Agent SDK。必须通过真实环境验证会话恢复、问题交互和权限事件。

在已登录的 Harness 用户下运行 `python scripts/probe_claude.py --bin <原生可执行文件>`。探测会发起两次真实模型调用。第一次在临时目录启动会话，接收并回答 AskUserQuestion，拒绝 Write 权限。第二次启动新的 CLI 进程，按 Session ID 恢复，要求返回第一次消息中的随机标识和选择的答案。脚本核对答复、权限请求的目标路径和文件实际状态后输出版本、Session 和两个 PID。Windows 上将 `--bin` 指向原生 `.exe`。

### 将实现任务交给 Claude Code

在 Worker 专用账号的环境中设置 `FORGEOS_CLAUDE_BIN` 为本机 Claude Code 可执行文件；Windows 使用原生 `.exe`。Worker 启动时检查版本、`--restricted` 和 `--safe-mode`。通过探测的节点才会声明 `supports_claude`。模型认证沿用服务账号的登录或专用用户设置。用户设置位于该账号的 Claude 配置目录，受限模式下由 Worker 显式传给本机 CLI。[Claude Code 设置来源](https://code.claude.com/docs/en/settings)

Owner 可在导入 Issue 时提交 `{"validation":"auto","harness":"claude"}`，也可在预览反馈的独立工作项表单选择 Claude Code。新任务将 Harness 选择与 Issue 快照一起冻结，重复导入复用原记录。未指定时默认使用 Codex。Claude 节点执行实现与退回修订，计划任务继续由具备只读规划能力的 Codex 节点领取。既有的仅验证任务沿用原候选。

执行时 CLI 使用文件读取、搜索和编辑工具，限制在工作树内；Worker 禁用扩展自定义、MCP 和浏览器集成，并传入选定的系统和模型环境变量。CLI 结果中的原生会话编号、实际版本和候选 SHA 在任务详情可查看。提交、验证、验收与 GitHub 交付沿用原有工作项流程。

## 6. Session 与 Process

- Session 是 Harness 原生持续上下文。
- Process 是 OS 进程实例。
- Execution 是中台一次工作尝试。
- 一个 Execution 可以经历多个 Process Instance。
- 普通等待、回答和进程重启不自动创建新 Execution。
- Session 丢失时用 Handoff Context 创建新 Session，并保留关联。

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

- Harness 进程崩溃：尝试恢复原 Session；
- Daemon 重启：恢复执行记录、Worktree 和事件；
- 服务器重启：检查工作区、租约、Session；
- Session 不可恢复：创建 Handoff Context；
- 对迁移、部署等非幂等操作，恢复前必须检查实际副作用。
