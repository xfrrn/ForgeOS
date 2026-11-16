# 全路线验收记录

状态按当前工作树的实测结果记录。下表中的历史运行可作交叉核对，真实环境要求仍需按当前版本复验。验收仓库的合并提交与 ForgeOS 实现提交分别记录。

| 路线要求 | 已有实现或证据 | 待补齐或待验收 |
| --- | --- | --- |
| 一 · 空库迁移、登记、派发、重启查询 | 2026-11-16 在独立 PostgreSQL 18 空库启动控制面，通过 HTTP 登记仓库、创建任务。独立 Worker 在 execution-2 工作区返回退出码 0 和仓库 HEAD `65d40b2a22dcec366c5146ed5924c607b09f7cbd`。重启控制面，任务 2 完整 JSON 相同，符合预期。 | Linux 专用身份与 systemd 见专题 07、14。 |
| 一 · 并发、失败、重报、失联 | 通过 HTTP 领取任务 1，再创建任务 2。第二次领取返回 204，任务 2 为 READY。回报受控失败后任务 1 为 FAILED，重报返回 204，任务和事件完全相同。PostgreSQL 集成测试覆盖过期租约及人工结束。 | 真实节点中断、在已有远端副作用下恢复见阶段四。 |
| 二 · Issue 识别、去重和执行 | beta 实库通过 `GET /projects/1/inbox` 得到普通讨论 [2,1]，任务列表为空。Worker 查询返回 401。Owner 两次转换 #2 均得任务 1，重启后收件箱只剩 #1。另一验收库为 beta #1 加模板标签后，Poller 创建任务 7，按编号同步仍返回同一任务。 | 通过。收件箱实现 `319aa65526b5e5e84fb04caca0300a939ee9a34c`。 |
| 二 · 验证、页面验收与退回 | alpha 任务 3 重复导入不变。在 Chrome 打开候选差异，退回 `93a7c42e`，真实 Codex 在新工作区补写示例，生成 `785df9cf`。两个候选各有对应 SHA 的 docs 验证。旧批准返回 409，新批准在页面返回 ACCEPTED。 | 通过。两次按 Owner 身份的页面操作均由 Playwright 执行。 |
| 二 · PR、必需 CI、Squash Merge、Issue 回写 | alpha 首次检查失败时 PR 为 OPEN/BLOCKED，任务交付为 WAITING。重跑通过后任务 DONE，Issue 已回写候选及合并 SHA `469cef06b81601ac2f382e230157c2e125787e39`。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/3) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/43815409052) | 通过。beta 再现同一场景，控制面明确指出已验收 head 的 GitHub 检查失败；停止控制面、重跑检查、重启，复用原 PR 合并为 `3b935429ad7588b2e6f4124ac8e4e04d6c80abb0`。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/3) · [检查](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43815783260) |
| 三 · Goal、Milestone、依赖与双重计划记录 | 在收件箱验收库将 beta #2 升级为 Goal 草案，保存两版计划。重报返回原版本，过时改写返回 409。重启后可读取历史，Chrome 展示任务树。beta #4 已走通依赖调度、业务答复、退回、新 SHA 验收、两个子 PR 和父 Issue 关闭，证据见下文。 | 显式 Owner 规划与激活已验收。只读 Planner 已验收（见任务 16）。Management 审核和激活后的滚动修订均在 alpha #4 验收，见下文专题 05–06。 |
| 三 · 决策、事件与恢复 | 子项 15 在验收边界提出业务问题 1，状态变为 WAITING_INPUT。重报只保留一个问题，直接批准返回 409。重启控制面后 Worker 回答返回 401。在 Chrome 以 Owner 回答后，恢复原执行 11、Session `01a1e315-16d2-7f31-8de3-7485865d1c95`、候选 `68d6097a` 和 WAITING_ACCEPTANCE。 | 已验收 Owner 发起和回答的验收检查点。角色分级、运行中 Harness 请求与恢复、需求修订和事件合并待实现与验收。 |
| 三 · 审查、验证规则与验收方法 | 已有 docs 和 go-test 两项预设验证。 | 只读 Review、V1–V4 风险判定、仓库规则、Playbook、测试计划待实现与验收。 |
| 三 · 预览、批注与知识 | 现有反馈可绑定执行和候选 SHA。 | Tailscale/Caddy、浏览器元素/截图/录屏、知识来源与决策索引待实现与验收。 |
| 四 · 多项目授权与密钥 | 两个项目有独立任务记录和项目范围内的 Worker 身份，实测跨节点回报与读取均被拒。 | 环境授权、OpenBao、按执行分发、脱敏共享、审计和恢复演练待实现与验收。 |
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

