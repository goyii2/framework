# 自建轻量 DbManager 保持 Yii2 RBAC 语义，不采用 Casbin 作默认

Casbin 的策略模型（`p, 角色, 资源, 操作` 扁平行 + 主体侧角色继承）与 Yii2 RBAC 的权限树（item 沿 DAG 回溯判定、Rule 挂节点执行）存在结构性错位，门面层会长期漏气；且 casbin/v2 连带 govaluate 表达式引擎，对四张表即可解决的问题是重型依赖。决定：自建轻量 DbManager（`auth_item` / `auth_item_child` / `auth_rule`(仅存名) / `auth_assignment`，GORM 访问，item 图内存缓存），`AuthManager` 定义为接口，Casbin 适配器留作后续/社区扩展位。

## Considered Options

- Casbin + Yii2 词汇门面（rejected：权限树与策略表、Rule 与 matcher 函数两组语义无法同时成立；用户最终要对抗两套模型）

## Consequences

- 需要 ABAC/多租户域/分布式策略同步的业务直接集成 Casbin，而非经 goyii 门面
- Yii2 的 Rule 类不复刻：规则只在代码中定义，库中存名字，启动时注册表对齐
