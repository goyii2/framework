# 绝对网址主机信息仅取 boot 配置（url 绝对生成）

## 背景

反向生成已覆盖相对网址（`url.Manager.URL` 与包级 `url.To`），但邮件渲染、队列任务、OAuth 回调等场景需要完整网址（`scheme://host[:port]` + 路径 + 查询串）。Yii2 的 `Url::to()` 第二参传 `true` 时从**当前请求**推导 hostInfo（Request 组件持有请求域信息），也允许组件级显式配置。goyii 要决定同一件事：生成绝对网址的主机信息（hostInfo，见 CONTEXT.md 术语「主机信息」）从哪来——请求推断、boot 配置，还是两者兼有。

## 决策

**主机信息只在 boot 期由装配代码经 `Manager.SetHostInfo` 注入，值来自项目配置（ADR-0005 的「项目自有配置 + 代码装配」形态）；不从当前请求推导。** 写读走 Manager 既有 RWMutex 模式：boot 期写、请求期读。

### 1. 来源：仅 boot 配置

绝对网址的真实消费者全部运行在**无请求上下文**里，配置来源恰好覆盖：

- **邮件渲染**：队列 worker 或 console 发信，手里没有 HTTP 请求；
- **队列任务**：延迟执行时原请求早已返回，request 不可用；
- **OAuth 回调**：redirect_uri 在装配期注册给第三方，与运行期生成的回调网址必须一致——这本来就是一份配置。

反过来，唯一握着请求的场景（渲染页面）恰恰不需要绝对网址：页面里的链接跟随当前访问域名才是期望行为，相对 URL 天然满足。

### 2. 不做请求推断

不从当前请求的 `Host` 头 / `X-Forwarded-Proto` 推导 hostInfo：

- **主力消费者无请求可推**：邮件与队列里推断来源是空集，推断版 API 对它们毫无用处；
- **正确推断本身依赖配置**：反代后面取哪个头、信哪个代理是信任边界配置——绕一圈还是回到 boot 配置，却多了一条与显式值打架的来源；
- **Host 头是攻击面**：密码重置类链接若按请求 Host 生成，Host 头注入可把收件人引向钓鱼域；
- **需要跟随当前访问域名的场景用相对 URL 即可**——相对 URL 在任何域下都正确，是更简单的答案。

### 3. API：链式 builder（Build / String / Absolute / Host）

绝对生成的公共面是一条链：`url.Build(name, params)` 经 Default 管理器起链（与 `To` 同源同表），链的终点决定产物——

- `.String()`：相对 URL，等价 `To(name, params)`；失败（注册路由缺路径参数）返回 `"#"`；主机信息与任何显式域覆盖都不参与。
- `.Absolute()`：绝对 URL，无显式域时在结果前拼管理器的 hostInfo；任何失败——hostInfo 未配置、`Host` 给了非法值、注册路由缺路径参数——返回 `"#"`，请求路径不 panic。
- `.Host(h)`：为**本次生成**显式指定域（跨分区 / 多租户：一个部署要产出另一个域上的链接，如后台在独立域名）；值在 `.Absolute()` 时校验（合法形状与 `SetHostInfo` 一致、尾斜杠容错），非法产 `"#"`；空串清除覆盖。

取代理由：链式调用是 Yii2 `Url::to($url, $scheme)` 可选参数语义的 Go 表达——**选择在调用点**（链停在 `.String()` 还是 `.Absolute()`、要不要 `.Host(...)`，调用处一眼可见），**值在配置**（域名由管理器 hostInfo 或装配注入的显式域持有，调用点不写域名字面量）。「这次要相对还是绝对」若不显式就看不见，若每个调用点都写全域名又重复部署知识——一条链同时化解显式性与重复的两难。

相对侧的既有入口不动：`url.To` / `Manager.URL` 照旧，`.String()` 与 `To` 等价。绝对侧首版落地的两函数形态（`Manager.AbsoluteURL` 显性 error + 包级 `url.ToAbsolute` 吞错）被本形态取代：`ToAbsolute` 删除，`AbsoluteURL` 收窄为 unexported 的内部实现（hostInfo 未配置仍是显性 error 而非半截网址，builder 把它转成 `"#"`）。`SetHostInfo` / `To` / `URL` 的签名与行为一律不动。

装配形态（示例 main.go 即此形态）：`url.SetDefault(handler.URLs())` 之后紧接调用 `handler.URLs().SetHostInfo(cfg.Web.HostInfo)`——值来自项目配置（yaml / overlay / env），行为在装配代码。跨分区显式域同法：advanced 示例把 `cfg.Web.BackendHostInfo` 经装配函数注入 backend 分区（`backend.Register(handler, app, auth, cfg.Web.BackendHostInfo)` -> 控制器字段），触点 `.Host(...).Absolute()`，不给 url 包添第二份全局状态。

### 4. SetHostInfo 非法值 fail-fast

`SetHostInfo(s)` 接收一体字符串 `scheme://host[:port]`（如 `https://example.com`），尾斜杠容错（`https://example.com/` 与不带斜杠产出一致）。缺 scheme、缺 host，或带 userinfo / 多余路径 / query / fragment 的值是装配错误，**boot 期 panic**（消息指明 boot-time wiring mistake），与 `Add` 重复注册即 panic 同哲学：这些值一旦被接受，产出的半截绝对网址会流向无请求上下文的消费者（邮件、回调地址），错得悄无声息——首用时错误已经进了用户邮箱。这也是 ADR-0005「错误一律 boot 期暴露，请求路径没有首用炸雷」在绝对生成上的延续。

