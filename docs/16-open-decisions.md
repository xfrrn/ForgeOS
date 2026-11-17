# 16 — 待验证与待决策事项

本文件防止把架构建议误认为已完成技术验证。

运行基座已用本地 `git_head` 任务验证首次派发、结果重报和控制面重启后的查询。连通性任务租约为两分钟，Codex Issue 任务租约为 25 分钟。两者在租约过期后均保留工作区和同项目运行名额。Owner 可在核对后结束失联执行，失败的 Issue 可在原工作项下用新的 Execution 和 Worktree 重试。执行期间的租约续约与断线结果补报见 B.5；原生 Session 恢复仍属 B.4。Linux systemd 的非 root 部署和未验收的模型登录项见 A.4。运行方法见[本地运行指南](local-development.md)。

Windows 本机已验证 GitHub Issue 导入、`codex exec --json` 的原生 Session、Worktree 修改及候选提交的本地验证。真实 PostgreSQL 已验证按 SHA 验收、退回重做和交付记录恢复。私有 GitHub 仓库已实测创建 PR、等待目标分支的必需 Actions 检查、Squash Merge、Issue 反馈与关闭。

## A. 需要真实环境验证

1. Codex App Server 0.169.0-alpha.11.1 已实测 stdio 初始化、只读 Thread、Turn 事件、新进程恢复与中断。正式 Worker 在 beta #35 使用 CLI 的 `exec resume`，写入和只读规划均在适配器丢失终态后恢复原 Thread 与沙箱。App Server 的审批交互、Worker 接入和其他版本兼容性待测。
2. Claude Code 2.1.310 已在 Windows 真实验证 Headless Session Resume、问题答复和权限拒绝。正式 Worker 已交付私有 beta 的 Issue #22，任务页关联原生会话、版本和候选。beta #29 已从正式 Worker 实测 Claude 适配器丢失终态后退出，使用另一进程恢复原生会话和原工作区并完成交付。Linux 服务身份、原生模型进程被强制终止以及 Worker 自身重启后的会话恢复仍待验收。
3. 同一 Linux 用户下多个 CLI 进程共享认证状态时的并发和账号限额行为。
4. Ubuntu systemd 的非 root 身份、HOME、PATH、回环代理例外、工作区与重启已验收。专用用户下的模型登录复用和经代理的真实模型请求待测。
5. 本次实测使用已登录的 GitHub CLI 账号。GitHub App 或细粒度 Token 所需的最小权限，以及要求第二位维护者批准 PR 时的交付行为，仍待实测。
6. 已在私有 beta 仓库验证 GitHub Commit Status 的成功、失败、并发重报和丢失确认后的恢复。将 `forgeos/local-ci` 临时设为必需检查后，真实 PR 已与 Actions 一同通过并合并。使用当前 GitHub CLI 身份，GitHub App 最小授权仍见 A.5。
7. 本机静态预览已验证隔离、持久化与撤销。已在本机经真实 Caddy 和双向 TLS 验证工作台、独立的候选子域名、Owner 鉴权与停止后回收。动态 local-mock 应用已从工作台启动和停止：固定候选在 Docker 中处理交互，重启保留计数，过期回收，Caddy 网关已验证带客户端证书的同源 POST。独立 Staging Caddy 已实测签名回调和路径隔离。外部提供方实际投递待验收。Tailscale 分流 DNS 和 ACL 需在真实 tailnet 与第二台设备验收，两者当前缺失。
8. 已在 Chrome for Testing 验证顶层 DOM、整页反馈、Canvas 相对位置、Esc 取消、隐私字段排除及错版本阻断。React 19.3.1 和 Vue 3.5.45 的一层及两层同源 iframe 已实测到反馈历史并可重定位。工作台 PNG 标注、敏感区域遮挡、归档和恢复后回读均已实测。扩展采集当前视口、自动隐藏私有区域、丢失响应后重试及上传标注截图已在真实 Chrome 验收。原生标签页录制、候选交互、脱敏、撤销及工作台回放已实测。
9. OpenBao 2.7.2 已验证 2/3 解封、KV v2 权限、分开加密的 Raft/审计备份及恢复。Secret Manager 已验证环境状态与身份轮换；Secret Broker 已通过真实 OpenBao 验证固定 KV 版本、运行中节点租约、用途、次数、有效期和一次性解包。
10. 已在 PostgreSQL 18.3 的两份实际验收库快照副本测量队列和事件存储。每份从 5 个项目、27 项任务、173 条事件开始，4 个 Worker 继续完成 27 项任务。列表、事件详情、入队、领取和完成的延迟与吞吐量见[专题 14、16 A.10](acceptance.md#queue-performance)。更大任务量与其他机器配置需要重新测量。

## B. 需要进一步细化

1. Agent Profile 的版本和配置结构。
2. Harness Adapter 的正式 Go 接口与错误模型。
3. ACP 内部消息 JSON Schema。
4. 已验证单个 Execution 关联多个进程并保留原生会话，覆盖 Claude 实现、Codex 实现和 Codex 只读规划的终态传输丢失。原生 CLI 被强制终止、节点重启后的会话恢复和跨 Session 交接仍待验收。
5. Issue 执行失败重试或验收退回后均使用新 Execution 和新 Worktree。退回修订从上一次被拒的候选开始，原工作区留存。多 Worker 租约续约已从正式 Worker 与项目专用身份验证，控制面重启后可续约；已过期、已结束或其他节点的执行拒绝续约。适配器异常退出后，正式 Worker 已验证在原租约内恢复会话，见 B.4。
6. `validation.yaml` 版本 1、候选自动选级、隔离 Runner 及检查报告已实测。beta #49 验证了 Worker 后台传输、从 Owner 工作台下载原始日志和产物，以及 Windows 节点向 Linux 控制面传送文件。中央 CI 文件按项目分类型到期清理见专题 13。beta #52 已验证关闭来源 Issue 时取消运行中的 Docker 检查、保留日志、收回容器和继续派发。beta #54 已从任务页主动取消运行中的 CI，核对名额释放、日志归档及原 Issue 保持开放。beta #55 已从任务页暂停真实 CI，重启后恢复同一候选的完整复验。模型回合中的暂停和原生 Session 恢复仍见 B.4。
7. `acceptance.yaml`、全局 Playbook、任务内计划生成、覆盖审查、修订归档和逐用例实测门禁已验证。六类预览反馈均有 Owner 处置入口。需求变更已实测到新候选页面，新工作项已独立完成真实 GitHub 交付，来源 Issue 保持开放。工作台已核对截图像素与归档回执。扩展视口截图已从工具栏采集到正式工作台归档。扩展录屏已从工具栏录制到归档回放；真实外部设备访问仍待验收。
8. 单项目交付已在真实仓库核对固定任务分支、PR 编号、验收 SHA、最终合并 SHA 与 Issue 评论。私有 beta #27 已实测创建 PR、请求 Squash Merge、发布回执评论和关闭 Issue 四次写入成功后分别丢失响应。每次重启控制面均能读取远端状态继续交付，只有一个 PR、一次合并和一条评论。[PR #28](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/28) · [唯一交付回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/27#issuecomment-6919716568)
9. 中央 CI 日志、报告和构建产物的项目保留策略已从真实文件、历史数据库及离线 Worker 队列验收。中央反馈截图支持单独设置期限，并保留原反馈和附件摘要。Worker 本地日志、报告和构建产物已在 Windows 与非 root Linux 验证独立期限、挂起上传、拒绝清理异常文件及旧备份恢复后的再次清理。反馈 WebM 的归档、回放、按项目独立到期及恢复后清理已经实测。扩展直接录制已与同一归档入口联通。
10. 多 Worker 项目授权及节点 bearer 令牌在线轮换已在真实运行中的租约验证。已用本机两个节点身份、TLS 1.3 和正式 Worker 验证客户端叶证书轮换、已建立连接撤销及重启后的授权。

## C. 产品交互待细化

1. Dashboard 信息架构；
2. 工作台已验证 Goal 计划逐版浏览、当前生效版与草案区分、子项返回绑定修订和已移除项追溯。Owner 可归档指定版本并核对 Git SHA，已从 Chrome 验证历史版和生效版、引用冲突、响应丢失与重启回读。
3. 运行中的 Codex 与 Claude 进程身份、原生会话及控制面重启后的工作台查询已验收。普通任务的原生工具事件已从运行中的任务页、最终回执和节点日志对照验收。通用实时日志展示仍待补齐。
4. 验收检查点问题的跨项目批量答复已从工作台验证，包括部分填写、响应丢失和不同 Owner 会话同时回答。Management 汇总其他阶段的问题仍待验收。
5. Owner 已从任务页起草计划、从通知查看原生 Management 的退回依据、发布修订并批准 D2 草案。校验失败、发布或批准响应丢失、两会话提交过时修订及重启后回读均已验收。跨操作审批中心仍待补齐。
6. 预览反馈工具；
7. 项目资料的导入、检索、版本回读和并发编辑已从 Owner 工作台验收。项目决策的提议、确认、拒绝、失效及竞争替代已从工作台验收。跨项目脱敏副本从固定资料修订起草、原生 Management 审核、Owner 发布、接收方回读及撤销已从 Chrome 验收。项目节点已从任务上下文接口核对固定需求、计划版本与前置子 PR。Codex 与 Claude 实现回合已从固定父需求取得子项所需的原文，Codex 依赖项还引用了前置交付的 PR 和合并 SHA。知识提取建议与其他角色上下文仍待完善。
8. 项目登记、Worker HEAD 检查和失败重试已验收。仓库入口、Go、Node.js、PHP Composer、Python 和 Rust Cargo 的声明依赖及已识别的包脚本入口均可按检查提交归档，原克隆离线后的浏览器回读已验收。工作台可核对分环境密钥需求和两种本地启动声明。从工作台和 Worker 命令行均已验证固定提交的 Local Mock / Integrated 启动。工作台留有数据库停机失败及恢复后重跑成功的两份独立回执。原生 Management Agent 对固定版本的用途、技术栈、依赖、启动及测试命令归纳已从 Chrome 验收，可核对来源。初始化报告已从工作台归档、切换和导出三版，克隆离线后可回读。Python 清单的动态字段、未解析来源和命令名均已核对。Cargo 工作区继承和重命名依赖已对照原生元数据验收。Maven 的父 POM 继承、属性替换、受管与条件依赖已对照原生模型验收。Ruby 的字面量 Gemfile 和锁文件直接依赖已对照原生 Bundler、工作台及离线归档验收，动态声明保留未解析状态。Gradle 版本目录、Groovy 与 Kotlin 顶层依赖声明已对照两版项目的原生配置和离线归档验收；运行时计算的依赖保留未解析状态。

## D. 不属于当前范围

- 自动生产部署；
- 生产环境运维；
- 对外租户；
- 公共 Agent 托管平台。
