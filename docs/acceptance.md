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
| 三 · 决策、事件与恢复 | 子项 15 在验收边界提出业务问题 1，状态变为 WAITING_INPUT。重报只保留一个问题，直接批准返回 409。重启控制面后 Worker 回答返回 401。在 Chrome 以 Owner 回答后，恢复原执行 11、Session `01a1e315-16d2-7f31-8de3-7485865d1c95`、候选 `68d6097a` 和 WAITING_ACCEPTANCE。GitHub 需求修订、回合间评论合并已在专题 06、12 实测。 | 跨操作的角色与决策分级、运行中 Harness 发起问题及恢复仍待验收。 |
| 三 · 审查、验证规则与验收方法 | alpha #10 经独立审查后交付。beta #12 经风险分级后交付。Goal 34 的 Management 审核将子项提升至 V4。beta #17 从全局方法生成测试计划、澄清并复审，两项用例实测通过后交付。 | 通过上述候选场景。预览和反馈见下一行。 |
| 三 · 预览、批注与知识 | Chrome、Caddy 双向 TLS、DOM 与 Canvas 批注、六类反馈、阻塞澄清、从反馈退回修订及新候选批准均已实测。beta #19 还验证了从反馈修订需求、采用和再次执行。项目知识的来源、决策索引和审核后的共享副本见专题 08。 | 外部设备与 tailnet、独立 Staging 待验收。动态 local-mock 应用的工作台交互和双向 TLS 网关已实测。扩展原生录屏已验收。已从扩展采集候选视口，在工作台遮挡、标注并归档。React 与 Vue 的同源 iframe 批注已实测。独立新工作项已从反馈 #2 实测到 beta PR #21 合并。 |
| 四 · 多项目授权与密钥 | 节点按项目和环境读取 OpenBao 状态。Broker 在执行租约、用途、版本和 TTL 满足时分发一次性包装响应；脱敏共享和密钥恢复也已实测。 | 生产专用操作、执行资源调度和跨节点恢复见专题 07、09、14。 |
| 四 · 多 Worker、多 Harness 与运维 | 两个真实项目各交付了一个工作项，beta 在控制面停止后重启，复用既有 PR 完成回写。Claude Code 已交付 Owner 明确选择的 beta #22，以及项目配置和标签自动派发的 #24。持久节点登记、心跳和能力查询见专题 07。数据库、反馈截图和工作区恢复演练见专题 14。 | 普通执行的节点容量、跨项目派发及失联占位已实测，见本页末尾。GitHub 四类写入的响应丢失恢复见 beta #27。普通 Worker 的模型与候选 CI 独立名额已在 alpha #20、beta #45 验收。独立角色预算、压力与节点原生会话恢复待验收。 |

MVP 页面验收与 GitHub 门禁证据对应基线、收件箱和交付诊断提交，最后一项为 `32c0fa07cc09eaf98750e588682d0f9701dd92f4`。两个私有项目已在同一 PostgreSQL 中分别派发和交付，并验证项目专用业务凭据的访问边界，见本页阶段四实测。GitHub 操作共用本机 CLI 身份，GitHub App 最小授权仍待验收。

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
| 02–03 系统与模块 | 独立控制面、Worker、PostgreSQL、React 工作台、只读规划、交付器、Management 审核、知识查询和静态预览均从实际入口运行 | `1a17822`、`cc2011f`、`f8503c5`、`a31c02c`、`7fe001f`、`99de4f6` | 通知、节点资源管理、其余管理协调与动态预览能力 |
| 04 领域模型 | 同一任务关联历史需求版本、执行、计划修订、候选、验收、子 PR 和 Merge SHA。已归档独立 CI Run ID、检查输出的相对文件名和内容摘要 | `a7f8b48`、`5d30bcf`、`94b6b08`、`70781ba`、`ee695b3` | 已完成的 Claude、Codex 实现及只读规划进程归档和工作台查询见下文。跨节点 CI 文件下载见 beta #49；其他跨节点工件、运行中的中央进程视图、独立审核角色待验收 |
| 05 角色与决策 | 任务 15 等待业务答复；Goal 16 的 Management 审核退回两版、批准第三版。跨项目验收问题的批量答复见本页末尾 | `70781ba`、`f8503c5`，批量答复见下文 | 跨操作 D0–D3 策略、ACP 路由及其他阶段的问题汇总 |
| 06 规划与编排 | 任务 16 从 Planner 草案、独立审核到运行中修订和两次子 PR 合并；Goal 13 显式激活。真实 Issue 评论分批进入回合，预览阻塞问题及候选退回在验收检查点处理 | `cc2011f`、`5d30bcf`、`83ea262`、`94b6b08`、`e981f07`、`f8503c5`、`f519840`、`4d31e7f`、`bf27241`、`7cbd8a3` | 通用命令队列、运行中暂停与取消 |
| 07 Worker 与 Harness | Windows 原生 Codex 和 Claude Code 实现任务、只读规划、失联租约及评论检查点；Ubuntu 非 root systemd。独立探针已验证 Claude Code 会话恢复、Codex App Server 会话恢复和中断 | `1a17822`、`cc2011f`、`b4e7c54`、`4d31e7f`、`bd1c734`、`1d9617e`、`fe35218`、`b11089b` | 节点登记与普通执行容量见下文。Claude 实现、Codex 实现与只读规划的终态丢失恢复见下文。原生进程被强制终止、Linux 模型登录复用、多节点恢复待验收 |
| 08 项目知识 | 固定版本工程事实、来源历史检索、项目决策、私有资料修订和跨项目脱敏审批均见下文验收记录 | `cc2011f`、`a31c02c`、`5225e60`、`7ae7254`，共享证据见下文 | 语义检索、全局规则、Curator 其余触发条件和角色上下文 |
| 09 权限与密钥 | Owner/Worker 分权、项目和环境成对授权、OpenBao 真实部署与恢复、状态接口、执行租约与用途绑定、一次性解包、签发与失败审计 | `1a17822`、`319aa65`、`70781ba`、`c513401`、`ab013b6`、`72cd89d`、`8c83ac8` | 生产专用操作策略、其他节点身份与证书轮换 |
| 10 审查与验证 | 真实 Docker V1–V4 检查、GitHub Status、Actions 门禁、重新验证、独立审查和页面验收。Issue 强制风险等级，Management 可提高下限 | `1a17822`、`32c0fa0`、`85cd1cb`、`9400068`、`427bc24`、`35866ba`、`ee695b3`、`80836d6`、`749351c`、`fdc4192`、`48a32f2`，Management 提级见下文 | 日志、报告及产物下载见 beta #49；中央保留策略见下文；运行取消和节点本地清理待验收 |
| 11 预览与人工验收 | 工作项归档计划、覆盖审核及用例实测；Caddy、静态预览、DOM/Canvas 和同源嵌套 iframe 选择、用例关联、六类 Management 分类、阻塞澄清、按反馈退回和批准、从反馈修订需求及派发独立工作项（beta #19–21）、工作台 PNG 标注与遮挡归档 | `1a17822`、`70781ba`、`fdc4192`、`86aff28`、`7565fa6`、`e71b7c8`、`978d184`、`71e6c9a`、`7fe001f`、`ec7a462`、`17bb819`、`c561e40`、`99de4f6`、`bf27241`、`7cbd8a3`、`5a5d685`、`d34260b`、`2b805b4`、`5279f11`、`c382473` | 动态应用预览已验收，Worker 提交 `9a08b44`，工作台证据见本页。Staging 和外部 tailnet 设备待验收。扩展采集证据见本页专题 11 |
| 12 GitHub 接入与交付 | 标签派单、普通讨论转换、受保护分支阻塞与合并、子 PR 按序合并、来源需求修订与检查点换版、目标分支前移后重新验证与交付。签名 Webhook 原文归档与补充轮询见 beta #51 | `319aa65`、`32c0fa0`、`94b6b08`、`5447445`、`a7f8b48`、`20c30e6`、`391309e`、`4d31e7f`、`e2912c5`，Webhook 见下文 | 细粒度凭据、第二人审查及来源关闭后的安全取消 |
| 13 数据与审计 | 数据库迁移、重启恢复、幂等事件、Git 计划快照、交付证据链与反馈截图校验回读 | `1a17822`、`83ea262`、`5447445`、`c382473` | 执行领取和结果离线补报见下文。已验证从独立快照恢复节点 SQLite、补报及保留已完成的进程记录。中央 CI 文件保留策略见下文。完整审计载荷、其他工件保留策略及其余进程和会话事件缓冲仍待验收 |
| 14 部署与运维 | Windows 主机、Docker Linux 引擎、Ubuntu systemd 重启与派发；PostgreSQL、OpenBao 和反馈截图备份恢复；Caddy 双向 TLS 网关。恢复工作区后已用独立 Worker 继续派发。CI 文件与原始回执、Worker SQLite 与配套数据库的恢复见本页末尾 | `1a17822`、`32c0fa0`、`bd1c734`、`06ed00b`、`ab013b6`、`ec7a462`、`42ba820`、`ff5d714` | 普通 Worker 的模型与候选 CI 独立名额见 alpha #20、beta #45。外部设备私网访问、独立角色及跨节点共享预算、其他工件恢复、失联模型会话恢复待验收 |
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

本机静态预览对应 ForgeOS 提交 `7fe001fa7cc59ba849d09dd1a8542126626fe1d7`。

## 专题 11、14 · 经 Caddy 打开及撤销 HTTPS 预览

使用 Caddy 2.11.4、隔离的临时 CA 和客户端证书、PostgreSQL 与本机 Chrome。测试客户端将专用测试域名解析至本机；服务端证书覆盖该区，回环网关端口为 55791。此项验证 HTTPS 网关。外部设备、Tailscale DNS 和 ACL 保持待验收。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 通过 TLS 访问工作台，分别使用无客户端证书、有效证书但无 Owner 令牌、证书和有效令牌 | 设备和应用分别鉴权 | 无证书时 TLS 连接被拒；有证书的客户端可打开登录页，读取任务列表返回 401，加 Owner 令牌查询任务 1 返回 200 |
| 从工作台域名用有效 Worker 令牌向内部领取入口发请求；从预览域名查询任务 1 | 私网入口仅提供各域名的指定功能 | 两个请求均返回 404 |
| 在 Chrome 登录工作台，点击“打开私网预览”。核对候选与脚本运行结果，随后在原工作台停止预览并刷新另一标签页 | 独立域名加载固定候选，停止立即收回访问 | 页面显示 Candidate queue 和 0 waiting，响应头绑定 `273616480856ec0a7d064ded86f13c032bb01f98`，预览来源的 localStorage 为空。停止后链接消失，原页刷新返回 404 |

启用 PostgreSQL 和 Docker 的回归有 94 项通过、零跳过，Go vet 和前端生产构建通过。

Caddy 网关对应 ForgeOS 提交 `ec7a462baf075e210456a364a0d697777773695c`。

## 专题 11、13 · 预览反馈登记与测试用例关联

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在固定候选的公开预览中查看标题和队列计数，再从任务详情进入同一实例的反馈表单，选择测试用例并提交观察 | 工作台保留预览、执行、SHA 和所选计划版本；反馈可复查并等待分类 | 本机 Chrome 显示 Candidate queue、0 waiting。反馈 #7 记下观察和计数无障碍标签建议，绑定任务 1、执行 1、候选 `273616480856ec0a7d064ded86f13c032bb01f98`、计划 v1 的 TC-001。该计划来自原生 Test Agent 会话 `01a1e441-461f-7180-92e8-b8486faa77fb`，覆盖审查已通过 |
| 从四个并发客户端重发同一提交，再使用不同意见、错误 SHA、不存在的用例、含查询参数的页面路径或 Worker 身份请求 | 重发幂等，错误绑定与不适合归档的路径被拒绝，读取需 Owner 身份 | 四次重报返回原编号。不同意见、错误 SHA、错误用例均为 409，错误路径 400，Worker 读取 401。PostgreSQL 回归另验证四个同时首次提交的客户端只产生一个编号与一个任务事件 |
| 重启控制面，从原入口查询历史和候选状态 | 保留原反馈及来源，等待 Management 处理 | 两次完整历史响应一致，author 为 owner、状态为 awaiting_review。工作项保持 WAITING_ACCEPTANCE |

预览反馈登记对应 ForgeOS 提交 `17bb81908ff8f730102abb8e2d2931485f2598b6`，启用 PostgreSQL 和 Docker 后 96 项回归通过，零跳过。

## 专题 11 · 浏览器扩展选择元素并提交反馈

使用独立配置目录的 Chrome for Testing 156.0.8078.4 加载仓库的 Manifest V3 扩展。在浏览器触发已安装扩展的工具栏动作，由 Playwright 操作实际预览、确认表单和登录页。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 连接本机工作台，在候选预览进入选择模式，按 Esc，再次进入后点击队列计数 | 取消时退出选择模式；点击后得到可重新定位的元素与原候选 | 定位器 `html:nth-of-type(1) > body:nth-of-type(1) > output:nth-of-type(1)` 在原页唯一匹配显示 0 waiting 的元素。工作台回执记录 Chrome/156、视口 1280 × 720、执行 1 和 SHA `273616480856ec0a7d064ded86f13c032bb01f98` |
| 在确认页选择 TC-001，填写观察并提交；随后从相同预览选择整页反馈 | 保存明确提交的元素反馈；整页草案允许省略定位器 | 反馈 #15 绑定计划 v1 的 TC-001。再次从工具栏打开的整页反馈表单没有元素定位器 |
| 打开带测试查询和片段的页面，选择带虚构私密属性及值的密码框；另以错 SHA 查看表单 | 草案仅含结构和路径；版本发生变化时不得提交 | 草案中没有查询、片段、表单名、ID 或输入值，仍能辨认 input 标签。错 SHA 页面提示重新选择，提交按钮禁用 |

扩展元素反馈对应 ForgeOS 提交 `c561e4022bce42ae4ff79cc9eba436f7966e4cdf`。

## 专题 11 · Management 分类、取证与重启复查

从 Owner 接口为同一预览提交六条分类样例，绑定既有候选 `273616480856ec0a7d064ded86f13c032bb01f98` 与 TC-001。启动两个该项目的 Management Worker，由原生只读 Codex 分别分类。表中描述是分类输入，每条回执均保存了输入原文中的证据片段。

| 操作（提交的反馈） | 预期类别 | 实际编号、类别和 Management 会话 |
| --- | --- | --- |
| 明确确认标题和计数符合 TC-001 | 验收通过 | #16 acceptance · `01a1e453-e1eb-7952-8423-1e4ab0b90789` |
| 描述已约定的计数行为、复现操作与不符的计数 | Bug | #17 bug · `01a1e454-1c16-7042-8609-7c858507ed1a` |
| 要求只修改计数的间距和字号 | UI 修改 | #18 ui_change · `01a1e454-3133-7b82-8ab5-fcd4e06d81a8` |
| 将已约定的容量计算改为包含暂停项 | 需求变更 | #19 requirement_change · `01a1e454-5a51-7973-b98b-fbb996c2f5a1` |
| 请求在当前范围外独立开发历史导出 | 新工作项 | #20 new_work_item · `01a1e454-7894-7e92-8039-7d6268c42f91` |
| 因预期未定义而无法继续，要求澄清 | 阻塞或澄清 | #21 blocked · `01a1e454-9764-7fa1-bddd-66a62f04c861` |

Chrome 打开 #21 的分类依据，显示同一会话。重启控制面后完整反馈历史一致，任务保持 WAITING_ACCEPTANCE。普通 Worker 上报分类返回 401。真实 PostgreSQL 回归另验证过期租约回收、跨项目拒收、无出处引用拒收、相同回执重报与分类事件去重。六条归类不会直接更改候选、执行命令或代替 Owner 验收。

### 本轮验收对照

| 文档要求 | 可复核证据 | ForgeOS 提交 |
| --- | --- | --- |
| 专题 11、14，私网网关的应用与预览路由 | Caddy 2.11.4 双向 TLS，工作台 200/401，内部路径 404，撤销后预览 404。外部 tailnet 单列待验收 | `ec7a462baf075e210456a364a0d697777773695c` |
| 专题 11、13，固定版本与用例关联的反馈 | #7 绑定执行 1、计划 v1、TC-001；并发重报只写一次事件 | `17bb81908ff8f730102abb8e2d2931485f2598b6` |
| 专题 11、ADR-014，浏览器内元素批注与页面上下文 | Chrome 工具栏操作产生 #15；结构定位器可重新选中原元素，虚构的私密值未进入草案 | `c561e4022bce42ae4ff79cc9eba436f7966e4cdf` |
| 专题 11，Management 将反馈归入六类 | #16–21 分别有独立原生会话、原文引用与理由，重启后可核对 | `99de4f602db0095167dd0312a7fbdad41291d673` |

在分类实现完成后的稳定源码上启用 PostgreSQL 18 和真实 Docker，运行 `go test -count=1 -p 1 -json ./...`。9 个包、97 项测试通过，零跳过。`go vet ./...` 和前端生产构建通过。此前一次跨越源码编辑的回归遇到未定义类型，该次运行未作为验收证据。

## 专题 06、11 · 将阻塞反馈转入验收检查点

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从 Chrome 打开反馈 #21，发起关于暂停队列计数的澄清 | 保存分类引用与候选，等待答复期间禁止批准 | 任务 1 进入 WAITING_INPUT，问题 #1 关联反馈 #21、执行 1 和候选 `273616480856ec0a7d064ded86f13c032bb01f98`。提前批准返回 409 |
| 在任务页确认暂停处理会保留计数，再回到预览下读取答复；三个客户端重放原请求，随后重启控制面 | 恢复同一候选，保留问题和答复各一份 | Chrome 显示原答复，任务恢复 WAITING_ACCEPTANCE，执行仍为 1。重启前后的完整预览反馈记录一致 |
| 改用错误 SHA、错引用、UI 修改反馈或 Worker 身份 | 拒绝错误绑定与身份，不写入问题 | 前三种请求返回 409，Worker 返回 401。PostgreSQL 回归另验证既有普通问题与反馈问题使用相同字符串键时，两者均可独立查询和回答 |