下列短 SHA 属于本地 ForgeOS 仓库。PR 内的 SHA 属于验收项目。历史检索版本 `a31c02ceb9ea5419f356d00aebb2989480a1a28b` 在专用 PostgreSQL 18 实例中启用数据库和 Docker 测试，`go test -count=1 -json ./...` 得到 63 项通过、零跳过，`go vet ./...` 通过。前端生产构建在 `bd1c734` 通过。

| 专题 | 已验证的入口和证据 | 实现 SHA | 仍待补齐或实测 |
| --- | --- | --- | --- |
| 00–01 产品与端到端流程 | 两个私有项目均从 Issue 到已合并 PR；复杂 Goal 13 从版本化计划到父 Issue 关闭 | `1a17822`、`32c0fa0`、`5447445` | 远程外部设备、知识归档、完整项目初始化 |
| 02–03 系统与模块 | 独立控制面和 Worker、PostgreSQL、同源 React 页面、只读规划和交付器均从实际入口运行 | `1a17822`、`cc2011f` | 分类、管理角色的协调职责、通知、节点资源管理、知识与预览服务 |
| 04 领域模型 | 同一任务关联历史需求版本、执行、计划修订、候选、验收、子 PR 和 Merge SHA | `a7f8b48`、`5d30bcf`、`94b6b08`、`70781ba` | 独立 Validation Run 和 Artifact 标识，以及完整 Session/Process 模型 |
| 05 角色与决策 | 任务 15 等待业务答复；Goal 16 的 Management 审核退回两版、批准第三版 | `70781ba`、`f8503c5` | 跨操作 D0–D3 策略、ACP 路由、批量答复 |
| 06 规划与编排 | 任务 16 从 Planner 草案、独立审核到运行中修订和两次子 PR 合并；Goal 13 显式激活 | `cc2011f`、`5d30bcf`、`83ea262`、`94b6b08`、`e981f07`、`f8503c5`、`f519840` | 运行时事件批处理和安全中断 |
| 07 Worker 与 Harness | Windows 原生 Codex 读写、只读规划、Session、真实失联租约、评论检查点；Ubuntu 非 root systemd 服务实测见下文 | `1a17822`、`cc2011f`、`b4e7c54`、`4d31e7f` | App Server、Claude Code 原生恢复、Linux Harness 登录复用、多节点恢复 |
| 08 项目知识 | Planner 在固定版本引用 README 和 CI 工程事实；Owner 和两个节点从真实项目历史检索需求与评论版本 | `cc2011f`、`a31c02c` | 导入资料、决策索引、语义检索、脱敏审批和共享副本 |
| 09 权限与密钥 | Owner/Worker 分权，两个 Worker 按项目领取，持另一节点有效租约的越权回报返回 403 | `1a17822`、`319aa65`、`70781ba`、`c513401` | 环境授权、OpenBao、Secret Broker、短期凭据与恢复 |
| 10 审查与验证 | 真实 docs 验证、离线 Docker Go 测试、失败的 Actions 门禁、候选变化后重新验证 | `1a17822`、`32c0fa0` | 只读审查、V1–V4、仓库验证 Schema、检查报告与 GitHub 状态回传 |
| 11 预览与人工验收 | Chrome 读取差异、按候选 SHA 退回或批准、回答问题 | `1a17822`、`70781ba` | Playbook、测试计划、Tailscale/Caddy、页面批注/截图/录屏；外部设备待验收 |
| 12 GitHub 接入与交付 | 标签派单、普通讨论转换、受保护分支阻塞与合并、子 PR 按序合并、来源需求修订与检查点换版、目标分支前移后重新验证与交付 | `319aa65`、`32c0fa0`、`94b6b08`、`5447445`、`a7f8b48`、`20c30e6`、`391309e`、`4d31e7f`、`e2912c5` | Webhook、细粒度凭据和第二人审查实测 |
| 13 数据与审计 | 数据库迁移、重启恢复、幂等事件、Git 计划快照和交付证据链 | `1a17822`、`83ea262`、`5447445` | 完整审计载荷、工件元数据/保留策略、Worker 离线缓冲 |
| 14 部署与运维 | Windows 主机、Docker Linux 引擎、Ubuntu systemd 服务重启和再次派发、PostgreSQL 恢复至空实例并逐项比对 | `1a17822`、`32c0fa0`、`bd1c734`，数据库证据见下文 | 私网访问、资源预算、OpenBao、工件和工作区备份恢复 |
| 15–16 路线与开放项 | 基座、单项目闭环、两子项 Goal 场景、第二个项目交付有以上记录 | 本表所列提交 | [开放事项 A–C](16-open-decisions.md)中其余真实环境与协议要求，以及全路线最终回归 |

