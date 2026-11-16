# ADR-009 — Harness 统一账号认证与多进程复用

状态：已确认

每台 Worker 使用一个专用非 root Linux 用户。用户在该身份下分别登录 Codex、Claude Code 一次，后续任务复用现有认证状态。Daemon 以同一身份运行。默认不按项目或任务创建独立账号。项目业务密钥仍由 OpenBao 管理。
