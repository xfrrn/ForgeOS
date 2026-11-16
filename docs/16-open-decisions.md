# 16 — 待验证与待决策事项

本文件防止把架构建议误认为已完成技术验证。

运行基座已用本地 `git_head` 任务验证首次派发、结果重报和控制面重启后的查询。连通性任务租约为两分钟，Codex Issue 任务租约为 25 分钟。两者在租约过期后均保留工作区和同项目运行名额。Owner 可在核对后结束失联执行，失败的 Issue 可在原工作项下用新的 Execution 和 Worktree 重试。原生 Session 恢复仍属 B.4，Linux systemd 环境仍需按 A.4 实测。运行方法见[本地运行指南](local-development.md)。

Windows 本机已验证 GitHub Issue 导入、`codex exec --json` 的原生 Session、Worktree 修改及候选提交的本地验证。真实 PostgreSQL 已验证按 SHA 验收、退回重做和交付记录恢复。私有 GitHub 仓库已实测创建 PR、等待目标分支的必需 Actions 检查、Squash Merge、Issue 反馈与关闭。

## A. 需要真实环境验证

1. Codex App Server 的目标版本、协议兼容和事件能力。
2. Claude Code 2.1.310 已在 Windows 真实验证 Headless Session Resume、问题答复和权限拒绝。Linux 服务用户和 ForgeOS 任务内的恢复仍待验收。
3. 同一 Linux 用户下多个 CLI 进程共享认证状态时的并发和账号限额行为。
4. Worker Daemon systemd 环境对 CLI 登录缓存、代理和 PATH 的继承。
5. 本次实测使用已登录的 GitHub CLI 账号。GitHub App 或细粒度 Token 所需的最小权限，以及要求第二位维护者批准 PR 时的交付行为，仍待实测。
6. 本地 CI 状态回传 GitHub Checks/Commit Status 的实现方式。
7. Tailscale 私有 DNS、Caddy 和动态 Preview 路由。
8. 浏览器扩展在不同前端技术栈、iframe、Canvas 下的元素定位能力。
9. OpenBao 部署、解封、备份和恢复方案。
10. PostgreSQL 队列与事件存储在实际任务量下的性能。

## B. 需要进一步细化

1. Agent Profile 的版本和配置结构。
2. Harness Adapter 的正式 Go 接口与错误模型。
3. ACP 内部消息 JSON Schema。
4. Execution/Session/Process 状态机的精确定义。
5. Issue 执行失败重试或验收退回后均使用新 Execution 和新 Worktree。退回修订从上一次被拒的候选开始，原工作区留存。原生 Session 恢复及多 Worker 租约续约仍待细化。
6. `validation.yaml` 正式 Schema。
7. `acceptance.yaml` 和 Playbook Schema。
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
