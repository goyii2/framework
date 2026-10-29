# docs

goyii/framework 使用文档目录（面向使用者：部署、配置、使用说明）。

需求、决策等开发过程文档见 [dev-docs/](../dev-docs/)。

## 文档索引

| 文档 | 内容 |
|---|---|
| [安装](安装.md) | 环境要求、获取框架、包与 Yii2 命名空间对应表 |
| [快速上手](快速上手.md) | 从零跑起 examples/basic：浏览器链路 + JSON API + 一键验收 |
| [配置参考](配置参考.md) | 全部配置段落、字段、默认值与环境变量 |

## 组件使用说明

| 文档 | 组件 |
|---|---|
| [组件：base 与 config](组件-base与config.md) | Application 组件容器、事件、Module、配置装载 |
| [组件：web 路由与中间件](组件-web路由与中间件.md) | Handler/Context/Action、显式路由 + 约定回退、Filter 族、错误处理、反向 URL |
| [组件：session 与认证](组件-session与认证.md) | 会话后端、Identity/User 登录态、无状态认证、Csrf |
| [组件：db 与 ActiveRecord](组件-db与ActiveRecord.md) | 连接、泛型 AR 查询链、Save/Delete、事务、钩子 |
| [组件：validators](组件-validators.md) | 场景 -> zog schema、保存链路自动触发、消息翻译联动 |
| [组件：migrate](组件-migrate.md) | 八命令、迁移编写、方言要点 |
| [组件：log](组件-log.md) | 分级/分类、多 Target 轮转、慢查询、profile |
| [组件：cache](组件-cache.md) | memory/file/redis 后端、依赖失效、GetOrSet |
| [组件：rbac](组件-rbac.md) | DbManager 层级与判定、规则、缓存失效、与授权联动 |
| [组件：i18n](组件-i18n.md) | JSON 消息源 embed、语言协商链、Formatter |
| [组件：dataprovider](组件-dataprovider.md) | 分页排序参数白名单、Active/ArrayDataProvider、Result |
| [组件：behaviors](组件-behaviors.md) | Timestamp / Blameable / Sluggable / OptimisticLock |
| [组件：mail](组件-mail.md) | 消息链式构建、log/SMTP 驱动 |
| [组件：view 与 templ](组件-view与templ.md) | 视图注册表、Layout 组合、Widget 四件、分页联动 |
| [组件：debug 面板](组件-debug面板.md) | 开发期请求内省、七面板、toolbar 注入 |
| [组件：vite 前端构建](组件-vite.md) | manifest 解析、Entry 标签、dev 直连与反向代理（HMR） |
| [组件：gii 代码生成器](组件-gii代码生成器.md) | model / controller / crud 三命令、模板覆盖、gofmt 产物 |
| [组件：authclient 第三方登录](组件-authclient第三方登录.md) | GitHub/Google provider、state 防 CSRF、回调 -> 本地用户映射 -> 登录 |
