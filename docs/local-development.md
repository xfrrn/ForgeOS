# 本地运行基座

控制面和 Worker 在同一台机器上独立运行。控制面持有数据库连接，Worker 通过回环 HTTP 领取任务和提交结果。当前使用单一 Owner 管理全部已登记项目，Owner 和 Worker 使用不同的 Bearer Token。`git_head` 用于检查基座是否连通。`codex_issue` 在独立 Worktree 中完成需求修改，生成候选 Commit 并验证。

## 准备环境

需要 Go 1.23 或更高版本、Git、可连接的 PostgreSQL，以及一个有提交的本地 Git 项目。为 ForgeOS 准备专用空数据库，连接角色需要在该库的当前 schema 中建表。控制面启动时自动迁移，之后重启会复用原数据。

以下命令在仓库根目录的 PowerShell 7.4 或更高版本中运行。将仓库根目录改成你允许执行的项目所在目录，工作区根目录使用单独的位置。选用普通用户启动 Worker。

```powershell
$env:FORGEOS_DATABASE_URL = Read-Host -MaskInput 'PostgreSQL 连接串'
$env:FORGEOS_REPO_ROOT = (Resolve-Path 'D:\projects').Path
$env:FORGEOS_WORKSPACE_ROOT = 'D:\forgeos-workspaces'
$env:FORGEOS_OWNER_TOKEN = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32))
$env:FORGEOS_WORKER_TOKEN = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32))
```

## 启动与验收

在同一个 PowerShell 中执行以下命令，两个子进程会继承各自需要的配置。默认控制面地址为 `http://127.0.0.1:8080`。若端口已占用，可在启动前同时设置 `FORGEOS_LISTEN_ADDR`（例如 `127.0.0.1:8081`）和 `FORGEOS_API_URL`（例如 `http://127.0.0.1:8081`）。

```powershell
$binDir = New-Item -ItemType Directory -Path (Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString()))
go build -o "$binDir/controlplane.exe" ./cmd/controlplane
go build -o "$binDir/worker.exe" ./cmd/worker
$control = Start-Process "$binDir/controlplane.exe" -PassThru -WindowStyle Hidden -RedirectStandardError "$binDir/control.log"
$workerEnv = @{ FORGEOS_OWNER_TOKEN = $null; FORGEOS_DATABASE_URL = $null; FORGEOS_TEST_DATABASE_URL = $null; GH_TOKEN = $null; GITHUB_TOKEN = $null }
$worker = Start-Process "$binDir/worker.exe" -PassThru -WindowStyle Hidden -Environment $workerEnv -RedirectStandardError "$binDir/worker.log"
.\scripts\smoke.ps1 -ProjectPath 'D:\projects\example'
```

验收脚本登记项目、创建工作项、轮询结果，并与提交任务时的 HEAD 比对。选择验收期间 HEAD 稳定的仓库。成功时会输出任务 ID、执行 ID、工作区、事件、提交 SHA 和退出码。Linux 可用相同环境变量，在不同进程中运行 `go run ./cmd/controlplane` 和 `go run ./cmd/worker`。

## 打开任务工作台

在项目根目录使用 Node.js 22.12 或更高版本构建 React 页面。控制面默认从启动目录下的 `web/dist` 提供页面。如果从其他目录启动控制面，先为该进程设置绝对路径。

```powershell
npm --prefix web ci
npm --prefix web run build
$env:FORGEOS_WEB_ROOT = (Resolve-Path .\web\dist).Path
```

在浏览器打开控制面地址（默认 `http://127.0.0.1:8080/`），输入启动时配置的 `FORGEOS_OWNER_TOKEN`。令牌保存在此标签页的会话存储中；侧栏的「断开连接」会清除它。任务列表支持按标题、仓库、任务编号和状态查找。「验收队列」只列等待人工决定的任务。

