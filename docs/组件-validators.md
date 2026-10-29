# 组件：validators

场景化验证，验证引擎为 zog（`github.com/Oudwins/zog`）。模型以 `Rules()` 声明「场景 -> schema」映射，保存链路按 Insert/Update 场景自动触发；错误形态统一为 `字段 -> []消息`。

## 模型规则

```go
func (p *Post) Rules() validators.RuleSet {
    title := func() *z.StringSchema[string] {
        return z.String().
            Required(z.Message("title cannot be blank")).
            Min(3, z.Message("title must be at least 3 characters"))
    }
    return validators.RuleSet{
        validators.ScenarioInsert: z.Struct(z.Shape{
            "title":  title(),
            "status": z.String().OneOf([]string{"draft", "published"},
                z.Message("status must be one of: draft, published")),
            "body": z.String().Max(10000, z.Message("body must be at most 10000 characters")),
        }),
        validators.ScenarioUpdate: z.Struct(z.Shape{
            "title": title(), // 更新只复检部分字段
        }),
    }
}
```

- `z.Message("...")` 的文案即消息 ID：源语言（en-US）原文，可被 [i18n](组件-i18n.md) 翻译；zog 默认消息同样进入翻译链。
- 需要查库的规则（唯一性）在 `TestFunc` 里用 `ar.New[T](u.Context())` 走记录绑定的 ctx。

## 触发路径

```go
// 1. 手动验证（返回 *validators.Errors，nil 即通过）
if verrs := validators.Validate(ctx, model, validators.ScenarioInsert); verrs != nil {
    return verrs.Fields        // map[字段][]消息
}

// 2. 保存链路自动触发：Save() 按主键是否为零选 Insert/Update 场景，
//    不通过即拒写，错误经 errors.As 取出
err := post.Save()
var verrs *validators.Errors
if errors.As(err, &verrs) {
    // verrs.Fields: map[字段][]消息（已按请求语言翻译）
}
```

Action 里的 422 形态：

```go
return c.JSONStatus(http.StatusUnprocessableEntity, verrs.Fields)
// -> {"title":["title must be at least 3 characters"],"status":["status must be one of: draft, published"]}
```

## 消息翻译联动

`validators.Validate` 在 flatten 边界统一翻译：消息 ID 查 i18n 的 `validators.MessageCategory`（默认 `"app"`）类别，未命中回原文。CLI / 裸 ctx 自动降级为不翻译。切换类别：

```go
validators.MessageCategory = "model"   // 启动期设置一次
```

## 相关文档

- [组件：db 与 ActiveRecord](组件-db与ActiveRecord.md)
- [组件：i18n](组件-i18n.md)
