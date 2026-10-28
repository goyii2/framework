# 04: 最小 HTTP 链路（tracer bullet）

**Status:** ready-for-agent

**What to build:** 第一条端到端链路：Application 挂载 http.Handler，开发者以 `ActionXxx(ctx *web.Context) error` 形态编写 Controller（默认约定 `post/view` -> `PostController.ActionView`），action 返回 JSON 或重定向；返回的 error 由统一错误处理器按 Accept 协商渲染 JSON/HTML（web.NotFound 等构造 HTTP 异常）；03 的生命周期事件在此真实接线。

**Blocked by:** 02, 03

- [ ] 示例 Controller action 经 curl 返回 JSON
- [ ] web.NotFound 等异常返回对应状态码，格式随 Accept 协商
- [ ] web.Context 提供 Request/Response 基础访问与 JSON/Redirect
- [ ] BeforeRequest/AfterRequest 在真实请求中触发
- [ ] httptest 覆盖正常与异常路径
