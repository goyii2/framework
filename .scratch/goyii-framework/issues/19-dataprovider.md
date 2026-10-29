# 19: DataProvider

**Status:** resolved

**What to build:** 数据提供者：ActiveDataProvider（包装 AR 查询自动应用分页排序，仅取当页数据）与 ArrayDataProvider（内存切片）；查询参数沿用 Yii2 惯例 `page`/`per-page`/`sort=-created_at,id`；响应含 items + 分页元信息（totalCount/pageCount/当前页），供 API 与 LinkPager（16）共用。

**Blocked by:** 08

- [x] ActiveDataProvider 分页排序下推到 SQL（只查当页与总数）
- [x] ArrayDataProvider 对内存切片分页排序
- [x] 参数越界（超总页数/非法 sort 字段）行为定义且安全（不透传到 SQL）
- [x] 分页元信息形态与 LinkPager 联动验证

## Comments

### 落地形态（main `f772935`，分支 feat/dataprovider 已 ff 合入并删除；包 `dataprovider`）

- **参数层（两种 Provider 共用）**：`dataprovider.ParseParams(page, perPage, sort string, sortable ...string)` 与 HTTP 便捷入口 `FromRequest(r *http.Request, sortable ...string)`（action 里 `FromRequest(c.Request(), "id", "created_at")`——只依赖 `net/http`，经 `web.Context.Request()` 取参，不引 web 包，无环）。产出 `Params{Page, PerPage, Sort []Sort}`，`Sort{Column, Desc}`。`DefaultPerPage` 直接别名 `view.DefaultPerPage`（20，两处默认锁死不漂移），`MaxPerPage = 100`。
- **ActiveDataProvider**：结构体配置 `ActiveDataProvider[T]{Query *ar.Query[T], Params, DefaultOrder}` + `Prepare() (*Result[*T], error)`。基查询（Where/Join）同时作用于 COUNT 与取页；ORDER BY/LIMIT/OFFSET 经 ar.Query 不可变分支追加，基查询零污染（有测试）。取页返回 `[]*T`（绑定 ctx 的 AR 实例）。总数超出 int 范围（仅 32 位构建可能出现）显式报错不截断。
- **ArrayDataProvider**：`ArrayDataProvider[T]{Items []T, Params, DefaultOrder, Key func(item T, column string) any}` + `Prepare() (*Result[T], error)`。内存排序走 `Key` 列值提取（支持 string / 全部 int/uint 宽度 / float / bool / time.Time，`compareOrdered` 泛型直接比较无减法溢出；类型混用或不受支持 → Prepare 报错，不做静默排序）；键按列一次性预提取并成对校验，排序用稳定排序且**在副本上进行**，调用方切片顺序不变。无任何排序时不需要 Key。
- **响应形态 `Result[T]`**：`Items / TotalCount / PageCount / Page(已钳制的生效页) / PerPage`，带 JSON tag（items/totalCount/pageCount/page/perPage），`c.JSON(res)` 即出 API 响应；空集 `items` 为非 nil 空切片（JSON `[]` 而非 null）。`res.Pagination()` 直接映射 `view.Pagination{TotalCount, PerPage, Page}`（16 号契约），供 `view.LinkPager` 消费。

### 越界与安全行为（定义并全部有测试）

- **page 越界**：超出总页数 → 钳到最后一页（响应 `Page` 为钳后生效页，LinkPager 渲染真实位置）；总数为 0 → 第 1 页 + 空集（且只执行 COUNT 一条 SQL）；page 非法/`<=0`/非数字 → 回退 1。
- **per-page 越界**：非法/`<=0` → 默认 20；`> 100` → 钳到 100。手写 `Params` 同样在使用点防御性归一。
- **sort 白名单（防 SQL 注入硬标准）**：排序字段必须与 action 显式声明的 `sortable` 集合**逐字相等**，且通过裸标识符校验（字母/数字/下划线、首字符非数字）——即使白名单本身被配错（如含 `id; DROP TABLE users`），标识符兜底仍拒绝，任何请求文本都到不了 ORDER BY。非法字段返回包装 `ErrInvalidSortField` 的错误（action 映射 400）；未声明任何 sortable 而带 sort 参数也拒绝；`sort=-`（空列名）拒绝而非静默忽略。

### API 分叉与决策（相对 Yii2）