实现提交为 `bf272410bb833322a04a74b777b920ea46a6341c`。

## 专题 06、11 · 从预览反馈退回修订并验收新候选

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 执行 1 的逐用例实测尚未完成，提交验收类反馈 #16 请求批准 | 批准继续受原有门禁约束 | HTTP 409，状态仍为 WAITING_ACCEPTANCE。错误 SHA、重复编号、混入新工作项的退回及仅引用验收反馈的退回也返回 409；Worker 身份返回 401 |
| 在 Chrome 任务页选中 #7 和 #18，填写 Owner 修订说明并退回 | 一次决定同时关联两条反馈，新执行消费修订说明 | 任务 1 进入 READY。两个旧预览记录都能读到该说明。调整编号顺序重放成功，减少一个编号返回 409 |
| 启动真实 Worker 修订计数样式和无障碍名称，打开执行 2 的静态预览 | 生成新的已验证候选并实际显示要求的效果 | 原生会话 `01a1e462-92c6-71f0-9879-0b10559a13d1`，Docker V2 通过。相对旧候选 `273616480856ec0a7d064ded86f13c032bb01f98` 仅变更 `public/index.html`。新候选 `b7fb608448ee82653a0cef95b6014d2a0e2cc844` 保持冻结基线 `0f5ef712f527768825a74590e6d3e17ff2c85568`。Chrome 读到标题 Candidate queue、名称 Waiting queue count、值 0 waiting、20px 字号及 12px 顶边距 |
| 在新候选登记通过反馈 #22，待 Management 分类后在 Chrome 引用并批准，重启控制面 | 旧反馈不能验收新候选。新反馈与批准记录可追溯 | 复用旧编号返回 409。Management 会话 `01a1e463-b845-7b31-abeb-4a53207a63a9` 判为 acceptance。任务进入 ACCEPTED，执行 1 的退回与执行 2 的批准分别保留。此项目使用本机交付策略 |

本阶段在 PostgreSQL 18 和真实 Docker 下运行 `go test -count=1 -p 1 -json ./...`，9 个包、98 项测试通过，无跳过。`go vet ./...` 和前端生产构建通过。

实现提交为 `7cbd8a3539800ca521007780cc5b5dabafa3c731`。

## 专题 11、ADR-014 · Canvas 内位置批注

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 打开 Docker V2 验证后的图表候选，从 Chrome for Testing 工具栏选择画布上的队列标记 | 能把点击位置与真实候选和元素一起带回工作台 | 执行 1、提交 `d5179fb57031f553c6972649ccb683c517128cf8`。画布为 400×200 像素，显示为 200×100 CSS 像素。在显示坐标 (50,75) 点击时，草案显示左 25.00%、上 75.00%，结构路径唯一指向 canvas |
| 从工作台提交并重新打开反馈地址，再重启控制面 | 历史中可区分同一画布上的不同位置，重试只写一次 | PostgreSQL 中反馈 #1 的 `canvas_point` 为 `25.00% 75.00%`，普通历史页可见。重试复用 #1，重启前后记录一致 |
| 提交超过 100% 的坐标，或把该位置关联到 div | 拒绝无效批注，保留已有观察 | 两次均返回 400，反馈总数仍为 1 |

独立扩展探针在首次运行时出现工具栏弹窗调试连接超时。调整探针等待弹窗关闭的时序后，连续两次从空白 Chrome 配置通过 Canvas、普通元素、Esc 取消和隐私字段回归。

Canvas 实现提交为 `5a5d685f0e70b600c7a7ca8f97442f82c19aae89`。包含该实现的阶段回归启用 PostgreSQL 18 和真实 Docker，9 个包、98 项 Go 测试通过，零跳过。`go vet ./...` 和前端生产构建通过。上方专题对照表中的 52 个不同 ForgeOS 提交均已通过本地 Git 对象核对。

## 专题 06、11、12 · 从已分类反馈修订需求

通过私有 beta 项目的正式 GitHub 导入入口派发手册任务。真实 Worker 生成首版页面，Management 在会话 `01a1e472-afef-7290-8fa7-5477682cf535` 将预览反馈 #1 归为需求变更。[验收 Issue #19](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/19)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 Chrome 的反馈页读取需求 1 并编辑完整正文，保存修订 2，再次从 GitHub 同步 | 保留原始需求，当前执行继续绑定第 1 版 | Issue 正文仍为“暂停项计入容量”。任务处于 WAITING_ACCEPTANCE，候选仍为 `562d84d6262d1aeef80831fe8aaf65e882fb0bea`。本地最新修订为 2，带有来源反馈 #1 |
| 4 个客户端重报同一请求，再发送改写的回执、错候选、错执行、错反馈或过期修订号 | 只写一版和一个事件，拒绝错误引用 | 4 次返回同一修订；改变回执或绑定返回 409，空正文返回 400，Worker 身份返回 401 |
| Owner 在任务页采用修订 2，启动 Worker，在 Chrome 分别打开两版候选 | 新执行采用新规则，旧页面仍可核对 | 执行 1 显示 “Paused jobs count toward waiting capacity.”。执行 2 的原生会话为 `01a1e475-8b5b-7e70-bee0-a66c78349eca`，候选 `908869f3ff4eec2dcdc5e0d93d70df06ab165d1f` 通过 Docker V2，显示 “Only active jobs count toward waiting capacity.” |
| 重启控制面，查询项目历史，在任务需求修订中回到原预览 | 能追溯反馈及完整来源 | 搜索 active 返回修订 2 和反馈 #1。Chrome 从需求历史打开原反馈并看到回到修订 2 的入口。PostgreSQL 回归还验证了运行中保存修订不会改变执行输入，后续真正的 GitHub 编辑会独立存档 |

需求修订实现提交为 `d34260b9d8ed4683867f85d8ddb379556b59cb05`。

