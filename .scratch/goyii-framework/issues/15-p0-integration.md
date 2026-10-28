# 15: P0 集成验证（examples/basic）

**Status:** claimed

**What to build:** examples/basic 以 SQLite 零依赖跑通 P0 全链路：配置装配 -> migrate 建表 -> seed -> 登录 -> 带验证的 CRUD 端点（错误处理/中间件/日志/缓存全部参与）；docs/ 起步（安装与快速上手）。

**Blocked by:** 05, 06, 07, 08, 09, 10, 11, 12, 13, 14

- [ ] 新环境按 docs/ 快速上手从零跑起 examples/basic，无需外部服务
- [ ] 演示链路覆盖：迁移、seed、登录、CRUD（含验证失败与错误格式）、中间件生效、日志与缓存可见
- [ ] docs/ 安装与快速上手文档完成
- [ ] 一键脚本跑通全部 P0 测试
