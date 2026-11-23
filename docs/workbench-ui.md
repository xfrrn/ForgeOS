# ForgeOS 工作台界面规范

本规范适用于 `web/` 下的全部工作台界面，包括登录、主框架侧边栏、项目目录、项目概览、功能话题、项目聊天、任务与详情、项目规划、资料与知识库以及系统设置。整体视觉参考 **AionUi（Arco Design 中性清透体系 + 桌面级 Cowork 内嵌工作台）** 并结合淡雅浅色科技风，使用 ForgeOS 的品牌、术语、路由与真实控制面数据。

## 设计变量

在[全局样式](../web/src/style.css)中维护 CSS 自定义属性。新增页面优先复用语义变量，所有视觉效果纯 CSS 实现以严格遵守控制面 `default-src 'self'` CSP 策略（禁止外部字体 CDN、内联 `<style>` 或 `data:` URL 图片）。

| 用途 | 变量与规则 |
|---|---|
| 壳体与画布 | `--shell-bg`（`#f3f5f8`）用于柔雾浅灰外框与侧边栏背景；`--canvas` / `--surface`（`#ffffff`）用于内嵌圆角纯白主工作区；`--surface-subtle`（`#f7f8fb`）与 `--surface-muted`（`#f2f4f8`）用于柔和填充卡片、控件与目录栏。 |
| 文本与层级 | `--ink`（`#1d2129`）用于主标题与正文；`--ink-soft`（`#4e5969`）用于次级正文与标签；`--muted`（`#86909c`）用于时间、路径与辅助说明。 |
| 边框与焦点 | `--line`（`#eaeef4`）与 `--line-strong`（`#d9e1ec`）用于轻盈分隔线；`--focus`（`#4080ff`）与 `--shadow-focus` 用于可见焦点与输入框柔光环。 |
| 品牌与 AI 点缀 | `--brand`（`#165dff`）用于主操作与当前激活图标；`--accent` / `--accent-ink` / `--accent-line` 用于淡蓝选中态；`--ai-gradient` 与 `--ai-tint` 克制用于品牌徽标、AI 对话角标与流式呼吸光标。 |
| 业务状态 | `--positive`（青绿）、`--warning`（柔琥珀）、`--danger`（淡玫瑰红）、`--info`（清脆蓝）各自对应 `-bg` 与 `-line`。`RUNNING` / `ACTIVE` 与在线 Worker 节点带柔和生命力脉冲光环（`pulse-ring`）。状态始终同时提供颜色与文字。 |
| 圆角与阴影 | `--radius-sm` 8px、`--radius-control` 10px、`--radius-md` 12px、`--radius-panel` 14px、`--radius-xl` 20px、`--radius-full` 999px。侧边栏与分段控件选中态采用纯白悬浮胶囊 `--shadow-pill`。 |
| 丝滑动效令牌 | 统一采用弹簧减速曲线 `--ease-spring: cubic-bezier(0.16, 1, 0.3, 1)`、`--dur-fast: 180ms`、`--dur-base: 280ms`、`--dur-slow: 400ms`，并完整遵守 `prefers-reduced-motion: reduce`。 |

主文本使用系统无衬线字体栈（`"Inter", "Geist", "PingFang SC", "Segoe UI", "Microsoft YaHei UI", sans-serif`），编号、SHA、路径与代码使用 `--font-mono`，所有计数与时间戳启用 `tabular-nums`。

## 布局与导航（桌面级内嵌工作台架构）

- **外框 + 内嵌纯白主画布**：桌面端（`>= 960px`）左侧为可收起侧边栏（展开 `244px` / 收起 `68px`，记忆用户偏好），右侧为 `16px` 圆角纯白工作画布（`.app-main`），顶部内嵌 `52px` 精简吸顶栏（实时面包屑 + 通知角标 + Owner 菜单），避免多层横向导航堆叠占用视野。
- **上下文导航切换**：未进入项目时侧边栏展示全局工作台入口（项目、Skill 库、Agent 管理、运行环境、密钥与凭据）；进入项目后自动切换为项目上下文卡片（带快捷返回所有项目与仓库标识）+ 三组项目导航（协作与智能 / 规划与交付 / 知识与配置）及展开式子菜单；收起侧边栏时自动在页面顶部提供紧凑分段子导航。
- **窄屏适配（`< 960px`）**：主画布全屏铺满，左侧边栏切换为平滑滑出抽屉（带磨砂遮罩，支持 `Escape` 关闭与路由切换自动收起）。
- **切页与交互动效**：所有路由切换包裹在 `.view-stage` 中平滑入场；项目目录卡、指标卡与聊天轮次提供轻量级交错渐入（Stagger）；侧边栏及目录列表悬停提供 `2px` 微位移与点击按压缩放反馈。

## 组件与页面规范

| 场景 | 实现规则 |
|---|---|
| 标题、指标、骨架与空态 | 复用[基础界面组件](../web/src/ui.tsx)中的 `PageHeading`、`MetricCard`、`EmptyState`、`BackLink`、`SkeletonLines`、`SkeletonCard`。 |
| 按钮与表单 | 默认按钮采用柔和浅灰无框填充（`.button`），主操作采用清脆蓝实底（`.button.primary`）；输入框与搜索框默认柔灰无框填充，聚焦平滑切换为纯白并亮起柔和蓝光环。 |
| 状态与详情 | 复用[共享组件](../web/src/shared.tsx)的 `Status`、`Icon` 和 `Fact`。任务详情顶部使用柔灰摘要英雄卡，执行记录采用可平滑旋转箭头的手风琴，代码 Diff 使用浅色语义行底色（[detail.css](../web/src/detail.css)）。 |
| 主题下拉框 | 表单、模型选择与筛选器统一使用 `ThemedSelect`（[themed-select.css](../web/src/themed-select.css)），支持方向键、Enter、Escape 与智能上下展开，浮层带 `pop-in` 动效。 |
| 功能话题与项目聊天 | 采用全视口分栏 AI 工作室布局（[feature-topics.css](../web/src/feature-topics.css)、[project-chat.css](../web/src/project-chat.css)）。AI 回复无冗余外框直接排版，用户消息采用柔和冰蓝气泡，底部为一体化 20px 大圆角悬浮 SendBox 输入坞。 |
| 项目规划、资料与知识库 | 规划时间线使用发光圆环里程碑节点；资料库与决策/经验/共享知识库统一采用「左侧柔灰目录 + 悬浮白胶囊选中态 + 右侧纯白阅读画布」分栏结构。 |
| 加载、空结果、失败 | 初次加载提供骨架屏或 `role="status"` 反馈；空态说明原因并提供下一步操作；错误明确展示原因并支持重试。所有前端改动保留原有深链接、权限判断、确认流程与证据入口。 |

## 开发与评审

1. 新建或修改页面前，优先复用 `style.css` 语义变量、`ui.tsx` 与 `shared.tsx` 组件及统一的动效曲线。
2. 提交前从仓库根目录运行 `npm --prefix web run build`（含 `tsc --noEmit && vite build`）。
3. 在桌面（1440px）与窄屏（390px）检查受影响页面：核对侧边栏展开/收起与抽屉、键盘焦点（`:focus-visible`）、长中文标题与路径断行、表格与日志滚动，以及加载/空态/失败/有数据状态。
