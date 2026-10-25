#!/usr/bin/env bash
# Rule exceptions (D13): every entry is complete, expires on or before the cap and has not expired.
# Also checks public-ports.yaml (D-0626, the permanent class for deliberate public ports): every entry has a plugin, a reason
# and one port number from 1 to 65535, and no entry repeats. These entries do not expire.
set -u
. "$(dirname "$0")/../lib.sh"
file=${1:-$CONF_DIR/exceptions.yaml}; pub=${2:-$CONF_DIR/public-ports.yaml}; today=$(conf_today); rc=0
cap=$(release_cap)   # fixed in lib.sh; a "# cap:" comment in the data file is ignored
while IFS="$(printf '\t')" read -r rule plugin reason owner exp; do
  [ -n "$rule$plugin$reason$owner$exp" ] || continue
  for f in "$rule" "$plugin" "$reason" "$owner" "$exp"; do [ -n "$f" ] || { fail exceptions "$plugin" "incomplete entry (rule, plugin, reason, owner_ref, expires)"; rc=1; continue 2; }; done
  [ "$exp" \< "$today" ] && { fail exceptions "$plugin" "$rule exception expired $exp (owner $owner)"; rc=1; }
  [ "$exp" \> "$cap" ] && { fail exceptions "$plugin" "$rule exception expires $exp, after the v1.5.0 cap $cap"; rc=1; }
done <<EOF
$(yaml_rows "$file" rule plugin reason owner_ref expires)
EOF
seen=
while IFS="$(printf '\t')" read -r plugin port reason; do
  [ -n "$plugin$port$reason" ] || continue
  for f in "$plugin" "$port" "$reason"; do [ -n "$f" ] || { fail public-ports "$plugin" "incomplete entry (plugin, port, reason)"; rc=1; continue 2; }; done
  case $port in *[!0-9]*) fail public-ports "$plugin" "port \"$port\" is not a single port number"; rc=1; continue;; esac
  { [ "$port" -ge 1 ] && [ "$port" -le 65535 ]; } || { fail public-ports "$plugin" "port $port is outside 1-65535"; rc=1; continue; }
  case " $seen " in *" $plugin:$port "*) fail public-ports "$plugin" "port $port is listed twice"; rc=1;; esac
  seen="$seen $plugin:$port"
done <<EOF
$(yaml_rows "$pub" plugin port reason)
EOF
exit $rc
