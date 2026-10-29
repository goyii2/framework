# 25: 嵌套 Module 落地

**Status:** resolved

**What to build:** 应用可注册嵌套模块（如 admin 模块下再挂 report 子模块）：路由前缀逐级拼接、控制器沿模块树分层查找、命名路由自动携带模块路径（如 `admin/report/dashboard`），并补齐模块级中间件 `ModuleRoutes.Use`（对应 Yii2 module beforeAction 的能力位）。嵌套此前仅有桩（`RegisterModule` 固定返回不支持错误），本票将其变为可用能力。不做模块级组件隔离——组件仍归全局 Application。

**Blocked by:** None (can start immediately)

- [x] RegisterModule 真正挂接子模块：前缀拼接、controller 分层查找，ErrModuleNestingUnsupported 消失
- [x] 模块级中间件作用于该模块全部路由（形态对齐 controller 级中间件先例）
- [x] 命名路由自动带模块路径，url.To 反向生成命中
- [x] 模块 Layout 隔离在嵌套下保持（子模块可覆写）
- [x] 单测覆盖：嵌套路由解析、反向生成、分层查找、Layout 继承；重复前缀/冲突仍拒绝

## Comments

- 2026-10-29 实施完成（commit 89d06c4，分支 feat/25-nested-modules 已 ff 合并回 main）。

  实现要点：
  - `base.Module.RegisterModule(child)` 落地：子模块前缀须恰为父前缀 + 一段（`ErrInvalidSubmodulePrefix`），同时落入应用扁平模块表（前缀全局唯一，重复 `ErrDuplicateModulePrefix`）；`Module.Parent()/Modules()` 暴露树结构。`ErrModuleNestingUnsupported` 已删除。
  - `web.ModuleRoutes.Use(mws...)` 模块级中间件：请求期解析，作用于该模块前缀下全部路由——显式挂载与约定回退一视同仁，嵌套子模块路由也包在内（祖先模块中间件在外层，控制器中间件最内），复用既有 `withMiddleware`（controller 级先例形态）。
  - 命名路由自动携带模块路径：`ModuleRoutes` 挂载的 `Route.Name("dashboard")` 注册为 `admin/report/dashboard`；`url.To` / `Handler.URLs().URL` 反向生成命中。

  验收证据（`go test ./...` 全绿，golangci-lint 0 issues）：
  - 嵌套路由解析 + 分层查找 + 隔离：`web.TestNestedModuleConventionResolution`（/admin/dash/index 与 /admin/report/user/list 各自解析；跨层泄露均 404）
  - 反向生成 + 自动模块路径：`web.TestNestedModuleExplicitRoutesAndNames`（URL(admin/report/dashboard) == /admin/report/dashboard；url.To 带参命中 /admin/report/user/4）
  - 模块级中间件：`web.TestModuleMiddlewareWrapsModuleRoutes`（显式子模块路由 trace = global -> admin-mw -> report-mw；模块约定路由 = global -> admin-mw -> controller；前缀外 = global）
  - Layout 继承/覆写：`web.TestNestedModuleLayoutOverride`（子模块 report-frame 生效、父模块 admin-frame 不受扰、页面级覆写仍最高优先）
  - 冲突拒绝：`base.TestModuleNestingRejectedPairs`（nil/foreign/前缀不恰多一段/全局重复前缀）、`base.TestModuleNestingRegistersChild`（含二级嵌套挂接）、`base.TestApplicationModuleRegistry`（原有重复/外属/nil 拒绝保持）
  - 既有测试回归：`web.TestModuleScopedExplicitRoutes` 更新为模块相对命名语义后通过；`web.TestMultiSegmentModulePrefixResolves`（扁平长短前缀）不受影响。

  备注：`go test -race` 在本机不可运行（无 cgo/gcc，Windows 环境），`base.TestModuleConcurrentAccess` 约定 `-race` 下验证；无待人工项。examples/basic 新增 /admin + /admin/report 二级嵌套 demo（模块级 AccessControl + 模块相对命名，`examples/basic` 测试通过）。
