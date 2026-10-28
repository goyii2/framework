# 10: Filter 族横切中间件

**Status:** ready-for-agent

**What to build:** 六件横切中间件（VerbFilter/Cors/ContentNegotiator/Csrf/RateLimiter/HttpCache，即 Yii2 Filter 族的 Go 落地）+ Controller 级 `Middleware()` 挂接机制 + 应用级全局链。Csrf 对齐 Yii2 默认：会话表单开启、纯 API 应用可全局关闭。

**Blocked by:** 05

- [ ] 六件中间件各自单测 + httptest 行为验证（如 VerbFilter 返回 405、Cors 预检正确响应）
- [ ] Controller 声明的中间件仅作用于该 Controller 的 action
- [ ] 全局链先于 Controller 级执行，顺序确定且可预测
- [ ] Csrf 默认开启可配置关闭；token 校验失败拒绝请求
