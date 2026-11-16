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
| 三 · 审查、验证规则与验收方法 | alpha #10 经独立审查后交付。beta #12 经风险分级后交付。Goal 34 的 Management 审核将子项提升至 V4。beta #17 从全局方法生成测试计划、澄清并复审，两项用例实测通过后交付。 | 通过上述候选场景。预览和反馈见下一行。 |
| 三 · 预览、批注与知识 | 本机 Chrome 验证了固定候选的静态站点、重启恢复、停止和到期；现有验收反馈可绑定执行与候选 SHA。 | 动态应用、独立 Staging、私网入口、浏览器元素/截图/录屏、知识来源与决策索引待实现或验收。 |
| 四 · 多项目授权与密钥 | 节点按项目和环境读取 OpenBao 状态。Broker 在执行租约、用途、版本和 TTL 满足时分发一次性包装响应；脱敏共享和密钥恢复也已实测。 | 生产专用操作、执行资源调度和跨节点恢复见专题 07、09、14。 |
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
| 08 项目知识 | 固定版本工程事实、来源历史检索、项目决策、私有资料修订和跨项目脱敏审批均见下文验收记录 | `cc2011f`、`a31c02c`、`5225e60`、`7ae7254`，共享证据见下文 | 语义检索、全局规则、Curator 其余触发条件和角色上下文 |
| 09 权限与密钥 | Owner/Worker 分权、项目和环境成对授权、OpenBao 真实部署与隔离恢复、Go 控制面只查元数据 | `1a17822`、`319aa65`、`70781ba`、`c513401`、`ab013b6`，状态接口证据见下文 | Secret Broker、按执行受控使用与业务审计 |
| 10 审查与验证 | 真实 Docker V1–V4 检查、GitHub Status、Actions 门禁、重新验证、独立审查和页面验收。Issue 强制风险等级，Management 可提高下限 | `1a17822`、`32c0fa0`、`85cd1cb`、`9400068`、`427bc24`、`35866ba`、`ee695b3`、`80836d6`、`749351c`、`fdc4192`、`48a32f2`，Management 提级见下文 | 运行管理和归档保留策略 |
| 11 预览与人工验收 | Chrome 核对候选、退回、批准与答疑；工作项归档测试计划、审查修订和逐次实测；已验收本机静态预览生命周期。 | `1a17822`、`70781ba`、`fdc4192`、`86aff28`、`7565fa6`、`e71b7c8`、`978d184`、`71e6c9a`，预览实测见下文 | 动态应用预览、独立 Staging、Tailscale/Caddy、页面批注/截图/录屏与反馈分类；外部设备待验收 |
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

上述 App Server 协议验收对应提交 `fe35218b32cf0bb296173ef0bc18b9a8a7e17d4a`。

## 专题 08、ADR-018 · 有明确确认和依据的项目决策索引

在已有真实项目和交付记录的 PostgreSQL 18 实例上升级迁移，从控制面 HTTP API 执行以下操作。提案依据为 beta 任务 15 的第 1 个业务问题及其交付记录。[依据 PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/6)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 提交有依据和适用范围的提案，重报后查询默认索引 | 提案唯一，Owner 确认前不进入有效决策集 | 提案 1 为 PROPOSED，decided_by 为空。相同请求返回相同提案，改写同一 key 返回 409。Worker 尝试确认返回 401 |
| 通过 Owner 入口确认，再用两个节点读取；尝试从其他项目引用旧决定 | 只有获准项目的节点可读，确认身份可追溯 | beta 可读，返回 decided_by=owner；alpha 查 beta 返回 403。在项目 2 创建替代 beta 决策的提案返回 404 |
| 提案 5 替代决定 1；再尝试确认另一份指向决定 1 的提案 6 | 替代关系仅在确认时生效，原决定只能被替代一次 | 提案 5 创建后决定 1 仍然有效。确认 5 后，1 的历史依次为 PROPOSED、CONFIRMED、SUPERSEDED。确认 6 返回 409，Owner 可将 6 退为 REJECTED |
| 将决定 5 置为 EXPIRED，重启控制面并按状态查询 | 失效决定退出默认索引，历史和依据完整保留 | 重启前后 Owner 的完整历史响应相同，beta 节点读取结果也一致。真实数据库测试另覆盖两个替代提案并发确认和截止时间自动失效 |

该功能启用 PostgreSQL 与 Docker 的全套 Go 回归为 64 项通过、零跳过。`go vet ./...` 通过。

决策索引实现对应 `5225e601300177bc0a768fbc5d144c3a72c1e4f0`。

## 专题 08 · 私有项目资料导入与版本查询

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 用 Owner PUT 入口将 UTF-8 运维记录导入 alpha，在 beta 使用相同 key 导入不同内容 | 两项目分别保留原文与校验值 | alpha 的 `handbook-ops-89ebca61` 修订 1 与本地文件逐字节计算的 SHA-256 一致，为 `34c5f2dd73bcc3ecafdaa5ff211cd5ed2498cfa3fdbae0dc42f06a10300b3d76`。beta 节点在本项目搜索 alpha 独有词返回空 |
| 导入项目档案、环境说明，以项目节点按类型和词查询 | 可以分别获取本项目的两类资料，节点仅有读权限 | alpha 查询命中对应的 profile 和 environment。交换目标项目后的两个读取请求均返回 403；节点尝试写入返回 401 |
| 发布修订 2，再重发原始请求、尝试改写和跳号 | 新版本用于检索，旧版本可按固定引用读取，冲突写入被拒 | 修订 2 的 SHA-256 为 `30175a8b30190f4c80cf9daa809133750b54c7dae41d9086ac1dbf20bec17b5b`。重报原始请求仍返回修订 1；改写与跳号均为 409。控制面重启后两个版本的完整 JSON 与重启前相同 |

