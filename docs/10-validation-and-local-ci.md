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
