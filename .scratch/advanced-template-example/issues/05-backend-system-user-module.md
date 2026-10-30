# 05: /backend/system 嵌套子模块——用户管理

Status: ready-for-agent
Blocked by: 03（backend 模块、RBAC 守卫与 seed）

**What to build:** backend 模块下嵌套 `system` 子模块（`/backend/system`）：用户管理列表控制器经模块 `RegisterController` 注册、走约定回退路由（如 `/backend/system/user/list`），DataProvider 分页，后台导航加入口。与 basic 的 `/admin/report` 显式挂载形成互补展示（约定式 vs 显式式），并验证嵌套模块继承祖先模块的守卫与中间件顺序。

- [ ] `/backend/system` 经 backend 分区装配为嵌套模块（父模块注册，前缀恰好多一段）
- [ ] 用户列表控制器经 `RegisterController` 约定路由可达，无显式 Get/Post 声明
- [ ] DataProvider 分页生效（翻页参数改变结果集），列表渲染于后台深色 Layout
- [ ] 守卫继承：未登录/无角色访问 `/backend/system/*` 与 `/backend/*` 行为一致（302/403）
- [ ] 分页路由 + 守卫继承 httptest 测试通过
- [ ] `go build ./...` 与 `go test ./...` 通过，gofmt 干净

## Comments
