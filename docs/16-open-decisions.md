# 16 — 待验证与待决策事项

本文件防止把架构建议误认为已完成技术验证。

运行基座已用本地 `git_head` 任务验证首次派发、结果重报和控制面重启后的查询。连通性任务租约为两分钟，Codex Issue 任务租约为 25 分钟。两者在租约过期后均保留工作区和同项目运行名额。Owner 可在核对后结束失联执行，失败的 Issue 可在原工作项下用新的 Execution 和 Worktree 重试。原生 Session 恢复仍属 B.4，Linux systemd 环境仍需按 A.4 实测。运行方法见[本地运行指南](local-development.md)。

Windows 本机已验证 GitHub Issue 导入、`codex exec --json` 的原生 Session、Worktree 修改及候选提交的本地验证。真实 PostgreSQL 已验证按 SHA 验收、退回重做和交付记录恢复。私有 GitHub 仓库已实测创建 PR、等待目标分支的必需 Actions 检查、Squash Merge、Issue 反馈与关闭。

## A. 需要真实环境验证

1. Codex App Server 0.169.0-alpha.11.1 已实测 stdio 初始化、只读 Thread、Turn 事件、新进程恢复与中断。审批交互、Worker 自动调度和其他版本兼容性待测。
2. Claude Code 2.1.310 已在 Windows 真实验证 Headless Session Resume、问题答复和权限拒绝。Linux 服务用户和 ForgeOS 任务内的恢复仍待验收。
3. 同一 Linux 用户下多个 CLI 进程共享认证状态时的并发和账号限额行为。
4. Ubuntu systemd 的非 root 身份、HOME、PATH、回环代理例外、工作区与重启已验收。专用用户下的模型登录复用和经代理的真实模型请求待测。
5. 本次实测使用已登录的 GitHub CLI 账号。GitHub App 或细粒度 Token 所需的最小权限，以及要求第二位维护者批准 PR 时的交付行为，仍待实测。
6. 已在私有 beta 仓库验证 GitHub Commit Status 的成功、失败、并发重报和丢失确认后的恢复。将 `forgeos/local-ci` 临时设为必需检查后，真实 PR 已与 Actions 一同通过并合并。使用当前 GitHub CLI 身份，GitHub App 最小授权仍见 A.5。
7. 本机静态预览已验证固定执行、公开目录、独立浏览器来源、到期、停止和重启恢复。动态应用、独立 Staging、Caddy 与 Tailscale 私有 DNS 路由仍待验收。当前缺少外部设备与 tailnet，外部设备项保持待验收。
8. 浏览器扩展在不同前端技术栈、iframe、Canvas 下的元素定位能力。
9. OpenBao 2.7.2 已验证 2/3 解封、KV v2 权限、分开加密的 Raft/审计备份及恢复。Secret Manager 已验证环境状态与身份轮换；Secret Broker 已通过真实 OpenBao 验证固定 KV 版本、运行中节点租约、用途、次数、有效期和一次性解包。
10. PostgreSQL 队列与事件存储在实际任务量下的性能。

## B. 需要进一步细化

1. Agent Profile 的版本和配置结构。
2. Harness Adapter 的正式 Go 接口与错误模型。
3. ACP 内部消息 JSON Schema。
4. Execution/Session/Process 状态机的精确定义。
5. Issue 执行失败重试或验收退回后均使用新 Execution 和新 Worktree。退回修订从上一次被拒的候选开始，原工作区留存。原生 Session 恢复及多 Worker 租约续约仍待细化。
6. `validation.yaml` 版本 1、候选自动选级、隔离 Runner 及检查报告已实测。运行取消、日志和产物读取接口、保留策略仍待补齐。
7. `acceptance.yaml`、全局 Playbook、工作项内 Test Agent 和 Management 的计划派发、覆盖审查、修订归档和逐用例实测门禁已在候选上验证。预览反馈与外部设备访问仍待验收。
8. 单项目交付已在真实仓库核对固定任务分支、PR 编号、验收 SHA、最终合并 SHA 与 Issue 评论。创建 PR、合并、评论和关闭 Issue 后丢失响应的恢复场景由模拟远端覆盖。
9. Artifact 保留和清理策略。
10. 多 Worker 认证和证书轮换。

## C. 产品交互待细化

1. Dashboard 信息架构；
2. Goal 和任务树交互；
3. Agent 实时日志与工具调用展示；
4. 问题批量回答界面；
5. 审批中心；
6. 预览反馈工具；
7. 知识库管理；
8. 项目初始化向导。

## D. 不属于当前范围

- 自动生产部署；
- 生产环境运维；
- 对外租户；
- 公共 Agent 托管平台。
