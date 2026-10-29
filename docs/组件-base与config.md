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
| `app.Cache() cache.Cache` | 缓存 | memory / file / redis 按 `cache.driver` |
| `app.Log() base.Log` / `app.Logger() *log.Logger` | 日志 | 按 `log` 段的 Target 列表 |
| `app.I18n() *i18n.MessageSource` | 翻译源 | 读 `i18n.messagesDir` 目录 |
| `app.Mailer() mail.Mailer` | 邮件 | log 驱动（smtp 按 `mail` 段） |

每个组件都有对应 `SetXxx` 替换位（如 `app.SetI18n(i18n.NewMessageSource(embedded))`），测试注入 fake 或换实现都走它；传 nil 还原懒初始化默认。

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
