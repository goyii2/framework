# 21: mail 组件

Status: resolved

**What to build:** Mailer 抽象 + SMTP 实现：消息构造（收发件人/主题/正文/附件）与发送接口，经 Application 取用；开发态提供 log Mailer（不外发，完整消息落日志），生产态 SMTP。

**Blocked by:** 02

- [x] SMTP 实现对本地桩服务器完成发送验证
- [x] log Mailer 在开发态记录完整消息内容
- [x] Mailer 为接口，单测可注入 fake

## Comments

**落地形态**（main `e808f60`，分支 feat/mail 已 ff 合入并删除）：

- **Message**（`mail.NewMessage()`）：链式 Set* 构建器对齐 Yii2 消息接口心智——`SetFrom/SetReplyTo/SetTo/SetCc/SetBcc/SetSubject/SetTextBody/SetHTMLBody/AttachContent/AttachFile`，getter `From()/To()/Cc()/Bcc()/ReplyTo()/Recipients()/Subject()/TextBody()/HTMLBody()/Attachments()`。地址入参同时接受裸地址与 `Name <addr>`（net/mail 解析）；Set* 语义为整体替换（同 Yii2 setTo）。**API 形态记录——链式而非选项式**：邮件消息天然是一串赋值表达式，链式保持单表达式可读；错误不打断链——首个非法地址/不可读附件文件记入 `Err()`，由 Mailer.Send 统一回报，避免每步判 err 的锯齿代码。
- **Mailer 接口 + 两实现**：`Send(ctx, *Message) error`。`LogMailer`（默认）经 `mail.LogSink`（`Info(msg, args...)` 最小契约，框架 log.Logger 天然满足）把 `Message.Render()` 完整落日志——含 From/Reply-To/To/Cc/Bcc/Subject 头、text/html 双正文、附件摘要（名称/类型/字节数；**字节本体不落日志**，base64 内容进日志无开发价值，属「完整消息内容」的刻意取舍）。`SmtpMailer` 每次 Send 独立连接（拨号->投递->关闭），无连接池状态、并发安全。
- **SMTP 库选型记录——`github.com/wneessen/go-mail v0.8.1`**：候选三方对比：`net/smtp` 冻结维护（只收安全修复）；`gopkg.in/mail.v2` 2019 年停在 v2.3.1，作者已结束维护；go-mail 活跃维护（v0.8.x）、零外部依赖（自身 go.mod 无任何 require）、MIT、RFC 5322/MIME 构建完整（multipart/alternative + multipart/mixed、附件 base64、编码字头）、ctx 感知（DialAndSendWithContext）、STARTTLS/隐式 TLS/无加密三态策略与 PLAIN/LOGIN 等认证。自研 MIME 层省不掉多少代码却要独自背 RFC 兼容性，选库。
- **TLS 策略**：`encryption: none | starttls | smtps`（默认 starttls——现代提交服务器要求之；`tls`/`ssl` 作为 Yii2 swiftmailer 拼写别名收下），端口缺省 587（smtps 时 465）；用户名非空即启用 AUTH PLAIN。smtps 用隐式 TLS 拨号（go-mail WithSSL，库自身跳过二次 STARTTLS）。
- **config**：`mail` 段 `{driver: log|smtp, from, smtp: {host, port, username, password, encryption}}`，`WithDefaults` 仅补 `driver=log`（smtp 的端口/加密缺省在 mail 包归一化）；env 覆盖 `MAIL_DRIVER`/`MAIL_FROM`/`MAIL_SMTP_HOST|PORT|USERNAME|PASSWORD|ENCRYPTION` 由现有反射机制自动获得。`mail.from` 为缺省发件人（消息未设 From 时填充，浅拷贝不改调用方消息）。
- **失败面记录**：配置错误（未知 driver、smtp 缺 host、坏 encryption/坏 from）不在装配期炸（组件持有器工厂不容失败），而是返回「每次 Send 都报配置错」的 mailer——错误在首次使用处 surfaced，绝不静默降级到 log mailer 假装发送成功。
- **base**：`Application.Mailer()/SetMailer()`（component 持有器），默认 `mail.NewFromConfig(cfg.Mail, loggerOf(log).Category("mail"))`——log mailer 输出走应用日志 mail 分类。

**验收证据**（合并后 main `e808f60` 上，CGO 经 scoop mingw gcc 启用）：

- `go build ./...` / `go vet ./mail/ ./base/ ./config/` / `gofmt -l`（mail/base/config）全部无输出；`go test ./...` 全仓 22 包 1195 项全绿。
- `CGO_ENABLED=1 go test -race -count=1 ./mail/ ./base/ ./config/` 三包 ok、无 data race——含 `TestLogMailerConcurrentSend`（32 goroutine × 20 消息共享一个 mailer+sink）与 `TestApplicationMailerConcurrentAccess`（64 goroutine 懒初始化 + SetMailer 替换）。
- **SMTP 对本地桩服务器完成发送验证**：`smtp_test.go` 内置 `stubSMTPServer`（net.Listener 起真 TCP 桩，完整对话 220/250/354/221 + 可配 AUTH 广告/RCPT 拒收）。`TestSmtpMailerDeliversThroughStubServer` 走真协议栈：EHLO 握手、`MAIL FROM:<ada@example.com>`、To/Cc/Bcc 四个 `RCPT TO` 逐一入信封、DATA、QUIT 全序断言；DATA 体断言 multipart/mixed 套 multipart/alternative、text/plain + text/html 双部件、附件 `application/pdf; name="report.pdf"` + base64 字节本体，且 **Bcc 不出现在投递数据**（仅信封，与真 SMTP 一致）。另有 AUTH PLAIN 凭据解码断言（桩收到的 `AUTH PLAIN` 载荷解出 alerts/s3cret）、RCPT 550 拒收失败、starttls 缺 STARTTLS 广告时拒发且无 MAIL FROM、端口不通时报错带主机名、HTML-only 消息投递、构造校验（缺 host/未知 encryption）与别名归一化（tls/ssl/大小写）。
- **log Mailer 完整消息内容**：`TestLogMailerRecordsCompleteMessage` 断言单条日志含 From/To/Bcc/Subject 头、text 与 html 双正文、附件摘要行；`mail/mailer_test.go` 经框架 log.Logger（内存 target）验证记录真实落到 log 组件；`base/mail_test.go` 端到端——未配置应用 `app.Mailer().Send(...)` 后 file target 里出现 `[mail]` 分类 + `Subject: Welcome aboard`。
- **接口可注入 fake**：`TestMailerContractAcceptsFake`（mail 包内 `var m Mailer = &fakeMailer{}` 捕获指针级同一消息）+ `base/mail_test.go` `TestApplicationSetMailerFake`（`app.SetMailer(fake)` -> Send -> 捕获 1 条；`SetMailer(nil)` 还原懒初始化）；配置错误经 `TestNewFromConfigFailuresSurfaceAtSend`（4 用例）验证在 Send 处 surfaced。
- **config**：`TestLoadMailSection`（yaml 全字段解析）、`TestMailDefaults`（driver 缺省 log、不发明其它值）、`TestMailEnvOverrides`（MAIL_DRIVER/MAIL_FROM/MAIL_SMTP_HOST/PORT/ENCRYPTION）。

**遗留**：隐式 TLS（smtps:465）与 STARTTLS 升级未做真实 TLS 握手测试（桩服务器为明文；TLS 层由 go-mail 承担，本票只验到协议对话与策略门禁）；附件流式发送未做（AttachContent 全量入内存，框架应用邮件体量下可接受）；examples/basic 未接邮件示例（P1 集成票一并考虑）。
