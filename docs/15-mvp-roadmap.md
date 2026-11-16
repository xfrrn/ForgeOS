# 15 — ForgeOS 总体实施路线

先建立能独立运行的控制面和 Worker 基座，再用它交付第一个真实 Issue。后续开发沿用同一套任务、执行和交付记录。前端从单项目后端闭环跑通后开始接入。

| 阶段 | 这一阶段解决的问题 | 进入下一阶段时已有的能力 |
| --- | --- | --- |
| 一、运行基座 | 中台能否把一项工作交给本机 Worker，并在重启后解释这次执行？ | 可通过 API 派发、查询和追踪的 Work Item 与 Execution |
| 二、单项目交付（MVP） | 一条 GitHub Issue 能否经过 Codex 修改、验证和人工验收，安全合入仓库？ | 一个 Work Item 从需求到 Merge SHA 的完整交付记录，以及可用的基础管理页 |
| 三、复杂需求协作 | 一个需要拆分、等待人工决定、返工的 Goal 能否逐项交付？ | 每个子工作项复用第二阶段的交付流程，父 Goal 可追踪计划、依赖和决策 |
| 四、多项目运行 | 第二个项目、另一种 Harness 或另一台 Worker 能否复用现有流程？ | 按项目隔离权限与资源，可审计、可恢复的多项目运行环境 |

## 一、运行基座

先在一台开发机上启动 Go 模块化单体控制面、PostgreSQL 和独立的 Worker Daemon。控制面按项目校验操作权限，Worker 只在获准的工作区执行，日志保留执行证据并排除凭据明文。基座以一个可重复的本地任务验收。GitHub、模型账号和 Web 界面都可以等到第二阶段接入。

1. **让控制面记住一次工作。** 首批代码包括控制面入口、数据库迁移和最少的 API。通过 API 登记一个本地 Git 项目、创建一个 Work Item、查询其状态和事件。PostgreSQL 保存项目、工作项、执行尝试及其关联。重启控制面后仍能查到同一条记录。
2. **让独立 Worker 完成这次工作。** 控制面创建 Execution 并按租约派发，非 root Worker 为任务准备独立 Worktree，运行一个预先定义的本地任务，例如用 `git rev-parse HEAD` 读取检出版本。Worker 回报工作区、Commit、退出码和事件，控制面据此更新正式状态。首期只需这一种可验证的任务。
3. **检查并发和故障恢复。** 在第一个写入型工作项仍在执行时，提交同项目的第二个写入型工作项，观察它是否排队。重放已处理的回报，确认不会重复推进状态。Worker 失联时能查询到上一次执行及其工作区，重新派发前先核对执行结果。

**基座验收**　从空数据库运行迁移，启动两个进程，通过 API 完成上述操作。维护者能从 Work Item 查到对应 Execution、工作区、提交版本和结果。执行失败时能定位到这次尝试，进程退出本身不会被当作任务成功。这个场景跑通后即可开始接交付链。

首批实现涉及的职责和状态定义见[系统架构](02-system-architecture.md)、[领域模型](04-domain-model.md)和[Worker 运行时](07-harness-and-worker-runtime.md)。租约超时、事件幂等以及重试时的 Worktree 处置随这次联调细化，见[待决事项 B.4–B.5](16-open-decisions.md)。

## 二、单项目交付与基础管理页