点开任务可查看 Issue、历次 Worker、Worktree、原生 Session、验证输出及交付记录。在执行记录内展开「查看代码差异」核对候选。验收面板会显示本次执行 ID 和完整 SHA。「通过验收」按项目现有配置决定是否触发 GitHub 交付。「退回修改」需要填写原因，并在同一任务下重新排队。已合并的任务提供 PR 链接和最终合并 SHA。任务详情可直接以 `/#/tasks/11` 一类的地址打开。

修改前端时可运行 `npm --prefix web run dev`，并打开该命令输出的本机地址。`FORGEOS_API_URL` 可指定开发代理所连接的本机控制面，默认是 `http://127.0.0.1:8080`。

## 用真实 GitHub Issue 验收

1. 在控制面运行身份下安装 GitHub CLI，使用 `gh auth status` 确认有目标仓库和 Issue 的读取权限。在 Worker 的专用普通用户下登录原生 Codex CLI，通过 `codex login status` 核实。Windows 可在启动 Worker 前把 `FORGEOS_CODEX_BIN` 设为 `(Get-Command codex.exe).Source`。Worker 启动时探测 CLI，只有声明支持 Codex 的 Worker 才能领取这类任务。
2. 在授权仓库根目录内准备带提交的 Git 克隆，`origin` 指向 `github.com` 的 HTTPS 或 SSH 仓库。控制面按该仓库的 GitHub 数字 ID 绑定本地项目。首次派单固定 Issue 标题、正文、更新时间、本地 HEAD 和验证方式。重复导入同一 Issue 返回已有任务及原快照。
3. Owner 指定一个开放 Issue 和一项验证。下面示例假设克隆路径和 Issue 编号已替换为实际值。

```powershell
.\scripts\smoke.ps1 -ProjectPath 'D:\projects\example' -IssueNumber 20 -Validation docs
```

Worker 仅在已授权仓库的独立 Worktree 中启动 `codex exec --json`，使用 workspace-write 沙箱、禁用模型 Shell 网络并复用 CLI 登录。Issue 正文作为需求数据通过标准输入传入。使用专用 OS 用户限制其可读文件。候选从冻结的基础提交派生，原项目工作目录可继续使用。执行上限 20 分钟，租约 25 分钟。验证通过后工作项进入 `WAITING_ACCEPTANCE`，查询结果中 `execution.result` 包含原生 Session ID、基础和候选 Commit、绑定候选 SHA 的验证记录。失败时保留已有候选、证据和工作区。

| 验证方式 | 检查及运行条件 |
| --- | --- |
| `docs` | 针对两个提交运行 `git diff --check`，确认有实际变更且只涉及普通、不可执行的 `.md`、`.rst`、`.txt`、`.adoc` 文件。需求是否满足仍由维护者对照 Issue 和差异核对。 |
| `go-test` | 在候选提交的全新只读检出上运行 `go test -buildvcs=false -count=1 ./...`。需本机 Docker Linux 引擎和预备好的 Go 镜像，默认 `golang:1.25-alpine`，可用 `FORGEOS_GO_IMAGE` 指向本地镜像。容器以普通用户离线运行，项目需自包含或已 vendor 依赖。测试日志位于执行目录同级的 `execution-<id>.go-test.log`。 |

按标签自动接入时，在控制面启动前设置 `FORGEOS_POLL_PROJECT_ID` 为已登记项目 ID，`FORGEOS_POLL_VALIDATION` 为上述一种验证方式。Owner 在 GitHub 仓库中为 Agent 任务模板配置 `forgeos-agent` 标签，Poller 只派发带该标签的开放 Issue，逐页读取并与手动导入共用去重逻辑。普通讨论可由 Owner 通过编号显式转为任务。默认采用按编号派单。

启用 Poller 后，用 Owner 身份访问 `GET /projects/{id}/inbox` 查看开放的普通讨论。结果按 Issue 编号倒序排列，每页最多 100 项，下一页传 `?before=上一页最后的编号`。完整扫描结束后刷新收件箱，重复同步不会为普通讨论启动 Worker。通过下表的按编号导入接口转换后，讨论立即从收件箱移出。

## 人工验收与退回

