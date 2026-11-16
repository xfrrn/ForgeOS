# 01 — 端到端项目流程

## 1. 流程总览

```mermaid
flowchart TD
    A["项目接入与初始化"] --> B["需求与任务接入"]
    B --> C["任务识别与分类"]
    C --> D["项目知识与上下文"]
    D --> E["Goal 分析与任务规划"]
    E --> F["执行编排与 Harness 管理"]
    F --> G["代码审查与自动化验证"]
    G --> H["人工测试与验收"]
    H --> I["代码交付与集成"]
    I --> J["结果反馈"]
    J --> K["归档与经验沉淀"]
    G -.失败修复.-> F
    H -.验收失败.-> F
    K -.知识更新.-> D
```

## 2. 阶段定义

### 01 项目接入与初始化

- 用户先在本地机器准备项目目录和 Git 仓库。
- 中台导入项目路径和远程仓库身份。
- Agent 扫描仓库入口文档、技术栈、依赖、启动和测试命令。
- 识别项目声明的密钥需求。
- 配置 Local Mock / Integrated 环境并进行启动验证。
- 生产凭据不参与普通初始化。

交付物：Project Profile、Repository Mapping、Environment Inventory、Secret Requirement Inventory、初始化报告。

Owner 在工作台选择“项目接入”，登记控制面机器上的 Git 仓库，随后运行 HEAD 检查。任务详情展示实际执行的 Worker、Worktree、提交号、退出码和事件。已有项目会列出最近一次 HEAD 检查。仓库尚无提交时可先检查失败回执，补上初始提交后在项目页重新检查。项目环境的启动验证和初始化报告分别验收。

成功的 HEAD 检查可在任务详情“读取仓库清单”。控制面按回执中的提交遍历已跟踪的普通文件，为入口文档、包清单、锁文件、构建与启动配置、ForgeOS 配置保存路径和 Git blob。Go 模块列出声明的依赖模块名。Node.js 包和 PHP Composer 清单分别列出包依赖及项目定义的脚本名。其他已识别的依赖清单先提供文件引用。打开历史清单可核对扫描时的 GitHub origin 和控制面绑定的仓库数字 ID。仓库更新后在项目接入页选择“检查当前 HEAD”，从新任务生成新版本的清单。

