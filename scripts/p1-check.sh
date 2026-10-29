#!/usr/bin/env bash
# P1 一键验收：格式检查 -> go vet -> 全量测试（含 -race）-> examples/basic
# 浏览器链路冒烟。
#
# 冒烟段在 examples/basic 下重置 runtime/，走 P1 浏览器完整链路：
# 匿名浏览 -> 登录 -> 列表分页/排序 -> 详情 -> 编辑（验证 + CSRF + 422 回显）
# -> RBAC 权限差异（admin/editor/demo 导航与按钮不同）-> i18n 切换（界面与
# 验证消息翻译）-> 邮件 log 记录 -> JSON API 共存。
#
# Git Bash (Windows) / Linux / macOS 均可运行；-race 需要 CGO 编译器（gcc），
# Windows 下脚本会探测 scoop mingw 的常见位置。p0-check.sh 覆盖 P0 API 面，
# 本脚本在其之上覆盖 P1 页面与组件面。
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

echo "== 1/5 gofmt =="
UNFORMATTED="$(gofmt -l .)"
if [ -n "$UNFORMATTED" ]; then
    echo "gofmt would reformat:"
    echo "$UNFORMATTED"
    exit 1
fi
echo "clean"

echo "== 2/5 go vet =="
go vet ./...
echo "clean"

echo "== 3/5 go test -race ./... =="
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

echo "== 4/5 build examples/basic =="
TMP="$(mktemp -d)"
SRV_PID=""
trap 'cleanup' EXIT
BIN="$TMP/basic"
if [ "$(go env GOOS)" = "windows" ]; then
    BIN="$BIN.exe"
fi
go build -o "$BIN" ./examples/basic

echo "== 5/5 examples/basic browser smoke =="
cd examples/basic
rm -rf runtime # 示例临时状态重置（数据库/日志/会话/缓存）

PORT="${SMOKE_PORT:-8093}"
BASE="http://127.0.0.1:$PORT"

"$BIN" migrate up
"$BIN" seed

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

# 等待端口就绪
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

