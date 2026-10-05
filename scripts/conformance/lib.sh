#!/usr/bin/env bash
# lib.sh: shared helpers for the conformance gate, the codemods and the oracle (P7-PLUG-03).
# Source it; it defines functions only. Needs bash 3.2+, jq, go, git, python3.
CONF_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
CONF_DIR=$CONF_ROOT/scripts/conformance
FREE_REPO_URL=https://github.com/nself-org/plugins
LICENSED_REPO_URL=https://github.com/nself-org/bundles

# conf_work prints the scratch directory. Docker (Colima) shares only $HOME, so
# it lives under $HOME by default; CONF_WORKDIR overrides it.
conf_work() { local d=${CONF_WORKDIR:-$HOME/.nself-plugins-conformance}; mkdir -p "$d"; printf '%s\n' "$d"; }
conf_today() { printf '%s\n' "${CONF_TODAY:-$(date +%Y-%m-%d)}"; }
pin_value() { sed -n "s/^$1=//p" "$CONF_ROOT/scripts/cli-tools.version" | head -1; }

# cli_src prints a directory holding the cli source at the pinned commit.
# CLI_TOOLS_SRC overrides it (a checkout or an extracted archive of that commit).
cli_src() {
  if [ -n "${CLI_TOOLS_SRC:-}" ]; then printf '%s\n' "$CLI_TOOLS_SRC"; return; fi
  local sha repo dir; sha=$(pin_value sha); repo=$(pin_value repo)
  dir=$(conf_work)/cli-src-$sha
  if [ ! -f "$dir/go.mod" ]; then
    rm -rf "$dir"; mkdir -p "$dir"
    ( cd "$dir" && git init -q . && git fetch -q --depth 1 "$repo" "$sha" && git checkout -q FETCH_HEAD ) >&2 || { echo "lib: cannot fetch cli $sha" >&2; return 1; }
  fi
  printf '%s\n' "$dir"
}

# cli_tool <name> prints the path of the built cli tool binary (tools/<name>).
cli_tool() {
  local src out; src=$(cli_src) || return 1
  out=$(conf_work)/bin-$(pin_value sha); mkdir -p "$out"
  ( cd "$src" && go build -mod=vendor -o "$out/$1" "./tools/$1" ) >&2 || return 1
  printf '%s\n' "$out/$1"
}

# local_tool <name> <dir> builds a Go program of this repo (own go.mod) into the work dir.
local_tool() {
  local out; out=$(conf_work)/bin-local; mkdir -p "$out"
  ( cd "$2" && go build -o "$out/$1" . ) >&2 || return 1
  printf '%s\n' "$out/$1"
}

sha256_of() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi; }
manifest_version() { jq -r '.manifest_version // 1' "$1/plugin.json"; }
plugin_name() { jq -r '.name // empty' "$1/plugin.json"; }
is_free() { [ "$(jq -r 'if .manifest_version == 2 then (.license // "free") else (if (.isCommercial // false) then "licensed" else "free" end) end' "$1/plugin.json")" != licensed ]; }

# fail <rule> <plugin> <message> prints one gate failure line.
fail() { printf 'FAIL %s %s: %s\n' "$1" "$2" "$3"; }

