#!/usr/bin/env bash
# 校验 atom-fakeip + smart 两个补丁是否都还活在编出来的内核里。
# 只用「真二进制」验证，不看源码 —— 上游改动把补丁悄悄改坏时源码看不出来。
#
# 用法: verify.sh <mihomo 二进制> [期望版本子串]
set -euo pipefail

BIN="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
EXPECT_VERSION="${2:-}"
CI_DIR="$(cd "$(dirname "$0")" && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

fail() { echo "::error::$*" >&2; exit 1; }
note() { echo "--- $*"; }

[ -x "$BIN" ] || fail "二进制不可执行：$BIN"

note "1/5 版本自报"
REPORTED="$("$BIN" -v 2>&1 | head -1)"
echo "$REPORTED"
if [ -n "$EXPECT_VERSION" ] && ! grep -qF "$EXPECT_VERSION" <<<"$REPORTED"; then
  fail "版本自报不含 $EXPECT_VERSION：$REPORTED"
fi

note "2/5 atom 私有 DNS 字段必须被接受"
mkdir -p "$WORK/atom"
cp "$CI_DIR/fixture-atom-dns.yaml" "$WORK/atom/config.yaml"
if ! "$BIN" -t -d "$WORK/atom" -f "$WORK/atom/config.yaml" 2>&1 | tee "$WORK/atom.log" | grep -q "test is successful"; then
  fail "atom 私有字段（atom-mixed-dns / atom-fake-ip-final）校验失败，FakeIP 决策链路已断"
fi

note "3/5 反向对照：去掉 atom-mixed-dns 必须失败（证明上一步不是空转）"
mkdir -p "$WORK/neg"
grep -v 'atom-mixed-dns:' "$CI_DIR/fixture-atom-dns.yaml" > "$WORK/neg/config.yaml"
if "$BIN" -t -d "$WORK/neg" -f "$WORK/neg/config.yaml" 2>&1 | grep -q "test is successful"; then
  fail "去掉 atom-mixed-dns 仍然通过 —— atom 补丁已失效或校验已失效"
fi

note "4/5 smart 组类型必须被接受"
mkdir -p "$WORK/smart"
cp "$CI_DIR/fixture-smart.yaml" "$WORK/smart/config.yaml"
if ! "$BIN" -t -d "$WORK/smart" -f "$WORK/smart/config.yaml" 2>&1 | tee "$WORK/smart.log" | grep -q "test is successful"; then
  fail "smart 组校验失败，vernesong 的 smart 代码已丢失"
fi

note "5/5 运行时：Clash API 里必须真的出现 Smart 组"
mkdir -p "$WORK/run"
sed 's/^mixed-port: .*/mixed-port: 17895/' "$CI_DIR/fixture-smart.yaml" > "$WORK/run/config.yaml"
cat >> "$WORK/run/config.yaml" <<'EOF'

external-controller: 127.0.0.1:19095
secret: ci-only
EOF
"$BIN" -d "$WORK/run" -f "$WORK/run/config.yaml" > "$WORK/run.log" 2>&1 &
PID=$!
trap 'kill "$PID" 2>/dev/null || true; rm -rf "$WORK"' EXIT
for _ in $(seq 1 20); do
  if curl -sf -H 'Authorization: Bearer ci-only' http://127.0.0.1:19095/proxies > "$WORK/proxies.json" 2>/dev/null; then break; fi
  sleep 0.5
done
python3 - "$WORK/proxies.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))['proxies']
smart = {k: v for k, v in d.items() if v.get('type') == 'Smart'}
if not smart:
    sys.exit('运行时没有 Smart 类型的组，smart 补丁未生效')
for k, v in smart.items():
    print(f'  {k}: type={v["type"]}')
PY
kill "$PID" 2>/dev/null || true
wait "$PID" 2>/dev/null || true

echo "全部通过：atom FakeIP 补丁 + smart 组都在。"