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

### 验证项目的本地启动环境

项目可在 `project.yaml` 的 `local_environments` 声明 `local-mock`、`local-integrated` 或两者。各环境包含受 Git 跟踪的仓库相对路径 `start_script`、应用监听的 `port`、`health_path` 和 1–300 秒的 `timeout_seconds`。提交脚本和配置后运行工作台的 HEAD 检查，并使用其完整提交号。[配置示例](../schemas/project.yaml.example)

执行节点使用非 root 身份、本地 Docker 引擎及预先准备的 Linux 镜像。设置 `FORGEOS_STARTUP_IMAGE` 为包含应用运行时、`/bin/sh`、`/usr/bin/curl` 和 `/usr/bin/timeout` 的本机镜像。Runner 解析本地镜像 ID，以只读挂载读取提交快照，在容器临时目录运行脚本。脚本可读取 `FORGEOS_LOCAL_ENVIRONMENT` 选择启动方式，并应保持前台运行。Worker 不向容器传入宿主环境变量或 Vault 凭据。

```powershell
$env:FORGEOS_WORKER_ID = 'local-startup'
$env:FORGEOS_STARTUP_IMAGE = 'golang:1.26.9-bookworm'
$request = @{ repo = 'example-project'; commit = (git -C 'D:\projects\example-project' rev-parse HEAD); environment = 'local-mock' }
$request | ConvertTo-Json -Compress | .\worker.exe verify-environment
```

Mock 模式只使用容器回环网络。集成模式由操作者创建项目专属的内部 bridge 网络，并连接实际需要的测试依赖。网络标签 `forgeos.project` 为仓库相对于节点 `FORGEOS_REPO_ROOT` 的路径，使用正斜线；`forgeos.environment` 为 `local-integrated`。将网络名称或 ID 配置在 `FORGEOS_STARTUP_NETWORK`。例如：

```powershell
docker network create --internal --label forgeos.project=example-project --label forgeos.environment=local-integrated example-integrated
$env:FORGEOS_STARTUP_NETWORK = 'example-integrated'
$request.environment = 'local-integrated'
$request | ConvertTo-Json -Compress | .\worker.exe verify-environment
```

探测请求在应用容器内访问声明的回环端口。2xx 表示就绪，重定向与服务错误持续至截止时间则失败。标准输出和 `FORGEOS_WORKSPACE_ROOT/environment-run-*/run.json` 保存 Worker、提交、配置和脚本 blob、镜像 ID、网络、探测次数、HTTP 状态与容器回收结果。失败返回非零。该入口不发布宿主端口；独立的服务依赖由其操作者管理。

要从工作台派发，在运行 Worker 守护进程前设置相同的启动镜像。需要 Integrated 的节点还要设置 `FORGEOS_STARTUP_PROJECT_ID` 为工作台中的项目数字 ID，并配置对应的内部网络。节点通过心跳声明能力，启动任务占用这个节点的一个 CI 名额。缺少可用节点时，任务留在待执行队列。

Owner 从成功的 HEAD 任务读取仓库清单，展开 `project.yaml`，选择对应环境的“启动检查”。打开任务链接可查看来源 HEAD、执行节点、HTTP 状态、镜像、脚本和清理结果。运行时可查看“实时启动日志”，完成、失败或取消后展开“启动日志”。节点先脱敏再写入执行目录的 `startup.log`，并在 `run.json` 的 `output` 保存已记录字节数和最近 8 KiB；控制面随执行归档回执。字节数以脱敏文件为准。未采集日志的旧组件及历史记录不补造输出。已完成或失败的检查可再次运行，历史任务保留原结果。

| 工作台 API | 请求和结果 |
| --- | --- |
| `GET /tasks/{id}/startup-checks` | 使用来源 HEAD 检查 ID，按创建时间倒序列出其环境检查任务。 |
| `POST /tasks/{id}/startup-checks` | `{"environment":"local-mock"}` 返回该环境的任务 ID。重跑时增加 `previous_task_id` 指向本次已观察到的终态任务。重复提交返回已有任务。 |

