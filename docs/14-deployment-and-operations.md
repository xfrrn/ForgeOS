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

在项目根目录构建 Linux 二进制。以下安装命令使用 root，部署前准备可连接的 PostgreSQL。控制面和 Worker 分别使用独立账号，只有项目克隆使用共同的 `forgeos-repos` 组。

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

分别创建 root 持有、权限为 `0600` 的环境文件。每行写入一项 `名称=值`，systemd 在切换到服务账号前读取文件。

| 文件 | 必填设置 |
| --- | --- |
| `/etc/forgeos/controlplane.env` | `FORGEOS_LISTEN_ADDR=127.0.0.1:8080`、`FORGEOS_DATABASE_URL`、`FORGEOS_OWNER_TOKEN`、`FORGEOS_WORKERS_FILE`、`FORGEOS_REPO_ROOT` |
| `/etc/forgeos/worker.env` | `FORGEOS_API_URL=http://127.0.0.1:8080`、节点的 `FORGEOS_WORKER_ID` 和 `FORGEOS_WORKER_TOKEN`、`FORGEOS_REPO_ROOT`、`FORGEOS_SHARED_REPOS`、`FORGEOS_WORKSPACE_ROOT=/var/lib/forgeos-worker/work` |

节点授权文件采用[本地运行说明](local-development.md)中的 JSON 格式，由控制面用户独占读取。需要外网代理时，在使用代理的服务环境文件中显式配置 `HTTP_PROXY`、`HTTPS_PROXY`。两个 unit 均设置 `NO_PROXY=localhost,127.0.0.1,::1`。服务首次启动时由 systemd 创建权限为 `0700` 的独立 HOME。模型登录须在 Worker 自己的 HOME 下完成。

执行 `systemctl daemon-reload` 和 `systemctl enable --now forgeos-controlplane forgeos-worker`。用 `systemctl status` 检查两项服务，随后通过 Owner API 登记仓库并创建 `git_head` 任务。任务应返回该仓库 HEAD。重启两个服务并再次查询，原执行的工作区、退出码和提交应与重启前一致。

## 3. 私有网络

第一版建议 Tailscale：
- 执行机器和远程电脑加入同一私有网络；
- 只开放中台 Web 和预览入口；
- 数据库、OpenBao 管理接口、Docker API、Worker 内部 API 默认不向远程普通客户端开放；
- 网络 ACL 与应用身份认证同时使用；
- 不默认启用 Funnel 公网暴露。

## 4. 反向代理

Caddy 可统一代理中台和预览域名。预览域名由 Preview Manager 动态登记，路由到对应本地实例。

### 私网 HTTPS 入口

使用仓库的 [Caddyfile](../deploy/caddy/Caddyfile)。为工作台与预览分配一个专用私有 DNS 区，例如 `qa.internal.example`。在私有解析器将 `*.qa.internal.example` 指向验收主机的 tailnet 地址，在 tailnet 中为该区配置分流 DNS。工作台域名为 `forgeos.qa.internal.example`。每次启动预览后，工作台会给出该候选独有的 HTTPS 链接。预览停止或到期后原链接失效。

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

### 随数据库备份和恢复反馈截图

在包含截图的部署上，从同一个维护窗口备份数据库和附件。暂停该控制面的写入及工件清理，保留 PostgreSQL 运行。按照上一节创建数据库备份，在恢复写入前执行以下操作。`artifact_root` 指向该控制面的实际截图目录，`backup_dir` 使用刚创建的私有绝对路径。附件清单取自数据库，归档只包含有回执的图片。

```sh
set -eu
umask 077
receipt_sql="SELECT sha256 || '  ' || storage_key FROM feedback_artifacts ORDER BY storage_key"
PGSERVICE=forgeos-source psql -XAtq -v ON_ERROR_STOP=1 -c "$receipt_sql" > "$backup_dir/feedback.sha256"
if test -s "$backup_dir/feedback.sha256"; then
  (cd "$artifact_root" && sha256sum -c "$backup_dir/feedback.sha256")
fi
cut -c67- "$backup_dir/feedback.sha256" > "$backup_dir/feedback.paths"
tar -C "$artifact_root" -czf "$backup_dir/feedback.tar.gz" --verbatim-files-from --files-from="$backup_dir/feedback.paths"
(cd "$backup_dir" && sha256sum forgeos.dump feedback.sha256 feedback.tar.gz > SHA256SUMS)
```

如果尚无回执且截图目录尚未创建，在备份账号的私有临时目录中创建一个空目录作为本次 `artifact_root`。源图缺失或摘要不符时修复对应回执后重新备份。备份放在受限存储中，将校验清单的副本独立保管。

在隔离实例核对整套备份的 `SHA256SUMS`，再按上一节恢复到空数据库。以恢复控制面的服务账号在私有的 `RESTORE_ROOT` 下新建附件目录，并核对恢复库中的清单。

```sh
set -eu
umask 077
artifact_root=$(mktemp -d "$RESTORE_ROOT/feedback-XXXXXXXX")
receipt_sql="SELECT sha256 || '  ' || storage_key FROM feedback_artifacts ORDER BY storage_key"
PGSERVICE=forgeos-restore psql -XAtq -v ON_ERROR_STOP=1 -c "$receipt_sql" > "$artifact_root/receipts.sha256"
cmp "$backup_dir/feedback.sha256" "$artifact_root/receipts.sha256"
tar -C "$artifact_root" --no-same-owner --no-same-permissions -xzf "$backup_dir/feedback.tar.gz"
if test -s "$artifact_root/receipts.sha256"; then
  (cd "$artifact_root" && sha256sum -c receipts.sha256)
fi
```

将恢复控制面的 `FORGEOS_ARTIFACT_ROOT` 指向新目录。通过 Owner 工作台打开原任务、候选预览及反馈，核对附件编号、图像和摘要。单独恢复数据库时仍可读历史元数据，缺少附件的回读返回 503。

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

## 7. 不属于第一版的内容

- Kubernetes；
- 多租户；
- 对外 Agent API；
- 自动生产部署；
- 复杂多节点调度；
- 默认每任务 Docker 隔离。
