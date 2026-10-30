# 01: 骨架与前台首页

Status: resolved
Blocked by: None（可立即开工）

**What to build:** `examples/advanced` 端到端可运行的骨架：项目配置（app.yaml + web-dev overlay）经代码装配为 Application，frontend 分区以根作用域提供首页，console 提供 migrate/serve，user 表迁移先行落地 common 分区。演示：`go run . migrate up && go run . serve` 后访问 `/` 返回带亮色 Layout 的前台首页。main.go 只做组装，frontend 分区经自身装配函数注册。详见同目录 spec.md。

- [x] `go run . migrate up` 成功建表（含 user 表迁移，出自 common 分区）
- [x] `go run . serve` 启动，`/` 返回前台首页（亮色 Layout，导航含登录占位链接）
- [x] main.go 只做组装；frontend 分区暴露装配函数，根作用域注册（约定回退或显式路由均可）
- [x] config 分区：app.yaml + web-dev.yaml overlay 生效（dev 差异可观察，如 debug 开关）
- [x] `go build ./...` 与 `go test ./...` 通过，gofmt 干净

## Comments

**验收证据（2026-10-31，Windows + Git Bash，仓库根 E:\workspace\go\goyii\framework）**

- `go run . migrate up`（examples/advanced 下，runtime/ 清空后执行）：
  `>>> applying m20260101_000001_create_user_table` / `<<< applied m20260101_000001_create_user_table` —— user 表迁移出自 common 分区（examples/advanced/common/migrations/）。
- `go run . serve` + curl：
  - `GET /` -> 200，返回亮色 Layout 首页（白底 header、品牌 "advanced"、Home 导航、Login 占位链接），正文含 "Welcome to advanced"、"env: dev"、"debug on" 徽标；
  - `GOYII_ENV=prod go run . serve` 后同一页面仅显示 "env: prod"、无 debug 徽标 —— dev overlay（web-dev.yaml 置 app.debug=true，app.yaml 基值 false）差异可观察；
  - `GET /site/index` -> 200（根作用域约定回退，经 app.RegisterController("site", ...)）；
  - `Accept: text/html` 请求未知路径返回 404 且错误页穿前台亮色 Layout。
- `go build ./...` 通过；`go vet ./examples/advanced/...` 干净；`gofmt -l examples/advanced/` 无输出。
- `go test ./...`：1453 个测试通过（45 个包），其中新增 examples/advanced/config 4 个测试（dev overlay 生效 / 无 overlay 环境用基值 / APP_DEBUG 环境变量覆盖 / 身份与组件段落解析）。

**关键文件**

- examples/advanced/main.go —— 仅组装：LoadAs 配置 -> base.New/ConnectDB/SetDefault -> frontend.Register(handler, app) -> console(migrate/serve)；backend 与 commands 装配点以注释预留
- examples/advanced/frontend/frontend.go —— frontend 分区装配函数 Register：Layout 中间件 + 视图注册 + 显式 `/` 路由 + 根作用域控制器注册
- examples/advanced/frontend/controllers/site.go、frontend/views/{views.go,views.templ,views_templ.go} —— 前台首页与亮色 Layout（templ，生成物已提交）
- examples/advanced/config/{config.go,app.yaml,web-dev.yaml,config_test.go} —— 项目配置 shape 与 overlay
- examples/advanced/common/migrations/{m20260101_000001_create_user_table.go,register.go} —— user 表迁移与注册（All() 供后续工单追加）
- .gitignore —— 追加 examples/advanced/runtime/（与 basic 同款运行态忽略）
