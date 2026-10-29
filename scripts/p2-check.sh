#!/usr/bin/env bash
# P2 一键验收：格式检查 -> go vet -> 全量测试（含 -race）-> examples/basic
# 编译 -> P2 组件面冒烟。
#
# 冒烟段在 examples/basic 下重置 runtime/，走 P2 端到端链路：
# 迁移（含队列表）-> seed（含 3 个演示 Job）-> queue run 消费 -> serve
# -> Vite 构建产物引用与静态服务 -> debug 面板（历史/详情/toolbar）
# -> GridView 过滤行/排序/分页 -> ActiveForm 422 回显 -> 嵌套 admin 模块
# 守卫（guest 403 / admin 200）-> PageCache 命中回放与过期重建
# -> 队列页入队 + 并发 worker 消费。
#
# Git Bash (Windows) / Linux / macOS 均可运行；-race 需要 CGO 编译器（gcc），
# Windows 下脚本会探测 scoop mingw 的常见位置。p0/p1-check 覆盖既有面，
# 本脚本覆盖 P2 新增组件面。真机 HMR（npm run dev + Vite 代理）与浏览器
# 人工目验项不在本脚本内，见工单 36 的验收记录。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

PASS=0
FAIL=0
FAILED_MSGS=()

ok() {
    PASS=$((PASS + 1))
    echo "  ok   - $1"
}
fail() {
    FAIL=$((FAIL + 1))
    FAILED_MSGS+=("$1")
    echo "  FAIL - $1"
}
status_is() {
    if [ "$3" = "$2" ]; then
        ok "$1 ($2)"
    else
        fail "$1: got $3, want $2"
    fi
}
contains() { # <描述> <子串> <文件>
    if grep -qF "$2" "$3"; then
        ok "$1"
    else
        fail "$1 (missing: $2 in $(basename "$3"))"
    fi
}
absent() { # <描述> <子串> <文件>
    if grep -qF "$2" "$3"; then
        fail "$1 (unexpected: $2 in $(basename "$3"))"
    else
        ok "$1"
    fi
}

echo "== 1/6 gofmt =="
UNFORMATTED="$(gofmt -l .)"
if [ -n "$UNFORMATTED" ]; then
    echo "gofmt would reformat:"
    echo "$UNFORMATTED"
    exit 1
fi
echo "clean"

echo "== 2/6 go vet =="
go vet ./...
echo "clean"

echo "== 3/6 go test -race ./... =="
# -race 需要 CGO 编译器；Windows Git Bash 无全局 gcc 时探测常见安装位。
if ! command -v gcc >/dev/null 2>&1; then
    GCC_DIR=""
    for d in \
        "$(cygpath "$USERPROFILE" 2>/dev/null || true)/scoop/apps/mingw/current/bin" \
        /c/mingw64/bin \
        /c/msys64/mingw64/bin; do
        if [ -x "$d/gcc.exe" ] || [ -x "$d/gcc" ]; then
            GCC_DIR="$d"
            break
        fi
    done
    if [ -z "$GCC_DIR" ]; then
        echo "no gcc found: -race requires a C compiler (install mingw-w64 gcc)" >&2
        exit 1
    fi
    PATH="$GCC_DIR:$PATH"
    export PATH
fi
CGO_ENABLED=1 go test -race -count=1 ./...

echo "== 4/6 build examples/basic + frontend outputs =="
TMP="$(mktemp -d)"
SRV_PID=""
trap 'cleanup' EXIT
BIN="$TMP/basic"
if [ "$(go env GOOS)" = "windows" ]; then
    BIN="$BIN.exe"
fi
go build -o "$BIN" ./examples/basic

# Vite 构建产物入库核对：manifest 与 hash 资源缺一不可（无 Node 环境时
# Go 侧以入库产物走 build 模式，本节即证明）。
WEB=examples/basic/web
if [ ! -f "$WEB/dist/.vite/manifest.json" ]; then
    echo "missing $WEB/dist/.vite/manifest.json (run npm install && npm run build in $WEB)" >&2
    exit 1
