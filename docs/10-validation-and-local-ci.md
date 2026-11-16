# 10 — 代码审查与自动化验证

## 1. 职责分离

- 项目仓库声明如何验证；
- 中台决定本次执行哪些验证和允许的权限；
- Local CI Runner 在本地执行；
- Review Agent 独立只读审查；
- Validation Aggregator 汇总结果。

## 2. validation.yaml

建议包含：
- checks：单项检查；
- profiles：检查组合；
- command：数组形式的可执行程序和参数；
- timeout；
- working directory；
- environment profile；
- report/artifact 定义；
- 必需或可选标识。

仓库配置不能授予自身更高权限。验证配置修改需独立审查，敏感能力由 Policy Engine 控制。

## 3. 验证等级

- V1：格式、Lint、类型检查；
- V2：V1 + 单元测试 + 构建；
- V3：V2 + 集成测试/E2E；
- V4：V3 + 安全检查 + 关键业务回归。

最终等级由任务类型、Git Diff、敏感模块、项目规则和交付目标共同决定。Management Agent 可提高等级，不能降低 Policy Engine 最低要求。

## 4. Review Engine

- 独立 Session；
- 固定 Commit；
- 只读工作区；
- 不允许自行修改代码；
- 输出 Finding ID、Severity、File/Line、Description、Evidence、Recommendation、Confidence、Blocking。

严重等级：Critical、High、Medium、Low。

LLM Finding 必须核对证据，不能只凭模型标签自动认定真实漏洞。

本地入口为 `worker review-candidate`。用非 root 审查账号设置 `FORGEOS_REPO_ROOT` 和原生 Claude Code 的 `FORGEOS_CLAUDE_BIN`，通过标准输入传入一个 JSON 对象。

```json
{"repo":"example","base_commit":"<完整基础 SHA>","commit":"<完整候选 SHA>","requirement":"队列达到上限时拒绝新增工作。"}
```

Worker 从授权仓库的 Git 对象读取变更文件的前后内容和模式，基础必须是候选的祖先。每次审查启动独立的 Claude 会话，向其提供固定输入，关闭文件、命令、网络工具和 MCP；审查进程在临时目录工作。输出包括两个 SHA、原生 Session、摘要和发现列表。每条发现绑定 base 或 candidate 的文件与一基行号。Worker 逐条核对原文行，`evidence_verified` 表示引用是否匹配。技术结论仍需结合上下文判断。

当前文本审查入口支持最多 128 个变更路径、前后文本合计 192 KiB。二进制、非普通文件或超限输入会明确报错，保留人工审查。命令输出可独立留档。

### 让审查报告参与候选验收

控制面设置 `FORGEOS_REVIEWER_PROJECT_ID` 和专用 `FORGEOS_REVIEWER_TOKEN`。审查节点设置相同的 Reviewer 令牌、控制面回环地址、仓库根目录和 Claude 可执行文件，运行 `worker reviewer`。开发 Worker、Owner、Management 与 Reviewer 的 API 凭据各自独立。审查节点和控制面均需能读取所登记仓库内的候选提交。

| 操作 | Owner 接口与结果 |
| --- | --- |
| 为项目启用必需审查 | `PUT /projects/{id}/review-policy`，请求 `{"required":true}`。最新候选通过本地验证后进入审查。已有项目默认沿用其原验收方式。 |
| 单独请求或重试失败的审查 | `POST /tasks/{id}/code-review`，提交 `execution_id`、`commit`。相同候选复用请求，失败作业可以重新领取。重复请求保留已完成的报告。 |
| 查看执行对应的回执 | `GET /tasks/{id}/code-review?execution_id=…`。回执包含基础和候选 SHA、请求和完成时间、报告或失败时间。Worker 中断后租约六分钟到期，其他 Reviewer 可重新领取。 |
| 核对发现 | 控制面重新读取固定 Git 对象并逐项校对引用。若仍有阻塞发现或引用未核实，Owner 可退回候选，或在核对后向 `POST /tasks/{id}/code-review/check` 提交 `execution_id`、`commit`、`note`，记录判断依据。随后按原验收入口提交决定。 |

项目已启用审查或 Owner 单独发起审查时，缺少报告、审查失败或发现尚待核对，批准请求返回 409。退回后产生的新执行需要自己的审查。Reviewer 无权批准或交付候选。

在任务详情展开对应的 Worker 执行，可请求或重试独立审查。“查看审查报告”展示两个固定 SHA、审查会话、摘要和逐项证据。Owner 可以填写发现核对说明，或在候选验收中说明退回原因。页面与批准接口使用同一门禁状态。历史执行保留报告和 Owner 说明。

## 5. Local CI Runner

接口建议：
- create_run；
- get_run；
- get_logs；
- get_artifacts；
- cancel_run；
- retry_run。

结果必须包含：
- Commit SHA；
- validation 配置版本；
- Profile；
- 每个 Check 的状态和退出码；
- 日志和报告引用；
- Runner 身份；
- 开始结束时间。

## 6. 失败分类

- CODE_FAILURE；
- BUILD_FAILURE；
- TEST_ENV_FAILURE；
- INFRA_FAILURE；
- TIMEOUT；
- CONFIG_FAILURE；
- SECURITY_FAILURE。

基础设施故障不应被当成代码缺陷。

## 7. 验证规则

- 每个结果绑定固定 Commit SHA；
- 代码变化后重新执行受影响检查；
- 所有必需检查通过才能形成有效结论；
- 自动修复轮数有限，建议初始最多两轮；
- 测试脚本不能自行降低验证要求；
- 生产密钥不注入普通 CI。
