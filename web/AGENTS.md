# ForgeOS 工作台

修改工作台前先读 [界面规范](../docs/workbench-ui.md)。页面和共享组件都遵循其中的视觉、交互和验收规则。

- 复用 `src/style.css` 的语义变量、`.panel` 与 `.button`，以及 `src/ui.tsx` 和 `src/shared.tsx` 的共享组件。详情专属布局维护在 `src/detail.css`。
- 新增入口沿用顶栏及窄屏菜单。通过真实控制面数据渲染指标和记录，保留现有路由、筛选、权限、确认及证据入口。
- 提交前运行 `npm --prefix web run build`，在桌面与窄屏检查受影响的页面和状态。共享设计规则变更时同步更新界面规范。
