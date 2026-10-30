# 05: examples/basic 样板重写

Status: ready-for-agent
Blocked by: 03、04

**What to build:** examples/basic 重写为「项目自有配置 + 代码装配」官方样板：项目自定义 Config struct（身份字段 + 复用各组件包参数包 + 自定义 Email 段），main.go 全急切装配（含 SetSession / SetRedis）；`EMAIL_APIKEY` 环境变量覆盖与 `web-dev.yaml` overlay 各有一处真实演示；自定义组件按「项目自持 struct + 构造注入」姿势呈现（现有 auth manager 模式为正源），不出现任何框架配置残留。

- [ ] 项目 Config struct 位于示例自己的 config 包；自定义 Email 段随 LoadAs 加载并可被 EMAIL_APIKEY 覆盖（有可见演示：启动日志或页面输出）
- [ ] main.go 全急切装配，构造错误在 boot 期处理
- [ ] overlay（app.yaml + web-dev.yaml）至少一处真实覆盖演示
- [ ] 自定义组件以项目自持 + 构造注入呈现，无字符串注册表
- [ ] serve / migrate / queue worker 冒烟可用；`go test ./...` 全绿
