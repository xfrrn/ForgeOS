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
