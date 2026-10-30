# CONTEXT

项目术语表（固定在仓库根目录）。领域概念在此定义；条目由 `/domain-modeling` 在术语真正落定时懒添加，不预先铺满。

## 术语

### goyii

用 Go 重写的 Yii2 风格 Web 框架（本仓库交付物）。保留 Yii2 的架构概念与心智模型，以 Go 惯用法实现。

### Application（应用）

进程级应用实例，持有并管理全部核心组件（数据库、缓存、日志、会话等）的生命周期。请求作用域状态经 context 随请求传递，不做全局可变单例。

### Component（组件）

由 Application 管理的功能单元统称（db、cache、log、session 等）。goyii 中组件是 Application 的强类型字段，可注入、可替换，不做字符串键定位。

### 项目配置（project config）

由具体项目定义并持有的配置数据：结构体、文件与取值方式均由项目自定，框架不规定其内容，仅提供通用加载工具（overlay 与环境变量覆盖）。

- 避免：框架配置——框架不再拥有大一统配置结构

### 装配（wiring）

在应用入口以代码将项目配置的值构造为组件并注入 Application 的过程；值来自项目配置，行为全在装配代码。

### Filter（过滤器）

请求进入 Action 前的横切处理（访问控制、CORS、动词限制等）。goyii 以 HTTP 中间件形式提供同类能力。

### Behavior（行为）

Yii2 中可运行时挂接到对象的横向能力。goyii 不复刻动态挂接机制；常用行为（时间戳、操作人填充、slug 生成等）以模型生命周期钩子提供。

### ServiceLocator / DI 容器（不复刻）

Yii2 的字符串键组件定位与依赖注入机制。goyii 不复刻，由强类型 Application 与构造函数注入取代。

### Controller（控制器）

按 URL 路由到的请求处理单元，聚合一组相关 Action。

### Action（动作）

Controller 内的最小可路由请求处理单元（如 `post/view` 中的 `view`）。Action 返回 error，由统一错误处理器接管渲染。

### Module（模块）

带路由前缀与独立 Controller 注册表的应用分区（如 `/admin`），可嵌套。

### Scenario（场景）

模型验证的上下文标签（如 insert / update / login），同一模型在不同场景下启用不同验证规则集。

### Active Record（活动记录）

以模型对应数据表、模型实例对应行的数据访问风格；模型生命周期各阶段（保存前后、删除后等）可挂接钩子。

### 命名路由（named route）

显式注册的路由可选携带的名字，沿用 Yii2 路由 ID 形态（如 post/view）；反向 URL 生成按名字查找。

### 约定回退路由（convention fallback）

未显式声明路由时按 controller/action 命名约定解析请求的兜底机制；可关闭。反向生成同样可按约定拼装兜底。

### DataProvider（数据提供者）

为列表场景统一封装取数、分页与排序的组件。

### RBAC（基于角色的访问控制）

以角色与权限为核心的授权模型：用户被赋予角色，角色与权限构成可继承的层级，访问判定沿层级向上回溯。

### 布局（Layout）

页面的公共外框（页头/页脚/资源引用位）。goyii 中以 templ 组件组合实现，而非运行时模板继承。

### 小部件（Widget）

可复用的界面组件单元（分页条、面包屑等）。goyii 中为“结构体配置 + 返回 templ 组件”的形态。

### 队列（Queue）

将任务延后异步执行的组件；任务代码不随队列出进程，按注册名在 worker 侧重建后执行。

- 避免：消息中间件统一封装（不做 AMQP/Kafka/SQS 适配）

### 队列任务（Job）

队列中的最小执行单元：具名注册、可携带数据载荷、可延迟、可设优先级与重试次数。

### 互斥锁（Mutex）

按名字获取的互斥机制，提供进程内与 Redis 跨进程两种实现；不可重入。

- 避免：Yii2 的可重入语义（Go 下不安全也不惯用，不复刻）

### 整页缓存（PageCache）

以过滤器形态对成功的 GET 文本响应整页缓存并回放、按依赖失效的机制。

### 活动表单（ActiveForm）

与模型校验错误回显联动的服务端表单渲染组件；客户端校验不在框架内做。

### 列表大件（GridView / ListView）

基于 DataProvider 结果渲染数据表格/条目列表的服务端大件；纯渲染、不自行取数。

### 调试面板（Debug Panel）

开发期的请求内省工具：按请求落盘历史、内嵌页面查看、页底工具栏注入；仅调试模式可用。

### 代码生成器（Gii）

从数据库表结构生成模型、控制器骨架与 CRUD 的命令行工具；纯参数无交互向导。

### 资源清单（manifest）

前端构建产物名到带 hash URL 的映射文件；框架按它引用资源，开发期直连 Vite dev server。

### advanced 示例（advanced template example）

对应 Yii2 高级模板的示例应用及其映射词汇：frontend=根作用域（非模块）、backend=`/backend` 模块（含嵌套子模块）、console=同一二进制内的命令、common=跨分区共享包、environments=按环境 overlay 的项目配置。

- 避免：前后台独立登录——以单一登录 + RBAC 角色守卫取代，示例文档注明该差异

<!-- 条目格式建议：

### 术语名（EnglishIdentifier）

一句话定义。

- 避免：本项目中容易混淆、不应使用的同义词

-->