- **结构体配置 + `Prepare()` vs Yii2 getter 惰性执行**：对齐仓库既有 Widget 形态（结构体配置 + 方法产出），`Prepare` 显式取数返回 Result，错误路径走 Go error 而非 Yii2 的异常。
- **ActiveDataProvider 返回 `*Result[*T]`（items 为 `[]*T`）**：AR 查询产物即绑定 ctx 的实例指针，保留实例持久化能力；ArrayDataProvider 泛型 `T` 不限（惯用 `[]*Model`，两Provider 响应形态自然一致）。
- **非法 sort 字段拒绝而非忽略**：Yii2 对未知 sort 属性仅告警忽略；goyii 返回 `ErrInvalidSortField` 错误交 action 决断（默认 400），失败可见、可测试，白名单语义更严格。
- **per-page 上限 100（Yii2 无默认上限）**：钳制而非报错，防整表拖取的滥用，行为可预期。
- **ArrayDataProvider 的 `Key` 提取器 vs Yii2 的数组值比较**：Go 无 PHP 式宽松比较，排序值类型必须显式可序（字符串/数值/bool/time.Time），类型不合法即报错——不做反射猜测。
- **分页 1-based**：与 16 号 `view.Pagination` 契约一致（见 16 号 Comments 的分叉记录），`Result.Pagination()` 零转换映射。

### 验收证据（2026-10-28，合并后 main）

- **SQL 下推**（`active_test.go`，SQLite 内存库 + 经 `db.WithGormLogger` 注入语句记录器）：`TestActiveProviderPushesPaginationAndSortToSQL` 断言 `Prepare` 恰好执行 **2 条**语句——一条 `count(*)` + 一条含 `ORDER BY`/`LIMIT`/`OFFSET` 的取页 SELECT，且 12 行 per-page 5 第 2 页 id DESC 精确取到 [7..3]；`TestActiveProviderEmptySet` 断言空表时只执行 1 条 COUNT、返回第 1 页空集（items 非 nil）。
- **越界行为**：`TestActiveProviderClampsPageBeyondLast`（page 99 → 第 3 页 + 末页 2 行）、`TestArrayProviderClampsPageBeyondLast`、`TestArrayProviderEmptyItems`；参数回退 `TestParseParamsPageFallsBackToOne` / `TestParseParamsPerPageDefaultAndCap`（含 999999999999 → 100）/ `TestParamsPerPageNormalizesHandConstruction`。
- **白名单防注入**：`TestParseSortRejectsFieldOutsideWhitelist`、`TestParseSortRejectsSQLFragmentsEvenWhenWhitelisted`（白名单被污染仍拒）、`TestParseSortWithoutDeclaredFieldsIsRejected`、`TestActiveProviderHandBuiltSortNeverReachesSQL`（绕过解析器手构恶意列名：Prepare 报错且只跑了 COUNT、无取页 SQL）；`TestParseSortEmptyVariantsSortNothing` 覆盖 `sort=-` 拒绝与空串忽略。
- **ArrayDataProvider**：`TestArrayProviderSortsAndPages`（多字段 priority DESC,name + 取页）、`TestArrayProviderSortsOnACopy`（调用方切片不被重排）、`TestArrayProviderStableSortKeepsEqualOrder`、`TestArrayProviderWithoutKeyFunctionRefusesSort`、`TestArrayProviderRejectsUnsupportedKeyType`、`TestArrayProviderRejectsMixedKeyTypes`、`TestArrayProviderDefaultOrder`。
- **LinkPager 联动**（`result_test.go`）：`TestResultPaginationFeedsLinkPager`（45 行/10 每页/第 2 页：PageCount/CurrentPage/Offset/HasPrev/HasNext 与 Result 元信息一致，渲染 HTML 含 `?page=1`/`?page=3`/active）；`TestResultPaginationClampedPageRendersTruePosition`（钳制页渲染为末页，无越页链接、next disabled）；`TestResultJSONCarriesItemsAndMeta` + `TestResultEmptyItemsEncodeAsArray`（API JSON 形态）。
- **基查询零污染**：`TestActiveProviderRespectsBaseQueryConditions`（Where 同塑 COUNT 与取页）、`TestActiveProviderLeavesBaseQueryUntouched`（Provider 跑完后基查询仍取全量 8 行）。
- **测试与静态检查**：`go test -count=1 ./dataprovider/` 30 测通过；`CGO_ENABLED=1 go test -race -count=1 ./dataprovider/` 通过；全仓 `CGO_ENABLED=1 go test -race -count=1 ./...` 21 包全绿；`go build ./...` / `go vet ./...` / `gofmt -l .` 无输出。
- **lint 环境注记**：本机 `golangci-lint` 2.13.1（go1.26 构建）在 go1.27.2 GOROOT 上对所有包 panic（`./view/...`、`./i18n/...` 等未改动包同样炸，属既有环境不匹配）；临时装 golangci-lint 2.15.0（go1.27.2 构建）验证：`./dataprovider/...` **0 issues**（全仓其余 38 个 findings 均在本次未触碰的包，为 2.15 默认集收紧暴露的存量，与本票无关）。

### 遗留

- GridView/ListView 等列表大件归 P2；`sort` 参数的 `.` 多字段分隔（Yii2 支持 `.` 与 `,` 两种）未复刻，仅逗号分隔（工单样例即逗号形态）；per-page=0 表示不分页（Yii2 `pagination => false`）未做，如需可后续在 Params 加开关。
