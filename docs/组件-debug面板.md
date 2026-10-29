# 组件：debug 面板

开发期的请求内省工具（`debug` 包）：每个请求落一条 JSON 记录（请求/路由匹配、日志、SQL、时间线、Session/User、配置摘要-脱敏、邮件七面板），内嵌 `/debug` 页面浏览历史与详情，成功 HTML 响应页底注入 toolbar。对应 `yii\debug`。

## 挂载

```go
handler := web.NewHandler(app)
// ...随后、在其他全局中间件之前：
panel := debug.New(app)          // 配置都可选，见下
panel.Mount(handler)             // 仅 app.Debug=true 时生效，生产可无条件调用
```

挂载要求（注释同 `Mount` 文档）：

- 采集中间件必须位于会写响应的中间件之外——`NewHandler` 之后、其他 `Use` 之前挂载即可；
  PageCache 等 Filter（控制器/模块级）因此运行在采集层**内层**，缓存页保持干净、回放页拿到当次请求的新 toolbar。
- 挂载时会把日志根级别降到 `debug` 并绑定一个按请求路由的内存 Target：查询与 profile 记录走 debug 级，面板只能看到 logger 分发出去的记录；原有 Target 的自身过滤不受影响。装了自定义 log 组件（`SetLog` 外来实现）时日志面板为空。

## 配置（全部可选）

```go
panel := &debug.Panel{
    Dir:        "runtime/debug", // 记录目录，默认 runtime/debug
    KeepFiles:  200,             // 保留最近 N 条，超量淘汰最旧，默认 200
    AllowCIDRs: nil,             // /debug 页面访问网段，默认仅 127.0.0.1/::1；配置后替换默认
}
```

`AllowCIDRs` 接受 CIDR 块或裸 IP（`"10.0.0.0/8"`、`"192.168.1.5"`）；配置后 loopback 不再隐含放行。访问控制只约束面板页面，不影响业务路由。

## 采集内容

| 面板 | 来源 |
| --- | --- |
| 请求 / 路由匹配 | method/path/query/remote、匹配的 ServeMux pattern 与通配参数、最终 status/耗时/错误 |
| 日志 | 该请求发出的全部 log 记录（按发出 goroutine 归属，并发请求互不串线） |
| SQL | `db` 类别记录中的语句/耗时/行数（GORM 适配器产生） |
| 时间线 | `log.Profiler` 的 profile begin/end 配对 |
| Session / User | 请求结束时会话快照与登录态 |
| 配置摘要 | 各组件选择一览，DSN 与密码脱敏（`***`） |
| 邮件 | log 驱动 mailer 记录的完整发信内容 |

每请求一个 JSON 文件落 `Dir`，文件名 = UTC 时间戳 + 序列（即详情页 `?id=`）；写满 `KeepFiles` 后淘汰最旧。

## toolbar 注入

仅对 **200 且 `Content-Type: text/html`** 的完整响应注入（复用 `web.CaptureWriter` 捕获后改写，注入在最后一个 `</body>` 前；无 `</body>` 则追加）。不注入：JSON 等非 HTML、错误页（统一错误渲染在采集层之外）、HEAD、流式（Flush 后响应已直通，不缓冲不注入）、面板自身页面。

## 相关文档

- [组件：web 路由与中间件](组件-web路由与中间件.md)（`web.CaptureWriter`、PageCache）
- [组件：log](组件-log.md)（Target/Filter 机制、profile 计时）
