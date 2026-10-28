# goyii/framework 工单规格

Go 重写 Yii2 框架：概念保真 + Go 惯用法，P0/P1 分期实施，P2 待拷问。本文件仅作索引；需求与决策以以下文档为准，勿在此复制内容：

- 需求：`dev-docs/PRD.md`
- 决策：`dev-docs/adr/0001`-`0004`
- 术语：根目录 `CONTEXT.md`

## 工单索引

- P0（01-15）：仓库骨架 -> Application/配置 -> 事件/Module -> HTTP 链路 -> 路由 -> console -> DB/迁移 -> AR -> 验证 -> 中间件 -> 认证 -> log/cache/helpers -> P0 集成
- P1（16-22）：templ 视图 -> RBAC -> i18n -> DataProvider -> 行为钩子 -> mail -> P1 集成
- 占位（23）：P2 规划拷问

实施边界：blocked-by 全部完成即可开工（frontier）；编号即依赖序。