## 专题 05–06 · Management 对计划修订的真实审核

在 alpha 私有项目启动控制面和独立的 `worker management`，使用原生 Codex、独立令牌与 PostgreSQL。普通 Worker 访问审核领取入口返回 401。[输入 Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/4)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 自动领取 Goal 16 的修订 1，沿用默认 D2 | 提供审核依据，等待 Owner | 会话 `01a1e32e-b07c-73a0-91f1-54337bd91c4a` 要求核实 README 与 CI 断言。状态 PLANNING，零子项 |
| 保存修订 2，并明确授权 D1 | 审核退回时保留草案 | 会话 `01a1e32f-1ec6-7612-b741-5a9a5160cdb5` 再次指出证据缺口，零子项。重启控制面后仍可读取历史审核 |
| 从固定提交 `469cef06b81601ac2f382e230157c2e125787e39` 核实两个文件，将摘录和检查步骤纳入修订 3 | D1 批准后派发独立项并阻塞依赖项 | 会话 `01a1e330-3dec-73d2-bb84-d017aa401852` 批准，Goal ACTIVE，一个 READY、一个 BLOCKED。Chrome 的任务详情显示 D1、依据和同一会话 ID |
| PostgreSQL 集成测试提交过期租约、旧版结果、跨项目结果、重复结果和试图降低 D3 的建议 | 过期和越权写入被拒；重放不重复派发，D3 留待 Owner | 全部通过。启用 PostgreSQL 与 Docker 的回归 52 项通过、零跳过；Go vet、前端生产构建通过 |