## 专题 06、11、12 · 从预览反馈派发独立工作项

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 #19 的新候选提出独立的历史报告文档，等待 Management 分类 | 保留原任务，单独处理新增范围 | 反馈 #2 绑定执行 2、候选 `908869f3ff4eec2dcdc5e0d93d70df06ab165d1f`，会话 `01a1e47b-6786-7602-8571-3bc72d167287` 归类为 new_work_item |
| 在 Chrome 核对独立 Issue 编号并派发，打开新工作项及其反馈来源链接，四个客户端重复派发 | 建立一条可双向追溯的关系，只派发一次 | [独立 Issue #20](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/20) 成为 T-3。错误 SHA、来源任务、分类和原 Issue 编号均返回 409。Worker 调用返回 401。重复操作均返回 T-3，来源事件只有一次 |
| 运行 Worker、批准并等待必需 CI | 新工作项按自己的 Issue 正文开发、验证和交付 | 执行 3、会话 `01a1e481-005b-7803-84fa-3bc015ca8019`。候选 `372bdf01873e958227005a3be85e7d9f55e57fea` 的历史文档包含 completed queue events，Docker V2 通过。GitHub 首次必需检查按验收仓库的工作流故意失败，确认失败步骤后在同一 head 重跑成功。[Actions 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43874497425) |
| 核对合并、两个 Issue 的状态和控制面重启后的重复派发 | 只关闭独立目标，已关闭的目标可用原回执查询 | [PR #21](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/21) 合并为 `638dff62d68f33d0ccc5b36e667d9b029aa95b6c`，T-3 为 DONE，#20 CLOSED。[来源 Issue #19](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/19) 仍为 OPEN，T-1 保持原候选和 WAITING_ACCEPTANCE。重启后再次派发返回同一个已完成的任务 |

首次 Chrome 派发暴露了工作台请求帮助函数仅允许任务路径的问题，修复后通过上述真实流程。本阶段启用 PostgreSQL 18 和真实 Docker 的 Go 回归中，9 个包、99 项测试通过，零跳过。`go vet ./...` 和工作台生产构建均通过。

独立工作项分派的实现提交为 `2b805b4674f863582f51dc97b1324dc8cc103fdc`。

## 专题 11、16 A.8 · 跨框架与嵌套 iframe 元素批注

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 独立 Worker 验证已提交的 React 19.3.1 和 Vue 3.5.45 页面，从正式静态预览入口打开 | 同源嵌套页面可运行，跨来源页面不能嵌入此候选 | 执行 1、候选 `304948ec31c7e6616230934f8d6de1b97f6da2cf` 通过 Docker V2。Chrome 成功渲染两级框架，另一个来源的嵌入收到 frame-ancestors 拒绝 |
| 在一层 React 框架和两层 Vue 框架分别操作按钮，再从扩展工具栏选择并提交 | 保存有序的框架路径与最内层元素，能重新定位到同一按钮 | 反馈 #1、#2 分别存有一段、两段 iframe 路径。逐层重新定位读到 React queue 1 和 Vue waiting 1，选择操作没有触发额外计数。再从空白 Chrome 配置运行，反馈 #4、#5 同样通过 |
| 查看反馈历史、重启控制面并重放；在独立探针里选择期间插入两级 iframe，在内层按 Esc，再选密码框 | 取消清理所有监听器，反馈中仅含结构，重启后仍能追溯执行 | Chrome 历史页显示框架顺序，重放返回同一编号，重启后 API 响应一致。内层 Esc 可取消。框架查询参数及样例密码没有进入草案；包含属性选择器或缺少目标元素的请求均返回 400 |

首轮完整场景在 Chrome 扩展工具栏的调试连接处超时。复用原候选完成诊断复跑后，再以原验收脚本独立跑通。

iframe 选择对应提交 `5279f11e3f93b6eb557580e570c054d9bcb46fd0`。该版本使用真实 PostgreSQL 和 Docker 运行 9 个包，99 项测试通过，零跳过。

## 专题 11、13 · 工作台截图遮挡、标注与归档

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 Chrome 打开上述候选的实际页面，截取 800 × 450 PNG。在反馈 #4 选图，按坐标遮挡 React 按钮，拖动绘制红框，撤销并重新绘制，点击确认 | 确认前文件留在本机，上传图仅保留遮挡与标注后的像素 | 确认前零次上传。提交体内从 (18,147) 起的 103 × 21 像素均为纯黑，红框存在；归档回读图的全部像素与上传图一致 |
| 由 Owner 在历史页查看截图，比较文件系统、API 与回执；并发重传两份 | 固定执行与反馈可追溯，重放保持同一编号和审计事件 | 执行 1、反馈 #4、附件 #1，2731 字节，SHA-256 `b8d921842bcf9d90c1958b9eae8e13ca0f0637ebfd2e3cce77727c53470ecc42`。历史画布显示不透明遮挡，两次返回同一回执，审核事件共一条 |
| 以 Worker 和错误的任务或反馈访问，上传非 PNG；损坏隔离验收目录内归档，重传并重启控制面 | 身份、归属、格式和完整性校验生效，修复后恢复原图 | Worker 401，错误归属 404，错误类型 415，坏 PNG 400。磁盘损坏后 GET 返回 503，重传恢复原始字节和回执。服务重启后历史与文件逐字节一致 |

本次回归在 PostgreSQL 18 和真实 Docker 上运行，9 个包、101 项测试通过，零跳过。Go 静态检查与工作台生产构建通过。

截图编辑与归档的实现提交为 `c382473c31775ebcc074682f8f55ce59adbad745`。

## 专题 14 · 从数据库备份和文件归档恢复反馈截图

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从源控制面读取执行 1、反馈 #4、附件 #1，停写后用 PostgreSQL 18 导出验收 schema，依据回执逐文件验算并在 Ubuntu 打包 | 私有备份包含可配对的数据库、清单和附件 | 数据库归档 123503 字节，SHA-256 `391361939ae5cf3e8b5a5c863dfe140e0b156d40e627bf6e07afa2fc1369d1bb`。图片归档 1267 字节，SHA-256 `0bf3539f9478855c5fc8645d150ddfa91cfcb40a577754b23e0fa7a27c1ec0b8`。备份整套校验通过 |
| 在新初始化的 PostgreSQL 实例单事务恢复，先用空附件目录启动控制面，再从备份解包 | 缺文件时保留反馈记录并报告不可读；恢复文件后可查原证据 | 目标库恢复前该 schema 有零张表。只恢复库时反馈历史一致，取图 503。恢复库导出的清单与源清单相同。解包后逐文件校验通过 |
| 重启隔离控制面，用 Chrome 打开原反馈和截图，同时验证 API | 归档像素、摘要、归属与原权限一致 | Chrome 显示原编号和摘要，遮挡区仍为不透明纯黑。Owner 读回的 2731 字节与源文件逐字节相等，Worker 读取返回 401 |

反馈截图备份与恢复演练对应提交 `42ba8206c286a65cd31e27f31d9f124f646035eb`。

## 专题 07、14 · 在隔离 Linux 环境恢复工作区并继续派发

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 Linux 容器通过 Owner API 登记仓库并派发 `git_head`，独立非 root Worker 完成任务 1。在工作区分别准备已暂存、未暂存和未跟踪的内容，停写并备份 | 数据库、项目克隆与执行工作区可在同一时间点恢复 | 执行 1 的 HEAD 为 `e90616bd3f91593d3098e056b6a721ddd18031b5`。状态为 `MM queue.txt` 与 `?? pending.txt`。工作区和克隆归档共 10859 字节，SHA-256 `142c389fc017eed4f3763ffd5a583a435b146261d8f293a8bae9ec8fe5b62408` |
| 启动独立 PostgreSQL 16.15 实例与空 Linux 卷；校验、解包到原绝对路径，恢复数据库；运行 Git 完整性检查并逐项对照 | Git 对象、工作树注册、索引差异、工作区差异和未跟踪内容一致 | 两端 HEAD、worktree list、status、两种二进制 diff、文件摘要和 fsck 的完整输出相同，SHA-256 `5ff4cf4a0cd68d92934c51d708336219c5e58e97183f5da28835997f5c1b1f86`。恢复控制面返回的任务 1 与源 JSON 完全相同 |
| 在恢复实例通过 Owner API 派发第二项任务 | 独立 Worker 使用新目录，不改变源实例 | 任务 2 完成于 `/state/work/execution-2`，报告相同 HEAD。源数据库仍只有任务 1，源克隆没有 execution-2 工作树 |

本次源、目标使用隔离 Linux 容器和不同的数据卷，服务进程均以 UID 1000 运行。两端绝对路径一致。本项验证 Git 与数据库恢复，运行中的模型进程和原生 Session 恢复仍待验收。

工作区恢复演练对应提交 `d83eb4be8bda77ddd2699317ac360c8c6e8f06dd`。

## 阶段四、专题 07 · 通过正式 Worker 交付 Claude Code 实现任务

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| Owner 从私有 beta 导入 #22，选择 Claude；只声明 Codex 的节点尝试领取，再启动具备 Claude 能力的 Worker | 控制面冻结所选 Harness，仅匹配节点可领取；不支持的组合有明确反馈 | 未知 Harness、Claude 规划请求均返回 400。Codex 节点返回 204，任务随后由 Claude Code 2.1.310 领取，工作项和执行编号均为 1。[输入 Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/22) |
| Worker 在固定工作树修改队列恢复手册，运行候选验证，再在 Chrome 核对执行详情 | 普通任务入口可读 Harness、版本、会话与验证结果 | 唯一改动为 `docs/queue-recovery.md`，正文说明恢复后待办项保持原顺序。会话 `5aad3ae6-5cd1-4e6d-a3eb-eb5a308f94bf`，候选 `669f0718010c3fb944f4ea2c069a4fa704ac7843`，V2 的三项检查通过 |
| Owner 批准，等待受保护分支检查并交付。重启控制面再次查询任务 | 检查失败时保留候选，全部通过后才可合并并关闭 Issue | 首次 Actions 运行的受控失败步骤阻止交付；重跑同一工作流，第二次通过。PR #23 的 `acceptance-check` 与 `forgeos/local-ci` 均成功，Squash Merge 为 `31b1f3a2745a45f51099c8a965b970cc9d7e020c`。#22 已关闭，任务 DONE，重启查询逐字段一致。[PR #23](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/23) · [Actions 运行](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43881504764) |

本次使用 Windows 本机已有的模型网关设置。受限模式默认忽略用户设置，首次探测返回未登录；显式交给 CLI 服务账号的设置文件后，真实编辑与交付成功。未把模型凭据写入仓库或任务回执。启用 PostgreSQL 18 与真实 Docker 的回归测试覆盖 9 个包，102 项通过，零跳过。Go 静态检查及前端生产构建通过。

按工作项选择 Claude Code 并交付的实现提交为 `b11089be5de8948af1eaef40353aebd5a50bed8c`。

## 阶段四、专题 07 · 项目默认 Harness 与标签派发

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 beta 私有库的专用受保护验收分支提交 `project.yaml` 指定 Claude，本机工作区将同一字段改为 Codex 并留作未提交改动。给 #24 添加派单标签 | Poller 使用基础提交，工作台可追溯配置，只有匹配的 Worker 可领取 | 基础提交 `da7796669227f979503c5cdf915e4ac93909ea88`，配置 blob `1008b690a8b5527149b87f5b21563102636e8fde`。Chrome 任务页显示 Claude Code 与该 blob。仅声明 Codex 的节点领取返回 204。[标签 Issue #24](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/24) |
| 由正式 Worker 实现，Owner 核对唯一改动的手册文件后批准，通过受保护分支交付 | 使用项目选定的 Harness，候选和已合并版本可追溯 | 会话 `14e58ae1-e6d6-4788-91a9-ab1b74358aa9`，候选 `c2c051d3e298c9ff1b2df42fd07b64b680ec413e`。PR #25 的 Actions 受控首轮失败后重跑成功，本地 CI 成功；合并到 `codex/accept-project-harness-9c4836`，Merge SHA 为 `81630b95429f0debfd9593544bba181ba0981708`。Issue 已关闭，重启控制面后任务记录相同。[PR #25](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/25) · [Actions 运行](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43884038472) |
| 在隔离克隆提交未知的默认 Harness 并尝试导入。切回原基础，Owner 明确选择 Codex，再按项目默认重放 | 无效声明阻止派发，Owner 显式选择可覆盖默认，已有快照稳定 | 无效配置返回 409 且任务数为零。显式请求得到 Codex 与原配置 blob，重放后任务逐字段一致 |
| 在独立数据库按规划模式导入，Owner 保存并激活一项计划，用两种节点声明分别尝试领取子项 | 规划和子项分开选择能力；子项保留来源配置 | 父项 1 归档默认子项 Harness 为 Claude。Owner 激活修订 1 后子项 2 为 Claude 并保留原 blob。Codex 领取返回 204，Claude 领取返回 200；此路由演练用受控失败结束领取 |

本次改动完成后启用 PostgreSQL 18 与真实 Docker，Go 全量回归 9 个包、103 项通过，零跳过。Go 静态检查和工作台生产构建均通过。

项目默认 Harness 对应提交 `22c724a04e53c53a48e43cfe5ca3a760c08eb6f5`。

## 阶段四、专题 03、07 · Worker 登记、心跳与节点工作台

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在空 PostgreSQL schema 启动控制面与持有项目专用令牌的正式 Worker，派发 Git HEAD 任务，从 Chrome 打开“执行节点” | 同一节点可关联身份、能力、资源与已完成执行 | 初始列表为空。节点 registry-edge 报告 windows/amd64、16 个逻辑 CPU、1 个名额，启动探测给出 Codex、Claude 和规划能力。执行 1 回报 `65d40b2a22dcec366c5146ed5924c607b09f7cbd`。页面显示在线及已占名额 0 / 1 |
| 等待第二次心跳，停止节点，等待超时，再依次重启控制面和节点 | 实际超时后离线，重连复用记录，查询保留首次登记时间 | 首次登记 22:24:53，第二次心跳 22:25:08。超过 45 秒后显示超时。重启控制面后离线记录逐字段相等，22:25:53 重连时首次登记时间相同 |
| 使用其他节点 ID、零名额和节点令牌分别测试登记与列表。再从真实 Caddy 双向 TLS 入口访问 | 授权、资源校验和网关路由分别生效 | 伪报另一节点 403，零名额 400，Worker 读取列表 401。持有客户端证书经 Caddy 访问列表时 Owner 200、缺少 Owner 令牌 401，访问内部登记路径 404 |

网关演练首次在预览环节打开了历史任务的另一条执行。按回执中的执行编号打开后，Chrome 的节点页、候选预览和撤销均通过。启用 PostgreSQL 18 与真实 Docker 后全量回归 9 个包、103 项通过，零跳过。Go 静态检查和前端生产构建通过。

节点登记与心跳对应提交 `611850ea12f91417f6e2b5e5167a7eb1acc36a7c`。

## 阶段四、专题 06、07、14 · 跨项目节点容量与失联占位

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| Windows 启动一个单名额 Worker，派发两个不同项目的 Git HEAD 任务。在专用 PostgreSQL schema 用锁延迟结果确认，并以同一节点身份并发发送八次领取 | 已领取的任务占用唯一名额，Worker 忙碌时继续上报心跳；确认后继续派发 | 任务 1 运行、2 就绪，八个领取请求均返回 204。心跳持续更新，释放结果锁后两个任务均完成，回执匹配各自的 Git SHA |
| 把主节点设为两个名额，向三个项目派发四项，再启动独立令牌的单名额第二节点。在 Chrome 检查节点页后释放结果锁 | 同项目串行，不同项目可分配给有空位的节点，工作区互相独立 | 主节点同时执行 3、5；同项目任务 4 排队。第二节点领取第三个项目的任务 6。页面显示占用 2/2 与 1/1。放行后前六项各执行一次，六个工作区独立，全部回执与所属仓库 HEAD 相符 |
| 以主节点令牌领取任务 7 后停止回报；重启控制面和同 ID 的正式 Worker，等待两分钟租约自然到期。随后 Owner 通过执行接口放弃 7 | 失联执行持续占用名额，Owner 处理后释放 | 到期后 7 仍运行，另一项目任务 8 仍就绪，再次领取返回 204。放弃后 7 失败，正式 Worker 领取并完成 8 |

本次使用三个本地 Git 仓库，未制造远端 PR 或合并副作用。三个 HEAD 分别为 `f324698324ad1248913694bad9cc35e6f72f1d46`、`229f9e3522c1cc1563fbd2795a2a383fea6a2ba1`、`ae39c7d32ef4217982738dddde763674e4d75f7c`。启用 PostgreSQL 18 与真实 Docker 后全量回归 9 个包、104 项通过，零跳过；Go 静态检查通过。

节点容量与失联占位对应提交 `9f6d39672a24da8ee6b989f722b5b779df49f276`。

## 阶段四、专题 06、07 · 按登记能力派发

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 通过 Owner 入口导入私有 beta #26，沿用项目声明的 Claude 和自动验证；以项目专用令牌登记只支持 Codex 的节点，再在领取请求中声明支持 Claude | 节点缺少目标 Harness 时不生成执行，控制面重启后继续使用已登记能力 | 重启前后均返回 204，占用为零。此前尝试导入已关闭的 #24，入口按设计返回 409，随后创建开放的验收 Issue。[Issue #26](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/26) |
| 登记支持 Claude 但缺少声明式检查的节点；随后登记完整能力，分别在领取中关闭风险策略和 Claude | 心跳和本次请求都须满足候选条件 | 三次均返回 204；任务 1 保持 READY，尚无执行历史 |
| 由同一身份上报并请求完整能力，检查工作项和名额，回报受控失败 | 匹配请求只产生一次执行，回报后释放名额 | 领取返回 200，执行 1 指向任务 1，快照指定 Claude。已占名额先为 1，确认结果后为 0。此场景验收到派发边界，随后关闭验收 Issue |

测试夹具补上自动验证策略快照后，启用 PostgreSQL 18 和真实 Docker 的全量回归共 9 个包、105 项通过，零跳过。Go 静态检查通过。

登记能力派发对应提交 `aaaf99292e7cc038c66f7deb786d1f5e2d4385aa`。

## 阶段四、专题 12、16 B.8 · GitHub 写入成功后丢失响应

正式控制面、Windows Worker 和 PostgreSQL 18 使用隔离的验收数据。只在验收控制面的进程环境里配置了 GitHub CLI 包装器。包装器调用真正的 `gh`，对指定仓库和 Issue 的四个写入分别丢弃第一次成功返回的响应。每次从任务 API 看到远端操作尚未确认后，停止并重启控制面。验收程序记录每种写入的成功次数，并另行查询 GitHub 和数据库核对最终结果。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从私有 beta #27 建立任务，由项目选用的 Claude 完成手册更新，核对差异和本地 CI，再批准执行 1 | 交付固定于已验证的 SHA | 仅修改 `docs/delivery-receipt.md`，说明如何核对验收和合并提交。基础 `81630b95429f0debfd9593544bba181ba0981708`，候选 `c856cf590ebf8421581d39edc7dd772c1ed98d43`，会话 `58168bb8-3701-46c4-8584-a0c0f5e406a0`。[Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/27) |
| 22:44:57 丢弃创建 PR 的响应，在交付仍为 PENDING 时重启 | 按交付分支找到已有 PR，遵守必需检查 | 复用 PR #28。确认首次 Actions 失败来自受控故障步骤后重跑，第二次成功；同一候选的 `forgeos/local-ci` 成功。[Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43889008466) |
| 22:46:22 丢弃受保护分支 Squash Merge 请求成功响应，在 WAITING 时重启 | 查询已合并 PR，恢复实际 Merge SHA | PR #28 已合并到 `codex/accept-project-harness-9c4836`，远端和本地均记录 `8a4e6a86c118e658bfefc5dcfd3c21f0aedf6091`。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/28) |
| 22:47:06 丢弃发布评论响应，22:47:43 丢弃关闭 Issue 响应，分别在 MERGED 时重启；交付完成后再次重启并比较任务 JSON | 复用交付评论、确认关闭状态，最终任务和事件唯一 | 四种成功远端写入各一次。同分支只有 PR #28，Issue 已关闭且只有一条评论，包含 PR、候选和合并 SHA。任务 DONE，最终重启前后 JSON 相等。`pr_created`、`pr_merged`、`delivery_completed` 事件各一条。[交付回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/27#issuecomment-6919716568) |

时间均为 2026-11-16 Asia/Shanghai。本次复用既有交付协调逻辑，相关实现提交为 `1a1782246225ab810ca34f3a5c1fd73194242ab8`、`544744520aeb6ea13724c7aaf4166ebf0a5c09b0`。实测二进制对应 `aaaf99292e7cc038c66f7deb786d1f5e2d4385aa`。Worker 进程中断后的原生会话恢复另行验收。

GitHub 四类写入的真实恢复验收对应提交 `aee11fb07db8c6fd3addf12da819bd02a790788c`。

## 阶段四、专题 07、13 · Worker 断线回报与本地恢复日志

在独立 PostgreSQL 18 schema 使用项目专用节点令牌，经正式 Windows Worker 领取三个 Git HEAD 任务。测试代理仅改变结果请求的传输：第一种返回 503 且不送达，第二种向真实控制面送达请求后断开响应。每次观察本地 SQLite 已提交待报结果后，强制终止 Worker，停止控制面，再重新启动两个进程。三个任务期望的仓库 HEAD 均为 `4208e82bbd0c537cb4a0c8693bdcbc1e9e01051d`。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 执行 1 的回报未送达；控制面与 Worker 均重启 | 使用原执行补报，控制面从 RUNNING 变为 DONE | Worker 在领取新任务前发送同一正文，收到 204。候选等于仓库 HEAD，本地记录 confirmed，清除待报正文 |
| 执行 2 的回报已生效，丢弃响应后重启 | 幂等确认，没有第二次执行和新增事件 | 补报返回 204。重启前后完整任务 JSON 相等，只有一次尝试和三个任务事件 |
| 执行 3 无法送达，停止 Worker，等待两分钟租约到期。Owner 从执行 API 放弃，再重启 Worker | Owner 决定生效，迟到回报留存冲突证据 | 收到 409；任务保持 FAILED。SQLite 记录 conflict，保存原候选回执并移除租约令牌。三个任务共三个工作区，均只执行一次 |

Ubuntu 非 root 用户另行运行原生 Linux Worker 测试，覆盖 503 重试、重开数据库补报及 409 冲突。首次运行发现测试框架创建的目录权限较宽，改用服务自行创建的私有子目录后通过。持久缓冲覆盖已产生的执行结果；Harness 在结果写入前崩溃的会话恢复待验收。

本次代码在 PostgreSQL 18 和真实 Docker 环境全量回归 9 个包、108 项通过，零跳过。Go 静态检查通过。

Worker 断线回报与恢复日志对应提交 `ab36ad9759dbe700098aea7df33bb33c63db0c85`。

## 阶段四、专题 07、16 B.5 · 正在执行的任务独立续约

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 两个项目使用各自的节点令牌。节点二持有执行 1；节点一尝试续约，随后节点二用错误的执行令牌尝试 | 项目授权和当前执行令牌均须匹配 | 跨节点返回 403；正确节点提交错误令牌返回 409。领取回执报告 Git 的租期为 120 秒 |
| 正式 Windows Worker 在项目一执行 Git HEAD。验收程序将 Git worktree 命令延迟 43 秒后调用真实 Git，使任务跨过 30 秒续约周期。在执行途中重启控制面 | 同一次执行延长租约并回报真实候选 | 执行 2 的截止时间由 23:06:46.912344 延长至 23:07:16.922013，随后 DONE。只有一次尝试，回执 HEAD 为 `9c21a277e8b4f1473691801658b57d05644d6e63` |
| 对执行 3 使用同样的延迟，仅在验收 schema 缩短该行的租约，等正式 Worker 发送下一次续约 | 过期租约不能复活；正在等待的操作被取消，并留下原因 | 续约返回 409，Worker 回报 `execution lease renewal rejected (HTTP 409)`。任务 FAILED、尝试一次，未产生 Git 工作区 |
| 让节点二持有的原始租约自然到期，再次续约。Owner 核对并放弃后，再尝试一次 | 到期和人工结束都排除续约；到期本身保留原记录 | 两次均为 409。Owner 放弃前状态 RUNNING，放弃后为 FAILED |

实测时间为 2026-11-16 Asia/Shanghai。第一次演练在缩短测试行时使用了另一种数据库客户端的连接参数，修正验收程序后在全新 schema 重跑全部步骤。数据库回归另行验证续约等待行锁时跨过截止时间，Worker 回归验证连续服务不可用直至本地确认期限耗尽。此处验证运行中执行的租约，原生模型会话重连仍待验收。

启用 PostgreSQL 18 和真实 Docker 的全量 Go 回归共 9 个包、112 项通过，零跳过。Go 静态检查通过。

运行中的执行续约对应提交 `6e865cec3ee03cbfb541145230e1d75f60b5dbd1`。

## 阶段四、专题 07 · Claude 适配器中断后的同会话恢复

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在私有 beta #29 通过正式 Worker 调用 Claude Code 2.1.310 编辑手册。验收适配器转发真实事件，在原生 CLI 完成后丢弃终态并以 1 退出 | Worker 确认已有会话及进程退出，核对租约、原工作区和起始提交，只恢复原 Session | 两个原生进程 PID 为 51084、36232，均确认会话 `bc51f9e0-f24b-481f-8a15-c4f9003ef87b`。SQLite 记录其直接启动的两个适配器 PID 47864、14580，退出码分别为 1、0。[Issue #29](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/29) |
| 对照原编辑与恢复后的文件，检查正式执行回执、本地验证、保护检查和最终回写 | 已有编辑保留；同一工作项、执行和工作区产生一个已验收候选 | 两次文件 SHA-256 均为 `35f73b387974ab65966d11e1ab4199a1aec540c43a2ccd9028104bcd9efb3086`。任务 1 只有执行 1，候选 `b2e8f370cc092e2935d8de3bfb13970cc81265b5`。PR #30 在本地 CI 和 Actions 均成功后合并为 `8e076bfca9fc2bf3ab9231913ae89dc3f9d50b66`。Issue 已关闭，重启控制面后任务 JSON 相同。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/30) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43894562492) · [回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/29#issuecomment-6920510928) |
| 回归夹具进程写入部分内容后自行以 23 退出，分别允许或拒绝续约；另输入错误会话和明确的模型失败事件 | 续约成功时追加剩余编辑，失败时保留原现场；已报告的模型失败不恢复 | 成功分支在同一文件保留两次编辑，两个进程退出码 23、0；409 分支只有一个进程且没有候选。错误会话和终态失败分别被识别 |

真实环境覆盖适配器在终态传输丢失后退出。自动审批拒绝了验收命令中按 PID 强制终止原生 Claude 的操作，返回“blocked by policy”。原生 CLI 被强制终止、节点重启后恢复及 Codex 的同类场景保持待验收。首次查询两张 SQLite 表时列名有歧义；修正验收查询后复用原候选完成交付。真实交付演练期间补充了结构化失败与异常退出的区分，这一分支由最终版本回归覆盖。PostgreSQL 18 与真实 Docker 上全量回归 9 个包、116 项通过，零跳过；Go 静态检查通过。

Claude 中断回合恢复对应提交 `d78ae18dc75305efe6e36386356e5358be96ea16`。

## 阶段四、专题 04、07、13 · 进程记录归档与 Owner 查询

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从私有 beta #31 派发项目默认的 Claude 实现任务，读取运行中的原生可执行文件与 PID，待执行结果产生后比较控制面回执 | 同一次 Execution 关联真实进程、原生会话、起止时间与退出结果 | 直接运行 Claude Code 2.1.310，观测 PID 49780。执行 1 归档的实例号为 1、PID 为 49780、会话为 `1930470a-c11a-4a00-9d55-72f05f83a321`，起止时间分别为 15:22:15、15:22:39 UTC，退出码 0。[Issue #31](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/31) |
| 在验收代理中先把进程会话改成另一值，随后送达完整原始回执并丢弃成功响应，让正式 Worker 重报 | 拒绝不匹配的证据，正确结果和过程记录幂等写入 | 篡改请求 409。两次原始正文摘要相同，均返回 204，最终只有一条尝试和一个进程实例 |
| 停止 Worker，通过 Chrome 以 Owner 身份展开对应执行的进程记录，核对 PID 与原生会话，批准候选并等待交付，重启控制面 | 不依赖在线节点即可读取记录；归档与交付关联稳定 | Chrome 可见 PID 和 Session。候选 `71c36f2e15a7237ce29ac722ee523868b5808e1d` 的受保护 PR #32 经两项必需检查后合并为 `7703deef8476a79fa6e6dd3c5f964de8aed52d14`。Issue 已关闭，完整任务 JSON 在控制面重启前后相等。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/32) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43895866795) · [交付回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/31#issuecomment-6920701144) |

首次浏览器定位同时匹配了外围执行和内部检查的折叠控件。改为选择进程记录自己的标题后，从同一候选继续验收。当前中央工作台显示已回报的 Claude 执行；运行中状态仍须查询节点日志。PostgreSQL 18 与真实 Docker 全量回归 9 个包、117 项通过，零跳过。Go 静态检查及工作台生产构建通过。

Claude 进程记录归档与工作台对应提交 `91a150bc0759fb8449707dbfb1dfa2e126679d9a`。

## 阶段四、专题 04、07、13 · Codex 实现与只读规划的进程回执

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在项目配置默认 Claude 的私有 beta 库中，为 #33 明确选择 Codex 实现。比对运行中的原生可执行文件、SQLite、控制面 API 和 Owner 工作台 | 实际 PID、完整版本、原生 Thread 与退出结果一致 | 四处均指向 PID 17244。版本 `0.169.0-alpha.11.1`，Thread `01a1e4f9-de2d-74e1-9df3-9d9eca1821fd`，退出码 0。错误 Thread 的回报返回 409；正确回报的响应丢失后，相同正文重报得到 204。[Issue #33](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/33) |
| 核对唯一改动的手册、固定候选和本地 CI，批准并等待受保护分支交付 | 一次尝试产生同一候选和已确认合并回执 | 候选 `c4c6e7e5840afb74e41d6a2c2b6dc5febf964e1a`，PR #34 两项检查均成功，合并为 `8124de0fae6c74100b3ad9e25b59f56386ad2fa6`。Issue 已关闭，重启控制面后任务 JSON 相同。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/34) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43897051354) |
| 在独立 schema 中，于同一 Issue 开放期间以规划模式导入。正式 Worker 只读生成计划，在 Chrome 展开进程并重启控制面 | 规划保持基础提交不变，执行绑定规划进程和版本 | 实测 PID 2004，Thread `01a1e4fa-6876-7100-879e-2f5fd64804b3`，版本相同，退出码 0。任务为 PLANNING，修订 1 与执行计划相同，工作区干净、HEAD 仍为 `7703deef8476a79fa6e6dd3c5f964de8aed52d14`。重启后任务 JSON 相同 |

进程记录在执行结果回报后进入中央工作台。独立管理、审查进程及节点重启后的在途会话恢复仍待验收。

启用 PostgreSQL 18 和真实 Docker 后，全量 Go 回归 9 个包、118 项通过，零跳过。Go 静态检查及工作台生产构建通过。

Codex 实现与只读规划的进程回执对应提交 `39d3f6899ab5c2a75bf72e635b349788e2ae1b64`。

## 阶段四、专题 07 · Codex 终态传输丢失后恢复原 Thread

2026-11-16，在 Windows 用正式 Worker、Codex CLI 0.169.0-alpha.11.1 和私有 beta #35 验收。适配器在首个原生 CLI 正常退出后丢弃 `turn.completed` 并以 1 退出；恢复进程转发完整事件。分别核对原生会话中的两条 `turn_context`、原生 PID、适配器退出码和 Owner 任务页。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 选 Codex 实现 #35，丢失首个终态，让 Worker 自行恢复 | 同一 Execution、Thread 和工作区完成授权改动，保留原权限 | 原生 PID 35548 → 17848，Thread `01a1e4ff-b270-7df1-8696-df3e87d4a033`。两次均为 workspace-write、网络关闭、审批 never。适配器 PID 45196 → 868，退出码 1 → 0。只有一次执行，候选只修改指定手册。[Issue #35](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/35) |
| 提交错误进程会话、重复提交正确回执，经 Owner 验收后等待受保护分支交付 | 错误回执被拒，正确回报幂等；两项必需检查通过后才可合并 | 分别为 409、204。候选 `e8a172b7e5f2d8ae9526352131236c8b23faa300` 通过本地 CI；Actions 首次按演练失败，重跑通过，PR #36 合并为 `4e1d4b148c06c4575effd6c4b16bd5f675c90b4e`。Issue 已关闭，只有一条交付评论。控制面重启后任务 JSON 相同。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/36) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43898251585) · [交付评论](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/35#issuecomment-6921039550) |
| 在独立 schema 中将同一开放 Issue 作为只读规划导入，重复丢失首个终态 | 同一只读 Thread 在干净的固定基础提交重新输出计划 | 原生 PID 38656 → 3284，Thread `01a1e4ff-cc08-7562-a8f5-b4fdc03e83f0`。两次均为 read-only、审批 never，工作目录一致。适配器 PID 37724 → 50792，退出码 1 → 0。修订 1 与执行结果相同，工作区干净，HEAD 保持 `8124de0fae6c74100b3ad9e25b59f56386ad2fa6`，控制面重启后可复查 |
| 回归夹具写入半个文件并以 23 退出，再分别返回原 Thread、错误 Thread 和明确失败事件 | 成功时保留已有编辑；会话身份不符或模型明确失败时禁止产生候选 | 原 Thread 补全了同一文件，另外两种情况均返回失败且无候选；明确失败场景没有续约请求 |

原生模型进程被强制终止和 Worker 自身重启后的在途恢复仍待验收。

启用 PostgreSQL 18 和真实 Docker 后，全量 Go 回归 9 个包、122 项通过，零跳过。Go 静态检查通过。

Codex 终态传输丢失后恢复原 Thread 对应提交 `9f83fff323be37a01b9066ec6d73578ac424834b`。

## 阶段三、专题 06、07 · 在原 Codex Thread 处理运行中的 Issue 评论

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在私有 beta 导入 #37，指定 Codex。观察正式 Worker 启动原生进程后，通过 GitHub 在 Issue 下补充手册示例，让正式轮询器采集评论 | 普通评论进入下一个安全检查点；原回合正常结束，后续回合沿用同一 Thread | 三个检查点依次收到 0、1、0 条评论。前两个确认同一 Thread `01a1e506-342b-76f2-873d-4a375463810f`，空的末批没有开启第三个进程。该执行的两个原生 PID 为 35272、35092，均正常退出。两条原生回合记录的工作区相同、审批为 never、workspace-write 禁网。[原始评论](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/37#issuecomment-6921147784) |
| 在任务详情展开进程记录，核对候选差异和本地 CI，提交候选验收 | 一次执行覆盖原始需求和评论补充，只产生一个候选 | Chrome 可见同一 Thread 的两条进程记录。候选 `48f935199c017f96492f3fa1ead4d1b860793ac9` 只修改指定手册，包含原需求的验收和合并说明，以及评论要求的完整示例句。错误进程回执为 409，正确回报在丢失响应后重报为 204 |
| 等待本地 CI 和 Actions，核对受保护分支、Issue 评论和重启后的控制面任务 | 合并已验收的候选，保留原讨论和交付回执 | Actions 第一次按演练失败，重跑成功。唯一的 PR #38 合并为 `ca1953f7d1e8ac05e22b8e82612b2a032503dd18`，Issue 已关闭，保留原评论和一条交付回执，重启后任务 JSON 相同。[PR](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/38) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43899626076) · [交付回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/37#issuecomment-6921230220) |

验收代理在下一批次请求前等正式轮询器读取评论，以复现实测时序。原生 Codex 直接由 Worker 启动。启用 PostgreSQL 18 和真实 Docker 后，全量 Go 回归 9 个包、123 项通过，零跳过。Go 静态检查通过。

普通评论沿用原生 Thread 对应提交 `97b20b20cb962ecfdc441244dae880858fa3d6b5`。

## 阶段四、专题 09 · 两个交付项目的执行与业务凭据隔离

2026-11-16 至 17 日，Windows 上的两个节点分别领取私有 alpha #14、beta #39。共用 PostgreSQL 18 和正式控制面程序，使用真实 Codex CLI 和 Docker 中启用 TLS 的 OpenBao 2.7.2。两边在 ci-test 环境使用同名 receipt-signing、相同用途和 KV 版本 1，各自生成随机测试密钥。验收代理在两次正式领取后，以对应节点身份调用 Broker；解包和签名比对在受信验收进程内完成。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| Owner 为两个运行中的工作项分别批准一次使用。在两个方向试用另一节点身份、另一执行的租约、另一执行编号和 staging 环境 | 均拒绝，不消耗本项目的授权 | alpha 为项目 1、任务 1、执行 2、授权 1；beta 为项目 2、任务 2、执行 1、授权 2。两个方向依次返回 403、409、409、403，随后查询各授权均为零次使用 |
| 用各自身份和租约领取包装响应，解包并对同一随机挑战计算 HMAC，再次解包；执行结束后重试领取 | 各签名只匹配自己的项目。一次性包装和执行结束门禁生效 | 两份签名分别通过本项目校验，交换预期密钥后均不匹配。第二次解包返回 400，执行结束后领取为 409。两份审计各有一次 ISSUED，任务及审计 JSON 在控制面重启前后相同 |
| 核对两个候选的文档差异，分别从对应交付入口批准并等待受保护分支 | 每项只改本库指定文件，合并自己的候选、回写并关闭来源 Issue | alpha 的 docs 检查通过，候选 `510365200cdda471b03ffcd01b4d4f4bb9257790` 合并为 `f5db71cfa976dcc019bf06738e531b98f1edbb63`。[PR #15](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/15) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/43902625675) · [回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/14#issuecomment-6921648615)。beta 的三个声明式检查通过，候选 `072b5953a9477c8126281794893e693f7a213126` 合并为 `af98f5d6022c3ccf5374cf8cffb0ffe938121c10`。[PR #40](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/40) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43902399671) · [回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/39#issuecomment-6921616298) |

首次将 alpha 配为 auto 校验时，其基础提交缺少 validation.yaml，正式 Worker 返回 CONFIG_FAILURE。另一次将 beta 的批准提交到仅对 alpha 开启交付的控制面，按现有配置只记录本地验收。最终演练使用各自项目的验证配置和交付入口。beta 端口首次绑定失败，经重新确认两个服务均可访问后，复用原候选完成交付。最终演练没有重新扫描数据库导出和日志中的明文；该项既有证据见专题 09 的 Broker 验收。

新增数据库回归还让同一节点同时持有两个项目的租约，交换授权和执行时均被拒，读取另一项目的回执返回未找到。启用 PostgreSQL 18 和真实 Docker 的全量 Go 回归 9 个包、124 项通过，零跳过。Go 静态检查通过。

双项目凭据隔离与交付验收对应提交 `81e2b1139c30e61856aa2b81db6fd1e43f265f97`。

## 阶段四、专题 12 · 单控制面按项目选择交付分支

2026-11-17，通过一份操作员配置让同一个控制面同时负责 alpha 的 main 和 beta 的 codex/accept-project-harness-9c4836。使用各自项目的节点、真实 Codex、Chrome Owner 工作台和受保护的 GitHub 分支。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从同一控制面导入 alpha #16 和 beta #41，分别核对页面差异和目标分支，点击通过验收 | 两项批准各自冻结所属项目的目标，后台独立等待对应仓库的检查 | 两份批准 HTTP 200，响应中的目标分支分别为 main 和 codex/accept-project-harness-9c4836。两次 Actions 首轮按演练失败，重跑完成后才合并 |
| 从原任务核对 alpha 的必需检查、状态回执和 Issue 关闭 | 只交付 alpha 自己的候选 | 执行 1，候选 `53e4185a05b093d9f8f95b264aab7d4f317bb118`，状态回执 59043755074，合并为 `5592a34169cd7fd7f7c8874b1f65b01a79fe8f8d`。[PR #17](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/17) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/43903355852) · [Issue 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/16#issuecomment-6921749246) |
| 从同一入口核对 beta 的两项必需检查、状态回执和 Issue 关闭，重启控制面 | beta 使用自己的固定分支；两项交付记录可恢复 | 执行 2，候选 `f559c4f5a431fe00978ce740578f049db3a55c6d`，状态回执 59043754713，合并为 `5d40f83c39bb0b51537ab4d03ba4e10061e2f6ba`。重启前后两份完整任务相同。[PR #42](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/42) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43903355714) · [Issue 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/41#issuecomment-6921749145) |
| 重启时仅启用 alpha，重放 beta 的原批准、发布状态和刷新请求；再将单项目环境变量和文件配置指向同一项目的不同分支 | 原验收可核对，已停用项目不能执行新远端操作；冲突配置启动失败 | 原批准重发返回相同任务；beta 的状态发布、交付刷新均返回 403。alpha 重用原状态回执。分支冲突时控制面非零退出并报告冲突 |

两库实际使用同一个 GitHub CLI 账号。每项目 GitHub App 身份和多项目自动轮询仍待验收。启用 PostgreSQL 18 和真实 Docker 后，全量 Go 回归 9 个包、125 项通过，零跳过。Go 静态检查通过。

单控制面多项目交付对应提交 `977910ee99f313832a367dd4ccd3cc17aee2c7ac`。

## 阶段四、专题 12、ADR-002 · 单控制面同时轮询两个私有仓库

2026-11-17，先登记 alpha 和 beta，分别在 GitHub 创建带 forgeos-agent 标签的 Issue。alpha 之前没有该标签，在其验收库中创建后复用。以下步骤均从控制面正式轮询入口开始，两个工作项都由后台自动导入。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 起初只配置 alpha 的 docs 轮询，再把 beta 的 auto 配置加入文件并重启 | 首次只导入 alpha；第二次保留原 ID，按 beta 自己的规则新建工作项。讨论按仓库进入收件箱 | 首次只有 alpha #18、任务 1，beta 收件箱为空。启用两项后 alpha 仍为任务 1，使用 Codex 和 docs；beta #43 成为任务 2，使用项目默认 Claude 和 auto。alpha 收件箱只有普通讨论 #2；beta 收件箱为 #19、#2。所有链接均属于对应仓库 |
| 停止控制面，分别给两个 Issue 添加普通评论。重启并等待真实扫描，再启动两个具名节点 | 各工作项收到本库评论，重复扫描仍只有两项工作 | 每项都保存了一条对应的原始评论。工作项 ID 保持 1、2，最终各只有一次执行。[alpha 评论](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/18#issuecomment-6921873049) · [beta 评论](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/43#issuecomment-6921873963) |
| 在 Chrome 以 Owner 核对两份文档差异并批准，待 GitHub 必需检查重跑成功后核对 PR、回写和控制面重启 | 两库分别交付按本项目规则生成的候选，保留讨论和交付评论 | alpha 候选 `eca73bb07027627a0640121d21b2a004662569d3`，合并 `8c8a314b6a72c939f0d9c214c25c016f7376f530`。[PR #19](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/19) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/43904786638) · [回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/18#issuecomment-6921948584)。beta 候选 `4bd43cb76582ac651cad9e09f37a6650e42d4fc8`，合并 `42c39e9698aebb2fc6f05080f2f045ff206fde5c`。[PR #44](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/44) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43904791081) · [回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/43#issuecomment-6921950067)。两项均已关闭 Issue，重启前后任务 JSON 相同 |

启用 PostgreSQL 18 和真实 Docker 的全量 Go 回归 9 个包、126 项通过，零跳过。Go 静态检查通过。两库共用 GitHub CLI 身份；GitHub App 最小授权仍待实测。

单控制面轮询两个项目对应提交 `76499c179711ac2c93fece4c6b4a3aef659ef67f`。

## 阶段四、专题 06、07、14 · 同一 Worker 分别限制模型和候选 CI

2026-11-17，在两个私有项目各派发一个 Issue。节点 budget-worker 的普通工作项容量为 2，模型和 CI 各为 1。alpha 使用 Codex 和 docs 校验，beta 使用 Claude Code 和项目声明的三项 Docker 检查。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 经转发代理领取模型名额，使首次成功响应丢失；观察两个项目的竞争 | 重试保留原名额，每个执行只启动一个原生模型进程 | alpha 执行 2，Codex Thread `01a1e529-90f7-7953-ac98-2cdbfbff42ec`；beta 执行 1，Claude Session `08d010e1-ad8f-44a9-b140-2056ec661d83`。beta 等待模型时收到 31 次 429。每项各一个原生进程，均退出 0 |
| 延迟转发已结束阶段的释放请求，在 Chrome 打开执行节点，重启控制面，再放行；让 beta 尝试领取尚占用的 CI 名额 | 不同资源的名额可同时持有，同类请求排队；重启保留占用，结算后释放全部名额 | 页面与接口均读取到工作项 2/2、模型 1/1、CI 1/1，控制面重启后仍相同。beta 领取 CI 收到一次 429，释放后完成三项 Docker 检查。两项候选等待 Owner 时三类占用均为零。并存观察针对名额，代理人为延长了首项 CI 的持有时间 |
| Owner 在 Chrome 核对并批准两个候选，等待各自 Actions 从失败重跑至通过，再重启并核对回执 | 每库只合并批准 SHA，并关闭原 Issue；交付证据可重新读取 | alpha 候选 `a4f790b45e29c1f7def58d3c28d7b4938c24662e`，合并 `99b295b93bab431978cc80d1dd90fc6c55231559`。[PR #21](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/pull/21) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/actions/runs/43906842688) · [Issue 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-alpha/issues/20#issuecomment-6922234379)。beta 候选 `40825fbf01d81125066cc4cba6731c76fb5eb13b`，合并 `b06f526c9b6955597c277cc16733ced24079763b`。[PR #46](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/46) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43906853449) · [Issue 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/45#issuecomment-6922235994) |

PostgreSQL 18 回归覆盖并发申请、旧步骤重放、旧节点升级及租约过期后由 Owner 结束执行。启用 PostgreSQL 和真实 Docker 的 Go 全量回归为 9 个包、127 项通过、零跳过。Go 静态检查和 Web 生产构建通过。独立角色、共用同一模型账号的多节点总额度及持续高负载仍待验收。

普通 Worker 独立模型与 CI 名额对应提交 `e8a84ea087fd7d6d4ff1c40db93c5727d2c50352`。

## 专题 10、13、14 · 恢复 CI 运行回执、日志、报告和产物

2026-11-17，从私有 beta #47 导入正式工作项。Codex 生成候选，Worker 在 Docker 中按项目配置执行三项检查。另用独立 Runner 检查含非空日志、报告、产物及可选失败项的固定候选。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| Owner 在 Chrome 核对候选并批准，等待两个必需检查成功，核对已关闭的 Issue | 已交付候选保留绑定执行和提交的 CI 回执 | 任务 1、执行 1、Runner `archive-runner`、运行 `ci-run-3326424383`，候选 `7ca38462210cca41418fc92e7e0c782b55443ffa`，合并 `4e85a882d1173169966556a22200034f6a3a7f04`。[PR #48](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/48) · [Actions 重跑](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43909046862) · [Issue 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/47#issuecomment-6922587678) |
| 停止演练服务，用 pg_dump 及 tar 备份。用 pg_restore 恢复到同机另一个空数据库，将运行目录还原到新的工作区根；从隔离控制面查询原任务及 Chrome 检查详情 | 恢复库独立给出同一份回执，包括三个空日志的大小和 SHA-256。运行元数据、日志、报告均可核验 | 任务 JSON 与源端相同。备份库摘要 `d172e090895f3a68e2eb87a0ac9d28349a8e10845974ad51b2fb6333b4259ca7`，运行目录压缩包摘要 `310c4e26b87eab208cb09877b298ded7ba0dde4bf3b0fefe72518081b43a80f7`。源端和恢复端均核对了三个日志和两个输出文件；逐个移除均返回非零，复原后全部通过 |
| 用独立入口重新验证固定候选 `df7547f581d76629137ff4f21a0cdab490875c18`。在副本中分别更改非空日志、报告和产物的一个字节，保持大小；另改运行元数据，再提供历史回执 | 内容或身份变化均被发现，旧日志缺少独立摘要时应点明缺项 | 运行 `ci-run-3857521277` 的两个日志、报告和产物均通过原始核验。220 字节的 manifest.log、manifest-1.bin 和 manifest-2.bin 分别被改写时，核验退出非零。Runner 身份变化及未知元数据字段均被拒。复原后重新通过。历史 `ci-run-1876710922` 明确报告缺少日志摘要 |

Docker 回归还覆盖超出 1 MiB 的检查输出，核对截断提示也计入最终文件摘要。启用 PostgreSQL 18 和真实 Docker 后，Go 全量回归 9 个包、127 项通过、零跳过，Go 静态检查和 Web 生产构建均通过。按项目定期清理、跨节点在线回读原始文件仍待验收。

恢复 CI 文件并按独立回执校验对应提交 `ff5d714e71c7e93d035b3f159928822c969511de`。

## 专题 07、13、14 · 从 SQLite 与 PostgreSQL 配套备份恢复 Worker 补报

2026-11-17，Windows 上的项目专用 Worker 经本机代理完成三项 Git HEAD 执行，仓库 HEAD 为 `f0e767426662ab396be6d709cca5fd3365405232`。每次在 SQLite 已持久保存待报结果后执行 VACUUM INTO，停止源 Worker，以 pg_dump 备份对应的 PostgreSQL 18 schema。恢复至三个独立空库和三个新状态目录，保持原控制面地址、Worker ID、项目及工作区路径。校验两份备份的 SHA-256 和 SQLite 完整性后启动恢复服务。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 执行 1 的请求在代理处遭遇 503。源控制面仍为 RUNNING。从配套备份启动两个服务后放行回报 | 新目录中恢复的待报结果先于下一次领取送达 | 同一执行回报 204，任务成为 DONE，SQLite 为 confirmed 且待报正文已清除。快照 SHA-256 为 `916872e0246a00238da8aa3d58c83f40f91efe312ab445e1fe967e981676bcd1` |
| 执行 2 的回报已到达控制面，代理丢弃响应。从备份恢复并再次发送 | 补报获得确认，保持原任务和事件 | 恢复前后完整任务 JSON 相同，204，confirmed。快照 SHA-256 为 `950d394660c7670f4613c42662a072e851f4508ecb822b4c666d3fdd44eea64d` |
| 执行 3 的回报未送达。恢复后等待两分钟租约自然到期，Owner 结束原执行，再启动恢复的 Worker | 已结束的执行保留 Owner 决定，迟到结果可查 | 收到 409，任务 FAILED，SQLite 记录 conflict，保留去掉租约的原结果。快照 SHA-256 为 `786c33de04aceb0d144f93ac6628a388ba33e8428d8327cc4d51195bc62fc19f`。三个场景各只有一次执行、三个任务事件和一个工作区 |
| 用运维文档中的两段 Shell 命令备份并恢复 beta #47 的已完成节点日志；比较两份 SQLite 的执行、进程及会话记录 | 归档文件名与节点身份匹配，恢复后能查询同一候选和原生进程 | Git Bash 和 SQLite CLI 实际执行通过。执行 1、进程实例 1、源 PID 3792、退出码 0，Codex Thread `01a1e534-be3a-7302-be18-12830d27a6fe` 与候选 `7ca38462210cca41418fc92e7e0c782b55443ffa` 均一致。[已交付 PR #48](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/48) |

本项复用正式 Worker 的补报入口，变更为运维手册和验收证据。演练在同一主机切换数据库和状态目录，运行中 Harness 的跨主机接管仍待验收。

Worker 日志配套备份恢复对应提交 `75b1495e7095bd8a2c5cc01eb0e0e99e26f31996`。

## 专题 07、10、13 · 在任务页下载其他节点产出的 CI 文件

2026-11-17，导入私有 beta #49，正式 Windows Worker 运行 Codex 和三项 Docker 检查。使用项目专用节点令牌，在验收代理上模拟上传中断和确认丢失。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 阻断文件上传，查询已确认的任务，再向同一 Worker 派发另一项 Git HEAD 任务 | 元数据和文件分别可追踪；归档积压期间仍可派发 | 任务 1、执行 1、运行 `ci-run-3034990605` 登记五个文件。首次下载返回 503。五次上传均遇到 503，SQLite 保留五项待传，任务 2 已完成 |
| 停止 Worker，恢复上传入口，重启 Worker 并丢弃首次成功 PUT 的响应。在 Chrome 下载三个日志及两个声明的输出 | 队列恢复，响应丢失后再次送达同一文件；下载字节、文件名与回执一致 | build.log 有两次 204，其余各一次；队列清空。五次浏览器下载均与源文件逐字节相同。unit-1.bin 的 SHA-256 为 `9344058c877bf855bace4f71bb95cf35d81889f82e839e6a68c03ffb0f00ce9c` |
| 更换节点令牌、任务号或文件名；上传同长度错误内容和超长内容。另在中央副本中更改一个字节后尝试下载，再补传原件 | 只能上传自己的执行所登记的原件，Owner 可发现并修复损坏 | 另一具名节点上传为 403，Worker 下载为 401，换任务或未登记文件为 404。两种错误内容均为 422。中央副本损坏时下载为 503；原件补传为 204，重新下载通过 |
| 在 Chrome 批准候选并等待受保护分支；重启控制面后重查任务与文件 | 按已验收的 SHA 交付，保留原始文件及关闭回执 | 候选 `34faab0d235cbc0ea274b250826f02e232e246ff`，两项必需检查成功，合并 `ddb3745b8af35b9f3d6952f3759f6a6a7fd2946e`。任务 JSON 及五个文件与重启前一致。[PR #50](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/50) · [Actions](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/actions/runs/43912064819) · [Issue 回执](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/49#issuecomment-6923001067) |
| 让 Windows Worker 从五项待传的 SQLite 快照恢复，连接 Linux 容器中的非 root 控制面。容器使用空的项目目录和独立工件存储，未挂载 Worker 工作区。上传后停止 Worker，从 Linux 控制面下载 | 跨系统领取和传输均可工作，节点离线后仍能核验附件 | 首次演练中 Linux 控制面误判 Windows 盘符路径，领取返回 400；修复后五项全部上传。Linux 控制面又派发任务 3，Windows 节点完成执行 3，工作区实际 HEAD 和回执均为 `4e85a882d1173169966556a22200034f6a3a7f04`。停止节点后 Chrome 从 Linux 控制面下载五份文件，摘要全部匹配 |

另从未推送的本地测试分支导入 beta #19，项目配置使 build 写出日志和产物后以 3 退出。真实 Codex 生成候选 `2b14e98d37b2038b035bcd2df86dcacca79cbc21`，真实 Docker 运行 `ci-run-1446598496` 的结果为 BUILD_FAILURE。停止 Worker 后，从 Chrome 的失败执行中下载 build.log（17 字节）和声明为 artifact 的 build-1.bin（12 字节），内容均与预期完全相同。产物 SHA-256 为 `c70ffde6c4269f6436eee0705b70ececf548f8a3603eab2de875c6b08b8e192c`。本场景关闭了远端交付。[输入 Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/19)

Windows 与 Linux 容器共用一台物理主机和 PostgreSQL，项目此前登记在 Windows。异地设备、独立站点网络、文件保留和清理策略仍待验收。

启用 PostgreSQL 18 和真实 Docker 后，全量 Go 回归 9 个包、130 项通过、零跳过。回归包含归档队列写入失败后的原子回滚与重启补报。Go 静态检查和 Web 生产构建通过。

跨节点 CI 文件归档及 Owner 下载对应提交 `6f9b0c141aad99dd9635586e47db8ac363f5dbe3`。

## 专题 07、10、14 · 不同挂载路径上的执行和独立审查

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在空 schema 启动 Linux 容器控制面，从 Owner API 以 `team/beta` 登记只读挂载的 Git 克隆，创建 Git HEAD 任务。Windows 节点使用自己的仓库根和工作区领取 | 控制面登记 Linux 路径；节点从本机对应项目执行 | 登记路径 `/mnt/projects/team/beta`。任务 1、执行 1 在 Windows 完成，Git Worktree、源克隆与控制面回执的 HEAD 均为 `4e85a882d1173169966556a22200034f6a3a7f04` |
| 在独立的控制面、Worker 和 Reviewer 根目录下各建一份 `team/beta` 克隆。正常导入 beta #19，在 Worker 镜像上生成候选。测试转发器在发送结果前用 Git 将固定候选对象同步给控制面 | 领取同时带登记绝对路径和相对路径。模型修改节点上的克隆，控制面能核对固定候选 | 普通领取和独立审查领取均保留控制面的绝对路径并传递 `team/beta`。Codex Thread `01a1e555-48ef-79f2-8f5f-5e9a1abd6a34` 生成 `693b82ff5c7cd0b21a9fe17a1e10bf6a006cd84e`，只更改指定页面。运行 `ci-run-1475177422` 通过本地 CI。[输入 Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/19) |
| 让尚未同步候选的 Reviewer 领取，再将固定对象同步到审查镜像，重试同一执行。在 Chrome 阅读报告和差异，核对标题及段落后批准；重启控制面 | 缺少候选时审查失败，可用对象到位后按原 SHA 审查和批准 | 首次报告候选不是可读取的 commit。同步后独立 Claude Session `8c97a582-98db-4cec-bda6-9c3752ceb21d` 对原候选给出零条发现。Owner 批准为 ACCEPTED，重启后任务和报告均相同。本场景关闭了 GitHub 交付 |

两次演练均使用同一物理主机。独立镜像的 Git 对象同步由演练显式执行。全量回归启用 PostgreSQL 18 和真实 Docker，9 个包、131 项通过、零跳过；Go 静态检查通过。

不同挂载路径上的执行与独立审查对应提交 `ef5699fdfd0d6386ce4a9be0a659011e8520285d`。

## 专题 10、13 · 中央 CI 文件按项目到期清理

对 beta 两次真实 Docker CI 的 PostgreSQL 18 数据做 `pg_dump`，恢复到新数据库，并复制中央文件目录。源任务及文件保留。控制面从迁移 34 启动，按原确认事件的时间登记失败运行的 6 份文件及已交付运行的 5 份文件。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 先在第二个项目设置期限，再用 Owner 和具名 Worker 调用源项目的策略与下载接口 | 只影响所选项目；仅 Owner 能设置策略 | 源项目的三个期限仍为零，11 份原件均可读，文件字节逐一匹配。Worker 尝试修改策略返回 401 |
| 在失败运行 `ci-run-1446598496` 设置日志期限，让日志在几秒后自然到期，构建产物立即到期，报告无限期保留。在 Chrome 打开原任务并下载 | 到期前可读，到期后拒绝下载和补传，报告仍可使用 | 从 Owner API 先读回 17 字节的 build.log；到期后 Chrome 提示文件已按项目策略到期，报告下载与原件逐字节一致。12 字节的 build-1.bin 及三份日志均返回 410。任务仍为 FAILED，候选与原始摘要完整 |
| 在隔离副本中将到期的 build.log 临时替换为含标记的目录，等待正常的分钟扫描，再恢复普通文件并重启控制面 | 存储故障可重试，其他文件照常清理，目录中的内容受到保护 | 本轮清除了其他两份日志及一份构建产物，保留了标记和 build.log 的待清理记录。重启后自动移除了恢复的文件，四条记录都有清理时间，两份报告仍存在 |
| 在已交付运行 `ci-run-3034990605` 使日志到期，再把期限改回无限期。恢复含五个待传项的 SQLite 快照，从真实 Worker 启动；移走其本地一份日志以模拟源文件遗失 | 延长期限不会恢复已到期文件；迟到的补传可收敛 | 三份日志均返回 410。Worker 对缺失文件查询是否到期，向中央补传两份保留的报告，队列由 5 降至 0。磁盘上仍只有两份报告，任务及交付 JSON 与操作前完全相同。[原 PR #50](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/pull/50) |

PostgreSQL 18 与 Docker 全量回归 9 个有测试的包、131 项通过、零测试跳过，`go vet ./...` 通过。新增检查覆盖策略与进行中上传的并发、清理失败重试及重复扫描。本项验收覆盖控制面的中央副本。Worker 本地工作区、反馈截图和录屏的保留策略另行验收。

中央 CI 日志、报告与产物的项目保留策略对应提交 `b46504858df38effd7f7f4a7aa7e0612e04d9747`。

## 专题 11、13 · 到期反馈截图与验收决定

从 PostgreSQL 快照及文件备份恢复静态预览候选 `304948ec31c7e6616230934f8d6de1b97f6da2cf` 的原反馈 #4，在独立数据库和工件目录中启动正式控制面。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 Chrome 打开归档截图 #1 并读取遮挡区像素，再由 Owner 在隔离项目退回候选并记录复测理由 | 图片可在期限内核验，验收决定绑定固定候选 | 归档图为 2731 字节，像素 RGBA 为 0、0、0、255；读回原件摘要为 `b8d921842bcf9d90c1958b9eae8e13ca0f0637ebfd2e3cce77727c53470ecc42`。退回后任务为 READY，保存一条验收决定 |
| 给另一个项目设置 1 秒期限，再给源项目设置 6 秒，同时上传新的 94 字节 PNG。尝试读取和重传已经到期的旧截图 | 项目隔离；每张截图从各自的首次归档时刻起算 | 第二项目配置未影响原图。源项目配置后旧图的读取、同图重传均为 410。新图 #12 先能读回并匹配摘要，7 秒后返回 410。Worker 修改策略为 401，负期限为 400 |
| 把源项目改回无限期，刷新 Chrome 反馈历史，再故意让旧图的清理遇到非普通文件。等待分钟扫描并重启复测 | 已到期图片不复活；故障可重试，其他图片仍清理 | Chrome 对两图均显示“已到期”、原编号和摘要。首次扫描移除新图，保留旧图路径下的标记；恢复普通文件后，控制面重启自动移除旧图，两份回执都有清理时间 |
| 重启后从 Owner 接口查询任务、历史反馈，再次提交原图 | 已归档的决定、候选和反馈可追溯，重传不能新增附件或事件 | 退回决定、理由、候选 SHA 和反馈内容与清理前相同，历史任务仅新增新图首次归档的一条事件（9 → 10）。原图重传仍为 410，两条附件回执均保留 |

PostgreSQL 18 与真实 Docker 全量回归 131 项通过，零测试跳过；Go 静态检查、TypeScript 检查和 Web 生产构建通过。此项只覆盖中央归档 PNG。录屏、节点本地文件和外部设备预览仍待验收。

反馈截图保留策略与工作台到期状态对应提交 `b5f399204936c0378eecb8e5f058eaaa97d03ee1`。

## 专题 13 · 从已清理文件追查策略修订

分别恢复真实的失败 CI 和已遮挡截图，再用旧版和新版控制面顺序访问同一份恢复数据。下面的修订编号分别来自两套独立的数据库 schema。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在版本 36 配置非默认策略，分别使 build.log 和截图 #1 到期，保留一份构建产物和新截图。用版本 37 接管 | 能读出迁移时的配置，审计启用前的删除保留未知来源 | 两个项目各有一条编号 1 的迁移记录，原配置与旧版接口一致，操作者为 migration，前一配置为空。已到期文件返回 410 且无修订号；保留的原件仍可下载 |
| 在恢复库中临时使项目策略写入失败；再并发四次提交同一有效变更 | 策略与审计同步提交，失败与重试不制造修订 | 故障返回 500，旧配置、文件和历史均不变。解除故障后，四个请求只新增一条由 Owner 发起的修订 3，前后配置和操作范围均正确。Worker 查询历史或修改策略为 401 |
| 将两项期限分别恢复为无限期，重启控制面，重查到期文件和历史 | 下载回执继续指向有效的到期依据 | build-1.bin 和截图 #10 均已物理清理，410 回执与数据库各自指向修订 3；修改后的修订 4 和迁移记录 1 保持独立。较早已到期的文件仍无修订编号 |
| 在独立项目提交 102 次不同配置，读第一页后再提交一项，再按游标读下一页 | 并发新增记录不改变已经取得的游标，同时保持项目隔离 | 两页分别返回 100 和 2 条记录，依次还原原来的 102 个期限，没有遗漏或重复；源项目仍只有三条历史。错误游标为 400，不存在的项目为 404 |

PostgreSQL 18 与 Docker 回归 131 项通过、零测试跳过，`go vet ./...` 通过。本项审计记录保留策略的成功变更，其他操作的审计覆盖见专题总表。

保留策略修订和已清理文件的关联对应提交 `079dec80a252a462355ad442305c541665be196a`。

## 专题 05、16 · 跨项目批量回答验收问题

恢复已验证的预览候选 `304948ec31c7e6616230934f8d6de1b97f6da2cf`，作为项目 1 的任务 1。另在项目 2 从 GitHub 导入私有 beta #19，正式 Worker 用 Codex Thread `01a1e578-9f8c-7531-9ba2-14d78c6025a4` 完成任务 2，候选为 `343179caa60ab2cc43e635ee7067832c4d1f6ed2`。两项均从 Owner API 在各自固定候选上提出问题，通过 Chrome 工作台回答。本次关闭远端交付。[第二个项目的输入 Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/19)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从侧栏进入待答问题，核对两个任务的依据、SHA、任务详情链接，再用 Worker 查询队列 | Owner 能看到跨项目的待答事项，Worker 无权读队列 | 两题分别指向任务 1、2 及各自候选，依据完整；Worker 收到 401，非法游标为 400 |
| 填写问题 #2、#1，一次提交；在 Chrome 测试转发器中，等 #2 写入成功后丢弃 HTTP 200 响应，再重试未确认的草稿 | 同批其他答复仍完成；重试不会重复写事件 | 页面先显示 1 条成功、1 条未确认，保留 #2 原文。重试后队列清空，两项各有一个 Owner 答复、一个答复事件 |
| 各提出新问题后，在队列填写两项。另一 Owner 会话先回答 #4，随后提交整批 | 保护已记录的决定，仍允许同批其他问题恢复 | #4 显示另一会话的实际答复，并保留本页草稿供比较；#3 按本页答复成功。两个任务各新增一个事件 |
| 第三轮只填写 #6。检查 #5 仍待答，再在宽 390 像素的 Chrome 页面提交 #5，重启控制面 | 空白项继续等待，已答项回到原候选验收，重启保留每次决策 | #5 在 API 中仍为 WAITING_INPUT。全部回答后，两个任务均为 WAITING_ACCEPTANCE，各有三条答复和三个答复事件；原执行号、SHA 及重启前后任务 JSON 均一致。窄屏无横向溢出 |

另用 22 条分页测试数据走实际工作台，提交首页的 20 条后显示“本页问题已处理”，点击“加载更多问题”显示余下两条。PostgreSQL 游标回归覆盖分页间新增问题及其他会话答题。启用 PostgreSQL 18 和 Docker 的 132 项 Go 测试全部通过、零测试跳过，Go 静态检查、TypeScript 检查和 Web 生产构建通过。

跨项目批量回答验收问题对应提交 `abb5257b9899049c22868821721030556f70a5ab`。

## 专题 12、13 · 真实 GitHub Webhook 接入

在私有 beta 仓库使用 GitHub 官方 CLI 转发器建立临时开发 Webhook，将 GitHub 实际发送的原始请求头和正文转发给回环控制面。仓库数字 ID 为 1459615168。工作项留在 READY，以便核对 Webhook 的派发结果。[Issue #51](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/51)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 创建普通 Issue，等签名投递，再从收件箱检查 | 提前同步且只进入收件箱 | opened 投递 `ed3fabd8-e1e7-11f1-82d3-fbb36c2bbed0` 返回 202，7.01 秒内可读，未创建工作项 |
| 添加 forgeos-agent 标签，修改正文，再新增一条评论 | 只产生一个任务；正文更新和评论可分别追溯 | 6.56 秒内出现任务 1。labeled、edited 和 issue_comment.created 各返回 202，需求升为第 2 版，原评论从任务评论接口可读 |
| 用 Owner 身份读回四份原文，与转发器收到的请求比较，并用仓库密钥独立验签 | 投递编号、长度、摘要和原始字节均一致 | 四份原文依次为 9009、9539、9362、10925 字节。评论投递 `f99347f0-e1e7-11f1-811e-bca69b1fa4d6` 的 SHA-256 为 `5b46470c4b9d3b8e65f95258df5996c25a8b421e2d1068a31ce84af095433f4a`。Worker 读取返回 401 |
| 在原正文后加空格沿用原签名，再重投完整原件；重启控制面并再次重投 | 篡改被拒，真实重复投递复用原回执 | 篡改返回 401，重投均为 200。重启后的四条投递、原文字节和完整任务 JSON 均与重启前一致 |

结束转发后已核实 GitHub 自动移除临时开发 Webhook，验收 Issue 已关闭。启用 PostgreSQL 18 和 Docker 的 134 项 Go 测试通过、零测试跳过，`go vet ./...` 通过。回归覆盖错配仓库、同号异文、原文回读及限流期间收到通知。本次验证了 GitHub 签名到本机正式入口的全程，生产公网代理部署和关闭 Issue 后的任务处置另行验收。

Webhook 接入实现及真实投递证据对应 ForgeOS 提交 `4ea0363db2e93e80be7722e4190e07eacda6c012`。

## 专题 06、10、12 · 来源 Issue 关闭时取消执行或归档 Goal

在私有 beta 仓库创建两个真实 Issue。第一项使用正常 Worker、Codex 和 V2 容器验证。在本地验收克隆的固定基础提交 `65a55e135a89f301e42e8612ecfb7564f60f52a1` 延长构建步骤，以便在运行中关闭 Issue。第二项从 Owner API 建立带依赖的 Goal。两个场景均通过定时轮询读取 GitHub 的实际状态。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 观察到 Docker 正在执行 build 后创建一个同项目任务，再在 GitHub 将来源 Issue 以 not_planned 关闭 | 取消请求先入库，在原执行交回名额前保留队列顺序 | 02:21:00 关闭，02:21:03 观测到请求。执行 1 仍为 RUNNING，后续任务 2 为 READY。02:21:30 原任务转为 CANCELLED，随后任务 2 完成。时间均为 UTC+8。[Issue #52](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/52) |
| 核对运行回执、容器、中央日志，并重启控制面从 Chrome 打开任务详情 | 候选和已执行检查可追溯，容器已回收，取消记录可查 | 候选 `51488d9b97d1ecb94332ea85ed800cd56dde4b47`；运行 `ci-run-312700951` 与 build 均为 CANCELLED，build 退出码为空。中央日志与本地同为 20 字节，SHA-256 为 `aed2fe905c544caa28ec2c21fd69f5f719fb0bb571f3036f23f675346bdf3d5b`。原容器已移除，任务无交付记录，重启后页面与状态筛选都能查到关闭记录 |
| 激活含 READY、BLOCKED 两个子项的 Goal，关闭父 Issue，再重新打开并导入一次 | 原 Goal 和子项停止派发，保留计划，不重复创建任务 | Goal 1、子项 2 和 3 全部为 CANCELLED，子项无执行且关联同一关闭回执，原计划逐字段相同。重新激活返回 409，重开后导入仍指向原 Goal，取消列表仍为三项。验收后已再次关闭 Issue。[Issue #53](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/53) |

PostgreSQL 18 和真实 Docker 的全量回归共 9 个有测试的包、137 项通过、零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。回归还覆盖远端对象错配、过时观察、运行中重复关闭、迟到的成功回执及已批准子项的历史保留。

来源 Issue 关闭的实现及验收对应 ForgeOS 提交 `a5d21934d8ecd6c288d20a169b8170e16d822ab3`。

## 阶段一、专题 01、16 · 工作台登记项目与首次 Worker 检查

在新的 PostgreSQL schema 中启动正式控制面。用 Chrome 从工作台登记一个新建的本地 Git 仓库，通过正式 Worker 检查从空仓库到首个提交的状态。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 输入允许范围外的路径，再用相对和绝对路径登记同一仓库 | 拒绝错误路径，登记结果可重复查询 | 浏览器显示路径错误，随后两次均获得项目 1。Worker 身份读取项目列表、创建首次检查均返回 401 |
| 点击运行 HEAD 检查，在控制面完成请求后丢弃浏览器收到的响应，再次点击 | 同一项检查可继续查看 | 两次指向任务 1；Worker 在尚无提交的仓库执行失败，留下非零退出码和 `git worktree add failed` |
| 提交 README 后从桌面和 390 像素宽的独立浏览器会话重试同一项失败检查 | 仅派发一次新检查，留存此前失败 | 两个页面均打开任务 2；Worker `project-setup` 在 execution-2 返回退出码 0，HEAD 为 `0bc57a3402ecc47044c70d2768c7f04b1642582b`，与直接读取仓库一致。再次重报仍为两个任务，窄屏无横向溢出。重启后任务 2 完整 JSON 相同 |
| 另在 31 个项目中从浏览器翻页，在两页之间新登记项目 32 | 已登记项目可完整浏览，旧脚本的 HEAD 检查能复用 | 首页 30 项，下一页只增 1 项。项目 1 显示旧 API 创建的检查 1。刷新后看见新项目 32，非法游标返回 400 |

PostgreSQL 18 和 Docker 全量回归 138 项通过，零测试跳过。Go 静态检查、TypeScript 检查与生产构建均通过。

工作台项目接入与验收对应 ForgeOS 提交 `d57f60da4b1dff1892b1c5f50559add4acd00e38`。

## 专题 01、08 · 固定提交上的仓库入口与依赖清单

在私有 beta 的本地克隆中准备 Go 模块、嵌套 Node.js 包、一个无效包清单和合成的隐私测试标记。通过真实 GitHub 轮询绑定仓库 ID 1459615168，由 Worker `inventory-accept` 完成 HEAD 检查后再推进本地分支。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| HEAD 检查尚未完成时请求扫描，再用 Worker 令牌调用 Owner 入口 | 拒绝无执行依据的扫描及未授权请求 | 分别为 409 和 401；首次归档前读回为 404 |
| 分支已经前进时并发发起两次扫描，逐文件核对 Git 对象 | 使用原检查提交，两次返回同一份归档 | 原提交 `7b38ce432cf62bfa1b4007fd34d20bd996b7706b`，新 HEAD 为 `0e4abdfb92320abec3e8d90ed3f117979fc2c7a5`。归档 6 个文件，所有 blob 均与原提交一致，两份响应字节相同 |
| 展开嵌套包，同时核对无效清单、Go 模块和测试标记 | 正常来源可使用，局部解析失败明确可见 | app/package.json 的 blob 为 `7ec36398254882b7d845d83bed745205891b50a4`，保留 start、test 和三项依赖名。Go 清单包含 example.org/queue-fixture，无效包标记 invalid_manifest。归档不含 .env、脚本正文与依赖地址中的测试标记，脚本未执行 |
| 重启控制面并将仓库根切换为空目录，在 390 像素宽的 Chrome 中读取 | 从数据库恢复原清单，能够打开同一份包详情 | GET 与重复 POST 返回原归档，页面可展开 npm run start，窄屏无横向溢出 |
| 再从项目页选“检查当前 HEAD”，由同一 Worker 创建第二份清单 | 新检查读取更新后的脚本，原任务继续显示原清单 | 独立复验中任务 1 绑定 `b0b7cf3d08963a135aff8cf6c4d08ee7e1bf5013`，任务 2 绑定 `f6ed6f19006a25d2917556e3b0e757443f31e0b2`。第二个页面只显示新脚本 npm run different，第一份归档逐字段相同 |

PostgreSQL 18 和 Docker 全量回归 139 项通过，零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

仓库入口和依赖清单对应 ForgeOS 提交 `e73088cbe63e0da79133d342e477d018c640021c`。

## 专题 01、09、ADR-001 · 按环境归档项目声明的密钥需求

在独立 PostgreSQL schema 中用正式控制面、Worker 和 Chrome 生成三次仓库清单。验收仓库使用合成名称。预先通过 Owner API 登记 local-mock/queue-api 的实际元数据为 S2。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在首个提交声明 local-mock 与 ci-test 下两项同名需求，再声明 production 的独立需求。执行 HEAD 检查，在 Chrome 展开项目配置 | 显示各环境声明，记录其固定来源 | 原提交 `6467fb6366933bb412dcb0e6f9ee467033bc3b5d`，根配置 blob 为 `372091daa991c34239188453d4a91e2326fbd880`。Chrome 展示两条 queue-api/S1 和一条 production/queue-admin/S3 |
| 比较扫描前后的元数据、授权和使用记录 | 声明不得修改实际风险、登记新凭据或授予使用权限 | PostgreSQL 唯一既有元数据的风险仍为 S2，更新时间相同。ci-test/queue-api 的 Owner 状态查询返回 404。执行授权和使用记录均为零 |
| 在需求中加入不受支持的 value 字段，再用第三次提交移除声明 | 错误可定位，回执不含字段值。空清单与配置错误能区分 | 第二次提交 `5783bb288bcb2514b1957994d3941a7222a7498d` 在浏览器提示配置无效；归档保留 blob 和 invalid_project，无测试字段值。第三次提交 `ce8ac6a156c3db4f10a9456cfba11f74044a691c` 返回空数组 |
| 重启控制面，并切换到此前生成的旧版清单 | 已归档的旧证据与首次扫描一致 | 前两版回执重启后逐字段相同。旧版项目清单仍未包含当时没有解析的字段 |

PostgreSQL 18 和 Docker 全量回归 140 项通过，零测试跳过。Go 静态检查、TypeScript 检查与生产构建通过。

分环境密钥需求对应 ForgeOS 提交 `6e51e433ac87e43f8b432d968908e5eca7bc1b22`。

## 专题 01 · 固定提交的本地环境清单与启动验证

验收使用正式控制面、Worker、Chrome、Docker 以及独立 PostgreSQL 16.15 容器。两次运行均通过。下表记录第二次运行，控制面使用 PostgreSQL 18 的独立 schema `startup_04196f07`。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 登记本地测试仓库、派发 HEAD 检查，从 Chrome 打开项目配置清单 | 两种环境均能追溯至同一次检查的 Git 提交 | 浏览器展示 Local Mock 与 Integrated 的脚本、端口及探测路径，绑定提交 `592602d4bce5cface58ebfd0aee9726debb97baf` |
| 在仓库 HEAD 更新后，对上述提交运行 Worker 的 verify-environment，选择 local-mock | 运行原提交脚本，禁用外部网络，就绪后清理容器 | `environment-run-436666246` 使用 none 网络，第一次探测返回 HTTP 204 |
| 为 local-integrated 配置带项目标签的内部 Docker 网络和数据库，再运行同一入口 | 应用完成真实数据库查询后才就绪 | `environment-run-3398551146` 两次探测后返回 204，PostgreSQL 日志记录了读取预置行的 SELECT |
| 选择返回重定向的新版本，再将 Integrated 指向错误网络 | 重定向无法通过就绪检查，错误网络在启动前被拒绝 | `environment-run-3494789491` 记录 302 并超时；`environment-run-1326673787` 记录网络不匹配，探测次数为零 |
| 停止数据库后重跑 Integrated，再检查一个启动脚本立即退出的版本 | 分别记录依赖故障和服务退出 | `environment-run-787047279` 记录 503 并超时；`environment-run-3387302772` 记录就绪前退出 |
| 比较每次命令的标准输出与本地 run.json，再查询 Docker | 失败也保留回执，所有已创建的启动容器均移除 | 六份 JSON 逐字段相同，已创建的探测容器均清理，验收数据库容器及临时网络已移除 |

PostgreSQL 18 和 Docker 全量回归 142 项通过，零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。工作台派发及汇总启动检查另行验收。

本地启动清单与 Worker 命令行验证对应 ForgeOS 提交 `b5e07294ca091f1b79000f818c808cce6ffc2cf4`。

## 专题 01、08 · 工作台派发本地启动检查

在 PostgreSQL schema `queue_01858544` 启动控制面、独立 Worker、桌面 Chrome 和 Docker 中的应用及 PostgreSQL 16.15 依赖。使用工作台派发全部四次检查。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 完成来源 HEAD 检查，读取清单后更改测试仓库 HEAD，再从清单派发 Mock 和 Integrated | 两项任务均运行清单固定的提交；Integrated 实际访问数据库 | 来源 T-1 固定 `f12eec09686793d8b7740830f4984b522deae8b3`，新 HEAD 为 `e6028b8c7c2096dae6135ba46e0575d5044bdea6`。T-2 Mock、T-3 Integrated 分别在第 1、2 次探测返回 204；数据库日志出现对预置行的 SELECT |
| 停止数据库，从 Chrome 重跑 Integrated，然后恢复同名数据库与预置行，再次重跑 | 故障保留独立回执，修复后可重新验证 | T-4 在 17 次探测后记录 HTTP 503，状态 FAILED。T-5 第一次探测取得 204，状态 DONE。两者固定同一提交和启动脚本 |
| 每次派发后重发相同请求，随后核对工作台、节点本地回执和重启后的控制面 | 请求重放复用任务，回执可回读 | 列表只含 T-2 至 T-5；节点 startup-node 的四份 run.json 与 API 逐字段一致。Chrome 可从各执行详情返回来源 HEAD，四次均显示容器已清理。重启控制面后 T-5 回执逐字段一致 |
| 用无凭据请求读列表、用 Worker 凭据尝试派发，再验证不匹配的节点、资源和回执 | Owner 管理入口限制访问，调度和回执绑定来源 | 两项真实 HTTP 请求均返回 401。数据库回归覆盖并发派发、跨项目授权、旧节点、错配集成项目、模型配额和不完整或篡改的成功回执 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 143 项通过，零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

工作台派发与启动回执归档对应 ForgeOS 提交 `42dee55abb05f6627a2acd0086a747cb5b4f25fe`。

## 专题 01 · PHP Composer 依赖和脚本清单

在独立 PostgreSQL schema `composer_7a54b0ad` 运行正式控制面和 Worker，从 Chrome 打开两次 HEAD 检查的归档。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 对同时包含 PHP 和 Node.js 的仓库执行 HEAD 检查，在 Chrome 展开两个包清单 | 各自显示正确的依赖和脚本入口 | T-1 对应 `90a66dc71c930fa8c7c1ec28ec904dcf1d8ab2e5`。PHP 条目包含 ext-pdo、monolog/monolog、php、phpunit/phpunit，脚本为 composer run-script start 和 test。Node.js 条目显示 npm run test |
| 同时提供损坏的清单、vendor 子目录、建议安装的包以及含合成标记的脚本和仓库 URL | 错误定位到文件；归档只含声明名称；扫描不执行脚本 | 损坏的条目在 Chrome 显示“清单格式无效”，vendor 下的清单与建议包未进入结果。标准回执中没有合成标记和脚本正文，测试脚本没有创建文件 |
| 更新 PHP 清单，从项目页再次检查 HEAD，重启控制面后回读两次结果 | 新旧清单独立归档 | T-2 对应 `e6007d00ce03e8babf5fe020c6a48d513692c808`，只显示 php 和 composer run-script verify。两次配置 blob 分别为 `bd018c27b8770c89506a7283c71910ba912050e8`、`eda3693ea975adcda0fd43013e96674df04a67a3`，重启后回执逐字段相同 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 144 项通过，零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

PHP Composer 清单对应 ForgeOS 提交 `a0b90a45b69f054b8537ccc11ef950b838cff1f2`。

## 专题 01 · 原生 Agent 归纳固定版本的入口文档

在 `analysis_5a06be3e` 使用正式控制面、独立 HEAD Worker、项目专用 Management Agent、已登录的原生 Codex 和 Chrome。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从 Chrome 读取 T-1 清单，更改 README 并提交，再从旧任务派发归纳 | Agent 使用 T-1 的文档和包清单 | 来源为 `e0276e7490e6c6d4ba0917fb5493a2fb6d13f82f`，新 HEAD 为 `1eeb42cb37929ef830b62423187961b251212b5f`。原生会话 `01a1e5d4-b0ae-7c51-9c54-f2c2e528f0b7` 返回五类事实，其内容没有新版命令 |
| 展开启动命令并核对依据 | 每项能定位到入口原文或清单摘要 | Chrome 展开 go run ./cmd/rain，显示 README.md 的引用及 blob `6a25245a5bb677f5d9214d0bcd1198c7e864e10b`。测试命令为 go test ./...，依赖 github.com/lib/pq 来自 go.mod 摘要 |
| 重复派发、用 Worker 凭据访问 Owner 接口、重启后回读 | 同一检查复用归纳结果且只向授权身份提供 | 未生成第二次归纳，Worker 访问返回 401；重启控制面后 GET 和 POST 的回执均与原结果逐字段相同。测试 .env 及其合成标记均未进入回执 |
| 在真实 PostgreSQL 中使旧租约过期，提交错误来源或凭空写出的命令，再提交正确结果 | 拒绝旧租约、跨项目写入和不匹配的证据；允许正确结果重放 | 数据库回归通过，旧轮次的重复重试未重新排队；正确结果重复提交保持同一份归档 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 145 项通过，零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。补充拒绝 PostgreSQL 无法归档的空字符后，相关数据库回归和 Go 静态检查再次通过。

固定来源的 Agent 归纳对应 ForgeOS 提交 `3541ac70b38e92bf10b93b1051732f255a767ce3`。

## 专题 01 · 版本化初始化报告

使用 Chrome、正式控制面及 Worker、原生 Management Agent、Docker 和真实 PostgreSQL 测试依赖重复验收两次。下表记录 schema `initreport_4253245f` 中固定提交 `76b1f05deca8fbb8cf29e857459b5f7c42e28513` 的三版报告。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 T-1 读取清单、归档报告，随即重发归档请求 | 初版记录当时可得的证据，重复操作复用版本 | 第 1 版包含项目路径、GitHub origin、分环境声明和 station-test-key/S1 密钥需求。Agent 为 NOT_REQUESTED，启动记录为零。两次请求返回同一版本，Worker 身份读取报告返回 401 |
| 归纳入口文档并运行 Mock 和 Integrated，停止数据库后重跑 Integrated，再归档 | 报告将固定来源的项目画像与真实失败回执保存在一起 | 第 2 版收录原生会话 `01a1e5dd-30d9-7f00-b8b7-c65b253b20b1`。T-2 和 T-3 返回 204，测试数据库记录实际 SELECT；T-4 返回 503，状态 FAILED |
| 恢复数据库并重跑，从工作台归档、切换旧版、下载新版 JSON | 新版记录恢复，旧版保留故障及当时的归纳状态 | 第 3 版收录四次检查，T-5 返回 204。第 1 版仍显示未派发。下载文件与第 3 版 API 逐字段相同，文件 SHA-256 为 `3ac6ee53bfb2f8bb9d9a173d83df6483d1e1a5fb8b946cfb2fc5423c61adbf52` |
| 将控制面仓库根切到空目录并重启，逐版回读、重复归档；另从数据库并发申请归档 | 无原始克隆也能读取，证据没有变化时不增加版本 | 三版报告均与首次结果相同，重复归档返回第 3 版。数据库回归中四个并发请求均得到第 1 版；新画像资料产生第 2 版，旧版内容保持相同 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 146 项通过，零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

初始化报告与版本回读对应 ForgeOS 提交 `9a75d9e04a2cdd2be90b833da5f8bd6c38d1e698`。

## 专题 01 · Python 依赖、动态字段与命令入口

使用真实控制面、HEAD Worker 和 Chrome 分别检查同一仓库的两个提交。在 schema `pydeps_9b6d6680` 核对工作台、报告 JSON 和项目专用 Management 领取的数据。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 读取带运行依赖、可选依赖、构建依赖、依赖组、具名 wheel URL、脚本及动态字段的 pyproject.toml | 包名规范化并去重；命令按 Python 入口显示 | T-1 固定 `7460e99bcbfa1b6c22e6b85228c6d412314734a3`。清单列出 http-x、pytest、ruff、setuptools、wheel-pkg，在 Chrome 显示 station-start 和动态字段 gui-scripts |
| 同时读取 requirements.txt、损坏的 TOML 及 Node.js 清单 | 解析具名要求，明确其余来源；错误只影响对应文件 | 读出 requests，远程包含与本地可编辑安装计入 2 项未解析来源；Chrome 显示损坏清单的格式错误，Node.js 条目仍显示 npm run test |
| 归档报告并以项目 Management 身份领取归纳输入 | 报告保留清单，Agent 可用依赖和入口名作引用 | 报告的清单逐字段一致。Agent 输入含包名与 station-start；其来源 blob 为 `8933ad4fe3a41824bfec3f0628c59396cf0aff80`。合成的私有 URL 标记和 Python 导入目标均未出现在输入中 |
| 更新清单，从项目页重新检查，再重启控制面 | 两次回执保留对应版本 | T-2 固定 `d1466a28a293aaf410d0dee951ab65b768f8ed31`，仅有 ruff 与 station-check，清单 blob 为 `3da111b613112a1be544786e5e194a0b9abd4297`。两版重启后回读均与首次相同 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 147 项通过，零测试跳过。Go 静态检查、模块校验、TypeScript 检查和 Web 生产构建通过。

Python 包清单对应 ForgeOS 提交 `c5608ae0ab3068485c40649a115a5bfc8c4c267d`。

## 专题 01 · Rust Cargo 工作区与固定版本的继承来源

在独立 PostgreSQL schema `cargo_d35e64dc` 中使用正式 Worker、控制面和 Chrome。将同一个验收克隆只读挂载到禁网容器，使用 Cargo 1.85.1 的 `cargo metadata --format-version=1 --offline --no-deps` 独立核对成员的直接依赖。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 对虚拟工作区、子 crate、重命名的共享依赖、开发依赖和条件构建依赖执行 HEAD 检查，再从 Chrome 展开成员清单 | 工作台与原生 Cargo 的直接包名集合相同，根清单另外显示共享声明 | T-1 固定 `27a292d88035429c718b6a1890c253e0290e0040`。两者均为 fixture-kit、rain-first、win-helper。unused-shared 仅出现在根清单的共享声明中 |
| 读取被工作区排除的子目录以及包含坏 TOML 的独立文件 | 无法继承的包保留未解析状态，其他条目可继续读取 | 原生 Cargo 在排除目录退出 101，提示找不到工作区根。ForgeOS 的该条目显示一项未解析、无继承包名；坏 TOML 标记 invalid_manifest |
| 只修改根清单的别名目标，再从项目页检查 HEAD | 成员文件未变，第二次检查显示新包名与新的根清单 blob | T-2 固定 `2ef9e6750295af87200aa1e66d56bc536c268d80`。成员 blob 均为 `f5998b86185ef975d4ffe7f9197dd78707f82422`，根 blob 从 `d31f256fc45d1aeb47925604db3c1af2cdfd15b0` 变为 `b6cc5c41ef9d256a9f51cad160c0ca411e68112b`，两端均读到 rain-next |
| 在两次任务各归档初始化报告、领取一次 Agent 输入并重启控制面 | 报告保留同一清单，Agent 收到声明名，历史版本可回读 | 两份报告中的清单逐字段相同于对应任务；成员与根清单的文本摘要不含声明中的私有路径标记。控制面重启后两版清单逐字段一致 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 148 项通过，零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

Cargo 工作区清单对应 ForgeOS 提交 `9e9e55269894dcb65e904c1906e07d539b739eee`。

## 专题 01 · Maven 父 POM 与条件依赖

使用 PostgreSQL 18 schema `maven_60738cc1`、正式 Worker、控制面和 Chrome。将同一提交只读挂载到禁网容器，用 Maven 3.9.16 Model Builder 分别构造默认模型和启用 qa 的模型。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在项目页执行 HEAD 检查，打开 app/pom.xml，展开 qa | 普通、受管和条件依赖的坐标与原生模型一致 | 提交 `2f5d5f1195a586a994fc2ccf5ddaf507da67b856`。两端均读到 org.first:core、org.fixture:helper、org.parent:base。受管的 org.managed:only 和 qa 的 org.qa:integration 分别展示 |
| 仅修改父 POM 的属性，再次检查并归档 | 子 POM blob 不变，新报告使用新的父属性 | 提交 `36063349f0a4fecd07e53972d6234b134fd0e31f`。子 blob 均为 `f1b0ce4f2e44914539cdaf2aa12a4f4bccd97a89`，父 blob 从 `25d55cb1c9990f73b4ca38b1dfe217d6156a2297` 变为 `e6b4e2b22e222d8caf12c9864e204ae0d6601406`。两端均读到 org.next:core |
| 同时扫描坏 XML、仅远端可取的父 POM 和环境变量 | 各文件独立记录读取结果，工作台可以继续归档 | 坏 XML 标为 invalid_manifest；远端父和环境属性共两项未解析 |
| 归档两版报告、领取 Management 输入，重启控制面回读 | 每版包含当时的依赖和来源，Agent 输入保留声明种类 | 两份报告与当次清单逐字段一致。Management 输入含直接及受管坐标，未包含验收用的私有标记。重启后两版清单均与重启前相同 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 149 项通过，零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

Maven 清单对应 ForgeOS 提交 `9e88bdd57971956e382d9141057b4ed9861e6fe7`。

## 专题 06、10 · Owner 从工作台取消运行中的 CI

真实验收使用私有 beta 的 Issue #54、正式 Codex Worker、Docker、Chrome 和 PostgreSQL 18 schema `ownercancel_e93befcb`。以下时间为 UTC+8。[验收 Issue](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/54)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 等待 Codex 自然结束并生成候选，观察真实 Docker build 后，排入同项目 HEAD 任务，在任务页选择“取消任务 · 执行 #1” | 请求绑定当前执行；结算前后续任务仍排队。重复提交无新增请求事件 | 候选 `b2bf1acda60f9adfa8eef62c527a75cfb75e26e9`，原生会话 `01a1e5fc-84fe-7103-85ce-00f13a891722`。04:12:38 浏览器收到 204，T-1 仍为 RUNNING，T-3 为 READY；再次 POST 返回 204，共一条 owner_cancel_requested |
| 等待续约、读取 CI 回执，并从控制面下载 build.log 与 Worker 原件比较 | CI 取消保留已有日志，回收容器，后续任务可派发 | 04:13:03 T-1 转入 CANCELLED。运行 `ci-run-2538423736` 和 build 均标为 CANCELLED；下载的日志 SHA-256 为 `9f6a670ca638ae057abe96b4e405841c1e074ec2330e02c312c115e4a70478a5`，逐字节相同。原容器已回收，T-3 完成 |
| 重启控制面，在 Chrome 回读执行与事件并查询远端 Issue | 已取消任务保持终态，重复请求安全；来源 Issue 仍开放 | 浏览器显示 Owner 取消结算事件，取消按钮已移除。原执行保留请求时间，无交付记录，GitHub 回读 OPEN。核对后另行关闭验收 Issue |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 150 项通过，零测试跳过。已覆盖并发重报、成功回执与取消交错、过期租约结算和非 Owner 请求。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

Owner 主动取消运行中任务对应 ForgeOS 提交 `84758c5b8d561b37ddefc2a103c75e2b2b4e9710`。

## 专题 06、10 · 暂停 CI 后恢复同一候选的验证

真实验收使用私有 beta、PostgreSQL 18 schema `cipause_5e3a49a2`、正式 Worker、Chrome 和 Docker。时间均为 UTC+8。[验收 Issue #55](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/55)

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 等原生 Codex 结束，确认 Docker 正在运行 build，排入 T-2，再从任务页暂停 T-1 的 CI | 先固定候选并登记请求，随后停止检查、释放名额 | 执行 1，候选 `aae51b5ff79b882a489d52e9869bd11aa3e47c11`。04:21:40 点击暂停，重复 POST 返回 204。T-1 当时仍 RUNNING。04:22:09 收到原 V2 运行 `ci-run-2186646805` 的 CANCELLED 回执，进入 PAUSED，T-2 随后完成 |
| 下载原检查的 build.log，检查原容器，重启控制面后筛选 PAUSED | 暂停任务、日志及固定 SHA 可以复核 | 日志与节点原件逐字节相同，SHA-256 为 `62fde2cd4ebcf34475c2f2458b1a5ff5566f6f9567368c56ee91da7cbeeee5b2`。原容器已回收。筛选结果包含 T-1，浏览器可见暂停事件及恢复按钮 |
| 从 Chrome 恢复，重复同一次恢复请求，再启动 Worker | 只排入一次验证，复用候选 SHA、重新执行全部检查并等待 Owner 验收 | 04:22:12 进入 READY。执行 3 在新 Worktree 验证原 SHA，V2 运行 `ci-run-3733003939` 通过，04:23:46 进入 WAITING_ACCEPTANCE。新执行没有模型 Session。执行 1 的原生会话 `01a1e604-db7d-7882-9d40-5cccf796c57d` 与取消回执均留存。GitHub 仍为 OPEN，核对后单独关闭测试 Issue |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 151 项通过，零测试跳过。任务列表覆盖 PAUSED 状态，数据库用例覆盖错租约、换 SHA、提前恢复、旧请求重放和暂停期间关闭来源 Issue。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

暂停及恢复同一候选的 CI 对应 ForgeOS 提交 `a221a0d94f9484c83d0fd347f737b8972b54df94`。

## 专题 08 · Owner 在工作台管理项目资料

使用两个 Git 项目、PostgreSQL 18 schema `materialui_434da048`、正式控制面和独立 Chrome 会话。浏览器通过“项目接入 → 项目资料”操作现有的版本化资料服务。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在项目 1 导入中文标识“部署-说明”，记录来源及含标记的正文，同时在项目 2 使用同一标识 | 保存 UTF-8 原文和 SHA-256，项目独立 | 项目 1 修订 1 摘要 `c48e9095bf0d203c39b99de8cd30f9e9b41424f3500d84253f3cff6b19164a3b`，标记以文本显示；项目 2 初始列表为空，导入后的摘要为 `71cfed9fe624e5891d5a5d39564aa0030aea6c05b19afc4da2adba039ad0b84b` |
| 两个浏览器会话从修订 1 独立编辑，先保存第一份，再保存第二份 | 碰到并发修订时保留草稿，Owner 可以核对当前版本 | 第一位保存修订 2，第二位收到 409。页面同时显示修订 2 和第二位的原草稿；明确选择修订 2 为基础后保存修订 3，其摘要为 `5aca50fc003e4436eb872d8690e4acf5c6b634778c9cbd33f71042bb44fb97e3` |
| 从修订 3 回读 2、1，再切回最新版。检索关键词及环境类型，清空筛选后继续翻页 | 每版均可查到原文与来源，列表按项目和条件分页 | 三份原文、来源及按字节重算的摘要一致。组合检索 1 条，全部浏览由 30 条增至 32 条 |
| 重启控制面再从项目页检索和打开，另在新空库运行仓库内的浏览器探针 | 已保存的修订可回读，冲突场景可复现 | 两项目记录及三版摘要与重启前一致。探针资料 `probe-12f7f10441aa4fe29ccce2b660b3ab5b` 独立复验三版与并发冲突 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 151 项通过，零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

项目资料工作台对应 ForgeOS 提交 `e80e961bd779abc241c28b2f9d75a83ca37e18ce`。

## 专题 08 · 从工作台管理项目决策

PostgreSQL 18 schema `decisionprobe_f48f0d3e`，两个 Git 项目、正式控制面和独立 Chrome 会话。原决定为 #1，工作台验收的提案前缀为 `probe-1833d8e23972413cb32fa20b24f5e48e`。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 填写提案，转发真实 POST 后丢弃响应，通过原表单重试并确认 | 重试只生成一项；保留适用条件、来源、引用及确认人 | 原提案只有 #1，按原键返回 200；确认后显示 owner，重启前后其完整记录一致 |
| 两个浏览器会话分别提出替代 #1 的提案，并先后确认 | 一项生效，过时的替代请求不能再推翻前一决定 | 第一项确认后 #1 为 SUPERSEDED；第二项确认返回 409，仍可拒绝。第一项随后从工作台标记 EXPIRED；原决定列出三次操作及两项证据 |
| 浏览全部 34 项，观察有明确截止时间的测试提案，查看另一项目 | 列表继续翻页；截止后提案不再可确认；项目之间互不混入 | 先读取 30 项，再加载至 34 项。页面到期后自动移除待确认提案，切换 EXPIRED 可见该项。第二项目的全部状态为空，重启后相同 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 151 项通过，零测试跳过。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

项目决策工作台对应 ForgeOS 提交 `4713fdc9d8139e3e9f92609f44ee4651876b8e16`。

## 专题 08、09 · 在工作台发布和撤销项目间共享副本

使用 PostgreSQL 18 schema `shareui_0e5bbecf`、两个独立项目、项目 2 的专用节点身份、真实 Chrome、正式控制面和原生 Codex Management 会话。原始资料与私有标识均为验收数据。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从项目 1 打开 handoff 的修订 2，切换至修订 1，在浏览器填写共享经验并提交给项目 2 | 副本固定到所选修订，待审时接收方无记录 | 副本 #1 为修订 1；Owner 提前发布返回 409，项目 2 的浏览器显示空列表 |
| 启动源项目 Management 进程，在工作台刷新并展开审核依据 | 独立只读审核给出依据，会话可查；审核自身不发布 | 原生会话 `01a1e61b-8a48-7110-9d75-db776d7d1fed` 给出 approve，生效等级 D3。此时接收方仍为空；提交错误摘要返回 409 |
| 在 Owner 页面发布，通过接收项目的页面及其专用节点查询 | 两个入口均只收到脱敏副本；源项目记录保持私有 | 摘要 `454147c7c97a3bfc99815de59d03cf922a94a66dc23db646d5093f07b779f201`。节点响应仅含 id、title、content、digest 和 published_at；读源资料返回 403，读私有发送列表返回 401 |
| 重启、回读、从源项目的发送列表撤销，再刷新接收页面 | 发布记录在重启后保留；撤销后副本不再返回 | 接收响应重启前后逐字段一致；撤销后接收页面为空。重放原摘要发布返回 409 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 151 项通过，零测试跳过。发送列表另有分页边界、项目隔离及 Owner 鉴权回归。Go 静态检查、TypeScript 检查和 Web 生产构建通过。

跨项目共享工作台对应 ForgeOS 提交 `f7a9bbe390ca5e39d69367e7cf97685178679345`。

## 专题 07、09、14 · 活动租约中的具名 Worker 令牌轮换

私有验收仓库 [beta Issue #56](https://github.com/xfrrn/forgeos-accept-fb8e915b-beta/issues/56)，PostgreSQL 18 schema `nodekey_73884dff`。正式轮询器导入 Issue，具名 Worker 领取执行 #1。原生 Codex 会话为 `01a1e622-3279-7030-bb26-e9a65329d898`，候选提交为 `033116c5167da73068505fc0f65337c60719b2fa`。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在 Docker 构建运行期间配置与第二个节点相同的摘要并重载 | 拒绝整份配置，原权限继续工作 | Owner 收到 409；原令牌访问本项目返回 200，第二节点访问同一项目返回 403 |
| 配置独立新摘要，分别用节点和 Owner 重载 | 仅 Owner 能启用新配置。重叠期间新旧令牌拥有同样的项目权限 | 节点重载 401，Owner 重载 204。新旧令牌读取本项目均为 200 |
| 原子替换节点令牌文件，删除授权文件中的旧摘要，再次重载 | 撤销立即约束后续请求；原进程用新凭据续原租约 | 旧令牌 401，新令牌 200，新令牌读取另一项目 403。执行 #1 在 RUNNING 中的租约截止时间延长，Worker PID 25268，全程仅一个执行尝试 |
| 等待 CI 结束，通过 Owner 入口下载构建日志，再重启控制面 | 原执行完成上传，重启后保留撤销和候选状态 | CI `ci-run-2247421916` 成功，下载内容逐字节匹配本地文件，SHA-256 为 `8940cfbb78a4a7a0e66261bef38092b05f1f758b9fd983a6ea8dacfa1fb9c542`。重启后旧令牌 401、新令牌 200，候选仍为 WAITING_ACCEPTANCE |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 153 项通过，零测试跳过。Go 静态检查通过。

活动租约中轮换节点令牌对应 ForgeOS 提交 `266f11fe1c6598a047b7dac217fa7793a51d8794`。

## 专题 07、09、14 · 具名 Worker 客户端证书轮换

Windows 本机启用两个独立端口、PostgreSQL 18 schema `nodetls_275f1ec9`、专用私有 CA、两个节点授权记录与正式 Worker 进程。通过 Owner 的任务入口创建三次 Git HEAD 检查。下表的握手和 HTTP 请求均走实际 TCP 连接。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 分别省略客户端证书、使用未受信 CA 的证书、在客户端仅信任错误服务端 CA | 在 TLS 握手阶段拒绝 | 三种握手均失败。合法连接协商 TLS 1.3 |
| 混用节点 A/B 的 bearer 和证书；用 B 的完整身份读项目 1；用 A 令牌走明文回环；从节点端口访问 Owner 路由 | 证书、令牌、项目须匹配，Owner 路由独立 | 三种跨身份或项目访问均为 403，明文回环为 403；Owner 任务和授权重载路由均返回 404 |
| 用旧证书领取任务，添加新指纹并重载，替换 Worker 的证书和密钥文件，再次领取 | 两张证书在重叠期均可用；进程无须重启 | Owner 重载 204，新旧证书读取本项目均为 200。PID 45816 完成任务 #1、#2，各一个执行 |
| 保持旧 TLS 连接打开，移除旧指纹并重载；从原连接、旧证书的新连接及新证书查询，再创建第三个任务 | 撤销对后续请求即时生效，相同节点继续回报 | 保持原 TCP 套接字的请求 403，旧证书重连 403，新证书 200。相同 PID 完成任务 #3；三次回报均为 `4e85a882d1173169966556a22200034f6a3a7f04` |
| 重启控制面再查询 | 已撤销授权和任务结果持久保留 | 旧证书 403、新证书 200，任务 #3 仍为 DONE |

旧、新叶证书的 DER SHA-256 分别为 `591105d8abeb8afef6474d8ac9ce67ce5398466c14a44a4d713fd96f0cb66a86`、`dbcaefdeaef041e330e8d344df07c533571f31e38b0c7bfe24c76f61373a7d3b`。本次环境为同一主机，专题 11 的外部设备 Tailscale 验收另行记录。

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 154 项通过，零测试跳过。Go 静态检查通过。

具名 Worker 客户端证书轮换对应 ForgeOS 提交 `40736ea2141c80ac4a6f1e5b4ba7bd1f0857c72a`。

## 专题 10、13、14 · Worker 本地 CI 日志、报告和产物保留期限

从历史真实候选备份恢复 PostgreSQL、Worker SQLite、CI 运行目录和归档，以原节点身份、原控制面地址运行正式程序。下面的文件故障和旧备份还原均只发生在独立验收副本。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 读取默认策略，仅给项目 2 设置期限。用项目 1 的节点尝试修改策略，再由 Owner 将项目 1 的日志期限设为 1 秒 | 默认无限期，项目隔离，仅 Owner 能改策略 | 项目 1 初始队列为空。节点写策略 401，负数 400。Owner 设置后产生 `local_ci` 修订。另一节点冒名查询 403，以自身身份查询为空，代为确认 404 |
| 在节点离线时让三个历史日志到期，损坏 format.log，给 build.log 恢复真实待传引用，并移走中央 unit.log。启动 Windows Worker | 先修复中央副本。损坏及待传文件留在本地，其他文件独立处理 | unit.log 的中央下载与原文件逐字节相同，随后本地副本移除。format.log 和 build.log 保留，待修复及补传后均被清理；两份报告、run.json 和额外文件保持原内容 |
| 将本地策略恢复为无限期、重启服务、重新放回已清理的日志 | 已到期项继续处理，中央策略、候选回执独立保留，节点复查旧备份 | 三份中央日志和两份报告仍可逐字节下载；旧日志再次清理，重复确认返回 204。候选 `34faab0d235cbc0ea274b250826f02e232e246ff` 的原 CI 回执完整 |
| 在 Linux 对失败候选分别设置中央产物、本地报告与产物期限，将一份报告替换为指向工作区外的符号链接 | 中央 410 允许本地产物清理；符号链接及其目标保留，其他报告正常归档 | UID 65534 的 Worker 保留链接和目标，仅在恢复正确常规文件后清理该报告。两份中央报告可回读，build-1.bin 的中央读取 410，恢复旧本地产物后重启节点再次清理。三个日志与 run.json 逐字节相同 |

Windows 恢复库为 `forgeos_local_retention_d25cd87d`，运行 `ci-run-3034990605`。Linux 恢复库为 `forgeos_local_artifact_ecabc830`，运行 `ci-run-1446598496`，失败候选为 `2b14e98d37b2038b035bcd2df86dcacca79cbc21`。两个本地清理策略的修订编号均为 2。启用 PostgreSQL 18 和 Docker 的全量 Go 回归 155 项通过，零测试跳过。Go 静态检查通过。

Worker 本地 CI 文件保留策略对应 ForgeOS 提交 `9f0c70425ac15942d093bcf6f940754486832c2f`。

## 专题 11 · 从预览扩展截图到 Owner 确认归档

在 Chrome for Testing 156.0.8078.4 加载仓库内的扩展，用正式控制面和 Web 生产构建打开固定候选 `304948ec31c7e6616230934f8d6de1b97f6da2cf`。PostgreSQL 18 恢复库为 `forgeos_capture_982e64b4`。测试在实际候选页面的 DOM 中添加了五个合成隐私区域，便于逐像素核对遮挡。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 从工具栏配对工作台，打开候选，点击“截取当前视口” | 采集原文档且排除受保护控件；能丢弃草案 | 普通洋红色参考块保持原像素，密码框、文本区、iframe、显式私有区域和封闭 Shadow DOM 宿主五处均呈背景白色。源控件随后恢复可见。截图和工作台地址不携带原页面的查询参数或片段 |
| 在采集期间连续改变隐私标记；对稳定页面重新采集，再从扩展页打开工作台 | 隐私边界变化时拒绝传图；稳定时仅把草案传给配对的工作台 | 变化中的页面显示重试提示，发送按钮不可见；另一稳定草案进入已登录工作台。登记前反馈计数保持原值，丢弃入口也从真实工具栏验收 |
| 提交观察后丢弃第一次 HTTP 回应，再次提交；收起并重开截图编辑器 | 复用反馈编号并恢复原图，需要 Owner 单独确认才产生附件 | 两次提交均对应反馈 #7，登记后附件数为零。“重新载入扩展截图”恢复原图。在参考块上添加遮挡后点击“确认并提交截图”，得到唯一附件 #10 |
| 核对上传像素、错版本和控制面重启后的下载 | 已遮挡像素与回执一致；错版本不可提交；归档可恢复 | 参考像素从洋红变为黑色，五个私有位置保持白色。归档 PNG 为 784 × 292、1573 字节，SHA-256 为 `44680831acd10ab0519f32f5242448e6dbd138700286be667b7332da86ad9df7`；重启下载字节一致。改为其他提交后，工作台显示版本不符，提交按钮禁用 |

`scripts/probe_preview_extension.py` 的实际工具栏回归通过，覆盖此次截图与既有 DOM、Canvas、嵌套 iframe、取消和隐私行为。TypeScript 检查、Web 生产构建、Go 静态检查通过。启用 PostgreSQL 18 和 Docker 的全量 Go 回归 155 项通过，零测试跳过。

预览扩展截图对应 ForgeOS 提交 `732a4a89b1d034d2b643dc71c36e94b1c86e2456`。

## 专题 11、13 · 在反馈工作台归档、回放和清理录屏

从 Chrome 录制真实候选预览 `304948ec31c7e6616230934f8d6de1b97f6da2cf`，加入用于核验的音轨、标题后，在独立恢复库 `forgeos_clip_fd0e9828` 通过 Owner 页面归档。此处验收文件进入工作台后的全过程。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 选取录屏并回放，在上传回应返回浏览器之前断开，再次确认 | 提交前仅本地回放，丢失回应后重试复用同一回执 | 反馈 #7 原有截图 #10；选择视频后仍只有一份附件。两次提交后只有一个录屏 #11 |
| 在工作台回放已归档文件，检查下载轨道、大小、摘要，重启控制面再下载 | 可播放，保留视频及时间信息，音轨和输入标题移除 | 已在 Chrome 播放并暂停。640 × 360、1.807 秒、16809 字节。FFprobe 确认单一视频轨且原测试标题已移除。SHA-256 为 `aedb85417dc8624e4e6e2e3df96ed8a7e7d101c856e79e14adb8193076924402`。重启后字节相同 |
| 用节点身份或其他任务路径读取附件，上传无效内容 | 限定 Owner、本任务与有效视频 | 节点返回 401，跨任务读取 404，无效上传 400 |
| 将本项目录屏保留期设为 1 秒，到期后恢复为无限期并重启 | 录屏到期不可恢复，截图仍有效，记录可审计 | 节点改策略 401。录屏读取及重传返回 410，关联修订 #1。重启清理了物理 WebM；反馈仍有已到期的录屏 #11 和可读取的截图 #10 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 156 项通过，零测试跳过。Go 静态检查、TypeScript 检查与 Web 生产构建通过。

反馈录屏归档与保留期限对应 ForgeOS 提交 `6280acdf49459b731cf283087e677a281fce251b`。

## 专题 11、ADR-014 · 从候选标签页录制交互并绑定反馈

在 Chrome for Testing 156.0.8078.4，从原生工具栏启动和停止扩展录制，使用正式控制面、工作台生产构建及 PostgreSQL 18 恢复库 `forgeos_native_record_5d233bdc`。候选 `304948ec31c7e6616230934f8d6de1b97f6da2cf` 的实际预览中放置了可点击的参考色块和五处合成隐私内容。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 在候选上开始录制，切换到独立红色标签页，再回原候选点击参考块，停止录制 | 录下原标签页的交互；其他标签页和私有控件不进入证据 | 共解码 9 帧，未出现红色标签页。密码框、文本区、iframe、显式私有区域和封闭 Shadow DOM 的采样点在所有有画面的帧中均为白色。回放中参考块从 RGB(199,0,201) 变为 (21,190,45)。停止后源控件恢复 |
| 预览录屏，打开工作台，登记观察，再确认提交录屏 | 绑定固定任务、预览、执行及提交；只有 Owner 确认上传后生成附件 | 登记前反馈数量未变；反馈 #7 初次生成时附件为空。Owner 本地回放并确认后生成附件 #10，800 × 500、2.473 秒、1986 字节，FFprobe 仅见 VP8 视频轨 |
| 重启控制面，再次下载，并在独立工具栏回归中测试取消、中途换文档与隐私变化 | 归档可回读；丢弃的片段不能发送，页面遮挡能撤销 | 重启下载字节一致，SHA-256 为 `4f109dd195c49793341641c818d784158425af18b68cd79b41dec10aa654f017`。录制中插入后移除封闭 Shadow DOM、重载源文档均触发丢弃。关闭录屏核对页后，表单恢复可见 |

同版工作台还重跑了扩展截图、反馈提交丢失后的重试和 PNG 归档，原遮挡像素与摘要一致。启用 PostgreSQL 18 和 Docker 的全量 Go 回归 156 项通过，零测试跳过，Go 静态检查、TypeScript 检查和 Web 生产构建通过。

### 专题 11 · Worker 动态应用预览生命周期

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| 启动真实 Worker Preview Manager，以固定提交申请 local-mock，将工作区启动脚本改成退出 | 运行提交中的服务，保持 Docker 沙箱隔离 | 候选 `04726f294b96963ccc1753dd42274eeca3c80bba` 启动了容器 `b5f66017996c`。应用只监听容器内 127.0.0.1，Docker 显示网络 none、用户 65534:65534、无端口映射 |
| 重复申请同一实例，提交带查询串和正文的 POST，再重启 Manager | 复用原实例和内存状态，服务凭据与浏览器 Cookie 不进入应用 | 同一租期返回 204，不同租期返回 422。应用核对请求头后返回 201 和计数 2。重启 Manager 后 GET 仍为 2，HEAD 无正文 |
| 连续两次停止，再新建 8 秒实例并退出 Manager | 停止可重试，过期无需控制进程在线 | 两次停止均为 204，旧地址返回 404，Docker 查无原容器。新实例初始值为 0，离线等待 9 秒后 Docker 查无该实例 |

### 专题 11 · 工作台打开固定候选的动态应用

使用 PostgreSQL 18、正式 Worker 和控制面、Docker、Chrome 及单独的 Caddy 进程。测试库 schema 为 `dynamic_preview_a80dd6c1`。Worker 从固定候选完成实际验证，任务 #1 的执行 #1 进入 WAITING_ACCEPTANCE。随后将克隆的启动脚本改成退出。

| 操作 | 预期 | 实际 |
| --- | --- | --- |
| Owner 从任务详情打开“本机预览”，启动 local-mock，打开应用并点击 Add two | 应用来自通过验证的候选，能处理带正文和查询的同源 POST | 项目、任务、执行、工作树和 SHA `87dbdee4aa4fd35598abe4c0f885d40eed9e99cd` 与执行相符。Chrome 显示 2，GET /state 返回 200、同一 SHA 和正文 2。错误 SHA 返回 409，Worker 凭据返回 401。跨来源 POST 返回 403 |
| 重复申请，重启控制面与 Manager，再经 Caddy 的候选 HTTPS 域名提交同源 POST | 原状态保留，私网域名能执行操作，网关和服务凭据不会进入应用 | 重复申请返回相同回执。重启后读到 2，持客户端证书的请求使计数变为 4，返回 201。应用核对未收到 Authorization、Cookie 或内部实例头。无证书握手失败 |
| 从工作台停止，再打开历史反馈。创建 8 秒预览并等待 9 秒 | 停止和到期均撤销路由并移除容器；历史反馈可查看 | 两个地址都返回 404，均查无容器。工作台历史页显示 local-mock，两个回执分别为 stopped 和 expired。相同候选的静态目录仍返回 200 |
| 关闭 Manager 后重新申请动态预览 | 报告启动失败，不增加活动回执，独立的静态预览仍可用 | API 返回 503，查询所得历史与申请前一致，静态地址仍为 200 |

启用 PostgreSQL 18 和 Docker 的全量 Go 回归 159 项通过，零跳过；Go vet、TypeScript 检查和 Web 生产构建通过。网关实测在本机完成。外部设备经 Tailscale 访问及独立 Staging 的第三方回调仍待验收。
