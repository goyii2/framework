# 06: 文档与决策记录

Status: resolved
Blocked by: 05

**What to build:** 按 spec 第 6 节产出文档：ADR 0002（project-owned-config-and-code-wiring，部分取代 0001——强类型 Application 保留；记录配置所有权反转、统一初始化五规则、急切装配语义、config 纯叶子工具包的取舍及否决项）；docs/配置参考.md 重写为 LoadAs + overlay + env 覆盖 + 组件参数包指南；docs/组件-base与config.md 同步访问器 / 装配 / 默认清单。文档内容与 05 落地后的真实 API 一致。

- [x] dev-docs/adr/0002 落盘：背景 / 决策 / 后果 / 取代关系与备选否决理由齐备
- [x] docs/配置参考.md 以 LoadAs 与组件参数包为主轴重写，含环境变量覆盖与 overlay 说明
- [x] docs/组件-base与config.md 与最终访问器 / 急切装配 / 默认清单 / panic 语义一致
- [x] 文档示例代码与 examples/basic 实际代码一致（无虚构 API）

## Comments

- 2026-10-30 实施记录（分支 feat/project-owned-config-06，已 --ff-only 合入 main，commit e161514；claim 提交 5579e3b）：
  - **ADR 编号偏离（0002 -> 0005）**：spec 与本票写「ADR 0002」，但规划基线提交 ff1cc38 已含 0002-0004（stdlib-serve-mux / self-built-rbac-dbmanager / yiimigrate-merged-raw-ddl）；按 dev-docs/adr/README.md「编号从 0001 递增」的唯一性约定顺延为 **0005-project-owned-config-and-code-wiring.md**。ADR 内容按 spec 第 6 节齐备：背景（0001 后使用痛点，含自定义段/组件需改框架源码）/ 决策（所有权反转、初始化五规则、急切装配语义、config 纯叶子化）/ 后果（正负两面）/ 部分取代 ADR-0001（强类型 Application 保留，懒初始化被急切装配取代）/ Considered Options（config 去留三选、初始化五选一、装配时机三选一，取自 spec 备选与否决记录）。落地中的三项再决策按 01-05 Comments 入册：log.NewWithTargets 保留（程序化构建件非过渡体）、queue WithDBFunc/WithRedisFunc 拆除（03 后直接值注入回归唯一形态）、examples/basic 有意不演示 SetRedis（零外部服务足迹，死代码不装）。
  - **docs/配置参考.md 重写**：以 config.LoadAs[T] + 组件参数包为主轴——加载流程 Mermaid 图、三台机器规则（GOYII_ENV / overlay 局部覆盖 / env 命名派生自项目 struct 字段路径、空值=未设置、标量与 duration、值错报错、非 struct 目标报错）、项目 Config struct 写法（身份字段 + 参数包直嵌 + 自定义段，对照 examples/basic/config/config.go）、七包 Config 字段/零值默认总表（cache/mutex/queue/db/session/mail/log，标注依赖注入位与缺依赖构造期报错）、i18n 项目自持段说明、examples/basic 对照表。旧版大一统段落表（app/web/redis 顶层段、WithDefaults 层、redis 覆盖子段语义）全部删除。
  - **docs/组件-base与config.md 同步**：Application 节重写为 base.New(opts...) + WithApp 身份、急切装配 Mermaid 图与 main.go 装配序列、访问器/兜底默认清单表（db/redis 无兜底 panic、SetXxx(nil) 两档语义）、ConnectDB 与 QueueDispatcher 便捷件、Session/Redis 槽（共享连接注入姿势）、内部消费者共享 app 连接节（migrate/gii 用 app.DB()、queue worker 用 app.Queue()、serve 地址 console.WithListen 注入、web 显式 option）。已删除的 RedisFor/懒初始化/大一统 config 引用全部清除；Mutex/Queue/事件/Module 节保留并同步驱动注入姿势。
  - **验收证据（纯文档票，无代码改动；三条文档全部事实对照 main 实码先读后写）**：文档引用的 API 逐一在 main 代码核实——LoadAs/Env/EnvVarName（config/load.go）、WithApp/New/访问器/Set 族/ConnectDB/QueueDispatcher/SetDefault（base/*.go）、七包 Config 字段与默认值（cache/mutex/mail/log/db/queue/session 各 config.go 与 manager.go）、queue.NewMigration、db 驱动别名表（空=mysql）、mail SMTP 端口/加密默认、NewGormLogger slow<=0 关慢查询、web.WithoutCsrf/WithoutConventionFallback、console.WithListen、queue 五个 PushOption；装配序列逐行对照 examples/basic/main.go（LoadAs -> log.New -> base.New(WithApp) -> ConnectDB -> cache/mail/queue/session -> SetDefault -> SetI18n）与 config/config.go / app.yaml / web-dev.yaml。符号规范 grep：三文件零命中 → ∧ ∨ ∈ ↔ § 与已删 API（RedisFor / config.Load( / WithDefaults / failingMutex）；图表全 Mermaid；逻辑表达式用 -> / == / =>。`git diff 40c122b..e161514 -- '*.go'` 为空（零代码改动），Go 构建不受影响。
  - 遗留（本票范围外，未动）：docs/组件-session与认证.md:83 与 docs/组件-web路由与中间件.md:85 仍提旧配置开关 `web.disableCsrf`（现行为 web.WithoutCsrf() option），建议后续小票清理。