fi
ok "vite manifest committed"
JS_FILE="$(ls "$WEB"/dist/assets/main-*.js | head -1)"
CSS_FILE="$(ls "$WEB"/dist/assets/main-*.css | head -1)"
if [ -n "$JS_FILE" ] && [ -n "$CSS_FILE" ]; then
    ok "hashed vite assets committed"
else
    fail "hashed vite assets committed"
fi

echo "== 5/6 migrate + seed + queue worker =="
cd examples/basic
rm -rf runtime # 示例临时状态重置（数据库/日志/会话/缓存/调试记录）

"$BIN" migrate up >"$TMP/migrate.log" 2>&1
contains "queue table migration applied" "create_queue_table" "$TMP/migrate.log"

"$BIN" seed >"$TMP/seed.log" 2>&1
contains "seed plants demo queue jobs" "queued 3 demo email job(s)" "$TMP/seed.log"

"$BIN" queue run >"$TMP/worker1.log" 2>&1
contains "worker drains the seeded jobs" "processed 3 message(s)" "$TMP/worker1.log"
contains "worker delivers queued mail" "Subject: Welcome to the goyii basic queue" "$TMP/worker1.log"

echo "== 6/6 serve + P2 browser smoke =="
PORT="${SMOKE_PORT:-8096}"
BASE="http://127.0.0.1:$PORT"

WEB_LISTEN="127.0.0.1:$PORT" "$BIN" serve >"$TMP/serve.log" 2>&1 &
SRV_PID=$!

cleanup() {
    if [ -n "${SRV_PID:-}" ]; then
        kill "$SRV_PID" 2>/dev/null || true
    fi
    if [ "${FAIL:-0}" -gt 0 ]; then
        echo
        echo "serve log: $TMP/serve.log (kept)"
        echo "temp dir kept: $TMP"
    else
        rm -rf "$TMP"
        rm -rf runtime
    fi
}

READY=0
for _ in $(seq 1 50); do
    if curl -s -o /dev/null "$BASE/"; then
        READY=1
        break
    fi
    sleep 0.2
done
if [ "$READY" != "1" ]; then
    echo "server never became ready; log:"
    cat "$TMP/serve.log"
    exit 1
fi

W="$TMP/pages"
mkdir -p "$W"

