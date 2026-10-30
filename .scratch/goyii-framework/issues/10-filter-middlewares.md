# 10: Filter 族横切中间件

Status: resolved

**What to build:** 六件横切中间件（VerbFilter/Cors/ContentNegotiator/Csrf/RateLimiter/HttpCache，即 Yii2 Filter 族的 Go 落地）+ Controller 级 `Middleware()` 挂接机制 + 应用级全局链。Csrf 对齐 Yii2 默认：会话表单开启、纯 API 应用可全局关闭。

**Blocked by:** 05

- [x] 六件中间件各自单测 + httptest 行为验证（如 VerbFilter 返回 405、Cors 预检正确响应）
- [x] Controller 声明的中间件仅作用于该 Controller 的 action
- [x] 全局链先于 Controller 级执行，顺序确定且可预测
- [x] Csrf 默认开启可配置关闭；token 校验失败拒绝请求

## Comments

**落地形态**（main `7d5ee09`，分支 feat/filter-middlewares 已 ff 合入并删除）：

- **中间件基座**（`web/middleware.go`）：`type Middleware func(http.Handler) http.Handler`（标准 net/http 签名）+ `Chain` 组合器。`Handler.Use(...)` 追加应用级全局链（注册序执行、先于路由，`Handler.pipe` 包裹 mux）；中间件经 `web.FromContext(r.Context())` 访问请求 Context（全局链与 Controller 级均可达，测试断言 App 同一实例）。拒绝路径统一走新增导出的 `Context.SetError(err)`——记录后不再调用 next，链解开后由统一错误渲染器出 negotiated 响应（404/405/403... 同一 body 形态）；无 Context 的独立使用退化为 plain status line。
- **Controller 级挂接**：可选接口 `interface{ Middleware() []web.Middleware }`；`Handler.Ctrl(ctrl)` 返回 ControllerRoutes（Get/Post/Put/Delete/Patch/Head/Options/Handle）挂接显式路由并包裹 action；约定回退解析（resolveConvention）返回 controller，同样包裹。`withMiddleware` 把 action 适配为最内层 http.Handler，中间件可短路（写响应或 SetError）而 action 不执行。全局链先于 Controller 级：顺序确定性由结构保证（pipe 外层 + action 包装内层）。
- **路由层 405+Allow 前置（05 号票遗留，VerbFilter 语义必要前置）**：catch-all "/" 使 ServeMux 自身永不 405，故 `Handler` 记录显式绑定的 path -> verbs（`verbBindings`，按原始 path pattern 聚合，方法前缀剥离；无前缀 = anyVerb）。`serveConvention` 在约定解析前用自研 `matchPathPattern`（覆盖 `{name}` / `{name:regexp}` / `{name...}` / `{$}` 语法，复刻本 handler 接受的 ServeMux 通配语义、无重定向语义；host pattern 不参与）匹配请求 path，命中且动词失配 -> `Allow` 头（排序去重）+ 405（新错误构造器 `MethodNotAllowed`）。**通配约束失配仍走约定回退**（PUT /post/abc 对 `GET /post/{id:[0-9]+}` -> 404，显式路由本就不适用）。API 分叉理由：复用 ServeMux 做 path 探测需要第二个 mux 且无法枚举 Allow 并集（Go 内部 `matchingMethods` 不导出），自研匹配器 40 行覆盖本 handler 接受的 pattern 子集。旧测试 `TestMethodHelpersAndMethodMismatch` 期望从「PUT -> 404 via 约定回退」改为「PUT -> 405 + Allow: GET, POST」（语义变更点，属本票目标）。
- **VerbFilter**：`VerbFilter{Allowed []string, Rules []VerbRule}`。Allowed 作用于包裹的全部请求；Rules 按 path 精确匹配分作用域，无 rule 的 path 直通。严格白名单（HEAD 需显式列出，不随 GET 隐含）；失配 405 + Allow（排序去重大写）。
- **Cors**：`Cors{Origins, Methods, Headers, ExposeHeaders, Credentials, MaxAge}`。预检（OPTIONS + A-C-Request-Method）：策略校验前置，拒绝（origin/动词/请求头任一不符）-> 403 且不带任何 access-control 头（API 分叉：拒绝的预检不泄露策略）；通过 -> 204 短路（action 不跑）+ Allow-Origin/-Methods/-Headers/-Max-Age + Vary。普通跨域请求：allowed origin -> 带简单头放行；disallowed origin -> 照常服务但零 CORS 头（API 分叉：非预检是普通请求，浏览器侧同源默认已挡住读取，服务端不二次拒绝，对齐主流 CORS 库行为）；无 Origin 头 -> 原样直通。Credentials 开启时 origin 回显不用 `*`（spec 禁止）。Methods/Headers 为空 = 不限（预检回显请求值）。
- **ContentNegotiator**：`ContentNegotiator{ResponseFormats []Format, RequestFormats []Format}`，`Format{Type, Format}`。API 分叉：用**有序切片**而非 map——通配 Accept（`*/*`、`text/*`）命中时取首个 offered，map 迭代序不确定无法定义优先级；q 值 + 位置稳定排序。响应侧协商结果经新增 `Context.ResponseFormat()` 发布并驱动统一错误渲染（"html" -> HTML 错误页，其余 -> JSON）；无匹配 Accept -> 406（新构造器 `NotAcceptable`）。请求侧：RequestFormats 非空时 POST/PUT/PATCH/DELETE 的 Content-Type 不在表内 -> 415（`UnsupportedMediaType`）；无 Content-Type（无 body 调用）放行。
- **Csrf（双提交 cookie 模式，不依赖 session）**：cookie `_csrf`（HttpOnly、SameSite=Lax、Secure 可配）/ header `X-Csrf-Token` / 表单字段 `_csrf` 三通道，查询串 token 明确不认（Referer 泄露面）；常量时间比较；失败 400（"unable to verify your data submission"，Yii2 措辞）。安全请求无 cookie 时签发 32 字节 CSPRNG token（base64url）并 Set-Cookie，action 经新增 `Context.CsrfToken()` 读取供表单渲染（同请求 cookie 与 action token 一致有断言）。**默认开启**：`NewHandler` 装入全局链最前，`config.Web.DisableCsrf`（yaml `disableCsrf`）全局关闭——纯 JSON API 的用法见 validators/web_test.go 的端到端测试（本票将其改为显式 DisableCsrf，正是该开关的目标场景）。**API 分叉记录：per-controller 无法豁免全局 Csrf**——全局链先于路由执行，路由未发生即无法定位 controller；Yii2 的按 controller `enableCsrfValidation=false` 语义在「验证挂在全局链」架构下不可达，如实记录于 Csrf 文档注释（`Disabled` 开关仅用于手工挂接场景），如后续需要 per-controller 豁免须把默认验证下沉到挂接层（留 11 号票 session 化时一并评估）。
- **RateLimiter**：`RateLimiter{Limit, Window, Key, Store}`，默认 60/min、key=RemoteAddr host。存储接口化 `RateStore{Allow(key, limit, window, now) (bool, retryAfter)}`（now 入参便于假时钟测试与未来 Redis 共享存储扩展）；默认实现 `SlidingWindowStore`（mutex + 每 key 时间戳环、按窗口剪枝）。超限 429（新构造器 `TooManyRequests`）+ `Retry-After`（向上取整秒）。
- **HttpCache**：`HttpCache{Etag, LastModified func(*http.Request) ..., CacheControl}`——校验器按请求派生（支持 per-path 内容版本）；ETag 自动加引号、`W/` weak 前缀归一比较；**If-None-Match 优先于 If-Modified-Since**（RFC 9110：INM 存在且失配即重发，测试覆盖「etag 失配 + IMS 满足 -> 200」）；304 响应带校验器与 Cache-Control、body 为空、action 不执行；不可解析的条件头忽略不采信。