同一份形状校验（`normalizeHostInfo`）也服务 builder 的 `Host`：boot 期 panic 与请求期 `"#"` 走同一合法形状（scheme+host 必备、尾斜杠容错、拒绝 userinfo / 多余路径 / query / fragment），只是暴露时机随调用语境不同。

## 后果

正面：

- 「默认对外主机名是什么」只有一个答案（一处配置），邮件、队列、回调不会各说各话；跨分区域经 `.Host` 显式给出，值同样来自项目配置，不另开第二套来源；
- 密码重置等安全敏感链接不受 Host 头注入影响；
- 装配错误（漏配 scheme、误填路径）boot 期即崩，不流入请求路径；
- builder 与 `To` 同源同表，相对 / 绝对 / 显式域一条链表达——Yii2 `Url::to` 可选参数的心智模型原样映射，零新概念。

负面 / 代价：

- 默认域仍是单值：管理器只持一个 hostInfo，多域消费点经 `.Host` 显式给出（值在项目配置）——不为假想需求把管理器做成域名注册表；
- 换域名要改配置并重启，不能请求期自适应——用显式换可预测与安全；
- `Absolute()` 吞错意味着漏配主机信息在页面上表现为 `"#"` 死链而非报错——兜底是装配期必经的 `SetHostInfo`（非法值 boot 期 panic）与示例的紧邻装配形态（漏调用首轮冒烟即暴露为死链）；显性 error 层收在内部 `absoluteURL`，单测仍可直接断言「未配置 == error」。

## 落地中的再决策

- **builder 取代两函数形态**：首版按「两层分层复刻 URL / To」落地（`Manager.AbsoluteURL` 显性 error + 包级 `url.ToAbsolute` 吞错 `"#"`），第二轮工单改造为链式 `url.Build(...)` + `.String()` / `.Absolute()` / `.Host(h)`——两函数形态把「相对还是绝对」烧在函数名里而非调用点，跨分区显式域也没有表达位；`ToAbsolute` 删除，`AbsoluteURL` 收窄为 unexported（见 §3）。
- **`Host` 的校验时机**：显式域在 `.Absolute()` 时才校验（非法产 `"#"` 不 panic）——`Host` 只记值，链的终点统一裁决，任何一环都不在请求路径 panic；与 `SetHostInfo` 的 boot 期 panic 共用同一份形状校验。
- **跨分区域走装配函数注入**：advanced 示例的 `cfg.Web.BackendHostInfo` 由 `backend.Register(...)` 形参注入控制器字段——值经项目配置 + 装配代码流动，不给 url 包加第二份全局状态。

## Considered Options

- **来源三选一**：**仅 boot 配置**（选定：恰好覆盖全部真实消费者，且是唯一在邮件/队列场景可用的来源）；请求推断（rejected：主力消费者无请求可推，Host 头即攻击面，正确推断仍需信任配置）；boot 配置为主 + 请求推断回退（rejected：同一问题两个答案——邮件里一个域、页面上另一个域，还把 Host 头注入引进密码重置类链接）。
- **API 形态**：**builder 链式（`Build` / `String` / `Absolute` / `Host`）**（选定：Yii2 `Url::to($url, $scheme)` 可选参数语义的 Go 表达——选择在调用点、值在配置，显式性与省重复两全）；两函数分层 `Manager.AbsoluteURL`（显性 error）+ `url.ToAbsolute`（吞错 `"#"`）（首版落地形态，superseded——选择藏在函数名里、跨分区显式域无表达位，显性 error 公共层在模板侧找不到消费者，收为内部实现后装配侧兜底改由 `SetHostInfo` 的 boot 期值校验承担）；仅显性 error 版（rejected：视图渲染处到处 err 分支）；仅吞错版（rejected：漏配 HostInfo 被静默吞成 `"#"` 死链——builder 形态下渲染侧确是统一吞错，但兜底前移到 boot 期值校验，吞错只留给模板调用点）；同一函数加选项开关错误行为（rejected：调用点行为不可读，非 Go 惯用法）。
- **builder 形态被否备选三组**：`WithHostInfo` 派生视图（rejected：为一个前缀再造一台机器——派生管理器的构造与生命周期都要管，`.Host()` 一个方法值直达同一效果）；调用点裸拼 host（`host + url.To(...)`，rejected：本分区每一处绝对链接都重复一遍部署知识，且无单一校验点，非法值要等运行期死链才暴露）；`To` 改返 builder（rejected：50+ 调用点从 `string` 语义机械扰动为 `.String()` 收尾——为零收益翻动全部既有调用，`Build` 作为新入口让旧调用零扰动）。
- **非法值处理**：**boot 期 panic**（选定，与 `Add` 重复注册同哲学，ADR-0005 的 boot 期暴露原则）；容忍存储、首用报错（rejected：failingMutex 形态，boot 绿灯请求期炸雷，且炸在无请求上下文里更隐蔽）；静默忽略（rejected：半配置状态直达消费者）。
- **值形状**：**一体字符串 `scheme://host[:port]`**（选定：就是配置文件里一行 URL，无字段间一致性可错）；`Scheme` + `Host` 两字段结构体（rejected：两字段要防互相矛盾，拆开写多一层无收益）。
