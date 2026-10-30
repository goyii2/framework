# 04: web / console 显式 option 化

Status: ready-for-agent
Blocked by: 03

**What to build:** web 与 console 的行为开关全部显式化：web.NewHandler 的 CSRF 开关与约定回退开关改为显式 option，session 改为消费 `app.Session()`（不再内部装配）；console serve 的监听地址由装配方注入（未注入时给出明确错误）；debug 面板删除 Config 摘要（保留请求 / 组件状态摘要）。

- [ ] web.NewHandler 不再从任何配置读值；CSRF / 约定回退的默认值与显式 option 行为有测试
- [ ] session 由 app.Session() 提供；替换组件经 SetSession 生效
- [ ] serve 命令监听地址来自装配注入，未注入时报错信息明确
- [ ] debug 面板无 Config 摘要，其余功能不回归
- [ ] `go build ./...` 与 `go test ./...` 全绿；examples 同步最小编译修复
