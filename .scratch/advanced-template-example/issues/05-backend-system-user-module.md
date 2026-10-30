# 05: /backend/system 嵌套子模块——用户管理

Status: resolved
Blocked by: 03（backend 模块、RBAC 守卫与 seed）

**What to build:** backend 模块下嵌套 `system` 子模块（`/backend/system`）：用户管理列表控制器经模块 `RegisterController` 注册、走约定回退路由（如 `/backend/system/user/list`），DataProvider 分页，后台导航加入口。与 basic 的 `/admin/report` 显式挂载形成互补展示（约定式 vs 显式式），并验证嵌套模块继承祖先模块的守卫与中间件顺序。

- [x] `/backend/system` 经 backend 分区装配为嵌套模块（父模块注册，前缀恰好多一段）
- [x] 用户列表控制器经 `RegisterController` 约定路由可达，无显式 Get/Post 声明
- [x] DataProvider 分页生效（翻页参数改变结果集），列表渲染于后台深色 Layout
- [x] 守卫继承：未登录/无角色访问 `/backend/system/*` 与 `/backend/*` 行为一致（302/403）
- [x] 分页路由 + 守卫继承 httptest 测试通过
- [x] `go build ./...` 与 `go test ./...` 通过，gofmt 干净

## Comments

### 验收证据（resolve）

**实现**（backend 分区内，未动框架包与 examples/basic）：

- `examples/advanced/backend/system/system.go`：`Register(parent)` 以 `base.NewModule(app, "/backend/system")` 建模块并 `parent.RegisterModule` 挂到 backend 模块下（前缀恰好多一段由 `ErrInvalidSubmodulePrefix` 强制）；不注册任何显式路由、不加自己的守卫。
- `examples/advanced/backend/system/user.go`：`UserController` 经 `mod.RegisterController("user", ...)` 注册；`ActionList` 用 `dataprovider.ActiveDataProvider[models.User]`（每页 10，id DESC 默认序，id/username 排序白名单）+ `view.LinkPager`；控制器 `Middleware()` 设深色 Layout（约定路由不经过 ModuleRoutes，Layout 只在显式挂载时绑定）。
- `examples/advanced/backend/views/`：侧栏新增 System 入口（地址由 `url.To("backend/system/user/list")` 约定兜底拼装，navActive 路径高亮）；`systemUserPage` 列表模板 + 分页条深色样式；`views_templ.go` 已用 templ v0.3.1100 重新生成（自 examples/advanced 目录运行，FileName 路径与既有提交一致）。
- `examples/advanced/backend/backend.go`：`Register` 内一行装配 `system.Register(mod)`；controllers 包文档注释同步指向 system 子包。

**测试**（`examples/advanced/backend/system_test.go`，复用既有三态守卫测试装配）：

- `TestSystemSubmoduleGuardInheritsBackendChain`：guest 访问 `/backend/` 与 `/backend/system/user/list` 均 302→`/login`；demo（无 admin 角色）403 且不渲染列表；admin 200，深色 frame（"backend partition"）、System 入口带 `class="active"`。
- `TestSystemUserListingPagesThroughConventionRoute`：断言 `h.URLs().Has("backend/system/user/list") == false`（无注册路由，纯约定）；自造 9 个用户共 11 行 → 2 页；page 1 含 bulk01/bulk09/demo、不含 admin、有 `?page=2` 链接与 "11 accounts · page 1 of 2"；page 2 仅含 admin（结果集随翻页参数改变）。

命令与输出摘要：

- `go build ./...` → 通过
- `go test ./...` → 全部 ok（无失败行）
- `gofmt -l .` → 无输出
- 真机冒烟（examples/advanced：migrate up → seed → 9× user/create → serve :8090）：guest `/backend/system/user/list` 302→`/login`；admin 表单登录后 page 1 = 200（11 accounts · page 1 of 2，bulk/demo 行，System 入口 active），`?page=2` = 200（仅 admin 行）。

**演示路径**：`go run . migrate up && go run . seed`，再执行 9 次 `go run . user/create --username bulkNN --password bulk-pass-123`，`go run . serve` 后浏览器登录 admin/admin123 → 侧栏 System → `/backend/system/user/list` 翻页（不造数时仅 2 个账号、单页，pager 显示当前页为 active）。
