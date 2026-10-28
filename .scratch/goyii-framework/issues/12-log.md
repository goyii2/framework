# 12: log 组件

**Status:** resolved

**What to build:** 基于 log/slog 的日志组件：分级/分类（category 过滤）、多 Target（console/file，可扩展）、性能 profile（计时区间产出开始/结束与耗时）；日志器经 Application 取用。

**Blocked by:** 02

- [x] 分级与 category 过滤正确（命中/排除规则各自验证）
- [x] console 与 file Target 输出验证（file 含滚动或至少追加写入）
- [x] profile 计时区间产出配对的开始/结束日志与耗时
- [x] Target 为接口，自定义 Target 单测注入可行

## Comments

- 2026-10-28 实施记录（Windows + Git Bash，Go 1.27.2 编译，go.mod 声明 go 1.25；-race 经 scoop mingw gcc 启用 CGO）：
  - **log 包（替换 02 的 base 内置 slog TextHandler 最小实现，接口向后兼容）**：`Logger` 以 slog 分级为底座——`Enabled/Debug/Info/Warn/Error` 与 02 的 `base.Log` 契约逐字兼容（`*log.Logger` 结构化满足它）；新增 `Log(level, category, msg, args...)` 显式形式、`Category(cat)` 派生作用域 logger（共享 targets/level、不可变分支）、`Wrap(minimal)` 把外部最小实现适配到全量 API（转发按级别）。未配 category 时落 `application`（Yii2 默认）。
  - **Target 接口**：`Name() / Write(Record) error / Close() error`——规则（级别地板 + category 命中/排除）在派发器侧统一执行，sink 只管输出；自定义 Target 经 `New(...)` 或 `WithTarget(t, Filter)` 注入（返回新 logger，不改 receiver）。内置 `ConsoleTarget`（io.Writer，默认 stdout，互斥串行写）与 `FileTarget`（追加写 + 按大小滚动：`app.log -> app.log.1/.2/...`，`maxFiles` 保留份数、<=0 不限，写前检查+互斥锁，行完整不交错）。`Filter` 模式对齐 Yii2 categories/except：精确 / 尾缀 `*` 前缀匹配 / 单独 `*` 全命中；排除优先于命中。`Filter` 零值地板为 info（对齐 slog HandlerOptions 零值语义，测试显式断言）；`New` 与未配 minLevel 的配置绑定用 `slog.LevelDebug` 关闭地板。
  - **profile**：`l.BeginProfile(name)` 立即产 `profile begin`（info 级、category `profile`、attr `token`），`Profiler.End()` 产配对 `profile end`（attr `token` + `elapsed_ms` 毫秒浮点）；End 幂等（defer 重复安全）；logger 级别过滤对成对记录生效（级别不够时 Begin/End 全静默）；category 过滤可只收 profile 对。
  - **config**：`log.targets[]`（type console|file、minLevel、categories、except、path、maxSizeMB 默认 10、maxFiles 默认 5）+ `log.gorm`（disable 默认 false=开、slowThresholdMs 默认 200）；未配 targets 时 `NewFromConfig` 装单个 stdout console target（与 02 默认行为等价）；任一 target 构建失败（未知类型/路径不可写）整体报错，base 侧降级 console 并向 stderr 报告——应用仍能启动。切片字段不参与 env 覆盖（walker 仅反射标量叶子，与既有约定一致）。
  - **gorm 适配器（接住 07/08 遗留，放在 db 包保持 log 不依赖 gorm）**：`db.GormLogger` 实现 `gorm/logger.Interface`——`Trace` 按 nil 错误->debug `query`、达慢阈值->warn `slow query`（带 threshold_ms）、其他错误->error `query failed`（带 err）、`ErrRecordNotFound` 视为常规未命中落 debug；SQL/行数仅在对应级别会真正发出时才渲染（被过滤的查询零插值开销）；`Info/Warn/Error` 直转对应级别；全部记录落 category `db`。logger 经 `func() *log.Logger` 惰性供应者解析（保持 log 组件懒初始化）。接线：`db.Connect(cfg, opts...)` + `db.WithGormLogger(...)`，`db.NewComponent` 透传 opts；`base.New` 默认注入（`log.gorm.disable: true` 才回退 gorm 自带 stdout logger）。
  - **base**：默认 Log 实现换成 `log.NewFromConfig`；新增 `Application.Logger() *log.Logger` 取全量 API（SetLog 装入的外部实现经 `log.Wrap` 包装、投递转发原实现；`Log()` 契约与既有调用点——web/console/session——零改动）。
  - 验收证据（均在合并后 main 上执行）：
    - `go test -race -count=1 ./...` 全仓 PASS（base/config/console/db/db/ar/log/migrate 系/url/validators/web，cache 等未实现包 no test files）；`-count=2 -race` 复跑 log/base/db/config 四包 PASS。log 包 31 个测试全数通过，覆盖：级别地板（TestLoggerRespectsLevelFloor / TestFilterAcceptLevelFloor）、category 命中与排除各自验证（TestFilterAcceptIncludeRules / TestFilterAcceptExcludeWinsOverInclude / TestMatchPattern / TestLoggerCustomTargetInjectionWithFilter）、console+file 双 Target 同记录投递（TestLoggerDeliversToConsoleAndFileTargets）、file 追加保留旧内容与按大小滚动+保留份数（TestFileTargetAppendsToExistingFile / TestFileTargetRotatesBySizeAndKeepsBackups / TestFileTargetUnlimitedRetention）、profile 配对+耗时+幂等+过滤（TestProfileEmitsPairedRecords 等 5 个）、自定义 Target 注入（memTarget 贯穿多测试）、-race 并发：16 goroutine × 50 写 file 零丢失零交错（TestFileTargetConcurrentWrites）与 32 goroutine × 8 区间并发 profile begin/end 配对完整（TestProfileConcurrentTiming）。
    - gorm 适配器：TestGormLoggerTrace{NormalQuery,SlowQuery,Error,RecordNotFoundIsRoutine,SkipsSQLWhenFilteredOut} + InfoWarnErrorMessages + LogModeReturnsSelf；应用级接线：TestApplicationGormQueryLoggingEnabledByDefault（sqlite 建表/插入后 file target 含 `[db] query` 与 SQL）/ TestApplicationGormQueryLoggingDisabled（disable 后目标无任何 db 记录）。config：TestLoadLogSection（yaml 解析 targets/gorm 全字段）/ TestLogDefaults（阈值 200 默认、默认开启）。
    - `go build ./...` / `go vet ./...` 无输出；`golangci-lint` 沿用 06 起的替代约定（本机版本与 Go 1.27 stdlib 不兼容），以 go vet + gofmt 干净替代（`gofmt -l .` 全仓无输出）。
    - 流程：claimed 单独提交 `37d4b4a` -> `feat/log-component` 上 `3d2cd8a`（实现 + 测试）-> main `--ff-only` 合并并删除任务分支。
  - 遗留：file 后端无按日期切分（按大小滚动已满足验收，日期切分可后续加 `type: dailyFile`）；无 syslog/网络 Target（接口已开放）；profile 未做嵌套聚合汇总（Yii2 debug 面板属 P2）；`Logger.Close()` 尚无应用关停钩子统一调用位（15 号 P0 集成票落 console/应用生命周期时接）。
