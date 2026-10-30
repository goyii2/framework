# 02: 组件包新形态·大组件（db / queue）

Status: claimed
Blocked by: None（可与 01 并行）

**What to build:** db 与 queue 完成统一初始化形态：db 构造器统一为 `New(cfg)` 并把 Config 参数包迁入包内（原 Connect 语义并入，现有 Option 能力并入 With 族）；queue 的 sync/db/redis 驱动 switch 从 base 包迁回 queue 包，Config 迁入，db 连接与 redis 客户端经 `WithDB` / `WithRedis` 选项注入，构造期返回错误。`db.Component` 懒 holder 保留为「想要懒 / 可重试语义」的可选件，不再是主装配路径。

- [ ] db 包导出 Config 与 `New(cfg, opts...)`，Connect 导出移除，现有 Option 能力并入 With 族
- [ ] queue 驱动选择 switch 全部位于 queue 包内；base 不再包含任何 queue 驱动分支
- [ ] queue 依赖经 `WithDB(*db.Conn)` / `WithRedis(UniversalClient)` 注入；缺必需依赖时构造期报错
- [ ] db.Component 懒 holder 保留且测试通过（可选件地位）
- [ ] `go build ./...` 与 `go test ./...` 全绿
