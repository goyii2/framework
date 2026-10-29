# 组件：behaviors（常用行为）

Yii2 Behavior 动态挂接的 Go 落地：结构体内嵌 + `behaviors.Hooks` 分发器。四件——Timestamp / Blameable / Sluggable / OptimisticLock，对应 `yii\behaviors` 同名行为。

## 形态

```go
type Article struct {
    ar.Model
    behaviors.Hooks        // 唯一的 GORM 钩子入口：把生命周期事件分发给各内嵌行为
    behaviors.Timestamp    // CreatedAt/UpdatedAt time.Time
    behaviors.Sluggable    // Slug string
    behaviors.OptimisticLock // Version int64
    behaviors.Blameable    // CreatedBy/UpdatedBy int64
}
```

`Hooks` 存在的原因：Go 无法从两个内嵌结构体同时提升同名 GORM 钩子方法（歧义会被静默排除）。四个行为自身不定义钩子；模型自定义同名钩子时遮蔽 Hooks 的提升，经导出的 `DispatchCreate/DispatchUpdate/DispatchUpdated` 手动接线。字段声明权在模型作者手里——非标准列名就自带字段、自己填充。

## Timestamp

插入填 `created_at` + `updated_at`（UTC 归一），更新只刷 `updated_at`，无条件覆盖（Yii2 默认语义）。秒级变体 `behaviors.TimestampUnix`（int64 Unix 秒，同列名）。

## Blameable

经请求身份填充 `created_by` / `updated_by`（int64 数值 ID）：请求内走会话登录或 HttpBasic/Bearer 的瞬态身份，HTTP 之外走 `web.WithIdentity(ctx, identity)` 显式绑定。guest/无身份填 0（Yii2 写 null 的对应物）。字符串 ID 的用户表请自带字段。

## Sluggable

模型实现 `SlugOptions()` 声明配置：

```go
func (a *Article) SlugOptions() behaviors.SlugOptions {
    return behaviors.SlugOptions{
        Source:      "Title",     // 喂给 slug 的字段名（必填）
        Separator:   "-",         // 默认
        KeepCase:    false,       // 默认小写
        KeepUnicode: false,       // 默认丢弃非 ASCII；true 保留（"中文 标题" -> "中文-标题"）
    }
}
```

- slug 为空才生成（含插入与更新）；已存在的 slug 更新时保持稳定（URL 不变）；插入时显式 slug 被尊重。
- 唯一性：查库判重（同一连接，事务内可见未提交行），冲突追加 `-2`/`-3`；并发窗口靠列上 UNIQUE 索引兜底（迁移里建）。
- 缺 `SlugOptions()` 的模型 save 明确报错。

## OptimisticLock

`Version int64`：插入初始化为 1；更新前把版本提到 old+1 并向语句追加 `WHERE version = old`，影响 0 行即冲突：

```go
err := doc.Save()
if errors.Is(err, behaviors.ErrStaleVersion) {
    // 他人已改：记录已回滚到加载时的版本，reload 后重试
}
```

Delete 不带版本条件（删除他人改过的行不属于丢失更新）。

## 相关文档

- [组件：db 与 ActiveRecord](组件-db与ActiveRecord.md)
- [组件：session 与认证](组件-session与认证.md)（身份接缝）
