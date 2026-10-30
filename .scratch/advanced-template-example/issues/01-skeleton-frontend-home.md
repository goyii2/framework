# 01: 骨架与前台首页

Status: ready-for-agent
Blocked by: None（可立即开工）

**What to build:** `examples/advanced` 端到端可运行的骨架：项目配置（app.yaml + web-dev overlay）经代码装配为 Application，frontend 分区以根作用域提供首页，console 提供 migrate/serve，user 表迁移先行落地 common 分区。演示：`go run . migrate up && go run . serve` 后访问 `/` 返回带亮色 Layout 的前台首页。main.go 只做组装，frontend 分区经自身装配函数注册。详见同目录 spec.md。

- [ ] `go run . migrate up` 成功建表（含 user 表迁移，出自 common 分区）
- [ ] `go run . serve` 启动，`/` 返回前台首页（亮色 Layout，导航含登录占位链接）
- [ ] main.go 只做组装；frontend 分区暴露装配函数，根作用域注册（约定回退或显式路由均可）
- [ ] config 分区：app.yaml + web-dev.yaml overlay 生效（dev 差异可观察，如 debug 开关）
- [ ] `go build ./...` 与 `go test ./...` 通过，gofmt 干净

## Comments
