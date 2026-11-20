CREATE TABLE agent_skills (
    id bigserial PRIMARY KEY,
    name text NOT NULL,
    description text NOT NULL,
    instructions text NOT NULL,
    builtin boolean NOT NULL DEFAULT false,
    fingerprint bytea NOT NULL CHECK (octet_length(fingerprint)=32),
    UNIQUE (name, fingerprint)
);
WITH initial(name,description,instructions) AS (VALUES
    ('任务拆解','将需求拆成可独立验收的工作项。','先列出目标、交付物和已知约束。按依赖顺序拆分工作项，每项写明可观察的验收结果。把缺乏依据的假设单列为待确认项。'),
    ('代码审查','依据变更和调用路径核对行为。','沿修改过的入口追踪调用方、返回值和错误处理。逐项检查对既有行为的影响。报告问题时指出触发条件、证据位置和用户可见的后果。'),
    ('测试设计','覆盖正常路径、边界条件和失败恢复。','从验收条件推导测试场景。为每个场景给出前置条件、输入和可观察的预期结果。区分已经执行的检查和仍需执行的检查。'),
    ('项目状态汇总','按项目证据整理进度和待决事项。','先回答当前进展，再分别列出已完成事项、阻塞原因和待决事项。将结论对应到提供的任务、审查或测试记录，注明缺少证据之处。')
)
INSERT INTO agent_skills(name,description,instructions,builtin,fingerprint)
SELECT name,description,instructions,true,sha256(convert_to(description || chr(31) || instructions,'UTF8')) FROM initial;
