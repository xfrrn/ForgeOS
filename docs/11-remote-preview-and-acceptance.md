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

运行 `worker review-test-plan` 进行 Management 覆盖审查。在标准输入的 `request` 字段提供原始生成请求，`plan` 字段提供生成的完整计划。Worker 根据独立提供的请求重新读取 Git 对象和规则，核对验收标准、方法库摘要、用例编号与 pending 状态，然后启动另一只读原生会话。

每个标准均返回所引用的用例、判定及理由。步骤与证据足以证明预期时为 covered，缺少可观测步骤或证据时为 gap，业务条件不足时为 needs_clarification。总结果优先呈现 changes_requested，其次是 needs_clarification。回执绑定计划 JSON 的 SHA-256、两个提交及两次会话。覆盖审查完成后，用例仍需要实际执行并记录结果。

### 在工作项中派发与修订测试计划

控制面配置 `FORGEOS_TEST_AGENT_PROJECT_ID`、`FORGEOS_TEST_AGENT_TOKEN`，以及同一项目的 Management 身份。Test Agent 节点设置回环 API 地址和专用令牌，运行 `worker test-agent`；Management 节点运行 `worker management`。两者使用独立只读会话和凭据，控制面提供已冻结的输入。

在任务详情展开最新的已验证候选，打开“查看测试计划”。可以逐行补充验收标准，也可直接点击“生成测试计划”。任务绑定的原始需求始终参与生成。控制面归档该候选的基础、差异、仓库规则、方法库及标准。Test Agent 完成计划后，Management 会自动领取覆盖审查。页面按修订展示来源、用例、环境、步骤、计划采集的证据、澄清问题和覆盖结论。

Owner 接口 `POST /tasks/{id}/test-plans` 接受 `execution_id`、`commit`、`previous_revision` 及可选的 `criteria`。首次提交使用 0。同一请求重试返回原修订；需要重试失败作业或提供澄清时，以当前修订号申请新版本。按 `GET /tasks/{id}/test-plans?execution_id=…&revision=…` 读取历史，省略 revision 返回最新版本。请求计划后，当前修订的覆盖审查必须通过才能批准候选；退回候选仍可用。工作台上的“待执行”表示计划尚未提供用例运行证据。

### 记录用例实测

组件规则要求人工验收，或要求根据条件决定是否人工验收时，控制面要求先生成计划。申请过计划的候选在最新修订通过覆盖审查后，还需每个用例的最近一次实测都通过，才能批准。修订和执行的历史证据保留供复查。

Owner 在任务的测试计划内展开 human 用例，点击“记录人工实测”，填写实际操作、观察结果和证据。runner 用例由获该项目授权的具名 Worker 通过 `POST /internal/projects/{project}/tasks/{id}/test-results` 提交。身份取自已认证入口。Owner 表单使用 `POST /tasks/{id}/test-results`。

两个入口都需要 `execution_id`、`revision`、`commit`、`plan_sha256`、`case_id`、`previous_attempt`、`operations`、`actual_result`、`evidence` 和 `result`。操作和证据逐条填写。结果可为 passed、failed 或 blocked。首次回报的 previous_attempt 为 0，复测时填写该用例最近一次实测序号。重复发送相同回报返回已有结果。任务详情可展开查看每次实测的身份、时间、操作、观察及证据。

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

### 本机静态预览

在控制面设置 `FORGEOS_PREVIEW_ADDR=127.0.0.1:8091`，重启后在通过验证的执行详情展开“本机静态预览”。填写项目中可公开的静态目录，例如 public。目录应在该候选提交中包含 index.html。点击“启动预览”“打开预览”。新标签页使用独立的随机 localhost 子域名，能读取该目录内已提交的普通文件。支持绝对站内路径、相对资源和目录首页；隐藏文件、符号链接、目录清单与工作区未提交内容不可读取。单个资源上限 8 MiB。

预览回执绑定项目、任务、执行、工作树、SHA、公开目录、local 环境、URL、访问策略和期限。持有完整预览地址的本机浏览器可访问；地址应按访问凭据保管。控制面 Owner 负责启动、查询和停止。每次读取页面或资源都会检查期限及停止状态，响应头 `X-ForgeOS-Commit` 可用于复核。保持相同预览端口重启控制面后，尚未到期的地址继续有效。