先用 Owner 身份读取任务并查看原始 Issue、验证结果与实际差异。以下 PowerShell 命令在持有运行配置的终端执行，`$taskId` 换成要验收的任务编号。

```powershell
$api = $env:FORGEOS_API_URL
if (-not $api) { $api = 'http://127.0.0.1:8080' }
$headers = @{ Authorization = "Bearer $env:FORGEOS_OWNER_TOKEN" }
$task = Invoke-RestMethod -NoProxy -Uri "$api/tasks/$taskId" -Headers $headers
$attempt = $task.execution
$task.spec.issue
$attempt.result.validation
git --no-replace-objects -C $attempt.workspace diff --no-ext-diff --no-textconv $attempt.result.base_commit $attempt.result.commit --
```

核对后，可提交通过或退回。下面是通过的请求。退回时将 `decision` 改为 `reject`，并在 `note` 写明应如何修正。

```powershell
$body = @{ execution_id = $attempt.id; commit = $attempt.result.commit; decision = 'approve'; note = '已核对需求、差异和验证结果' }
Invoke-RestMethod -NoProxy -Method Post -Uri "$api/tasks/$taskId/acceptance" -Headers $headers -ContentType 'application/json' -Body ($body | ConvertTo-Json)
```

相同执行、SHA、决定和说明的重发只记录一次。版本不匹配或与已有决定矛盾时返回 HTTP 409。退回后同一工作项重新排队，Worker 在新 Worktree 从被退回版本继续修订。新的候选提交保留原始基础版本为父提交，须重新验证和验收。历次候选和验收决定可从 `attempts`、`reviews` 查阅；工作项结束前保留相关 Worktree，让提交对象有可用的 Git 引用。

## 将 Issue 工作项升级为 Goal 草案

首次按编号导入复杂 Issue 时可传 `{"mode":"plan","validation":"docs"}`。工作项冻结规划模式，只有探测到只读沙箱和结构化输出能力的 Worker 才能领取。Worker 在冻结版本的独立 Worktree 中调用 Codex，只读查阅项目资料，校验返回的任务图、原始 HEAD 和工作区状态。成功后控制面保存原生 Session、基础 SHA 和计划修订 1，进入 PLANNING。维护者可核对范围、引用路径、依赖和验收条件，再显式激活。既有任务复用首次导入的模式。

Owner 向 `POST /tasks/{id}/plans` 提交以下结构。首次提交传 `previous_revision: 0`，后续传当前修订号。每次成功返回完整版本。相同版本和内容的重报返回原记录，版本冲突返回 409。支持待执行、执行失败、待验收和规划中的 Issue 任务。运行中的任务先到达安全停止点。

```json
{
  "previous_revision": 0,
  "plan": {
    "objective": "补齐队列手册",
    "milestones": [{
      "name": "使用示例",
      "items": [
        {"key":"status","objective":"记录状态命令","scope":"README","acceptance_criteria":"包含可复制的命令","deliverables":"命令示例"},
        {"key":"waiting","objective":"解释等待状态","scope":"README","acceptance_criteria":"能从示例辨认待执行任务","deliverables":"输出示例","dependencies":["status"]}
      ]
    }]
  }
}
```

`GET /tasks/{id}` 和任务详情页展示当前版本，`GET /tasks/{id}/plans/{revision}` 可查询指定历史版本。原执行和验收记录随任务保留。草案进入 PLANNING，Worker 不领取父项。

确定正式计划后，Owner 调用 `POST /tasks/{id}/plans/{revision}/archive`。返回的 `git_commit` 对应项目仓库中的独立快照，以需求冻结的基础提交为父提交。用 `git show refs/forgeos/plans/goal-1/r2:.forgeos/plans/goal-1.json` 可读取任务 1 的第二版计划。每版单独保留，可重复归档；引用内容冲突返回 409。该操作写本地项目 Git。远端交付仍经既有验收流程。