### 从工作台登记仓库

仓库清单中的“归纳入口文档”由配置了该项目 ID 与专用凭据的 `worker management` 领取。控制面在首次派发时固定来源。输入包括已归档入口文件的 Git 原文、包清单名称、已解析的依赖和脚本名。每项至多读取 16 KiB，共 64 KiB，最多 48 项，优先读取入口文档。超过上限时保留完整行并显示截断和省略计数。只读 Codex 返回的每条引文在控制面再次核对。无证据的类别显示“未从本次输入确认”。

Owner 可向 `POST /tasks/{id}/repository-analysis` 发送 `{}` 首次派发，使用 `GET` 回读。失败后发送 `{"previous_attempt":1}` 重试已观察到的失败轮次。原生 Agent 会话与固定来源随成功结果归档。

新组件组合在归纳下方显示“Management 归纳进程”，每三秒更新运行观察。展开可读节点、PID、原生会话、退出及最近的脱敏公开消息。失败后点击“重新归纳”沿用首次保存的来源，旧失败尝试保留。页面重开后先点击“读取仓库清单”，再展开进程历史。未知退出只表示没有收到退出回执，不能保证进程在线。旧组件组合不补造进程记录。

从同一清单的“初始化报告”可以归档当前证据。每版保存当时的归纳状态、输出和来源、仓库身份、项目画像与环境资料的修订及摘要，以及对应固定提交的所有启动检查和回执。包清单与密钥需求来自同一次 HEAD 检查。较早的清单若没有记录密钥需求，报告会标明缺少可核对的声明。版本内容可以在原克隆离线后读取。

| Owner 接口 | 结果 |
| --- | --- |
| `POST /tasks/{id}/initialization-reports` | 以已生成清单的 HEAD 任务 ID 归档快照。有新证据时新增版本，相同证据复用最新版本。 |
| `GET /tasks/{id}/initialization-reports` | 按版本倒序列出归档时间。 |
| `GET /tasks/{id}/initialization-reports/{revision}` | 读取该版本的完整报告，与工作台“下载 JSON”内容一致。 |

启动服务后，Owner 可从工作台的“项目接入”登记仓库。路径支持控制面机器的绝对路径和相对于 `FORGEOS_REPO_ROOT` 的路径。登记后选择“运行 HEAD 检查”进入任务详情。原 Worker 未上线时任务显示待执行；成功后可核对仓库的完整提交号。如果失败，保留原任务，在项目页选择“重新检查”生成下一次检查。

Owner API 的 `GET /projects` 每页返回最多 30 个项目，下一页使用 `?before=<next_before>`。原有脚本登记的仓库和通过 `POST /tasks` 创建的 HEAD 检查也会显示。向 `POST /projects/{id}/check` 提交 `{}` 获取或创建检查；重试失败任务时提交 `{"retry_task_id":1}`，将编号替换为本项目的失败检查。并发重试复用已排队的新检查。

项目更新后，可在成功检查的项目行选择“检查当前 HEAD”，也可以使用同一 API 提交已完成的检查编号。每次检查都可以按独立任务编号查看当时的仓库清单。

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

在浏览器打开控制面地址（默认 `http://127.0.0.1:8080/`），输入启动时配置的 `FORGEOS_OWNER_TOKEN`。令牌保存在此标签页的会话存储中；侧栏的「断开连接」会清除它。登录后默认打开“工作台总览”。四个数字可进入全部任务、运行中任务、待答问题和待验收队列。下方的通知和审批可打开对应的任务、计划修订或项目记录。每组最多展示五条，完整队列从各组右上角进入。侧栏的“任务列表”支持按标题、仓库、任务编号和状态查找。「验收队列」只列等待人工决定的任务。

点开任务可查看 Issue、历次 Worker、Worktree、原生 Session、验证输出及交付记录。在执行记录内展开「查看代码差异」核对候选。验收面板会显示本次执行 ID 和完整 SHA。「通过验收」按项目现有配置决定是否触发 GitHub 交付。「退回修改」需要填写原因，并在同一任务下重新排队。已合并的任务提供 PR 链接和最终合并 SHA。任务详情可直接以 `/#/tasks/11` 一类的地址打开。

