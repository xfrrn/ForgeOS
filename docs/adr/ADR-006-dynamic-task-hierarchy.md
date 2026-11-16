# ADR-006 — 动态任务层级

状态：已确认

简单任务直接创建 Work Item；复杂任务使用 Goal、Milestone、Work Item。执行过程中发现任务复杂度超出预期时，可以升级为 Goal，保留原任务和执行历史。
