#!/usr/bin/env bash
# Rule exceptions (D13): every entry is complete, expires on or before the cap and has not expired.
set -u
. "$(dirname "$0")/../lib.sh"
file=${1:-$CONF_DIR/exceptions.yaml}; today=$(conf_today); rc=0
cap=$(release_cap)   # fixed in lib.sh; a "# cap:" comment in the data file is ignored
while IFS="$(printf '\t')" read -r rule plugin reason owner exp; do
  [ -n "$rule$plugin$reason$owner$exp" ] || continue
  for f in "$rule" "$plugin" "$reason" "$owner" "$exp"; do [ -n "$f" ] || { fail exceptions "$plugin" "incomplete entry (rule, plugin, reason, owner_ref, expires)"; rc=1; continue 2; }; done
  [ "$exp" \< "$today" ] && { fail exceptions "$plugin" "$rule exception expired $exp (owner $owner)"; rc=1; }
  [ "$exp" \> "$cap" ] && { fail exceptions "$plugin" "$rule exception expires $exp, after the v1.5.0 cap $cap"; rc=1; }
done <<EOF
$(yaml_rows "$file" rule plugin reason owner_ref expires)
EOF
exit $rc
