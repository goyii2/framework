# 01: 组件包新形态·小组件（cache / mutex / mail / log）

Status: resolved
Blocked by: None（可立即开工）

**What to build:** cache、mutex、mail、log 四个组件包率先落地统一初始化形态（见 spec 第 2 节）：每包拥有自己的 Config 参数包（字段从 config 包对应 section 迁入，yaml tag 保留），唯一构造器 `New(cfg, opts...)`，跨组件依赖经 `WithRedis` / `WithLogger` 一族选项注入；非法驱动/参数在构造期返回 error（废除 failingMutex / failingMailer 延迟失败）；默认值内聚于 New，零值 Config 构造即得默认行为。config 包旧 section 本票暂不删（03 收口），base 内工厂调用点用结构体等值转换一行过渡。

- [x] 四包各自导出 Config 与唯一 `New(cfg, opts...)`，`NewFromConfig` 系列导出移除
- [x] 零值 Config 直接构造成功：cache→memory、mutex→process、mail→log driver、log→console/info
- [x] 非法驱动名/参数在构造期返回 error；failingMutex / failingMailer 及其测试删除，错误路径有新测试
- [x] 依赖注入位落地：cache / mutex 挂 WithRedis，mail 挂 WithLogger（LogSink）
- [x] base 内过渡调用编译通过；`go build ./...` 与 `go test ./...` 全绿

## Comments

- 2026-10-30 实施记录（分支 feat/project-owned-config-01，已 --ff-only 合入 main，commit 92655d3）：
  - 参数包落位：`cache.Config{Driver,KeyPrefix,TTL,Path,Redis}`（嵌套 `cache.RedisConfig`，避让后端类型 `cache.Redis`）、`mutex.Config{Driver}`、`mail.Config{Driver,From,SMTP}`（嵌套复用 `mail.SMTPConfig`，补 yaml tag）、`log.Config{Level,Targets,Gorm}`（嵌套 `log.TargetConfig` / `log.GormConfig`）；字段名/类型/yaml tag 与 config 包 section 一致。
  - 构造器：`cache.New(cfg, WithRedis(...))`、`mutex.New(cfg, WithRedis(...))`、`mail.New(cfg, WithLogger(...))` 均返回 `(T, error)`；`log.New(cfg) (*Logger, error)` 无 opts——log 无跨组件依赖，无注入位可设（spec 2.2 的 opts 仅承担依赖注入）。原程序化 `log.New(level, targets...)` 改名 `log.NewWithTargets`（与 cache 的 NewMemory/NewFile/NewRedis 同类的底层构建件同地位）。
  - 零值默认测试：cache `TestNewSelectsDriver/memory is the zero-value default`；mutex `TestNewDispatch`（"" 分支）；mail `TestNewDefaultsToLogMailer`（"" 分支）；log `TestNewZeroConfigIsConsoleInfo`（info + 单 console target）。
  - 错误路径新测试：cache `TestNewRejectsUnknownDriver`；mutex `TestNewRejectsUnknownDriver` + `TestNewRedisDriverRequiresConnection`（redis 驱动缺 WithRedis / 传 nil 均报错）；mail `TestNewRejectsUnusableConfig`（unknown driver / smtp 缺 host / 坏 encryption / 坏 from 四例，均在 New 期报错）；log `TestNewRejectsUnknownTargetType` / `TestNewRejectsUnwritableFile`。failingMutex / failingMailer 类型及其测试随延迟失败语义一并删除。
  - base 过渡：cache/mutex/mail 工厂调用点结构体等值转换（log 的 Targets 切片需循环，落在 `logConfigOf` 小助手）；构造被拒时按既有 newDefaultLog 先例 stderr 警告并兜底默认组件（cache→memory、mutex→process、mail→log mailer），原两条「错误延迟到使用期」的 base 测试改写为断言兜底行为（`TestApplicationMutexUnknownDriverFallsBackToProcess`、`TestApplicationMailerSmtpWithoutHostFallsBackToLog`）。
  - 组件包已全部不再 import config（grep 验证零引用），满足 spec「任何组件包不得 import 它」；config 包本票零改动（03 收口）。
- 2026-10-30 验收证据：`go build ./...` 通过；`go test ./...` 全绿（29 包 ok，无 FAIL）；`golangci-lint run`（base/cache/mutex/mail/log/console/db）0 issues；gofmt 无差异。附带触点：console/console_test.go 与 db/gormlog_test.go 的 `log.New` 调用随改名同步为 `log.NewWithTargets`。
