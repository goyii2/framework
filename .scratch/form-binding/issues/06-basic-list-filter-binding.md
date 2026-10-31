# 06: basic 列表过滤迁移（BindQuery + Apply 保留）

Status: resolved

**What to build:** examples/basic 列表页的过滤读取改走表单绑定：`postFilters` 打 `form` tag 经 `form.BindQuery` 读取，`readPostFilters` 的手写 `Query().Get` 消失；`Apply`（title 的 LIKE 转义 + status 等值）与 `active()` 保留在项目代码——过滤**语义**显式、读取**机制**归框架。分页/排序 URL 保留过滤值的 `keep` 闭包改由过滤 struct 派生（不再逐字段手抄键名）。`dataprovider.FromRequest` 的分页排序解析**维持现状不动**。

**Blocked by:** 01

- [x] 标题 / 状态过滤行为与迁移前一致（含 LIKE 转义防注入用例）
- [x] `readPostFilters` 删除；`keep` 闭包不再逐字段手抄
- [x] `Apply` / `active()` 保留在项目代码，查询组装仍显式
- [x] `FromRequest` 调用方式不变
- [x] 既有列表相关测试通过

## Comments

- **2026-10-31 实施验收（落地提交 `b8dc08f`，经 `583360e` 认领；feat/form-binding-06 已 ff-only 合并 main 并删除）**
- 交付物：`examples/basic/controllers/post.go`——`postFilters` 字段改导出并打 `form:"title"` / `form:"status"` tag；`ActionList` 改 `form.BindQuery(c.Request(), &filters)` 读取（绑定后 `TrimSpace(Title)` 保留原去空白语义）；`readPostFilters` 删除；`active()` 与 `apply()`（LIKE 转义 + status 等值）原样保留（仅字段名随导出调整）；`dataprovider.FromRequest` 及 per-page/sort 读取一字未动。
- **keep 派生形态裁决**：采用 `form.Values(&filters)` —— keep 闭包遍历 Values 导出的 map 跳过空值写入目标参数，保留值随 struct 的 form tag 自动派生（新增过滤字段零改动自动携带），不再手抄键名；空值不进 URL、Title 以去空白后的文本入链，均与迁移前一致。
- BindQuery 错误处理：过滤字段全为文本，类型错误不可能发生；仍按 `web.BadRequest` 归档（与上方 FromRequest 的 400 一致），防未来引入非文本过滤字段时坏值被静默丢弃——注释已说明该裁决。
- 测试证据：新增 `TestPostListFilterBehavior`（title 包含匹配 + status 等值 + 过滤输入回显；`%`/`_` 端到端只匹配字面自——含 "underXscore bait" 诱饵行防转义退化；`%20report%20` 按去空白文本过滤；空过滤全量列出）与 `TestPostListKeepsFiltersInLinks`（排序头 `href="/posts?sort=title&amp;status=published&amp;title=Pager+report"`、页码键 `href="/posts?page=2&amp;status=published&amp;title=Pager+report"` 均携带派生保留值；无过滤时链接不含过滤参数且保持 `?sort=id` 原形）。测试 harness 的 `/posts` 路由由 JSON 桩改为真实 `ActionList` 以支撑端到端断言。
- 回归：`rtk go test ./... -count=1` → **1524 passed in 52 packages**（含既有 TestLikePatternEscapesWildcards 转义契约用例）；`rtk go vet ./...` 干净；`golangci-lint run ./examples/basic/...` 0 issues；gofmt 干净。
- 遗留说明：GridView 回显 `view.QueryValues(c.Request(), "title", "status")` 仍按请求原值（未 trim）回显，与迁移前一致——工单只要求 keep 派生，回显不在范围内；07 票 gii 过滤骨架可参照本票形态。
