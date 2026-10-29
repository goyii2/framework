# 24: 示例文章列表搜索

**Status:** resolved

**What to build:** examples/basic 的文章列表页顶部加搜索框（GET 表单），查询参数 `q` 对文章标题与正文做包含匹配（不分词，LIKE 模式且转义 `%`/`_` 输入）；分页与列头排序链接保留 `q`；命中为空时列表区显示友好提示；`p1-check.sh` 冒烟补搜索断言（命中若干篇 + 未命中空集 + q 与分页共存）。

**Blocked by:** 22

- [x] 搜索框经 GET 表单提交 q，列表按 title/content 包含匹配过滤
- [x] LIKE 特殊字符（% 与 _）被转义，用户输入不改变匹配语义
- [x] 分页与排序翻页时 q 保留，不丢过滤条件
- [x] 空结果显示友好提示（而非空白或报错）
- [x] p1-check.sh 冒烟新增搜索断言并通过

## Comments

### 落地形态（main `384e692`，分支 feat/post-search 已 ff 合并并删除）

- **action 层过滤（examples/basic/pages.go）**：`q` 是过滤参数不是分页参数，不进 `dataprovider.Params`——`ActionList` 在基查询上叠加 `(title LIKE ? ESCAPE '\' OR body LIKE ? ESCAPE '\')`（注意 Post 的正文列是 `body`，工单描述里的 content 即此列），两处绑定同一个参数值。转义由 `likePattern` + `strings.NewReplacer` 单趟完成：`\` `%` `_` 全部转成 `\x` 字面量（escape 字符自身先转义，单趟替换保证不会被二次加工），ESCAPE 子句声明 `\` 为转义符。pattern 经占位符绑定，绝不拼接。q 做 TrimSpace，空白即无过滤。
- **基查询进 Provider 的方式**：带 Where 的 `ar.Query` 直接作为 `ActiveDataProvider.Query` 基查询——COUNT 与取页同塑（19 号既有契约"基查询同时作用于 COUNT 与取页"），总数/页码/钳制全部自动按过滤后集合计算，无需额外交互。
- **q 保留机制（沿用 22 号 sort 保留的同一手法）**：LinkPager 的 `URL func(page)` 与 `sortToggle` 生成的列头链接各自把 `q` 塞进 `url.P`，`url.To` 把路径参数以外的键按字母序编成 query string。sort toggle 点击仍重置到第 1 页（既有行为），pager 翻页保持当前 sort 与 q。`sortToggle` 顺带去掉了原本未使用的 `c *web.Context` 参数。
- **页面（views.templ + templ generate）**：列表页顶部 GET 搜索表单（`input[type=search]` 回显当前 q，placeholder "Search posts"），GET 表单不带 CSRF（Yii2 惯例：仅 POST 校验）；空结果区分两种提示——无过滤时保持 "No posts yet."，有 q 时显示 "No posts match your search."。新增三键进 zh-CN/app.json：`Search`/`搜索`、`Search posts`/`搜索文章`、`No posts match your search.`/`没有找到匹配的文章。`（en 为源语言，键即文案）。
- **单测（examples/basic/pages_test.go，包内首个测试文件）**：`TestLikePatternEscapesWildcards` 六个子测钉死转义契约（纯文本/%/_/转义符自身/混合/空串）。反向字符串以 `string(rune(92))` 构造，规避本机 Write 工具对反斜杠序列的转写歧义。
- **p1-check.sh 冒烟（55 -> 69 断言，新增 14 条）**：搜索框渲染于列表页（`name="q"`）；zh 会话粘住期间验证中文搜索文案（`>搜索</button>`）与空结果提示翻译（`没有找到匹配的文章。`）；搜索段造中文夹具后断言：中文关键词命中（q=中文搜索 -> Total 1 + 行内标题）、正文关键词命中（q=冒烟 只存在于夹具 body -> Total 1，证明 content 列参与）、裸 `%`+`_` 输入转义后按字面匹配（q=`%uns_matchable_%` -> Total 0 + 友好提示；未转义则应命中全表）、q 与翻页共存（q=Sample -> Total 24，page=2 取到 Sample post 06，pager 链接 `page=3&amp;q=Sample`）、q 与列头排序共存（`q=Sample&amp;sort=title`）。

### 工具链分叉记录（本机 Windows Git Bash，记档备查）

- **本机 curl（/ucrt64/bin/curl 8.22.0）会把命令行参数里的非 ASCII 文本按系统代码页（GBK）转码**：冒烟脚本 argv 里直接写中文，经 `--data-urlencode` 到达服务端的是 GBK 字节被当 UTF-8 存库（现象：查询能命中同样被转码的标题，但响应 HTML 是 GBK 字节，UTF-8 grep 断言失败）。修复：冒烟里中文负载一律 ASCII 百分号编码经 `--data` 直送（脚本注释里保留解码对照表）；grep 断言里的中文模式不受影响（msys grep 的 UTF-8 argv 正常）。另注：该 curl 的 `--data-urlencode name@file` 读绝对 POSIX 路径（/tmp/...）报读文件错误，相对路径可用——本次未依赖该形态。
- 无框架层缺陷：LinkPager 的 `URL func(page)` 闭包本就支持携带任意额外参数，q 保留纯示例侧改动。

### 验收证据（合并后 main `384e692`，CGO 经 scoop mingw gcc 启用）

- `bash scripts/p1-check.sh` -> **69 passed, 0 failed, P1 checks green**：gofmt 无文件、go vet 无输出、`CGO_ENABLED=1 go test -race -count=1 ./...` 全包全绿（无 data race，含 examples/basic 新增 pages_test.go 7 子测）、示例编译通过；冒烟新增 14 条搜索断言全过（输出见上节清单：搜索框渲染 / zh 搜索文案与空提示翻译 / 中文标题命中 / 中文正文命中 / 裸通配符转义空集 + 友好提示 / Total 收窄 / 翻页共存 / 排序链接共存）。
- `bash scripts/p0-check.sh` -> **31 passed, 0 failed, P0 checks green**（API 面回归，示例改动未影响既有断言）。
- `golangci-lint run ./...`（2.15.0）-> **0 issues**；`gofmt -l .` 无输出。
- 流程：claimed 单独提交 `b7676b7` -> feat/post-search 上 `384e692` -> main `--ff-only` 合并并删除任务分支。
