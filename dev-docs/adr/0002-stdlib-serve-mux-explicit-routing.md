# 标准库 ServeMux 显式绑定路由，放弃 Yii2 规则匹配与 chi

Yii2 UrlManager 的"规则表逐条正则匹配"是每请求 O(规则数) 的老范式，现代框架（含 Yii3 的 yiisoft/router）均已改为显式绑定 + 树形匹配。决定：路由基于 Go 1.22+ `net/http.ServeMux`，启动时显式注册（`r.Get("/post/{id:[0-9]+}", handler).Name("post/view")`，名字可选），反向生成按命名表查找、约定拼装兜底；未显式声明的路由由可选的 `/{controller}/{action}` 约定回退解析（查注册表，O(1)）。

## Considered Options

- 保留 Yii2 规则语法与顺序匹配语义（rejected：性能范式过时，且 Yii2 自身在 Yii3 也已放弃）
- chi（rejected：不支持内联参数正则，其相对标准库 mux 的剩余价值不足以引入依赖）

## Consequences

- 反向 URL 生成的参数使用专用类型 `url.P` 而非裸 map
- 约定回退默认开启、可配置关闭（性能敏感场景）
