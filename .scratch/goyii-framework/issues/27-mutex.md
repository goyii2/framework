# 27: mutex

**Status:** claimed

**What to build:** 按名互斥锁组件：`Lock(ctx, name, ttl) (release func(), err)` 与同形的 `TryLock`，语义不可重入（不复刻 Yii2 可重入，见 PRD 放弃清单）。process 实现=按名锁；redis 实现=SET NX PX + 唯一 token + Lua 校验释放，TTL 到期自动放、不自动续期（长持锁由调用方分段重取）。作为 Application 懒初始化组件，config 增 `Mutex{Driver}` 段（默认 process）。

**Blocked by:** 26

- [ ] Lock/TryLock 接口落地，release 幂等
- [ ] process 驱动：按名互斥、跨 goroutine 生效
- [ ] redis 驱动：SET NX PX + token；错误 token 无法释放他人锁；TTL 到期自动释放
- [ ] config Mutex 段 + Application 组件挂接（redis 复用 26 共享连接）
- [ ] 单测：互斥、TryLock 竞争、超时释放、token 校验；文档注释明确不可重入与分段续锁姿势
