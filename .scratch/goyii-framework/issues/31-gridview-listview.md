# 31: GridView/ListView

Status: resolved

**What to build:** 列表渲染大件：GridView（表格 + 列 DSL + 排序链接 + 过滤行）与 ListView（item 模板 + Summary + 分页）。输入是 action 里 `Prepare()` 好的 dataprovider.Result——纯渲染、不自行取数；过滤行只负责渲染 GET 表单与当前值回显，查询组装显式归 action（读 query 自行 Where，不引入 SearchModel 概念）。

**Blocked by:** None (can start immediately)

- [x] GridView：输入 dataprovider.Result；DataColumn{Attr, Label, Format(默认转义、html 可选), Value, Sortable, Filter}
- [x] 排序链接走既有 sort 参数与列白名单（dataprovider 既有约定）
- [x] 过滤行：Filter 配置渲染（text/select 等）+ 当前值回显；查询组装归 action（注释与文档明确分工）
- [x] ListView：item 渲染函数 + Summary 行 + 复用 LinkPager
- [x] 单测：渲染断言（默认转义、排序 URL、过滤回显、空数据态）

## Comments

- 落地提交 376dea6（feat(view): gridview and listview widgets over provider results），ff-only 合入 main。
- 形态：`view.GridView{Columns []DataColumn, ItemCount, Item func(i int) any, Sort, SortURL, FilterValues, EmptyText}` + `Render() templ.Component`，ListView 同款形态；泛型条目经 `Item func(i int) any` 闭包桥接（`func(i int) any { return res.Items[i] }`），分页经 `res.Pagination()`，纯渲染不取数（注释与 doc.go、docs/组件-view与templ.md「列表大件」节写明分工）。
- DataColumn：`Format` 默认 `FormatText` 转义、`FormatHTML` 显式 opt-in 透传；`Value func(item any) any`（nil 空单元格、组件内联、其余 fmt.Sprint）；`Sortable` 列头渲染升/降切换链接（`?sort=col` / `?sort=-col`，`SortURL` 回调可保留查询串），非 Sortable 列无链接即 widget 侧白名单。
- 过滤行：`Filter *ColumnFilter{Type, Options}`（text/select，复用 ActiveForm 的 FieldType/FieldOption），当前值经 `FilterValues map[string]string` 回显，action 侧助手 `view.QueryValues(r, names...)` 从请求 query 取值；任一列配 Filter 时表格包 `<form method="get">`，未过滤列渲染空单元格保持对齐。
- ListView：`ItemView func(any) templ.Component` + Summary 行（`Showing <b>11-12</b> of <b>42</b> items.`，空集 `Total <b>0</b> items.`）+ 复用 LinkPager（`Pager LinkPager` 的 Pagination 恒取列表自己的，URL 回调由调用方给）。
- 验收证据（`go test ./view/`：46 passed；全量 `go test ./...`：1339 passed in 25 packages；golangci-lint ./view/ 0 issues）：
  - 默认转义/html opt-in：`TestGridViewEscapesByDefaultHTMLOptIn`（`<script>` 全转义、`FormatHTML` 透传 `<b>bold</b>`、nil 空单元格、组件内联）
  - 排序 URL：`TestGridViewSortLinksFollowSortParameter`（未排序→`?sort=id`、asc→`?sort=-id`+`sortable asc`、`-id`→`?sort=id`+`sortable desc`、多字段 sort 解析、SortURL 回调、非 Sortable 无链接）
  - 过滤回显：`TestGridViewFilterRowEchoesValues`（form method=get、text 回显 value、select 选中态、空单元格对齐）+ `TestGridViewFilterValuesFromRequestHelper`
  - 空数据态：`TestGridViewEmptyState`、`TestListViewEmptyState`（默认/自定义 EmptyText、占位行 colspan）
  - ListView 组合：`TestListViewRendersItemsSummaryAndPager`（Summary 区间、item 顺序渲染、LinkPager 复用）、`TestListViewPagerHidesOnSinglePage`
  - 表格/行渲染基础：`TestGridViewRendersTableRows`
- 顺手小重构：ActiveForm 与 GridView 过滤 select 的选项选中态解析抽成 `resolveOptions` 共用（activeform.go）。
- examples/basic 列表页重构留给 36 号集成票，本票未动 examples。
