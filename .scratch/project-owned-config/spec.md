# spec：配置所有权反转 + 统一组件初始化（project-owned-config）

状态：共识已确认（grill-with-docs 会话定稿）。

## 一句话

配置由具体项目定义并持有，框架组件在应用入口用代码装配；框架降格为「组件包 + 泛型装载工具 + 强类型 Application」，不再拥有任何配置格式。

## 决策清单

### 1. 配置层

- `config` 包降格为纯叶子工具包：只有 `LoadAs[T](dir)` 与 `Env()`，零框架依赖，任何组件包不得 import 它。
- 保留三台机器：`GOYII_ENV` 环境选择、`app.yaml` + `web-<env>.yaml` overlay、反射 env 覆盖（任意项目 struct 的字段路径 → 大写下划线环境变量）。
- 删除大一统 `config.Config`、`config.Load`、`WithDefaults`；loader 不再填默认值。
- 项目不使用该 loader 完全合法（自选 viper/koanf 等），框架核心不依赖。

### 2. 组件包统一形态（配置结构体为主、函数选项只管依赖）

1. 参数包归组件包所有：各 section struct 迁入组件包（`cache.Config`、`db.Config`…），yaml tag 保留。
2. 每包唯一构造器 `New(cfg Config, opts ...Option)`；值全走 Config；跨组件依赖一律 `WithRedis` / `WithDB` / `WithLogger` 一族选项。
3. 错误一律构造期返回 `(T, error)`；废除 failingMutex / failingMailer 延迟失败。
4. 默认值在 New 内部处理，零值 = 默认。
5. 无值组件（i18n 等）免 ceremony，直接 `New(deps...)`。
6. queue 驱动 switch 从 base 迁回 queue 包。

### 3. Application

- `base.New(opts...)`，不收配置；身份三件套 `base.WithApp(name, env, debug)`。
- Session 升格一等组件槽（默认 file 后端）。
- 内部消费者重定向：migrate/gii/queue-worker 改用 `app.DB()` / `app.Queue()`（不再自行开连接）；console serve 地址由装配方注入；debug 面板 Config 摘要删除；web 的 CSRF/约定回退改显式 option。

### 4. 装配语义

- 全急切：main.go 逐个构造并处理错误后 `SetXxx`；访问器零错误；未装配即访问 = panic（明确 programmer error 消息）。
- 构造器只校验、不做网络 IO（go-redis / gorm 懒拨号，急切装配不拖慢 boot）。
- `db.Component` 懒 holder 保留为可选件（懒 / 可重试语义），不是主装配路径。
- 兜底默认：log→console(info)、cache→memory、queue→sync、session→file、mutex→process、mail→log driver、i18n→空消息源；**db / redis 无兜底**。

### 5. examples/basic 样板

- 项目自定义 Config struct（身份字段 + 复用组件参数包 + 自定义 Email 段），`EMAIL_APIKEY` env 覆盖与 `web-dev.yaml` overlay 各有一处真实演示。
- main.go 全急切装配（含 SetSession / SetRedis）。
- 自定义组件官方姿势 = 项目自持 struct + 构造注入（现有 SetAuthManager 模式为正源），不引入字符串注册表。

### 6. 文档与记录

- ADR 0002（project-owned-config-and-code-wiring，部分取代 0001：强类型 Application 保留）。
- docs/配置参考.md 重写为 LoadAs + 参数包指南；docs/组件-base与config.md 同步。
- CONTEXT.md 已增「项目配置 / 装配」两条术语（会话中已落盘）。
- 干净破坏：不留兼容 shim，examples 与测试一次迁移。

## 备选与否决记录

- config 包去留：保留纯叶子工具（**选定**）／彻底删除（否决：示例即模板，等于让每个项目复制 overlay+env 机器）／并入 base（否决：职责混淆）。
- 初始化方式五选一：结构体字面量（否决：无构造语义）／裸构造函数（否决：驱动×参数组合爆炸）／配置结构体（**主选**）／函数选项（**辅选**，仅限依赖注入位）／建造者（否决：非 Go 惯用法）。
- 装配时机：全急切（**选定**）／全懒 + 错误访问器（否决：调用点处处 err）／急切为主 + db 特例（否决：破坏统一性）。
