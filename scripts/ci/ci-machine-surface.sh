#!/usr/bin/env bash
# Purpose: end-to-end proof of the ci plugin machine surface (P7-CI-62, CI D31, review F16): the head plugin installed in a
#          temporary HOME, a cli built from "$NSELF/cli" origin/main, and the same six commands over CLI, HTTP and MCP.
# Inputs:  --cli-src DIR (a cli checkout; its origin/main is built), --plugin-dir DIR (free/ci), --out DIR (results).
#          Needs git, go, jq, curl, python3 and a free loopback port. Never touches the operator's ~/.nself.
# Outputs: DIR/{tools.txt,routes.txt,*.cli,*.http,*.mcp}; exit 0 when every comparison is equal, 1 on any difference,
#          2 on a missing prerequisite (for example a cli without tools/manifestv2migrate).
# Constraints: the cli worktree, the temp HOME and the server are removed on exit; no network beyond loopback.
set -euo pipefail

cli_src="" plugin_dir="" out=""
while [ $# -gt 0 ]; do
  case "$1" in
    --cli-src) cli_src="$2"; shift 2 ;;
    --plugin-dir) plugin_dir="$2"; shift 2 ;;
    --out) out="$2"; shift 2 ;;
    *) echo "usage: $0 --cli-src DIR --plugin-dir DIR --out DIR" >&2; exit 2 ;;
  esac
