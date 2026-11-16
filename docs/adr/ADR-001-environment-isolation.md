# ADR-001 — 环境凭据隔离

状态：已确认

不同环境使用独立实际凭据：Local Mock、Local Integrated、CI/Test、Staging、Production。开发与生产密钥绝不共用。项目声明需求不等于授予权限。生产凭据不参与普通初始化和 Agent 开发。