通过 `POST /tasks/{id}/previews` 发送 `execution_id`、`commit`、`directory` 和可选 `ttl_seconds`（默认一小时，最长一天）。同一执行、目录和端口已有活动实例时返回原回执。`GET /tasks/{id}/previews?execution_id=…` 查询历次预览，`DELETE /tasks/{id}/previews/{preview}` 停止对应实例。

配置私网网关后，任务详情提供“打开私网预览”。Owner 列表接口的 gateway 字段返回配置的 DNS 区和端口，每个实例的私网地址为 `https://p-<预览 ID>.<gateway>/`。访问时需要网关认可的客户端证书。Caddy 的部署和 DNS 约定见[部署与运行](14-deployment-and-operations.md)。

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

### 安装与选择页面元素

在 Chromium 扩展管理页开启开发者模式，选择“加载已解压的扩展程序”，打开仓库内的 [extension 目录](../extension/manifest.json)。在已登录的工作台打开“ForgeOS 预览反馈”，点击“连接此工作台”。扩展自身只保存工作台的来源地址。工具栏按钮也可用 Alt+Shift+F 打开。

1. 在候选预览标签页打开扩展。点击“选择页面元素”，再点需要反馈的元素。Esc 可取消。反馈整页时直接选“反馈整个页面”。
2. 扩展用首页响应头核对任务、执行和提交，打开已连接工作台的反馈表单。新标签页若尚未连接，按工作台提示登录。核对路径、所选元素、浏览器主版本和视口，再填写观察并提交。页面版本与归档不符时，工作台要求重新选择。
3. 从原预览的“预览反馈”回看该记录。元素定位使用标签与同类型序号，不读取文本、属性值、输入值、Cookie 或页面存储。页面路径不含查询参数和片段。采集仅在用户调用扩展后进行，扩展清单声明 activeTab 和 scripting 两项权限。[Chrome activeTab 权限说明](https://developer.chrome.com/docs/extensions/develop/concepts/activeTab)

安装 Python Playwright 后，可以用 `python scripts/probe_preview_extension.py --browser <Chrome-for-Testing可执行文件>` 重跑工具栏、取消、选择和隐私回归。探针启动一次性浏览器配置与两个本机页面，不需要项目凭据。验收工作台与数据库的完整场景见[验收记录](acceptance.md)。

## 7. 反馈流转

反馈先进入 Feedback Manager，由 Management Agent 分类：
- 验收通过；
- Bug；
- UI 修改；
- 需求变更；
- 新 Work Item；
- 阻塞/澄清。

不能直接把任意反馈文本注入正在运行的 CLI。应进入任务/命令队列，并按安全检查点处理。

### 从候选预览登记反馈

在任务详情展开静态预览，点击该实例旁的“预览反馈”。页面显示执行编号、目录和固定提交。在表单填写预览中的页面路径、实际观察或修改意见，可选同一次执行已有测试计划中的用例。提交后可以在该预览下复查反馈编号、时间、路径、备注、计划版本和用例。Owner 在预览停止或到期后仍可对同一历史候选登记观察。

Owner 使用 `GET /tasks/{id}/previews/{preview}/feedback` 读取预览上下文、测试计划和反馈历史。向同一地址 POST 时提供 `request_id`（UUID）、`execution_id`、`commit`、`path`、`note`，关联用例时另提供 `plan_revision` 和 `case_id`。可选采集字段为浏览器系列与主版本 `browser`、视口 `width` 与 `height`、纯结构定位器 `element`。页面路径省略查询和片段，定位器由标签及同类型序号组成。后端核对版本和来源；同一请求重试复用编号并只写一次任务事件。

收到的记录进入 awaiting_review，等待 Management 分类。登记反馈本身保留原任务的验收状态。

## 8. 验收版本约束

人工验收绑定 Commit SHA。验收后代码发生变化时，必须判断是否需要重新自动验证和人工验收。
