#!/usr/bin/env bash
# vendor.sh [--dry-run] [--tidy] <plugin dir>: make every Go module of the plugin build from its own
# extracted tarball. Replace targets outside the plugin dir are copied to third_party/<name> and the
# replace is rewritten to it; `go mod vendor` refreshes vendor/; Dockerfile `go build` gets -mod=vendor.
# Only the plugin's own modules are vendored: not third_party/ (copied replace targets) and not a module that
# another module of the plugin reaches through a local replace (its packages land in that module's vendor/).
# All-or-nothing: any failure restores the plugin directory exactly.
# Prints one line: "vendor <state> [detail]". State: converted | noop | none | manual: <why> | blocked: <why>.
set -u
. "$(dirname "$0")/../conformance/lib.sh"
dry=0; tidy=0
while [ $# -gt 1 ]; do case $1 in --dry-run) dry=1;; --tidy) tidy=1;; esac; shift; done
dir=$1; root=$(norm_path "$dir")
mods=$(own_go_mods "$dir"); [ -n "$mods" ] || { echo "vendor none: no go.mod"; exit 0; }
# directories reached through a local replace from inside the plugin (skipped as vendor roots)
# A Dockerfile that downloads modules at build time cannot become a no-network build by editing go.mod alone:
# report manual before anything is written.
for df in "$dir"/Dockerfile*; do
  [ -f "$df" ] || continue
  if grep -q 'go mod download' "$df"; then echo "vendor manual: Dockerfile runs go mod download (needs network): $(basename "$df")"; exit 0; fi
done
reached=""
for gm in $mods; do
  while read -r mod target; do
    [ -n "$mod" ] || continue
    abs=$(norm_path "$(dirname "$gm")/$target"); case "$target" in /*) abs=$(norm_path "$target");; esac
    case "$abs/" in "$root"/*) reached="$reached $abs";; esac
  done <<EOF
$(local_replaces "$gm")
EOF
done
[ $dry -eq 1 ] || { snap_take "$dir" || { echo "vendor blocked: cannot snapshot $dir"; exit 1; }; }
bail() { echo "vendor blocked: $1"; [ $dry -eq 1 ] || snap_restore; exit 1; }
tree_state() { find "$dir" -type f \( -name go.mod -o -name go.sum -o -name 'Dockerfile*' -o -path '*/vendor/*' -o -path '*/third_party/*' \) | sort | while IFS= read -r x; do printf '%s %s\n' "$x" "$(sha256_of "$x")"; done | { sha256sum 2>/dev/null || shasum -a 256; } | cut -d' ' -f1; }
before=$(tree_state); notes=""; would=""
for gm in $mods; do
  base=$(dirname "$gm"); babs=$(norm_path "$base")
  while read -r mod target; do
    [ -n "$mod" ] || continue
    abs=$(norm_path "$base/$target"); case "$target" in /*) abs=$(norm_path "$target");; esac
    case "$abs/" in "$root"/*) continue;; esac
    [ -d "$abs" ] || bail "replace target $target of $mod does not exist"
    dest=$root/third_party/$(basename "$abs"); would="$would copy:$(basename "$abs")"
    [ $dry -eq 1 ] && continue
    rm -rf "$dest"; mkdir -p "$dest"
    ( cd "$abs" && tar cf - --exclude=.git --exclude=vendor --exclude=node_modules . ) | ( cd "$dest" && tar xf - )
    rel=$(python3 -c 'import os,sys; print(os.path.relpath(sys.argv[1], sys.argv[2]))' "$dest" "$base"); case $rel in .*) ;; *) rel=./$rel;; esac
    ( cd "$base" && go mod edit -replace="$mod=$rel" ); notes="$notes replace:$mod"
  done <<EOF
$(local_replaces "$gm")
EOF
  case " $reached " in *" $babs "*) continue;; esac
  if [ $dry -eq 1 ]; then [ -d "$base/vendor" ] || would="$would vendor:$(basename "$base")"; continue; fi
  if [ $tidy -eq 1 ]; then ( cd "$base" && GOFLAGS=-mod=mod go mod tidy >/dev/null 2>&1 ) || bail "go mod tidy failed in $gm"; fi
  ( cd "$base" && GOFLAGS=-mod=mod go mod vendor 2>"${TMPDIR:-/tmp}/vendor.$$.err" ) || { m=$(tail -1 "${TMPDIR:-/tmp}/vendor.$$.err"); rm -f "${TMPDIR:-/tmp}/vendor.$$.err"; bail "go mod vendor failed in $gm: $m"; }
  rm -f "${TMPDIR:-/tmp}/vendor.$$.err"
done
for df in "$dir"/Dockerfile*; do
  [ -f "$df" ] || continue
  if grep -q 'go build' "$df" && grep 'go build' "$df" | grep -qv -e '-mod='; then
    would="$would dockerfile:-mod=vendor"
    [ $dry -eq 0 ] && awk '/go build/ && !/-mod=/ { sub(/go build/, "go build -mod=vendor") } { print }' "$df" > "$df.tmp" && mv "$df.tmp" "$df"
  fi
done
if [ $dry -eq 1 ]; then [ -n "$would" ] && echo "vendor converted would:$would" || echo "vendor noop"; exit 0; fi
snap_drop
[ "$before" = "$(tree_state)" ] && echo "vendor noop" || echo "vendor converted$notes"
