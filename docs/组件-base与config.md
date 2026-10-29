# 组件：base 与 config

`base` 是框架的应用层：强类型组件容器 `Application`、Module 注册表、事件调度。`config` 是配置装载：值走 yaml + 环境变量覆盖，行为走代码组装。

## Application：强类型组件容器

对应 Yii2 的 `yii\base\Application`，但不做运行时服务定位——组件是具名字段，经方法访问，懒初始化：

```go
cfg, err := config.Load("config")     // 读 config/app.yaml（+ web-<env>.yaml 覆盖层 + env 覆盖）
app := base.New(cfg)
base.SetDefault(app)                  // 无请求上下文的代码（console/后台）经 FromContext 解析到它
```

组件访问器与替换位（全部懒初始化，首次访问才构建）：

| 方法 | 组件 | 默认实现 |
|---|---|---|
| `app.DB() (*db.Conn, error)` | 数据库连接 | 按 `db.driver/dsn` 建立 |
| `app.Redis() redis.UniversalClient` | 共享 redis 连接 | 按顶层 `redis` 段；cache/session 的 redis 后端共用（各自 `redis:` 子段非空则另开专用连接，`app.RedisFor(section)`） |
| `app.Cache() cache.Cache` | 缓存 | memory / file / redis 按 `cache.driver` |
| `app.Mutex() mutex.Mutex` | 互斥锁 | process / redis 按 `mutex.driver`（redis 用共享连接） |
| `app.Queue() *queue.Queue` | 异步队列 | sync / db / redis 按 `queue.driver`（redis 用共享连接） |
| `app.Log() base.Log` / `app.Logger() *log.Logger` | 日志 | 按 `log` 段的 Target 列表 |
| `app.I18n() *i18n.MessageSource` | 翻译源 | 读 `i18n.messagesDir` 目录 |
| `app.Mailer() mail.Mailer` | 邮件 | log 驱动（smtp 按 `mail` 段） |

每个组件都有对应 `SetXxx` 替换位（如 `app.SetI18n(i18n.NewMessageSource(embedded))`），测试注入 fake 或换实现都走它；传 nil 还原懒初始化默认。

## Mutex：按名互斥锁

对应 Yii2 的 `yii\mutex`，但**不可重入**（Go 下按名可重入不安全也不惯用，不复刻）：

```go
release, err := app.Mutex().Lock(ctx, "report-generator", 30*time.Second)
if err != nil { return err }
defer release()

// ...临界区...
```

- `TryLock` 同形：名被占立即返回 `mutex.ErrLocked`，不等待。
- `ttl` 到期锁自动放（redis 服务端键过期 / process 定时器），不自动续期；持有方崩溃也不会永久堵死他人。
- 长于 `ttl` 的工作分段重取：锁定 -> 做一段有界工作 -> release -> 再取锁继续；分段点也是领导权可能易手处。
- release 幂等，TTL 过期后调用也无害——redis 驱动按获取令牌校验后才删键，晚到的 release 不会误删后来者的锁。
- process 驱动是进程内跨 goroutine 的按名锁；redis 驱动（`mutex.driver: redis`）跨进程，走顶层共享 redis 连接。


## Queue：异步队列

对应 Yii2 的 `yii\queue`，但任务代码不随队列出进程——push 只存**注册名 + JSON 载荷**，worker 按名从应用级注册表重建后执行：

```go
// 任务：具名 + 载荷走导出字段（JSON 序列化）
type EmailJob struct {
    To      string `json:"to"`
    Subject string `json:"subject"`
}
func (j *EmailJob) Name() string            { return "email" }
func (j *EmailJob) Execute(ctx context.Context) error { /* 发信 */ return nil }

// 启动时注册（console 与 web 共用同一 app）
app.Queue().Jobs().Register(&EmailJob{})

// 请求里推送：可延迟、可优先级、可重试
app.Queue().Push(ctx, &EmailJob{To: "a@b.c", Subject: "hi"},
    queue.WithDelay(time.Hour),      // 默认立即
    queue.WithPriority(5),           // 默认 0，高者优先
    queue.WithAttempts(3),           // 默认 1；总尝试次数（含首次）
    queue.WithTTR(10*time.Minute),   // 默认 5m；保留超时，超时自动回炉
    queue.WithBackoff(myBackoff),    // 默认 2^attempt 秒封顶 15m
)
```

