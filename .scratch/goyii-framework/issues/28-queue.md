# 28: queue

**Status:** claimed

**What to build:** 异步队列端到端：定义 Job（`Name()` + `Execute(ctx)`，应用级注册表）→ Push（JSON 序列化载荷；options：Delay/Priority/Attempts/TTR/Backoff，默认 Attempts=1、TTR=5m、退避 `2^attempt` 秒封顶 15m）→ worker 子命令消费（listen 常驻 / run 单轮 / retry 重推 failed）。驱动 sync/db/redis（file 不做，db+SQLite 覆盖零依赖）；db 驱动自带建表迁移；reserved 超 TTR 自动回炉；重试耗尽置 failed 保留。push/exec 前后五事件走 base 事件调度。Application 懒初始化组件 + config `Queue{Driver, TableName, PollIntervalMs}` 段。

**Blocked by:** 26

- [ ] Job 接口 + 应用级注册表：push 序列化 JSON 载荷、worker 按名重建执行
- [ ] Push options：Delay/Priority/Attempts/TTR/Backoff（默认值如上，Backoff 可自定义函数）
- [ ] sync 驱动：即推即执行、失败即返错（测试用）
- [ ] db 驱动：自带建表迁移（对齐 rbac 迁移先例）；延迟、优先级、重试退避、failed 保留、超 TTR 回炉
- [ ] redis 驱动：与 db 驱动同等语义（复用共享连接）
- [ ] queue/commands 子命令 listen/run/retry（对齐 migrate/commands 挂载先例）
- [ ] 五事件 BeforePush/AfterPush/BeforeExec/AfterExec/ExecError 走 base On/Trigger
- [ ] config Queue 段 + Application 组件挂接
- [ ] 单测：延迟、优先级、退避重试、失败重推、TTR 回炉（sync 与 db[sqlite] 必测；redis 驱动测试手段实现时定并说明）
