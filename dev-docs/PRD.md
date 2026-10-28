# PRD — goyii/framework

用 Go 重写 Yii2 框架：保留 Yii2 的架构概念与心智模型，以 Go 惯用法实现。本文经 `/grill-with-docs` 多轮拷问达成共识后落盘；配套术语表见根目录 `CONTEXT.md`，决策记录见 `dev-docs/adr/`。

## 背景与问题

Yii2 的组件化 Application、ActiveRecord、约定式路由、RBAC、i18n 等架构经过长期验证，但绑定 PHP 运行时。本项目在 Go 生态提供同款开发体验：熟悉 Yii2 的开发者按原有心智模型直接写 Go，同时享受 Go 的类型安全、并发模型与单二进制部署。

移植策略为**概念保真、惯用法落地**：Yii2 架构概念全集保留，但依赖 PHP 动态特性（运行时属性、动态挂接、数组配置实例化）的机制不复刻，改用 Go 接口、泛型与显式注册表达。

## 目标 / 非目标

**目标**

- 覆盖 Yii2 模块全集（按 P0/P1/P2 分期，全部规划在内）
- Yii2 使用者心智无缝迁移：包名对应 Yii2 命名空间，类型名沿用（Controller、ActiveRecord…）
- 准开源质量：公共 API 当稳定承诺对待，P0 全模块带单元测试，`docs/` 提供使用文档

**非目标**

- 与 PHP 代码互操作或提供兼容层
- 复刻 PHP 动态机制（见"显式放弃清单"）
- 以跑分夺魁为目标（性能以真实业务够用为准）

## 总体架构

```mermaid
flowchart LR
    C[客户端] --> H[net/http Server]
    H --> MW[中间件链<br/>Filter 族: AccessControl / VerbFilter / Cors /<br/>ContentNegotiator / RateLimiter / 认证]
    MW --> R["Router 显式绑定<br/>ServeMux {id:[0-9]+} + 命名路由 + 约定回退"]
    R --> M[Module 分区 /admin 等]
    M --> A["Controller.ActionXxx(ctx *web.Context) error"]
    A --> V[validators / zog<br/>按场景校验]
    A --> AR[ActiveRecord 泛型层<br/>ar.New[T] + GORM 钩子]
    AR --> G[GORM]
    G --> DB[(数据库)]
    A --> T["templ 视图注册表<br/>Layout 组合 / JSON"]
    subgraph APP ["Application: 强类型组件容器, 经 context 携带"]
        DB2[db] CT[cache] LG[log] SS[session] AU[auth] I1[i18n] U1[urlManager]
    end
    A -. FromContext .-> APP
    MIG["migrate（yiimigrate 并入, database/sql 直连）"] --> DB
```

请求链路：`net/http -> 中间件链 -> Router（显式绑定优先，约定回退兜底）-> Controller Action -> 验证/AR/视图`。请求作用域状态（当前用户、语言）经 `context.Context` 传递；组件由强类型 Application 持有、懒初始化。DDL 归 migrate 子系统（database/sql 直连），DML 归 GORM，两者共用 DSN。

## 技术选型

| 领域 | 选型 | 说明 |
|---|---|---|
| Go 版本 | go 1.25+ | 本机 1.27 开发；完整泛型 + slog + 成熟 ServeMux |
| HTTP 引擎 | `net/http.ServeMux`（1.22+ 语法） | 显式绑定 `{id:[0-9]+}`；chi 出局（见 ADR-0002） |
| ORM | GORM | DML 与事务；模型生命周期经 GORM 钩子 |
| CLI | `github.com/spf13/cobra` | `goyii serve / migrate/* / ...` 子命令 + 用户命令注册 |
| 视图 | `github.com/a-h/templ` | 视图注册表 + Layout 组件组合（P1） |
| 验证 | `github.com/Oudwins/zog` | 按场景挂 Model，`字段 -> []消息` 错误形态 |
| 迁移 | yiimigrate 源码并入 | `database/sql` 直连不引 ORM（见 ADR-0004） |
| 日志 | `log/slog` | 分级/分类 + 多 Target（console/file）+ profile |
| RBAC | 自建轻量 DbManager | 四表 Yii2 语义（见 ADR-0003） |
| 缓存/会话 Redis | `github.com/redis/go-redis/v9` | cache 接口实现之一 |
| i18n 格式化 | `golang.org/x/text` | 日期/数字/货币 Formatter |

