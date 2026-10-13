#!/usr/bin/env bash
# verify-extraction.sh: prove releases.json holds exactly the release data a hand-kept registry.json carried.
#
# Purpose: P7-PLUG-36 acceptance. The one-time extraction (plugin-counts.sh --init-releases) must lose nothing: for every
#   basis entry that has a checksum, releases.json has a row with the same version, sha256, release tag, tarball URL,
#   tarball and per-platform checksums; it has no row for an entry without a checksum, and no row the basis lacks.
# Usage:   scripts/catalog/verify-extraction.sh <basis-registry.json> [releases.json]
#   e.g.   git show 5431b904:registry.json > /tmp/basis.json && scripts/catalog/verify-extraction.sh /tmp/basis.json
# Output:  one line per difference, then a summary. Exit 0 identical, 1 differences, 2 usage.
set -euo pipefail
BASIS="${1:-}"
REL="${2:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/releases.json}"
[ -f "$BASIS" ] && [ -f "$REL" ] || { echo "usage: $0 <basis-registry.json> [releases.json]" >&2; exit 2; }

diffs="$(jq -r --slurpfile r "$REL" '
  ($r[0].releases) as $rel
  | [ .plugins | to_entries[] | select(.value.checksum != null) | .key as $s | .value as $v | ($rel[$s] // null) as $row
      | if $row == null then "\($s): no row in releases.json"
        else
          ( [ ["version", $v.version, $row.version],
              ["sha256", $v.checksum, $row.sha256],
              ["release_tag", $v.releaseTag, $row.release_tag],
              ["tarball_url", $v.tarballUrl, $row.tarball_url],
              ["tarball", ($v.tarball // $row.tarball // $row.tarball_url), ($row.tarball // $row.tarball_url)],
              ["platform_checksums", ($v.checksums.platforms // null), ($row.platform_checksums // null)] ][]
            | select(.[1] != .[2]) | "\($s): \(.[0]) basis=\(.[1]|tojson) releases=\(.[2]|tojson)" ) // empty
        end ][]?,
    ( [ .plugins | to_entries[] | select(.value.checksum == null) | .key as $s | select($rel[$s] != null) | "\($s): releases.json has a row but the basis entry has no checksum" ][] ),
    ( ($rel | keys) - [.plugins | keys[]] | .[] | "\(.): releases.json row has no basis entry" )' "$BASIS")"

rows="$(jq '.releases | length' "$REL")"
withsum="$(jq '[.plugins[] | select(.checksum != null)] | length' "$BASIS")"
if [ -n "$diffs" ]; then printf '%s\n' "$diffs"; echo "verify-extraction: DIFFERENT ($rows rows, $withsum basis entries with a checksum)" >&2; exit 1; fi
echo "verify-extraction: identical ($rows rows = $withsum basis entries with a checksum)"
