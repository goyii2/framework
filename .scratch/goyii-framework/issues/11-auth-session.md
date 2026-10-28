# 11: 认证与授权（P0 形态）

**Status:** ready-for-agent

**What to build:** User/Identity 认证与会话：Session（file/redis 后端）+ Cookie 管理、Identity 接口、HttpBasic/HttpBearer 认证中间件、会话登录/登出；AccessControl 以角色回调判定授权（P0 形态，P1 由 17 切换 AuthManager 后端，接口不变）。

**Blocked by:** 05, 10

- [ ] HttpBasic 与 HttpBearer 分别保护 action，未认证返回 401
- [ ] 会话登录后 FromContext 可取当前 Identity；登出后会话失效
- [ ] AccessControl 按角色放行/拒绝（403），角色来源为 Identity 回调
- [ ] session file/redis 后端行为一致（redis 用 miniredis 或测试容器验证）
