# 全路线验收记录

状态按当前工作树的实测结果记录。下表中的历史运行可作交叉核对，真实环境要求仍需按当前版本复验。验收仓库的合并提交与 ForgeOS 实现提交分别记录。

| 路线要求 | 已有实现或证据 | 待补齐或待验收 |
| --- | --- | --- |
| 一 · 空库迁移、登记、派发、重启查询 | 2026-11-16 在独立 PostgreSQL 18 空库启动控制面，通过 HTTP 登记仓库、创建任务。独立 Worker 在 execution-2 工作区返回退出码 0 和仓库 HEAD `65d40b2a22dcec366c5146ed5924c607b09f7cbd`。重启控制面，任务 2 完整 JSON 相同，符合预期。 | Linux 专用身份与 systemd 见专题 07、14。 |
| 一 · 并发、失败、重报、失联 | 通过 HTTP 领取任务 1，再创建任务 2。第二次领取返回 204，任务 2 为 READY。回报受控失败后任务 1 为 FAILED，重报返回 204，任务和事件完全相同。PostgreSQL 集成测试覆盖过期租约及人工结束。 | 真实节点中断、在已有远端副作用下恢复见阶段四。 |
| 二 · Issue 识别、去重和执行 | beta 实库通过 `GET /projects/1/inbox` 得到普通讨论 [2,1]，任务列表为空。Worker 查询返回 401。Owner 两次转换 #2 均得任务 1，重启后收件箱只剩 #1。另一验收库为 beta #1 加模板标签后，Poller 创建任务 7，按编号同步仍返回同一任务。 | 通过。收件箱实现 `319aa65526b5e5e84fb04caca0300a939ee9a34c`。 |
| 二 · 验证、页面验收与退回 | alpha 任务 3 重复导入不变。在 Chrome 打开候选差异，退回 `93a7c42e`，真实 Codex 在新工作区补写示例，生成 `785df9cf`。两个候选各有对应 SHA 的 docs 验证。旧批准返回 409，新批准在页面返回 ACCEPTED。 | 通过。两次按 Owner 身份的页面操作均由 Playwright 执行。 |
| 二 · PR、必需 CI、Squash Merge、Issue 回写 | alpha 首次检查失败时 PR 为 OPEN/BLOCKED，任务交付为 WAITING。重跑通过后任务 DONE，Issue 已回写候选及合并 SHA `469cef06b81601ac2f382e230157c2e125787e39`。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/3) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/43815409052) | 通过。beta 再现同一场景，控制面明确指出已验收 head 的 GitHub 检查失败；停止控制面、重跑检查、重启，复用原 PR 合并为 `3b935429ad7588b2e6f4124ac8e4e04d6c80abb0`。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/3) · [检查](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43815783260) |
| 三 · Goal、Milestone、依赖与双重计划记录 | 在收件箱验收库将 beta #2 升级为 Goal 草案，保存两版计划。重报返回原版本，过时改写返回 409。重启后可读取历史，Chrome 展示任务树。beta #4 已走通依赖调度、业务答复、退回、新 SHA 验收、两个子 PR 和父 Issue 关闭，证据见下文。 | 显式 Owner 规划与激活已验收。只读 Planner 已验收（见任务 16）。Management 审核、激活后的滚动重规划待实现与验收。 |
| 三 · 决策、事件与恢复 | 子项 15 在验收边界提出业务问题 1，状态变为 WAITING_INPUT。重报只保留一个问题，直接批准返回 409。重启控制面后 Worker 回答返回 401。在 Chrome 以 Owner 回答后，恢复原执行 11、Session `01a1e315-16d2-7f31-8de3-7485865d1c95`、候选 `68d6097a` 和 WAITING_ACCEPTANCE。 | 已验收 Owner 发起和回答的验收检查点。角色分级、运行中 Harness 请求与恢复、需求修订和事件合并待实现与验收。 |
| 三 · 审查、验证规则与验收方法 | 已有 docs 和 go-test 两项预设验证。 | 只读 Review、V1–V4 风险判定、仓库规则、Playbook、测试计划待实现与验收。 |
| 三 · 预览、批注与知识 | 现有反馈可绑定执行和候选 SHA。 | Tailscale/Caddy、浏览器元素/截图/录屏、知识来源与决策索引待实现与验收。 |
| 四 · 多项目授权与密钥 | 项目有独立任务记录，目前共用 Owner 和 Worker 身份。 | 项目/环境授权、OpenBao、按执行分发、脱敏共享、审计和恢复演练待实现与验收。 |
| 四 · 多 Worker、多 Harness 与运维 | 两个真实项目各交付了一个工作项，beta 在控制面停止后重启，复用既有 PR 完成回写。 | 另一种 Harness 或节点、资源调度、能力注册、备份、压力与节点失联验收待完成。 |

