# 项目自有配置与代码装配（配置所有权反转）

## 背景

ADR-0001 确立了强类型 Application，但配置仍归框架所有：`config` 包持有大一统 `Config`（app/web/db/redis/cache/… 全部段落）与 `Load` + `WithDefaults`，Application 在内部按这份配置懒构建组件。实际使用中暴露出痛点：

- **自定义组件或自定义配置段要改框架源码**——大一统 `Config` 是装载器能填的唯一形状，项目自己的段（如 transactional-mail 凭据）无处安放；
- **默认值分裂**：一部分在 `WithDefaults`（装载器），一部分散在组件内部，回答「这个字段默认是什么」要看两处；
- **错误延迟暴露**：非法配置产出 failingMutex / failingMailer 这类「首用才报错」的假组件，boot 期绿灯、请求期炸雷；
- **内部消费者各开各的连接**：migrate / gii / queue worker 自行按配置开库，与请求进程的连接各走各的；web / console 的行为开关埋在配置里，装配路径不可见。

## 决策

**配置所有权反转：配置由具体项目定义并持有，框架组件在应用入口用代码装配。** 框架降格为「组件包 + 泛型装载工具 + 强类型 Application」，不再拥有任何配置格式。

### 1. 配置层：`config` 降格为纯叶子工具包

只剩 `LoadAs[T](dir)` 与 `Env()`，零框架依赖，任何组件包不得 import 它。三台机器作用于任意项目自定义 struct：

1. `GOYII_ENV` 选环境（默认 `dev`）；
2. `app.yaml` 基础值 + `web-<env>.yaml` overlay（overlay 只覆盖点名字段，其余保基值）；
3. 反射 env 覆盖：字段路径大写下划线（`DB.DSN -> DB_DSN`、`Email.APIKey -> EMAIL_APIKEY`）。

装载器不填任何默认值；项目不用这个装载器（自选 viper / koanf / flags）完全合法，框架核心不依赖该包。

### 2. 组件包统一初始化（配置结构体为主，函数选项只管依赖）

1. 参数包归组件包所有：`cache.Config`、`db.Config` 等定义在消费它的组件旁边，yaml tag 保留；
2. 每包唯一构造器 `New(cfg Config, opts ...Option)`：**值全走 Config，跨组件依赖（`WithRedis` / `WithDB` / `WithLogger` 族）全走 Option**；
3. 错误一律构造期返回 `(T, error)`，废除 failingMutex / failingMailer 式延迟失败；
4. 默认值内聚于 `New`，**零值 Config = 默认行为**；
5. 无值组件（i18n 等）免 ceremony，直接 `New(deps...)`；queue 驱动 switch 从 base 迁回 queue 包。

程序化底层构建件（`log.NewWithTargets`、`cache.NewMemory`、`queue.NewWithDriver`…）与 `New` 并存——它们是绕开配置形状直接拼装的正源，不是待清理过渡体（`log.NewWithTargets` 为原 `log.New` 改名，console 测试在用）。

### 3. Application：身份 + 急切装配

`base.New(opts...)` 不收配置，身份三件套 `base.WithApp(name, env, debug)`。Session 升格一等组件槽，Redis 槽直收 go-redis `UniversalClient`。

- **全急切**：`New` 即装好全部兜底默认，main.go 逐个构造真组件、处理错误后 `SetXxx` 装入；构造器只校验、不做网络 IO（go-redis / gorm 懒拨号，急切装配不拖慢 boot）。
- **访问器零错误**；未装配即访问 db / redis = panic（明确指名 `SetDB` / `SetRedis` 的 programmer error 消息）。
- 兜底默认清单：log→console(info)、cache→memory、queue→sync、session→file、mutex→process、mail→log driver、i18n→空消息源；**db / redis 无兜底**。`db.Component` 懒 holder 保留为「想要懒 / 可重试语义」的可选件，不是主装配路径。
- 内部消费者重定向：migrate / gii 改用 `app.DB()` 共享连接，queue worker 用 `app.Queue()`，console serve 地址由装配方注入（`console.WithListen`），web 的 CSRF / 约定回退开关改显式 option（`web.WithoutCsrf()` / `web.WithoutConventionFallback()`）。
- 自定义组件官方姿势 = **项目自持 struct + 构造注入**（控制器字段注入 auth manager 的模式为正源），不引入字符串注册表。

## 后果

正面：

- 自定义段、自定义组件、换装载器，**零框架改动**；框架源码不再随项目需求膨胀；
- 配置错误全部在 boot 期暴露（构造期返回 error），请求路径没有「首用炸雷」；
- 访问器零错误，请求代码无 err 分支噪音；「零值 = 默认」让默认值只有一处答案（组件的 `New`）；
- 装配路径全部显式可见：main.go 一屏看完哪个组件、哪个驱动、哪条连接；
- 值（yaml）与行为（代码）彻底分轨，配置漂移靠构造期校验兜住。

负面 / 代价：

- main.go 变长：每个组件一行构造 + 一行 Set + 错误处理，这是显式装配的固有成本；
- 反射 env 覆盖只支持标量与 `time.Duration`（slice / map 不覆盖），字段路径拼写错误不会报错、只是静默不生效；
- env 变量名派生自**项目 struct 的字段路径**——项目改名嵌入字段，env 名跟着变，运维侧需同步；
- panic 语义要求装配顺序纪律：`SetDB` 之前任何路径访问 `app.DB()` 直接崩；
- 项目自担配置结构演化（版本迁移、字段废弃），框架不再提供大一统 schema。

## 落地中的再决策

- `log.NewWithTargets` **保留**（原程序化 `log.New` 改名）：与 `cache.NewMemory` 等同位的程序化构建件，console 测试在用，不是过渡体；
- queue 的 `WithDBFunc` / `WithRedisFunc` 懒注入位**拆除**：01/02 过渡期为 base 保「首用报错」语义所需，03 的 Application 重构后 base 默认队列是纯 sync，直接值注入（`WithDB` / `WithRedis`）回归唯一形态；
- examples/basic **有意不演示 `SetRedis`**：示例保持零外部服务足迹（sqlite / file / log driver），装配一个无人消费的 redis client 属于死代码；槽位行为已由 base 测试覆盖（未装配 panic、`SetRedis(nil)` 回未装配态）。

## 取代关系

**部分取代 ADR-0001**：强类型 Application 保留并强化（组件仍是强类型字段 + 代码组装，不复刻 ServiceLocator / DI 容器的立场不变）；但 0001 的「组件懒初始化」语义被急切装配取代——兜底默认在 `New` 期就位、真组件由 main.go 构造后 `SetXxx` 替换，懒 holder 降级为可选件（`db.Component`）。Filter 映射 HTTP 中间件、`context.Context` 贯穿请求链路等其余内容不变。

## Considered Options

- **config 包去留**：保留纯叶子工具（**选定**）；彻底删除（rejected：示例即模板，等于让每个项目复制 overlay + env 两台机器）；并入 base（rejected：装载工具与组件容器职责混淆）。
- **组件初始化方式五选一**：裸结构体字面量（rejected：无构造语义，无法校验）；裸构造函数族（rejected：驱动 × 参数组合爆炸）；**配置结构体为主**（选定：值有形状、可 yaml、可 env）；**函数选项为辅**（选定，仅限依赖注入位：依赖是值不是数据）；建造者（rejected：非 Go 惯用法）。
- **装配时机三选一**：**全急切**（选定：错误 boot 期全暴露，访问器零错误）；全懒 + 错误访问器（rejected：调用点处处 err）；急切为主 + db 特例（rejected：破坏统一性，特例会繁殖）。