资料导入实现对应 `7ae7254394ae1f6bd4279b2314a4f9e035ac38a2`。

## 专题 08、09 · 从 alpha 向 beta 发布脱敏副本

在控制面、PostgreSQL 和真实 `worker management` 上，以 alpha 资料 `handbook-ops-89ebca61` 的修订 1 为来源。beta 的节点只获准查询项目 4。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 尝试复制整个原文，然后提交含测试用私有标识的摘要 | 全文复制被拒，保留私有标识的摘要不能发布 | 全文返回 409。副本 1 的原生 Management 会话 `01a1e393-f0bc-7bc1-8836-171ecb259479` 要求修改。Owner 尝试发布返回 409 |
| 提交通用操作经验供独立审核，核对发布前的接收项目 | 有依据的脱敏副本可以通过内容审核，随后须经 Owner 显式批准 | 副本 2 返回 needs_owner，保持未发布。澄清两种审批的职责并另交副本 5 后，Management 会话 `01a1e395-0306-71e3-89ce-dde46eb05fab` 判为 approve。beta 仍查不到副本；错误摘要发布返回 409，Worker 发布返回 401 |
| Owner 按审核摘要发布，重报并用 beta 凭据查询 | 接收方获得被批准的副本，源资料和审核内容维持私有 | 重报复用相同发布时间。beta 只收到 ID、标题、正文、发布时间和摘要 `8e096f1715403fecc316a78594d596acb452279b966754632d4c904963b0d764`。直读 alpha 私有资料返回 403，私有审核入口返回 401 |
| 重启后重新查询，再按原摘要撤销、尝试重新发布 | 恢复保留发布状态，撤销收回后续读取 | 重启后副本内容一致。撤销后 beta 查询不到该 ID，复用已撤销审批发布返回 409 |

该版本启用 PostgreSQL 和 Docker 的 Go 回归为 66 项通过、零跳过。`go vet ./...` 通过。

脱敏副本审核与发布实现对应 `66a84f646b54a18f7b4d2247e858d21ecf052c96`。

## 专题 09、14、16 A.9 · OpenBao 部署、隔离与恢复

在本机 Docker 用仓库中的 Compose 和 HCL 启动真实 OpenBao 2.7.2。使用短期 TLS 证书、随机验收值以及两个独立的单节点实例。结束后清理临时容器、数据卷和测试私钥。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 初始化三个解封份额，每次提交一个，通过 TLS 写入两个项目及两种环境的 KV v2 数据 | 一个份额保持 sealed，第二个可解封；各路径和版本独立 | 达到 2/3 后正常服务。项目 2 的 local-mock 同一键分别写入版本 1、2；staging 和项目 4 的同名键保留独立值 |
| 以仅有项目 2 local-mock 精确读权限的身份尝试跨环境、跨项目和写入，撤销初始 Root Token | 只读本路径，其他请求被拒 | 三类越权操作均为 403，Root Token 撤销后读取也为 403。独立限时备份身份仍可取快照 |
| 获取 Raft 快照并导出独立文件审计，再用独立恢复证书分别进行 CMS AES-256-GCM 加密并解密核对 | 数据与审计能独立恢复，审计不含测试密钥、令牌和解封份额明文 | 快照 26,137 字节；审计 44,924 字节，共 35 条，含拒绝记录和 HMAC。两份解密内容与归档前逐字节相等。密文 SHA-256 分别为 `3ede5ce266f4396f8356c003c228be869aa40b024e377364889cf97f09470aa4` 和 `9837969f98c1ef2e0bf2cdcd81614f7afe0c42af1882dde9c8ede947d519cb59` |
| 移除源容器和源数据卷，在独立空 Raft 实例恢复，使用源份额解封 | 旧数据、版本和权限策略恢复，目标初始化身份不再有效 | 恢复前目标键为 404。导入快照后节点封存，用源份额 2/3 解封。两个历史版本均匹配原随机值；跨环境和跨项目仍为 403，目标引导令牌返回 403 |

OpenBao 部署及恢复对应 `ab013b603c4bb3368e1daf777dcae7b6f3599b02`。

## 专题 09 · Go Secret Manager 的环境权限与实时状态

验收使用独立 TLS OpenBao、真实 Go 控制面、PostgreSQL、两个项目和三个节点身份。项目 2 的 local-mock、staging 及项目 4 的 local-mock 使用相同名称 `status-fd4d9c2f`，分别写入不同的随机值。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 通过 Owner 入口登记三个环境键，尝试附加 value 字段 | 登记风险与环境，不接收值；未写入时显示 MISSING | 相同风险重报的元数据相同，三个未写入路径均显示 MISSING；含 value 的注册请求为 400 |
| 写入两个版本，删除、恢复、销毁当前版本；用控制面和 alpha 节点读取 | Owner 和有对应环境授权的节点实时看到同一版本，普通查询只返回状态 | 版本 2 的状态依次为 AVAILABLE、DELETED、AVAILABLE、DESTROYED；各次响应只含项目、环境、名称、风险、更新时间、版本与状态 |
| 改用其他节点和环境，或直接用控制面元数据身份读 OpenBao 数据路径 | 仅授权的项目环境可读，元数据身份无法读取值 | 同项目跨环境、跨项目、仅有项目授权三个场景均为 403。beta 的合法请求保留其独立 S0 风险。元数据身份直读 data 路径也为 403 |
| 写入无效身份文件，替换为新的限时令牌并重启 | 失效身份不误报为缺失；进程内可轮换，重启后状态一致 | 请求先返回 503，替换后在同一进程恢复，重启结果与先前相同。扫描 public schema 的完整数据归档及控制面日志，均未出现四个测试值和两次有效元数据令牌 |

