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

### Secret Manager 元数据和状态

控制面使用 `FORGEOS_BAO_ADDR` 指定 HTTPS origin，`FORGEOS_BAO_METADATA_TOKEN_FILE` 指定只读身份的令牌文件绝对路径，私有 CA 使用 `FORGEOS_BAO_CA_FILE`。令牌文件由控制面独占读取，可原子替换。身份策略只授予已配置项目和环境的 KV v2 **metadata** read 权限。以下为项目 2 单个测试键的示例。

```hcl
path "secret/metadata/projects/2/local-mock/example" {
  capabilities = ["read"]
}
```

Owner 通过 `PUT /projects/{id}/environments/{environment}/secrets/{name}` 登记 `{"risk":"S1"}`。环境标识为 `local-mock`、`local-integrated`、`ci-test`、`staging`、`production`。风险独立选择 S0–S3。PostgreSQL 只记录项目、环境、名称、风险和更新时间。实际值由具有相应权限的操作者写入 OpenBao KV v2。

在同一路径执行 GET 可读取 `state` 和当前 `version`。可用状态为 MISSING、AVAILABLE、DELETED 和 DESTROYED。未登记返回 404；OpenBao 不可连接、拒绝身份或返回无效数据时返回 503。节点使用带 `/internal` 前缀的 GET 入口，需额外取得项目与环境的成对授权。这一接口只查询 KV 元数据。

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

### 绑定执行的 Secret Broker

除元数据身份外，控制面通过 `FORGEOS_BAO_BROKER_TOKEN_FILE` 读取独立的 Broker 令牌。为每个允许分发的 KV 数据路径配置 read，并让 OpenBao 强制响应包装，最短 1 秒、最长 60 秒。下面的策略仅允许读取指定测试键的包装响应。

```hcl
path "secret/data/projects/2/local-mock/example" {
  capabilities     = ["read"]
  min_wrapping_ttl = "1s"
  max_wrapping_ttl = "60s"
}
```

1. 具名节点领取任务后，Owner 向 `POST /projects/{id}/secret-grants` 提交 `execution_id`、`principal`、`environment`、`name`、`version`、`purpose`、`expires_at`、`request_key`，以及可选的正整数 `max_uses`。版本须为当前可用版本。获批记录绑定该任务、节点、密钥元数据版本和确切 KV 路径。production 和 S3 的原始凭据不经此接口分发。
2. 受信执行器以原节点身份向 `POST /internal/projects/{id}/secret-grants/{grant}/use` 提交 `execution_id`、`lease_token`、`environment`、`name`、`purpose` 和本次唯一的 `request_key`。成功响应携带 `wrapping_token` 和 `expires_at`。在 OpenBao 的 `POST /v1/sys/wrapping/unwrap` 使用该令牌，解包仅能成功一次。将解包值保留在获准操作的进程内。模型提示、Worktree、普通 CI 和任务结果均不需要这些令牌。
3. 包装令牌不超过 60 秒，也不能超出授权或执行租约的截止时间。服务逐次检查执行状态、身份、用途、环境权限、风险及秘密版本，串行处理同一授权的并发领取。校验失败不消耗次数。已交付的包装令牌有自己的到期时间；撤销、结束任务或调整风险会阻止后续领取。
4. Owner 通过 `GET /projects/{id}/secret-grants/{grant}` 核对 ISSUED、FAILED 和对应请求键，或 `POST /projects/{id}/secret-grants/{grant}/revoke` 撤销。响应丢失时先核对回执。已记录的使用请求不会重发令牌。OpenBao 故障记录 FAILED，修复后可使用新请求键重试。数据库仅记录操作和包装有效期。

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
