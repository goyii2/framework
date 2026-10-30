# 03: backend 模块、RBAC 守卫与 seed

Status: ready-for-agent
Blocked by: 02（用户登录与 user/create 命令）

**What to build:** backend 分区成形为 `/backend` 模块：装配函数、深色侧边栏 Layout（ModuleRoutes.Layout）、模块级 AccessControl 要求 `admin` 角色（rbac DbManager 及其迁移）、dashboard 页；`seed` 命令创建 admin 用户并授予角色。演示 Yii2 高级模板的 backend 应用形态：未登录访问被跳转登录，无角色用户被拒，admin 可见深色 dashboard。

- [ ] `/backend` 模块经 backend 分区装配函数注册（含 Layout 与模块中间件），main.go 不感知细节
- [ ] rbac DbManager 迁移入 migrate 列表；seed 创建 admin 用户并授予 admin 角色，且可重复执行不报错（幂等或明确语义一致）
- [ ] 未登录访问 `/backend` 下任意路径 → 跳转登录页；已登录但无 admin 角色 → 403；admin → dashboard 200
- [ ] 深色后台 Layout 生效（与前台亮色布局可区分）
- [ ] 守卫三态（302/403/200）httptest 测试通过
- [ ] `go build ./...` 与 `go test ./...` 通过，gofmt 干净

## Comments