**验收证据**（合并后 main `7d5ee09` 上，CGO 经 scoop mingw gcc 启用）：

- `go build ./...` / `go vet ./...` / `gofmt -l .` 全部无输出；`go test ./...` 全仓 17 包全绿；`CGO_ENABLED=1 go test -race -count=1 ./web/... ./config/... ./validators/...` 三包 ok 无 data race（web 包 74 项顶层测试，本票新增 filter/中间件相关 40 项）。
- **六件各自单测 + httptest 行为**：VerbFilter 5 项（`TestVerbFilterRejectsUnlistedVerb405WithAllow` 断言 405 + `Allow: "GET, POST"` 排序去重；Allowed/Rules/Controller 级/standalone 四形态）；Cors 9 项（预检 204 头全集、disallowed origin/verb/header 各自 403 且无 Allow-Origin、简单请求带/不带头、credentials 回显 origin 不用 `*`、无 Origin 直通、空 Methods/Headers 回显请求值）；ContentNegotiator 6 项（7 种 Accept 形态协商断言、406、错误渲染随协商格式切换 JSON/HTML、通配取声明序首选、415、无 Content-Type 放行）；Csrf 9 项（cookie 签发与 `Context.CsrfToken` 一致性、无 token/仅 cookie/失配三方均 400、header 与表单字段双通道 201、查询串 token 400、`Web.DisableCsrf` 与 `Disabled` 两级关闭）；RateLimiter 4 项 + Store 3 项（限额内 200 超限 429 + Retry-After、按 key 隔离、假时钟下窗口滑动/过期精确断言、200 goroutine 并发恰 admit limit 个——-race 下通过）；HttpCache 6 项（首请求校验器三头、INM 命中 304 且 action 未跑 body 为空、weak tag 命中、INM 优先级、IMS 命中/过期/不可解析、按请求派生 etag）。
- **Controller 级仅作用本 Controller**：`TestControllerMiddlewareOnlyAffectsItsOwnActions`——声明 Middleware() 的 controller 其显式挂接（Ctrl）与约定回退（/tagged/echo）路径响应均带标记头，普通 controller 显式/约定路径不带，不可路由路径不触达任何 controller 中间件。
- **全局链先于 Controller 级、顺序确定**：`TestGlobalMiddlewareRunsInRegistrationOrder`（全局链按注册序 trace）、`TestControllerMiddlewareRunsAfterGlobalChain`（[global-a, global-b, controller] 精确断言）、`TestControllerMiddlewareRejectsWithUnifiedError`（Controller 中间件经 `Context.SetError` 拒绝 -> 403 统一错误体 + action 未执行）。
- **Csrf 默认开/可关/失配拒绝**：`TestCsrfRejectsUnsafeRequestWithoutToken`（默认配置 POST 无 token -> 400）、`TestCsrfDisabledGloballyByConfiguration`（`Web.DisableCsrf: true` -> 201）；默认开启使既有 POST 用例（router_test 约定路由任意动词、validators 端到端 API）改为携带意图的显式配置，变更点已在上述记录。
- **405+Allow 路由前置**：`TestMethodHelpersAndMethodMismatch`（更新后断言 PUT -> 405 + `Allow: "GET, POST"`）、`TestExplicitRouteVerbMismatch405`（通配路径 405、约束失配仍 404、无方法绑定任意动词直通）。

**遗留**：Csrf 迁移到 session 绑定 token（11 号票，wire 形态不变）；per-controller Csrf 豁免若实际需要，随 session 化一并把默认验证下沉挂接层评估；RateLimiter 的 Redis 共享存储（RateStore 接口已留）；本机 golangci-lint 对全部包 panic 的环境问题沿袭 09 号票记录（go vet + gofmt 已覆盖等价检查，非本票引入）。