MVP 页面验收与 GitHub 门禁证据对应基线、收件箱和交付诊断提交，最后一项为 `32c0fa07cc09eaf98750e588682d0f9701dd92f4`。两个私有项目均已交付。它们共用本机 GitHub 身份，跨项目业务凭据隔离保持待验收。

阶段三计划草案实现为 `5d30bcf451d838fae10d684d5c6bd718f5a4f73a`。在 beta 克隆通过 Owner 归档接口分别重复提交修订 1、2，各自返回相同 SHA。Git 快照分别为 `4903725d96065d014e8d4437d6d50b4b71d3bdf5` 和 `933478ea8f00aa0c71797386dc52ec99acd0f0b4`。用 `git show` 读取文件逐字段比对 API 版本成功，工作区状态与 HEAD 前后相同。回归测试还验证了暂存内容保留和引用冲突。

基线回归使用独立 PostgreSQL 测试库执行 `go test -count=1 -json ./...`。44 项测试通过，容器测试因未启用而跳过。前端执行 `npm --prefix web ci` 和 `npm --prefix web run build` 成功。专题待实测与待细化事项详见[环境和协议清单](16-open-decisions.md)。

基座验收对应 ForgeOS 提交 `1a1782246225ab810ca34f3a5c1fd73194242ab8`。随后启动本机 Docker Linux 引擎，启用 `FORGEOS_TEST_DOCKER=1` 单独运行 `TestGoValidationInLocalContainer`，真实容器中的成功与失败案例均符合预期（27.45 秒）。

归档功能对应 `83ea26272aebbb001a4ef0a521075fc0580a1e52`。该版本启用独立 PostgreSQL 和 Docker 后运行全套 Go 测试，48 项通过。另以 HTTP Worker 身份领取任务 11（执行 8）后断开，重启控制面、实际等待两分钟租约到期，原执行仍可查询，尝试领取返回 204；Owner 核对并结束执行后才派发任务 12。

阶段三在 beta 的真实 Issue #4 将修订 1 连续激活两次，只得到子项 14 和 15。Chrome 任务树展示 15 等待 14，并能从子项返回父项 13。子项 14 经 Codex、docs 验证、页面按 SHA 批准后，PR #5 合并为 `e45f9066d9865c64d0702d00320df0f39d5d63ee`。GitHub 查询显示父 Issue 为 OPEN，评论已包含子项 14 的候选和合并 SHA。[子 PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/5) · [父 Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/4)

外部设备通过 Tailscale 的验收保持待验收。2026-11-16 用户确认当前没有可用 tailnet 和第二台验收设备。

Goal 激活与首项交付对应 `94b6b0845c7bbb370701110c1ab2f841fffdc953`。PR #5 合并后启动新版交付器，子项 15 从 BLOCKED 转为 READY，以前置合并提交 `e45f9066d9865c64d0702d00320df0f39d5d63ee` 为基础。通过 Git 对象查询确认其中已有 `docs/status.md`，Worker 随后领取任务 15。数据库回归覆盖未交付时保持阻塞、回写后冻结新基础和实际领取。

依赖释放实现为 `e981f071a0a860fb4b88c0f86e21497b75936c3f`，业务问题检查点为 `70781ba9badb584e94c9594954bcfbcdae2ddbda`。在页面回答后退回子项 15，要求补写恢复条件。第二次执行通过 docs 验证，在 Chrome 确认新增文案并批准 `8dccb433c7cf630c011071b2e58dddb7b7868611`。PR #6 合并为 `5ab5f7f71c697aead887c16438ebb3818bb70fe9`。API 返回父项 13 和两个子项全部 DONE，GitHub 返回父 Issue CLOSED，Goal 完成摘要仅一条。[第二个子 PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/6) · [父 Issue 的两次交付与完成摘要](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/4)

父 Issue 关闭实现对应 `544744520aeb6ea13724c7aaf4166ebf0a5c09b0`。检查规划和派发的并发边界时，用真实 PostgreSQL 锁住 READY → PLANNING 的状态更新，让 Worker 请求在写入时等待。旧实现会在锁释放后把父任务误领走。修复后连续五次数据库回归通过；独立控制面的 `POST /internal/claim` 在锁释放后返回 204，任务保留 PLANNING，执行次数为零。