该版本启用 PostgreSQL 和 Docker 的 Go 回归为 67 项通过、零跳过。`go vet ./...` 与前端生产构建通过。

环境密钥状态对应 `72cd89d8f6ddeb4e5674805a2ee61afcd5225ac8`。

## 专题 09 · 执行绑定的限时凭据领取

通过真实控制面 API、PostgreSQL 和 TLS OpenBao 2.7.2 验证。alpha 节点领取项目 2 的任务 27、执行 23。测试用签名值仅写入独立 OpenBao。控制面分别使用只读元数据身份与强制响应包装的 Broker 身份。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| Owner 对正在运行的 alpha 执行批准固定版本、签名用途和一次用量，再重报相同请求 | 复用同一授权。其他项目、同项目其他节点、错误租约和用途不能领取 | 复用授权 1。跨项目 403，其他三项 409；Owner 查询使用记录为零。Broker 身份不带包装头直读 OpenBao 返回 403 |
| 同时发起两次领取，用得到的包装令牌解包并校验随机挑战的 HMAC | 只返回一个有效包装令牌，可以执行获准操作，重复解包失败 | 并发结果为 200 和 409。签名比对成功，第二次解包被拒。回执仅有一次 ISSUED |
| 创建 8 秒授权，等待 9 秒后解包；撤销另一份授权；模拟 Broker 身份无效 | 过期不能解包或再领取，撤销生效，后端失败可审计 | 到期后解包失败，再领取 409。撤销后领取 409。无效 Broker 身份返回 503，审计记录 FAILED |
| 在 execution-23 的独立 Worktree 核对 HEAD 后提交结果，重启控制面；尝试生产环境和 S3 | 任务结束后租约不能签发包装令牌，敏感凭据保留专用策略 | 工作项 DONE，结果为 `469cef06b81601ac2f382e230157c2e125787e39`，结束后领取返回 409，生产环境和 S3 的普通分发请求均为 403。重启前后审计相同。扫描数据库归档和控制面日志，未出现签名值、两类 OpenBao 身份令牌、执行租约或包装令牌 |

启用 PostgreSQL 和 Docker 的 Go 回归为 68 项通过、零跳过，`go vet ./...` 通过。并发和风险变更数据库测试另连续运行三次均通过。

执行绑定 Broker 对应提交 `8c83ac81ca46585cac3e17588ec19f1e6fd04a03`。

## 专题 07、10 · 固定提交的独立只读审查

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 用 `worker review-candidate` 审查队列配额代码。工作树已位于修复版，传入有边界错误的历史候选 | 审查读到固定候选，报告具体行号和可核对证据 | Claude Code Session `bfda2a07-502c-4ae6-8012-64ce4798e0e5` 在 `63d21b5ef9fb07acdd9c7e8107c5094e7252638f` 报告 High、blocking，指出 quota.py 第 3 行的 `<=`。Git 对象核对 evidence_verified=true |
| 改传修复候选，再审查此前通过真实 CI 的 alpha PR #9 候选 | 每次产生独立会话，报告绑定各自候选，保留原工作树 | `b1a07b8e4619487ba9842e78859da8157c24c937` 的会话 `1d1d8a72-2923-46fe-aadc-4fa44fbd4192` 无发现。alpha 的 `deb856d2e1e1b0c1e0102869998450c396899a5c` 由 `c6d8dbf4-1407-4c3b-8a3b-f5c27f8467ba` 完成，亦无发现。前后工作树 HEAD 及含未跟踪文件的状态一致 |
| 回归错误引用、删除行、重复 Finding ID 和二进制输入 | 错行或工作区独有内容不能伪造 Git 证据，无法完整处理的输入明确失败 | Git 对象测试通过，原页面 diff 的截断和外部 diff 禁用测试通过 |

只读候选审查对应 ForgeOS 提交 `85cd1cb0b3763e160f24bff722531c99cfce0469`。

## 专题 07、10、12 · 独立审查参与真实 GitHub 交付

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 启用项目 2 的审查门禁，按编号导入 alpha #10，并让开发 Worker 完成 | 得到可追溯的候选及本地验证。缺报告或请求审查其他 SHA 时拒绝 | 任务 28、执行 24、候选 `eeee49d87d1d503c97f2397bac6d2063646cbb5a`。报告尚未完成时批准返回 409，旧 SHA 请求审查返回 409。开发身份领取审查、审查身份提交批准都返回 401 |
| 按固定执行请求审查，在独立进程启动 `worker reviewer` | Claude 与开发会话分离，控制面核对固定对象并保存独立回执 | 会话 `39ae331b-2e6e-4512-9b7d-f05d8e2bc3a1` 完成，未报告发现。回执与开发结果的基础和候选 SHA 逐字段一致。Owner 批准成功，控制面重启后回执完全相同 |
| 让交付器创建 PR，等待必需检查，在相同 head 重跑首次失败的检查 | 检查通过才允许合并，Issue 回写并关闭 | PR #11 的 `acceptance-check` 成功，合并 SHA 为 `e20d92779c846102f3e58d1fd33169bbabb59d8b`。任务 DONE，Issue CLOSED。[Issue #10](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/10) · [PR #11](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/11) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/43841822142) |
| 用 PostgreSQL 测试失败、重试、错租约、开发会话复用、报告重放和引用未核实 | 无有效报告或缺核对说明时不能批准，重试使用新租约 | `TestCodeReviewGate` 通过。启用 PostgreSQL 和 Docker 的全套 Go 测试共 70 项通过、零跳过，`go vet ./...` 通过 |

