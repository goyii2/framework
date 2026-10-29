# 34: Gii 代码生成器

**Status:** ready-for-agent

**What to build:** 代码生成器命令 `goyii gii model|controller|crud`（gii 包）。model 经 GORM Migrator 读表结构，生成内嵌 ar.Model 的 struct + `Rules()` 骨架；controller 生成骨架并打印路由注册片段；crud 生成 model + controller + `.templ` 视图（list 用 GridView、form 用 ActiveForm）并提示运行 templ generate。text/template + embed 内置模板，`--templates` 目录同名覆盖；纯 flags 无交互向导；产物统一 gofmt。

**Blocked by:** 30, 31

- [ ] `gii model --table`：字段/类型/主键正确；Rules() 骨架按 NOT NULL->required、长度->max 猜测，不确定留空注释
- [ ] `gii controller --id`：骨架 + 路由注册片段打印
- [ ] `gii crud --table`：list（GridView + LinkPager）/ form（ActiveForm）/ delete 动作 + 视图注册代码，提示 templ generate
- [ ] input 类型映射：TEXT->textarea、数值->number、bool->checkbox、时间->datetime-local、其余 text
- [ ] `--templates` 覆盖机制（同名文件替换）；产物统一 gofmt
- [ ] 验收：对 examples/basic 的表执行生成，产物可编译（不入库）
