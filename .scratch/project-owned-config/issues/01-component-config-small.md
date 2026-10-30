# 01: 组件包新形态·小组件（cache / mutex / mail / log）

Status: claimed
Blocked by: None（可立即开工）

**What to build:** cache、mutex、mail、log 四个组件包率先落地统一初始化形态（见 spec 第 2 节）：每包拥有自己的 Config 参数包（字段从 config 包对应 section 迁入，yaml tag 保留），唯一构造器 `New(cfg, opts...)`，跨组件依赖经 `WithRedis` / `WithLogger` 一族选项注入；非法驱动/参数在构造期返回 error（废除 failingMutex / failingMailer 延迟失败）；默认值内聚于 New，零值 Config 构造即得默认行为。config 包旧 section 本票暂不删（03 收口），base 内工厂调用点用结构体等值转换一行过渡。

- [ ] 四包各自导出 Config 与唯一 `New(cfg, opts...)`，`NewFromConfig` 系列导出移除
- [ ] 零值 Config 直接构造成功：cache→memory、mutex→process、mail→log driver、log→console/info
- [ ] 非法驱动名/参数在构造期返回 error；failingMutex / failingMailer 及其测试删除，错误路径有新测试
- [ ] 依赖注入位落地：cache / mutex 挂 WithRedis，mail 挂 WithLogger（LogSink）
- [ ] base 内过渡调用编译通过；`go build ./...` 与 `go test ./...` 全绿
