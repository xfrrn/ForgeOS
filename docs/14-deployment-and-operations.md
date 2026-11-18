# 14 — 部署与运行

## 1. 推荐部署形态

按[总体实施路线](15-mvp-roadmap.md)，基座联调只需启动控制面、PostgreSQL 和 Worker Daemon。后续接入单项目交付、私网预览和密钥管理时，可在同一台自有 Linux 机器上逐步部署以下组件：
- Go Control Plane；
- PostgreSQL；
- OpenBao；
- Worker Daemon；
- Local CI Runner；
- Preview Manager；
- Caddy；
- Tailscale。

Control Plane 与 Worker Daemon 是独立进程，但可以同机部署。

## 2. Linux 用户

Worker Daemon 使用已登录 Codex/Claude 的专用非 root Linux 用户运行。

该用户：
- 分别登录 Codex 和 Claude Code 一次；
- 后续执行复用现有认证状态；
- 认证目录严格权限保护；
- 不复制认证文件到项目或 Worktree；
- 不记录认证信息。

systemd 必须显式配置 HOME、PATH、工作目录、代理环境和必要变量，不假设继承交互式 Shell 环境。

### 安装 systemd 服务

在项目根目录构建 Linux 二进制。以下安装命令使用 root，部署前准备可连接的 PostgreSQL。控制面和 Worker 分别使用独立账号，只有项目克隆使用共同的 `forgeos-repos` 组。启用反馈录屏归档时，在控制面服务的 PATH 中提供 FFmpeg，并使其临时目录可写。原文件由 Owner 确认提交后，控制面提取视频轨并写入附件存储。

```sh
install -d /usr/local/libexec/forgeos /etc/forgeos
go build -o /usr/local/libexec/forgeos/controlplane ./cmd/controlplane
go build -o /usr/local/libexec/forgeos/worker ./cmd/worker
groupadd --system forgeos-repos
for account in forgeos-control forgeos-worker; do
  useradd --system --user-group --groups forgeos-repos --no-create-home \
    --home-dir "/var/lib/$account" --shell /usr/sbin/nologin "$account"
done
install -m 0644 deploy/systemd/forgeos-*.service /etc/systemd/system/
```

为每个批准访问的项目准备由控制面账号持有、`forgeos-repos` 组可读写的克隆。可在设置了组继承权限（`2770`）的目录中使用 `git init --shared=group` 后抓取项目。Worker 账号的环境文件中，`FORGEOS_SHARED_REPOS` 填入共享克隆的实际绝对路径，在 Linux 上用冒号分隔。只列出信任其 Git 管理目录的项目。仓库仍须位于 `FORGEOS_REPO_ROOT` 内。

使用不同挂载点时，控制面、执行节点和审查节点分别设置本机的 `FORGEOS_REPO_ROOT`。例如控制面的 `/mnt/projects/team/beta` 和节点的 `D:\projects\team\beta` 对应同一相对路径 `team/beta`。可以把同一个 Git 对象库挂载到不同位置。部署独立镜像时，需要提前同步固定版本，以便执行节点读取基础 SHA，控制面在确认结果时、审查节点在领取审查时读取候选 SHA。仅配置路径不会同步 Git 对象。

分别创建 root 持有、权限为 `0600` 的环境文件。每行写入一项 `名称=值`，systemd 在切换到服务账号前读取文件。

