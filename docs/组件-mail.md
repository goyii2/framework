# 组件：mail

Mailer 抽象 + 两个实现：log 驱动（开发默认，完整消息落应用日志）与 SMTP 驱动（生产投递）。对应 `yii\swiftmailer` 的使用面。

## 发送

```go
msg := mail.NewMessage().                       // 链式构建，对齐 Yii2 消息接口
    SetFrom("goyii basic <no-reply@example.com>").   // 裸地址或 "Name <addr>" 均可
    SetTo("ada@example.com", "bob@example.com").
    SetCc("lead@example.com").
    SetSubject("Welcome aboard").
    SetTextBody("plain body").
    SetHTMLBody("<p>rich body</p>").
    AttachFile("report.pdf")                    // 或 AttachContent(name, data, contentType)

if err := c.App().Mailer().Send(c.Context(), msg); err != nil {
    return err                                   // 构造期错误（坏地址等）也在这里浮出
}
```

- `Set*` 语义为整体替换（同 Yii2 setTo）；错误不打断链——首个非法值记入 `msg.Err()`，由 Send 统一回报。
- `mail.from` 配置为缺省发件人（消息未设 From 时填充）。
- log 驱动记录完整消息：全部信头、text/html 双正文、附件摘要（名称/类型/字节数；字节本体不落日志）。`[mail]` 分类，走应用日志。

## 配置

```yaml
mail:
  driver: log        # log | smtp（默认 log）
  from: "myapp <no-reply@example.com>"
  smtp:
    host: smtp.example.com
    port: 587            # 默认 587；encryption: smtps 时 465
    username: alerts
    password: s3cret     # 用户名非空即启用 AUTH PLAIN
    encryption: starttls # none | starttls | smtps（别名 tls/ssl）；默认 starttls
```

SMTP 驱动每次 Send 独立连接（拨号 -> 投递 -> 关闭），并发安全。配置错误（未知 driver、缺 host）不在装配期炸，而是每次 Send 都报配置错——绝不静默降级到 log 假装发送成功。

## 测试注入

Mailer 是接口（`Send(ctx, *Message) error`），单测注入 fake：

```go
app.SetMailer(fake)          // 捕获 *Message 断言内容
defer app.SetMailer(nil)     // 还原懒初始化默认
```

## 相关文档

- [快速上手](快速上手.md)（examples/basic 首页的测试邮件按钮）
- [配置参考](配置参考.md)（`mail` 段全字段）
