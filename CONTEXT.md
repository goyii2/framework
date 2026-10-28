# CONTEXT

项目术语表（固定在仓库根目录）。领域概念在此定义；条目由 `/domain-modeling` 在术语真正落定时懒添加，不预先铺满。

## 术语

### goyii

用 Go 重写的 Yii2 风格 Web 框架（本仓库交付物）。保留 Yii2 的架构概念与心智模型，以 Go 惯用法实现。

### Application（应用）

进程级应用实例，持有并管理全部核心组件（数据库、缓存、日志、会话等）的生命周期。请求作用域状态经 context 随请求传递，不做全局可变单例。

### Component（组件）

由 Application 管理的功能单元统称（db、cache、log、session 等）。goyii 中组件是 Application 的强类型字段，可注入、可替换，不做字符串键定位。

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

<!-- 条目格式建议：

### 术语名（EnglishIdentifier）

一句话定义。

- 避免：本项目中容易混淆、不应使用的同义词

-->
