# 12 — GitHub 需求、代码交付与结果反馈

## 1. GitHub Issue 接入

- GitHub Issue 是第一版主要需求入口。
- 中台 Web 是辅助入口。
- Agent 任务模板自动触发。
- 普通讨论模板不自动触发。
- 普通讨论可转换为 Agent 任务。
- 仓库 ID 与导入项目映射，不按名称猜测本地目录。
- 本地 Poller 为主，Webhook 为补充。
- GitHub 事件需要身份验证、去重和原始记录。

Owner 可在项目的「讨论收件箱」按编号派发本仓库的开放 Issue，无需等待轮询。默认沿用远端声明的类型；遇到待澄清的 Issue，选定工作类型后再提交。重复派发会打开原工作项。[工作台验收](acceptance.md#web-issue-number)

启用 Webhook 的项目逐条保存已验证的原始 JSON 字节、事件名、投递 UUID、仓库 ID、接收时间和 SHA-256。Owner 从 `GET /projects/{id}/github-events` 查看摘要，按最后一条的数字 `id` 传入 `?before=` 可继续翻页，每页最多 100 条。访问 `GET /projects/{id}/github-events/{delivery_uuid}` 可读回原文。原文可能包含私有 Issue 与评论内容，随项目数据库一起备份。

新的 Issues 和 Issue comments 投递唤醒本项目的轮询，扫描运行期间的多个通知合并到下一次扫描。控制面从已绑定的仓库重新读取状态，沿用普通讨论、标签派单、需求版本和评论检查点规则。投递确认表示原文已保存；后续工作项和评论有独立回执。GitHub 限流时保留原重试间隔。项目可继续单独使用定时轮询。[接入配置](local-development.md#github-webhook)

## 2. Issue 变化

- 补充信息：加入事件队列；
- 需求变更：创建 Requirement Revision；
- 任务执行中：安全检查点统一处理并支持事件合并；
- CI 中：固定当前 Commit，不修改正在验证的版本；
- 已完成：默认不自动重启；
- 关闭/取消：根据任务状态发起安全取消或归档。

完成开放 Issue 的分页后，Poller 会逐个回查从列表消失的在办任务。只有 GitHub 返回已关闭状态，且仓库、Issue 数字 ID、编号、链接和时间均与已跟踪记录相符，控制面才处理关闭。分页失败、单条读取失败或远端仍开放时保留任务。关闭回执可在任务详情的 `source_closure` 查看，包括 GitHub 关闭时间、原因与本地观测时间。

尚待执行、待处理或待验收的工作项转为 CANCELLED。父 Goal 的关闭同时处理未完成子项，保留计划和已批准或已交付子项。运行中的工作项先记录取消请求并继续占用项目名额，等 Worker 回报或 Owner 核实失联执行后再结算。重新打开同一 Issue 可以继续查阅原任务。已批准的交付按原交付回执核对远端状态。

导入或轮询已跟踪的开放 Issue 时，标题或正文的变化保存为新的 Requirement Revision。移除派单标签后仍会跟踪已导入的任务。每版有独立 ID、版本号、观测时间、Issue 来源链接和原始标题正文。重复同步、仅标签或更新时间变化均不会生成新版本。轮询能保存读取到的版本；两次轮询之间未被观察到的编辑不能还原。

任务详情显示任务绑定版本与可浏览的需求历史。Owner 可以调用 `GET /tasks/{id}/requirements/{revision}` 获取完整快照。从子项访问时返回父 Goal 的原始需求。执行领取时冻结需求 ID；观察到新版本会记录事件，已有候选、验收和执行记录保持其原版本。

工作台还有一个需求来源。对 Management 已归类为“需求变更”的预览反馈，Owner 可在原反馈下读取最新需求，编辑完整正文并保存。新修订保留原 Issue 身份和标题，标注 Owner、反馈编号和来源预览。子项的反馈进入父 Goal 的需求历史。重复读取未改动的 GitHub Issue 会保留本地修订；之后真实发生的 GitHub 标题或正文修改会形成另一版快照。可以在历史中分别核对和采用。

对于尚未转为 Goal 的 Issue，Owner 可在任务详情查看最新正文，选择“采用修订并排队执行”。API 是 `POST /tasks/{id}/requirements/{revision}/apply`。适用检查点为待领取、执行失败或待验收；运行中返回 409。采用版本后，Work Item 保留原 ID，新 Execution 从同一基础提交和新需求开始，旧候选、退回意见与旧 Execution 保持关联。已创建计划的 Goal 通过计划修订处理子项范围。

轮询已跟踪的开放 Issue 时按页读取 GitHub 评论。每条观测包含原始 API 对象、来源评论 ID、更新时间和中台事件 ID；正文编辑另存事件。重复读取同一版本不会重复入队。从 Owner 入口 `GET /tasks/{id}/comments?after={event_id}` 顺序读取，每页最多 30 条，子工作项读取父 Issue 的原始评论。未转换的普通讨论仍留在收件箱。轮询只能记录实际读到的版本。

## 3. PR 流程

人工验收通过后：
1. 冻结验收 Commit；
2. 推送任务分支；
3. 自动创建 PR；
4. 关联 Issue、Goal、Work Item、Validation、Acceptance；
5. 检查 CI、Review、冲突、权限和分支保护；
6. 默认 Squash Merge；
7. 记录最终 Merge Commit SHA；
8. 更新任务和 Issue。

目标分支由项目配置决定，默认仓库默认分支。控制面可通过 `FORGEOS_DELIVERY_PROJECTS_FILE` 启用多个已登记项目。批准请求按工作项所属项目选择目标，同一 Owner 入口可向不同仓库交付。各交付循环只领取自己项目的队列、发布其候选状态、释放其后续工作并处理该项目的 Goal 关闭。[配置示例](local-development.md#github-交付)

当 GitHub 报告 BEHIND，Owner 可在任务详情选择“在最新目标版本上重新验证”。控制面核对项目、Issue、PR、分支和原批准 SHA，从目标分支 ref 核对当前提交，在私有 Git 索引中应用原补丁。PR 的 base SHA 可能仍是旧基线，不能据此判断目标没有前进。关闭旧 PR 后再次核对目标 ref；若期间前进则返回 409，保留原交付记录供重试。干净应用后关闭被替换的 PR，并归档其原交付记录。不能干净应用时返回 409，原 PR 保持原状。

支持 `supports_validation` 的 Worker 检出这个新 SHA 并重跑本地验证，执行无模型会话。Owner 按新 SHA 重新查看差异和验收，随后创建新 PR 并运行原有必需检查。页面保留旧 PR、原候选、新基线和新候选。原 HTTP 操作为 `POST /tasks/{id}/delivery/refresh`，提交 `execution_id` 和原批准 `commit`；重放可恢复已确认的刷新。原执行的差异始终与其记录的基础提交比较。

## 4. 自动合并条件

- 必需人工验收已通过（若该任务要求人工验收）；
- 验收 Commit 与待合并代码一致；
- 必需 CI 全部通过；
- 不存在未解决的阻塞 Review；
- 无合并冲突；
- Policy Engine 允许；
- GitHub 分支保护条件满足。

不能绕过仓库保护规则。若仓库要求额外人工 Review，则等待人工完成。

### 本地 CI 回传

配置了 GitHub 交付的项目在记录 PR 后，将固定候选的本地验证结果写入 Commit Status，context 为 `forgeos/local-ci`。仓库维护者可以把它加入目标分支的必需检查。现有远端 CI、人工 Review 和保护规则继续各自判定。成功验收显示 success；已在远端的失败候选可以显式同步，代码或构建失败显示 failure，超时、环境或基础设施失败显示 error。[GitHub Commit Status 接口](https://docs.github.com/en/rest/commits/statuses)

Owner 可向 `POST /tasks/{id}/validation-status` 提交 `execution_id` 和完整 `commit`，用于补写旧执行或已在远端的失败候选。控制面核对本地登记的 origin、GitHub 仓库 ID 和远端提交对象，同一仓库同一 SHA 的写入串行。缺少远端提交时返回 409。状态说明只包含任务号、执行号、验证方案和检查数量。执行记录保存 GitHub status ID，重试先核对已有状态。

## 5. 合并策略

默认 Squash Merge。项目可配置其他策略，但第一版建议统一 Squash。

## 6. Issue 关闭

- 简单 Work Item：PR 合并成功后关闭；
- 复杂 Goal：全部必需 Work Item/Milestone 完成后关闭父 Issue；
- 子 PR 只引用父 Issue，不提前关闭；
- 普通讨论不自动关闭；
- 失败或取消保留 Issue 并反馈原因。

项目配置 GitHub 交付后，交付器也会处理执行失败、Owner 取消、来源关闭和失联执行的终态事件。实际执行返回失败时报告退出码，已确认本地验证失败时标明验证失败。取消和人工结束分别说明触发原因。评论带任务、执行（若有）和事件编号。工作台时间线保留 GitHub 确认时间，Owner 可返回来源 Issue 查看评论。节点日志、原始错误正文及凭据留在项目运行环境。

控制面为每条新终态事件事务性建档，重新派发同一任务不会覆盖早先尝试。交付器验证项目及 Issue 身份，通过分页查找同一条评论后确认送达。若写入成功而确认丢失，2 分钟领取期限后可重领。该入口仅反馈结果，Issue 仍保持 GitHub 上原有的开闭状态。迁移 110 之前的历史事件保留原记录。

## 7. 结果反馈

重要事件：
- 任务接收；
- 计划完成；
- 等待人工；
- CI/Review 结果；
- 等待验收；
- PR 创建；
- PR 合并；
- 执行失败。

避免每条命令都发评论。可以维护由 Bot 管理的状态摘要评论。

## 8. 生产发布边界

Delivery Manager 在 PR 合并后结束。生产部署、生产发布和生产环境验证由人工负责。
