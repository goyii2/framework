# 06: console 骨架（cobra）

Status: resolved

**What to build:** cobra 命令行骨架：`goyii` 根命令、`goyii serve` 启动 HTTP 服务（优雅停机）、用户命令注册位；命令与应用装配复用同一份 config，CLI 与 Web 共享同一 Application。

**Blocked by:** 04

- [x] `goyii serve` 启动并处理请求，收到退出信号优雅停机（在途请求完成）
- [x] 用户自定义命令经 Application 注册为子命令
- [x] `--help` 输出全部内建子命令
- [x] serve 与 Web 应用共用组件装配（同一 config 来源）

## Comments

**落地形态**（main `e82f5e2`）：

- **`console.Console`**：`console.New(app *base.Application, opts ...Option)` 构造 cobra 根命令 `goyii`（SilenceUsage=true，运行期错误不糊 usage）。Console 与 `web.NewHandler` 持有**同一个** `*base.Application`——CLI 与 Web 一份 config、一套组件装配。`App()` 取应用；`Command()` 取根命令（挂持久 flag/命令组用）；`AddCommand(cmds ...*cobra.Command)` 是用户命令注册位（链式返回 Console）。
- **`serve` 子命令**（console/serve.go）：监听地址取 `app.Config().Web.Listen`（同一 config 来源，测试断言传入 listener 的地址即配置值）；HTTP handler 由 `console.WithHandler(h)` 注入（典型 `web.NewHandler(app)`，未配置时报清晰的 setup 错误）。启动经 `net.Listen` 后 `srv.Serve`，经 `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` 等待停止信号；停止后 `srv.Shutdown`（超时 `WithShutdownTimeout` 可配，默认 `DefaultShutdownTimeout=10s`）等在途请求完成后返回——超时仍有请求在途则作为命令 error 透出。启动/停机日志走 `app.Log()`（共享组件）。
- **上下文贯通**：`Execute()/ExecuteContext(ctx)` 把 `app.WithContext(ctx)` 传给 cobra，任何子命令内 `base.FromContext(cmd.Context())` 拿到的就是 Console 的应用实例（非包级默认），用户命令与 Web 侧同一套组件（Config/Log/Event/…）。
- **`--help`**：cobra 默认 help 列出全部子命令（内建 `serve` + 用户命令 + cobra 自带 `completion`/`help`）；测试断言输出含 `serve` 及其 Short、用户命令名与 "Available Commands"。

**验收证据**（合并后 main 上，CGO 经 scoop mingw gcc 启用）：

- `go build ./...` / `go vet ./...` 无输出；`gofmt -l .` 无文件。
- `CGO_ENABLED=1 go test -race -count=1 ./...` -> 78/78 PASS（5 个含测试包，无 data race）；`console` 包 6 测全绿。
- **serve 启动/处理/优雅停机**：`TestServeServesRequestsThroughSharedApplication`——真 TCP 监听（默认 `net.Listen` 路径，`:0` 临时端口），`/site/hello` 经共享 Application 的 controller 注册表约定路由解析并应答 200，停机后 serve 返回 nil 且记 "server stopped"；`TestServeGracefulShutdownWaitsForInFlightRequest`——停止触发先于在途请求放行（顺序化验证），请求仍完整应答、serve 零错误退出；`TestServeShutdownTimeoutFailsTheCommand`——在途请求不放行 + 50ms 停机超时 -> 命令报 graceful shutdown 错误。信号注册为 SIGINT/SIGTERM（`signal.NotifyContext`），测试经 ctx 取消驱动与信号**完全相同**的停机代码路径（Windows 测试进程无法自投 SIGTERM，此为等价覆盖）。
- **用户命令注册**：`TestUserCommandsRegisterBelowRootAndReachApplication`——`AddCommand` 后 `goyii greet` 执行，命令内 `base.FromContext(cmd.Context())` 与构造时的 app 指针相等，Config 取到同一份值。
- **--help 全量**：`TestHelpListsBuiltInAndUserCommands`——输出含 `goyii`、内建 `serve`（含 Short）、用户命令、`Available Commands`。
- **共用装配**：上述 serve 测试中 handler、controller 注册表、监听地址、日志组件全部来自同一个 `base.Application`（recordedLog 经 `app.SetLog` 注入即收到 serve 的启动/停机日志）。

**遗留**：本机 golangci-lint v2.13.1（go1.26 构建）与 Go 1.27 stdlib 不兼容（typecheck panic），本票以 `go vet` + `gofmt` 干净替代验证；migrate 子命令（PRD console 条目另一半）在 07 号票落地，届时经同一 `AddCommand` 注册位挂入。