login() { # <jar> <user> <pass>：经表单登录（先种 csrf cookie，稍候会话落盘）
    curl -s -c "$1" "$BASE/login" -H 'Accept: text/html' -o /dev/null
    local t
    t="$(curl -s -b "$1" -c "$1" "$BASE/site/csrf" | sed 's/.*"csrf":"\([^"]*\)".*/\1/')"
    curl -s -o /dev/null -b "$1" -c "$1" -X POST "$BASE/login" \
        -H 'Accept: text/html' -H 'Content-Type: application/x-www-form-urlencoded' \
        --data-urlencode "_csrf=$t" --data-urlencode "username=$2" --data-urlencode "password=$3"
    sleep 0.3
}
token() { curl -s -b "$1" -c "$1" "$BASE/site/csrf" | sed 's/.*"csrf":"\([^"]*\)".*/\1/'; }

# --- Vite：页面 head 引用构建产物，静态服务可取 -----------------------------
curl -s "$BASE/" -H 'Accept: text/html' -o "$W/home.html"
contains "head loads the hashed module script" '<script type="module" src="/assets/main-' "$W/home.html"
contains "head loads the hashed stylesheet" '<link rel="stylesheet" href="/assets/main-' "$W/home.html"
ASSET_JS="$(grep -o 'src="/assets/main-[^"]*' "$W/home.html" | head -1 | cut -d'"' -f2)"
CODE_TYPE="$(curl -s -o /dev/null -w '%{http_code} %{content_type}' "$BASE$ASSET_JS")"
case "$CODE_TYPE" in
    200\ text/javascript*) ok "hashed asset served ($CODE_TYPE)" ;;
    *) fail "hashed asset served: got $CODE_TYPE" ;;
esac

# --- debug 面板：toolbar 注入 + 历史 + 详情 ---------------------------------
contains "debug toolbar injected into pages" "goyii-debug-toolbar" "$W/home.html"
status_is "/debug history answers" 200 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/debug")"
curl -s "$BASE/debug" -o "$W/debug.html"
contains "history lists recorded requests" "recorded requests" "$W/debug.html"
DBG_ID="$(grep -o 'id=[0-9T.:-]*' "$W/debug.html" | head -1 | cut -d= -f2)"
status_is "/debug detail answers" 200 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/debug/view?id=$DBG_ID")"
curl -s "$BASE/debug/view?id=$DBG_ID" -o "$W/dbgview.html"
contains "detail shows the routing panel" "Route pattern" "$W/dbgview.html"

# --- 列表大件：GridView 排序列 + 过滤行 + LinkPager -------------------------
curl -s "$BASE/posts" -H 'Accept: text/html' -o "$W/list.html"
contains "GridView table renders" "grid-view" "$W/list.html"
contains "filter row renders" 'tr class="filters"' "$W/list.html"
contains "title filter input renders" 'name="title"' "$W/list.html"
contains "status filter select renders" '<select name="status"' "$W/list.html"
contains "sort toggle links render" "sort=title" "$W/list.html"
contains "LinkPager widget renders" "pagination" "$W/list.html"
contains "summary line counts all rows" "Total 25 posts" "$W/list.html"

curl -s "$BASE/posts?title=Sample&status=draft" -H 'Accept: text/html' -o "$W/filt.html"
contains "filters narrow the count" "Total 6 posts" "$W/filt.html"
contains "title filter echoes its value" 'value="Sample"' "$W/filt.html"
contains "sort links keep the filters" 'sort=title&amp;status=draft' "$W/filt.html"

# 裸 % 与 _ 输入若未转义会匹配全表；转义后按字面匹配 -> 空集
curl -s -G "$BASE/posts" --data-urlencode "title=%uns_matchable_%" -H 'Accept: text/html' -o "$W/fesc.html"
contains "like wildcards stay literal" "Total 0 posts" "$W/fesc.html"
contains "filtered empty state renders" "No posts match your filters." "$W/fesc.html"

status_is "sort outside whitelist rejected" 400 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/posts?sort=password")"

# --- ActiveForm：422 回显值与消息 ------------------------------------------
JE="$W/editor.jar"
login "$JE" editor editor123
curl -s -b "$JE" "$BASE/posts" -H 'Accept: text/html' -o "$W/elist.html"
contains "editor rows show the Edit action" "/posts/20/edit" "$W/elist.html"
TE="$(token "$JE")"
curl -s -b "$JE" -X POST "$BASE/posts" -H 'Accept: text/html' \
    -H 'Content-Type: application/x-www-form-urlencoded' \
    --data-urlencode "_csrf=$TE" --data-urlencode "title=ab" --data-urlencode "status=bogus" \
    -o "$W/f422.html" -w '%{http_code}' >"$W/f422.code"
status_is "invalid create re-renders 422" 422 "$(cat "$W/f422.code")"
contains "ActiveForm refills the entered value" 'value="ab"' "$W/f422.html"
contains "ActiveForm echoes the title message" "title must be at least 3 characters" "$W/f422.html"
contains "ActiveForm echoes the status message" "status must be one of" "$W/f422.html"
contains "failing fields carry the error state" "has-error" "$W/f422.html"

# --- 嵌套 admin 模块：模块级守卫 + 登录后可达 -------------------------------
status_is "guest blocked from admin panel" 403 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/admin/panel/index")"
status_is "guest blocked from nested report" 403 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/admin/report/dashboard")"
JA="$W/admin.jar"
login "$JA" admin admin123
curl -s -b "$JA" "$BASE/admin/panel/index" -o "$W/panel.json"
contains "admin panel answers inside the module" '"module":"admin"' "$W/panel.json"
curl -s -b "$JA" "$BASE/admin/report/dashboard" -o "$W/report.json"
contains "nested submodule answers" '"module":"admin/report"' "$W/report.json"

# --- PageCache：窗口内回放（时间戳冻结），过期后重建 ------------------------
curl -s "$BASE/pagecache" -H 'Accept: text/html' -o "$W/pc1.html"
sleep 0.5
curl -s "$BASE/pagecache" -H 'Accept: text/html' -o "$W/pc2.html"
STAMP1="$(grep -o 'pagecache-stamp">[^<]*' "$W/pc1.html")"
STAMP2="$(grep -o 'pagecache-stamp">[^<]*' "$W/pc2.html")"
if [ -n "$STAMP1" ] && [ "$STAMP1" = "$STAMP2" ]; then
    ok "second request replays the cached page"
else
    fail "second request replays the cached page ($STAMP1 vs $STAMP2)"
fi
contains "cached replay still gets a toolbar" "goyii-debug-toolbar" "$W/pc2.html"
# VaryBy splits variants: the admin session's page caches separately from the
# guest's, so its stamp renders fresh rather than replaying the guest copy.
curl -s -b "$JA" "$BASE/pagecache" -H 'Accept: text/html' -o "$W/pcadmin.html"
STAMPA="$(grep -o 'pagecache-stamp">[^<]*' "$W/pcadmin.html")"
if [ -n "$STAMPA" ] && [ "$STAMPA" != "$STAMP1" ]; then
    ok "vary-by splits visitor variants"
else
    fail "vary-by splits visitor variants ($STAMPA vs $STAMP1)"
fi
sleep 3.5
curl -s "$BASE/pagecache" -H 'Accept: text/html' -o "$W/pc3.html"
STAMP3="$(grep -o 'pagecache-stamp">[^<]*' "$W/pc3.html")"
if [ -n "$STAMP3" ] && [ "$STAMP3" != "$STAMP1" ]; then
    ok "expired entry renders fresh"
else
    fail "expired entry renders fresh ($STAMP3 vs $STAMP1)"
fi

# --- 队列：页面入队 + 并发 worker 消费 --------------------------------------
JQ="$W/queue.jar"
curl -s -c "$JQ" "$BASE/queue" -H 'Accept: text/html' -o "$W/queue.html"
contains "queue page shows the worker hint" "queue run" "$W/queue.html"
contains "queue page counts zero waiting" "<td>0</td>" "$W/queue.html"
TQ="$(token "$JQ")"
LOC="$(curl -s -o /dev/null -w '%{http_code} %{redirect_url}' -b "$JQ" -c "$JQ" -X POST "$BASE/queue/push" \
    -H 'Accept: text/html' -H 'Content-Type: application/x-www-form-urlencoded' \
    --data-urlencode "_csrf=$TQ" --data-urlencode "to=smoke@example.com")"
case "$LOC" in
    302\ *) ok "queue push redirects back ($LOC)" ;;
    *) fail "queue push redirects back: got $LOC" ;;
