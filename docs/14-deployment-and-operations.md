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

## 7. 不属于第一版的内容

- Kubernetes；
- 多租户；
- 对外 Agent API；
- 自动生产部署；
- 复杂多节点调度；
- 默认每任务 Docker 隔离。