审查门禁及以上交付对应 ForgeOS 提交 `940006841453be623726e3b2d0d58c859c940fa5`。

## 专题 10、11 · 从任务页面核对审查发现

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 390 × 844 的 Chrome 窗口打开已交付任务 28，展开执行 24 的审查报告 | 加载关联会话、候选和摘要，窄屏可读 | 页面显示原生会话 `39ae331b-2e6e-4512-9b7d-f05d8e2bc3a1` 和“未报告发现”，页面无横向溢出 |
| 在独立 PostgreSQL 测试 schema 中从页面请求审查，受控返回失败，再点击重试 | 批准被锁定，退回仍可用。重试使用新租约 | Chrome 中按钮状态符合预期，重试生成与原值不同的租约 |
| 把此前配额样例的真实 Claude 报告提交给控制面，在页面核对并退回候选 | 行证据可核对，空说明无法保存。Owner 可据此退回，历史记录保留 | 页面展示 High、阻塞、`quota.py:3`、原文和引用匹配结果。保存核对说明后批准按钮可用；因确认边界错误，实际选择退回。任务变为 READY，重载后仍能打开原报告并读到说明 |

第二、三项使用预置开发执行和真实 Git 对象，回放已实测的原生报告。浏览器、控制面、租约、证据核对和数据库写入均按实际入口运行；该样例未创建远端 PR。

审查页面对应 ForgeOS 提交 `427bc24871b091cae2eb43204cbc2799aef3609c`。该版本 70 项 Go 测试通过、零跳过，前端生产构建和 `go vet ./...` 通过。

## 专题 10、16 · 仓库验证配置合同

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 将仓库示例提交到本地验收项目 8，再改写工作树，从 Owner API 分别请求 V1–V4 | 读取固定 Git 配置，缺检查时指出具体缺口 | 提交 `ac17ea0bf8e757b296d5ebf3ec1e876eeb2c3ba9`、配置 blob `db25f663f8560417c4994d5fa4bd46bb2db04926`。V1、V2、V3 分别列出 3、5、6 项。V4 缺 critical Profile、security 和 regression 必需检查 |
| 新增两种检查和 critical Profile，再用具有和不具有项目授权的节点查看 V4 | 完整配置返回 8 项去重检查；项目授权独立 | 授权节点读取提交 `108844db6b36d65e8b476b22eb5c410076d43ec8`、配置 blob `1ef073028e3c4a765d82c21280f592389b0d76bc`，ready=true。其他节点返回 403 |
| 在另一提交添加 privileged 字段，另以 HEAD 请求计划 | 配置不能为自己加权；只能按固定提交检查 | 提交 `1d07b4ec714e3ce820000c8eb1bdb89e463efa7a` 返回 422。HEAD 请求也返回 422。操作前后的未提交配置一致 |

本项验收的是可执行检查清单的解析与覆盖判定。启用 PostgreSQL 和 Docker 的全套 Go 回归共 81 项通过、零跳过；`go vet ./...` 和 `go mod verify` 通过。

仓库检查配置对应提交 `35866baa5cd8635d4a5906fd21a4016c90bf48bd`。

## 专题 10 · 在隔离容器中执行仓库检查并归档证据

通过非 root Worker 的 `validate-candidate` 命令验收。配置基础为 `c8cb70674f17701f75ab0655405f8b165d64bf8f`，实际 Linux 容器镜像为 `sha256:c05ba4b73604069d376c4f41346b05374335b5ca0c46fb6dfede5a59f5196931`。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 以 V1 检查历史候选 `df7547f581d76629137ff4f21a0cdab490875c18`，另留未提交内容，并在宿主设置两个凭据哨兵 | 检查读取固定候选；容器没有凭据、默认路由或源挂载写权限；可选失败不阻断通过 | `ci-run-1876710922` 的容器 UID 为 65534。必需检查退出 0，可选检查退出 7。命令返回 0。工作树仍保留原未提交内容。报告和产物字节均与固定候选内容一致，按文件复算 SHA-256 匹配 |
| 在候选中改写检查命令并提供错误输入 | 原配置仍生效且归档失败检查的证据 | `6675f97283beb8450e9540424c3987f1f1beb389` 的运行 `ci-run-2228911632` 返回 CODE_FAILURE，配置 blob 与前次一致，报告内容对应失败的候选 |
| V2 中让一个必需检查超时、另一个构建退出 3；另请求缺少 Profile 的 V4 | 区分超时、构建失败、配置缺项；未执行不能生成通过记录 | `ci-run-1095856787` 同时记录 TIMEOUT（退出码 null）和 BUILD_FAILURE（退出码 3）。V4 运行 `ci-run-907231703` 为 CONFIG_FAILURE，检查记录为空。命令完成后按容器名查询无残留 |

PostgreSQL 和 Docker 全套 Go 回归为 82 项通过、零跳过。额外覆盖命令退出 0 但未生成承诺的报告，结果为 TEST_ENV_FAILURE。

离线容器 Runner 对应 ForgeOS 提交 `ee695b39ca1d61ec7d3d8bfd0ebaefe1681cfaf6`。

## 专题 10、12 · 从真实 Issue 发起仓库检查并交付