## 需求清单（分期）

### P0 核心骨架（可跑通完整 Web 应用）

- **base**：Application（强类型组件容器 + 懒初始化 + context 携带 + 包级默认实例）、Module（单层路由分区，嵌套接口预留）、轻量事件调度（类型化 `On/Trigger`，应用级与自定义事件；模型生命周期事件不走它）
- **config**：Config struct 双轨——值走 yaml 多环境 + 环境变量覆盖，行为走代码组装
- **web**：web.Context（Request/Response/Session/User/Flash + JSON/Render/Redirect）、Controller/Action（`ActionXxx(ctx *web.Context) error`，默认路由约定 `post/view` -> `PostController.ActionView`）、Router（显式绑定 + `.Name()` 可选 + 约定回退可关）、`url.To(name, url.P{...})`（命名路由表 -> 约定拼装兜底）、Filter 族中间件（AccessControl/VerbFilter/Cors/ContentNegotiator/RateLimiter/HttpCache/Csrf + HttpBasic/Bearer 认证）、User/Identity/Session（file/redis 后端）/Cookie、统一错误处理器（`web.NotFound` 等构造 HTTP 异常；输出按 Accept 协商 JSON/HTML）。Csrf 对齐 Yii2 默认：会话表单开启、纯 API 应用可全局关闭。AccessControl 在 P0 基于 Identity 的角色回调判定授权，P1 接 AuthManager 后端（接口不变）
- **db**：泛型 AR（`ar.New[T](ctx)`、`FindByID`、`Where/OrderBy/Limit/With/Join`、`Save/Delete/Reload`）、GORM 钩子承担 beforeSave/afterSave/afterDelete、事务
- **validators**：`Rules()` 返回 场景 -> zog schema 映射，`Validate(scenario)` 返回 `map[字段][]消息`，保存链路按 Insert/Update 场景自动触发，消息模板可自定义
- **log**：slog 底座，分级/分类、多 Target（console/file）、性能 profile
- **cache**：Cache 接口 + memory/file/redis 实现 + 依赖失效（Dependency）
- **helpers**：Array/Str/File/Json/Url/Html 等惯用帮助函数
- **console**：cobra 集成（`goyii serve`、migrate 子命令、用户命令注册位）、yiimigrate 并入（八命令 up/down/redo/history/new/mark/to/create、历史表 `migration`、方言锁、dry-run，DSN 从应用配置注入并兼容 `DB_DSN` 环境变量）

### P1 完整框架体验

- **view**：templ 视图注册表（`ctx.Render("post/view", data)` 按名查表，零反射；亦支持直接传组件）、Layout 组件组合、Widget（结构体配置 + 返回 templ 组件），内置最小集：LinkPager / Breadcrumbs / Menu / DetailView
- **rbac**：AuthManager 接口 + 自建 DbManager（`auth_item` / `auth_item_child` / `auth_rule`(仅存名) / `auth_assignment`，GORM 访问，item 图内存缓存）；规则（Rule）只在代码定义、库中存名字，启动时注册表对齐；`auth.Can/Assign/Revoke` + `user.Can()` 联动 AccessControl；Casbin 适配器留接口位不实现
- **i18n**：`messages/<lang>/<category>.json` + go:embed；消息 ID 用源语言字符串；Formatter 基于 x/text；源语言 en-US
- **dataprovider**：ActiveDataProvider / ArrayDataProvider；查询参数沿用 Yii2（`page` / `per-page` / `sort=-created_at,id`）；响应含分页元信息
- **mail**：Mailer 抽象 + SMTP 实现
- **常用行为**：Timestamp / Blameable / Sluggable / OptimisticLock，以 GORM 钩子与可复用结构体内嵌实现

### P2 生态（做，但最后）

