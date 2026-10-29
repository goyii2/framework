# 组件：session 与认证

会话组件（file/redis 后端）承载登录态；`web.User` 是请求级认证状态；Csrf 双提交保护写请求；HttpBasic/HttpBearer 提供无状态认证。对应 `yii\web\session` 与 `yii\web\User`。

## 会话

`web.NewHandler` 默认安装会话中间件；配置走 `session` 段：

```yaml
session:
  name: goyii-session   # cookie 名
  backend: file         # file | redis
  path: runtime/session # file 后端目录
  timeout: 1440         # 秒（默认 24h）
  redis: { addr: "127.0.0.1:6379" }
```

action 内经 `ctx.Session()` 读写（`Get/GetString/Set/Remove/Destroy/RegenerateID`）。会话在请求收尾按需持久化，登录时轮换 id（防会话固定）。

## Identity 与登录

Identity 是最小接口，账号模型实现即可：

```go
type User struct { ... }

func (u *User) GetID() string      { return strconv.FormatInt(u.ID, 10) }
func (u *User) GetName() string    { return u.Username }
func (u *User) GetRoles() []string { return []string{u.Role} }
```

装配时接上「会话存的 id -> 账号行」解析器：

```go
handler.SessionManager().IdentityFinder = func(id string) (web.Identity, bool) {
    uid, err := strconv.ParseInt(id, 10, 64)
    if err != nil { return nil, false }
    user, err := ar.FindByID[User](appCtx, uid)
    if err != nil { return nil, false }   // 失效登录降级为 guest
    return user, true
}
```

```go
// action 内
identity := c.User().Identity()     // nil = guest，懒解析每请求一次
c.User().IsGuest()
c.User().Login(user)                // 绑定会话 + 轮换 id
c.User().Logout()                   // 销毁会话与 cookie
c.User().Can("post/delete")         // 问 RBAC 后端（见 组件：rbac）
```

HTTP 之外的代码（console、后台任务）用接缝声明当前身份：

```go
ctx = web.WithIdentity(ctx, actor)          // 显式绑定优先
who := web.IdentityFromContext(ctx)         // Blameable 等从这里取操作人
```

## 无状态认证

```go
h.Use(web.HttpBasic{Realm: "api", Authenticate: func(user, pass string) (web.Identity, bool) {
    // 校验用户名口令，返回 Identity；ok 为 false 即拒绝
}}.Middleware())

h.Use(web.HttpBearer{Authenticate: web.StaticBearerTokens(map[string]web.Identity{
    "token-1": apiIdentity,
})}.Middleware())
// 两者都建立「瞬态 Identity」：本请求有效，不落会话
```

## Csrf（双提交 cookie）

`web.NewHandler` 默认安装；安全请求种 `_csrf` cookie（HttpOnly），写请求必须回传同值——头 `X-Csrf-Token` 或表单字段 `_csrf`。表单渲染内嵌：

```html
<input type="hidden" name="_csrf" value="{{ .Csrf }}">
```

action 侧 `c.CsrfToken()` 取当前 token。纯 API（每请求自带认证）可全局关闭：`web.disableCsrf: true`。查询串里的 token 永不接受（会经 Referrer 泄漏）。

## 相关文档

- [组件：rbac](组件-rbac.md)
- [组件：web 路由与中间件](组件-web路由与中间件.md)
