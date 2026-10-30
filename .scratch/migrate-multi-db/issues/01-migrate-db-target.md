# 01: migrate 多目标（--db 具名数据库目标）

Status: claimed
Blocked by: None（可立即开工）

**What to build:** `migrate --db=stats up` 能对项目装配注入的副库执行迁移：migrate 命令树接收具名目标（`migratecommands.Target(name, conn, migrations)` 可变参数），`--db` 挂根命令为 persistent flag，缺省仍为主库；up/down/redo/history/new/mark/to 全支持，未知目标报错列出已注册名，重名装配 panic；examples/basic 加 stats 副库演示目标（独立 sqlite + 自有迁移列表），docs/配置参考.md 补「多数据库」小节。详见同目录 spec.md。

- [ ] migratecommands.New 接受可变 Target 参数；--db persistent flag 生效于七个数据库子命令；缺省行为与现状完全一致
- [ ] 未知 --db 值报错并列出已注册目标名；重名目标装配期 panic；有测试
- [ ] --db=stats 全链路有测试（up/history 针对副库 sqlite 生效，主库不受影响）
- [ ] examples/basic：stats 副库目标可 `migrate --db=stats up` 冒烟通过；主库冒烟不回归
- [ ] docs/配置参考.md「多数据库」小节与示例一致
- [ ] go build ./... && go test ./... 全绿；lint/gofmt 干净