**当前实测进度**　在公开项目的临时克隆上，本地 Codex 修正了 README 中两处仍存在的拼写问题。候选 `5aa24090` 通过文档验证，等待维护者验收。[原始 Issue](https://github.com/galiakr/ai-dev-toolkit/issues/20)

另在隔离项目实测了退回修订。首轮候选 `bf8b1171` 被要求补充示例，Worker 在新的 Worktree 中保留了原修复并加入示例，得到 `6e5c1fab`。两轮均通过文档验证。批准第二个 SHA 后重启控制面，仍能查到两次决定及最终的 `ACCEPTED` 状态。

临时私有仓库已完成真实 GitHub 交付。任务 10 验证了按 SHA 验收、创建 PR、Squash Merge、评论回写和关闭 Issue。随后将 GitHub Actions 的 `example` 设为 `main` 的必需检查。任务 11 从 Issue #3 生成候选 `79c5d36b`，通过本地验证和验收后创建 PR #4。GitHub 在检查尚未出现时返回 `BLOCKED`，检查成功后才合并为 `a85dff98`。配置方式见[本地运行指南](local-development.md)。

**先完成后端闭环。** 选一个真实仓库，把 GitHub 仓库 ID 映射到已登记项目。Poller 将 Agent Issue 转成基座里的 Work Item，普通讨论留在收件箱。Worker 接入 Codex Adapter 和进程管理，记录原生 Session ID、候选 Commit 与执行事件。Local CI Runner 验证候选 Commit，维护者先通过受控 API 验收同一 SHA。验收通过后才冻结版本、创建 PR，满足 CI、审查、权限、分支保护及无冲突条件后默认 Squash Merge，回写 Issue 和中台。

**任务工作台。** React 任务列表和详情页已接入同一控制面。维护者可查看 Worker 的每次尝试、验证输出及 Git 差异，对照执行 ID 与候选 SHA 批准或退回。已交付任务可从详情页追到 PR 和合并 SHA。工作台与原有 API 共用 Owner 鉴权和验收事务。[页面启动和操作方法](local-development.md)

**MVP 验收**　用一个简单 Issue 在管理页完成人工验收，从 Issue 能追到 PR 和最终 Merge SHA。重复同步该 Issue 只生成一个工作项。必需 CI 失败时交付停在验证环节，候选 SHA 变化后重新执行受影响的检查并复核验收，旧记录不能直接放行新版本。交付条件见[本地验证](10-validation-and-local-ci.md)和[GitHub 交付](12-github-delivery-and-feedback.md)。

## 三、复杂需求协作

单个 Work Item 已能独立交付后，引入 Planner 和 Management Agent。Planner 根据仓库资料把一个 Goal 拆成有依赖的 Milestone 和 Work Item，控制面保存 Plan Revision，并将就绪的子项交给既有执行链。需要 Owner 决定的问题进入队列，在安全检查点暂停和恢复。管理页沿用任务详情，增加任务树、待回答的问题和验收反馈入口。

在同一个 Goal 上逐步加入只读 Review、按风险选择的验证等级、项目验收规则与 Playbook。需要人工检查页面的任务再接 Tailscale 私网预览和浏览器批注，把反馈绑定到正在验收的 Commit。项目知识和决策记录为后续子项提供有来源的上下文。

**阶段验收**　选一个包含两个有依赖工作项的 Goal。让其中一项因业务问题等待，在 Owner 回答后从检查点继续。验收时退回一次修改，能查到反馈、修正后的 SHA 和新验证结果。第一个子 PR 合并后父 Issue 仍然打开，所有必需项交付后才具备关闭条件。详见[规划与编排](06-orchestration-and-planning.md)和[远程验收](11-remote-preview-and-acceptance.md)。

## 四、多项目运行

用第二个真实项目检验配置、环境凭据和知识的隔离。按项目和环境授权，通过 OpenBao 与 Secret Broker 管理业务凭据。涉及业务密钥的项目，应在首次使用前接好相应的凭据管理。项目经验经审核、脱敏后才进入共享知识库。结合实际检索需求完善索引、归档和审计。

在多个项目争用执行资源时扩展 Worker 注册、容量调度和失联恢复。接入 Claude Code 或通用 CLI 时，由 Adapter 报告实际能力。不同项目沿用相同的 Work Item、验证和交付规则，每个项目仍遵守已确认的写入并发策略。

**阶段验收**　让两个项目各交付一个工作项，验证两个项目的执行记录和凭据访问范围彼此独立。让另一台 Worker 或另一种 Harness 完成同类任务，通过同一套 API 查询结果。模拟节点失联后核对已有执行、PR 和合并结果，再决定是否重试。详见[知识管理](08-project-knowledge-and-context.md)、[权限与密钥](09-security-and-secrets.md)和[部署与恢复](14-deployment-and-operations.md)。

每次进入新阶段，根据上一阶段的实测结果补齐该阶段的范围、依赖、数据与 API、风险、恢复办法和验收记录，并更新相关专题文档。[滚动规划原则](06-orchestration-and-planning.md)
