# 组件：web 路由与中间件

`web` 对应 `yii\web`：HTTP 入口 `Handler`、请求上下文 `Context`、Controller/Action 约定、显式路由 + 约定回退、Filter 族中间件、统一错误处理与反向 URL 生成。

## 请求管线

```mermaid
flowchart LR
    REQ[请求] --> PIPE["全局中间件链<br/>Session -> Language -> Csrf -> 应用自加"]
    PIPE --> MUX["ServeMux 显式绑定<br/>GET /post/{id:[0-9]+}"]
    MUX -- 未命中 --> FALL["约定回退 /{controller}/{action}<br/>RegisterController 注册表"]
    MUX --> CMW["Controller 中间件<br/>AccessControl / VerbFilter"]
    FALL --> CMW
    CMW --> ACT["Action(ctx *web.Context) error"]
    ACT -->|nil| RESP[响应已写]
    ACT -->|error| ERR["统一错误处理<br/>按 Accept / 协商格式渲染"]
```

## Handler 与路由

```go
h := web.NewHandler(app)                    // 默认装 Session + Language + Csrf 中间件

h.Get("/post/{id:[0-9]+}", ctrl.ActionView).Name("post/view")
h.Post("/post", ctrl.ActionCreate).Name("post/create")

sc := h.Ctrl(ctrl)                          // 挂到控制器：动作自动包上控制器声明的中间件
sc.Get("/site/hello", ctrl.ActionHello).Name("site/hello")

url.SetDefault(h.URLs())                    // url.To 反向生成走这个路由表
```

- 通配符支持 ServeMux 全语法，外加约束形式 `{id:[0-9]+}`（框架在分发时校验）。
- 已显式绑定的路径收到未绑定动词 -> `405 + Allow`；其它未命中走约定回退（`web.DisableConventionFallback` 可关）。
- 命名路由供反向生成：`url.To("post/view", url.P{"id": 7})` -> `/post/7`；未命名的 route id 按 `/<name>` 约定拼装。

## Context 与 Action

```go
type PostController struct{ web.Controller }   // 内嵌即成控制器，ActionXxx 方法可路由

func (c *PostController) ActionView(ctx *web.Context) error {
    id, err := ctx.ParamInt("id")              // 路径参数
    if err != nil { return web.NotFound("post not found") }
    post, err := ar.FindByID[Post](ctx.Context(), id)
    ...
    return ctx.JSON(post)                      // 或 ctx.Render / Redirect
}
```

`web.Context` 承载单请求的全部状态：`Request()/Response()`、`Param/ParamInt/ParamInt64`、`JSON/JSONStatus/Redirect`、`Render/RenderStatus`（templ 页面，见 [组件：view 与 templ](组件-view与templ.md)）、`CsrfToken()`、`Session()`、`User()`、`App()`、`ResponseFormat()`（协商出的格式）。action 返回 error 即交给统一错误处理。

## 中间件（Filter 族）

全局链经 `h.Use(...)` 追加；单控制器经实现 `Middleware() []web.Middleware` 只包自己的动作：

| 中间件 | 用途 | 要点 |
|---|---|---|
| `web.AccessControl` | 角色授权 | 规则序评估，首条命中即决；`?`=guest、`@`=登录、其余为 RBAC 项（接 AuthManager 后端）；默认拒 |
| `web.VerbFilter` | 动词限制 | 命中规则外的动词 `405 + Allow` |
| `web.ContentNegotiator` | 内容协商 | 响应按 Accept 选格式，请求体 Content-Type 白名单（415） |
| `web.Csrf` | 双提交 cookie | 安全请求种 `_csrf` cookie；写请求须回同值（头 `X-Csrf-Token` 或表单字段 `_csrf`）；`web.disableCsrf` 可全局关 |
| `web.Cors` | 跨域 | 预检与常规 CORS 头 |
| `web.HttpCache` | 协商缓存 | ETag / Last-Modified |
| `web.RateLimiter` | 限流 | 按键限速 |
| `web.HttpBasic` / `web.HttpBearer` | 无状态认证 | 解出瞬态 Identity（不落会话） |

```go
func (pc *PostController) Middleware() []web.Middleware {
    return []web.Middleware{
        web.AccessControl{Auth: pc.Auth, Rules: []web.AccessRule{
            {Allow: true, Roles: []string{"?", "@"}, Verbs: []string{http.MethodGet}},
            {Allow: true, Roles: []string{"post/delete"}, Verbs: []string{http.MethodDelete}},
        }}.Middleware(),
    }
}
```

## 统一错误处理

HTTP 语义错误用构造函数表达，action 直接返回：

```go
return web.NotFound("post not found")      // 404
return web.BadRequest("malformed body")    // 400
return web.Unauthorized("...")             // 401
return web.Forbidden("...")                // 403
return web.ServerError("...")              // 500
```

渲染格式跟随协商：JSON 客户端得到 `{"name":"Not Found","message":...,"status":404}`，HTML 得到错误页（可在视图注册表用保留名 `error/<status>`、`error` 覆盖）。action panic 被恢复为 500 并记日志。

## 反向 URL 生成

```go
url.To("post/view", url.P{"id": strconv.FormatInt(post.ID, 10)})  // -> /post/26
url.To("post/index", url.P{"page": "2", "sort": "-id"})           // 冗余参数成查询串
```

生成失败（命名路由缺路径参数）返回 `"#"`，与 Yii2 的 cannot-create 标记一致。

## 相关文档

- [组件：session 与认证](组件-session与认证.md)
- [组件：rbac](组件-rbac.md)
- [组件：view 与 templ](组件-view与templ.md)
