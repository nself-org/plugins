package darwin

import "fmt"

// ownershipScan checks every mounted volume without crossing devices or following symlinks.
// Inputs: a fixed find predicate; output: an owned path or an incomplete-scan marker.
// Constraints: one 30-second cap spans all volumes; errors fail closed so an unchecked UID is never freed.
func ownershipScan(match string) string {
	return mountedOwnershipScan("/sbin/mount", match, 30)
}

// mountedOwnershipScan accepts the native mount listing. Ambiguous or malformed lines
// fail closed; mountpoints containing ordinary spaces are preserved without word splitting.
func mountedOwnershipScan(command, match string, seconds int) string {
	return fmt.Sprintf(`scan_deadline=$(($(date +%%s) + %d))
mounts=$(%s) || { echo scan-incomplete; exit 0; }
[ -n "$mounts" ] || { echo scan-incomplete; exit 0; }
paths=$(echo "$mounts" | awk '
  {
    if (split($0, parts, " on ") != 2) { bad=1; next }
    tail=parts[2]; at=0
    for (i=1; i<length(tail); i++) if (substr(tail,i,2)==" (") at=i
    if (!at || substr(tail,length(tail),1)!=")") { bad=1; next }
    dir=substr(tail,1,at-1)
    details=substr(tail,at+2,length(tail)-at-2)
    split(details, flags, ","); kind=flags[1]
    if (substr(dir,1,1)!="/" || kind=="" || index(kind," ") || index(kind,"(") || index(kind,")")) { bad=1; next }
    if (kind!="devfs" && kind!="autofs") print dir
  }
  END { if (bad) print "scan-incomplete" }
')
[ -n "$paths" ] || { echo scan-incomplete; exit 0; }
echo "$paths" | while IFS= read -r d; do
  [ "$d" != scan-incomplete ] || { echo scan-incomplete; break; }
  [ -d "$d" ] || { echo scan-incomplete; break; }
%s
done`, seconds, command, ownershipVolumeScript(match))
}

func boundedOwnershipScan(dirs, match string, seconds int) string {
	return fmt.Sprintf(`scan_deadline=$(($(date +%%s) + %d))
for d in %s; do
  [ -d "$d" ] || continue
%s
done`, seconds, dirs, ownershipVolumeScript(match))
}

func ownershipVolumeScript(match string) string {
	return fmt.Sprintf(`  scan_budget=$((scan_deadline - $(date +%%s)))
  if [ "$scan_budget" -le 0 ]; then echo scan-incomplete; break; fi
  find "$d" -xdev %s -print -quit 2>/dev/null &
  scan_pid=$!
  (sleep "$scan_budget" & sleeper=$!; trap 'kill "$sleeper" 2>/dev/null || true; exit 0' TERM; wait "$sleeper"; kill -KILL "$scan_pid" 2>/dev/null || true) >/dev/null 2>&1 &
  timer_pid=$!
  scan_rc=0
  wait "$scan_pid" 2>/dev/null || scan_rc=$?
  kill "$timer_pid" 2>/dev/null || true
  wait "$timer_pid" 2>/dev/null || true
  if [ "$scan_rc" -ne 0 ]; then echo scan-incomplete; break; fi`, match)
}
