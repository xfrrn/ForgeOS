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

## 2. Issue 变化

- 补充信息：加入事件队列；
- 需求变更：创建 Requirement Revision；
- 任务执行中：安全检查点统一处理并支持事件合并；
- CI 中：固定当前 Commit，不修改正在验证的版本；
- 已完成：默认不自动重启；
- 关闭/取消：根据任务状态发起安全取消或归档。

导入或轮询已跟踪的开放 Issue 时，标题或正文的变化保存为新的 Requirement Revision。移除派单标签后仍会跟踪已导入的任务。每版有独立 ID、版本号、观测时间、Issue 来源链接和原始标题正文。重复同步、仅标签或更新时间变化均不会生成新版本。轮询能保存读取到的版本；两次轮询之间未被观察到的编辑不能还原。

任务详情显示任务绑定版本与可浏览的需求历史。Owner 可以调用 `GET /tasks/{id}/requirements/{revision}` 获取完整快照。从子项访问时返回父 Goal 的原始需求。执行领取时冻结需求 ID；观察到新版本会记录事件，已有候选、验收和执行记录保持其原版本。

对于尚未转为 Goal 的 Issue，Owner 可在任务详情查看最新正文，选择“采用修订并排队执行”。API 是 `POST /tasks/{id}/requirements/{revision}/apply`。适用检查点为待领取、执行失败或待验收；运行中返回 409。采用版本后，Work Item 保留原 ID，新 Execution 从同一基础提交和新需求开始，旧候选、退回意见与旧 Execution 保持关联。已创建计划的 Goal 通过计划修订处理子项范围。

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

目标分支由项目配置决定，默认仓库默认分支。

## 4. 自动合并条件

- 必需人工验收已通过（若该任务要求人工验收）；
- 验收 Commit 与待合并代码一致；
- 必需 CI 全部通过；
- 不存在未解决的阻塞 Review；
- 无合并冲突；
- Policy Engine 允许；
- GitHub 分支保护条件满足。

不能绕过仓库保护规则。若仓库要求额外人工 Review，则等待人工完成。

## 5. 合并策略

默认 Squash Merge。项目可配置其他策略，但第一版建议统一 Squash。

## 6. Issue 关闭

- 简单 Work Item：PR 合并成功后关闭；
- 复杂 Goal：全部必需 Work Item/Milestone 完成后关闭父 Issue；
- 子 PR 只引用父 Issue，不提前关闭；
- 普通讨论不自动关闭；
- 失败或取消保留 Issue 并反馈原因。

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
