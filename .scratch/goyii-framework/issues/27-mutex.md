# 27: mutex

Status: resolved

**What to build:** 按名互斥锁组件：`Lock(ctx, name, ttl) (release, err)` 与同形的 `TryLock`，语义不可重入（不复刻 Yii2 可重入，见 PRD 放弃清单）。process 实现=按名锁；redis 实现=SET NX PX + 唯一 token + Lua 校验释放，TTL 到期自动放、不自动续期（长持锁由调用方分段重取）。作为 Application 懒初始化组件，config 增 `Mutex{Driver}` 段（默认 process）。

**Blocked by:** 26

- [x] Lock/TryLock 接口落地，release 幂等
- [x] process 驱动：按名互斥、跨 goroutine 生效
- [x] redis 驱动：SET NX PX + token；错误 token 无法释放他人锁；TTL 到期自动释放
- [x] config Mutex 段 + Application 组件挂接（redis 复用 26 共享连接）
- [x] 单测：互斥、TryLock 竞争、超时释放、token 校验；文档注释明确不可重入与分段续锁姿势

## Comments

### 验收证据（2026-10-29，提交 fdb454e）

- 接口与幂等：`mutex.Mutex`（`mutex/mutex.go`）文档注释明确不可重入与分段续锁姿势（ttl 到期自动放、不自动续期；长持锁分段重取）。`TestProcessReleaseIdempotent`（双重 release 安全且真正放锁）；redis release 为 sync.Once + Lua 比对 token 后删键。
- process 驱动：`TestProcessLockExcludesConcurrentCallers`（16 goroutine × 50 轮同争 "job"，临界区最大并发 == 1）、`TestProcessLockWaitsUntilRelease`（阻塞等待持有者释放后获取）、`TestProcessTTLFreesWithoutRelease`（ttl 到期免 release 放锁，过期者晚 release 不扰动后继）、`TestProcessTryLockCompetes`、`TestProcessLockContextDoneWhileWaiting`、`TestProcessRejectsInvalidArguments`。
- redis 驱动（miniredis）：`TestRedisLockExcludesSecondAcquirer`（同服务器第二实例同样被拒——跨进程语义）、`TestRedisTryLockCompetes`（16 并发 TryLock 恰 1 胜者，15 个 ErrLocked）、`TestRedisTTLFreesLock`（FastForward 越过 ttl 键过期，后继可取，过期者晚 release 无害）、`TestRedisReleaseChecksToken`（foreign token 下 release 不删键；token 匹配时正常删键）、`TestRedisLockWaitsUntilRelease`、`TestRedisLockContextDoneWhileWaiting`、`TestRedisCustomPrefixIsolatesKeys`、`TestRedisRejectsInvalidArguments`。
- config + Application：`config.Mutex{Driver}`（yaml `mutex`，默认 `process`；`TestLoadMutexSection`/`TestMutexDefaults`/`TestMutexEnvOverrides`，`MUTEX_DRIVER` 环境覆盖自动生效）；`app.Mutex()`/`app.SetMutex()` 懒初始化组件（`TestApplicationMutexComponent`）；redis 驱动走 26 共享连接（`TestApplicationMutexRedisUsesSharedConnection`，键落在 `goyii:mutex:` 前缀）；默认 process 零 redis 开销（`TestApplicationMutexDefaultProcessZeroRedisOverhead`）；未知驱动首用报配置错（`TestApplicationMutexUnknownDriverFailsOnUse`、`TestNewFromConfigDispatch`）。
- 测试摘要：`go test ./...` 全绿（23 包）；mutex+base 65 个测试全过；`go vet ./...`、`golangci-lint run ./mutex/... ./base/... ./config/...` 0 issues；gofmt 无差异。
- 说明：本机无 C 工具链，`-race` 不可用（CGO 关闭环境限制）；并发正确性以多轮 `-count=3` 重复运行覆盖。
- 文档：`docs/配置参考.md` 增 mutex 段；`docs/组件-base与config.md` 增 `app.Mutex()` 组件行与锁语义小节。
