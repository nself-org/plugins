#!/bin/bash
# =============================================================================
# Tests for GitHub Actions Runner plugin
# =============================================================================
# Run: bash tests/github-runner.test.sh
#
# Tests validate script structure and offline logic only.
# Registration/download tests require live GitHub credentials.

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PASS=0
FAIL=0

pass() { PASS=$((PASS + 1)); printf "  ✓ %s\n" "$1"; }
fail() { FAIL=$((FAIL + 1)); printf "  ✗ %s\n" "$1"; }

# =============================================================================
# 1. File structure tests
# =============================================================================
echo "File structure"

[[ -f "$PLUGIN_DIR/plugin.json" ]] && pass "plugin.json exists" || fail "plugin.json missing"
[[ -f "$PLUGIN_DIR/install.sh" ]] && pass "install.sh exists" || fail "install.sh missing"
[[ -f "$PLUGIN_DIR/uninstall.sh" ]] && pass "uninstall.sh exists" || fail "uninstall.sh missing"
[[ -f "$PLUGIN_DIR/README.md" ]] && pass "README.md exists" || fail "README.md missing"

for action in start stop status logs update; do
  [[ -f "$PLUGIN_DIR/actions/${action}.sh" ]] && pass "actions/${action}.sh exists" || fail "actions/${action}.sh missing"
done

# =============================================================================
# 2. plugin.json validation
# =============================================================================
echo ""
echo "plugin.json"

name=$(python3 -c "import json; print(json.load(open('$PLUGIN_DIR/plugin.json'))['name'])")
[[ "$name" == "github-runner" ]] && pass "name is 'github-runner'" || fail "name is '$name'"

license=$(python3 -c "import json; print(json.load(open('$PLUGIN_DIR/plugin.json'))['license'])")
[[ "$license" == "MIT" ]] && pass "license is MIT" || fail "license is '$license'"

req_env=$(python3 -c "import json; d=json.load(open('$PLUGIN_DIR/plugin.json')); print(len(d['envVars']['required']))")
[[ "$req_env" -ge 2 ]] && pass "has $req_env required env vars" || fail "expected >=2 required env vars, got $req_env"

# =============================================================================
# 3. Shell script syntax validation
# =============================================================================
echo ""
echo "Shell syntax (bash -n)"

for script in "$PLUGIN_DIR/install.sh" "$PLUGIN_DIR/uninstall.sh" "$PLUGIN_DIR"/actions/*.sh; do
  if bash -n "$script" 2>/dev/null; then
    pass "$(basename "$script") — valid syntax"
  else
    fail "$(basename "$script") — syntax error"
  fi
done

# =============================================================================
# 4. Architecture detection (offline)
# =============================================================================
echo ""
echo "Architecture detection"

# Source install.sh functions in a subshell to test detect_arch/detect_os
# We can't source directly because install.sh calls install_github_runner at the end.
# Instead, test that the functions exist by grepping.
grep -q 'detect_arch()' "$PLUGIN_DIR/install.sh" && pass "detect_arch() defined" || fail "detect_arch() missing"
grep -q 'detect_os()' "$PLUGIN_DIR/install.sh" && pass "detect_os() defined" || fail "detect_os() missing"
grep -q 'get_registration_token()' "$PLUGIN_DIR/install.sh" && pass "get_registration_token() defined" || fail "missing"
grep -q 'get_latest_runner_version()' "$PLUGIN_DIR/install.sh" && pass "get_latest_runner_version() defined" || fail "missing"

# =============================================================================
# 5. Action scripts check required vars
# =============================================================================
echo ""
echo "Action scripts guard env vars"

grep -q 'RUNNER_DIR' "$PLUGIN_DIR/actions/start.sh" && pass "start.sh references RUNNER_DIR" || fail "start.sh missing RUNNER_DIR"
grep -q 'RUNNER_DIR' "$PLUGIN_DIR/actions/stop.sh" && pass "stop.sh references RUNNER_DIR" || fail "stop.sh missing RUNNER_DIR"
grep -q 'RUNNER_DIR' "$PLUGIN_DIR/actions/status.sh" && pass "status.sh references RUNNER_DIR" || fail "status.sh missing RUNNER_DIR"
grep -q 'LOG_DIR' "$PLUGIN_DIR/actions/logs.sh" && pass "logs.sh references LOG_DIR" || fail "logs.sh missing LOG_DIR"

# =============================================================================
# Summary
# =============================================================================
echo ""
TOTAL=$((PASS + FAIL))
echo "Results: $PASS/$TOTAL passed"
[[ $FAIL -eq 0 ]] && echo "All tests passed." || { echo "$FAIL test(s) failed."; exit 1; }