Owner 可显式批准当前计划并调用 `POST /tasks/{id}/plans/{revision}/activate`。同一修订重复激活复用已有工作项。目标进入 ACTIVE，无前置依赖的子项进入 READY，其他子项进入 BLOCKED。任务树提供实际工作项链接，子项详情可回到父 Goal。每个子项使用计划中的范围、上下文引用和验收条件，沿用原项目的验证方式。每个子 PR 回写候选和合并 SHA，父 Issue 保持开放。

交付器在前置子项全部完成 GitHub 回写后检查目标分支。它获取分支提交到项目对象库，核对原始基础及每个前置 Merge SHA 都在该提交的历史中，再把后续子项的基础版本固定并转为 READY。可以从子项的 `spec.base_commit` 和 `dependencies_delivered` 事件复核派发依据。

待所有里程碑已有详细工作项、每个子任务和交付记录均为 DONE 后，交付器核对原仓库和父 Issue 身份，回写一次 Goal 摘要并关闭 Issue。远端确认后才把父 Goal 标为 DONE。待细化的空里程碑会保留父 Issue。

## 验收检查点的业务问题

在候选完成验证、需要业务澄清时，Owner 可向 `POST /tasks/{id}/questions` 提交 `execution_id`、`commit_sha`、`request_key`、`question` 和 `evidence`。控制面将任务置为 WAITING_INPUT，暂停对此 SHA 的验收。按「等待回答」筛选任务可定位问题。维护者在详情页回答，或向 `POST /tasks/{id}/questions/{question}/answer` 提交 `{"answer":"业务答复"}`。任务回到同一执行、会话及候选的验收检查点，历史问题和答复保存在 `questions` 中。

## GitHub 交付

默认情况下，通过人工验收只将任务记为 `ACCEPTED`。要让某个项目自动发布，在**批准该候选前**配置控制面的 `FORGEOS_DELIVERY_PROJECT_ID`，使用有权向此仓库推送、管理 PR 和更新 Issue 的 GitHub 身份，再重启控制面。可选 `FORGEOS_DELIVERY_TARGET_BRANCH` 指定目标分支，留空时采用 GitHub 仓库默认分支。使用普通受保护分支权限的凭据。审批发生时会同时保存交付任务，之后更改配置不会把之前的本地验收追溯发布。

交付器根据已验收 SHA 推送任务分支、创建或找回对应 PR，并核对远端 PR 的仓库、目标分支和 head。门禁尚未通过时，`delivery.detail` 说明等待原因；维护者可在 GitHub 完成所需 Review 或检查。合并使用冻结的 head SHA 和 Squash 策略。回写结果并关闭对应简单 Issue 后，任务进入 `DONE`。`delivery.pr_url` 和 `delivery.merge_sha` 可用于核对远端结果。服务重启或外部请求超时后，会先查询已有 PR、合并和反馈，再决定下一步操作。

如果某项 CI 必须通过才能合并，请在 GitHub 目标分支的保护规则中把对应的检查名列为必需，并让规则适用于交付账号。GitHub Actions 的工作流可能在 PR 创建后才上报检查。仅配置 `pull_request` 工作流时，GitHub 在首份状态上报前可能允许合并。本次私有仓库验收将 `example` 设为来自 GitHub Actions 的必需检查，开启严格模式并将规则应用到管理员。

## API 与故障恢复

