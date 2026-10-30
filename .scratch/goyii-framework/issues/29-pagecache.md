# 29: PageCache

Status: resolved

**What to build:** 整页缓存 Filter（web 包，形态对齐 HttpCache）：`PageCache{Duration, Dependency, VaryBy}`——命中直接回放 body + status + Content-Type，未命中捕获响应后写入应用 cache 组件。key = 路由 + 规范化查询串 + VaryBy；仅 200 的 GET 文本响应参与；失效走 cache.Dependency。body 捕获 ResponseWriter 包装导出，供 debug 面板 toolbar 注入复用（32 依赖此点）。

**Blocked by:** None (can start immediately)

- [x] PageCache Filter 落地：仅 200 GET 文本响应缓存，POST/写动作不参与
- [x] 命中回放（含 Content-Type）；未命中捕获后按 Duration/Dependency 写入 cache 组件
- [x] key = 路由 + 规范化查询串 + VaryBy；依赖失效重建
- [x] body 捕获 writer 导出可复用
- [x] 单测：命中/未命中/依赖失效重建/VaryBy 变体分裂

## Comments

### 验收证据（2026-10-29，提交 23e0ca7）

- Filter 落地：`web.PageCache{Duration, Dependency cache.Dependency, VaryBy func(*http.Request) string}`，`Middleware() Middleware` 形态对齐 HttpCache；cache 组件经 `FromContext(r.Context()).App().Cache()` 取（与其他 Filter 一致），无请求 Context 时透传。仅 200 GET 文本（text/*）响应参与：`TestPageCacheSkipsWriteVerbs`（POST 不读不写缓存）、`TestPageCacheSkipsHeadRequests`（HEAD 透传，注释写明避免空 body 污染共享 key 的取舍）、`TestPageCacheSkipsNonTextResponses`（JSON 不缓存）、`TestPageCacheSkipsNonSuccessStatuses`（404 页与 action 报错均不入缓存）、`TestPageCacheSkipsFlushedResponses`（流式/Flush 响应不缓存，取舍注释写明）。
- 命中回放/未命中写入：`TestPageCacheHitReplaysWithoutAction`（命中不跑 action，回放 body+status+Content-Type）；`TestPageCacheStoresEntryInAppComponentWithTTLAndDependency`（条目落应用 cache 组件 key="/page"，Duration→ttl、Dependency 原样传入 GetOrSet）。缓存内容 = `cachedPage{Status, ContentType, Body}`，经 cache 包 JSON 信封往返。
- key 与依赖失效：`TestPageCacheKeyNormalizesQueryOrder`（`?sort=id&filter=new` 与 `?filter=new&sort=id` 同一条目，参数值不同另开条目）；`TestPageCacheDependencyInvalidationRebuilds`（动态依赖指纹翻转 → 重建新页 → 新指纹继续回放）；`TestPageCacheVaryBySplitsVariants`（Accept-Language 两变体各自缓存/回放）。
- 导出捕获 writer：`web.CaptureWriter` + `NewCaptureWriter(w)`，`StatusCode()/Body()/Flushed()/Err()` + `Unwrap()`（ResponseController 兼容）+ 直接实现 `Flush()`（可观察流式，供缓存资格判定与 32 复用）。`TestCaptureWriterRecordsAndForwards`（捕获/转发/重复 WriteHeader 丢弃/Flusher 断言/ResponseController.Flush）、`TestCaptureWriterRemembersWriteError`（客户端断连写错误记忆）。
- 实现要点（管线适配）：action 经 Context 的 writer 写响应、不经链上传入的 w，故未命中路径以同包身份把 `c.w` 换成包捕获的 responseWriter（defer 还原）——全局链挂载（`TestPageCacheHitReplaysWithoutAction`）与控制器中间件挂载（`TestPageCacheMountedAsControllerMiddleware`）两条挂载路径均测。后端故障降级：lookup 失败直接裸跑 action、store 失败不影响已出响应（`TestPageCacheBackendFailureServesUncached`）；`TestPageCacheStandalonePassesThrough`。
- 测试摘要：`go test ./... -count=1` 25 包 1320 用例全绿；web 包 `-count=2` 314 用例重复稳定；`go vet ./web/...`、`golangci-lint run ./web/...` 0 issues；gofmt 无差异。文档：`docs/组件-web路由与中间件.md` Filter 表增 `web.PageCache` 行。
