#!/usr/bin/env bash
# Purpose: end-to-end proof of the ci plugin machine surface (P7-CI-62, CI D31, review F16): the head plugin installed in a
#          temporary HOME, a cli built from "$NSELF/cli" origin/main, and the same commands over CLI, HTTP and MCP.
# Inputs:  --cli-src DIR (a cli checkout; its origin/main is built), --plugin-dir DIR (free/ci), --out DIR (results).
#          Needs git, go, jq, curl, python3 and a free loopback port. Never touches the operator's ~/.nself.
#          Also proves, each in ONE MCP process: the confirmation round trip really acts (a fixture node is revoked and the
#          nonce is single use), `ci --detach` after its confirmation, `runs wait`, and that cli_only flags (`nodes sync
#          --cmd`, `doctor --alert`) and a destructive positional (`artifacts prune`) cannot act without consent.
# Outputs: DIR/{tools.txt,routes.txt,*.cli,*.http,*.mcp,*.session}; exit 0 when every comparison is equal, 1 on any
#          difference, 2 on a missing prerequisite (for example a cli without tools/manifestv2migrate).
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

fail=0
# 4. normaliser for volatile ids and times.
norm() { jq -S 'walk(if type=="string" then gsub("[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.]+Z?";"TIME") | gsub("run_[0-9a-f]+";"RUN") else . end) | del(.. | .duration_ms?, .elapsed_ms?, .waited_ms?, .world_digest?, .free_bytes?)'; }

# 5. the MCP client: stdio JSON-RPC in ONE `nself mcp` process per invocation, because the cli keeps its confirmation
# nonces per process. `mcp list` prints the tools; `mcp session STEPS` runs a JSON array of {tool, arguments, confirm_from?}
# in order (confirm_from N copies the nonce of step N's result into this step) and prints the array of raw results;
# `mcp call TOOL ARGS` is one step and prints its structured content.
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
def body(res):
    data = res.get("structuredContent")
    return data if data is not None else json.loads(res["content"][0]["text"])
call(1, "initialize", {"protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "ci62", "version": "1"}})
notify("notifications/initialized")
verb = sys.argv[1]
if verb == "list":
    print(json.dumps(call(2, "tools/list", {})["result"]["tools"]))
else:
    steps = json.loads(sys.argv[2]) if verb == "session" else [{"tool": sys.argv[2], "arguments": json.loads(sys.argv[3])}]
    results = []
    for n, step in enumerate(steps):
        args = dict(step.get("arguments", {}))
        if "confirm_from" in step:
            args["confirm"] = body(results[step["confirm_from"]]).get("confirm", "")
        results.append(call(10 + n, "tools/call", {"name": step["tool"], "arguments": args})["result"])
    print(json.dumps(results if verb == "session" else results[0].get("structuredContent", results[0])))
proc.stdin.close(); proc.wait(timeout=10)
PY
); }
same() { if ! diff -u "$1" "$2" >"$out/diff.txt"; then echo "DIFFERENT: $1 vs $2" >&2; cat "$out/diff.txt" >&2; fail=1; fi; }
# is_error FILE INDEX: true when result INDEX of a session was refused; kind FILE INDEX: the body's kind member.
is_error() { jq -r ".[$2].isError // false" "$1"; }
kind() { jq -r ".[$2] | (.structuredContent // (.content[0].text | fromjson? // {})) | .kind // \"\"" "$1"; }

# 6. help --json: the declared class and confirmation of the rows that matter, and the cli_only marking of the flags that
# run a program or send a message. A bare name check is not enough: the class is the consent contract.
nself help --json >"$out/help.json"
row() { jq -r --arg p "$1" '[.. | objects | select(.path? == $p)] | first | [.side_effect, (.confirm != null | tostring)] | join(" ")' "$out/help.json"; }
flag_cli_only() { jq -r --arg p "$1" --arg f "$2" '[.. | objects | select(.path? == $p)] | first | .flags[] | select(.name == $f) | .cli_only' "$out/help.json"; }
expect_row() { [ "$(row "$1")" = "$2" ] || { echo "help --json: $1 is '$(row "$1")', want '$2'" >&2; fail=1; }; }
expect_row "nself ci" "remote true"
expect_row "nself ci plan" "read false"
expect_row "nself ci artifacts" "destructive true"
expect_row "nself ci nodes revoke" "destructive true"
expect_row "nself ci nodes label" "remote true"
expect_row "nself ci nodes projects remove" "remote true"
expect_row "nself ci runs resolve" "destructive true"
expect_row "nself ci runs rerun" "remote true"
expect_row "nself ci runs wait" "read false"
[ "$(flag_cli_only "nself ci nodes sync" cmd)" = "true" ] || { echo "nodes sync --cmd is not cli_only" >&2; fail=1; }
[ "$(flag_cli_only "nself ci doctor" alert)" = "true" ] || { echo "doctor --alert is not cli_only" >&2; fail=1; }

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

