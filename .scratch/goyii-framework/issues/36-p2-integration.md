# 36: P2 集成验收

**Status:** ready-for-agent

**What to build:** P2 端到端收口（对齐 P0/P1 集成票先例）：examples/basic 增嵌套 admin demo、queue demo（db 驱动 seed + drain）、PageCache 演示页、列表/表单页重构为 GridView/ListView/ActiveForm、debug 面板开启、最小 `web/` 前端工程（构建产物 manifest.json 与 hash 资源入库，head 经 vite.Entry 引用，Node 仅构建期需要）；docs/ 补齐 P2 组件使用文档；one-shot 校验全绿；resolve 23 号票。

**Blocked by:** 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35

- [ ] examples/basic：嵌套 admin demo、queue demo、PageCache 页、三件套重构、debug 面板——全部可浏览器访问验收
- [ ] Vite 最小前端工程：构建脚本与产物入库，head 引用生效
- [ ] docs/ 覆盖 queue/mutex/PageCache/大件/debug/Gii/Vite/嵌套模块（authclient 教程随 35）
- [ ] one-shot 校验脚本全绿（对齐 15/22 先例）
- [ ] 23 号票 resolve（共识、wontfix、拆解三项均完成）
