# 25: 嵌套 Module 落地

**Status:** ready-for-agent

**What to build:** 应用可注册嵌套模块（如 admin 模块下再挂 report 子模块）：路由前缀逐级拼接、控制器沿模块树分层查找、命名路由自动携带模块路径（如 `admin/report/dashboard`），并补齐模块级中间件 `ModuleRoutes.Use`（对应 Yii2 module beforeAction 的能力位）。嵌套此前仅有桩（`RegisterModule` 固定返回不支持错误），本票将其变为可用能力。不做模块级组件隔离——组件仍归全局 Application。

**Blocked by:** None (can start immediately)

- [ ] RegisterModule 真正挂接子模块：前缀拼接、controller 分层查找，ErrModuleNestingUnsupported 消失
- [ ] 模块级中间件作用于该模块全部路由（形态对齐 controller 级中间件先例）
- [ ] 命名路由自动带模块路径，url.To 反向生成命中
- [ ] 模块 Layout 隔离在嵌套下保持（子模块可覆写）
- [ ] 单测覆盖：嵌套路由解析、反向生成、分层查找、Layout 继承；重复前缀/冲突仍拒绝
