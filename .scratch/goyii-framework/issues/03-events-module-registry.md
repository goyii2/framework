# 03: 事件调度 + Module 注册表

**Status:** resolved

**What to build:** base 包两件语义：类型化事件调度（On 订阅 / Trigger 触发，含 BeforeRequest/AfterRequest 应用生命周期事件）；Module 单层注册表（路由前缀、controller 归属登记，组件作用域与嵌套接口预留）。模型生命周期事件不走此机制（由 GORM 钩子承担，见 PRD 放弃清单）。

**Blocked by:** 02

- [x] On/Trigger 并发安全，事件携带请求上下文，支持注销
- [x] BeforeRequest/AfterRequest 单测模拟触发（真实请求接线在 04）
- [x] Module 注册表可登记前缀与 controller 归属；嵌套能力接口预留（P2 验证）
- [x] 订阅/触发/注销全路径单测

## Comments

- 2026-10-28 实施记录（Windows + Git Bash，Go 1.27.2 编译，go.mod 声明 go 1.25）：
  - **事件调度（base/event.go）**：事件名强类型 `EventName`（typed string），预定义 `EventBeforeRequest`（"beforeRequest"）/`EventAfterRequest`（"afterRequest"）生命周期常量，应用自定义事件即再声明 `EventName` 值。载荷为具体 `Event{Name, Data}` 结构体（可选 Data 字段，生命周期事件为 nil），不做 map[string]any 动态载荷；`Handler func(ctx context.Context, e Event)` 签名携带请求上下文。`Application.On(name, h)` 返回注销函数（内部以 subscription 指针身份匹配——Go 函数值不可比较，不能按 handler 相等删除；注销幂等）；`Application.Trigger(ctx, name, data)` 按注册顺序同步派发。并发安全：RWMutex 保护 + 派发前快照 handler 列表，handler 可在派发中订阅/注销（含自注销），改动自下次触发生效。事件注册表为 Application 值字段（懒建 map），零值 Application 亦可安全使用事件 API。
  - **Module 注册表（base/module.go）**：`NewModule(app, prefix)` 归一化前缀（去尾 `/`；须以 `/` 开头且为非空段，`""`/`"/"` 拒绝——应用自身即根作用域）；Application 持单层注册表：`RegisterModule`（拒绝重复前缀 / 跨应用模块 / nil）、`Module(prefix)` 查找、`Modules()` 按前缀排序返回。controller 归属登记：`Module.RegisterController(id, c)`（id 即约定路由 controller 段，拒空串与含 `/`、拒重复）、`Controller(id)` 取回、`ControllerIDs()` 注册序。组件作用域预留：`Module.App()` 返回属主 Application，模块代码经它解析组件（模块级组件覆盖为后续预留，文档注明）；嵌套预留：`Module.RegisterModule(child)` 恒返 `ErrModuleNestingUnsupported`（P2 实施嵌套时替换语义）。
  - **TDD**：事件并发路径先红后绿——先落 `base/event_test.go` 全量 7 测（`go test ./base/` 构建失败：`undefined: Event / EventName / On / Trigger`），实现 `event.go` + `application.go` 接线后转绿。Module 侧直接实现 + 测试。
  - 验收证据（均在合并后 main 上执行，CGO 经 scoop mingw gcc 启用，PATH 指向 `~/scoop/apps/mingw/current/bin`）：
    - `CGO_ENABLED=1 go test -race -count=1 ./base/... ./config/...` -> 34/34 PASS（含 -race）；`-race -count=2 ./base/...` 复跑 52/52 PASS（base 26 测试 × 2）。测试名：TestEventTriggerDeliversEventAndContext / TestEventHandlersRunInRegistrationOrder / TestEventOffUnsubscribesOnlyTargetHandler / TestEventOffDuringDispatchTakesEffectNextTrigger（派发中自注销+新订阅，快照语义）/ TestEventTriggerWithoutSubscribersIsNoop / TestLifecycleEventsSimulatedRequestTrigger（模拟 04 将做的请求前后触发，含常量值断言）/ TestEventConcurrentSubscribeTriggerOff（48 goroutine 混合并发订阅/注销/触发，静默后精确核对稳定订阅者）；TestNewModuleValidatesPrefix / TestModuleResolvesThroughOwningApplication / TestModuleRegistersControllerOwnership / TestModuleRegisterControllerValidation / TestApplicationModuleRegistry / TestModuleNestingReserved / TestModuleConcurrentAccess
    - `go build ./...` / `go vet ./...` -> 无输出；`gofmt -l .` -> 无文件
    - `golangci-lint run` -> 代码 "0 issues"；退出码 7 系环境偏差：本机 linter 2.13.1 内嵌 go1.26.5 无法解码 Go 1.27.2 stdlib 导出数据（export data version 5 > 4），对未改动的 02 号基线文件同样报 typechecking 错（已在 stash 后的干净树上复现证实），非本票引入
  - 流程：claimed 单独提交 `a07e7a3` -> `feat/events-module-registry` 上 `c569952`（实现 + 测试）-> main `--ff-only` 合并并删除任务分支。
  - 遗留：BeforeRequest/AfterRequest 真实请求接线在 04（web 层中间件触发，接口已定）；模块嵌套与模块级组件覆盖为 P2 预留（`ErrModuleNestingUnsupported` + `App()` 缝）；controller 实例暂以 `any` 登记，04 的 Router 以类型断言/反射解析 ActionXxx 时如需更强类型再收紧。
