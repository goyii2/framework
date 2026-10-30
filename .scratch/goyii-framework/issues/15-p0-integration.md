# 15: P0 集成验证（examples/basic）

Status: resolved

**What to build:** examples/basic 以 SQLite 零依赖跑通 P0 全链路：配置装配 -> migrate 建表 -> seed -> 登录 -> 带验证的 CRUD 端点（错误处理/中间件/日志/缓存全部参与）；docs/ 起步（安装与快速上手）。

**Blocked by:** 05, 06, 07, 08, 09, 10, 11, 12, 13, 14

- [x] 新环境按 docs/ 快速上手从零跑起 examples/basic，无需外部服务
- [x] 演示链路覆盖：迁移、seed、登录、CRUD（含验证失败与错误格式）、中间件生效、日志与缓存可见
- [x] docs/ 安装与快速上手文档完成
- [x] 一键脚本跑通全部 P0 测试

## Comments

**落地形态**（main `9e4eed7`，分支 feat/p0-integration 已 ff 合入并删除）：

- **examples/basic（同 module 下的 main 包目录）**：`go run ./examples/basic`（或 cd 后 `go run .`）零外部服务跑通全链路。**分叉记录——示例放同一 Go module 而非独立 go.mod**：① 示例随框架每次 CI `go build ./...`/`go vet ./...` 编译，框架 API 演进时示例永不漂移成"编译不过的文档"；② clone 后无需 replace/额外 module 处理；③ 代价是示例被 `go test ./...` 纳入（无测试文件，仅编译检查）——可接受。
- **演示链路**（examples/basic 七个文件）：
  - `config/app.yaml`：值轨示范——SQLite DSN、file 缓存/会话、双 Target 日志（console info 级排除 db；file debug 级收 `db*`+`cache` 分类，慢查询阈值 1ms 使普通查询即以 WARN 落盘）、全部字段可 env 覆盖（DB_DSN/WEB_LISTEN/...）。
  - `migrations/`：两条手写迁移（user/post 表，migrate.Columns 建表 + DefaultValue）+ `All()` 清单注入 `commands.New(app, migrations.All())`。
  - `models.go`：`User` 实现 web.Identity + insert 场景规则（用户名必填/Min(3)/TestFunc 查库唯一性）；`Post` 双场景规则（insert 全量、update 复检 title/status）+ `Author *User` 关联；密码 bcrypt（`golang.org/x/crypto` 随此由 indirect 转 direct）。
  - `controllers.go`：`SiteController`（/site/index 约定回退 + VerbFilter 规则演示、/site/csrf 发 token、/login（zog 内联校验 + 401 不区分用户名/密码错）、/logout、/me）；`PostController`（AccessControl 规则：读公开 `?`/`@`、写需 `@`、删需 admin、默认拒；列表 `cache.GetOrSet` + `ValueDependency(max id)`，X-Cache 头 + cache 分类日志使命中可见；view 带 `With("Author")` 预加载；create/update 走保存链验证（422 字段映射表）+ `url.To` 命名路由 Location）。
  - `main.go`：装配——config.Load -> base.New -> SetDefault -> web.NewHandler（IdentityFinder 接 user 表、ContentNegotiator 全局中间件：错误协商 + 415）-> console（serve + migrate 八命令 + seed）。
  - `seed.go`：用户命令注册位示范（幂等 seed admin/demo + 示例文章）。
- **12 号遗留 Logger.Close 接住**：`console.ExecuteContext` 在根命令返回后统一 `app.Logger().Close()`——覆盖 serve（优雅停机后）、migrate、seed 与一切用户命令的进程关停；Close 失败仅报 stderr 不改命令结果；SetLog 装入的外部最小实现经 Wrap 无 targets 不受影响。回归测试 `console.TestExecuteClosesLogTargets`（file target 在 Execute 后处于 closed 状态）。
- **docs/**：`docs/安装.md`（环境要求/Windows -race PATH 说明/目录速览 mermaid + 包对应表）、`docs/快速上手.md`（clone -> migrate -> seed -> serve -> curl 全链路 walkthrough、请求管线 mermaid、可观察性 grep、env 覆盖切换数据库）。遵守仓库文档规范：图表全 mermaid、无 ASCII 字符画。
- **scripts/p0-check.sh**（Git Bash/Linux/macOS）：gofmt -> go vet -> `CGO_ENABLED=1 go test -race -count=1 ./...`（无 gcc 时探测 scoop mingw / mingw64 / msys64 常见位）-> 编译示例 -> 冒烟：重置 runtime -> migrate -> seed -> serve（临时端口，env 覆盖 WEB_LISTEN）-> curl 断言（cookie jar 维护会话与 _csrf 双提交配对）-> kill 后断言 runtime/app.log 含 `[db]`/`slow query`/`[cache]` -> 成功清理、失败保留现场。31 项断言。

**验收证据**（合并后 main `9e4eed7` 上，CGO 经 scoop mingw gcc 启用）：

- `bash scripts/p0-check.sh` -> **31 passed, 0 failed, P0 checks green**：gofmt 无文件、go vet 无输出、`go test -race -count=1 ./...` 14 个含测试包全 ok（无 data race）、示例编译通过、冒烟 31 项断言全过——覆盖：约定路由 200；CSRF 无 token 400 / token 配对正常写；ContentNegotiator 415；VerbFilter 405+`Allow: GET, HEAD`；AccessControl guest 写 403 / demo 删 403 / admin 删 204；错密码 401 / 登录 200 / me 绑定身份 / 登出 204 后 guest；create 验证失败 422（`{"title":[...]}`）/ 成功 201+`Location: /post/2`（命名路由反向生成）/ 作者预加载；列表依赖失效后 MISS、再读 HIT（X-Cache 头）；update 验证失败 422 / 成功 200；404 JSON 与 Accept:text/html 的 HTML 协商；日志含 db 记录、慢查询 WARN、cache 命中记录。
- 手动冒烟（8090 端口，与脚本同覆盖面）逐条核对过响应体与头；`runtime/`（数据库/日志/会话/缓存）入 .gitignore。
- 流程：claimed 单独提交 `9a92cb2` -> `feat/p0-integration` 上 `9e4eed7` -> main `--ff-only` 合并并删除任务分支。

**集成观察（非缺陷，记档备查）**：

1. **全局 CSRF 先于 controller 级 AccessControl**：未登录 POST 若无 `_csrf` cookie 得 400（CSRF 层）而非 403（授权层）——中间件分层（全局链先于路由）的正确行为；冒烟对 guest-403 的断言带 csrf cookie 走通授权层。
2. `web.Handler` 无 `App()` 访问器：示例 registerRoutes 需额外传 app 指针（等价信息 handler 已持有）。人体工学缝，未动框架。
3. `cache.GetOrSet` 不回报命中/未命中：示例用 build 闭包置 flag 推断（build 仅在 miss/失效时运行）；如后续多应用需要，可考虑返回 hit 标记（接口变更，未做）。
4. 集成全程未发现需修复的框架缺陷；唯一框架改动是 12 号预留的 Logger.Close 统一调用位（见上）。

**遗留**：模板视图未含（PRD 归 16 号 P1 templ 票，examples 届时加页面）；RBAC DbManager（17 号）落地后示例的 AccessControl 角色来源可切 AuthManager；debug 面板（P2）可把 X-Cache/慢查询观测升级为面板。
