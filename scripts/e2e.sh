#!/usr/bin/env bash
# end-to-end smoke of the built binary against a throwaway HOME
set -euo pipefail

bin=$(cd "$(dirname "${1:-./isthmos}")" && pwd)/$(basename "${1:-./isthmos}")
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
export HOME="$tmp"
unset ISTHMOS_SHADOW

echo '{"rules":[{"tool":"mcp__*","drop_keys":["noise"],"max_items":3,"keep_last":1}]}' >"$tmp/rules.json"
export ISTHMOS_RULES="$tmp/rules.json"

fail() { echo "e2e FAIL: $1" >&2; exit 1; }

small='{"noise":"xxxxxxxxxxxxxxxxxxxxxxxx","keep":"y"}'
big='{"items":[{"name":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},{"name":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},{"name":"cccccccccccccccccccccccccccccccccccccccccccccccccc"},{"name":"dddddddddddddddddddddddddddddddddddddddddddddddddd"},{"name":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"},{"name":"ffffffffffffffffffffffffffffffffffffffffffffffffff"}]}'

out=$(echo "$small" | "$bin" filter -tool mcp__github__x)
[[ "$out" != *noise* ]] || fail "filter kept a dropped key"
[[ "$out" == *keep* ]] || fail "filter lost a kept key"

out=$(echo "{\"tool_name\":\"mcp__github__x\",\"tool_response\":$small}" | "$bin" hook)
[[ "$out" == *updatedToolOutput* ]] || fail "hook emitted no rewrite"
[[ "$out" != *noise* ]] || fail "hook kept a dropped key"

out=$(echo "$big" | "$bin" filter -tool mcp__github__x)
[[ "$out" == *"items truncated"* ]] || fail "no truncation marker"
id=$(echo "$out" | grep -oE 'isthmos reveal [0-9a-f]{16}' | awk '{print $3}')
[[ -n "$id" ]] || fail "marker has no reveal id"
[[ "$("$bin" reveal "$id")" == "$big" ]] || fail "reveal did not return the original"

out=$(echo "$small" | ISTHMOS_SHADOW=1 "$bin" filter -tool mcp__github__x)
[[ "$out" == "$small" ]] || fail "shadow filter rewrote the payload"

[[ "$("$bin" stats)" == *mcp__github__x* ]] || fail "stats missing the measured tool"
"$bin" doctor >/dev/null || fail "doctor reported failure"
[[ -n "$("$bin" version)" ]] || fail "version printed nothing"

echo "e2e ok"
