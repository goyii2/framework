# 05: examples/basic 样板重写

Status: resolved
Blocked by: 03、04（均已 resolved）

**What to build:** examples/basic 重写为「项目自有配置 + 代码装配」官方样板：项目自定义 Config struct（身份字段 + 复用各组件包参数包 + 自定义 Email 段），main.go 全急切装配（含 SetSession / SetRedis）；`EMAIL_APIKEY` 环境变量覆盖与 `web-dev.yaml` overlay 各有一处真实演示；自定义组件按「项目自持 struct + 构造注入」姿势呈现（现有 auth manager 模式为正源），不出现任何框架配置残留。

- [x] 项目 Config struct 位于示例自己的 config 包；自定义 Email 段随 LoadAs 加载并可被 EMAIL_APIKEY 覆盖（有可见演示：启动日志或页面输出）
- [x] main.go 全急切装配，构造错误在 boot 期处理
- [x] overlay（app.yaml + web-dev.yaml）至少一处真实覆盖演示
- [x] 自定义组件以项目自持 + 构造注入呈现，无字符串注册表
- [x] serve / migrate / queue worker 冒烟可用；`go test ./...` 全绿

## Comments

- 2026-10-30 实施记录（分支 feat/project-owned-config-05，已 --ff-only 合入 main，commit a0dbaa9）：
  - **config 包化**：`examples/basic/config/config.go` 定义项目自有 Config（app/web 身份字段 + db/queue/cache/session/mail/log 参数包直嵌 + i18n 段 + 自定义 `Email{apiKey,sender}` 段，yaml tag 齐全）；main.go 以 `goyii.LoadAs[config.Config]("config")` 加载（框架 config 包别名 goyii，项目包持有 config 本名）。
  - **三层机器可见演示**：新增 `config/web-dev.yaml`（dev overlay 用 `email.apiKey: dev-key-from-web-dev-overlay` 覆盖 app.yaml 的 base 值）；main.go 构造 notifier 后打 boot 日志 `notifier: transactional email ready sender=... apiKey=...`，冒烟实测三种形态：默认 dev → `dev-key-from-web-dev-overlay`、`GOYII_ENV=prod` → `base-key-from-app-yaml`、`EMAIL_APIKEY=key-from-my-shell` → 环境变量值。notify 页面再给一个页面级出口（显示自定义段 sender）。
  - **自定义组件**：`examples/basic/notifier`——项目自持 struct，`New(cfg config.EmailConfig, mailer mail.Mailer) (*Notifier, error)` 构造期校验（apiKey/sender 缺失即 boot 失败）、Send 组稿（From=配置 sender）并投递给注入的 mailer；main.go 构造后经 `controllers.SiteController{Notify: notify}` 构造注入（与 PostController{Auth} 同姿势），无字符串注册表。
  - **03 最小修复残留清理**：jobs/defaultFrom 常量与 site ActionNotify 硬编码 From 删除（消息不再显式 SetFrom，由 mail.from 缺省回填——queue run 实测 `From: goyii basic <no-reply@goyii.example>`）；app.yaml 的 i18n.language 死值接线到 `i18n.WithDefaultLanguage`。
  - **SetRedis 说明**：示例保持零外部服务足迹（sqlite/file/log driver），无任何组件消费 redis client，装配一个无人使用的 client 属于死代码，故样板不演示 SetRedis（槽位本身已由 03 的 base 测试覆盖：未装配 panic / SetRedis(nil) 回未装配态）。如需演示，建议后续以「REDIS_ADDR 存在才装配并切 session/cache 后端」的形态另开票。
  - **templ 再生成**：views.templ notify 页改版（From/To 行 + notifier 说明），`templ generate ./views` 于 examples/basic 下执行，views_templ.go 仅 notifyPage 及行号位移变化。
- 2026-10-30 验收证据：
  - `go build ./...` 通过；`go test ./...` 全绿（1443 tests / 38 packages，较 03 的 1429 净增 14：notifier 9 例——TestNewRejectsIncompleteSection（4 子例）/ TestSenderComesFromSection / TestSendComposesFromConfiguredSender / TestSendPropagatesErrors / TestSendRejectsUnparseableAddress；示例 config 4 例——TestLoadDevAppliesOverlay / TestLoadEnvironmentWithoutOverlayUsesBaseValues / TestEmailAPIKeyEnvironmentOverride / TestLoadFillsIdentityAndComponentSections）；`golangci-lint run ./...` 0 issues；gofmt 无差异。
  - 冒烟（examples/basic 下）：清空 runtime 后 `go run . migrate up`（含 auth 表与 queue 表迁移）→ `seed`（3 用户 + 24 文章 + RBAC + 3 队列消息）→ `serve`：`/site/index` 200 返回 `{"app":"basic","env":"dev",...}`（身份字段）、`/login` 200、`/posts` 200、未知路径 404、CSRF 往返后 `POST /site/notify` 200 且页面含 `notifications@goyii.example`（自定义段出口）→ `queue run` 处理 3 条消息、投递邮件 From 为 mail.from 配置值。
