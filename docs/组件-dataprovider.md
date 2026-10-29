# 组件：dataprovider

列表页与列表 API 的取数层：ActiveDataProvider 把分页排序下推 SQL，ArrayDataProvider 分页内存切片；查询参数沿用 Yii2 惯例。

## 参数解析（防注入硬标准）

```go
params, err := dataprovider.FromRequest(c.Request(), "id", "title", "status", "created_at")
if err != nil {
    return web.BadRequest(err.Error())   // 非白名单字段：400，任何请求文本到不了 SQL
}
```

- `page`：1-based；非法/越界回退或钳到最后一页。
- `per-page`：默认 20，上限 100（钳制不报错）。
- `sort`：`sort=-created_at,id`——前导 `-` 降序，逗号分隔多字段；**必须逐字命中 action 声明的白名单**，且通过裸标识符校验兜底（白名单本身被配错也拒）。错误包装 `dataprovider.ErrInvalidSortField`。

## ActiveDataProvider（SQL 下推）

```go
res, err := dataprovider.ActiveDataProvider[Post]{
    Query:        ar.New[Post](c.Context()).With("Author"),  // 基查询同塑 COUNT 与取页
    Params:       params,
    DefaultOrder: []dataprovider.Sort{{Column: "id", Desc: true}},  // 请求未排序时
}.Prepare()
```

`Prepare` 执行恰好两条语句（空集时一条 COUNT）：`count(*)` + 带 `ORDER BY/LIMIT/OFFSET` 的取页 SELECT；基查询不被污染。

## ArrayDataProvider（内存分页）

```go
res, err := dataprovider.ArrayDataProvider[*Item]{
    Items:        allItems,
    Params:       params,
    DefaultOrder: []dataprovider.Sort{{Column: "priority", Desc: true}},
    Key:          func(item *Item, column string) any {   // 排序列取值：string/数值/bool/time.Time
        switch column {
        case "priority": return item.Priority
        case "name":     return item.Name
        }
        return nil
    },
}.Prepare()
```

排序在副本上进行（调用方切片顺序不变），稳定排序；类型不合法即报错，不静默猜。

## Result：响应与 LinkPager 共用

```go
type Result[T any] struct {
    Items      []T `json:"items"`        // 非 nil：空集编码为 []
    TotalCount int  `json:"totalCount"`
    PageCount  int  `json:"pageCount"`
    Page       int  `json:"page"`        // 已钳制的生效页
    PerPage    int  `json:"perPage"`
}
```

API 直接 `c.JSON(res)`；页面侧 `res.Pagination()` 映射到 `view.Pagination` 供 [LinkPager](组件-view与templ.md) 消费——两处分页语义锁死一致（1-based）。

```go
pager := view.LinkPager{
    Pagination: res.Pagination(),
    URL:        func(page int) string { return url.To("post/list", url.P{"page": strconv.Itoa(page)}) },
}.Render()
```

## 相关文档

- [组件：view 与 templ](组件-view与templ.md)
- [组件：db 与 ActiveRecord](组件-db与ActiveRecord.md)
