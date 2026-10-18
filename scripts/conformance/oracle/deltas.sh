#!/usr/bin/env bash
# deltas.sh: intentional differences between a v1 original and its converted twin (oracle-deltas.yaml).
# Source it (needs lib.sh). apply_deltas <converted output dir> "<plugin slugs under test>" <deltas.yaml> [<original output dir>]
# Row fields: slug, file (an output file name), key, from, to, owner_ref, expires.
#   from "-"   key is the EXACT full line the converted side has and the original lacks; only a line equal to
#              key, in that file, is removed.
#   to "-"     key is the EXACT full line the ORIGINAL has and the converted side lacks (for example the warning
#              v1.4.12 prints for a v1 manifest it cannot read); only a line equal to key, in that file of the
#              original output dir (4th argument, required for such a row), is removed. `from` is ignored; write "-".
#   otherwise  in that file, lines containing the literal key have the literal `to` replaced by `from`.
# Matching is fixed-string (no regex) and per file. A row applies only when its slug is under test; an applicable
# row that matches nothing is STALE and fails, as does an expired row, a row whose expiry is after release_cap,
# and an incomplete row. Prints one "FAIL oracle-deltas ..." line per problem; returns 1 on any.
apply_deltas() {
  local out=$1 names=$2 yaml=$3 orig=${4:-} rc=0 slug file key from to ref exp today cap n f
  today=$(conf_today); cap=$(release_cap)
  while IFS="$(printf '\t')" read -r slug file key from to ref exp; do
    [ -n "$slug$file$key$from$to$ref$exp" ] || continue
    if [ -z "$slug" ] || [ -z "$file" ] || [ -z "$key" ] || [ -z "$from" ] || [ -z "$to" ] || [ -z "$ref" ] || [ -z "$exp" ]; then
      echo "FAIL oracle-deltas: incomplete row (slug, file, key, from, to, owner_ref, expires all required): '$slug' '$file'"; rc=1; continue; fi
    if [ "$exp" \< "$today" ]; then echo "FAIL oracle-deltas: $slug $file expired $exp (owner $ref)"; rc=1; continue; fi
    if [ "$exp" \> "$cap" ]; then echo "FAIL oracle-deltas: $slug $file expires $exp, after the v1.5.0 cap $cap (owner $ref)"; rc=1; continue; fi
    case " $names " in *" $slug "*) ;; *) continue;; esac
    f=$out/$file
    if [ "$to" = "-" ]; then
      if [ -z "$orig" ]; then echo "FAIL oracle-deltas: $slug row for $file removes an original-only line but no original output dir was given"; rc=1; continue; fi
      f=$orig/$file
    fi
    if [ ! -f "$f" ]; then echo "FAIL oracle-deltas: $slug row names $file, which the oracle did not produce (stale)"; rc=1; continue; fi
    if [ "$to" = "-" ] || [ "$from" = "-" ]; then
      n=$(awk -v k="$key" '$0 == k { c++ } END { print c + 0 }' "$f")
      awk -v k="$key" '$0 != k' "$f" > "$f.tmp" && mv "$f.tmp" "$f"
    else
      n=$(awk -v k="$key" -v t="$to" 'index($0, k) && index($0, t) { c++ } END { print c + 0 }' "$f")
      awk -v k="$key" -v t="$to" -v fr="$from" 'index($0, k) { l = $0; o = ""; while ((i = index(l, t)) > 0) { o = o substr(l, 1, i - 1) fr; l = substr(l, i + length(t)) } $0 = o l } { print }' "$f" > "$f.tmp" && mv "$f.tmp" "$f"
    fi
    [ "$n" -gt 0 ] || { echo "FAIL oracle-deltas: $slug row matched nothing in $file (stale): key '$key'"; rc=1; }
  done <<EOF
$(yaml_rows "$yaml" slug file key from to owner_ref expires)
EOF
  return $rc
}
