# 16: templ 视图层

Status: resolved

**What to build:** 视图层（PRD P1）：视图注册表——`ctx.Render("post/view", data)` 按名查表（map 查找零反射），亦支持直传 templ 组件；Layout 以组件组合实现（模块级/页面级可覆盖）；Widget 最小集四件（LinkPager/Breadcrumbs/Menu/DetailView）。不做 AssetManager（PRD 放弃清单，替代路径在 P2）。

**Blocked by:** 04

- [x] 按名渲染与直传组件两条路径均返回完整 HTML
- [x] Layout 可按模块/页面覆盖，公共外框只写一遍
- [x] Widget 四件可用且各自单测
- [x] 错误页可经视图注册表自定义覆盖（联动 04 的错误处理器）

## Comments

### API 形态

- `view.Registry`（`Handler.Views()` 取得）：`Register(name, Factory)`（重复注册 panic，装配方错误）+ `Lookup(name)`；渲染路径 = map 查找 + 工厂调用，零反射。`view.Typed[T](func(T) templ.Component)` 从类型化构造器派生断言。
- `web.Context`：`Render(name, data)`（按名）/ `RenderComponent(comp)`（直传），两路都经当前 Layout 装帧输出完整 HTML（未覆盖时用内置 `view.Shell`）；`SetLayout` / `ClearLayout`（裸片段）/ `SetTitle` 为页面级控制。
- Layout 组合：`view.Layout = func(view.Page) templ.Component`，`view.Page{Title, Head, Nav, Content, Footer}` 为槽位结构。层级：内置 Shell < 模块级（`ModuleRoutes.Layout(l)`，作用于其后挂载的路由）< 页面级（action 内 `SetLayout`）。
- 错误页：注册表保留名 `error/<status>`（如 `error/404`）优先、`error` 兜底，工厂收 `view.ErrorData{Status, Name, Message}`；未注册回落 04 的内置最小 HTML 页；JSON 协商路径不受影响；错误视图工厂 panic 时安全降级（记日志走下一候选）。
- Widget 四件：结构体配置 + `Render() templ.Component`。LinkPager 消费 `view.Pagination{TotalCount, PerPage, Page}`（1-based 当前页，`PageCount/CurrentPage/Offset/HasPrev/HasNext` 派生；`URL func(page int)` 缺省 `?page=N`）；Breadcrumbs 末位无 URL 即当前页（active 不带链接）；Menu 支持嵌套 + Active 标记；DetailView 显式行（`DetailRow{Label, Value}`，Value 为 templ.Component 内联渲染、nil 空单元格、其余 `fmt.Sprint`，模板自动转义）。

### API 分叉与决策（相对 Yii2）

- **视图注册表 vs Yii2 视图文件路径渲染**：Yii2 按 `views/<path>.php` 文件系统查找 + MVC 渲染；goyii 按名查显式注册表（PRD 既定：零反射、编译期可见）。错误页覆盖也走同一注册表（Yii2 用 error handler 的 errorView 路径）。
- **分页 1-based**：Yii2 Pagination 内部 0-based（`page` 参数 1-based 转换）；goyii `Pagination.Page` 直接 1-based（与请求参数一致），19 号 DataProvider 落地时将其响应分页元信息（totalCount/pageCount/当前页）映射到此结构。
- **HideOnSinglePage 默认 false**：对齐 Yii2 默认（单页仍渲染 pager），可配置隐藏；隐藏时返回 `view.Blank()`（空组件，调用方可无条件内嵌）。
- **Breadcrumbs 当前页判定**：Yii2 靠 linkTemplate/labelTemplate 区分；goyii 用约定——URL 为空的 crumb 即当前页（unlinked + active）。
- **DetailView 显式行**：Yii2 从模型属性反射取值；goyii 不反射，行由调用方显式给出（值自动转义，组件值内联）。
- **templ 细节**：URL 一律经 `templ.URL` 消毒（javascript: 等危险 scheme 替换为 `about:invalid#TemplFailedSanitizationURL`）；转义沿用 templ 默认，未自建转义。

### 工具链

- templ 运行库与 CLI 均钉 `v0.3.1100`（`go install github.com/a-h/templ/cmd/templ@v0.3.1100` 后 `templ generate ./view/`）；`*_templ.go` 生成代码已提交入库。

### 验收证据（2026-10-28）

- 两条路径（`web/render_test.go`，httptest 打真实 Handler）：`TestRenderByNameReturnsFullHTML`（200 + `text/html; charset=utf-8` + `<!doctype html>` + title + 视图内容）、`TestRenderComponentReturnsFullHTML`；`TestRenderEscapesModelText` 断言模型文本转义。
- Layout 覆盖：`TestPageLayoutOverride`（页面级生效且默认 Shell 不再渲染）、`TestModuleLayoutOverride`（模块路由带模块框、模块外不受影响、页面级覆盖胜过模块级）、`TestClearLayoutRendersBareFragment`（裸片段）；公共外框 `view.Shell` 单写于 `view/layout.templ`（`TestShellRendersFullDocument` / `TestShellOmitsEmptySlots`）。
- Widget 四件（`view/widgets_test.go`）：LinkPager 5 测（默认窗口/首末页步进禁用/滑动窗口/自定义 URL 与标签/单页隐藏）、Breadcrumbs 2 测（结构 + 转义）、Menu 2 测（嵌套与 active/危险 scheme 消毒）、DetailView 1 测（四种 Value 形态 + 转义）；`TestWidgetsComposeInsideShell` 验证组合。`view/pagination_test.go` 4 测覆盖 PageCount/CurrentPage/Offset/HasPrev/HasNext。
- 错误页（`web/render_test.go`）：`TestErrorViewOverrideByStatus`（`error/404` 状态页优先、500 落到 `error` 通用页）、`TestErrorViewOverrideGoesThroughLayout`、`TestErrorPageFallsBackToBuiltinWithoutRegistration`（未注册回落 04 内置页）、`TestJSONErrorsStayJSONWithViewOverride`（JSON 协商不受覆盖影响）。
- 测试：`go test ./...` 全仓 20 包 1031 通过；`go test -race -count=1 ./view/ ./web/` 通过；`go vet ./...`、`gofmt -l`、`golangci-lint run ./view/... ./web/...` 干净。
- main 合并 `eb4aa9b`（--ff-only，feat/templ-views 已删）。