对于 `composer.json`，清单列出 `require` 和 `require-dev` 中的包及平台依赖。展开文件可查看 `composer run-script` 入口。扫描只归档名称和来源 blob，脚本内容及仓库地址配置不进入回执。[Composer 项目配置](https://getcomposer.org/doc/04-schema.md) · [Composer 脚本](https://getcomposer.org/doc/articles/scripts.md)

对于 Python，`pyproject.toml` 清单列出运行、可选、构建及依赖组中的包名，以及项目和 Poetry 声明的命令入口。包名按 PyPA 规则统一小写，把连续的点、短横线与下划线合并为短横线。动态依赖或命令字段单独标明。`requirements.txt` 支持具名直接引用、版本标记、续行和哈希选项；缺少可直接确认包名的路径、外部引用、可编辑安装及包含文件计入未解析项。扫描不运行构建后端或安装依赖。[项目元数据](https://packaging.python.org/en/latest/specifications/pyproject-toml/) · [依赖组](https://packaging.python.org/en/latest/specifications/dependency-groups/) · [pip requirements 文件](https://pip.pypa.io/en/stable/reference/requirements-file-format/)

Rust Cargo 清单列出普通、开发、构建及平台条件下声明的直接包名。重命名依赖使用实际包名。对 `workspace = true` 的条目，扫描在同一提交内查找工作区声明，记录根清单路径和 blob。虚拟工作区展示可供成员继承的共享依赖。无法从归档确认的引用计入未解析来源。此清单汇总各平台声明，具体构建选用的依赖另由 Cargo 确定。[Cargo 工作区](https://doc.rust-lang.org/cargo/reference/workspaces.html) · [Cargo 依赖声明](https://doc.rust-lang.org/cargo/reference/specifying-dependencies.html)

Maven 清单从固定提交的 `pom.xml` 读取依赖坐标。若子项目的相对路径和父项目坐标均匹配，工作台显示父 POM 的路径及 blob，并使用这条继承链上的属性。直接依赖、dependencyManagement 声明、导入的 BOM 和各 profile 的条件声明分开列出。远端父 POM、环境变量及 settings 引用计入未解析来源。扫描不下载 BOM 或运行插件，profile 的实际激活由 Maven 决定。[Maven POM](https://maven.apache.org/pom.html) · [Maven Model Builder](https://maven.apache.org/ref/3.9.11/maven-model-builder/)

Owner 用 `POST /tasks/{id}/repository-inventory` 生成或复用清单，`GET` 读取归档。首次扫描需要本机克隆仍有对应提交，归档随项目数据库保存。无效或过大的清单会在对应条目标明原因。整棵树的 Git 列表上限为 16 MiB，最多归档 2000 个匹配文件；解析单个包清单最多读取 256 KiB，每次累计 8 MiB。符号链接、子模块、vendor 和 node_modules 目录不作为初始化输入。依赖声明与脚本名称仅作发现依据，启动及测试结果另有实际执行回执。

需要声明项目密钥时，将环境、名称和预期风险写入根配置的 `secret_requirements`。扫描结果附在该文件的条目中。实际元数据登记与执行授权见[密钥需求和授权](09-security-and-secrets.md#项目声明的需求)。

在仓库清单选择“归纳入口文档”，对应项目的 Management Agent 会读取归档版本的入口文档原文和包清单摘要。工作台按项目用途、技术栈、依赖、启动及测试命令展示结果，展开一项可核对引用与来源 blob。启动及测试命令必须是所引来源内的原文；实际运行结果由对应检查记录。

选择“归档当前报告”可保存这次 HEAD 检查的项目画像、仓库映射、环境与密钥需求清单，以及归档时已有的启动回执。尚未派发的归纳、未记录的声明、未执行的检查和失败结果分别显示。新证据出现后可生成下一版，使用“报告版本”切换，或将选中的版本下载为 JSON。

在同一文件的 `local_environments` 中声明本地启动脚本和 HTTP 就绪路径。在工作台展开根配置，选择环境的“启动检查”。Worker 领取该任务，按来源 HEAD 的 Git 提交运行脚本并探测 HTTP 就绪路径。任务详情保留执行节点、容器清理和探测结果。集成环境通过项目专属的 Docker 内部网络访问测试依赖。也可在执行节点单独运行 `worker verify-environment`。[启动配置和回执](local-development.md#验证项目的本地启动环境)

### 02 需求与任务接入

- GitHub Issue 是第一版主要远程入口。
- 中台 Web 是辅助入口。
- Agent 任务模板自动进入待调度流程。
- 普通讨论模板只进入讨论收件箱，不启动 Agent。
- 允许普通讨论后续转换为 Agent 任务。
- 本地 Poller 为主要同步方式，Webhook 可作为补充。
- 事件去重、需求版本、角色权限必须具备。

### 03 任务识别与分类

主要类型：
- Development；
- Code Review；
- Testing；
- Investigation；
- Operations。

分类规则优先，必要时调用分类 Agent。意图不清楚时进入澄清，不擅自扩大任务范围。

### 04 项目知识与上下文

- Agent 自主读取仓库文档和代码。
- 中台提供项目结构化信息 API 和知识检索 API。
- 不把整个仓库预先塞入模型上下文。
- 按 Agent 角色提供不同的启动提示词和工具权限。
- 检索结果必须包含来源、状态和适用范围。

### 05 Goal 分析与任务规划

- 简单任务直接 Work Item。
- 复杂任务使用 Goal → Milestone → Work Item。
- Planner 先 Discovery，再做滚动规划。
- Work Item 必须有目标、范围、依赖、验收标准和交付物。
- 形成 DAG，避免循环依赖。
- Management Agent 审核计划。
- Policy Engine 检查权限。
- 大型 Goal 的正式计划和重要变更同步 Git；运行状态留在中台。

### 06 执行编排与 Harness 管理

- Orchestrator 调度任务。
- Worker Daemon 在本地执行机器运行。
- 第一版复用已登录的 Linux 用户身份和本地 Harness 登录状态。
- 每个写入型 Work Item 使用独立 Git Worktree。
- Harness Adapter 管理 Codex、Claude Code 等 CLI。
- Session 与 Process 分离管理。
- 命令进入 Command Queue，普通变更在安全检查点处理。
- 等待人工输入时保存检查点，尽可能恢复原生 Session。

### 07 代码审查与自动化验证

- 项目仓库声明 `validation.yaml`。
- 中台结合任务类型、Git Diff、敏感模块和交付目标选择 V1-V4。
- Review Agent 使用只读环境和固定 Commit。
- Local CI Runner 执行确定性命令。
- 所有验证结果绑定 Commit SHA。
- 失败分类后交给开发 Agent、基础设施处理或人工决策。

### 08 人工测试与验收

- 全局 Acceptance Playbooks 提供通用验收方法。
- 项目仓库 `acceptance.yaml` 声明模块特殊验收要求。
- Test Agent 生成具体测试计划。
- Management Agent 检查覆盖。
- 需要人工操作时，Preview Manager 启动预览。
- 用户通过 Tailscale 私网访问。
- 浏览器反馈扩展支持元素批注、截图、录屏和问题报告。
- 验收记录绑定 Commit SHA。

### 09 代码交付与集成

- 人工验收通过后冻结验收 Commit。
- 推送任务分支。
- 自动创建 GitHub PR。
- 检查 CI、Review、冲突、权限和分支保护。
- 默认 Squash Merge。
- 目标分支由项目配置指定，默认仓库默认分支。
- 成功后记录 Merge Commit SHA。
- 不触发生产部署。

### 10 结果反馈

- 更新 Task / Goal 状态。
- 根据任务层级判断是否关闭 Issue。
- 生成交付报告。
- 更新 GitHub Issue 和中台面板。
- 对需要人工处理的事项发送通知。

### 11 归档与经验沉淀

- 归档需求、计划、决策、Execution、Session 关联、Commit、CI、验收、PR。
- 清理安全可清理的临时资源。
- Knowledge Curator 提取知识建议。
- Management Agent 审核。
- 正式决策、工程规则和跨项目共享遵循各自审批规则。

## 3. Issue 关闭规则

- 简单 Work Item：PR 合并成功后可关闭对应 Agent Issue。
- 复杂 Goal：所有必需 Work Item 和 Milestone 完成后才关闭父 Issue。
- 普通讨论 Issue：不自动关闭。
- 失败或取消：保留 Issue 并记录结果。
- 多个子 PR 使用引用关联父 Issue，不能由第一个子 PR 提前关闭父 Issue。
