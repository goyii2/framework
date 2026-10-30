# 05: 路由完整化

Status: resolved

**What to build:** 完整路由能力（ADR-0002）：基于标准库 ServeMux 的显式绑定，`{id:[0-9]+}` 参数语法、`.Name()` 可选命名、`url.To(name, url.P{...})` 反向生成（命名表 -> 约定拼装兜底）、`/{controller}/{action}` 约定回退（默认开、可关、显式路由优先）、Module 分区前缀挂载。

**Blocked by:** 04

- [x] 显式绑定路由的参数可经 web.Context 类型化取值（ParamInt 等）
- [x] 命名路由反向生成正确；未命名路由按约定拼装兜底（与 Yii2 Url::to 语义对齐）
- [x] 约定回退解析未声明的 controller/action；可配置关闭；显式路由始终优先
- [x] Module 前缀下的路由挂载与隔离验证
- [x] 路由表单测 + httptest 集成测试

## Comments

**落地形态**（main `02e93ce`）：

- **显式绑定 + 参数**：`Handler.Get/Post/Put/Delete/Patch/Head/Options(path, action)` 返回 `*Route`，`.Name("post/view")` 可选命名（可链式）；原始 `Handle("GET /x", a)` 保留。类型化取值：`Context.Param/ParamInt/ParamInt64/ParamBool`（读 `r.PathValue`，解析失败返回带参数名的 error）。
- **`{name:regexp}` 约束**：stdlib ServeMux 不接受内联参数正则（注册即 panic），故注册时改写为裸 `{name}` 通配并把正则留作每请求前置校验；值不匹配视为"该显式绑定未命中"，落回约定处理器（等价 Yii2 规则不匹配继续下探）。非法正则在注册期 panic（与 ServeMux 非法 pattern 同类）。
- **反向生成（`url` 包）**：`url.P` 专用参数类型；`url.Manager` 命名表 O(1) 查找——命中则按 pattern 填充路径参数（PathEscape 转义，剩余参数按 key 排序成确定性 query），缺路径参数报 error；未命中走约定拼装 `"/"+name + query`（与 Yii2 UrlManager 无规则命中时的 createUrl 兜底语义一致）。`url.To(name, P)` 走包级默认表（`url.SetDefault(h.URLs())` 接线，多应用场景各自隔离），失败返回 `"#"`（Yii2 cannot-create 标记）。重名注册 panic（同 ServeMux 重复 pattern）。
- **约定回退**：`NewHandler` 在 mux 挂 `/` 兜底（最不特异 pattern，显式路由恒优先；04 遗留的"未知路径纯文本 404"随之修复为 Accept 协商 404）。开启时解析 `/{controller}/{action}`：先按段边界匹配最长注册模块前缀定作用域，段数恰为 2 时查该作用域 controller 注册表（O(1)）并反射取 `ActionXxx` 方法（签名 `func(*web.Context) error`，反射返回的是未命名 func 类型，按签名断言后转换）；未知 controller/action 或段数不符 -> 404；方法名存在但签名不符 -> 500。关闭途径：`config.Web.DisableConventionFallback`（YAML/env 可配，默认 false=开）或 `SetConventionFallback(bool)` 运行期切换。catch-all 匹配所有 method，错误动词请求落到回退而非裸 405（动词管控属 filter 层，与 Yii2 一致）。
- **Module 前缀挂载**：`h.Module("/admin")` 返回 `ModuleRoutes`（前缀须已在 base 注册表登记，否则报错），其 `Get/Post/...` 把相对 path 拼到前缀下，返回同样的 `*Route` 可命名。约定回退按模块注册表隔离：模块 controller 只在其前缀下解析、根 controller 只在根解析、长短前缀（`/api` vs `/api/v1`）取最长匹配。
- **base 联动**：`Application` 增加根作用域 controller 注册表（`RegisterController/Controller/ControllerIDs`，校验复用 03 的错误值），与 `Module` 共用抽出的 `controllerRegistry` 实现（Module 公共 API 不变）。

**TDD**：核心路径先红后绿——先落 `url/url_test.go`（10 测）与 `web/router_test.go`（13 测），`go test ./url/ ./web/` 构建失败（包/API 未定义），实现后转绿。

**验收证据**（合并后 main 上，CGO 经 scoop mingw gcc 启用）：

- `go build ./...` / `go vet ./...` 无输出；`gofmt -l .` 无文件；`golangci-lint run` 0 issues（03 时的 typechecking 环境偏差已随本机 linter 更新消失）。
- `CGO_ENABLED=1 go test -race -count=1 ./...` -> 72/72 PASS（14 包，无 data race）；其中 `url` 10 测、`web` 27 测（新增 13）、`base` 含新增 `TestApplicationControllerRegistry`。
- `web/router_test.go` 13 测逐项对应验收项：`TestExplicitRouteParamsTypedValues` + `TestTypedParamParseFailureBecomesBadRequest`（ParamInt/ParamInt64/ParamBool/Param，溢出 -> 400）、`TestMethodHelpersAndMethodMismatch`（GET/POST 各答、PUT 落回退 404）、`TestNamedRoutesPowerReverseGeneration`（命名 `/post/9`、未命名约定 `/site/hello?id=1`、`url.To` 接线）、`TestExplicitRouteBeatsConventionFallback`（同路径显式胜出 + 未显式动作照常约定解析）、`TestConventionFallbackDisabled`（配置关 -> 404，运行期重开 -> 200）、`TestConventionFallbackModuleScopeAndIsolation`（模块/根双向隔离 + 各类 404 边界）、`TestConventionRoutesServeAnyMethod`、`TestConventionActionSignatureMismatchIsServerError`（500）、`TestMultiSegmentModulePrefixResolves`（/api 与 /api/v1 最长前缀各答各的）、`TestModuleScopedExplicitRoutes`（`h.Module` 挂载 + 命名反查 `/admin/user/9` + 模块内显式胜约定 + 未注册前缀报错）、`TestUnmatchedPathsRenderNegotiatedNotFound`（JSON/HTML 协商 404）、`TestRoutingOverRealServer`（httptest.NewServer 真 TCP：显式参数路由 + 约定回退 + 正则失配经回退 404）。

**遗留**：约定路由不解析默认 action（`/post` 单段不指向 ActionIndex，严格两段）；405 Allow 头未提供（catch-all 吞掉方法失配，动词管控待 10 号 filter 票）；`url.To` 需显式 `url.SetDefault(h.URLs())` 接线（不做隐式全局注入）；04 的 `web/error_test.go` 存在 gofmt 漂移，本票顺手格式化（无语义变更）。