在 beta 私有验收仓库通过 PR #7 引入 Go 包及验证配置，合并后的基础为 `9d0213cfa67f8fded11bda56d7c4a6fc86713981`。随后通过 Owner 接口按编号导入 Issue #8，选 V2。[配置 PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/7) · [需求](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/8)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 让旧 Worker 和支持仓库检查的 Worker 先后领取 | 旧 Worker 无任务；新 Worker 执行固定格式、单元和构建检查 | 旧领取返回 204。任务 29、执行 25 的三项退出码均为 0。首次候选有 953 个变更文件，页面差异截断。核查发现带入编译缓存，因此通过 Owner 入口退回 |
| 修订后重新检查并核对 SHA、文件与报告 | 新候选有新运行记录，实际交付只含需求范围内的实现和测试 | 执行 26、运行 `ci-run-418587960`，候选 `84997d86d4d4a1cfd32a36cf14c090c66e3e2721`。仅有 queue.go 和 queue_test.go 两个变更文件，覆盖等于、超过上限和负数输入。两次执行均绑定配置 blob `24c99f7c289bd37b2fd5a6f3d1a981ee001d3828`；第二次三个检查全通过，归档报告复算 SHA-256 与 API 一致 |
| 在 Chrome 展开执行 26 的“逐项检查”，查看运行编号与代码差异，批准并等待远端交付 | 本地检查有独立证据，GitHub 的必需检查仍约束合并。重启可读原回执 | PR #9 的首次远端检查失败，同一 head 重跑成功。合并 SHA `bc83d9a1399a0ab8eb78f2cb0f0f6464e19a8ebf`，任务 DONE、Issue CLOSED。重启前后完整验证回执相等。[PR #9](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/9) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43847029410) |
| 用测试仓库提交其他 Runner、错执行号、错误配置 blob、遗漏检查和将必需检查改为可选的回执 | 控制面逐一拒绝；候选若修改验证配置，即使通过仍需独立审查 | `TestDeclaredChecksRequiredForIssue` 通过，配置变动的候选处于 WAITING_ACCEPTANCE，直接批准返回 409。Go 回归 83 项通过、零跳过；前端生产构建通过 |

Issue 仓库检查和交付对应 ForgeOS 提交 `80836d649574426c6e6984c66aaff5feab0e2da3`。

## 专题 10、12、16 · 本地 CI 的 GitHub Commit Status

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 对已交付的 beta 任务 29、执行 26 并发补写，重启后再次请求；尝试错 SHA 和其他项目 | 同一回执，远端仅一条；不允许错目标 | 两个请求均返回 status ID `59014274755`。重启后不变，远端计数 1。错 SHA、跨项目和仅在本地的候选都返回 409 |
| 在 beta 严格分支保护中保留 acceptance-check 并临时加入 forgeos/local-ci，再交付 Issue #10 | 两项均成功后才允许合并，状态与验收 SHA 一致 | 任务 32、执行 27，候选 `a54213eddb57ea1cc89539581037c1d2bc0e560d`，本地状态 `59014432428` 自动写回，Chrome 执行详情可见。Actions 初次失败，同一 head 重跑通过。PR #11 合并为 `42a7f8dfcaf18e91aec089169335de96e158d69d`，Issue CLOSED。实测结束后恢复原必需检查及 strict=true。[PR #11](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/11) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43848242665) |
| 将一个故意失败的测试提交预置到专用远端分支，以隔离测试 schema 派发该 SHA 的真实 V2 验证。模拟 GitHub 已接收但本地未记录确认 | Worker 得到非零退出码，回传 failure；恢复沿用原 status ID | 提交 `b784f0f8fa9f57d4d2c6312ba88f406496e1037d` 的单元检查失败，任务 FAILED，状态 `59014437445` 为 failure。移除测试 schema 中该回执后，经 API 恢复相同 ID，远端计数保持 1。此分支未创建 PR。[验收提交](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/commit/b784f0f8fa9f57d4d2c6312ba88f406496e1037d) |

含 PostgreSQL 和 Docker 的 84 项 Go 测试全部通过，零跳过。原生测试还覆盖超时映射为 error、未知远端 SHA 和远端写入后丢失响应。前端生产构建与 `go vet ./...` 通过。

GitHub Commit Status 对应 ForgeOS 提交 `749351cf484cb665f275355fc25e67912651fb8c`。

## 专题 10、11 · 固定候选的风险判定与 V3、V4 容器实测

项目 9 的基础提交为 `86470cf7df0a3245e9d7ad78a5b3deb5d23f80db`，验收规则 blob 为 `cbe4e01e3d6e197d048afc3c73bb0db8f0c24ce4`。通过控制面查询评估，将返回等级交给 `worker validate-candidate`。本次覆盖独立评估与 Runner 入口。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 将只修改文档的候选按 staging 目标评估，请求最低 V1，再执行选出的等级 | 目标提高到 V3，运行格式、单元、构建与 HTTP 集成 | 候选 `91a226260d7c3c1962720f1531132cf6b7586976`，运行 `ci-run-3762549469`。四项均退出 0，HTTP 检查在独立 Linux 容器内启动服务并实际发送请求 |
| 将 accounting 下的文档移出组件目录，管理建议 V1 | 基础版本中的旧路径命中 ledger 规则，保持 V4，追加安全与关键回归 | 候选 `a4a9f6373d89115f2aacebfda5585e0aa16a097e` 的三条差异路径中命中 `accounting/guide.md`。运行 `ci-run-527488150` 的六项检查全部退出 0，含 go vet 和队列边界回归 |
| 从 Owner 与项目 Worker 入口读取匹配详情，尝试跨项目；在候选中降低验收规则 | 两种身份可读取授权项目的相同规则和人工验收要求；候选不能改写本次选级 | Worker 在其他项目被拒绝。ledger 返回 api-backend、payment、staging 人工验收及容量边界预期。降低规则的候选仍按基础规则评估 |