# yaml_rows <file> <key>...: rows of a flat "- key: value" YAML list as TSV in key order.
yaml_rows() {
  local f=$1; shift
  [ -f "$f" ] || return 0
  awk -v keys="$*" 'BEGIN { n = split(keys, K, " ") }
    function kv(l,  k, val) { k = l; sub(/:.*/, "", k); val = l; sub(/^[^:]*:[[:space:]]*/, "", val); sub(/[[:space:]]+#.*$/, "", val); gsub(/^"|"$/, "", val); v[k] = val }
    function flush(  i, s) { if (has) { s = ""; for (i = 1; i <= n; i++) s = s (i > 1 ? "\t" : "") v[K[i]]; print s } delete v; has = 0 }
    /^[[:space:]]*#/ { next }
    /^[[:space:]]*-[[:space:]]+[A-Za-z_]+:/ { flush(); has = 1; l = $0; sub(/^[[:space:]]*-[[:space:]]+/, "", l); kv(l); next }
    /^[[:space:]]+[A-Za-z_]+:/ { if (has) { l = $0; sub(/^[[:space:]]+/, "", l); kv(l) } next }
    END { flush() }' "$f"
}

# exception_active <exceptions.yaml> <rule> <plugin>: 0 when an unexpired entry matches.
exception_active() {
  local today; today=$(conf_today)
  yaml_rows "$1" rule plugin expires | awk -F'\t' -v r="$2" -v p="$3" -v t="$today" '$1 == r && $2 == p && $3 >= t { f = 1 } END { exit f ? 0 : 1 }'
}

# norm_path <path>: absolute normalised path without requiring it to exist.
norm_path() { python3 -c 'import os,sys; print(os.path.normpath(os.path.abspath(sys.argv[1])))' "$1"; }

# go_mods <plugin dir>: every go.mod outside vendor/ and node_modules/.
go_mods() { find "$1" \( -name vendor -o -name node_modules \) -prune -o -name go.mod -print | sort; }
# own_go_mods <plugin dir>: the plugin's own modules: go_mods minus third_party/ (copied replace targets).
own_go_mods() { go_mods "$1" | grep -v '/third_party/' || true; }

# All-or-nothing edits. snap_take <dir> saves the plugin directory, snap_restore puts it back exactly
# (deleting anything the codemod added), snap_drop discards the copy. Used by every codemod so a failed or
# blocked step leaves the original untouched.
snap_take() { SNAP_DIR=$(mktemp -d "$(conf_work)/snap.XXXXXX"); ( cd "$1" && tar cf "$SNAP_DIR/p.tar" . ) || return 1; SNAP_TARGET=$1; }
snap_restore() { [ -n "${SNAP_DIR:-}" ] || return 0; find "$SNAP_TARGET" -mindepth 1 -maxdepth 1 -exec rm -rf {} +; ( cd "$SNAP_TARGET" && tar xf "$SNAP_DIR/p.tar" ); rm -rf "$SNAP_DIR"; SNAP_DIR=; }
snap_drop() { [ -n "${SNAP_DIR:-}" ] && rm -rf "$SNAP_DIR"; SNAP_DIR=; }

# local_replaces <go.mod>: "<module> <local path>" for every replace to a filesystem path.
local_replaces() {
  awk '/^replace[[:space:]]*\(/ { b = 1; next } b && /^\)/ { b = 0; next }
    { l = $0; if (!b) { if ($1 != "replace") next; sub(/^replace[[:space:]]+/, "", l) }
      if (l ~ /=>/) { split(l, a, "=>"); m = a[1]; sub(/^[[:space:]]+/, "", m); sub(/[[:space:]].*$/, "", m); gsub(/["`]/, "", m)
        p = a[2]; sub(/^[[:space:]]+/, "", p); sub(/[[:space:]].*$/, "", p); gsub(/["`]/, "", p)
        if (p ~ /^(\.|\/)/) print m, p } }' "$1"
}

# go_work_uses <go.work>: every `use` directory, quoted or not, single line or block.
go_work_uses() {
  awk '/^use[[:space:]]*\(/ { b = 1; next } b && /^\)/ { b = 0; next }
    { l = $0; if (!b) { if ($1 != "use") next; sub(/^use[[:space:]]+/, "", l) }
      sub(/^[[:space:]]+/, "", l); sub(/[[:space:]].*$/, "", l); gsub(/["`]/, "", l); sub(/\/\/.*/, "", l); if (l != "") print l }' "$1"
}

# go_works <plugin dir>: every go.work outside vendor/ and node_modules/.
go_works() { find "$1" \( -name vendor -o -name node_modules \) -prune -o -name go.work -print | sort; }

# release_cap: the latest date any exception or oracle delta may expire (the v1.5.0 release window).
# A constant in the gate on purpose: a data file must not be able to raise its own cap.
release_cap() { printf '2026-12-31\n'; }
