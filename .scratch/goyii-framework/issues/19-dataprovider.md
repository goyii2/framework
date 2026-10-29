# 19: DataProvider

**Status:** claimed

**What to build:** 数据提供者：ActiveDataProvider（包装 AR 查询自动应用分页排序，仅取当页数据）与 ArrayDataProvider（内存切片）；查询参数沿用 Yii2 惯例 `page`/`per-page`/`sort=-created_at,id`；响应含 items + 分页元信息（totalCount/pageCount/当前页），供 API 与 LinkPager（16）共用。

**Blocked by:** 08

- [ ] ActiveDataProvider 分页排序下推到 SQL（只查当页与总数）
- [ ] ArrayDataProvider 对内存切片分页排序
- [ ] 参数越界（超总页数/非法 sort 字段）行为定义且安全（不透传到 SQL）
- [ ] 分页元信息形态与 LinkPager 联动验证
