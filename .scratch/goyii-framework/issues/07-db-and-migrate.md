# 07: DB 组件 + yiimigrate 并入

**Status:** ready-for-agent

**What to build:** 数据库接入（ADR-0004）：GORM 连接作为 db 组件懒初始化（DSN 来自应用配置，兼容 DB_DSN 环境变量）；yiimigrate 源码并入 migrate 包，保持 database/sql 直连、不引 ORM 的原架构；`goyii migrate` 暴露八命令（up/down/redo/history/new/mark/to/create），历史表、方言锁、dry-run 按原语义。

**Blocked by:** 02, 06

- [ ] 首次访问建立 GORM 连接；DSN 从应用配置注入，DB_DSN 环境变量兼容
- [ ] yiimigrate 源码并入后编译通过、原测试随之迁移并可运行（以其测试为准绳）
- [ ] 对 SQLite 完成 create -> up -> down -> redo 全流程验收
- [ ] dry-run 与迁移锁行为保持原语义
