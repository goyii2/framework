# 组件：migrate

版本化数据库迁移（yiimigrate 并入）：`database/sql` 直连不走 ORM，方言覆盖 MySQL/SQLite/Postgres/SQLServer，历史表 `migration`，支持 dry-run。

## 八命令

```bash
go run . migrate up          # 应用所有未执行迁移
go run . migrate down [N]    # 回退 N 步（默认 1）
go run . migrate redo [N]    # 回退再重放 N 步
go run . migrate history     # 已应用清单与时间
go run . migrate new <name>  # 生成空迁移文件骨架
go run . migrate mark <name> # 标记已应用（不执行）
go run . migrate to <name>   # 迁移到指定版本（可向前可回退）
go run . migrate create <name>  # 同 new
```

DSN 从应用配置 `db` 段注入，`DB_DSN` 环境变量兼容；`--dry-run` 打印 SQL 不执行。

## 编写迁移

```go
type m20260101_000002_create_post_table struct{}

func (m20260101_000002_create_post_table) Name() string {
    return "m20260101_000002_create_post_table"
}

func (m20260101_000002_create_post_table) Up(ctx context.Context, m *migrate.MigrationContext) error {
    return m.Schema().
        CreateTable("post", migrate.Columns().
            Add("id", m.BigPrimaryKey()).
            Add("title", m.String(200).NotNull()).
            Add("body", m.Text().Null()).
            Add("status", m.String(16).NotNull().DefaultValue("draft")).
            Add("author_id", m.BigInteger().NotNull()),
        ).
        Exec(ctx)
}

func (m20260101_000002_create_post_table) Down(ctx context.Context, m *migrate.MigrationContext) error {
    return m.Schema().DropTable("post").Exec(ctx)
}
```

清单注入应用（应用顺序 = 列表顺序）：

```go
con.AddCommand(migratecommands.New(app, migrations.All()))
```

## 构建词汇

- 列：`m.PrimaryKey()/BigPrimaryKey()/Integer()/BigInteger()/String(n)/Text()/Boolean()/Decimal(p,s)/Money(p,s)/DateTime(p)/Timestamp(p)/Date()/Time()/Json()/UUID()/Enum(...)/Set(...)`，链式 `NotNull()/Null()/DefaultValue(v)/DefaultExpression(sql)/Unique()/Index? via CreateIndex/After(col)/Comment(s)`。
- 表与结构：`CreateTable/CreateTableIfNotExists/DropTable/RenameTable/TruncateTable/AddColumn(/IfNotExists)/AlterColumn/DropColumn(/IfExists)/RenameColumn/CreateIndex(/IfNotExists)/DropIndex(/IfExists)/AddPrimaryKey(/IfNotExists)/AddForeignKey/Raw(sql)`。
- DML 便捷：`m.Execute(ctx, "UPDATE ...", args...)` 与 `migrate.Expr("NOW()")` 表达原生 SQL 默认值。

## 方言说明（SQLite）

SQLite 无真时间类型，但驱动按**声明类型**做 `time.Time` 转换——`dateTime/timestamp` 映射为 `DATETIME`、`date -> DATE`、`time -> TIME`（值仍以文本存储）。声明为 `TEXT` 的时间列会静默打断 GORM 的 time 往返，故不要用 `m.Text()` 承载时间字段。

## 相关文档

- [组件：db 与 ActiveRecord](组件-db与ActiveRecord.md)
- [配置参考](配置参考.md)（`db` 段）
