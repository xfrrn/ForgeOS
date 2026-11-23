# ForgeOS 工作台

修改或新增工作台页面前，必须先阅读 [工作台界面规范](../docs/workbench-ui.md)。所有页面、布局、动效与共享组件必须严格遵循其中的 **AionUi × Aero Mist** 视觉与交互规则。

## 硬性约束（每次改动必核对）

- **复用设计系统与组件**：统一复用 `src/style.css` 的语义变量（`--shell-bg`、`--canvas`、`--surface-subtle`、`--surface-muted`、`--brand`、`--ease-spring` 等）、`.panel`、`.button`，以及 `src/ui.tsx`（`PageHeading`、`MetricCard`、`EmptyState`、`BackLink`、`SkeletonCard`、`SkeletonLines`）与 `src/shared.tsx`（`Status`、`Icon`、`Fact`）。详情专属样式维护在 `src/detail.css`。
- **导航与架构**：新增全局或项目内入口统一纳入**左侧可收起侧边栏**（并在窄屏自动归入滑出抽屉），禁止在页面顶部继续堆叠多层横向导航条。次级视图切换优先放入侧边栏子菜单，或使用单行紧凑分段控件。
- **信息层级与可扩展性**：
  - 详情与知识类页面必须按「头部标识与主操作 → 核心正文卡 → 原文引句与结构化元信息网格 → 可折叠审计/历史抽屉」分层，禁止平铺无层级的纯文本 `<p>` 墙。
  - 面对后续可能大量增长的集合（如 Skill、模型、文档、条目），禁止全量平铺复选框或无限长列表，必须使用「已选胶囊 + 按需展开的限高搜索/筛选选择器」或带搜索与分批加载的网格/分栏列表。
  - 时间线/路线图禁止左右双栏重复列举同一套节点，保持单列聚焦与排期进度直观可读。
- **丝滑动效与严格 CSP**：交互与折叠动效统一使用 `--ease-spring` 与 `--dur-fast` / `--dur-base` / `--dur-slow`，并兼容 `prefers-reduced-motion`。控制面强制 `default-src 'self'` CSP，**严禁引入外部字体/样式 CDN、内联 `<style>`、内联 `style` 属性（除 Portal 动态定位外）或 CSS `data:` URI**。
- **业务完整性与验证**：通过真实控制面数据渲染指标和记录，完整保留现有路由、筛选、权限、确认及证据入口。提交前必须从仓库根目录运行 `npm --prefix web run build`，并在桌面（1440px）与窄屏（390px）核对效果；变更共享设计规则时同步更新 `docs/workbench-ui.md`。
