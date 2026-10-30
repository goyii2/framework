# spec：migrate 多目标（--db）（migrate-multi-db）

状态：共识已确认。

## 决策

1. **具名目标由装配注入**：`migratecommands.New(app, migrations, migratecommands.Target("stats", conn, statsMigrations)...)`。副库连接与迁移列表都是项目资产，Application 不开具名连接注册表（维持 ADR 0001/0005 的无字符串定位原则）。
2. **CLI 形态**：`--db` 挂 migrate 根命令（persistent flag），与 Yii2 `yii migrate --db=...` 同形；缺省 = 主库（app.DB() + 主迁移列表）。up/down/redo/history/new/mark/to 七个子命令全部支持；`create` 不变（只生成文件）。未知目标名 → 明确报错并列出已注册目标。重名目标 → 装配期 panic（programmer error）。`MIGRATE_TABLE` / `MIGRATE_DRY_RUN` 对所有目标行为一致。
3. **范围**：examples/basic 增加 `stats` 副库演示目标（独立 sqlite + 自有迁移列表，冒烟含 `migrate --db=stats up`）；docs/配置参考.md 补「多数据库」小节；`create` 保持仅主库（`--dialect` 已够副库手写场景，`--dir` 目录化留后续）。

## 背景

多数据库在新架构（project-owned-config，ADR 0005）下的官方姿势：主库占框架槽位（ConnectDB/DB()），副库为项目自持组件（db.New 构造 + 构造注入）。唯一框架级缺口是 migrate/gii 命令只面向主库；本特性补 migrate 侧。gii 多目标不在本特性内。