修改前端时可运行 `npm --prefix web run dev`，并打开该命令输出的本机地址。`FORGEOS_API_URL` 可指定开发代理所连接的本机控制面，默认是 `http://127.0.0.1:8080`。

## 用真实 GitHub Issue 验收

1. 在控制面运行身份下安装 GitHub CLI，使用 `gh auth status` 确认有目标仓库和 Issue 的读取权限。在 Worker 的专用普通用户下登录原生 Codex CLI，通过 `codex login status` 核实。Windows 可在启动 Worker 前把 `FORGEOS_CODEX_BIN` 设为 `(Get-Command codex.exe).Source`。Worker 启动时探测 CLI，只有声明支持 Codex 的 Worker 才能领取这类任务。

   需要使用 App Server 时，为普通 Worker 设置 `FORGEOS_CODEX_TRANSPORT=app-server`。该选择用于 Codex 实现和只读规划，复用当前用户的原生登录、任务工作区和恢复日志。允许值为 `exec`、`app-server`，默认值为 `exec`。能力探测失败会终止 Worker 启动。当前角色的审批策略为 `never`，收到超出该权限的原生请求会令回合失败。[运行时说明](07-harness-and-worker-runtime.md)
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
| `V1`、`V2`、`V3`、`V4` | 按所选等级运行基础提交中 `validation.yaml` 声明的检查。Worker 使用 `FORGEOS_CI_IMAGE` 指定的本机镜像。任务详情的“逐项检查”提供执行编号、运行目录、日志及报告引用。操作与配置见[验证与本地 CI](10-validation-and-local-ci.md)。 |

按标签自动接入时，在控制面启动前设置 `FORGEOS_POLL_PROJECT_ID` 为已登记项目 ID，`FORGEOS_POLL_VALIDATION` 为上述一种验证方式。Owner 在 GitHub 仓库中为 Agent 任务模板配置 `forgeos-agent` 标签，Poller 只派发带该标签的开放 Issue，逐页读取并与手动导入共用去重逻辑。普通讨论可由 Owner 通过编号显式转为任务。默认采用按编号派单。

要同时轮询多个仓库，设置 `FORGEOS_POLL_PROJECTS_FILE` 指向操作员维护的 JSON 文件。例如以下配置让第一个项目使用文档检查，第二个项目按固定基础提交的策略选择检查。

```json
[
  {"project_id": 1, "validation": "docs"},
  {"project_id": 2, "validation": "auto"}
]
```

