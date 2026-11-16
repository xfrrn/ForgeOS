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

全局方法的 ID、目标、前置条件、步骤、通用预期及取证要求保存在[方法库](../schemas/playbooks.json)，格式遵循[版本 1 合同](../schemas/playbooks.schema.json)。库中另有 security 和 documentation，分别供安全检查和项目文档验收使用。Owner 通过 `GET /projects/{id}/acceptance-playbooks` 读取当前库；已获该项目授权的节点使用 `GET /internal/projects/{id}/acceptance-playbooks`。响应带版本和内嵌方法库文件的 SHA-256。

将固定候选的 `validation-assessment.matches[].playbooks` 按 ID 对应到 `acceptance-playbooks.playbooks`，即可同时取得通用方法与该组件的项目预期。例如 payment 提供支付失败、事件重送和账务核对步骤，具体金额、状态与测试账号由该项目的测试环境提供。

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

非 root 节点设置 `FORGEOS_REPO_ROOT`，通过 `worker plan-tests` 从标准输入读取请求。需要支持结构化输出的原生 Codex，可用 `FORGEOS_CODEX_BIN` 指定。每个验收标准写为一项，示例中的 SHA 替换为实际提交。

```json
{"repo":"example","base_commit":"<基础 SHA>","commit":"<候选 SHA>","criteria":["容量已满时拒绝入队。"],"policy":{"target":"pull_request"}}
```

Test Agent 在独立只读会话中读取固定差异、基础规则、验收标准和全局方法。输入差异最多 128 KiB，截断会告知模型。输出 JSON 保存两个提交、规则 blob、方法库摘要、会话、验收标准来源、用例及澄清问题。每个用例引用一个标准和一个已有方法，预期结果引用标准原文；每个标准都必须有用例或澄清问题。组件可指定人工测试和环境。Evidence 列的是要采集的证据，所有新用例的 Result 均为 pending。

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
