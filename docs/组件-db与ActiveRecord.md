# 组件：db 与 ActiveRecord

`db` 管理 GORM 连接，`db/ar` 是泛型 ActiveRecord：模型内嵌 `ar.Model` 获得 Save/Delete/Reload/Validate，查询走链式 `ar.Query[T]`。DML 归 GORM，DDL 归 [migrate](组件-migrate.md)，两者共用 DSN。

## 连接

```go
// 经 Application（推荐）：app.DB() 懒初始化并缓存
conn, err := app.DB()

// 直接建立（测试、工具）
conn, err := db.New(db.Config{Driver: "sqlite", DSN: "runtime/app.sqlite"})
defer conn.Close()
```

驱动：`sqlite`（纯 Go，无 CGO）/ `mysql` / `postgres` / `sqlserver`。查询日志经 `log.gorm` 段接入框架 log（慢查询 WARN）。

## 模型

```go
type Post struct {
    ar.Model                                        // Save/Delete/Reload/Validate 与 ctx 绑定
    ID       int64  `gorm:"column:id;primaryKey"`
    Title    string `gorm:"column:title"`
    AuthorID int64  `gorm:"column:author_id"`
    Author   *User  `gorm:"foreignKey:AuthorID"`    // 关联，With("Author") 预加载
}
func (Post) TableName() string { return "post" }
```

模型可同时内嵌行为结构体（Timestamp 等），见 [组件：behaviors](组件-behaviors.md)。

## 查询链

每次链式调用返回新 Query（原查询不变），词汇对齐 Yii2 的 ActiveQuery：

```go
// New[T](ctx) 从 ctx 解析连接（事务内则加入事务）
posts, err := ar.New[Post](ctx).
    Where("status = ?", "published").       // SQL 片段 + ?，或 map/struct 等值
    With("Author").                          // 预加载关联（点号嵌套："Author.Posts"）
    OrderBy("id DESC").
    Limit(10).Offset(20).
    All()                                    // []*Post，各自绑定 ctx

one, err := ar.New[Post](ctx).Where("id = ?", id).One()   // 未命中报 gorm.ErrRecordNotFound
n, err := ar.New[Post](ctx).Where("status = ?", "draft").Count()
post, err := ar.FindByID[Post](ctx, id)                    // 单列主键便捷入口
fresh := ar.New[Post](ctx).New()                           // 绑定 ctx 的零值记录，填字段后 Save
```

## 保存 / 删除 / 重载

```go
post := ar.New[Post](ctx).New()
post.Title, post.Status = "Hello", "published"
err := post.Save()          // 主键为零 -> INSERT，否则 UPDATE
                            // 保存前自动按场景跑 Rules() 验证（见 组件：validators）

post.Title = "Hello v2"
err = post.Save()           // UPDATE；受影响 0 行报 ErrRecordNotFound 语义

err = post.Reload()         // 从库刷新字段，丢弃内存修改
err = post.Delete()
```

事务内取得的记录保持事务绑定，事务结束后继续用该实例操作即回到普通连接：

```go
err := ar.Transaction(ctx, func(ctx context.Context) error {
    // 这里的 ctx 已带上事务；ar.New[T](ctx) 与 New().New() 全部在事务内
    if err := post.Save(); err != nil { return err }   // 返回 error 即回滚
    return audit.Save()
})
```

## 生命周期钩子

GORM 钩子承担 Yii2 的 beforeSave/afterSave/afterDelete：模型实现 `BeforeSave(tx *gorm.DB) error` 等标准 GORM 钩子方法即可。行为包（Timestamp/Blameable/Sluggable/OptimisticLock）经内嵌 `behaviors.Hooks` 分发器接入，见 [组件：behaviors](组件-behaviors.md)。

## 相关文档

- [组件：validators](组件-validators.md)
- [组件：dataprovider](组件-dataprovider.md)
- [组件：migrate](组件-migrate.md)