esac
PUSH_URL="${LOC#302 }"
curl -s -b "$JQ" "$PUSH_URL" -H 'Accept: text/html' -o "$W/queue2.html"
contains "queue page reports the pushed job" "it is waiting for a worker" "$W/queue2.html"
contains "queue page counts one waiting" "<td>1</td>" "$W/queue2.html"
# serve 进程运行期间以独立 worker 进程消费：WAL + busy_timeout 让两进程共享
# 同一 sqlite 文件（生产队列的标准形态）。
"$BIN" queue run >"$TMP/worker2.log" 2>&1
contains "concurrent worker drains the pushed job" "processed 1 message" "$TMP/worker2.log"
contains "queued mail delivered to the recipient" "To: smoke@example.com" "$TMP/worker2.log"

# --- 停服后：可观察性 ------------------------------------------------------
kill "$SRV_PID" 2>/dev/null || true
wait "$SRV_PID" 2>/dev/null || true
SRV_PID=""

LOG=runtime/app.log
if [ ! -f "$LOG" ]; then
    fail "log file runtime/app.log exists"
else
    contains "db queries recorded" '[db]' "$LOG"
fi

echo
echo "== summary: $PASS passed, $FAIL failed =="
if [ "$FAIL" -gt 0 ]; then
    for msg in "${FAILED_MSGS[@]}"; do
        echo "  - $msg"
    done
    exit 1
fi
echo "P2 checks green."