# login <jar> <user> <pass>：经表单登录（先种 csrf cookie，POST 表单，稍候会话落盘）
login() {
    curl -s -c "$1" "$BASE/login" -H 'Accept: text/html' -o /dev/null
    local t
    t="$(curl -s -b "$1" -c "$1" "$BASE/site/csrf" | sed 's/.*"csrf":"\([^"]*\)".*/\1/')"
    curl -s -o /dev/null -b "$1" -c "$1" -X POST "$BASE/login" \
        -H 'Accept: text/html' -H 'Content-Type: application/x-www-form-urlencoded' \
        --data-urlencode "_csrf=$t" --data-urlencode "username=$2" --data-urlencode "password=$3"
    sleep 0.3
}
token() { curl -s -b "$1" -c "$1" "$BASE/site/csrf" | sed 's/.*"csrf":"\([^"]*\)".*/\1/'; }

JE="$W/editor.jar"
JA="$W/admin.jar"
JD="$W/demo.jar"

# --- 匿名浏览 ------------------------------------------------------------
curl -s "$BASE/" -H 'Accept: text/html' -o "$W/home.html"
contains "home renders in app layout" "goyii basic demo" "$W/home.html"
contains "guest sees login link" ">Login</a>" "$W/home.html"
contains "language switcher offered" "中文" "$W/home.html"

curl -s "$BASE/posts" -H 'Accept: text/html' -o "$W/list.html"
contains "list default order newest first" "Sample post 24" "$W/list.html"
contains "LinkPager widget renders" "pagination" "$W/list.html"
contains "sort toggle links render" "sort=title" "$W/list.html"

curl -s "$BASE/posts?page=2" -H 'Accept: text/html' -o "$W/list2.html"
contains "page 2 carries the oldest rows" "Sample post 06" "$W/list2.html"
contains "pager links page 2" "page=2" "$W/list2.html"

curl -s "$BASE/posts?sort=title" -H 'Accept: text/html' -o "$W/sorted.html"
head -c 200000 "$W/sorted.html" | grep -qF "Hello goyii" &&
    ok "sort=title orders alphabetically" || fail "sort=title orders alphabetically"

status_is "sort outside whitelist rejected" 400 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/posts?sort=password")"

curl -s "$BASE/posts/2" -H 'Accept: text/html' -o "$W/show.html"
contains "Breadcrumbs widget renders" "breadcrumb" "$W/show.html"
contains "DetailView widget renders" "detail-view" "$W/show.html"
contains "detail eager-loads author" "<td>admin</td>" "$W/show.html"
contains "detail shows timestamp column" "Created at" "$W/show.html"

status_is "missing post answers 404" 404 "$(curl -s -o /dev/null -w '%{http_code}' -H 'Accept: text/html' "$BASE/posts/999")"
curl -s -H 'Accept: text/html' "$BASE/posts/999" -o "$W/404.html"
contains "404 uses registered error view" "post not found" "$W/404.html"
contains "error view inside app frame" "goyii basic demo" "$W/404.html"

status_is "guest write page redirects to login" 302 "$(curl -s -o /dev/null -w '%{http_code}' -H 'Accept: text/html' "$BASE/posts/new")"

# --- editor 会话：可写、不可删 -------------------------------------------
login "$JE" editor editor123
curl -s -b "$JE" "$BASE/posts" -H 'Accept: text/html' -o "$W/elist.html"
contains "editor nav shows New post" "/posts/new" "$W/elist.html"
contains "editor rows show Edit" "/posts/20/edit" "$W/elist.html"

curl -s -b "$JE" "$BASE/posts/2" -H 'Accept: text/html' -o "$W/eshow.html"
contains "editor detail has Edit" "/posts/2/edit" "$W/eshow.html"
if grep -qF "/posts/2/delete" "$W/eshow.html"; then
    fail "editor detail hides Delete"
else
    ok "editor detail hides Delete"
fi

TE="$(token "$JE")"
curl -s -b "$JE" -X POST "$BASE/posts" -H 'Accept: text/html' \
    -H 'Content-Type: application/x-www-form-urlencoded' \
    --data-urlencode "_csrf=$TE" --data-urlencode "title=ab" --data-urlencode "status=bogus" \
    -o "$W/e422.html" -w '%{http_code}' >"$W/e422.code"
status_is "invalid create re-renders 422" 422 "$(cat "$W/e422.code")"
contains "422 echoes entered values" 'value="ab"' "$W/e422.html"
contains "422 shows title message" "title must be at least 3 characters" "$W/e422.html"
contains "422 shows status message" "status must be one of" "$W/e422.html"

curl -s -b "$JE" -X POST "$BASE/posts" -H 'Accept: text/html' \
    -H 'Content-Type: application/x-www-form-urlencoded' \
    --data-urlencode "_csrf=$TE" --data-urlencode "title=Editor wrote this" \
    --data-urlencode "status=published" --data-urlencode "body=hi" \
    -o /dev/null -w '%{http_code} %{redirect_url}' >"$W/ecreate.out"
contains "valid create redirects to detail page" "/posts/26" "$W/ecreate.out"

TE="$(token "$JE")"
curl -s -b "$JE" -X POST "$BASE/posts/26" -H 'Accept: text/html' \
    -H 'Content-Type: application/x-www-form-urlencoded' \
    --data-urlencode "_csrf=$TE" --data-urlencode "title=x" --data-urlencode "status=nope" \
    -o "$W/eupd.html" -w '%{http_code}' >"$W/eupd.code"
status_is "invalid update re-renders 422" 422 "$(cat "$W/eupd.code")"
contains "update 422 form targets the row" 'action="/posts/26"' "$W/eupd.html"

TE="$(token "$JE")"
status_is "valid update redirects" 302 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JE" -X POST "$BASE/posts/26" -H 'Accept: text/html' -H 'Content-Type: application/x-www-form-urlencoded' --data-urlencode "_csrf=$TE" --data-urlencode 'title=Editor revised this' --data-urlencode 'status=draft' --data-urlencode 'body=hi v2')"

TE="$(token "$JE")"
status_is "editor delete answers 403" 403 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JE" -X POST "$BASE/posts/26/delete" -H 'Accept: text/html' -H 'Content-Type: application/x-www-form-urlencoded' --data-urlencode "_csrf=$TE")"

TE="$(token "$JE")"
code="$(curl -s -o /dev/null -w '%{http_code}' -b "$JE" -X POST "$BASE/posts" -H 'Accept: text/html' -H 'Content-Type: application/x-www-form-urlencoded' --data-urlencode "_csrf=wrong-token" --data-urlencode 'title=nope' --data-urlencode 'status=draft')"
status_is "form post with bad csrf rejected" 400 "$code"

# --- i18n：语言切换与会话粘住 --------------------------------------------
curl -s -b "$JE" -c "$JE" "$BASE/?lang=zh-CN" -H 'Accept: text/html' -o "$W/zh.html"
contains "zh home translated" "欢迎使用 goyii 基础演示应用" "$W/zh.html"
contains "zh nav translated" ">主页</a>" "$W/zh.html"
curl -s -b "$JE" "$BASE/posts" -H 'Accept: text/html' -o "$W/zhlist.html"
contains "zh choice sticky in session" "共 26 篇文章" "$W/zhlist.html"
contains "zh status translated" "已发布" "$W/zhlist.html"

TE="$(token "$JE")"
curl -s -b "$JE" -X POST "$BASE/posts" -H 'Accept: text/html' \
    -H 'Content-Type: application/x-www-form-urlencoded' \
    --data-urlencode "_csrf=$TE" --data-urlencode "title=ab" --data-urlencode "status=bogus" \
    -o "$W/zh422.html"
contains "zh validation message translated" "标题长度至少为 3 个字符" "$W/zh422.html"

curl -s -b "$JE" -c "$JE" "$BASE/?lang=en-US" -H 'Accept: text/html' -o "$W/en.html"
contains "switch back to english" "Welcome to the goyii basic application" "$W/en.html"

# --- demo 会话：登录但无额外权限 ------------------------------------------
login "$JD" demo demo123
curl -s -b "$JD" "$BASE/posts" -H 'Accept: text/html' -o "$W/dlist.html"
if grep -qF "/posts/new" "$W/dlist.html"; then
    fail "demo nav hides New post"
else
    ok "demo nav hides New post"
fi
if grep -qF "/edit" "$W/dlist.html"; then
    fail "demo rows hide Edit"
else
    ok "demo rows hide Edit"
fi

# --- admin 会话：完整层级权限 ---------------------------------------------
login "$JA" admin admin123
curl -s -b "$JA" "$BASE/posts" -H 'Accept: text/html' -o "$W/alist.html"
contains "admin nav shows New post" "/posts/new" "$W/alist.html"
contains "admin rows show Edit" "/posts/20/edit" "$W/alist.html"

curl -s -b "$JA" "$BASE/posts/26" -H 'Accept: text/html' -o "$W/ashow.html"
contains "admin detail shows Delete" "/posts/26/delete" "$W/ashow.html"

TA="$(token "$JA")"
status_is "admin delete succeeds" 302 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JA" -X POST "$BASE/posts/26/delete" -H 'Accept: text/html' -H 'Content-Type: application/x-www-form-urlencoded' --data-urlencode "_csrf=$TA")"
status_is "deleted post gone" 404 "$(curl -s -o /dev/null -w '%{http_code}' -H 'Accept: text/html' -b "$JA" "$BASE/posts/26")"

# --- 邮件组件（log 驱动）---------------------------------------------------
TA="$(token "$JA")"
curl -s -b "$JA" -X POST "$BASE/site/notify" -H 'Accept: text/html' \
    -H 'Content-Type: application/x-www-form-urlencoded' \
    --data-urlencode "_csrf=$TA" -o "$W/notify.html" -w '%{http_code}' >"$W/notify.code"
status_is "notify page answers 200" 200 "$(cat "$W/notify.code")"
contains "notify page reports delivery to log" "Mail sent" "$W/notify.html"
sleep 0.3
if grep -q "Subject: A test email from basic" "$TMP/serve.log"; then
    ok "log mailer records the composed message"
else
    fail "log mailer records the composed message"
fi

# --- JSON API 与页面共存 ---------------------------------------------------
status_is "API list still answers" 200 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/post")"
curl -s "$BASE/post" -o "$W/api.json"
if grep -qF '"id"' "$W/api.json"; then
    ok "API list stays JSON"
else
    fail "API list stays JSON"
fi

# --- 登出 ------------------------------------------------------------------
TA="$(token "$JA")"
status_is "browser logout redirects home" 302 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JA" -c "$JA" -X POST "$BASE/logout" -H 'Accept: text/html' -H 'Content-Type: application/x-www-form-urlencoded' --data-urlencode "_csrf=$TA")"
curl -s -b "$JA" "$BASE/me" -o "$W/me.json"
contains "session invalidated after logout" '"guest":true' "$W/me.json"

# --- 停服后：可观察性 ------------------------------------------------------
kill "$SRV_PID" 2>/dev/null || true
wait "$SRV_PID" 2>/dev/null || true

LOG=runtime/app.log
if [ ! -f "$LOG" ]; then
    fail "log file runtime/app.log exists"
else
    contains "db queries recorded" '[db]' "$LOG"
    contains "cache events recorded" '[cache]' "$LOG"
fi
contains "login events recorded" "user logged in" "$TMP/serve.log"

echo
echo "== summary: $PASS passed, $FAIL failed =="
if [ "$FAIL" -gt 0 ]; then
    for msg in "${FAILED_MSGS[@]}"; do
        echo "  - $msg"
    done
    exit 1
fi
echo "P1 checks green."
