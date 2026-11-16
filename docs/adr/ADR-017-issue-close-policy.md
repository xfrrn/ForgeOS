# ADR-017 — 基于任务层级的 Issue 自动关闭

状态：已确认

简单 Work Item 对应 PR 合并后可关闭 Agent Issue。复杂 Goal 的所有必需 Work Item/Milestone 完成后才关闭父 Issue。普通讨论不自动关闭。失败或取消保留 Issue 并反馈原因。子 PR 不得提前关闭父 Issue。
