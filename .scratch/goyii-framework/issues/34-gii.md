# 34: Gii 代码生成器

**Status:** resolved

**What to build:** 代码生成器命令 `goyii gii model|controller|crud`（gii 包）。model 经 GORM Migrator 读表结构，生成内嵌 ar.Model 的 struct + `Rules()` 骨架；controller 生成骨架并打印路由注册片段；crud 生成 model + controller + `.templ` 视图（list 用 GridView、form 用 ActiveForm）并提示运行 templ generate。text/template + embed 内置模板，`--templates` 目录同名覆盖；纯 flags 无交互向导；产物统一 gofmt。

**Blocked by:** 30, 31

- [x] `gii model --table`：字段/类型/主键正确；Rules() 骨架按 NOT NULL->required、长度->max 猜测，不确定留空注释
- [x] `gii controller --id`：骨架 + 路由注册片段打印
- [x] `gii crud --table`：list（GridView + LinkPager）/ form（ActiveForm）/ delete 动作 + 视图注册代码，提示 templ generate
- [x] input 类型映射：TEXT->textarea、数值->number、bool->checkbox、时间->datetime-local、其余 text
- [x] `--templates` 覆盖机制（同名文件替换）；产物统一 gofmt
- [x] 验收：对 examples/basic 的表执行生成，产物可编译（不入库）

## Comments

- 2026-10-30 resolved（feat/34-gii，merge --ff-only 到 main）：

  **落地**：`gii` 包（schema.go 表结构内省、naming.go 命名、template.go embed+覆盖、generate.go 三个生成入口 + 四个内置模板）+ `gii/commands`（cobra 子命令组，挂载对齐 migrate/commands，`console.AddCommand(giicommands.New(app))` 由使用方调用）。flags：`--table/--id/--dsn/--out/--package/--templates`。

  **验收证据**（`go test ./gii/...` 全绿，golangci-lint 0 issues）：
  - `TestGenerateModelMapsColumnsAndRules`（generate_test.go）：sqlite 建表（varchar/text/decimal/bool/datetime/默认值/唯一索引）-> 生成 -> 断言字段类型（可空列指针、TEXT->`*string`、DATETIME->`*time.Time`）、`primaryKey` 标签、TableName、Rules 骨架（NOT NULL 无默认->`Required`、varchar(120)->`Max(120)`、默认值列不带 Required、唯一索引 TODO 提示、猜不准 TODO 注释）；`assertGofmtClean` 验证产物即 gofmt 输出。
  - `TestGenerateControllerSkeletonAndSnippet`：骨架 + 路由片段（`c.Get("/post", ctl.ActionIndex).Name("post/index")` 等）打印；`TestGenerateControllerRejectsInvalidID` 非法 id 拒绝。
  - `TestGenerateCrudProducesModelControllerAndViews`：三件产物（model/controller/.templ）；五个动作；`dataprovider.FromRequest` 白名单 + `ActiveDataProvider` + `view.GridView`/`view.LinkPager`/`view.ActiveForm`；input 映射断言（decimal->FieldNumber、bool->FieldCheckbox、datetime->FieldDatetimeLocal、TEXT->FieldTextarea）；TEXT 列不进网格、时间戳列不进表单；可空列空值保持 NULL（`stringPtr`、`if in.publishedAt != ""`）；snippet 含路由/视图注册与 `Run "templ generate"` 提示。`TestGenerateCrudRequiresPrimaryKey` 无主键拒绝。
  - `TestGenerateModelTemplateOverride`：`--templates` 同名模板整体替换生效。
  - `TestGenerateModelAgainstExampleUserTable` / `TestGenerateCrudAgainstExamplePostTable` / `TestIntrospectReportsExampleTables`（examples_test.go）：把 examples/basic 的迁移（migrations.All()）应用到**临时 sqlite 文件**（不碰 examples 的 runtime 目录）后生成，全部 `.go` 产物 `go/parser` 解析通过 + gofmt clean；post 模型含 `CreatedAt/UpdatedAt *time.Time`（迁移 4 的时间戳列照常生成）、body `*string`、status 带默认不生成 Required。
  - commands_test.go：`gii model/controller/crud` 端到端（输出缓冲断言 created 行 + 片段）、`--dsn` 覆盖（覆盖连接用后关闭）、缺表报错。

  **取舍/备注**：
  - examples 验收用「example 迁移 -> 临时 sqlite」替代直接读 examples/basic/runtime 的库文件（runtime 不入库、机器本地）——表结构同一份，测试可重复。
  - 真实 `templ generate` + `go build` 依赖用户工程，不入测试；开发期已人工跑通两轮（数字主键 product 表、字符串主键 gadget 表含可空数值/窄整型/unmapped UUID），产物经 templ v0.3.1100 编译并 `go build`/`go vet` 通过后删除。
  - SQLite 把一切整数（含 BigPrimaryKey）声明为 INTEGER，故主键在 sqlite 下生成 `int`（诚实映射，文档已注明）。
  - 网格不列 TEXT 族长文本列（进表单 textarea）；无主键表 crud 拒绝并报错。
  - 文档：docs/组件-gii代码生成器.md（三子命令、flags、模板覆盖、验收边界）。