固定候选风险评估对应 ForgeOS 提交 `fdc4192e440223fefd375fbb64e8ec979583d516`。启用 PostgreSQL 和 Docker 时 86 项 Go 测试通过，零跳过。

## 专题 10、12 · 自动分级贯穿 Issue、容器检查和交付

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 通过 Owner 接口将 beta #12 以 docs、documentation、local 导入，再以仅有旧检查能力的 Worker 请求领取 | 策略冻结。GitHub 工作项最低为 PR 目标；旧节点不能领取 | 任务 33 保存自动分级、Owner 最低 V1 和 PR 目标。旧节点收到 204。新 Worker 领取执行 28，原生开发会话只修改了 docs/queue-inputs.md |
| 在 Chrome 查看选级依据、检查和差异并批准，等待真实 GitHub 回执 | 文档差异为 V1，PR 提高到 V2，三项检查和远端必需检查通过才能交付 | 候选 `0a6de37033a90b6df3b270bf1e8dd3bf55a6f9ee`，运行 `ci-run-3420652059` 的 format、unit、build 均退出 0。状态 ID `59016109988` 标注 V2。远端必需检查首次失败，同 head 重跑成功；合并 SHA `fdef7f0867f55743acffd6c0b564736e3b6ff7a4`。任务 DONE、Issue CLOSED；重启后回执一致。[Issue #12](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/12) · [PR #13](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/13) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43851591297) |
| 用真实 Git 对象构造一个有完整 V1 通过回执、但变更策略文件的候选，伪报自动等级为 V1 | 控制面从固定输入独立复算出 V4 并拒绝低等级回执 | `TestDeclaredChecksRequiredForIssue` 得到冲突；原固定 V1 历史任务仍可提交真实 V1 回执。87 项 Go 回归通过，零跳过；go vet 和前端生产构建通过 |
| 独立 Runner 使用 auto、V1 最低要求与 PR 目标重新检查 ledger 文档移出目录的候选 | 本地 Runner 自行匹配原路径，强制运行 V4 | `ci-run-3800396296` 在候选 `a4a9f6373d89115f2aacebfda5585e0aa16a097e` 运行六项检查，全部通过。run.json 中保存 ledger 组件、基础规则及升级理由 |

Issue 强制风险分级对应 ForgeOS 提交 `48a32f2110a4e1818fe42dbab7f62f89b80ef3bd`。

## 专题 05、06、10 · Management 提高计划的验证下限

先在 beta 私有验收项目提交 HTTP 集成测试与 V4 Profile，经必需 CI 后合并到 `bea34fc766114c19c9dd79d3598a4f27838ce4e1`。随后导入只要求修改发布清单文档的 Issue #15，Owner 指定 docs、documentation；Issue 要求对候选运行关键回归。[环境 PR #14](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/14) · [Issue #15](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/15)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 由独立 Management Worker 领取 Goal 34 的修订 1，在 Chrome 查看审核，随后从 Owner 入口激活计划 | V4 建议有具体依据；D2 待 Owner 激活，建议在审批前不改变执行策略 | 原生 Codex 会话 `01a1e3fc-8946-71c2-aad3-9d057da9b5db` 引用了 Issue 对 HTTP、go vet 与边界回归的要求，返回 D2、approve、V4。激活前为 PLANNING、零子项；激活后子项 35 的冻结策略含管理下限 V4 |
| 在 PostgreSQL 中依次审核 V3、V4、V1 三版计划，第一版激活后先让 Worker 领取一个子项 | 后续提级覆盖未启动子项；在途执行沿用原策略。低建议不能覆盖已生效下限 | `TestManagementValidationMinimumAppliesOnActivation` 通过。等待的子项由 V3 升至 V4，第三版仍为 V4；已领取的子项维持 V3 |
| 用开发 Worker 完成子项，核对清单内的边界示例与测试名称，批准该 SHA 并等待 Goal 关闭 | 文档 V1、PR V2、Management V4；必须实际跑完六项。子项交付后才关闭父 Issue | 执行 29、`ci-run-3288283744`、候选 `bb383121eea44590f4db686735466844fc8ad865`。仅修改 docs/release-checklist.md，format、unit、build、http、vet、boundary 均退出 0。GitHub Status `59017049649` 写回 V4 和六项检查。PR #16 合并为 `f4972a88317187fda5d169d788287c8d7a7ffe4e`，子项与 Goal 均 DONE，父 Issue CLOSED。重启后原计划审核仍可读取。[PR #16](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/16) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43853419062) |

本次 PostgreSQL 与 Docker 全套 Go 回归 88 项通过，零跳过。`go vet ./...` 和前端生产构建通过。

Management 提高计划验证下限对应 ForgeOS 提交 `0420f455c76949ddb9d5240e1632b53f594bcd40`。

## 专题 11 · 全局验收方法库

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从 Owner 和获项目 9 授权的 Worker 入口读取库，与源码数据逐项比较并独立复算摘要 | 两种入口返回文档列出的十二类方法及项目示例引用的 security，所有方法可按 ID 查阅 | 两份响应一致，包含 13 项。版本为 1，SHA-256 为 `c3aee4047dc85cebf29caca6f145c9b548019b0d4810a8087e4372d7f8f06942`。重启后结果一致 |
| 从固定候选的组件匹配结果读取方法 ID，再查全库；用其他项目节点读取项目 9，或用 Owner 查询不存在的项目 | 匹配的 ID 对应具体步骤与取证要求，越权和不存在的项目返回不同错误 | ledger 的 api-backend、payment 均能取到三步操作和两类证据。跨项目请求返回 403，未登记项目返回 404。此次核对方法内容与访问入口，各方法的业务实测由对应项目的验收计划记录 |

方法库 Schema、专题范围和示例引用测试通过，对应 ForgeOS 提交 `86aff28aff1ff9b0b6559f9d65dfc73897ae9334`。

### 容器回归复核

首次并行回归时，容器集成测试的两个候选共用一分钟超时。第一项通过，第二项启动时上下文已超时，未取得 Git 工作树。为每个候选单独设置测试时限后，在真实 Docker 再次验证正常报告和缺失报告两条路径。预期第一项归档匹配的 SHA-256，第二项即使命令退出 0 也应标记 TEST_ENV_FAILURE；两项断言在 8.50 秒内通过。项目声明的单项检查仍使用原超时。

在专用 PostgreSQL 18 和本机 Docker 上运行 `go test -count=1 -p 1 -json ./...`，89 项通过、零跳过。`go vet ./...` 通过。

容器回归时限修复对应 ForgeOS 提交 `b02d69924d6bad2e847e777cd7688618cda60434`。

## 专题 11 · 从固定候选生成测试计划

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从 `worker plan-tests` 提交 beta #15 的文档候选、两个明确标准与冻结的 V4 策略 | 输出可操作的用例，每例能追溯原文，保留待测状态 | 初次生成发现库中缺少文档核对方法。补入 documentation 后，候选 `bb383121eea44590f4db686735466844fc8ad865` 的原生会话 `01a1e412-0029-7933-919b-ae0a7652bca2` 返回 TC-001、TC-002。分别核对两个测试名称与允许/拒绝示例，均为 pending。方法库摘要 `f9af76bc56b37bd6c52fd99841d263ac3bf5041b591c9f1a9d853587dd61722c` |
| 对移出 ledger 模块的候选指定容量标准，再加入“足够快”的模糊性能标准 | 使用基础版本的组件预期，要求 staging 人工操作；缺阈值时提问 | 候选 `a4a9f6373d89115f2aacebfda5585e0aa16a097e` 的原生会话 `01a1e412-320a-7bc0-b173-153e4865a9b5` 生成两项 pending 用例。组件用例绑定 component/ledger/1、api-backend、human、staging。requirement/2 返回指标、阈值、负载、环境及观测时长的澄清问题 |
| 尝试提交虚构的预期、已通过的结果、错误环境、错误方法或遗漏标准 | 拒绝无来源的期望及未执行先报通过；允许无业务预期时只提问 | `TestPlanBindingsAndClarifications` 逐项通过。独立生成入口没有写入人工测试结果 |

独立测试计划生成对应 ForgeOS 提交 `7565fa6af37a2395fb45310bf9dff7e291f3445d`。含 PostgreSQL 与 Docker 的 90 项 Go 回归通过，零跳过。

## 专题 11 · Management 对待测计划逐项审查

通过 `worker review-test-plan` 提交原始请求和完整计划。以下三个审查均使用原生 Codex 独立会话。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 检查 beta #15 两项文档用例，核对步骤和证据 | 两项均有可操作的检查，总结果 covered，用例保持 pending | 会话 `01a1e415-6ed0-7642-889b-0b74ab713b14` 分别引用了对应的 TC 编号。候选 `bb383121eea44590f4db686735466844fc8ad865`，计划摘要 `3c803e466bb47638d205d9b3b722caa7251834aee0a1aab9bddaa1648c3b3e63` |
| 在同一候选把第一个用例改为仅查看 docs 目录，保留原标准、预期和编号 | 目录截图无法证明文中出现指定测试名称，审核指出缺口 | 会话 `01a1e415-6ecf-78a2-a030-ebafa28200dc` 将第一项判为 gap、第二项为 covered，总结果 changes_requested，收到不同计划摘要 `879bc089893f4eda00b975e37bd4ee043e025c18fcb9411598e34f18d4649a4f` |
| 核查 ledger 计划中的模糊性能要求；另以已修改的原始标准提交旧计划 | 未解决的标准保持澄清；来源发生变化时拒绝旧计划 | 会话 `01a1e415-6ed0-75a0-863c-4a9ca31d5a80` 给两个容量标准 covered、性能标准 needs_clarification。错来源请求返回非零且没有审查回执 |

独立覆盖审查对应 ForgeOS 提交 `e71b7c8b142f4dceccf7d3665c54ae7d037edae3`。

## 专题 11、13 · 候选任务内派发计划、覆盖门禁与修订归档

导入 beta #17，开发 Worker 生成任务 36、执行 30、候选 `3f20ee3a6a1cf53c37fe90d9b326cc5b992f49de`。基础为 `f4972a88317187fda5d169d788287c8d7a7ffe4e`，唯一变化是 docs/empty-queue.md。[Issue #17](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/17)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 Chrome 生成测试计划，附加“应该很快”的标准。尝试普通 Worker 领取 Test Agent 作业、使用错误 SHA、重发原请求并尝试直接批准候选 | 身份与候选必须匹配，重发复用修订，覆盖未通过时不能批准 | 错角色 401，错 SHA 与批准请求均为 409，原请求返回同一修订。Test Agent 会话 `01a1e41e-b9b4-7dd3-86fb-032cfd20532c` 和 Management 会话 `01a1e41f-0bb7-7a63-9d69-88a916166c7f` 对修订 1 返回 needs_clarification |
| 通过 Chrome 将补充标准明确为两种容量输入和对应布尔结果，生成修订 2；阅读审查、翻回修订 1，并重启控制面 | 新修订保留原始 Issue，独立生成与审核；两个版本均可复查 | 两个新会话分别为 `01a1e41f-5855-7ea1-b352-1087a77f273f` 和 `01a1e41f-ce12-7741-be2d-3bc561206adc`。第二版 covered，两项用例仍为 pending，计划摘要 `359a406bbb3a8c0565e41a600183684196b069d8556a3ea72e7d2b6fa34bd47b`。重启前后两个完整回执分别相等 |
| 在 PostgreSQL 中模拟并发领取、修订后的迟到结果、其他项目身份、开发会话冒充 Test Agent、伪造覆盖摘要及重复上报 | 只接收当前版本、正确身份和匹配计划的结果；重试不得重复派发 | `TestCandidatePlanRolesAndRevisions` 通过。生成及覆盖审查期间批准均返回冲突。覆盖通过后恢复批准入口 |

工作项内计划与审查对应 ForgeOS 提交 `978d1849db027f2af78a7b4e59491465dd45c267`。该版 PostgreSQL 与 Docker 回归有 92 项通过。以下实测在此基础上新增了逐用例的批准条件。

## 专题 11 · 冻结候选的用例执行与交付

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 beta #17 的执行 30、修订 2 通过覆盖审查后直接批准；以错误项目、错误 SHA 或 human 身份上报 runner 用例 | 拒绝提前批准和错误来源 | 提前批准 409、其他项目 403、错误 SHA 与错误用例身份均为 409 |
| 具名项目节点 case-runner-beta 在固定候选检查变更文件、队列实现和文档，运行 `go test -count=1 -v -run 'TestCanEnqueue/(empty_queue\|zero_capacity)' ./...`。先在基础版本读取示例，再在正确候选复测 | 两项用例均有实际操作和可复核的证据。缺少文档时记录阻塞，复测保留前次结果 | TC-001 通过；TC-002 先受阻后通过。Chrome 展开第 2 次实测读到文档 blob `936801e8bb1b8e31fb5e6377842d240bf398438d`。重启控制面、重放同一回报，两项共三条记录 |
| 在 Chrome 批准该候选，等待 GitHub 必需检查并读取 Issue 与 PR | 全部用例通过后才可批准和交付 | 候选 `3f20ee3a6a1cf53c37fe90d9b326cc5b992f49de` 合并为 `218f96c03ea383724ccd5915ce16d602758ff9d6`，Issue 已关闭。[PR #18](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/18) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43859752697) |
| 在单独的 PostgreSQL schema 中登记本地仓库及测试 Issue，运行真实 Worker、Docker、Test Agent 与 Management。项目规则要求 Owner 在 Chrome 检查 docs/owner-check.md。打开差异核对新增的“Queue is empty.”，经人工实测表单逐例填写操作、观察和文件行号 | 无计划时批准返回冲突，覆盖通过后仍须采证，提交两项结果后可以批准 | 候选 `e87782520198c8d974fd9dc149f1a734203ac7e3`，计划摘要 `c7d1b423e08e0f0845b29bfb77a4bbda901af4ddb2f0eb90a50c4ce03d3a989c`。TC-001 和 TC-002 均记录 actor=owner、tester=human。批准返回 ACCEPTED。此次通过 Playwright 操作本机 Chrome 的 Owner 入口 |

该版本启用 PostgreSQL 和 Docker 运行 `go test -count=1 -p 1 -json ./...`，92 项通过、零跳过。`go vet ./...`、前端生产构建通过。

用例实测与交付门禁对应 ForgeOS 提交 `71e6c9adb9d8e3c452c3a7d30baf3cc4d1c73537`。

## 专题 11 · 本机静态站点预览

在独立 PostgreSQL schema 登记本地站点仓库，执行 1 在工作树 execution-1 以真实 Docker 完成 V2 检查。候选为 `273616480856ec0a7d064ded86f13c032bb01f98`。通过 Playwright 操作本机 Chrome。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在执行详情选择 public，启动并打开预览。另用普通 Worker 身份、错误候选、缺少入口的目录分别尝试启动 | 页面和根路径脚本可加载，执行、工作树、环境、访问策略、SHA 在回执中一致；拒绝错误请求 | Chrome 标题区显示 Candidate queue，脚本将计数更新为 0 waiting。该页面的 localStorage 初始为空，响应头与候选一致。错误身份返回 401、错 SHA 返回 409、缺少入口返回 422 |
| 分别修改项目克隆与执行工作树的 index.html，重新打开；尝试访问公开目录外文件、隐藏文件、Git 元数据、控制面路由及无预览域名的回环地址 | 页面保持候选版本，越界请求不可读 | 刷新后仍显示 Candidate queue。五个路径与错误域名均返回 404，Git 对象读取回归另覆盖符号链接 |
| 重启控制面，重复启动，再在任务详情停止预览；新建另一个三秒预览并等待到期 | 重启后原地址有效，停止及到期均能收回访问权限 | 原回执完全相同且页面返回 200。点击停止后旧地址返回 404。新预览由 200 变为 404，历史列表同时保留 expired 与 stopped |

重启后分别重复停止已停止和已到期的实例，两个完整回执均保持一致。启用 PostgreSQL 与 Docker 的 Go 回归有 93 项通过、零跳过，Go vet 与前端生产构建通过。
