# 03: base / config 反转（核心收口）

Status: claimed
Blocked by: 01、02（均已 resolved）

**What to build:** 所有权反转的收口大张。config 包只剩 `LoadAs[T](dir)` 与 `Env()`：三台机器（GOYII_ENV、app.yaml + web-<env>.yaml overlay、反射 env 覆盖）作用于任意项目自定义 struct；删除大一统 Config / Load / WithDefaults，此后任何框架包不得 import config 的具体配置定义。`base.New` 改为 `New(opts...)` + `base.WithApp(name, env, debug)` 身份；全部组件访问器急切化、零错误返回，未装配即访问 panic（db / redis 无兜底，其余按 spec 默认清单兜底）；Session 升格一等组件槽（默认 file），Redis 槽直收 go-redis UniversalClient。删除 `app.Config()` 访问器并机械重定向全部内部消费者：migrate / gii 改用 `app.DB()` 不再自行开连接，queue worker 用 `app.Queue()`，web 错误页与 debug 面板的 Debug/Name 改读身份字段，console serve 地址改由装配注入。examples/basic 本票只做最小编译修复（正式样板在 05）。01 / 02 的过渡等值转换与 config 包残留 section 在本票删除。

- [ ] config 包仅剩 LoadAs[T] / Env()，模块内无任何包 import 它的具体配置定义
- [ ] LoadAs 对任意项目 struct 生效：overlay 与 env 覆盖（含自定义段字段路径）有测试
- [ ] base.New(opts...) + WithApp 落地；访问器零错误；未装配 db/redis 访问 panic 有测试；默认清单（log console / cache memory / queue sync / session file / mutex process / mail log driver / i18n 空）行为有测试
- [ ] Session 槽（Session / SetSession）与 Redis 槽（SetRedis 收 UniversalClient）可用
- [ ] migrate / gii / queue-worker / web / debug / console 不再读取任何配置对象
- [ ] `go build ./...` 与 `go test ./...` 全绿；examples/basic 编译可跑
