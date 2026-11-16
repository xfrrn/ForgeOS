# 09 — 安全、权限与密钥管理

## 1. 环境隔离

至少区分：
- Local Mock；
- Local Integrated；
- CI/Test；
- Staging；
- Production。

不同环境凭据必须独立，不共享实际密钥值。开发和生产凭据绝不共用。

## 2. OpenBao + 自研 Secret Manager

- OpenBao：真实密钥加密存储、版本、底层访问策略和审计。
- Go Secret Manager：项目、环境、密钥元数据、状态和业务授权。
- Secret Broker：按任务和执行身份受控分发。
- PostgreSQL：不保存明文密钥。

路径建议：`secret/projects/{project_id}/{environment}/{secret_name}`。

## 3. 风险等级

- S0：非敏感配置；
- S1：低风险测试凭据；
- S2：敏感开发/测试凭据；
- S3：生产私钥、生产管理员凭据、主加密密钥等。

风险等级与环境是独立维度。

## 4. Secret 使用

每次授权绑定：
- project；
- environment；
- principal；
- task/execution；
- purpose；
- resource scope；
- TTL；
- 可选最大使用次数。

Agent 默认不能直接读取生产密钥。优先使用短期、最小权限凭据；支付私钥优先考虑签名服务而非把私钥交给 Agent。

## 5. Agent 执行安全

- Worker 以非 root 用户运行；
- 不挂载 Docker Socket；
- 不默认挂载其他项目目录；
- Worktree 隔离代码修改；
- 不把 Harness 登录缓存复制进 Worktree；
- 不将认证信息写入日志；
- 对不可信代码启用更强隔离；
- 网络访问按任务风险限制；
- Issue、评论和外部文档都是不可信输入。

## 6. Policy Engine

Policy Engine 负责强制校验。Management Agent 可以提出建议，但不能扩大自身权限、批准自己的策略修改或绕过硬性规则。

审批绑定具体：
- Task/Goal；
- Plan Revision；
- 操作；
- 环境；
- Commit/Artifact；
- 有效期。

计划或操作实质变化时，原审批可能失效。

## 7. OpenBao 自身安全

- Root Token 初始化后撤销或严格离线管理；
- 解封材料离线保存；
- 数据和审计日志独立备份；
- 备份加密；
- 恢复演练；
- 不把解封密钥和业务数据放在同一位置。
