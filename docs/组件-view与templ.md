# 组件：view 与 templ

视图层基于 [templ](https://templ.guide)：命名视图注册表 + Layout 组件组合 + Widget 最小集。渲染按名查表（map 命中，零反射），页面在 `.templ` 文件里写、`templ generate` 生成 Go 代码。

## 注册与渲染

```go
// 装配期注册：名字 -> 工厂（view.Typed 从类型化构造器派生断言）
h.Views().Register("post/show", view.Typed(postShowPage))
h.Views().Register("error", view.Typed(func(d view.ErrorData) templ.Component { ... }))

// action 里渲染（经当前 Layout 装帧输出完整 HTML）
c.SetTitle(post.Title)
return c.Render("post/show", postShowData{...})

// 422 表单回显等带状态码的场景
return c.RenderStatus(http.StatusUnprocessableEntity, "post/form", data)

// 不经注册表直传组件
return c.RenderComponent(myComponent)
```

## Layout 组合

Layout 是 `func(view.Page) templ.Component`，`Page` 是槽位结构（Title/Head/Nav/Content/Footer）。层级由宽到窄，窄的胜：

```mermaid
flowchart LR
    B["内置 view.Shell<br/>（无覆盖时）"] --> M["模块级<br/>ModuleRoutes.Layout(l)"]
    M --> P["页面级<br/>c.SetLayout(l)"]
    P --> C["action 渲染的内容"]
```

应用级公共外框 = 装配一段全局中间件给每个请求 SetLayout（`examples/basic` 的做法；布局闭包捕获请求 Context，菜单可见性随身份/权限/语言变化）。`c.ClearLayout()` 输出裸片段。嵌套模块下子模块经自己的 registrar 设 `Layout` 即覆写父模块的框架，父模块路由不受影响。

## Widget 一览

结构体配置 + `Render() templ.Component`，可在 Go 里组装、也可在 templ 里 `@` 内嵌：

| Widget | 配置 | 要点 |
|---|---|---|
| `view.Menu` | `Items []MenuItem{Label, URL, Active, Items}` | 嵌套无界；Active 标记当前项 |
| `view.Breadcrumbs` | `Links []Crumb{Label, URL}` | URL 为空的 crumb 即当前页（unlinked + active） |
| `view.LinkPager` | `Pagination, URL func(page int), MaxButtonCount, 首末页标签, HideOnSinglePage` | 消费 `view.Pagination{TotalCount, PerPage, Page}`（1-based）；URL 缺省 `?page=N` |
| `view.DetailView` | `Rows []DetailRow{Label, Value}` | Value：组件内联 / nil 空格 / 其余 `fmt.Sprint`，全部自动转义 |
| `view.ActiveForm` | `Fields, Errors, Values, CSRF...` | 服务端表单，回显 validators 校验错误与提交值 |
| `view.GridView` | `Columns []DataColumn{Attr, Label, Format, Value, Sortable, Filter}, ItemCount, Item, Sort, SortURL, FilterValues` | 列表大件之一，见下节 |
| `view.ListView` | `Pagination, ItemCount, Item, ItemView, Pager` | 列表大件之一，见下节 |

```go
detail := view.DetailView{Rows: []view.DetailRow{
    {Label: "Title", Value: post.Title},
    {Label: "Author", Value: author},
}}.Render()
```

## 列表大件：GridView / ListView

两个列表 widget 都是**纯渲染**：输入是 action 里 `Prepare()` 好的 `dataprovider.Result`，widget 不取数。泛型条目经 `Item func(i int) any` 闭包桥接（`func(i int) any { return res.Items[i] }`），分页元数据经 `res.Pagination()` 映射成 `view.Pagination`：

```go
params, _ := dataprovider.FromRequest(c.Request(), "id", "created_at") // 列白名单在这里声明
res := provider.Prepare(ctx, params)
q := c.Request().URL.Query()

grid := view.GridView{
    Columns: []view.DataColumn{
        {Attr: "id", Sortable: true, Value: func(item any) any { return item.(Post).ID }},
        {Attr: "title", Label: "标题", Value: func(item any) any { return item.(Post).Title }},
    },
    ItemCount:    len(res.Items),
    Item:         func(i int) any { return res.Items[i] },
    Sort:         q.Get("sort"),
    FilterValues: view.QueryValues(c.Request(), "title"),
}
```

- **列格式**：`Format` 默认转义（`FormatText`）；`FormatHTML` 显式 opt-in 才透传原始 HTML；`Value` 返回 `templ.Component` 时内联渲染，nil 渲染空单元格。
- **排序**：`Sortable` 列头渲染升/降切换链接，值走 dataprovider 既有 `sort=-created_at,id` 约定；widget 只给 `Sortable` 列出链接（widget 侧的列白名单），URL 缺省 `?sort=...`，`SortURL` 回调可保留当前查询串。
- **过滤分工**：widget 只负责渲染 GET 过滤行（`Filter: &ColumnFilter{Type, Options}`，text/select 等）并回显当前值（`FilterValues`，可 `view.QueryValues(r, names...)` 从请求取）；**查询组装归 action**——读 query 自行 `Where` 后再 `Prepare`，不引入 SearchModel。
- **空数据态**：`Items` 为空时渲染占位行（`EmptyText` 可配，默认 "No results found."）。

ListView = item 模板 + Summary + 分页，分页直接复用 `LinkPager`（`Pagination` 自动取列表自己的，URL 回调由调用方给）：

```go
list := view.ListView{
    Pagination: res.Pagination(),
    ItemCount:  len(res.Items),
    Item:       func(i int) any { return res.Items[i] },
    ItemView:   func(item any) templ.Component { return postRow(item.(Post)) },
    Pager:      view.LinkPager{URL: func(page int) string { return url.To("post/index", url.P{"page": page}) }},
}
```

## 分页联动

[dataprovider](组件-dataprovider.md) 的 `Result.Pagination()` 直接映射 `view.Pagination`，两者 1-based 语义由常量锁死；LinkPager 渲染真实位置（越界请求已钳到末页）。

## templ 工具链

```bash
go install github.com/a-h/templ/cmd/templ@v0.3.1100
templ generate ./examples/basic/    # 生成 *_templ.go（生成物入库提交）
```

templ 组件里可直接 `i18n.T(ctx, "app", "Home", nil)`——渲染 ctx 携带请求语言与消息源，一套模板服务所有语言。URL 属性一律经 `templ.URL(...)` 消毒。

## 相关文档

- [组件：dataprovider](组件-dataprovider.md)
- [组件：i18n](组件-i18n.md)
- [快速上手](快速上手.md)（examples/basic 的页面全链路）
