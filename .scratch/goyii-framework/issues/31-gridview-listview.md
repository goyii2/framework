# 31: GridView/ListView

**Status:** claimed

**What to build:** 列表渲染大件：GridView（表格 + 列 DSL + 排序链接 + 过滤行）与 ListView（item 模板 + Summary + 分页）。输入是 action 里 `Prepare()` 好的 dataprovider.Result——纯渲染、不自行取数；过滤行只负责渲染 GET 表单与当前值回显，查询组装显式归 action（读 query 自行 Where，不引入 SearchModel 概念）。

**Blocked by:** None (can start immediately)

- [ ] GridView：输入 dataprovider.Result；DataColumn{Attr, Label, Format(默认转义、html 可选), Value, Sortable, Filter}
- [ ] 排序链接走既有 sort 参数与列白名单（dataprovider 既有约定）
- [ ] 过滤行：Filter 配置渲染（text/select 等）+ 当前值回显；查询组装归 action（注释与文档明确分工）
- [ ] ListView：item 渲染函数 + Summary 行 + 复用 LinkPager
- [ ] 单测：渲染断言（默认转义、排序 URL、过滤回显、空数据态）
