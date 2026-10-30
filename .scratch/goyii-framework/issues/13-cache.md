# 13: cache 组件

Status: resolved

**What to build:** 缓存组件：Cache 接口 + memory/file/redis 三实现 + 依赖失效（Dependency：依赖值变化/时间条件触发失效）；TTL 与并发安全。

**Blocked by:** 02

- [x] 三后端行为一致：get/set/delete/存在性/过期（redis 用 miniredis）
- [x] Dependency 失效：依赖变化后缓存条目被判定失效并重建
- [x] 并发访问安全（竞态测试）
- [x] Cache 为接口，业务侧可注入 fake（单测）

## Comments

### 验收证据（2026-10-28）

- 提交：feat `93019fc`（main，经 feat/cache-component ff-only 合并）；认领 `3fa4e29`（chore: claim ticket 13 cache component）。
- 测试：`go test ./...` 17 包 948 项全过；`CGO_ENABLED=1 go test -race ./cache/ ./base/` 全过（含三后端 16 goroutine x 50 次混合读写删/GetOrSet/Flush 竞态矩阵与 Dependency 并发求值）；`go vet ./...`、`gofmt -l` 干净。本票新增 cache 包 53+ 测试项与 base 挂载测试。
- 行为一致性：`TestCacheBehaviorConsistency` 同一场景表跑 memory/file/redis(miniredis) 三后端——往返/解码副本（无别名泄漏）/覆盖写/缺失即 miss/Delete 幂等/Exists 跟踪/过期读 miss（memory/file 改 envelope 绝对时间、redis FastForward）/Flush 清空/空键拒绝/不可编码值报错；`TestCacheNonPositiveTTLFallsBackToDefault` 验证 ttl<=0 回退默认 TTL。
- Dependency：`TestDependencyInvalidation`（ValueDependency 值变即重建，值同则命中）、`TestTimeDependencyInvalidation`（窗口内命中、窗口过即重建且持续失效直至换新窗口）、指纹语义单测；依赖校验只发生在 GetOrSet，普通 Get 按存储值直接命中。
- 组件化：`base.Application.Cache()/SetCache()`（懒初始化持有器，`base/cache_test.go` 用 fake 注入验证可替换与可还原）；`config.cache` 段 Driver/KeyPrefix/TTL/Path/Redis，缺省 memory / 3600s / `os.TempDir()/goyii-cache`。

### API 分叉与决策（相对 Yii2）

1. **序列化选 JSON 而非 gob**：gob 解码回 `any` 需逐类型 `gob.Register`，JSON 开箱即用且 file/redis 中人可检视。配套 Get 为「解码进调用方 dest 指针」形态（同 `json.Unmarshal`），类型保真不依赖类型注册。三后端（含 memory）统一走序列化信封，取回值必为解码副本，杜绝存储对象别名泄漏，行为完全一致。
2. **`Set` 的 ttl<=0 语义 = 后端默认 TTL（配置默认 1h），非 Yii2 的 0=永不过期**：长驻进程中"永不过期"是泄漏隐患；需要更久条目显式传大 ttl。
3. **redis `Flush` 按 key 前缀 SCAN+DEL，非 Yii2 的 FLUSHDB**：redis 常被多组件共享，FLUSHDB 会殃及邻居（`TestRedisFlushScopedToPrefix` 验证隔离）。
4. **普通 `Get` 不带 Dependency 参数**（Yii2 `get($key, $dependency)` 可带）：保持 dest 解码签名无可选参数，依赖仲裁集中在 GetOrSet（`cache.Dependency` 接口：Capture 存指纹 / Changed 按当前状态判定，当前状态为权威——存档指纹失配或当前不可判定即失效重建）。
5. **file 后端文件名 = md5(prefix+key) 十六进制**（同 Yii2 FileCache）：键任意字符串安全落盘；Flush 只删 32-hex 名与自身 `cache-*` 残留临时文件，目录内外来文件不动。
6. memory 后端对应 Yii3 ArrayCache（Yii2 无核心内存缓存，APCu 属共享内存扩展）；后台清扫间隔 = 默认 TTL/2（夹在 10ms~1m），`Stop()` 可停，停后读路径仍惰性过期。
