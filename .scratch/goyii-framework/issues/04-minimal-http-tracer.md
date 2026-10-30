# 04: 最小 HTTP 链路（tracer bullet）

Status: resolved

**What to build:** 第一条端到端链路：Application 挂载 http.Handler，开发者以 `ActionXxx(ctx *web.Context) error` 形态编写 Controller（默认约定 `post/view` -> `PostController.ActionView`），action 返回 JSON 或重定向；返回的 error 由统一错误处理器按 Accept 协商渲染 JSON/HTML（web.NotFound 等构造 HTTP 异常）；03 的生命周期事件在此真实接线。

**Blocked by:** 02, 03

- [x] 示例 Controller action 经 curl 返回 JSON
- [x] web.NotFound 等异常返回对应状态码，格式随 Accept 协商
- [x] web.Context 提供 Request/Response 基础访问与 JSON/Redirect
- [x] BeforeRequest/AfterRequest 在真实请求中触发
- [x] httptest 覆盖正常与异常路径

## Comments

**落地形态**（main `cf1e0b4`）：

- `web.Handler`（`web.NewHandler(app)`）：应用的 HTTP 入口，实现 `http.Handler`；`Handle(pattern, action)` 用 ServeMux 语法挂载 action。每请求管线：创建携带 Application 的 `web.Context` -> `beforeRequest` -> 分发 -> 统一错误渲染（Accept 协商 JSON/HTML，panic 兜底 500）-> `afterRequest`（defer 保证 panic 路径也触发）。
- `web.Context`：`Request()/Response()/App()/Context()/StatusCode()` + `JSON(v)` / `Redirect(status, url)`；响应 writer 记录状态码且部分写出后不再覆盖。
- `web.Controller` 嵌入标记 + `web.Action = func(ctx *Context) error`；示例 Controller `postController`（`web/handler_test.go`）以 `ActionXxx(ctx *web.Context) error` 形态编写。
- 错误：`HTTPError` 接口（Status/Message）+ `BadRequest/Unauthorized/Forbidden/NotFound/ServerError/NewHTTPError` 构造器；普通 error -> 500（非 debug 隐藏内部文本，`App.Debug` 开启时透出）；HTML 输出经 `html.EscapeString` 转义。
- Accept 协商规则：按 q 值排序的媒体范围逐一匹配，json/+json -> JSON、text/html 与 XHTML -> HTML、通配符不表态、缺省（无 Accept 或仅 `*/*`，即 curl 默认）-> JSON。

**验收证据**：

- `go build ./... && go vet ./...` 通过；`go test ./...` 48 passed（13 包），其中 `PATH="/c/Users/wutao/scoop/apps/mingw/current/bin:$PATH" CGO_ENABLED=1 go test -race -count=1 ./...` 同样 48 passed，无 data race。
- `web` 包 14 个测试：`TestJSONActionReturnsJSON`、`TestRedirectAction`（302 + Location）、`TestNotFoundRendersJSONByDefault`、`TestForbiddenRendersHTMLWhenPreferred`（浏览器 Accept -> HTML）、`TestExplicitJSONAcceptRendersJSONError`、`TestPlainErrorIs500WithoutInternals` / `TestPlainErrorRevealsTextInDebugMode`、`TestPanicRecoveredAs500AndEventsStillFire`、`TestErrorAfterPartialWriteKeepsFirstResponse`、`TestRequestEventsFireInOrder`（before/200/before/404）、`TestRealServerServesJSONOverTCP`（httptest.NewServer 真 TCP 请求 + 事件触发）、`TestActionSeesApplicationAndWebContext`；另有 `error_test.go` 构造器与协商表驱动测试。
- curl 实测（临时 demo 程序，127.0.0.1:18080，事后删除）：
  - `curl -i /site/hello`（默认 Accept `*/*`）-> `200` `application/json`，`{"message":"hello from goyii"}`
  - `curl -i -H "Accept: application/json" /site/gone` -> `404`，`{"name":"Not Found","message":"page gone","status":404}`
  - `curl -i -H "Accept: text/html" /site/gone` -> `404` `text/html`，`<h1>404 Not Found</h1>` 页面
  - 服务端日志每请求打印 `event: beforeRequest` / `event: afterRequest`

**遗留**：未知路径回落到 ServeMux 自带 404 纯文本（路由完整化属 05）；Session/User/Flash、Render 未做（后续票）；HTML 错误页为内置最小页，模板覆盖机制未做。
