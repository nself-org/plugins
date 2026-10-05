#!/usr/bin/env bash
# record.sh --meta DIR [--root DIR] [--fixtures]: the write step of plugin-images.yml. Turns the build metadata
# (DIR/<slug>/metadata.json from build.sh --push=true) into images.d/plugins.json, then re-assembles images.json.
# It writes exactly those two files; fragments of other owners (images.d/<other>.json) are never touched.
# Exit: 0 ok, 1 missing or malformed digest, 2 usage.
set -eu
. "$(dirname "$0")/lib.sh"
img_need jq
root=$IMG_ROOT; meta=""; fixtures=0
while [ $# -gt 0 ]; do
  case $1 in
    --meta) meta=$2; shift 2 ;;
    --root) root=$2; shift 2 ;;
    --fixtures) fixtures=1; shift ;;
    *) echo "usage: record.sh --meta DIR [--root DIR] [--fixtures]" >&2; exit 2 ;;
  esac
done

record() {
  local m slug version digest rows="" tmp
  for m in "$meta"/*/metadata.json; do
    [ -e "$m" ] || continue
    slug=$(basename "$(dirname "$m")")
    version=$(jq -r '.version // empty' "$m")
    digest=$(jq -r '.["containerimage.digest"] // empty' "$m")
    printf '%s' "$digest" | grep -Eq '^sha256:[0-9a-f]{64}$' || { echo "record: $slug: no valid containerimage.digest in $m" >&2; return 1; }
    [ -n "$version" ] || { echo "record: $slug: no version in $m" >&2; return 1; }
    rows="$rows$(jq -nc --arg n "$slug" --arg i "nself/nself-$slug:$version@$digest" \
      '{name: $n, kind: "plugin", image: $i, platforms: ["linux/amd64", "linux/arm64"], owner: "plugin-images"}')
"
  done
  [ -n "$rows" ] || { echo "record: no metadata under $meta" >&2; return 1; }
  mkdir -p "$root/images.d"
  tmp=$(mktemp "$root/images.d/.plugins.XXXXXX")
  printf '%s' "$rows" | jq -s 'sort_by(.name)' > "$tmp"
  mv "$tmp" "$root/images.d/plugins.json"
  bash "$IMG_DIR/assemble.sh" --dir "$root/images.d" --out "$root/images.json"
}

if [ "$fixtures" = 1 ]; then
  t=$(img_tmp); trap 'rm -rf "$t"' EXIT; rc=0
  ok() { echo "pass  $1"; }; no() { echo "FAIL  $1"; rc=1; }
  d1=sha256:$(printf '1%.0s' $(seq 64)); d2=sha256:$(printf '2%.0s' $(seq 64))
  mkdir -p "$t/r/images.d" "$t/m/notify" "$t/m/backup"
  printf '{"containerimage.digest":"%s","version":"1.2.6"}' "$d1" > "$t/m/notify/metadata.json"; printf '{"containerimage.digest":"%s","version":"1.0.0"}' "$d2" > "$t/m/backup/metadata.json"
  printf '{"name":"runner-base","kind":"ci","image":"nself/ci-base:2@sha256:%s","platforms":["linux/amd64"],"owner":"P7-TEST-CI"}\n' "$(printf 'd%.0s' $(seq 64))" > "$t/r/images.d/ci.json"
  printf '[{"name":"stale","kind":"plugin","image":"nself/nself-stale:1@sha256:%s","platforms":["linux/amd64"],"owner":"plugin-images"}]\n' "$(printf 's%.0s' $(seq 64))" > "$t/r/images.d/plugins.json"
  printf 'text\n' > "$t/r/images.d/README.md"; cp "$t/r/images.d/ci.json" "$t/ci.before"
  ( cd "$t/r" && find . -type f | sort | xargs shasum ) > "$t/before"
  bash "$0" --meta "$t/m" --root "$t/r" >/dev/null 2>&1 && ok "record runs over build metadata" || no "record failed"
  ( cd "$t/r" && find . -type f | sort | xargs shasum ) > "$t/after"
  changed=$(diff "$t/before" "$t/after" | grep '^[<>]' | awk '{print $3}' | sort -u | tr '\n' ' ')
  [ "$changed" = "./images.d/plugins.json ./images.json " ] && ok "only images.d/plugins.json and images.json are written" || no "files changed: $changed"
  cmp -s "$t/ci.before" "$t/r/images.d/ci.json" && jq -e '.images["runner-base"].owner == "P7-TEST-CI"' "$t/r/images.json" >/dev/null && ok "another owner's fragment survives the regeneration" || no "ci fragment lost"
  jq -e '.images.notify.image == "nself/nself-notify:1.2.6@'"$d1"'" and .images.backup.image == "nself/nself-backup:1.0.0@'"$d2"'" and (.images | has("stale") | not)' "$t/r/images.json" >/dev/null && ok "plugin entries are pinned by digest and the stale plugin is replaced" || no "plugin entries wrong"
  bash "$IMG_DIR/assemble.sh" --check --dir "$t/r/images.d" --out "$t/r/images.json" >/dev/null 2>&1 && ok "the result passes assemble --check" || no "result drifts"
  printf '{"version":"1.0.0"}' > "$t/m/backup/metadata.json"; cp "$t/r/images.json" "$t/images.keep"
  if bash "$0" --meta "$t/m" --root "$t/r" >/dev/null 2>&1; then no "metadata without a digest was accepted"; else cmp -s "$t/images.keep" "$t/r/images.json" && ok "metadata without a digest fails and writes nothing" || no "failed run changed images.json"; fi
  exit $rc
fi

[ -n "$meta" ] || { echo "usage: record.sh --meta DIR [--root DIR]" >&2; exit 2; }
record