## 专题 06 · 已激活 Goal 的滚动修订

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在真实 Worker 执行任务 17 时替换其范围 | 拒绝变更，保留该执行 | HTTP 409。执行 14 继续，并以候选 `3583fb690a323a556a478dc3d46b146a4cb5606d` 通过 docs 验证 |
| 保存修订 4，更新未执行的任务 18，新增检查项 | 审核期间沿用修订 3；批准后按 key 调整依赖 | Goal 16 显示 PLANNING、active_revision=3、两个子项。批准后恢复 ACTIVE；17 仍属于修订 3，18 保留原 ID 并绑定修订 4，新项 19 等待 18 |
| 修订 5 移除暂列的检查项，重启控制面 | 当前计划反映移除，历史版本仍可读取 | 19 为 SUPERSEDED；修订 4 有三个里程碑、修订 5 有两个。18 的内容和创建记录仍指向修订 4 |
| 完成第一个子项 | 保留原执行，父 Issue 保持开放 | 任务 17 的 PR 已合并，Merge SHA 为 `1dea711e585b05dc1d6aa4a79cc9cf1c96e565ab`；任务 18 开始执行时父 Issue 仍为 OPEN。[PR #5](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/5) |
| 从旧的依赖检查返回结果、覆盖已交付子项、在关闭租约期间再提交计划 | 各次写入被拒，旧结果不能放行已改版任务 | PostgreSQL 集成测试通过，已交付子项的执行 ID 和 Merge SHA 保留。全套 PostgreSQL/Docker 回归 53 项通过、零跳过 |
| 完成修订后的任务 18 | 从前置 Merge SHA 开始验证，两个必需子项交付后关闭父 Issue | 执行 15 基于 `1dea711e585b05dc1d6aa4a79cc9cf1c96e565ab`，候选 `8ee909d7b313ca88cb3877ced22b44102ff69d65` 经 docs 验证与必需 CI 后合并为 `b179f04fec439a392af1fe1504450931f9f1f203`。Goal 16 为 DONE，Issue 已关闭，回写计数为 2。[PR #6](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/6) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/43823799864) · [父 Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/4) |

## 专题 04、12 · 来源需求修订与执行绑定

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从 GitHub 导入 alpha #7，然后在真实 Worker 执行时修改正文并重复导入 | 工作项保持唯一，修订 2 可读取，执行继续使用版本 1 | 任务 20，执行 16，绑定 Requirement ID 5。两个版本都能查到相应正文；候选 `114d73ef1e26ed33eaf2cc58b6b74fe5b56a03dc` 通过 docs 验证。[输入 Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/7) |
| 再改一次无派单标签的 Issue，启动真实轮询器，打开 Chrome 任务详情 | 自动观察修订 3，浏览新旧正文，既有候选保持待验收 | 页面显示三版，可从第 3 版回看第 2 版。原工作项仍为 WAITING_ACCEPTANCE，只有一次执行、两条需求变化事件 |
| 重启控制面，重放同步，查询既有子任务 18 的需求版本 | 没有新增执行或修订，既有子执行可追溯到原 Goal | 任务 20 保持最新版本 3、执行绑定 1；18 的执行与 Goal 16 原始需求对应同一个 ID |
本次 PostgreSQL 与 Docker 回归 54 项通过，零失败、零跳过。Go vet 和前端生产构建通过。


## 专题 06、12 · 在验收检查点采用新需求

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 Chrome 打开任务 20 的需求历史，阅读第 3 版并选择采用 | 保留工作项，重新入队；重复提交不产生第二次转换 | HTTP 200，任务进入 READY。再次采用后只有一个 `requirement_adopted` 事件 |
| 提交已过时的修订 2，或尝试批准修订 1 的旧执行和旧 SHA | 拒绝两次提交 | 两次均返回 409。PostgreSQL 测试还验证了运行期间拒绝更换需求，换版后不会带入旧版退回意见 |
| 启动真实 Worker，检查差异，再重启控制面 | 新执行绑定修订 3，旧版本仍可追溯 | 原执行 16 保留候选 `114d73ef1e26ed33eaf2cc58b6b74fe5b56a03dc`；新执行 17 产生 `c789c99feb7edd751f3de912796bb679099b6566`。差异包含修订 3 指定的完整句子，docs 验证通过，重启后两次执行均可查询。[验收 Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/7) |

## 专题 06、12 · GitHub 评论采集

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 向 alpha #7 添加两条普通评论，启动真实轮询器 | 保存来源与正文，运行状态保持原状 | 任务 20 出现事件 1、2，来源评论 ID 分别为 6910489666、6910490346。任务仍为 WAITING_ACCEPTANCE，执行 17、需求修订 3 均相同。[第一条评论](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/7#issuecomment-6910489666) |
| 编辑第二条，重启并再次扫描同一 Issue | 旧正文可追溯，重复项去重，游标可读取新版本 | 历史为事件 1、2、4。游标 2 仅返回事件 4，新旧正文均保留。另一个 Goal 查询为空，普通 Worker 读取 Owner 接口返回 401。[第二条评论](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/7#issuecomment-6910490346) |


## 专题 06、07、12 · 原生回合间的评论合并

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 退回任务 20 的候选，让旧 Worker 和新 Worker 分别领取 | 由支持检查点的 Worker 处理已有评论 | 旧 Worker 返回 204。新 Worker 开始执行 18，批次 0 截至事件 4，两条有效评论只包含各自最新正文 |
| 在第一个 Codex 回合进行期间向真实 Issue 添加第三条评论并轮询 | 当前命令继续；回合结束后才读取新信息 | 批次 0 在原生会话 `01a1e356-8989-7270-8a48-67fb7043678c` 完成。批次 1 处理事件 7，另一个原生会话 `01a1e357-2cda-7932-89c6-770526eeae30` 在同一工作区续做。[回合期间的原评论](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/7#issuecomment-6910610401) |
| 提交前再取批，在 Chrome 打开检查点和候选差异 | 零新增时提交；候选应覆盖修订 3 和讨论内的补充示例 | 批次 2 截至事件 7，零新增。页面显示三个批次的评论数 2、1、0。候选 `7daa15661a39d02287b2750a4c3936637278a802` 含修订 3 句子、`forge status` 和 `Waiting tasks remain visible.`，docs 验证通过 |
| 重放批次、跨租约读取、带着未处理评论提交成功结果 | 同一批次分页读取稳定，错租约和未处理完的回报被拒 | PostgreSQL 测试通过。启用 PostgreSQL 和 Docker 的全套回归 57 项通过、零跳过，Go vet 和前端生产构建通过 |
| 通过 Chrome 批准该候选并查看远端门禁 | 必需 CI 和分支保护分别判定 | PR #8 的必需 CI 重跑后成功，目标分支已前进，GitHub 状态为 BEHIND，交付保持 WAITING。新基线上的验证与验收另行记录。[PR #8](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/8) · [CI](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/43827169232) |


## 专题 10、12 · 严格分支保护下的新基线验收

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 查看 PR #8 检查与目标分支保护 | 已成功 CI 仍不能放行落后的候选 | `acceptance-check` 成功，保护配置 strict=true，GitHub 为 BEHIND，任务保持 WAITING |
| 在 Chrome 选择重新验证，再重放同一请求 | 原差异应用到最新目标，保留原验收记录，旧候选不能直接放行 | PR #8 已关闭且未合并。归档条数为 1，原执行 18 的差异仍可读取。新基线 `b179f04fec439a392af1fe1504450931f9f1f203`，新候选 `deb856d2e1e1b0c1e0102869998450c396899a5c`。[原 PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/8) |
| 让旧 Worker 领取，再启动具备验证能力的 Worker | 旧 Worker 返回 204；新 SHA 单独验证、重新人工验收 | 执行 19 检出固定 SHA，docs 检查通过，未启动模型。Chrome 同屏查看历史交付与新差异，重新核对三项内容后按新 SHA 批准 |
| 在新 PR 上运行必需 CI 并回写 | 新候选获得独立远端检查和合并记录 | 新 PR #9 的必需检查先失败，按同一 head 重跑通过，Merge SHA 为 `15ebbfc2dc682075de1e8c9d96f42346a00a1c88`。任务 20 为 DONE，Issue #7 为 CLOSED。[PR #9](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/9) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/43828534029) · [Issue #7](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/7) |


## 专题 09、14、15 · 多 Worker 项目授权

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 为 alpha-node 授权项目 2，beta-node 授权项目 4，先排 beta 再排 alpha | 凭据和 principal 匹配，只能领取允许项目的工作 | alpha 以 beta 名称领取返回 403。改回本名后领取 alpha 的工作，beta 的较早任务仍在队列；alpha 再领取返回 204 |
| 将上述执行的有效租约与 beta 凭据组合，读取检查点或回报结果 | 跨身份请求应被拒，原身份可正常回报 | 两个跨节点请求均为 403，原节点回报受控失败返回 204 |
| 分别启动两个真实 Daemon，配置各自项目仓库、独立工作区及秘密令牌 | 查到与项目、节点、执行和实际检出 SHA 一致的记录 | beta 完成任务 24、执行 21，返回 `3b935429ad7588b2e6f4124ac8e4e04d6c80abb0`，期间 alpha 工作仍为 READY。alpha 随后完成任务 26、执行 22，返回 `469cef06b81601ac2f382e230157c2e125787e39`，均与各自 Git HEAD 一致 |

## 专题 07、14 · Ubuntu systemd 服务与重启

在 WSL Ubuntu 的 systemd 系统实例安装本仓库的两个 unit，使用 Linux amd64 二进制、独立 PostgreSQL schema 和本地 Git 项目。验收完成后清理临时服务、账号、工作区和 schema。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 校验并启动 unit，检查 MainPID、UID、HOME、工作目录和 Worker 环境 | 以不同非 root 用户运行，HOME 权限为 0700，Worker 只持节点凭据 | 控制面与 Worker 为 UID 997、995。两者工作目录均为各自 HOME，代理绕过回环地址，Worker 进程环境中无 Owner 令牌和数据库连接 |
| 在控制面所有的组共享克隆上派发 git_head，随后给 Worker 配置该克隆的精确路径 | 默认 Git 所有权检查生效，显式授权后节点完成真实检出 | 原任务 1 失败，Git 报 dubious ownership。授权后任务 2 为 DONE，工作区 `/var/lib/forgeos-worker/work/execution-2` 归 Worker 所有，结果与 HEAD `9b09c97060aeec20b3779a5db346f89c64292fa2` 相同 |
| systemctl restart 两个服务，通过 Owner API 重新查询并派发新任务 | 原记录完整，新进程可以继续工作 | PID 从 2249、2250 变为 2283、2284。任务 2 的完整响应前后一致；任务 3 由新 Worker 完成并返回同一 SHA |

该版本启用 PostgreSQL 和 Docker 的完整 Go 回归为 61 项通过、零跳过。`go vet ./...` 与前端生产构建通过。Linux 专用用户下的 Codex 和 Claude 登录与会话恢复仍待验收。

## 专题 13–14 · 从 PostgreSQL 备份恢复到全新实例

使用 ForgeOS `bd1c734d80413c87e032cb250c12baea9ad314cc` 与 PostgreSQL 18。源库包含两个真实 GitHub 项目的交付历史。以下演练未启用轮询、Worker 或交付器。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 用 pg_dump 创建 custom 格式备份，列出归档目录，再 initdb 启动独立实例 | 能读取备份，目标库中尚无应用表 | 备份 60,275 字节，SHA-256 为 `380414258a6f3b6232c29deca6445584ffb95cc78be33e1e62803cd955e8351b`。恢复前目标 public schema 表数为 0 |
| pg_restore 单事务恢复，连接目标库启动控制面 | 数据结构、事件和可读历史与源库一致 | 全部 15 张应用表计数相同，包括 19 项任务、22 次执行、120 个事件和 7 次交付。逐字段比较 24 个 Owner API 响应均相同，覆盖全部任务详情、列表、任务 20 的评论和检查点、需求历史及 Goal 16 的计划修订 |
| 核对已合并交付，再从恢复控制面新建任务 | 显示原先合并的候选，恢复库可继续写入 | 任务 20 保留 PR #9、候选 `deb856d2e1e1b0c1e0102869998450c396899a5c` 和合并 SHA `15ebbfc2dc682075de1e8c9d96f42346a00a1c88`。新任务 27 为 READY，在源库查询该 ID 为 0 行 |

PostgreSQL 恢复演练对应提交 `06ed00b8e47fa764e31611b655fbc5ca9e7fa2d1`。

## 专题 08 · 按项目检索历史来源

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 以 Owner 和 alpha 节点凭据分别查项目 2，参数为 `source=comment&task_id=20&q=requirement+revision` | 只检索该项目的源记录，两种授权入口返回一致结果，可区分原文和修订 | 两个响应逐字段相同。[同一条真实评论](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/7#issuecomment-6910490346) 返回事件 4（latest）和事件 2（historical），均带时间、引用与各版本原文摘要 |
| 以 beta 节点查本项目相同评论，再搜索 `source=requirement&q=queue` | 根据来源和项目筛选 | 评论查询返回空数组；需求查询仅返回项目 4 的任务 7 和 13 |
| 交换两个节点的目标项目，再尝试无效来源和负任务编号 | 授权失败时拒绝访问，参数错误可区分 | 两个跨项目请求均为 403；两个非法参数均为 400。数据库测试另以同一关键词和中文子串验证项目隔离与旧版本匹配 |

## 专题 07、16 A.2 · Claude Code 原生权限交互与恢复

在 Windows 已登录账号执行 `python scripts/probe_claude.py --bin <claude.exe>`。CLI 报告版本 2.1.310，使用原生 stream-json 双向通信。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 用进程 13936 启动会话，等待 AskUserQuestion 并通过相同 request_id 回答 | 工具交互到达宿主，模型得到选择的答案 | 收到 can_use_tool 请求，宿主选择 Green 并回传；同一 Session ID 为 `90231784-7dea-4ee8-994f-552af586ab52` |
| 拒绝对临时目录 probe.txt 的 Write 请求 | 调用受权限门禁控制，文件未落盘 | 宿主返回 deny；任务结束后实际检查文件不存在 |
| 退出进程，使用 --resume 在新进程 44392 恢复 | 新进程返回相同 Session ID，并能同时回忆第一轮随机标识和已选答案 | 第二次结果与随机标识和 Green 完全匹配。此项覆盖协议探测，Worker 自动恢复、Linux 登录与并发仍需独立验收 |

Claude Code 原生探测对应提交 `1d9617e11095434c41f68956f6e8a0d45c014108`。

## 专题 07、16 A.1 · Codex App Server 线程与中断

在 Windows 本机从安装的 `codex-cli 0.169.0-alpha.11.1` 生成协议 schema，运行 `python scripts/probe_codex_app_server.py --bin <codex.exe>`。两个服务器均使用 stdio。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 初始化、新建只读 Thread，发送携带随机标识的文本，关联原生事件 | 服务确认只读和 never 审批，首轮正常完成 | PID 1580 建立线程 `01a1e385-6183-7192-8b93-23dee8be39b2`。首轮 `01a1e385-63fe-7372-927d-1b0f9de0850b` 收到 started、agentMessage 和 completed |
| 关闭服务器，在新服务器按原 Thread ID 恢复，上下文不重发 | 返回同一线程，模型能答出上一进程提供的标识 | PID 47960 的轮次 `01a1e385-8d3d-7fb3-8ca3-b5526ebdd9cd` 返回完全相同的随机值，服务确认轮次完成 |
| 对第三轮发送中断，核对同一 Thread/Turn 的最终状态 | 运行中的 Turn 变为 interrupted | 在 start 响应后立即中断曾得到协议错误 -32600。改在收到 turn/started 后发送，轮次 `01a1e385-b2b7-7c41-83b9-f563f4d56966` 的 turn/completed 状态为 interrupted |
