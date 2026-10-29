package darwin

import "fmt"

// ownershipScan checks every mounted volume without crossing devices or following symlinks.
// Inputs: a fixed find predicate; output: an owned path or an incomplete-scan marker.
// Constraints: one cap (ownershipBudget) spans all volumes; errors fail closed so an unchecked UID is never freed.
// A hosted macOS runner needs more than 30 seconds to walk its volumes, so the cap is generous and the
// allocation script walks once for the whole id range instead of once per candidate id.
// The script touches no filesystem metadata itself: a missing or hung mountpoint is validated by the
// watchdog-bounded find, so a blocked lookup cannot outlive the budget.
func ownershipScan(match string) string {
	return mountedScan("/sbin/mount", match, "-print -quit", ownershipBudget)
}

// ownershipBudget is the whole-scan cap in seconds.
const ownershipBudget = 600

// heldIDsScan walks the volumes once and prints the uids in lo..hi that own a file, one per line (several
// times each), or scan-incomplete. A failed owner lookup also fails closed.
func heldIDsScan(lo, hi int) string {
	return mountedScan("/sbin/mount", heldIDsMatch(lo, hi), heldIDsAction, ownershipBudget)
}

const heldIDsAction = `-exec sh -c 'stat -f %u "$@" 2>/dev/null || echo scan-incomplete' sh {} +`

func heldIDsMatch(lo, hi int) string { return fmt.Sprintf("-uid +%d -uid -%d", lo-1, hi+1) }

// mountedOwnershipScan accepts the native mount listing. Ambiguous or malformed lines
// fail closed; mountpoints containing ordinary spaces are preserved without word splitting.
func mountedOwnershipScan(command, match string, seconds int) string {
	return mountedScan(command, match, "-print -quit", seconds)
}

func mountedScan(command, match, action string, seconds int) string {
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
%s
done`, seconds, command, ownershipVolumeScript(match, action))
}

func boundedOwnershipScan(dirs, match string, seconds int) string {
	return boundedScan(dirs, match, "-print -quit", seconds)
}

func boundedScan(dirs, match, action string, seconds int) string {
	return fmt.Sprintf(`scan_deadline=$(($(date +%%s) + %d))
for d in %s; do
%s
done`, seconds, dirs, ownershipVolumeScript(match, action))
}

func ownershipVolumeScript(match, action string) string {
	return fmt.Sprintf(`  scan_budget=$((scan_deadline - $(date +%%s)))
  if [ "$scan_budget" -le 0 ]; then echo scan-incomplete; break; fi
  scan_err=$(mktemp "${TMPDIR:-/tmp}/nself-scan.XXXXXX") || { echo scan-incomplete; break; }
  find "$d" -xdev -ignore_readdir_race %s %s 2>"$scan_err" &
  scan_pid=$!
  (sleep "$scan_budget" & sleeper=$!; trap 'kill "$sleeper" 2>/dev/null || true; exit 0' TERM; wait "$sleeper"; kill -KILL "$scan_pid" 2>/dev/null || true) >/dev/null 2>&1 &
  timer_pid=$!
  scan_rc=0
  wait "$scan_pid" 2>/dev/null || scan_rc=$?
  kill "$timer_pid" 2>/dev/null || true
  wait "$timer_pid" 2>/dev/null || true
  if [ "$scan_rc" -ne 0 ]; then head -n 3 "$scan_err" >&2; rm -f "$scan_err"; echo scan-incomplete; break; fi
  rm -f "$scan_err"`, match, action)
}
