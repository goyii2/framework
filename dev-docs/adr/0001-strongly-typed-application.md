# 强类型 Application 取代 Yii2 动态组件机制

Yii2 的 ServiceLocator/DI 容器/动态 Behavior 建立在 PHP 运行时属性与数组配置实例化之上，Go 中模拟需要反射与 `map[string]any`，牺牲类型安全与性能。决定：不复刻这三者，`Application` 是强类型 struct，组件为接口字段 + 懒初始化，经 `context.Context` 贯穿请求链路（另有包级默认实例供 CLI/后台使用）；扩展方式就是 Go 本身的接口 + 代码组装。

## Considered Options

- 复刻字符串键组件注册（rejected：Yii2 心智完整但反射代价与类型安全损失不可接受）
- 引入通用 DI 容器如 fx/wire（rejected：框架级强依赖，且与"Application 即组件容器"的 Yii2 心智重复）

## Consequences

- 不存在运行时注册任意字符串组件的机制；第三方扩展通过实现接口 + 代码装配接入
- Filter 不属于放弃范围：它映射为标准 HTTP 中间件（零反射零成本），全套保留
