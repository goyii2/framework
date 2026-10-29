#!/usr/bin/env bash
# P0 一键验收：格式检查 -> go vet -> 全量测试（含 -race）-> examples/basic 冒烟。
#
# 冒烟段在 examples/basic 下重置 runtime/（示例的临时状态目录，已 gitignore），
# 走完整链路：migrate -> seed -> serve -> curl 断言（登录/验证/授权/缓存/日志）。
# Git Bash (Windows) / Linux / macOS 均可运行；-race 需要 CGO 编译器（gcc），
# Windows 下脚本会探测 scoop mingw 的常见位置。
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

# expect_status <描述> <期望码> <实际码>
expect_status() {
    if [ "$3" = "$2" ]; then
        ok "$1 ($2)"
    else
        fail "$1: got $3, want $2"
    fi
}

# expect_contains <描述> <文本> <子串>
expect_contains() {
    if printf '%s' "$2" | grep -q "$3"; then
        ok "$1"
    else
        fail "$1: missing /$3/ in: $(printf '%s' "$2" | head -c 200)"
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

echo "== 5/5 examples/basic smoke =="
cd examples/basic
rm -rf runtime   # 示例临时状态重置（数据库/日志/会话/缓存）

PORT="${SMOKE_PORT:-8091}"
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
    if curl -s -o /dev/null "$BASE/site/index"; then
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

JG="$TMP/guest.jar"   # 未登录（仅有 csrf cookie）
JA="$TMP/admin.jar"   # admin 会话
JD="$TMP/demo.jar"    # demo 会话

# --- 未登录：中间件三件套 ---------------------------------------------
code="$(curl -s -o /dev/null -w '%{http_code}' -c "$JG" "$BASE/site/index")"
expect_status "convention route /site/index answers" 200 "$code"

# open_jar <jar>：先经安全请求种下 _csrf cookie，再返回与该 jar 配对的 token
open_jar() {
    curl -s -o /dev/null -c "$1" "$BASE/site/index"
    curl -s -b "$1" -c "$1" "$BASE/site/csrf" | sed 's/.*"csrf":"\([^"]*\)".*/\1/'
}

TG="$(curl -s -b "$JG" -c "$JG" "$BASE/site/csrf" | sed 's/.*"csrf":"\([^"]*\)".*/\1/')"
if [ -z "$TG" ] || [ "$TG" = "$BASE/site/csrf" ]; then
    fail "csrf token extraction"
else
    ok "csrf token issued"
fi

code="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/login" \
    -H 'Content-Type: application/json' -d '{"username":"admin","password":"admin123"}')"
expect_status "Csrf: unsafe request without token rejected" 400 "$code"

code="$(curl -s -o /dev/null -w '%{http_code}' -b "$JG" -X POST "$BASE/login" \
    -H 'Content-Type: text/plain' -H "X-Csrf-Token: $TG" -d 'x')"
expect_status "ContentNegotiator: non-JSON body rejected" 415 "$code"

code="$(curl -s -o /dev/null -w '%{http_code}' -b "$JG" -X POST "$BASE/site/index" \
    -H "X-Csrf-Token: $TG")"
expect_status "VerbFilter: POST /site/index rejected" 405 "$code"
allow="$(curl -s -D - -o /dev/null -b "$JG" -X POST "$BASE/site/index" \
    -H "X-Csrf-Token: $TG" | grep -i '^allow:' | tr -d '\r')"
expect_contains "VerbFilter: Allow header lists verbs" "$allow" "GET, HEAD"

code="$(curl -s -o /dev/null -w '%{http_code}' -b "$JG" -X POST "$BASE/post" \
    -H 'Content-Type: application/json' -H "X-Csrf-Token: $TG" \
    -d '{"title":"nope","status":"draft"}')"
expect_status "AccessControl: guest write rejected" 403 "$code"

# --- admin 会话：登录 + CRUD -------------------------------------------
TA="$(open_jar "$JA")"

code="$(curl -s -o /dev/null -w '%{http_code}' -b "$JA" -c "$JA" -X POST "$BASE/login" \
    -H 'Content-Type: application/json' -H "X-Csrf-Token: $TA" \
    -d '{"username":"admin","password":"wrong"}')"
expect_status "login with wrong password rejected" 401 "$code"

code="$(curl -s -o /dev/null -w '%{http_code}' -b "$JA" -c "$JA" -X POST "$BASE/login" \
    -H 'Content-Type: application/json' -H "X-Csrf-Token: $TA" \
    -d '{"username":"admin","password":"admin123"}')"
expect_status "login succeeds" 200 "$code"

body="$(curl -s -b "$JA" "$BASE/me")"
expect_contains "session identity bound after login" "$body" '"name":"admin"'

resp="$(curl -s -w '\n%{http_code}' -b "$JA" -X POST "$BASE/post" \
    -H 'Content-Type: application/json' -H "X-Csrf-Token: $TA" \
    -d '{"title":"ab","status":"bogus"}')"
code="${resp##*$'\n'}"
body="${resp%$'\n'*}"
expect_status "invalid create rejected by validation" 422 "$code"
expect_contains "422 body carries field messages" "$body" 'title'

resp="$(curl -s -w '\n%{http_code}' -D "$TMP/create.headers" -o "$TMP/create.body" -b "$JA" -X POST "$BASE/post" \
    -H 'Content-Type: application/json' -H "X-Csrf-Token: $TA" \
    -d '{"title":"First post","body":"hello world","status":"published"}')"
expect_status "create succeeds" 201 "${resp##*$'\n'}"
expect_contains "Location header via named route" "$(cat "$TMP/create.headers")" 'Location: /post/26'

body="$(curl -s "$BASE/post/26")"
expect_contains "view eager-loads the author" "$body" '"author"'

cache="$(curl -s -D - -o /dev/null "$BASE/post" | grep -i '^x-cache:' | tr -d '\r')"
expect_contains "list rebuilt after data change" "$cache" "MISS"
cache="$(curl -s -D - -o /dev/null "$BASE/post" | grep -i '^x-cache:' | tr -d '\r')"
expect_contains "list served from cache" "$cache" "HIT"

resp="$(curl -s -w '\n%{http_code}' -b "$JA" -X PUT "$BASE/post/26" \
    -H 'Content-Type: application/json' -H "X-Csrf-Token: $TA" \
    -d '{"title":"x","status":"draft"}')"
expect_status "invalid update rejected by validation" 422 "${resp##*$'\n'}"

resp="$(curl -s -w '\n%{http_code}' -b "$JA" -X PUT "$BASE/post/26" \
    -H 'Content-Type: application/json' -H "X-Csrf-Token: $TA" \
    -d '{"title":"First post (edited)","status":"draft"}')"
expect_status "update succeeds" 200 "${resp##*$'\n'}"

resp="$(curl -s -w '\n%{http_code}' "$BASE/post/999999")"
expect_status "missing post answers 404" 404 "${resp##*$'\n'}"
expect_contains "404 rendered as JSON" "$resp" '"name":"Not Found"'
resp="$(curl -s -H 'Accept: text/html' "$BASE/post/999999")"
expect_contains "404 negotiated as HTML" "$resp" '<h1>404 Not Found</h1>'

# --- demo 会话：授权分档 ------------------------------------------------
TD="$(open_jar "$JD")"
code="$(curl -s -o /dev/null -w '%{http_code}' -b "$JD" -c "$JD" -X POST "$BASE/login" \
    -H 'Content-Type: application/json' -H "X-Csrf-Token: $TD" \
    -d '{"username":"demo","password":"demo123"}')"
expect_status "demo login succeeds" 200 "$code"

code="$(curl -s -o /dev/null -w '%{http_code}' -b "$JD" -X DELETE "$BASE/post/26" \
    -H "X-Csrf-Token: $TD")"
expect_status "AccessControl: non-admin delete rejected" 403 "$code"

code="$(curl -s -o /dev/null -w '%{http_code}' -b "$JA" -X DELETE "$BASE/post/26" \
    -H "X-Csrf-Token: $TA")"
expect_status "admin delete succeeds" 204 "$code"

code="$(curl -s -o /dev/null -w '%{http_code}' "$BASE/post/26")"
expect_status "deleted post is gone" 404 "$code"

code="$(curl -s -o /dev/null -w '%{http_code}' -b "$JA" -c "$JA" -X POST "$BASE/logout" \
    -H "X-Csrf-Token: $TA")"
expect_status "logout succeeds" 204 "$code"
body="$(curl -s -b "$JA" "$BASE/me")"
expect_contains "session invalidated after logout" "$body" '"guest":true'

# --- 停服后：日志可观察性 ------------------------------------------------
kill "$SRV_PID" 2>/dev/null || true
wait "$SRV_PID" 2>/dev/null || true

LOG=runtime/app.log
if [ ! -f "$LOG" ]; then
    fail "log file runtime/app.log exists"
else
    expect_contains "db queries recorded" "$(cat "$LOG")" '\[db\]'
    expect_contains "slow query warnings recorded" "$(cat "$LOG")" 'slow query'
    expect_contains "cache events recorded" "$(cat "$LOG")" '\[cache\]'
fi

echo
echo "== summary: $PASS passed, $FAIL failed =="
if [ "$FAIL" -gt 0 ]; then
    for msg in "${FAILED_MSGS[@]}"; do
        echo "  - $msg"
    done
    exit 1
fi
echo "P0 checks green."
