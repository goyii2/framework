# 组件：rbac

自建轻量 DbManager，四表 Yii2 语义（auth_item / auth_item_child / auth_rule / auth_assignment），GORM 访问，item 图与分配进程内缓存。对应 `yii\rbac`。

## 装配

```go
authConn, _ := app.DB()
auth, err := rbac.NewDbManager(authConn.DB /*, rules...*/)
handler.SetAuthManager(auth)
```

接上后端后，`web.User.Can` 与 `web.AccessControl` 的具体角色/权限名都问管理器；`?`（guest）与 `@`（登录）语义不变。未接时回落到 Identity 自带角色列。建表用框架迁移 `rbac.NewMigration()`。

## 层级与判定

item 分 role 与 permission，`AddChild(parent, child)` 组成 DAG；判定沿 parents 上溯——持有 admin 即间接持有其全部子孙：

```go
auth.AddItem(rbac.Item{Name: "admin", Type: rbac.TypeRole})
auth.AddItem(rbac.Item{Name: "editor", Type: rbac.TypeRole})
auth.AddItem(rbac.Item{Name: "post/delete", Type: rbac.TypePermission})
auth.AddChild("admin", "editor")            // admin 继承 editor 的一切
auth.AddChild("admin", "post/delete")

auth.Assign("admin", userID)                // 按名分配给用户（字符串 ID）
auth.Revoke("admin", userID)
auth.RolesByUser(userID)                    // 直接分配的角色（不含继承）
auth.Children("admin")                      // 直接子项
```

```go
ok, err := auth.CheckAccess(userID, "post/delete", rbac.Params{"post": p})
// action 内的便捷判定（错误静默为 false；要报错走 AccessControl 中间件）：
if c.User().Can("post/delete") { ... }
```

环检测在 AddChild 时做（自环/回边/传递回边拒绝）；菱形继承合法。

## 规则（Rule）

规则只在代码里定义，库中仅存名字——Go 没有可序列化执行：

```go
type onlyOwnPost struct{}
func (onlyOwnPost) Name() string { return "onlyOwnPost" }
func (onlyOwnPost) Execute(userID string, params rbac.Params) bool {
    post, _ := params["post"].(*Post)
    return post != nil && strconv.FormatInt(post.AuthorID, 10) == userID
}

auth, _ := rbac.NewDbManager(conn, onlyOwnPost{})
// item 的 RuleName 挂上后，判定路径经过它时执行；false 否决该项及其授予
```

启动时注册表与库内 `auth_rule` 名字对齐：有名无实现（或有实现未入库）在加载时报错点名，不静默降级。

## 缓存失效

- 结构变更（AddItem/AddChild/RemoveItem/RemoveChild）：整体失效图 + 分配缓存，同一管理器实例的下一次判定立即可见。
- 分配变更（Assign/Revoke）：精确驱逐该用户条目。
- 进程内缓存：跨进程部署时，其它进程的结构变更要等该进程自身下一次失效或重启——选定架构的既定代价。

## 授权与页面联动

- 路由级：`web.AccessControl{Auth: auth, Rules: ...}`（角色名走后端，后端故障报 500 不冒充 403）。
- 页面/菜单级：`c.User().Can("post/create")` 决定菜单项与按钮可见性——`examples/basic` 的导航即此用法。

## 相关文档

- [组件：session 与认证](组件-session与认证.md)
- [组件：web 路由与中间件](组件-web路由与中间件.md)