- **状态机**：waiting -> reserved -> done；失败且还有尝试次数 -> 按退避延迟重回 waiting；尝试耗尽 -> failed 保留。reserved 超 TTR（worker 崩溃）自动回炉 waiting。
- **worker**：`queue/listen` 常驻（SIGINT/SIGTERM 优雅退出）、`queue/run` 单轮（cron 式）、`queue/retry` 重推 failed（尝试次数归零）——`queue/commands` 提供，挂载方式同 migrate：`console.AddCommand(queuecommands.New(app))`。
- **事件**：`queue.EventBeforePush / AfterPush / BeforeExec / AfterExec / ExecError` 五个走 `app.On` / `app.Trigger`（Data 为 `queue.PushEventData` / `queue.ExecEventData`）。
- **驱动**：`sync` 即推即执行、失败直接返错（开发/测试默认，零外部依赖）；`db` 建表存消息（`queue.NewMigration(cfg.Queue.TableName)` 并入应用迁移列表），SQLite 即可覆盖零依赖场景；`redis` 跨进程走共享连接。sync 驱动忽略 Attempts/重试——没有存储就没有重试。
- **退避**：默认 `2^attempt` 秒（attempt 为刚失败的尝试序号）封顶 15 分钟；自定义函数在 push 时解析成延迟表随消息存储，跨进程重试不依赖闭包。


## context 携带

请求作用域的一切经 `context.Context` 传递，`base.WithContext(ctx)` 把 app 放进 ctx，`base.FromContext(ctx)` 取回：

```go
// HTTP 管线自动完成；自己构造（console、后台任务）时：
ctx := app.WithContext(context.Background())
conn, err := base.FromContext(ctx).DB()
```

所有框架入口（`ar.New[T](ctx)`、`cache`、`i18n.T(ctx, ...)`）都从这个 ctx 解析 app 与请求状态（语言、当前身份）。

## 事件调度

类型化的事件，不是字符串万能总线：

```go
off := app.On(base.EventAfterRequest, func(ctx context.Context, data any) {
    // 请求收尾：访问日志、指标上报等
    defer off() // 返回的取消函数移除监听
})
app.Trigger(ctx, base.EventAfterRequest, nil)
```

内置事件：`EventBeforeRequest` / `EventAfterRequest`（HTTP 管线触发）。模型生命周期（beforeSave 等）**不走**事件——那是 ActiveRecord 的钩子机制，见 [组件：db 与 ActiveRecord](组件-db与ActiveRecord.md)。自定义事件直接声明自己的 `base.EventName` 常量再 On/Trigger。

## Module 与 Controller 注册表

Module 是带路由前缀的分区（`/admin`），可嵌套：子模块前缀逐级拼接（`/admin/report`），控制器沿模块树分层查找——约定回退按最长匹配前缀落到所属模块的注册表。组件不做模块级隔离，仍归全局 Application。

```go
mod, err := base.NewModule(app, "/admin")
app.RegisterModule(mod)                 // 顶层模块挂在应用上

report, err := base.NewModule(app, "/admin/report")
mod.RegisterModule(report)              // 子模块挂在父模块下（前缀必须恰好多一段）

routes, err := handler.Module("/admin")       // 路由挂载见 组件：web 路由与中间件
routes.Use(myMiddleware)                // 模块级中间件：作用于该模块全部路由
routes.Layout(myLayout)                 // 模块级页面框架（见 组件：view 与 templ）
routes.Get("/dash", ctrl.ActionDash).Name("dash")  // 名字自动带模块路径 -> "admin/dash"
```

注册约束：前缀全局唯一（重复注册 `ErrDuplicateModulePrefix`）；子模块前缀必须是父前缀加恰好一段（`ErrInvalidSubmodulePrefix`）；`mod.Modules()` 列直接子模块、`mod.Parent()` 回溯父模块。

Controller 注册表喂约定回退路由（`/{controller}/{action}` -> `ActionXxx`）：

```go
app.RegisterController("site", &SiteController{})
// 未显式绑定的 /site/index 请求由注册表解析到 SiteController.ActionIndex
```

模块内控制器只在该模块前缀下可解析（`/admin/user/list` 走 admin 模块的注册表），根注册表与各模块注册表互不泄露。

## config：yaml + env 双轨

「值走 yaml，行为走代码」——配置只描述值，不描述装配：

```yaml
# config/app.yaml
app:   { name: myapp, debug: true }
web:   { listen: "127.0.0.1:8080" }
db:    { driver: sqlite, dsn: runtime/my.sqlite }
```

- 环境选择：`GOYII_ENV=prod` 时叠加 `config/web-prod.yaml` 覆盖层。
- 环境变量覆盖：字段路径大写下划线（`DB_DSN`、`WEB_LISTEN`、`CACHE_DRIVER`、`MAIL_SMTP_HOST` ...），无需改 yaml 即可切换。
- 全部段落与默认值见 [配置参考](配置参考.md)。

## 相关文档

- [组件：web 路由与中间件](组件-web路由与中间件.md)
- [组件：log](组件-log.md)
- [配置参考](配置参考.md)