| 文件 | 必填设置 |
| --- | --- |
| `/etc/forgeos/controlplane.env` | `FORGEOS_LISTEN_ADDR=127.0.0.1:8080`、`FORGEOS_DATABASE_URL`、`FORGEOS_OWNER_TOKEN`、`FORGEOS_WORKERS_FILE`、`FORGEOS_REPO_ROOT` |
| `/etc/forgeos/worker.env` | `FORGEOS_API_URL=http://127.0.0.1:8080`、节点的 `FORGEOS_WORKER_ID` 和 `FORGEOS_WORKER_TOKEN` 或 `FORGEOS_WORKER_TOKEN_FILE`（[在线轮换步骤](local-development.md#为多个-worker-分配项目)）、`FORGEOS_REPO_ROOT`、`FORGEOS_SHARED_REPOS`、`FORGEOS_WORKSPACE_ROOT=/var/lib/forgeos-worker/work` |

多节点部署可在私网绑定独立的 TLS 1.3 节点端口，客户端证书与具名节点逐一绑定。服务身份文件、私有 CA 与在线轮换步骤见[具名节点证书](local-development.md#为具名节点签发和轮换客户端证书)。

节点授权文件采用[本地运行说明](local-development.md)中的 JSON 格式，由控制面用户独占读取。需要外网代理时，在使用代理的服务环境文件中显式配置 `HTTP_PROXY`、`HTTPS_PROXY`。两个 unit 均设置 `NO_PROXY=localhost,127.0.0.1,::1`。服务首次启动时由 systemd 创建权限为 `0700` 的独立 HOME。模型登录须在 Worker 自己的 HOME 下完成。

执行 `systemctl daemon-reload` 和 `systemctl enable --now forgeos-controlplane forgeos-worker`。用 `systemctl status` 检查两项服务，随后通过 Owner API 登记仓库并创建 `git_head` 任务。任务应返回该仓库 HEAD。重启两个服务并再次查询，原执行的工作区、退出码和提交应与重启前一致。

服务账号可以使用 `nologin` 登录 Shell。运维人员可在终端以目标身份直接执行 CLI，核对专用 HOME 的登录状态。下面的路径对应安装在 Worker 自己的 `.local/bin` 下的两个 CLI。

```bash
sudo -H -u forgeos-worker env PATH=/var/lib/forgeos-worker/.local/bin:/usr/local/bin:/usr/bin:/bin codex login status
sudo -H -u forgeos-worker env PATH=/var/lib/forgeos-worker/.local/bin:/usr/local/bin:/usr/bin:/bin claude auth status --text
```

执行节点页面显示的是 CLI 版本和已探测到的操作。验证账号可用性还要在同一身份下完成登录，并通过 Worker 提交一次真实模型请求。

## 3. 私有网络

第一版建议 Tailscale：
- 执行机器和远程电脑加入同一私有网络；
- 只开放中台 Web 和预览入口；
- 数据库、OpenBao 管理接口、Docker API、Worker 内部 API 默认不向远程普通客户端开放；
- 网络 ACL 与应用身份认证同时使用；
- 不默认启用 Funnel 公网暴露。

### 独立 Staging 回调

在独立的 Staging 主机或容器网络上运行[回调网关配置](../deploy/caddy/Staging.Caddyfile)。入口地址为 `https://<Staging 域名>/callback`。网关只将该域名的 GET 和 POST 送到一个 Staging 应用处理器。原查询、正文和提供方签名头会转交给应用。其他域名、路径及方法返回 404。OAuth 状态、Webhook 签名和重放检查由接收应用按提供方协议处理。

| Staging 进程变量 | 设置 |
| --- | --- |
| `FORGEOS_STAGING_HOST`、`FORGEOS_STAGING_BIND`、`FORGEOS_STAGING_PORT` | 回调域名、该入口自己的绑定地址和端口。默认监听 127.0.0.1:8443；提供方使用实际可达的 HTTPS 地址。 |
| `FORGEOS_STAGING_TLS_CERT`、`FORGEOS_STAGING_TLS_KEY` | Staging 域名的证书链和私钥。 |
| `FORGEOS_STAGING_UPSTREAM`、`FORGEOS_STAGING_APP_PATH` | Staging 网络中的接收服务，例如 `receiver:8080`；处理路径默认 `/callback`，可设为应用实际路径，例如 `/provider/hook`。 |

Staging 接收器使用项目在 Staging 环境中的实际凭据。网关和接收器不加入工作台、Worker 或私网预览所在网络。在 Staging 服务环境中运行：

```sh
caddy validate --config deploy/caddy/Staging.Caddyfile --adapter caddyfile
caddy run --config deploy/caddy/Staging.Caddyfile --adapter caddyfile
```

在提供方登记 HTTPS 回调地址并发送一次测试事件，用提供方的投递 ID 对照接收端结果。验收时还需访问同一域名的 `/ui/`、`/tasks/1`、`/internal/claim`，核对三者均返回 404，并确认接收服务没有收到这些请求。

## 4. 反向代理

Caddy 可统一代理中台和预览域名。预览域名由 Preview Manager 动态登记，路由到对应本地实例。

### 私网 HTTPS 入口

使用仓库的 [Caddyfile](../deploy/caddy/Caddyfile)。为工作台与预览分配一个专用私有 DNS 区，例如 `qa.internal.example`。在私有解析器将 `*.qa.internal.example` 指向验收主机的 tailnet 地址，在 tailnet 中为该区配置分流 DNS。工作台域名为 `forgeos.qa.internal.example`。工作台域名上的 `/global-rules` 允许持客户端证书及 Owner 令牌的用户读取历史、发布及撤下全局规则。每次启动预览后，工作台会给出该候选独有的 HTTPS 链接。预览停止或到期后原链接失效。

| 设置 | 配置位置和用途 |
| --- | --- |
| `FORGEOS_GATEWAY_DOMAIN`、`FORGEOS_GATEWAY_PORT` | Caddy 和控制面使用相同的私有 DNS 区和端口。填写小写 DNS 名，端口默认为 443。 |
| `FORGEOS_GATEWAY_BIND` | Caddy 绑定本机的 tailnet 地址。本机联调可用默认值 127.0.0.1。 |
| `FORGEOS_TLS_CERT`、`FORGEOS_TLS_KEY`、`FORGEOS_CLIENT_CA` | Caddy 可读取的 PEM 文件路径。服务端证书覆盖该区的通配域名。客户端 CA 用来核对准入设备的客户端证书。 |
| `FORGEOS_CONTROL_UPSTREAM`、`FORGEOS_PREVIEW_UPSTREAM`、`FORGEOS_PREVIEW_PORT` | 同机默认分别为 127.0.0.1:8080、127.0.0.1:8091 和 8091。预览端口应对应控制面的 `FORGEOS_PREVIEW_ADDR`。 |

在 Caddy 的服务环境中设置上述变量后，运行 `caddy validate --config deploy/caddy/Caddyfile --adapter caddyfile`，用同一环境运行 `caddy run --config deploy/caddy/Caddyfile --adapter caddyfile`。在验收设备安装受信任的服务端 CA 和该设备自己的客户端证书、私钥。tailnet 访问规则将验收人员对该主机的访问限制到网关端口。登录工作台仍需 Owner 令牌。

工作台仅代理静态 UI 和任务、项目、执行操作的路径。私网预览从另一浏览器来源读取，Caddy 转发到预览监听器时移除 Authorization 和 Cookie。控制面、预览上游及数据库等服务保持原来的本机监听配置。[Caddy 客户端证书配置](https://caddyserver.com/docs/caddyfile/directives/tls) · [反向代理的上游请求头](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy)

## 5. 资源管理

- 任务并发由 Orchestrator 控制；
- CI 与 Agent 构建分别计入资源；
- 等待人工的任务释放活动模型执行额度；
- 空闲进程仍可能占用内存；
- 资源不足时排队，不盲目启动更多 CLI。

执行 Worker 可用 `FORGEOS_WORKER_SLOTS` 设置本节点愿意同时运行的普通工作项数，默认 1。按本机可用内存、CPU 和模型账号额度配置，在节点的服务环境中修改并重启 Worker。单个 Worker 可在不同项目并行执行；同项目继续遵守一个写入工作的限制。控制面从 PostgreSQL 中统计未完成、未被人工结束的执行。下调容量会阻止新增领取，直到占用低于新值。

Owner 可在“执行节点”核对已占名额与声明容量。登记节点心跳超时后暂停给该身份派发新任务。旧版未登记节点可按单名额领取。租约过期保留原执行与名额；核实原工作和副作用后，Owner 可结束失联执行。

在普通执行 Worker 的服务环境中，`FORGEOS_WORKER_MODEL_SLOTS` 限制并行的模型回合，`FORGEOS_WORKER_CI_SLOTS` 限制并行的候选验证。两个参数均为正整数，分别默认等于 `FORGEOS_WORKER_SLOTS`。例如总名额为 2、模型与 CI 各为 1 时，两个项目可以分别运行模型和 CI；同时需要同一类资源时，其中一个在启动对应程序前等待。页面及 Owner 接口 `GET /workers` 显示这两类占用和上限。

模型名额覆盖实现、退回修订和只读规划，检查点之间可以释放。CI 名额覆盖内置 docs、go-test、项目声明的检查，以及刷新候选后的单独验证。控制面按节点身份串行分配，在原执行租约内处理同一步骤的重报；任务等待名额期间继续续约，并沿用执行超时。升级前尚在运行的 Issue 执行无法报告阶段，结算前会同时计入两类预算。控制面重启可恢复占用。独立代码审查的模型名额见下文。独立调用的 Runner 和管理等角色的预算仍待验收。

### 多个执行节点共用一个模型账号

Owner 可以为同一个 Harness 账号设置共享并发上限。以 Owner 身份向 `PUT /workers/model-budgets` 发送完整的配置数组。例如，下面两个节点共用一个 Codex 名额，它们的 Claude 和 CI 仍使用各自的配置。节点身份以部署时分配的 Worker ID 为准，账号名称由维护者指定，无需提供模型登录凭据。

```json
[{"name":"team-codex","harness":"codex","slots":1,"workers":["build-a","build-b"]}]
```

`GET /workers/model-budgets` 和工作台“执行节点”显示账号、Harness、成员节点、上限和当前占用。节点同时满足本机名额与共享名额后才启动模型。其他节点在原执行租约中等待并重试。可以为 Claude 配置另一组名额，或按账号拆分节点。同一节点的同一 Harness 只能出现在一组里；重复配置会返回 409，保留此前的完整配置。传空数组可撤下配置。

开始配置时已在执行的模型也计入占用。调整节点归属时，已分配的回合仍占原账号名额，下一回合按新配置申请；下调上限时，已有执行可以结束。失联超时的占用保留到执行结算或 Owner 核实并结束。旧节点升级到支持阶段资源协商的 Worker 后，才能领取已配置共享预算的模型工作。

独立代码审查使用 Claude Code。要让某个项目的 Reviewer 与执行节点共用账号，请在这个 Claude 预算中列出 `reviewer_projects`。例如，下面的配置表示节点 build-a、build-b 和项目 7 的独立 Reviewer 合计使用一个名额。

```json
[{"name":"team-claude","harness":"claude","slots":1,"workers":["build-a","build-b"],"reviewer_projects":[7]}]
```

项目编号来自控制面对 Reviewer 身份的授权。工作台显示该项目及共享占用。没有可用名额时，候选继续等待；控制面有名额才签发六分钟的审查租约。报告、失败回执或租约到期都会释放本次审查占用。普通开发执行的失联处置仍沿用上一段规则。为已有审查租约首次配置预算时，有效的在途审查也纳入计数。

Test Agent 生成候选测试计划和 Management 审核该计划的覆盖率都调用 Codex。Owner 在同一预算的 `test_plan_roles` 中按授权项目分别配置 `test` 和 `management`。例如，以下配置给项目 7 的两个阶段及执行节点共用一个名额。

```json
[{"name":"team-codex","harness":"codex","slots":1,"workers":["build-a"],"test_plan_roles":[{"project_id":7,"role":"test"},{"project_id":7,"role":"management"}]}]
```

角色在取得名额后领取六分钟租约；工作台“执行节点”显示 Test Agent、覆盖审核及其共享占用。Owner 在旧回合运行时修订测试计划，新版等待旧回合交回结果或租约到期。旧回合收到 409 后释放名额，旧结果不能推进新版。此配置用于候选测试计划的两种任务。其他 Management 任务的预算另记。

知识提取和知识审核也使用 Codex。在同一预算的 `agent_roles` 中分别加入项目的 `curator` 和 `curation_review`，可与执行节点、候选测试角色共用名额。

```json
[{"name":"knowledge","harness":"codex","slots":1,"agent_roles":[{"project_id":7,"role":"curator"},{"project_id":7,"role":"curation_review"}]}]
```

每次提取或审核分别领取六分钟租约。已完成提取的材料在等待审核期间释放名额；审核领取时按当时的配置重新申请。原生回合最长五分钟，也受控制面签发的租约截止时间限制。若回报丢失或角色退出，占用保留至有效回报或租约过期。旧 Worker 可继续处理未配置预算的知识任务。

若同一账号还审核 Goal 的计划草案，在该预算的 `agent_roles` 中加入 `{"project_id":7,"role":"plan_review"}`，按实际项目替换编号。管理 Worker 需要支持计划审核的租约截止时间。名额已满时草案继续等待；控制面重启后仍按库中有效租约计数。Owner 在审核期间发布新版，旧回合的结果返回 409 并归还自己的名额，新版另行领取和审核。

跨项目共享资料的 D3 审核使用来源项目的 `share_review` 名额。将 `{"project_id":7,"role":"share_review"}` 加入相应 Codex 账号的 `agent_roles`。Owner 在审核期间撤销草稿后，接收方看不到该副本；原生审核交回结果或租约到期才释放名额。其他草稿通过审核后仍须 Owner 发布。

仓库归纳任务从固定的仓库清单读取资料。将 `{"project_id":7,"role":"repository_analysis"}` 加入 Codex 预算后，Management Worker 取得名额才开始分析。成功报告或失败回执都会释放名额；中断且没有回报的分析保留占用至租约到期。重新领取时再次检查当前配置。

预览反馈分类使用预览所在项目的预算。将 `{"project_id":7,"role":"feedback_classification"}` 加入 Codex 的 `agent_roles`。名额满时，新观察仍可提交，状态为待审核。Management 的有效分类回报释放名额，Owner 随后可在验收决定中引用对应反馈。

实施检查点或候选验收中的业务问题建议使用项目的 `question_advice` 名额。如果 Owner 在建议生成期间答复，或问题已被撤回，在途回合仍占用名额。它交回旧问题的结果时返回 409 并释放自己的名额。后续问题独立领取。D2 建议保留来源和证据，正式答复由 Owner 提交。

### 测量队列与事件读取

在恢复出的隔离数据库上启动正式控制面和 Worker，将 Worker 指向验收克隆。选择一个已有事件的终态任务，并取要参与派发的项目 ID。下面的命令会真实创建 27 个 Git HEAD 任务。Owner 身份从环境变量 `FORGEOS_OWNER_TOKEN` 读取。

```powershell
python scripts/probe_queue_performance.py --url http://127.0.0.1:8080 --detail-task 20 --projects 1 2 4 8 9 --jobs 27 --clients 4 --samples 50
```

探针先测任务列表和含事件的任务详情，分项目入队，等待实际 Worker 交回 Git 结果，再复测相同的读取。HTTP 耗时从发送请求到读完响应。排队和执行耗时分别取数据库的创建、领取和完成事件时间差。输出使用最近秩法计算百分位；样本数不足 100 时，P99 即观测到的最大值。仅用同一快照、并发数和 Worker 配置比较两次运行。[实际测量](acceptance.md#queue-performance)

控制面在 Go 数据库连接池中最多保留 8 个空闲 PostgreSQL 连接。工作台刷新与节点轮询同时到达时可复用这些连接。按控制面进程数评估数据库会话占用；此值只控制闲置连接的保留数量。

## 6. 备份

需要备份：
- PostgreSQL；
- OpenBao 数据和必要解封材料（解封材料离线管理）；
- 项目中台配置；
- Artifact 元数据；
- Worker 恢复记录。

定期进行恢复演练。

### OpenBao 单节点部署与恢复

仓库提供 [Compose 配置](../deploy/openbao/compose.yaml) 和 [服务配置](../deploy/openbao/server.hcl)。镜像按 2.7.2 的摘要固定，进程以镜像自带的非 root 用户运行。宿主端口仅绑定回环，Raft 与审计分别使用独立卷。服务端以 TLS 接受连接。审计设备由 HCL 配置并使用 HMAC 记录敏感字段。[官方镜像](https://openbao.org/docs/install/) · [声明式审计配置](https://openbao.org/docs/configuration/audit/)

1. 准备 `server.crt` 和 `server.key`，证书需包含客户端实际访问的地址。把 `FORGEOS_BAO_TLS_DIR` 设置为该目录的绝对路径，并只向服务用户授予私钥读权限。在项目根目录执行 `docker compose -p forgeos-bao -f deploy/openbao/compose.yaml up -d`。
2. 在管理终端设置 `BAO_ADDR=https://127.0.0.1:8200`、`BAO_CACERT=<签发 CA 证书路径>`。使用 `bao operator init -key-shares=3 -key-threshold=2` 初始化，份额分交保管人离线保管。两名持有人分别在提示符下执行 `bao operator unseal`。初始化时的 Root Token 仅用于引导独立的运行身份和底层策略，完成后撤销或离线封存。
3. 首次初始化后用 `bao audit list` 检查 `forgeos-file/`，用 `bao secrets enable -path=secret kv-v2` 安装版本化 KV。为业务身份只授予所需的项目、环境和操作；备份身份只需对 `sys/storage/raft/snapshot` 具有 read、sudo。使用独立登录角色签发限时凭据。验收中的限时 orphan token 在引导 Root Token 撤销后仍按其原策略工作。
4. 在权限为 0700 的临时加密卷或 tmpfs 中，使用备份身份执行 `bao operator raft snapshot save raft.snap`。单独归档文件审计设备的已关闭日志段。使用离线恢复方的**公钥证书**分别加密两个文件；下面以 OpenSSL 3 为例。备份仓库只接收两个 CMS 文件及其校验值。恢复私钥与解封份额由恢复方分开保管。

```sh
set -eu
umask 077
openssl cms -encrypt -binary -outform DER -aes-256-gcm -in raft.snap -out raft.cms "$RECOVERY_CERT"
openssl cms -encrypt -binary -outform DER -aes-256-gcm -in audit.jsonl -out audit.cms "$RECOVERY_CERT"
sha256sum raft.cms audit.cms > SHA256SUMS
```

5. 恢复演练使用独立的 Compose 项目名、空数据卷和不同的 `FORGEOS_BAO_PORT`。初始化并解封这一恢复目标。核对校验值，用恢复私钥解密出快照和审计文件。将管理终端的 `BAO_ADDR` 指向恢复目标并核对地址，在该空实例执行 `bao operator raft snapshot restore -force raft.snap`。该操作覆盖目标的现有状态。恢复后的节点用**源实例**的份额解封。用业务身份验证旧版本、当前版本和跨项目、跨环境的拒绝结果，另核对解密后的审计文件。生产切换前复核快照时间点之后的令牌撤销和凭据轮换记录。[Raft 快照命令](https://openbao.org/docs/commands/operator/raft/)

```sh
sha256sum -c SHA256SUMS
openssl cms -decrypt -binary -inform DER -in raft.cms -recip "$RECOVERY_CERT" -inkey "$RECOVERY_KEY" -out raft.snap
openssl cms -decrypt -binary -inform DER -in audit.cms -recip "$RECOVERY_CERT" -inkey "$RECOVERY_KEY" -out audit.jsonl
```

### PostgreSQL 恢复演练

使用与服务端主版本匹配的 PostgreSQL 客户端。以下示例使用 libpq 的两个 service 配置，`forgeos-source` 指向应用数据库，`forgeos-restore` 指向隔离实例中的空数据库。连接密码通过权限为 0600 的 passfile 提供。备份含项目内容、内部事件和执行租约，保存在仅备份账号可读的目录中。

```sh
set -eu
umask 077
backup_dir=$(mktemp -d "$BACKUP_ROOT/forgeos-XXXXXXXX")
PGSERVICE=forgeos-source pg_dump --format=custom --file="$backup_dir/forgeos.dump"
pg_restore --list "$backup_dir/forgeos.dump" >/dev/null
(cd "$backup_dir" && sha256sum forgeos.dump > SHA256SUMS)
```

把备份与校验文件传至恢复实例，在备份目录执行以下命令。先独立配置目标实例的数据库用户和所有者，再使用应用数据库所有者恢复。

```sh
set -eu
sha256sum -c SHA256SUMS
pg_restore --dbname='service=forgeos-restore' \
  --single-transaction --no-owner --no-privileges forgeos.dump
```

用恢复库的连接启动独立控制面，演练时关闭 GitHub 轮询和交付。通过 Owner API 对照任务列表、任务详情、计划修订、需求版本、评论和检查点。选择已交付任务核对候选、合并 SHA 和事件，再创建一条 `git_head` 任务确认序列可以继续分配。另行恢复项目 Git 对象与配置、Worker 工作区和认证材料后，才可恢复涉及这些资源的操作。

<a id="随数据库备份和恢复反馈截图"></a>

### 随数据库备份和恢复反馈截图及录屏

在同一个维护窗口备份数据库、反馈 PNG 和 WebM。暂停控制面的写入及工件清理，保留 PostgreSQL 运行。按照上一节创建数据库备份，再从 ForgeOS 仓库根目录执行以下操作。`artifact_root` 指向控制面的工件目录，`backup_dir` 使用刚创建的私有绝对路径。清单根据数据库时间选取仍可读取的文件。过期媒体的原始回执随数据库备份。

```sh
set -eu
umask 077
PGSERVICE=forgeos-source psql -XAtq -v ON_ERROR_STOP=1 -c "SELECT extract(epoch FROM statement_timestamp())" > "$backup_dir/feedback.cutoff"
PGSERVICE=forgeos-source psql -XAtq -v ON_ERROR_STOP=1 -v cutoff="$(cat "$backup_dir/feedback.cutoff")" \
  -f deploy/postgres/feedback-manifest.sql > "$backup_dir/feedback.sha256"
if test -s "$backup_dir/feedback.sha256"; then
  (cd "$artifact_root" && sha256sum -c "$backup_dir/feedback.sha256")
fi
cut -c67- "$backup_dir/feedback.sha256" > "$backup_dir/feedback.paths"
tar -C "$artifact_root" -czf "$backup_dir/feedback.tar.gz" --verbatim-files-from --files-from="$backup_dir/feedback.paths"
(cd "$backup_dir" && sha256sum forgeos.dump feedback.cutoff feedback.sha256 feedback.tar.gz > SHA256SUMS)
```

如果清单为空且工件目录尚未创建，在备份账号的私有临时目录中创建一个空目录作为本次 `artifact_root`。有效媒体缺失或摘要不符时，从可信副本恢复匹配原回执的内容，再重新备份。备份放在受限存储中，将校验清单的副本独立保管。

在隔离实例核对整套备份的 `SHA256SUMS`，再按上一节恢复到空数据库。以恢复控制面的服务账号从 ForgeOS 仓库根目录执行以下命令。恢复清单沿用归档时的截止时间，Owner 读取仍服从当前到期时间。

```sh
set -eu
umask 077
artifact_root=$(mktemp -d "$RESTORE_ROOT/feedback-XXXXXXXX")
if test -f "$backup_dir/feedback.cutoff"; then
  PGSERVICE=forgeos-restore psql -XAtq -v ON_ERROR_STOP=1 -v cutoff="$(cat "$backup_dir/feedback.cutoff")" \
    -f deploy/postgres/feedback-manifest.sql > "$artifact_root/receipts.sha256"
else
  # 旧版备份按全部媒体回执生成清单。
  PGSERVICE=forgeos-restore psql -XAtq -v ON_ERROR_STOP=1 \
    -c "SELECT sha256 || '  ' || storage_key FROM feedback_artifacts ORDER BY storage_key" > "$artifact_root/receipts.sha256"
fi
cmp "$backup_dir/feedback.sha256" "$artifact_root/receipts.sha256"
tar -C "$artifact_root" --no-same-owner --no-same-permissions -xzf "$backup_dir/feedback.tar.gz"
if test -s "$artifact_root/receipts.sha256"; then
  (cd "$artifact_root" && sha256sum -c receipts.sha256)
fi
```

将恢复控制面的 `FORGEOS_ARTIFACT_ROOT` 指向新目录。通过原反馈核对附件编号、类型、字节数和摘要。有效文件可以读取，已到期文件返回 410。只恢复数据库时，有效文件缺失会返回 503。

### 恢复项目克隆和 Worker 工作区

暂停派发，等正在运行的 Harness 和 CI 写入结束，停止相应的 Worker 与控制面。在这一维护窗口备份数据库、登记的项目克隆和对应的 Worker 工作区。Git 的对象、引用与每个关联 Worktree 的索引位于共享的 Git 管理目录，已暂存、未暂存和未跟踪的内容都可能用于恢复。将工作区的 `.git` 文件与它所指向的克隆一起保存。备份包含源码与尚未提交的内容，使用私有存储。

下面的例子中项目根目录为 `/state/repos`，工作区根目录为 `/state/work`。部署时替换为两个服务实际使用的路径，控制面与 Worker 分机时分别在各自保存对应文件。授权的备份账号需要能读取这些目录。快照同时包含截图时，将 `workspaces.tar.gz` 加入同批附件的校验清单。

```sh
set -eu
umask 077
tar -C / -czf "$backup_dir/workspaces.tar.gz" state/repos state/work
(cd "$backup_dir" && sha256sum forgeos.dump workspaces.tar.gz > workspace.SHA256SUMS)
```

准备路径布局相同、两项目录均为空的隔离恢复主机。核对离线保管的清单，恢复数据库及两个目录。以受信任的备份恢复身份还原文件所有权和模式，确保专用服务账号、共享组和 UID/GID 与源部署匹配。使用 ACL 或扩展属性的部署需在备份、恢复时成对启用相应 tar 选项。服务的环境配置和登录凭据按各自的恢复流程配置。

```sh
set -eu
(cd "$backup_dir" && sha256sum -c workspace.SHA256SUMS)
tar --numeric-owner -C / -xzf "$backup_dir/workspaces.tar.gz"
git -C /state/repos/example worktree list --porcelain
git -C /state/work/execution-1 rev-parse HEAD
git -C /state/work/execution-1 status --short
git -C /state/work/execution-1 diff --cached --binary
git -C /state/work/execution-1 diff --binary
```

用服务账号执行 Git 检查。对照源记录核对工作项、执行编号、绝对路径、HEAD、索引、工作区差异和未跟踪文件的内容摘要。确认历史任务可从 Owner API 查询后，再派发一个独立 `git_head` 工作项。新的执行应获得自己的目录。若源执行原本失联，另行核对它的租约、会话和远端已产生的结果，再决定如何恢复。

### 核对恢复的 CI 运行目录

声明式 CI 将日志、报告、产物及 `run.json` 写在 Runner 的工作区根目录下。上节的工作区归档已经包含这些目录。分机部署时，按运行回执的 `runner` 到对应节点保存或恢复文件，同一时间点备份 PostgreSQL。在源端停写后和隔离恢复后分别核对一次。下面从数据库取指定执行的权威回执；也可以从 Owner 的任务详情中选择该次执行的 `result.validation.run`。独立运行 `worker validate-candidate` 时，将其标准输出另存到受限的独立备份介质上，作为对应回执。

```sh
set -eu
umask 077
# 执行号及目录取自本次恢复记录。在源端换用 forgeos-source 和源节点目录。
PGSERVICE=forgeos-restore psql -XAtq -v ON_ERROR_STOP=1 -v execution_id="$execution_id" <<'SQL' > "$backup_dir/verify-run.json"
SELECT result->'validation'->'run' FROM executions WHERE id = :'execution_id'::bigint;
SQL
FORGEOS_WORKSPACE_ROOT="$restored_workspace_root" worker verify-ci-run < "$backup_dir/verify-run.json"
```

核对程序先比对运行目录的 `run.json` 与输入回执，再检查每个登记的日志、报告和产物是否为普通文件、长度及 SHA-256 是否一致。成功时输出运行编号、Runner、执行号、候选 SHA 和已核对的文件数。退出非零时，从独立备份恢复相应文件后重新核对。老版本回执可继续查看；缺少日志摘要时，日志完整性保持待验收。用随运行目录保存的 `run.json` 作为唯一依据无法辨别目录与回执一并被替换的情况。

控制面还保存已上传文件的独立副本。在同一维护窗口，将控制面工件根下的 `ci/` 连同上述数据库、截图和节点运行目录纳入备份及校验清单。恢复时将 `ci/` 放回恢复控制面的工件根，使用 Owner 工作台逐项下载，按恢复数据库的回执复算摘要。中央副本缺失时，仍可从对应节点的运行目录取原文件补传。[下载与补传入口](10-validation-and-local-ci.md#5-local-ci-runner)

若下载返回 503，先在恢复节点的 SQLite 中查询 `SELECT file,bytes,retry_at FROM ci_uploads WHERE run_id = ?`，绑定回执中的运行编号。恢复原节点身份并启动 Worker 后，各文件独立重试。若队列只剩一个文件，按上述回执核对该文件，从可信备份恢复原件并再次运行 `verify-ci-run`。队列清空后从工作台下载复核。登记为零字节的文件也须匹配空文件的 SHA-256。[五个待传文件的恢复记录](acceptance.md#ci-backup-pending)

### 备份和恢复 Worker 的 SQLite 日志

在暂停派发的维护窗口中，先让活动的 Harness 和 CI 写完结果，保留尚未送达或尚未确认的回执。停止源 Worker 后，配套备份控制面数据库、项目克隆和工作区。SQLite 也支持在 Worker 运行时读取一致快照；与 PostgreSQL 配套恢复时，需要记录两个快照之间实际送达了哪些回报。恢复时只有一台 Worker 使用原节点身份。

使用 SQLite CLI。每个控制面地址和 Worker ID 有一份独立的 SQLite 文件，文件名是对“去除末尾斜线的 API 地址、一个零字节、Worker ID”计算的 SHA-256 加 `.sqlite`。从源服务配置核对这两项及状态目录。以下示例备份一个身份；存在多份日志时分别保留各自原始文件名。归档中尚未确认的回执含执行租约，使用仅备份身份可读的私有存储。服务认证材料按原有权限单独配置。

```sh
set -eu
umask 077
# source_journal 是原状态目录下的具体 .sqlite 文件。backup_dir 已存放同批 forgeos.dump。
receipt_dir=$(mktemp -d "$backup_dir/worker-XXXXXXXX")
journal_name=$(basename "$source_journal")
(cd "$receipt_dir" && sqlite3 -bail -readonly "$source_journal" "VACUUM INTO 'worker.sqlite';")
test "$(sqlite3 -bail -readonly "$receipt_dir/worker.sqlite" 'PRAGMA integrity_check;')" = ok
printf '%s\n' "$journal_name" > "$receipt_dir/original-name"
(cd "$receipt_dir" && sha256sum worker.sqlite original-name ../forgeos.dump > SHA256SUMS)
```

将这份校验清单独立保管。隔离演练先恢复配套的 PostgreSQL 数据库，关闭真实交付及轮询，保留原控制面 URL 和 Worker ID。以 Worker 服务账号在私有磁盘创建新的状态目录，位于项目克隆和工作区之外。

```sh
set -eu
umask 077
(cd "$receipt_dir" && sha256sum -c SHA256SUMS)
# api 与 worker_id 按源服务配置填写。先核对原文件名对应当前身份。
journal_name=$(printf '%s\0%s' "${api%/}" "$worker_id" | sha256sum | cut -d' ' -f1).sqlite
test "$(cat "$receipt_dir/original-name")" = "$journal_name"
state_dir=$(mktemp -d "$RESTORE_ROOT/worker-state-XXXXXXXX")
install -m 0600 "$receipt_dir/worker.sqlite" "$state_dir/$journal_name"
test "$(sqlite3 -bail -readonly "$state_dir/$journal_name" 'PRAGMA integrity_check;')" = ok
```

将恢复服务的 `FORGEOS_WORKER_STATE_DIR` 指向 `state_dir`。启动后从 Owner 任务页核对原执行号、工作区、回执与事件，并在新 SQLite 文件中核对 `outcome`、`settled_at` 以及 `report IS NULL`。正常补报和已送达的重复回报都应为 confirmed；Owner 已放弃的执行应保留 conflict 和脱去租约的原结果。核对活动进程、会话时，备份中的 PID 仅是源节点上的历史记录。[本地日志查询](13-data-storage-and-audit.md#worker-执行恢复日志)

判断已离线节点上的工作是否需要重试时，先核对原工作区的 Git remote、HEAD 与节点候选，再到该仓库核对 PR head、目标分支、合并 SHA 和原 Issue 回写。`confirmed` 表示控制面接收了执行回报；是否交付还要看对应仓库的合并和回写。[两节点离线核对实例](acceptance.md#offline-node-deliveries)

## 7. 不属于第一版的内容

- Kubernetes；
- 多租户；
- 对外 Agent API；
- 自动生产部署；
- 复杂多节点调度；
- 默认每任务 Docker 隔离。

### 备份节点进程输出归档

中央副本随 PostgreSQL 备份。普通执行对应 `process_archive_transfers`、`process_archive_chunks`，独立角色对应 `role_archive_transfers`、`role_archive_chunks`。还原数据库后，用 Owner 在工作台下载 JSONL，再与原节点 SQLite 的 `process-logs export` 正文逐字节核对。节点导出另含元数据首行和校验末行，工作台下载的是中间的记录。

如果备份时尚有待传日志，工作台下载返回 409。按上节恢复同一节点的 SQLite、API 地址和节点 ID，启动 Worker 让其补传，确认 `process_log_uploads` 中对应记录已清除。已完成传输的归档可在节点离线时下载。下载返回 422 时校验未通过，应从可信备份恢复；410 表示保留期已到。[两类归档的恢复演练](acceptance.md#central-process-backup)

公开模型消息与工具结果的本地归档随 Worker 私有 SQLite 一同备份，采用同一时点的 SQLite 快照，不能只复制正在写入的单个文件。归档默认每进程 32 MiB、已确认结果保留 30 天；服务环境可设置 FORGEOS_PROCESS_LOG_MAX_MIB 与 FORGEOS_PROCESS_LOG_DAYS。普通 Daemon 启动及每小时清理，独立角色打开日志时清理；离线维护可运行 process-logs prune。

还原后先用 process-logs list 核对实例和原生 Session，再 export 校验正文序号、字节数与摘要。过期正文按当前策略重新清理，原进程、业务回执及摘要保留；旧备份不会永久绕过期限。此流程已从四类真实角色日志副本和非 root Linux CLI 验证。导出文件、原生 Harness 自己的会话记录和恢复工作目录不由本项清理。

<a id="role-recovery-retention"></a>

### 独立角色的恢复资料保留期限

Curator、Management、Test Agent 和 Reviewer 首次领取前检查节点 SQLite，此后在领取时至多每小时扫描一次。`FORGEOS_ROLE_RECOVERY_DAYS` 取 1–3650 天，默认 30。计时覆盖该租约的原生进程退出、进程回报确认、业务结果结算三个时间点。仍在使用的分配、待补报结果和未结束的进程继续占用原恢复目录。

服务账号可对一个角色日志单独执行清理：

```shell
worker prune-role-recovery --journal /var/lib/forgeos/state/<角色日志>.sqlite --days 30
```

返回 JSON 的 expired 为这次完成清理的回合数。每个回合只移除登记在该私有状态目录下的 schema.json、output.json 和空目录；Reviewer 可只有空目录。链接、异常路径和额外文件保留现场，命令返回非零，其他合格回合仍可完成。排查后用相同命令重试。自动扫描遇到异常会记入节点日志，角色继续领取工作。

恢复时将私有角色 SQLite 快照和对应工作目录配套还原到登记路径。再次启动角色或执行上述命令后，当前期限适用于还原的资料。已确认的进程记录、Session、业务结果摘要和清理时间可复核。[操作与验收证据](acceptance.md#role-recovery-retention)
