# 06: console 骨架（cobra）

**Status:** ready-for-agent

**What to build:** cobra 命令行骨架：`goyii` 根命令、`goyii serve` 启动 HTTP 服务（优雅停机）、用户命令注册位；命令与应用装配复用同一份 config，CLI 与 Web 共享同一 Application。

**Blocked by:** 04

- [ ] `goyii serve` 启动并处理请求，收到退出信号优雅停机（在途请求完成）
- [ ] 用户自定义命令经 Application 注册为子命令
- [ ] `--help` 输出全部内建子命令
- [ ] serve 与 Web 应用共用组件装配（同一 config 来源）
