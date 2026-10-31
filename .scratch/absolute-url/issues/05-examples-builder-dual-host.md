# 05: examples 迁移 builder + advanced 双域名

Status: claimed

**What to build:** 两个示例迁到 builder 形态并演示双域名。basic：文章页 Permalink 触点从 `url.ToAbsolute` 迁到 `url.Build(...).Absolute()`（单域名，默认 host）。advanced：`Config.Web` 增 `BackendHostInfo`（yaml `backendHostInfo`，env `WEB_BACKEND_HOSTINFO` 覆盖，prod overlay 插值——形态与既有 `HostInfo` 一致）；backend 分区的 Permalink 触点用 `url.Build(...).Host(cfg.Web.BackendHostInfo).Absolute()` 演示跨分区显式域（后台在独立域名的部署形态），frontend 触点走默认 `.Absolute()`。装配代码把 backend host 传进 backend 分区（装配函数注入，不引全局）。既有相对链接/重定向/表单 action 保持不变。

**Blocked by:** 04

- [ ] basic 触点迁移 `url.Build(...).Absolute()`，行为与断言不回退
- [ ] advanced `BackendHostInfo` 配置全链路（yaml 基值 + env 覆盖 + prod overlay 插值）
- [ ] backend 触点 `.Host(backendHost).Absolute()` 生效：渲染输出为 backend 域名的完整网址
- [ ] 既有绝对链接断言测试迁移 builder 形态并覆盖 backend 显式域触点
- [ ] 既有相对链接/重定向/表单 action 零改动；两示例构建通过，全量测试绿
