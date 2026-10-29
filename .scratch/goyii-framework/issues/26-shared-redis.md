# 26: redis 共享连接

**Status:** claimed

**What to build:** 多处 redis 使用（cache、session，及后续 mutex/queue）共享一条连接：config 增设顶层可选 Redis 段，`Cache.Redis` / `Session.Redis` 降级为可选覆盖（空则继承顶层，非空覆盖）；Application 懒初始化共享 redis 连接组件，redis 后端从它取连接、不再各自建连。旧配置不改也能跑（向后兼容），未配置 redis 时零开销。本票是 mutex（27）与 queue（28）的装配前置。

**Blocked by:** None (can start immediately)

- [ ] config 顶层可选 Redis 段；Cache.Redis/Session.Redis 空则继承顶层、非空覆盖，旧配置行为不变
- [ ] Application 共享 redis 连接组件懒初始化；cache/session 的 redis 后端复用同一连接（不重复建连）
- [ ] 未配置 redis 且无人使用时零开销
- [ ] 单测覆盖：继承/覆盖解析、各处拿到同一 client 实例
