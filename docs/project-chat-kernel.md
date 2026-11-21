# 项目聊天内核

ForgeOS 将 Codex 的 Agent 核心放在 `kernels/codex-core`，无界面执行模块放在 `kernels/codex-exec`。源文件来自 OpenAI Codex 的 `codex-rs/core` 和 `codex-rs/exec`，固定提交为 `581161ab41e643d942e1e1b5f561034060bf18e3`（`rust-v0.171.0-alpha.13`）。许可证和上游声明随源码保存。ForgeOS 入口是 `kernels/codex-exec/src/forgeos.rs`。项目内可直接修改核心中的回合调度、上下文和工具路由；剩余 Rust 依赖由同一提交的工作区提供。

## 构建与启用

准备 Git、PowerShell 7.4+ 和上游 `codex-rs/rust-toolchain.toml` 指定的 Rust 工具链。Windows 原生编译还需要 MSVC、Windows SDK 以及上游依赖的原生构建工具。在 ForgeOS 根目录执行以下示例。已有克隆可跳过第一条并替换源码路径。

```powershell
git -c core.longpaths=true clone --depth 1 --branch rust-v0.171.0-alpha.13 https://github.com/openai/codex.git ../codex-source
./scripts/build-chat-kernel.ps1 -CodexSource ../codex-source -BuildDirectory ../forgeos-kernel-build
```

脚本从固定提交创建独立构建目录，把 ForgeOS 维护的执行模块放入该工作区，保留上游 Cargo.lock，并仅指定 `forgeos-chat-kernel` 为构建目标。首次构建使用新目录。迭代时在原构建目录同步更改的模块文件，进入 `codex-rs` 运行 `cargo build --locked --release --package codex-exec --bin forgeos-chat-kernel`。

在项目专属的 `management-chat` Worker 上，将 `FORGEOS_CHAT_KERNEL_BIN` 设置为编译出的 `forgeos-chat-kernel.exe` 的绝对路径。启动控制面与 Worker 后，打开工作台的「账号与系统 → 密钥与凭据」，在「聊天模型」中添加连接。选择预设模型并保存 API Key 即可启用；自定义连接还需填写 Base URL、模型 ID 和回复格式。设置和聊天均显示模型 ID。所有项目可使用同一列表。项目聊天输入框旁的选择器决定下一条消息使用哪条连接。发送时固定连接版本，随后编辑或移除连接不影响已排队的消息。升级时已有的项目连接会导入共享列表。

Base URL 通常以 `/v1` 结尾。Worker 向其追加 `/chat/completions`，远程连接使用 HTTPS，本机回环地址支持 HTTP。密钥保存在控制面配置的 OpenBao 中。持有该项目有效聊天租约的节点可以在执行期间读取模型连接，内核和 MCP 工具只需连接当次回合的回环适配器。内核按项目使用独立的数据目录。

模型需要支持文本，如挂载工具还需支持 function calling。回复格式可选 JSON 对象或严格 JSON Schema。同一回合的模型请求和工具回传使用发送时选定的连接。

## 挂载项目工具

每个项目单独配置 stdio MCP 服务。服务名、程序绝对路径、参数和暴露的方法由节点配置决定。下面用一个示例服务说明字段；实际运行时需替换为已安装的服务。

```json
{
  "project_docs": {
    "command": "C:/ForgeOS/tools/project-docs.exe",
    "args": ["--project", "1"],
    "tools": ["search", "read"]
  }
}
```

在节点的可选文件中设置 `project_id` 及 `mcp_servers`，将上面的对象作为 `mcp_servers` 的值。通过 `FORGEOS_CHAT_KERNEL_CONFIG` 指定文件绝对路径。工具服务应在服务端限定该项目可读的数据范围。项目管理聊天适合挂载只读查询；对具体需求，聊天可以生成 Issue 草稿。Owner 在卡片中编辑标题、Markdown 正文及工作类型，确认后控制面在当前项目的 GitHub 仓库创建 Issue 并派发任务。仓库凭据需要 Issues 写权限。已有 Issue 派发和 HEAD 检查也通过操作卡确认。项目本身已分配的 Skill 仍可通过既有的 Skill 读取工具使用。已有项目如果自行保存过启动提示词，请在「Agent 管理」核对它是否允许使用新增的只读工具。

## 管理项目记忆

在项目的「项目资料 → 记住一条项目经验」保存条目。控制面在每次聊天中按当前项目读取最新修订，主动带入按 key 排序的前五条 Owner 记忆请求；其余条目可通过已有的项目资料检索命中。引用包含资料 key、修订号及工作台入口。记忆请求、已审核经验和已确认决策保留各自的来源和审核状态。

## 验证

```powershell
$env:FORGEOS_TEST_CHAT_KERNEL = (Resolve-Path ../forgeos-kernel-build/codex-rs/target/release/forgeos-chat-kernel.exe).Path
go test ./internal/chatkernel -count=1 -v
```

按实际构建目录调整路径。测试启动自编译的 ForgeOS 内核、本机模拟 Chat Completions 服务及 stdio MCP 工具，核对模型、鉴权、工具白名单、工具结果回传和最终结构化输出。数据库集成测试 `TestProjectChatWorkflow` 在测试 PostgreSQL 中验证跨项目隔离和长期记忆召回。

聊天节点向 Chat Completions 请求流式响应，从结构化输出中逐步提取回答正文。浏览器通过当前项目和回合的 SSE 连接接收进度，断线重连后读取最新正文。引用及操作卡在回合完成并校验后展示。提供方若直接返回完整 JSON 响应，工作台将展示完整回答。若提供方明确拒绝流式选项，节点会使用普通请求重试一次。

项目聊天内置两个只读工具。`resolve_task_reference` 在当前项目内按最近讨论、已确认派发、任务号、Issue 编号或标题定位任务，遇到多个匹配时由 Owner 选择。`get_task_progress` 查询当前任务和子任务的执行记录、交付依赖与待答问题，任务和问题链接可以随回答引用。两个工具由 Worker 按聊天回合挂载，使用当前项目的有效租约，无需额外配置 MCP 服务。
