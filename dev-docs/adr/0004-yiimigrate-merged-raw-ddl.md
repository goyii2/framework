# 迁移子系统并入 yiimigrate 源码，DDL 走 database/sql 不走 GORM

数据层 DML 用 GORM，但版本化迁移（DDL）并入 yiimigrate 源码（`database/sql` 直连，MySQL/PostgreSQL/SQL Server/SQLite 四方言、历史表 `migration`、方言级迁移锁、dry-run），不用 GORM AutoMigrate。理由：AutoMigrate 无版本化/回滚能力，而迁移需要精确 DDL 控制与跨方言锁语义；yiimigrate 为本仓库维护者本人开发维护，形态（`Name/Up/Down` 接口 + 时间戳命名）与 Yii2 迁移几乎同构，并入无许可障碍。

## Consequences

- DDL 与 DML 分属两个直连层，共用同一 DSN；GORM 的 schema 解析仅用于类型映射，不产生表结构变更
- goyii CLI 的 `goyii migrate <cmd>` 子命令复用 yiimigrate 的八个命令语义，DSN 从应用配置注入并兼容 `DB_DSN` 环境变量
- 原独立仓库的去留（归档或停止演进）由维护者另行决定
