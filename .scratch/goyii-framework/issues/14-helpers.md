# 14: helpers 包

**Status:** claimed

**What to build:** 惯用帮助函数包：Array/Str/File/Json/Url/Html 的 Yii2 高频能力对应物（数组取值/合并/多维查询、字符串转写、文件查找/复制、JSON 安全编码、URL 与 HTML 生成），命名遵循 Go 惯用法而非 PHP 下划线风格。

**Blocked by:** 01

- [ ] 各 helper 单测覆盖 Yii2 高频使用场景
- [ ] 命名与 Go 惯用法一致（包限定短名，如 helpers.ArrayGet）
- [ ] Html 输出默认转义，显式 raw 才输出原文