派发竞态修复为 `b4e7c54b06726fd8949b20faa052f75a0aeb8d1a`。alpha #4 通过按编号导入入口选择规划模式，旧 Worker 的领取请求返回 204。真实 Codex CLI 0.169.0-alpha.11.1 在只读沙箱中完成任务 16，Session 为 `01a1e322-fda7-7751-903d-93519a6cae56`。修订 1 包含两项工作、README 和 CI 上下文、依赖及验收条件。基础 SHA 与导入时一致，Git 工作区干净，草案未自动派发子任务；重启控制面后任务 JSON 相同。[规划输入](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/4)

## 专题覆盖与实现提交

下列短 SHA 属于本地 ForgeOS 仓库。PR 内的 SHA 属于验收项目。当前回归版本为 `cc2011fc18e85937c031086d119659731a721056`。在专用 PostgreSQL 18 实例中启用数据库和 Docker 测试，`go test -count=1 -json ./...` 得到 51 项通过、零跳过；`go vet ./...` 与前端生产构建通过。

| 专题 | 已验证的入口和证据 | 实现 SHA | 仍待补齐或实测 |
| --- | --- | --- | --- |
| 00–01 产品与端到端流程 | 两个私有项目均从 Issue 到已合并 PR；复杂 Goal 13 从版本化计划到父 Issue 关闭 | `1a17822`、`32c0fa0`、`5447445` | 远程外部设备、知识归档、完整项目初始化 |
| 02–03 系统与模块 | 独立控制面和 Worker、PostgreSQL、同源 React 页面、只读规划和交付器均从实际入口运行 | `1a17822`、`cc2011f` | 分类与管理角色、通知、节点资源管理、知识与预览服务 |
| 04 领域模型 | 同一任务关联历史执行、计划修订、候选、验收、子 PR 和 Merge SHA | `5d30bcf`、`94b6b08`、`70781ba` | 独立 Requirement Revision、Validation Run 和 Artifact 标识，以及完整 Session/Process 模型 |
| 05 角色与决策 | 任务 15 的候选在等待业务答复期间拒绝批准；Owner 回答与重报有留痕 | `70781ba` | Management 审核与 D0–D3 策略、ACP 路由、批量答复 |
| 06 规划与编排 | 任务 16 自动生成草案，Goal 13 显式激活；后继项从实际合并版本执行 | `cc2011f`、`5d30bcf`、`83ea262`、`94b6b08`、`e981f07` | 已激活计划滚动修订、管理角色审核、运行时事件批处理和安全中断 |
| 07 Worker 与 Harness | Windows 原生 Codex 读写、只读规划、Session ID、工作区和真实失联租约验证 | `1a17822`、`cc2011f`、`b4e7c54` | App Server、Claude Code 原生恢复、Linux 专用用户和 systemd、多节点恢复 |
| 08 项目知识 | Planner 在固定版本引用 README 和 CI 工程事实 | `cc2011f` | 私有知识存储、检索 API、决策权威性、脱敏审批和共享副本 |
| 09 权限与密钥 | Owner/Worker 分权，Worker 回答业务问题或读取 Owner 收件箱返回 401 | `1a17822`、`319aa65`、`70781ba` | 项目及环境授权、OpenBao、Secret Broker、短期凭据与恢复 |
| 10 审查与验证 | 真实 docs 验证、离线 Docker Go 测试、失败的 Actions 门禁、候选变化后重新验证 | `1a17822`、`32c0fa0` | 只读审查、V1–V4、仓库验证 Schema、检查报告与 GitHub 状态回传 |
| 11 预览与人工验收 | Chrome 读取差异、按候选 SHA 退回或批准、回答问题 | `1a17822`、`70781ba` | Playbook、测试计划、Tailscale/Caddy、页面批注/截图/录屏；外部设备待验收 |
| 12 GitHub 接入与交付 | 标签派单、普通讨论转换、受保护分支阻塞与合并、子 PR 不提前关闭父 Issue | `319aa65`、`32c0fa0`、`94b6b08`、`5447445` | Issue 修改/评论进入事件队列、Webhook、细粒度凭据和第二人审查实测 |
| 13 数据与审计 | 数据库迁移、重启恢复、幂等事件、Git 计划快照和交付证据链 | `1a17822`、`83ea262`、`5447445` | 完整审计载荷、工件元数据/保留策略、Worker 离线缓冲 |
| 14 部署与运维 | Windows 主机、独立 PostgreSQL、Docker Linux 引擎和控制面重启实测 | `1a17822`、`32c0fa0` | Linux 常驻服务、私网访问、资源预算、数据库/OpenBao/工件备份恢复演练 |
| 15–16 路线与开放项 | 基座、单项目闭环、两子项 Goal 场景、第二个项目交付有以上记录 | 本表所列提交 | [开放事项 A–C](16-open-decisions.md)中其余真实环境与协议要求，以及全路线最终回归 |
