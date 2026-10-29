# 组件：gii 代码生成器

从数据库表结构生成模型、控制器骨架与 CRUD 的命令行工具；纯参数无交互向导（术语见 CONTEXT.md「代码生成器（Gii）」）。

```bash
go run . gii model --table=post            # 生成内嵌 ar.Model 的模型 + Rules() 骨架
go run . gii controller --id=post          # 生成控制器骨架 + 打印路由注册片段
go run . gii crud --table=post             # 模型 + 控制器（list/new/edit/save/delete）+ .templ 视图
```

挂载方式与 migrate 一致（`console.AddCommand` 由使用方调用）：

```go
con := console.New(app, console.WithHandler(handler))
con.AddCommand(giicommands.New(app))
```

## 共用 flags

| flag | 说明 |
|---|---|
| `--table` / `--id` | 目标表 / 控制器 id（必填） |
| `--dsn` | 覆盖应用配置的 `db.dsn`（连接走 base.Application 的 DB，驱动仍取配置） |
| `--out` | 输出目录（默认当前目录；不存在则创建） |
| `--package` | 生成文件的包名（默认取输出目录名；目录名不是合法标识符时需显式传） |
| `--templates` | 模板覆盖目录，同名文件替换内置模板 |

## model

经 GORM Migrator（`ColumnTypes` / `GetIndexes`）读表结构后生成 `<table>.go`：

- 字段名 snake_case -> CamelCase（`author_id` -> `AuthorID`）；类型映射 Go 基础类型：整数族 -> `int`/`int64`/…，varchar/text 族 -> `string`，bool -> `bool`，数值小数 -> `float64`，date/datetime/timestamp -> `time.Time`，blob 族 -> `[]byte`。可空列生成指针字段；映射不了的类型落 `string` 并留 TODO 注释。
- 主键列写 `primaryKey`（检测到自增则追加 `autoIncrement`），并生成 `TableName()`。
- `Rules()` 骨架按猜测生成：NOT NULL 且无默认值 -> `Required`，varchar 长度 -> `Max`；带默认值/数值/时间列猜不准的留 TODO 空注释；单列唯一索引留唯一性检查提示。`created_at`/`updated_at` 照常生成字段但不进规则。

注意：SQLite 把所有整数类型（含迁移的 `BigPrimaryKey`）声明为 `INTEGER`，因此主键在 SQLite 下生成为 `int`。

## controller

生成 `<id>_controller.go`：内嵌 `web.Controller`，Index/View/Create/Update/Delete 五个占位动作，并在控制台**打印**路由注册片段（不自动改 main）：

```go
ctl := &PostController{}
c := h.Ctrl(ctl)
c.Get("/post", ctl.ActionIndex).Name("post/index")
c.Get("/post/{id:[0-9]+}", ctl.ActionView).Name("post/view")
...
```

## crud

一次生成三个文件（`<table>.go`、`<table>_controller.go`、`<table>_views.templ`）：

- **list**：`dataprovider.FromRequest` 解析分页/排序（白名单=网格列），`ActiveDataProvider` 取数，`GridView` + `LinkPager` 渲染。
- **form**（new/edit 共用，save 同时服务创建与更新两个路由）：`ActiveForm` 渲染，422 回显校验消息与已填值；input 类型按列猜：TEXT -> textarea、数值 -> number、bool -> checkbox、date/datetime -> datetime-local、其余 text。
- **delete**：POST 删除后回列表。
- 长文本列（TEXT 族）不进网格，进表单 textarea；`created_at`/`updated_at` 不进表单。
- 控制台打印路由注册片段、视图注册片段，并提示运行 `templ generate`——生成器只产出 `.templ` 源码，不执行编译；真实编译依赖你的工程上下文。

## 模板覆盖

内置模板以 `go:embed` 打包在 `gii/templates/`：`model.go.tmpl`、`controller.go.tmpl`、`crud_controller.go.tmpl`、`crud_views.templ.tmpl`。`--templates <dir>` 下同名文件整体替换内置版（解析失败会在写入前报错）。模板引擎为 `text/template`，数据结构见 `gii/generate.go`（`modelData` / `crudData`）。生成的 `.go` 产物统一经过 gofmt；`.templ` 文件只生成源码。

## 验收与边界

- 表结构缺失 / 无主键（crud 需要）等会直接报错退出，不落半成品。
- 生成文件头部注明「按当前表结构生成，可自由修改，生成器不做同步」。
- 单测以 sqlite 内存表建表 -> 生成 -> 断言产物，并对 examples/basic 的迁移表（临时 sqlite）生成后用 `go/parser` 校验可解析性；真实 `templ generate` + 编译由使用方工程完成（本仓库已人工验证通过一次）。

## 相关文档

- [组件：db 与 ActiveRecord](组件-db与ActiveRecord.md)（模型与 `ar.Model`）
- [组件：view 与 templ](组件-view与templ.md)（GridView / LinkPager / ActiveForm）
- [组件：dataprovider](组件-dataprovider.md)（分页排序约定）
- [组件：migrate](组件-migrate.md)（建表与方言说明）