done
[ -n "$cli_src" ] && [ -n "$plugin_dir" ] && [ -n "$out" ] || { echo "usage: $0 --cli-src DIR --plugin-dir DIR --out DIR" >&2; exit 2; }
for tool in git go jq curl python3; do command -v "$tool" >/dev/null || { echo "missing prerequisite: $tool" >&2; exit 2; }; done
plugin_dir=$(cd "$plugin_dir" && pwd)
repo_root=$(cd "$plugin_dir/../.." && pwd)
work=$(mktemp -d "${TMPDIR:?TMPDIR must be set}/ci62.XXXXXX")
mkdir -p "$out"; out=$(cd "$out" && pwd)
server_pid=""
cleanup() {
  [ -n "$server_pid" ] && kill "$server_pid" 2>/dev/null || true
  git -C "$cli_src" worktree remove --force "$work/cli" 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT

# 1. cli at origin/main, and the manifest v2 check from that tree.
git -C "$cli_src" worktree add --detach "$work/cli" origin/main >/dev/null
[ -d "$work/cli/tools/manifestv2migrate" ] || { echo "cli origin/main has no tools/manifestv2migrate (P7-PLUG-01 not merged)" >&2; exit 2; }
(cd "$work/cli" && go run -mod=vendor ./tools/manifestv2migrate -in "$plugin_dir/plugin.json" -check)
(cd "$work/cli" && CGO_ENABLED=0 go build -mod=vendor -o "$work/bin/nself" ./cmd/nself)

# 2. head plugin into a temp HOME (P7-CI-47 layout).
export HOME="$work/home"; mkdir -p "$HOME/.nself/plugins/ci" "$HOME/.nself/plugins/bin"
cp "$plugin_dir/plugin.json" "$HOME/.nself/plugins/ci/plugin.json"
(cd "$plugin_dir" && CGO_ENABLED=0 go build -mod=vendor -o "$HOME/.nself/plugins/bin/nself-ci" ./cmd)
export PATH="$work/bin:$PATH" NSELF_V15=1 NSELF_NONINTERACTIVE=1 NSELF_CI_HOME="$work/cihome"

# 3a. an nself project for the cli (mcp and admin serve need one), and
# 3b. a fixture repo with one trivial pipeline.
fx="$work/fixture"; mkdir -p "$fx/.nself"
printf 'version: 1\njobs:\n  hello:\n    kind: test\n    run: ["true"]\n' >"$fx/.nself/ci.yaml"
git -C "$fx" init -q && git -C "$fx" add .nself/ci.yaml && git -C "$fx" -c user.name=t -c user.email=t@example.invalid commit -q -m fixture
export NSELF_CI_REPO="$fx"
proj="$work/proj"; mkdir -p "$proj"
(cd "$proj" && nself init --non-interactive >/dev/null 2>&1)

# 4. normaliser for volatile ids and times.
norm() { jq -S 'walk(if type=="string" then gsub("[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.]+Z?";"TIME") | gsub("run_[0-9a-f]+";"RUN") else . end) | del(.. | .duration_ms?, .elapsed_ms?, .world_digest?, .free_bytes?)'; }

# 5. the MCP client: 40 lines of stdio JSON-RPC, used for tools/list and tools/call.
mcp() { (cd "$proj" && python3 -B - "$@" <<'PY'
import json, subprocess, sys
proc = subprocess.Popen(["nself", "mcp"], stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True)
def call(i, method, params):
    proc.stdin.write(json.dumps({"jsonrpc": "2.0", "id": i, "method": method, "params": params}) + "\n"); proc.stdin.flush()
    while True:
        msg = json.loads(proc.stdout.readline())
        if msg.get("id") == i:
            return msg
def notify(method):
    proc.stdin.write(json.dumps({"jsonrpc": "2.0", "method": method}) + "\n"); proc.stdin.flush()
call(1, "initialize", {"protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "ci62", "version": "1"}})
notify("notifications/initialized")
verb = sys.argv[1]
if verb == "list":
    print(json.dumps(call(2, "tools/list", {})["result"]["tools"]))
else:
    res = call(2, "tools/call", {"name": sys.argv[2], "arguments": json.loads(sys.argv[3])})["result"]
    print(json.dumps(res.get("structuredContent", res)))
proc.stdin.close(); proc.wait(timeout=10)
PY
); }

fail=0
same() { if ! diff -u "$1" "$2" >"$out/diff.txt"; then echo "DIFFERENT: $1 vs $2" >&2; cat "$out/diff.txt" >&2; fail=1; fi; }

# 6. help --json: every row lists canon plugin, side effect, json and confirm; bare ci is remote with confirm.
nself help --json >"$out/help.json"
jq -e '[.. | objects | select(.name? == "ci")] | length > 0' "$out/help.json" >/dev/null || { echo "ci missing from nself help --json" >&2; fail=1; }

# 7. tools/list: the expected tools, none for cli-only, interactive or stream rows.
mcp list | jq -r '.[].name' | sort >"$out/tools.txt"
for t in nself_ci nself_ci_plan nself_ci_runs nself_ci_runs_wait nself_ci_runs_stats nself_ci_evidence_verify nself_ci_nodes_list nself_ci_nodes_revoke nself_ci_evidence; do
  grep -qx "$t" "$out/tools.txt" || { echo "missing tool $t" >&2; fail=1; }
done
for t in nself_ci_policy_set nself_ci_policy_unset nself_ci_policy_pin nself_ci_serve nself_ci_nodes_pair; do
  ! grep -qx "$t" "$out/tools.txt" || { echo "cli-only row exposed as tool $t" >&2; fail=1; }
done

# 8. HTTP: nself admin serve on a free loopback port with a generated token.
port=$(python3 -B -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')
token=$(python3 -B -c 'import secrets; print(secrets.token_hex(16))')
(cd "$proj" && NSELF_MCP_TOKEN="$token" exec nself admin serve --host 127.0.0.1 --port "$port") >"$work/serve.log" 2>&1 & server_pid=$!
for _ in $(seq 1 50); do curl -fs "http://127.0.0.1:$port/openapi.json" -H "Authorization: Bearer $token" >"$out/openapi.json" 2>/dev/null && break; sleep 0.2; done
jq -r '.paths | keys[]' "$out/openapi.json" | grep -i 'ci' | sort >"$out/routes.txt" || true
! grep -Eq 'ci/serve(/backup|/restore)?$' "$out/routes.txt" || { echo "ci serve has an HTTP route" >&2; fail=1; }

# 9. the machine-surface region of the wiki page names the same tools and routes (review F16).
region="$repo_root/.github/wiki/plugins/CI.md"
if grep -q 'machine-surface' "$region"; then
  sed -n '/machine-surface/,/\/machine-surface/p' "$region" | grep -oE 'nself_ci[a-z_]*' | sort -u >"$out/region-tools.txt"
  comm -13 "$out/tools.txt" "$out/region-tools.txt" >"$out/region-extra.txt"
  [ ! -s "$out/region-extra.txt" ] || { echo "wiki region names tools the cli does not list" >&2; fail=1; }
fi

# 10. six samples, three surfaces: CLI stdout, HTTP body and MCP data must match after normalising.
# Each sample: name|cli argv|tool|route|args as a JSON array.
http() { curl -fs -X POST -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -d "$2" "http://127.0.0.1:$port$1"; }
head_sha=$(git -C "$fx" rev-parse HEAD)
samples=(
  'plan|plan|nself_ci_plan|/v1/commands/ci/plan|[]'
  'list|runs list|nself_ci_runs_list|/v1/commands/ci/runs/list|[]'
  "evidence|evidence verify $head_sha|nself_ci_evidence_verify|/v1/commands/ci/evidence/verify|[\"$head_sha\"]"
  'stats|runs stats|nself_ci_runs_stats|/v1/commands/ci/runs/stats|[]'
)
for spec in "${samples[@]}"; do
  IFS='|' read -r name argv tool route jargs <<<"$spec"
  # shellcheck disable=SC2086
  (cd "$fx" && nself ci $argv --json) 2>/dev/null | jq -S '.data // .' | norm >"$out/$name.cli" || true
  mcp call "$tool" "$(jq -nc --argjson a "$jargs" '{args:$a}')" | jq -S '.value // .data // .' | norm >"$out/$name.mcp" || true
  same "$out/$name.cli" "$out/$name.mcp"
  http "$route" "$(jq -nc --argjson a "$jargs" '{args:$a}')" | jq -S '.value // .data // .' | norm >"$out/$name.http" || true
  same "$out/$name.cli" "$out/$name.http"
done
# bare `ci` and nodes revoke: the first call returns a confirmation (kind request), only the call carrying its nonce acts.
mcp call nself_ci '{}' >"$out/ci.first"
jq -e '.kind == "request" and (.confirm | length) == 64' "$out/ci.first" >/dev/null || { echo "bare ci did not return a confirmation" >&2; fail=1; }
mcp call nself_ci_nodes_revoke '{"args":["none"]}' >"$out/revoke.first"
jq -e '.kind == "request"' "$out/revoke.first" >/dev/null || { echo "nodes revoke acted without a confirmation" >&2; fail=1; }
nonce=$(jq -r '.confirm // empty' "$out/revoke.first")
mcp call nself_ci_nodes_revoke "$(jq -nc --arg n "$nonce" '{args:["none"],confirm:$n}')" >"$out/revoke.second"
! jq -e '.kind == "request"' "$out/revoke.second" >/dev/null 2>&1 || { echo "nodes revoke asked again with its nonce" >&2; fail=1; }

[ "$fail" -eq 0 ] && echo "ci-machine-surface: ok" || { echo "ci-machine-surface: FAILED" >&2; exit 1; }