# 9. the machine-surface region of the wiki page names the same tools and routes (review F16); an absent region fails.
region="$repo_root/.github/wiki/plugins/CI.md"
if grep -q 'BEGIN GENERATED:machine-surface' "$region" && grep -q 'END GENERATED:machine-surface' "$region"; then
  sed -n '/BEGIN GENERATED:machine-surface/,/END GENERATED:machine-surface/p' "$region" | grep -oE 'nself_ci[a-z_]*' | sort -u >"$out/region-tools.txt"
  [ -s "$out/region-tools.txt" ] || { echo "wiki machine-surface region names no tools" >&2; fail=1; }
  comm -13 "$out/tools.txt" "$out/region-tools.txt" >"$out/region-extra.txt"
  [ ! -s "$out/region-extra.txt" ] || { echo "wiki region names tools the cli does not list" >&2; fail=1; }
else
  echo "wiki page $region has no machine-surface region" >&2; fail=1
fi

# 10. samples, three surfaces: CLI stdout, HTTP body and MCP data must match after normalising.
# Each sample: name|cli argv|tool|route|args as a JSON array.
http() { curl -fs -X POST -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -d "$2" "http://127.0.0.1:$port$1"; }
head_sha=$(git -C "$fx" rev-parse HEAD)
# a recorded run (no status: the fixture has no remote) for the `runs wait` sample.
(cd "$fx" && nself ci --no-status --json) >"$out/run.cli" 2>/dev/null || true
run_id=$(jq -r '[.. | strings | select(test("^r[0-9]{8}T[0-9]{6}-[0-9a-f]+$"))] | first // empty' "$out/run.cli")
[ -n "$run_id" ] || { echo "the fixture run recorded no run id" >&2; fail=1; }
samples=(
  'plan|plan|nself_ci_plan|/v1/commands/ci/plan|[]'
  'list|runs list|nself_ci_runs_list|/v1/commands/ci/runs/list|[]'
  "evidence|evidence verify $head_sha|nself_ci_evidence_verify|/v1/commands/ci/evidence/verify|[\"$head_sha\"]"
  'stats|runs stats|nself_ci_runs_stats|/v1/commands/ci/runs/stats|[]'
  "wait|runs wait $run_id|nself_ci_runs_wait|/v1/commands/ci/runs/wait|[\"$run_id\"]"
)
for spec in "${samples[@]}"; do
  IFS='|' read -r name argv tool route jargs <<<"$spec"
  # shellcheck disable=SC2086
  (cd "$fx" && nself ci $argv --json) 2>/dev/null | jq -S '.data // .' | norm >"$out/$name.cli" || true
  [ -s "$out/$name.cli" ] || { echo "sample $name: the CLI printed nothing" >&2; fail=1; }
  mcp call "$tool" "$(jq -nc --argjson a "$jargs" '{args:$a}')" | jq -S '.value // .data // .' | norm >"$out/$name.mcp" || true
  same "$out/$name.cli" "$out/$name.mcp"
  http "$route" "$(jq -nc --argjson a "$jargs" '{args:$a}')" | jq -S '.value // .data // .' | norm >"$out/$name.http" || true
  same "$out/$name.cli" "$out/$name.http"
done