queue、debug 调试面板、Gii 代码生成器（cobra 子命令生成 model/CRUD/controller）、Vite manifest 集成（hash URL + dev server 代理）、服务端渲染 Widget 的最小资源注册表（如出现）、redis/mongo/es 扩展对应物、mutex（进程内 + redis 分布式锁）、PageCache、GridView/ListView/ActiveForm 服务端渲染大件（客户端校验另议）、httpclient/authclient 等扩展对应物按需评估、嵌套 Module 完整验证、Casbin 适配器（社区位）

## 关键 API 形态（共识样例）

```go
// 应用装配：代码管行为，yaml 管值
app := goyii.New(goyii.LoadConfig("config/")) // db/cache/log/session 组件懒初始化

// 路由：显式绑定 + 可选命名
r := app.Router()
r.Get("/post/{id:[0-9]+}", postCtrl.ActionView).Name("post/view")
url.To("post/view", url.P{"id": 1}) // -> /post/1（未命名则约定拼装兜底）

// Controller
type PostController struct{ web.Controller }
func (c *PostController) ActionView(ctx *web.Context) error {
    p, err := ar.FindByID[Post](ctx, ctx.ParamInt("id"))
    if err != nil { return web.NotFound("post not found") }
    return ctx.JSON(p)
}

// AR + 场景验证
q := ar.New[User](ctx).Where("status = ?", 1).OrderBy("id DESC").Limit(10)
user.Validate(validators.ScenarioInsert) // map[field][]msg

// 迁移（yiimigrate 形态，并入后由 goyii migrate 驱动）
goyii migrate up && goyii migrate redo 1
```

## 显式放弃清单

| 放弃项 | 替代 | 依据 |
|---|---|---|
| Behavior 动态挂接 | GORM 钩子 + 结构体内嵌 | 动态挂接需反射/map，损类型安全 |
| ServiceLocator / DI 容器 | 强类型 Application + 构造注入 | ADR-0001 |
| UrlManager 规则逐条匹配 | 显式绑定 + 命名路由 + 约定回退 | ADR-0002（chi 同时出局） |
| AssetManager | go:embed 直接引用；P2 Vite manifest 触发式补 | 资源发布问题在 Go 下不存在 |
| Casbin 作默认 RBAC | 自建 DbManager + AuthManager 接口留适配位 | ADR-0003（语义结构性错位） |
| Yii2 Rule 类（库存可执行规则） | 规则代码内定义、库中存名字 | Go 无代码序列化执行，存名对齐 |
| GORM AutoMigrate | yiimigrate 版本化 DDL | ADR-0004 |

## 工程约定

- 单 Go module，module path `github.com/goyii2/framework`（GitHub org `goyii2`：`goyii` 已被 2016 年注册的空置个人账号占用，`goyii2` 为实测可用的最短对应名；org 由用户自行创建）；包名对应 Yii2 命名空间：`base/web/db/log/cache/rbac/i18n/console/helpers/validators/migrate/dataprovider`
- P0 全模块单元测试；golangci-lint；公共 API 按稳定承诺对待
- 交付物含 `examples/basic`（migrate + CRUD + 登录 + templ 页面）与 `docs/` 使用文档（本目录分工见仓库 AGENTS.md）
- 决策记录进 `dev-docs/adr/`，术语进根目录 `CONTEXT.md`

## 开放问题

- P2 各项（queue 语义、Gii 模板形态、Vite 集成深度、多存储扩展范围）进入实施前单独拷问
- yiimigrate 并入后原仓库去留（归档或停止演进）——待用户决定
- 错误处理器输出协商的细节规则（Accept 头优先级、HTML 错误页模板覆盖机制）实现时定
- `web.Context` 与 `context.Context` 的互转便利方法命名，实现时定
- yaml/env 多源配置加载库选型（koanf / viper / 轻量自研绑定），实现时定
- 配置文件命名方案（草案：`config/app.yaml` + `config/web-<env>.yaml` + 环境变量覆盖），实现时定
- examples/basic 演示数据库选型（建议 SQLite 零依赖启动；yiimigrate 已支持 SQLite 方言）
