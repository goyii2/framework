# 02: Application 强类型容器 + config 双轨

Status: resolved

**What to build:** 框架的心脏（ADR-0001）：Application 为强类型组件容器——组件接口字段 + 懒初始化（并发安全），经 context 携带贯穿请求链路（FromContext），另有包级默认实例供 CLI/后台使用。config 双轨：值走 yaml 多环境 + 环境变量覆盖，行为走代码组装。yaml/env 加载库选型在本票定稿。

**Blocked by:** 01

- [x] New(Config) 装配应用；组件首次访问时懒初始化，并发安全（竞态测试）
- [x] FromContext 取回请求作用域应用实例；包级默认实例在无 context 场景可用
- [x] 多环境配置加载（命名方案按 PRD 草案定稿），环境变量可覆盖任意配置值
- [x] 组件可替换：接口注入 fake 的单测
- [x] yaml 库选型决定记入 dev-docs（勾销 PRD 开放问题）

## Comments

- 2026-10-28 实施记录（Windows + Git Bash，Go 1.27.2 编译，go.mod 声明 go 1.25）：
  - **base**：`Application` 强类型容器（`New(config.Config)` 装配，零值补默认）；组件为接口字段，经泛型 `component[T]`（`atomic.Pointer` 快路径 + mutex 双检首建）懒初始化，`Set` 可替换、置 nil 恢复默认懒初始化。首个组件 `Log`（接口：Enabled/Debug/Info/Warn/Error，默认实现 slog TextHandler 按配置分级）。`app.WithContext(ctx)` 注入 / `base.FromContext(ctx)` 取回（无携带时回退包级默认实例）；`base.Default()` 懒建单例（atomic CAS）、`SetDefault` 可安装显式装配的应用（CLI/后台用）。后续组件（db/cache/session 等）在各自工单按同模式加字段。
  - **config**：值轨 = `config/app.yaml`（基线）+ `config/web-<env>.yaml`（环境覆盖层，字段级合并：顺序反序列化，覆盖层缺省的键保留基线值）+ 环境变量最终覆盖；环境由 `GOYII_ENV` 选择（默认 dev）。任意叶子字段可被环境变量覆盖：字段路径大写下划线连接（`DB.DSN` -> `DB_DSN`、`App.Debug` -> `APP_DEBUG`、`Cache.Redis.DB` -> `CACHE_REDIS_DB`），反射解析 string/bool/int/uint/float/duration，非法值报错并点名变量。零值由 `WithDefaults` 补框架默认（listen :8080、level info、cache memory、session file/1440s 等）。行为轨在代码组装，不进 yaml。
  - **yaml 库选型定稿**：轻量自研绑定 + `github.com/goccy/go-yaml v1.19.2`（单依赖、维护活跃；本机模块缓存可用，代理超时不阻塞）。koanf/viper 依赖体更重，其多源 provider 生态对固定的「两 yaml + env」源集合无增益。已勾销 PRD「开放问题」对应条目（附一句理由），PRD 其余未动。
  - **TDD**：懒初始化与并发部分先红后绿——先落 `base/component_test.go` + `base/application_test.go`（`go test ./base/` 构建失败：config 包缺失/API 未定义），实现后转绿；竞态测试 `TestComponentConcurrentSetAndGet` / `TestApplicationConcurrentUse` 混合并发 set/get/FromContext/Default。
  - 附带修复：Windows `core.autocrlf=true` 检出导致 gofmt 报 CRLF——新增 `.gitattributes`（`*.go text eol=lf`，独立 chore 提交），基线 lint 恢复全绿。
  - 验收证据（均在合并后 main 上执行）：
    - `go test -race -count=1 ./base/... ./config/...` -> 20/20 PASS（含 `-race`；CGO 经 scoop mingw gcc 启用）；`-count=2` 复跑 40/40 PASS。测试名：TestNewAssemblesApplication / TestApplicationLogLazyDefault / TestApplicationSetLogFake（接口注入 fake）/ TestApplicationContextRoundTrip / TestFromContextFallsBackToDefault / TestFromContextForeignValueFallsBackToDefault / TestDefaultIsSingletonAndReplaceable / TestApplicationConcurrentUse / TestComponentLazyInitBuildsOnce（并发 50 goroutine 仅构建 1 次）/ TestComponentSetReplacesWithoutBuilding / TestComponentSetNilRebuildsDefault / TestComponentConcurrentSetAndGet / TestLoadEnvDevOverlay / TestLoadEnvProdOverlay / TestLoadEnvWithoutOverlayUsesBaseOnly / TestEnvOverrides / TestEnvOverrideInvalidValueFails / TestLoadSelectsEnvironmentFromGOYIIEnv / TestCurrentEnv / TestLoadEnvMissingFilesFail
    - `go build ./...` / `go vet ./...` -> 无输出；`golangci-lint run` -> `No issues found`
  - 流程：claimed 单独提交 `5a39c1c` -> `feat/application-config` 上 `db2d073`（.gitattributes chore）+ `0fba1e3`（实现 + PRD 勾销）-> main `--ff-only` 合并并删除任务分支。
  - 遗留：`Log` 接口为最小集（分级方法 + Enabled），12 号工单（分级/分类、多 Target、profile）到时扩展或以富实现注入 `SetLog`；`config/app.yaml`+`web-<env>.yaml` 命名已按 PRD 草案落定（PRD「开放问题」中该条未在许可范围内，未另改 PRD）。