# 11. the confirmation round trip, in ONE mcp process (nonces live per process): a fixture node is imported, revoked through
# request + confirm and must then be revoked; the spent nonce must not act a second time.
printf '{"schema":"inventory/v1","entries":[{"id":"fixnode","ssh":"ci@fixnode.invalid"}]}\n' >"$work/inventory.json"
(cd "$fx" && nself ci nodes sync --from "$work/inventory.json" --json) >"$out/sync.json" 2>&1 || { echo "nodes sync of the fixture inventory failed" >&2; cat "$out/sync.json" >&2; fail=1; }
(cd "$fx" && nself ci nodes list --json) >"$out/nodes.before" 2>&1 || true
node_state() { jq -r '.data.nodes[] | select(.id == "fixnode") | .state' "$1" 2>/dev/null; }
[ -n "$(node_state "$out/nodes.before")" ] || { echo "fixture node not in the registry" >&2; fail=1; }
mcp session '[{"tool":"nself_ci_nodes_revoke","arguments":{"args":["fixnode"]}},
              {"tool":"nself_ci_nodes_revoke","arguments":{"args":["fixnode"]},"confirm_from":0},
              {"tool":"nself_ci_nodes_revoke","arguments":{"args":["fixnode"]},"confirm_from":0}]' >"$out/revoke.session"
[ "$(kind "$out/revoke.session" 0)" = "request" ] || { echo "nodes revoke acted without a confirmation" >&2; fail=1; }
[ "$(is_error "$out/revoke.session" 1)" = "false" ] && [ "$(kind "$out/revoke.session" 1)" != "request" ] || { echo "the confirmed nodes revoke was refused or asked again" >&2; fail=1; }
[ "$(is_error "$out/revoke.session" 2)" = "true" ] || [ "$(kind "$out/revoke.session" 2)" = "request" ] || { echo "a spent nonce acted a second time" >&2; fail=1; }
(cd "$fx" && nself ci nodes list --json) >"$out/nodes.after" 2>&1 || true
[ "$(node_state "$out/nodes.after")" = "revoked" ] || { echo "fixture node was not revoked after the confirmed call" >&2; fail=1; }
[ "$(node_state "$out/nodes.before")" != "revoked" ] || { echo "fixture node was already revoked before the call" >&2; fail=1; }

# 12. bare `ci --detach` after its confirmation round trip (same process).
mcp session '[{"tool":"nself_ci","arguments":{"flags":{"detach":true}}},
              {"tool":"nself_ci","arguments":{"flags":{"detach":true}},"confirm_from":0}]' >"$out/detach.session"
[ "$(kind "$out/detach.session" 0)" = "request" ] || { echo "ci --detach did not ask for a confirmation" >&2; fail=1; }
[ "$(is_error "$out/detach.session" 1)" = "false" ] && [ "$(kind "$out/detach.session" 1)" != "request" ] || { echo "the confirmed ci --detach was refused" >&2; fail=1; }

# 13. what must NOT act: cli_only flags are not in the tool schema (refused, nothing runs); a destructive positional
# (`artifacts prune`) needs the confirmation first.
marker="$work/pwned"
mcp session "$(jq -nc --arg c "touch $marker" '[{"tool":"nself_ci_nodes_sync","arguments":{"flags":{"cmd":$c}}},{"tool":"nself_ci_doctor","arguments":{"flags":{"alert":true}}}]')" >"$out/clionly.session"
[ "$(is_error "$out/clionly.session" 0)" = "true" ] || { echo "MCP accepted nodes sync --cmd" >&2; fail=1; }
[ "$(is_error "$out/clionly.session" 1)" = "true" ] || { echo "MCP accepted doctor --alert" >&2; fail=1; }
code=$(curl -s -o "$out/http-cmd.body" -w '%{http_code}' -X POST -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -d "$(jq -nc --arg c "touch $marker" '{flags:{cmd:$c}}')" "http://127.0.0.1:$port/v1/commands/ci/nodes/sync")
case "$code" in 4??) ;; *) echo "HTTP accepted nodes sync --cmd (status $code)" >&2; fail=1 ;; esac
[ ! -e "$marker" ] || { echo "a caller-supplied program ran through the machine surface" >&2; fail=1; }
mcp session '[{"tool":"nself_ci_artifacts","arguments":{"args":["prune"]}}]' >"$out/artifacts.session"
[ "$(kind "$out/artifacts.session" 0)" = "request" ] || { echo "artifacts prune acted without a confirmation" >&2; fail=1; }
mcp session '[{"tool":"nself_ci_runs","arguments":{"args":["prune"]}}]' >"$out/runs-prune.session"
[ "$(is_error "$out/runs-prune.session" 0)" = "true" ] || [ "$(kind "$out/runs-prune.session" 0)" = "request" ] || { echo "runs prune acted through the read row nself_ci_runs" >&2; fail=1; }

[ "$fail" -eq 0 ] && echo "ci-machine-surface: ok" || { echo "ci-machine-surface: FAILED" >&2; exit 1; }
