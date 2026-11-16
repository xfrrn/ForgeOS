# ADR-010 — 执行进程与 Agent 会话生命周期分离

状态：已确认

Worker Daemon 长期运行；Harness Runtime 按适配器决定驻留或按需启动；Session 尽可能持久化；Active Turn 按需运行。等待人工输入时保存检查点并释放活动模型执行。恢复尽可能沿用原生 Session。