文件与单项目环境变量中的相同条目共用一个轮询循环。若同一项目声明不同验证方式，控制面在启动时报告冲突。各项目独立扫描、更新收件箱和重试。轮询配置只授权读取 GitHub；交付目标由后面的[交付配置](#github-交付)单独指定。

启用 Poller 后，用 Owner 身份访问 `GET /projects/{id}/inbox` 查看开放的普通讨论。结果按 Issue 编号倒序排列，每页最多 100 项，下一页传 `?before=上一页最后的编号`。完整扫描结束后刷新收件箱，重复同步不会为普通讨论启动 Worker。通过下表的按编号导入接口转换后，讨论立即从收件箱移出。

### GitHub Webhook

先让项目完成一次轮询或 Issue 导入，建立数字仓库 ID 的绑定。在 `FORGEOS_POLL_PROJECTS_FILE` 中给该项目添加 `"webhook_secret_file":"/etc/forgeos/beta-webhook.key"`，文件内容为该仓库 Webhook 的密钥，可有一个结尾换行。限制文件读取权限，重启控制面后生效。仓库的 Webhook 使用 JSON，订阅 Issues、Issue comments；把其请求送到 `POST /hooks/github/{项目号}`。若有独立 HTTPS 接收代理，仅转发所配置的 Webhook 路径，原样传递正文及 GitHub 头。私有回环环境可使用 GitHub CLI 的 Webhook 转发功能实测。[GitHub 官方转发说明](https://docs.github.com/en/webhooks/testing-and-troubleshooting-webhooks/using-the-github-cli-to-forward-webhooks-for-testing)

首次有效投递返回 202；相同 delivery ID、事件名和原文的重投返回 200。签名无效返回 401，数字仓库 ID 错配返回 403，同一投递号对应不同内容返回 409。项目尚未绑定仓库返回 503，可在绑定后重投。投递使用原始请求字节校验 HMAC-SHA256，控制面支持至多 25 MiB 的正文。[GitHub 签名说明](https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries)

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

在隔离测试库准备一个已有未激活草案的 Goal，并设置 `FORGEOS_OWNER_TOKEN`。运行 `python scripts/probe_plan_activation.py http://127.0.0.1:8080 <目标任务号>`。探针会发布内容相同的下一版草案，验证旧版批准被拒绝，再从 Chrome 批准新版，模拟批准响应丢失并核对子项。

交付器在前置子项全部完成 GitHub 回写后检查目标分支。它获取分支提交到项目对象库，核对原始基础及每个前置 Merge SHA 都在该提交的历史中，再把后续子项的基础版本固定并转为 READY。可以从子项的 `spec.base_commit` 和 `dependencies_delivered` 事件复核派发依据。

待所有里程碑已有详细工作项、每个子任务和交付记录均为 DONE 后，交付器核对原仓库和父 Issue 身份，回写一次 Goal 摘要并关闭 Issue。远端确认后才把父 Goal 标为 DONE。待细化的空里程碑会保留父 Issue。

## 验收检查点的业务问题

在候选完成验证、需要业务澄清时，Owner 可向 `POST /tasks/{id}/questions` 提交 `execution_id`、`commit_sha`、`request_key`、`question` 和 `evidence`。控制面将任务置为 WAITING_INPUT，暂停对此 SHA 的验收。按「等待回答」筛选任务可定位问题。维护者在详情页回答，或向 `POST /tasks/{id}/questions/{question}/answer` 提交 `{"answer":"业务答复"}`。任务回到同一执行、会话及候选的验收检查点，历史问题和答复保存在 `questions` 中。

## GitHub 交付

多个项目共用控制面时，将 `FORGEOS_DELIVERY_PROJECTS_FILE` 指向控制面操作员维护的 JSON 文件。每一行配置对应登记接口返回的项目 ID。以下配置让项目 1 交付到 main，项目 2 交付到 release/internal。

```json
[
  {"project_id": 1, "target_branch": "main"},
  {"project_id": 2, "target_branch": "release/internal"}
]
```

省略某个条目的 `target_branch` 会使用该仓库的默认分支。文件配置和原单项目环境变量可同时使用；同一项目若声明不同目标，控制面会在启动时报告冲突。重新启动后，新批准的候选采用新配置。所有已启用项目仍使用控制面进程的 GitHub 身份。

默认情况下，通过人工验收只将任务记为 `ACCEPTED`。要让某个项目自动发布，在**批准该候选前**配置控制面的 `FORGEOS_DELIVERY_PROJECT_ID`，使用有权向此仓库推送、管理 PR 和更新 Issue 的 GitHub 身份，再重启控制面。可选 `FORGEOS_DELIVERY_TARGET_BRANCH` 指定目标分支，留空时采用 GitHub 仓库默认分支。使用普通受保护分支权限的凭据。审批发生时会同时保存交付任务，之后更改配置不会把之前的本地验收追溯发布。

交付器根据已验收 SHA 推送任务分支、创建或找回对应 PR，并核对远端 PR 的仓库、目标分支和 head。门禁尚未通过时，`delivery.detail` 说明等待原因；维护者可在 GitHub 完成所需 Review 或检查。合并使用冻结的 head SHA 和 Squash 策略。回写结果并关闭对应简单 Issue 后，任务进入 `DONE`。`delivery.pr_url` 和 `delivery.merge_sha` 可用于核对远端结果。服务重启或外部请求超时后，会先查询已有 PR、合并和反馈，再决定下一步操作。

如果某项 CI 必须通过才能合并，请在 GitHub 目标分支的保护规则中把对应的检查名列为必需，并让规则适用于交付账号。GitHub Actions 的工作流可能在 PR 创建后才上报检查。仅配置 `pull_request` 工作流时，GitHub 在首份状态上报前可能允许合并。本次私有仓库验收将 `example` 设为来自 GitHub Actions 的必需检查，开启严格模式并将规则应用到管理员。

## API 与故障恢复

| API | 身份 | 用法 |
| --- | --- | --- |
| `POST /projects` | Owner | 请求体 `{"path":"example"}`。路径可相对于授权根目录，同一实际仓库重复登记返回相同 ID。 |
| `POST /tasks` | Owner | 请求体 `{"project_id":1,"kind":"git_head"}`。返回新工作项，初始状态为 `READY`。 |
| `GET /tasks` | Owner | 按 ID 倒序返回任务摘要和各状态总数。可选 `q`、`status`、`limit`（默认 30，最多 100）及 `before`。下一页使用响应的 `next_before`。 |
| `POST /projects/{id}/issues/{number}` | Owner | 请求体例如 `{"validation":"V2"}`，支持 docs、go-test 或 V1–V4。读取该仓库的开放 Issue 并派单。 |
| `GET /tasks/{id}` | Owner | 查询任务状态、执行历史、结果和事件。连通性任务成功为 `DONE`，Issue 通过本地验证为 `WAITING_ACCEPTANCE`，执行失败为 `FAILED`。 |
| `GET /tasks/{id}/diff` | Owner | 提供 `execution_id` 和完整候选 `commit`，读取该次执行相对冻结基础提交的 Git 差异。展示上限 256 KiB，超出时标记 `truncated`。 |
| `POST /tasks/{id}/acceptance` | Owner | 提交 `execution_id`、完整 `commit`、`decision` 和可选的 `note`。退回必须附说明，批准时按该项目配置决定是否交付 GitHub。 |
| `POST /internal/claim` | Worker | 领取任务，空队列返回 HTTP 204。每个项目同时最多一个运行中的任务。 |
| `POST /internal/executions/{id}/renew` | Worker | 发送该执行的 `lease_token`。生效时返回 204，租约已到期、执行已结束或令牌不符返回 409；超出节点授权返回 403。 |
| `POST /internal/executions/{id}/result` | Worker | 以领取时的租约令牌提交结果。重发相同结果只记一次。 |
| `POST /executions/{id}/pause-ci` | Owner | 暂停已登记候选 SHA 的 CI；回执到达后进入 PAUSED。 |
| `POST /executions/{id}/resume-ci` | Owner | 使用暂停执行的候选 SHA 重新排队复验。对同一执行重放请求仅恢复一次。 |
| `POST /executions/{id}/cancel` | Owner | 请求取消指定运行中的执行。已登记的请求可安全重报。任务详情显示请求时间，Worker 回传结果后才能释放名额。 |
| `POST /executions/{id}/instructions` | Owner | 提交 `{"sequence":0,"key":"request-1","body":"在本页补充已约定的副标题"}`。sequence 取自当前执行的 queue_sequence。同一执行、检查点和请求键的原文可幂等重报，成功返回 204。改写已用键、过时回合或队列已满返回 409。 |
| `POST /executions/{id}/abandon` | Owner | 核对失联执行并确认原 Worker 已停止后，人工结束过期租约。标记失败并释放项目名额，保留工作区与事件。 |
| `POST /executions/{id}/retry` | Owner | 核对失败原因后重试该次 Issue 执行。相同执行 ID 的重试请求幂等，继续沿用原工作项与冻结需求，下一次执行使用新 Worktree。 |

控制面在领取响应的 `lease_seconds` 中给出本次租期，Git HEAD 为 120 秒，Issue 为 1500 秒。正式 Worker 在执行期间每 30 秒确认一次租约，使用节点身份和该执行的令牌。暂时不可达时，Worker 在上一次确认的有效期内重试；确认超时或续约被拒时取消当前操作。控制面按数据库时间核对截止点。Worker 以请求开始的单调时钟计时，不需要与控制面交换本机时间。一般节点心跳只登记在线状态，不延长其他执行。最终结果先写入本地恢复日志再补报。

Worker 失联时，先从任务详情查 `execution.lease_expired`、执行 ID 和工作区。租约过期后中台仍等待原执行回报，避免同一任务执行两次。人工结束后，Issue 使用上表中的重试接口，连通性测试可创建新的 `git_head` 任务。工作区保留用于核对，完成核对后可用 `git -C <仓库> worktree remove <工作区>` 清理。

在没有运行中任务时，可在原 PowerShell 中执行 `Stop-Process -Id $worker.Id,$control.Id` 停止两个进程。再次启动控制面时指向同一数据库即可恢复查询。启动失败时查看 `$binDir/control.log` 和 `$binDir/worker.log`。

## 测试

`go test ./...` 运行 HTTP、GitHub 接入和 Worker 测试。交付测试向临时裸 Git 仓库推送，并模拟 GitHub 合并门禁与响应丢失。设置 `FORGEOS_TEST_DATABASE_URL` 为独立测试库连接串后运行 `go test -count=1 ./...`，还会验证旧数据迁移、派发、重报、失联与重试、验收退回和交付队列恢复。若本地已备好 Docker 引擎和 Go 镜像，设置 `FORGEOS_TEST_DOCKER=1` 同时验证真实 Go 测试通过和失败的情形。

## Management 审核进程

控制面设置 `FORGEOS_MANAGEMENT_PROJECT_ID` 和独立的 `FORGEOS_MANAGEMENT_TOKEN` 后，为指定项目开放审核队列。用普通用户启动 `go run ./cmd/worker management`，给这个进程设置同一 Management 令牌和回环 `FORGEOS_API_URL`。它只需已登录的本地 Codex CLI，无需仓库目录、数据库密码或 Owner、Worker、GitHub 令牌。

管理进程自动领取最新的待规划修订，在单独的只读会话中审阅冻结的 Issue 和草案。任务详情显示建议、判定等级、理由、依据和原生会话 ID。Owner 可通过 `GET /tasks/{id}/plans/{revision}` 查阅历史版本。模型执行限时五分钟，领取租约六分钟；进程退出后可重新领取。

未指定策略时，最低决定等级为 D2。Owner 保存修订时可附加 `minimum_level`，也可在首次领取前向 `POST /tasks/{id}/plans/{revision}/policy` 提交该字段。已指定的等级绑定当前修订。D1 只开放给 docs 验证的低风险范围；管理建议只能提高等级。D1 通过会在同一事务中激活子任务；退回、请求人工决定、D2 或 D3 均保留草案，Owner 可保存下一版或通过现有激活入口明确批准。代码交付仍经过独立的候选验收与 GitHub 门禁。

业务问题审阅期间，在“待答问题”或任务详情点击“刷新”，展开“Management 问题审阅进程”。答复后从原任务回读，进程消息与正式建议、Owner 答复分别保存。Owner 不必等待建议结束；迟到回报不会改写已答问题。旧组件组合继续显示原建议，不补造进程。

共享审核期间，在来源项目的副本页选择“刷新共享记录”，展开“Management 审核进程”；接收项目只读取发布后的副本。计划审核期间，在对应 Goal 修订展开“Management 审核进程”查看节点、PID 和原生 Thread；回合结束后可回读退出码与公开消息。FORGEOS_WORKER_ID 可为审核节点命名，缺省使用主机名；FORGEOS_WORKER_STATE_DIR 指向仓库和工作区之外的私有目录，缺省沿用服务用户的 .forgeos/worker，角色日志文件与普通 Worker 分开。需要跨角色或节点定位时，同时核对节点名、计划修订和原生 Thread，PID 只在对应节点有意义。

## 启动知识提取角色

在控制面设置 `FORGEOS_CURATOR_PROJECT_ID` 和该项目独立的 `FORGEOS_CURATOR_TOKEN`。把 Management 配置到同一项目，用两个单独的进程启动 `go run ./cmd/worker curator` 和 `go run ./cmd/worker management`。每个进程只需自己的角色令牌、回环 `FORGEOS_API_URL` 和能输出结构化结果的已登录本地 Codex。可用 `FORGEOS_CODEX_BIN` 指定可执行文件。

从工作台导入含有实测观察的项目资料，或选择“记住一条项目经验”并提交观察原文。打开已保存修订的“知识提取与审核”。也可在项目决策中确认含实测观察的提案，点开其决策编号查看同一审核流程及确认事件。依次核对原文引句、两个会话和审核决定；项目节点可通过 `GET /internal/projects/{id}/experience` 回读审核通过的当前修订。在“知识提取与审核”展开“Curator 提取进程”或“Management 知识审核进程”，可核对节点、PID、原生会话、退出和最近公开消息；“刷新审核记录”可立即读取。两个角色即便使用相同节点名和状态目录，也分别保存本地日志。引文修复产生的两次原生尝试各自保留；新旧组件组合不补造进程。提取和审核分别使用六分钟租约。原进程中断且没有成功回报时，角色进程会在租约到期后重新领取。

## 为多个 Worker 分配项目

控制面可用 `FORGEOS_WORKERS_FILE` 代替单一 `FORGEOS_WORKER_TOKEN`。文件是 JSON 数组，每项含 `worker_id`、`token_sha256`（原始随机令牌的 SHA-256 十六进制摘要）与 `projects`（允许领取的项目 ID 数组）。在对应 Worker 上设置相同的 `FORGEOS_WORKER_ID`。可用 `FORGEOS_WORKER_TOKEN` 设置固定令牌，或用 `FORGEOS_WORKER_TOKEN_FILE` 指向节点服务账号可读、位于项目和工作区之外的绝对路径。两种方式只选一种。文件内容为一行可打印 ASCII 令牌，允许末尾换行。使用文件时，Worker 每次发起请求都会重新读取。

要在线轮换节点令牌，按以下顺序操作。

1. 在同一节点的授权记录中保留 `token_sha256`，添加新令牌的 `next_token_sha256`，原子替换控制面授权文件。
2. 使用 Owner 身份向控制面的 `POST /workers/reload-credentials` 发送空请求。204 表示整份配置已启用。两个摘要共享该节点的项目和环境授权。配置无效时返回 409，此前生效的配置继续工作。
3. 在节点上原子替换令牌文件，确认新的令牌能访问获准项目，并观察心跳或活动执行续约。
4. 将新摘要移至 `token_sha256`，移除 `next_token_sha256`，再次替换授权文件并由 Owner 重载。核对旧令牌返回 401，新令牌仍可用。

多身份模式下，控制面先限制候选项目再领取任务。节点只能读自己执行的检查点和回报自己的结果，后续请求仍校验执行租约。使用不同的本地仓库根目录与工作区根目录，可进一步限制各节点可解析的 Git 工作区。历史单令牌配置适用于共用信任域。

节点使用密钥状态接口时，在该节点的配置中添加 `"environments":[{"project_id":2,"environment":"local-mock"}]`。所列项目必须已在该节点的 `projects` 数组内。未列出环境的节点仍可领取已有授权项目的普通任务。OpenBao 连接、只读策略与状态接口见[密钥管理](09-security-and-secrets.md)。

### 为具名节点签发和轮换客户端证书

控制面保留回环 Owner 入口。要单独启用节点 HTTPS 入口，在控制面设置 `FORGEOS_NODE_TLS_ADDR`（私网 IP 与端口）、`FORGEOS_NODE_TLS_IDENTITY_FILE`（包含服务端证书链和私钥的 PEM 文件）和 `FORGEOS_NODE_TLS_CA_FILE`（签发客户端证书的 CA）。配置须使用绝对文件路径和 `FORGEOS_WORKERS_FILE`。服务端证书的 SAN 须匹配节点访问的地址。该入口只注册具名 Worker 的路由。

在节点上将 `FORGEOS_API_URL` 指向上述 HTTPS 地址。节点自己的 `FORGEOS_NODE_TLS_CA_FILE` 信任服务端 CA，`FORGEOS_NODE_TLS_IDENTITY_FILE` 包含客户端证书链与私钥。将私钥文件保存在仓库和工作区之外并限制为服务账号可读。仍需配置节点的 bearer 凭据。用 `openssl x509 -in node.crt -noout -fingerprint -sha256` 取得叶证书 DER 的 SHA-256，移除冒号后，将十六进制摘要放入该节点的 `certificate_sha256` 数组。绑定指纹的节点每次请求都须提交已验证且与 bearer 属于同一授权记录的证书。

1. 在已有指纹旁添加新证书的指纹。原子替换授权文件，从 Owner 回环入口调用 `POST /workers/reload-credentials`，核对返回 204。
2. 用新的客户端证书链和私钥原子替换节点的 PEM 文件。Worker 的下一次请求会重新握手并读取新文件。核对相同节点已用新证书成功领取任务或回报结果。
3. 删除旧指纹并再次由 Owner 重载。保留的旧连接和使用旧证书重连的请求均应返回 403。要完全撤销节点访问，删除整条节点授权记录。服务端身份和信任根文件在控制面启动时加载。

## 查看反馈分类进程

从任务的预览反馈入口提交观察，点击“刷新反馈分类”，展开对应反馈下的“Management 分类进程”。可核对节点、PID、原生版本和会话、最近上报、退出及最近 8 KiB 脱敏公开消息。回合失败时不会显示分类结论；原租约自然到期后可由 Management 重领，页面保留两次尝试。候选是否通过仍由原验收流程决定。控制面重启后从相同任务、预览和反馈回读；旧组件组合不补造进程。

## 查看候选测试计划进程

在待验收候选的“查看测试计划”中生成计划，展开“Test Agent 计划进程”和“Management 覆盖审核进程”，核对原生版本、节点、PID、会话、实际退出及最近 8 KiB 脱敏公开消息。待完成记录每八秒刷新；任务“刷新”可立即重读。使用上一版／下一版按钮回查被替代和失败修订。两个角色即便共用节点名及状态目录也分别保存 SQLite；只给各进程自己的角色令牌。生成或覆盖审核失败后可请求新修订。覆盖通过仍须执行用例并记录证据。

## 查看独立 Reviewer 进程

在候选执行中请求独立审查后打开“查看审查记录”，展开“Reviewer 审查进程”。待完成记录每八秒刷新，也可从任务“刷新”立即重读。核对节点、PID、Claude 版本、Session、最近上报、实际退出及最近 8 KiB 脱敏公开消息。没有公开文本的回合不补造消息。异常退出后使用“重试审查”，同一候选下可查看两次进程；成功报告仍需按原门禁处理发现与 Owner 验收。FORGEOS_WORKER_STATE_DIR 中的 reviewer 角色日志与普通开发节点分开。

## 恢复独立角色的待发结果

重启 Management、Curator、Test Agent 或 Reviewer 时，保持原 FORGEOS_API_URL、FORGEOS_WORKER_ID 和 FORGEOS_WORKER_STATE_DIR，并配置该角色当前有效的凭据。节点先补报本身份已写入 SQLite 的结果，再领取作业。凭据错误时不要删除日志；修复角色令牌后重启。控制面接收的重复结果保持幂等，过时结果按 409 冲突结束，不覆盖新修订。需要排查时，在私有角色 SQLite 的 role_receipts 查看 path、buffered_at、settled_at、status；不要把待发正文中的租约复制到公开日志。

这个入口仅恢复已持久化的最终结果。运行中的模型会话中断及尚未记录的领取仍按原租约处理，不能宣称会自动接回原会话。
