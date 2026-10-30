# 02: 用户登录与 user/create 命令

Status: resolved
Blocked by: 01（骨架与前台首页）

**What to build:** common 分区的 User 模型（ActiveRecord + 验证 + identity 查找）落地，`user/create` 自定义命令（对应 Yii2 高级模板同名命令）创建用户，前台登录/登出表单与会话状态。演示：CLI 建用户 → 前台表单登录 → 导航显示已登录、可登出；错误凭据回显错误。

- [x] `go run . user/create`（参数形态自定，如 --username/--email/--password）创建用户成功并有输出确认
- [x] common 的 identity 查找接入 handler 的 IdentityFinder；登录基于 User 模型验证
- [x] 前台登录表单：凭据错误回显错误不落库会话；正确凭据登录后导航显示已登录，登出后回到匿名态
- [x] 登录/登出流程 httptest 测试通过（错误凭据回显 + 正确凭据登录态 + 登出）
- [x] `go build ./...` 与 `go test ./...` 通过，gofmt 干净

## Comments

### 验收证据（2026-10-31）

参数形态取 `--username/--password/--role`（role 默认 user；工单明示参数形态自定，user 表自 01 起无 email 列）。

命令实测（examples/advanced 下）：

- `go run . user/create --username demo --password demo123` → `created user demo (id 1, role user)`
- `go run . user/create --username demo --password other` → 退出码 1，`create user demo: validators: insert scenario rejected: username: username is already taken;`
- `go run . user/create --username admin --password admin123 --role admin` → `created user admin (id 2, role admin)`

serve 后 curl 实测（cookie jar 回放 _csrf 双提交）：

- GET `/` 匿名态：导航只有 `Login` 链接
- POST `/login`（demo/wrong）→ 401，页面含 `Wrong username or password.` 且用户名回填 `value="demo"`；随后 GET `/` 仍匿名（失败登录未落库会话）
- POST `/login`（demo/demo123）→ 302 → `/`；GET `/` 导航含 `<span>demo</span>` 与 `Logout` 按钮、正文 `Signed in as`
- POST `/logout` → 302 → `/`；GET `/` 回到 `Login` 链接

测试：`TestLoginLogoutFlow`、`TestLoginBlankFieldsAnswerPerFieldErrors`（frontend）、`TestUserCreateCommandCreatesAccount`、`TestUserCreateCommandRefusesDuplicateUsername`（commands）全过；`go test ./...` 全绿、`go build ./...` 通过、gofmt 无输出。测试装配走真实迁移（migrations.All() + Migrator）与生产同形管线（CSRF 开、IdentityFinder 接 models.FindIdentity）。

关键文件：

- `examples/advanced/common/models/user.go` — User 模型（验证 + bcrypt + web.Identity）与 FindIdentity
- `examples/advanced/commands/user.go` — user/create 命令（cobra，标志形态，输出确认）
- `examples/advanced/frontend/controllers/site.go` — 登录表单/提交/登出（401 回显、302 回首页）
- `examples/advanced/frontend/views/{views.go,views.templ,views_templ.go}` — 会话区随登录态翻转、登录页、首页问候
- `examples/advanced/frontend/frontend_test.go`、`examples/advanced/commands/user_test.go` — httptest 流程与命令测试
- `examples/advanced/main.go`、`examples/advanced/config/{config.go,app.yaml}` — session 组件装配、IdentityFinder、命令注册
