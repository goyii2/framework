# 35: authclient

**Status:** ready-for-agent

**What to build:** 第三方登录薄封装：基于 golang.org/x/oauth2 的 provider 接口（BuildAuthURL / Exchange / FetchIdentity）+ GitHub、Google 内置实现，与 web Identity 登录流联动（回调 -> FetchIdentity -> 既有 login 路径）。不进 examples 主线（避免外部凭据依赖）；交付为 docs 教程 + mock OAuth server 单测。

**Blocked by:** None (can start immediately)

- [ ] provider 接口 + GitHub/Google 内置（x/oauth2 基座）
- [ ] Identity 联动路径：回调 -> FindIdentity -> login，文档化
- [ ] docs/ 教程：配置 provider、回调路由、身份映射
- [ ] mock OAuth server 单测：授权跳转、code 交换、身份获取
