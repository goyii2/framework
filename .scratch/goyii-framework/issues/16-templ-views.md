# 16: templ 视图层

**Status:** claimed

**What to build:** 视图层（PRD P1）：视图注册表——`ctx.Render("post/view", data)` 按名查表（map 查找零反射），亦支持直传 templ 组件；Layout 以组件组合实现（模块级/页面级可覆盖）；Widget 最小集四件（LinkPager/Breadcrumbs/Menu/DetailView）。不做 AssetManager（PRD 放弃清单，替代路径在 P2）。

**Blocked by:** 04

- [ ] 按名渲染与直传组件两条路径均返回完整 HTML
- [ ] Layout 可按模块/页面覆盖，公共外框只写一遍
- [ ] Widget 四件可用且各自单测
- [ ] 错误页可经视图注册表自定义覆盖（联动 04 的错误处理器）