| API | 身份 | 用法 |
| --- | --- | --- |
| `POST /projects` | Owner | 请求体 `{"path":"example"}`。路径可相对于授权根目录，同一实际仓库重复登记返回相同 ID。 |
| `POST /tasks` | Owner | 请求体 `{"project_id":1,"kind":"git_head"}`。返回新工作项，初始状态为 `READY`。 |
| `GET /tasks` | Owner | 按 ID 倒序返回任务摘要和各状态总数。可选 `q`、`status`、`limit`（默认 30，最多 100）及 `before`。下一页使用响应的 `next_before`。 |
| `POST /projects/{id}/issues/{number}` | Owner | 请求体 `{"validation":"docs"}` 或 `{"validation":"go-test"}`。读取该仓库的开放 Issue 并派单。 |
| `GET /tasks/{id}` | Owner | 查询任务状态、执行历史、结果和事件。连通性任务成功为 `DONE`，Issue 通过本地验证为 `WAITING_ACCEPTANCE`，执行失败为 `FAILED`。 |
| `GET /tasks/{id}/diff` | Owner | 提供 `execution_id` 和完整候选 `commit`，读取该次执行相对冻结基础提交的 Git 差异。展示上限 256 KiB，超出时标记 `truncated`。 |
| `POST /tasks/{id}/acceptance` | Owner | 提交 `execution_id`、完整 `commit`、`decision` 和可选的 `note`。退回必须附说明，批准时按该项目配置决定是否交付 GitHub。 |
| `POST /internal/claim` | Worker | 领取任务，空队列返回 HTTP 204。每个项目同时最多一个运行中的任务。 |
| `POST /internal/executions/{id}/result` | Worker | 以领取时的租约令牌提交结果。重发相同结果只记一次。 |
| `POST /executions/{id}/abandon` | Owner | 核对失联执行并确认原 Worker 已停止后，人工结束过期租约。标记失败并释放项目名额，保留工作区与事件。 |
| `POST /executions/{id}/retry` | Owner | 核对失败原因后重试该次 Issue 执行。相同执行 ID 的重试请求幂等，继续沿用原工作项与冻结需求，下一次执行使用新 Worktree。 |

Worker 失联时，先从任务详情查 `execution.lease_expired`、执行 ID 和工作区。租约过期后中台仍等待原执行回报，避免同一任务执行两次。人工结束后，Issue 使用上表中的重试接口，连通性测试可创建新的 `git_head` 任务。工作区保留用于核对，完成核对后可用 `git -C <仓库> worktree remove <工作区>` 清理。

在没有运行中任务时，可在原 PowerShell 中执行 `Stop-Process -Id $worker.Id,$control.Id` 停止两个进程。再次启动控制面时指向同一数据库即可恢复查询。启动失败时查看 `$binDir/control.log` 和 `$binDir/worker.log`。

## 测试

`go test ./...` 运行 HTTP、GitHub 接入和 Worker 测试。交付测试向临时裸 Git 仓库推送，并模拟 GitHub 合并门禁与响应丢失。设置 `FORGEOS_TEST_DATABASE_URL` 为独立测试库连接串后运行 `go test -count=1 ./...`，还会验证旧数据迁移、派发、重报、失联与重试、验收退回和交付队列恢复。若本地已备好 Docker 引擎和 Go 镜像，设置 `FORGEOS_TEST_DOCKER=1` 同时验证真实 Go 测试通过和失败的情形。

## Management 审核进程

控制面设置 `FORGEOS_MANAGEMENT_PROJECT_ID` 和独立的 `FORGEOS_MANAGEMENT_TOKEN` 后，为指定项目开放审核队列。用普通用户启动 `go run ./cmd/worker management`，给这个进程设置同一 Management 令牌和回环 `FORGEOS_API_URL`。它只需已登录的本地 Codex CLI，无需仓库目录、数据库密码或 Owner、Worker、GitHub 令牌。

管理进程自动领取最新的待规划修订，在单独的只读会话中审阅冻结的 Issue 和草案。任务详情显示建议、判定等级、理由、依据和原生会话 ID。Owner 可通过 `GET /tasks/{id}/plans/{revision}` 查阅历史版本。模型执行限时五分钟，领取租约六分钟；进程退出后可重新领取。

未指定策略时，最低决定等级为 D2。Owner 保存修订时可附加 `minimum_level`，也可在首次领取前向 `POST /tasks/{id}/plans/{revision}/policy` 提交该字段。已指定的等级绑定当前修订。D1 只开放给 docs 验证的低风险范围；管理建议只能提高等级。D1 通过会在同一事务中激活子任务；退回、请求人工决定、D2 或 D3 均保留草案，Owner 可保存下一版或通过现有激活入口明确批准。代码交付仍经过独立的候选验收与 GitHub 门禁。
