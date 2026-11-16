# ADR-007 — 任务规划与执行全过程可追溯

状态：已确认

Git 负责代码和项目文档版本追溯；中台负责运行过程事件记录。通过 Task ID、Plan Revision、Execution ID、Commit SHA、Validation ID、Acceptance ID、PR 和 Merge SHA 关联。Git 回滚不等于回滚中台事件历史。
