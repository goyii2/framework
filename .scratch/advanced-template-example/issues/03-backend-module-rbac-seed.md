# 03: backend 模块、RBAC 守卫与 seed

Status: resolved
Blocked by: 02（用户登录与 user/create 命令）

**What to build:** backend 分区成形为 `/backend` 模块：装配函数、深色侧边栏 Layout（ModuleRoutes.Layout）、模块级 AccessControl 要求 `admin` 角色（rbac DbManager 及其迁移）、dashboard 页；`seed` 命令创建 admin 用户并授予角色。演示 Yii2 高级模板的 backend 应用形态：未登录访问被跳转登录，无角色用户被拒，admin 可见深色 dashboard。

- [x] `/backend` 模块经 backend 分区装配函数注册（含 Layout 与模块中间件），main.go 不感知细节
- [x] rbac DbManager 迁移入 migrate 列表；seed 创建 admin 用户并授予 admin 角色，且可重复执行不报错（幂等或明确语义一致）
- [x] 未登录访问 `/backend` 下任意路径 → 跳转登录页；已登录但无 admin 角色 → 403；admin → dashboard 200
- [x] 深色后台 Layout 生效（与前台亮色布局可区分）
- [x] 守卫三态（302/403/200）httptest 测试通过
- [x] `go build ./...` 与 `go test ./...` 通过，gofmt 干净

## Comments

### 验收证据（2026-10-31）

- 落地形态：`examples/advanced/backend/`（`Register(h, app, auth)` 装配模块 + 守卫链 + 深色 Layout + dashboard 控制器/视图），`commands/seed.go`，`common/migrations.All()` 追加 `rbac.NewMigration()`，main.go 接线 `rbac.NewDbManager` + `handler.SetAuthManager` + `backend.Register` + seed 命令。
- 守卫三态 httptest（`backend/backend_test.go` TestBackendGuardThreeStates，红-绿节奏：对无守卫桩先红后绿）：
  - 未登录 GET `/backend/` 与 `/backend/dashboard/index`（显式+约定路由）→ 302，Location `/login`；
  - demo 登录（无角色）GET `/backend/` → 403；
  - admin 登录 GET `/backend/` → 200，正文含 `backend partition`（深色框架）且不含 `frontend partition`。
- 真实二进制复核（runtime sqlite 重置后）：
  - `go run . migrate up` → applied `m20260101_000001_create_user_table`、`m20260101_000003_create_auth_tables`；
  - `go run . seed` #1 → `created user admin (id 1)` / `created user demo (id 2)` / `rbac: admin role granted to admin`；#2 → `user admin already exists` ×2 + 同上授权行，无报错（幂等；`seed_test.go` 钉住 user=2、auth_assignment=1 不增长）；
  - curl：guest `GET /backend` → 302→`/backend/`（裸前缀规范化跳转）；`GET /backend/` → 302→`/login`；demo 登录后 → 403；admin 登录后 → 200，`<title>Dashboard</title>`。
- 测试：`go test ./...` → 1461 passed / 50 packages；`go build ./...` 通过；`gofmt -l` 干净；templ 生成物已重新生成（与仓库既有 `views.templ` 相对路径约定一致）。

### role 列处置说明（共识 #3）

选择**移除**而非降级：user 表 `role` 列、`user/create --role` 标志、`User.Role` 字段与 `GetRoles()` 的角色来源全部删除（迁移、命令、既有测试随之调整，均在本票范围）。理由：RBAC 的 `auth_assignment` 是唯一角色真源，账户行上并存一份角色列必然产生第二真源、误导授权判定；Yii2 advanced 的 user 表本就无 role 列。`User.GetRoles()` 现固定返回 nil——授权只经 auth manager 的 `CheckAccess`。授权链：`rbac.NewDbManager` → `handler.SetAuthManager` → `AccessControl{Auth: auth, Roles: ["admin"]}`，与角色列无任何耦合。

### 备注

- 裸前缀 `/backend`（无尾斜杠）无法经模块注册器挂载（模块路由一律挂在前缀之下），由 backend 分区在根作用域挂一条 302 规范化跳转到 `/backend/`，守卫在下一跳生效。
- 403 错误页沿用 handler 级注册表中的 frontend "error" 视图（浅色框）——单一视图注册表的取舍，后续切片如需可在文档注明。
