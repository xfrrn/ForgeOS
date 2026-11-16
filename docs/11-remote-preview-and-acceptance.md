# 11 — 远程预览与人工验收

## 1. 目标

本地机器运行项目和 Agent，用户在外部电脑通过 Tailscale 私有网络访问预览，直接检查页面并提交修改意见。

## 2. 组件

- Acceptance Strategy Engine；
- Global Acceptance Playbooks；
- Project `acceptance.yaml`；
- Test Plan Generator；
- Management Review；
- Preview Manager；
- Remote Access Gateway；
- Browser Feedback Extension；
- Feedback Manager。

## 3. 双层验收方法库

### 全局 Playbooks

覆盖：
- UI/Frontend；
- API/Backend；
- Database/Migration；
- Authentication/Authorization；
- Payment；
- Cloud Integration；
- Async Jobs；
- AI/Media；
- Desktop；
- Mobile；
- Deployment；
- Performance。

### 项目验收规则

仓库通过 `acceptance.yaml` 和项目专用文档声明：
- 模块路径；
- 对应 Playbooks；
- 最低验证等级；
- 是否需要人工验收；
- 测试环境；
- 项目特有预期行为。

项目根目录的 `acceptance.yaml` 使用[版本 1 Schema](../schemas/acceptance.schema.json)。项目可声明整体最低等级，各组件配置路径、Playbook、最低等级，以及人工验收要求、环境和预期行为。`manual_acceptance.required` 可为 true、false 或 `conditional`。路径从仓库根开始；`*` 和 `?` 在一个片段内匹配，独立片段 `**` 可跨目录。

Owner 通过 `GET /projects/{id}/validation-assessment?base=<基础 SHA>&commit=<候选 SHA>` 查询命中的模块及方法。授权节点使用 `/internal/projects/{id}/validation-assessment`。响应包含模块命中数量、示例路径、Playbook、人工验收条件和项目声明的预期。删除和移出敏感目录也参与匹配，规则始终取自基础提交。

## 4. 测试计划

至少包含：
- Test Case ID；
- Playbook ID；
- Objective；
- Preconditions；
- Environment；
- Steps；
- Expected Result；
- Evidence；
- Tester；
- Result。

每项重要验收标准应有对应测试覆盖。信息不足时提出澄清，不得凭空编造业务预期。

## 5. 远程预览

Preview 必须绑定：
- Project；
- Work Item；
- Worktree；
- Commit SHA；
- Environment；
- URL；
- 生命周期；
- 访问策略。

Local Preview 与 Staging 分离。第三方回调等场景需要独立 Staging 回调入口，不应为方便验收公开整个私有网络。

## 6. 浏览器反馈扩展

功能：
- 页面元素批注；
- 截图标注；
- 录屏；
- Bug 报告；
- 页面 URL；
- 浏览器和视口信息；
- Commit/Preview 上下文；
- 关联测试用例。

采用最小浏览器权限。不得收集 Cookie、密码、Token 或未授权表单敏感内容。截图应支持敏感区域遮挡。

## 7. 反馈流转

反馈先进入 Feedback Manager，由 Management Agent 分类：
- 验收通过；
- Bug；
- UI 修改；
- 需求变更；
- 新 Work Item；
- 阻塞/澄清。

不能直接把任意反馈文本注入正在运行的 CLI。应进入任务/命令队列，并按安全检查点处理。

## 8. 验收版本约束

人工验收绑定 Commit SHA。验收后代码发生变化时，必须判断是否需要重新自动验证和人工验收。
