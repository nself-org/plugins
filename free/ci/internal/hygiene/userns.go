package hygiene

// Purpose: ProbeUserns decides whether user-mode Linux can separate a job from the agent with an
// unprivileged user namespace, by trying to read a 0600 file the agent owns from inside one.
// Inputs:  a directory the agent owns and the first subordinate UID to map the job to.
// Outputs: a Probe; the agent reports Available as `isolation.userns`.
// Constraints: the namespace is created through internal/exec (`unshare --user --map-users=0:<sub>:1`);
// reading the probe file must FAIL for the mapping to count; the probe file is always removed.

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/exec"
)

// Probe reasons.
const (
	ProbeOK         = "userns.ok"
	ProbeNoSubuid   = "userns.no_subuid"
	ProbeUnavail    = "userns.unavailable"
	ProbeReadable   = "userns.probe_readable"
	ProbeNotLinux   = "userns.not_linux"
	probeFileSecret = "agent-private probe, not a secret\n"
)

// Probe is the outcome of ProbeUserns.
type Probe struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
}

// ParseSubuid returns the first subordinate range of the user in the /etc/subuid format
// (`name-or-uid:start:count`).
func ParseSubuid(r io.Reader, user string, uid int) (start, count int, ok bool) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Split(strings.TrimSpace(sc.Text()), ":")
		if len(f) != 3 || (f[0] != user && f[0] != strconv.Itoa(uid)) {
			continue
		}
		s, e1 := strconv.Atoi(f[1])
		c, e2 := strconv.Atoi(f[2])
		if e1 == nil && e2 == nil && s > 0 && c > 0 {
			return s, c, true
		}
	}
	return 0, 0, false
}

// ProbeUserns runs the probe in a fresh subdirectory of dir.
func ProbeUserns(ctx context.Context, dir string, subUID int) (Probe, error) {
	if subUID <= 0 {
		return Probe{Reason: ProbeNoSubuid}, nil
	}
	work, err := os.MkdirTemp(dir, "userns-probe-")
	if err != nil {
		return Probe{}, err
	}
	defer os.RemoveAll(work)
	// the directory is traversable so only the 0600 file mode can deny the read
	if err := os.Chmod(work, 0o755); err != nil {
		return Probe{}, err
	}
	file := filepath.Join(work, "probe")
	if err := os.WriteFile(file, []byte(probeFileSecret), 0o600); err != nil {
		return Probe{}, err
	}
	readable, ok, err := exec.UsernsProbe(ctx, subUID, file)
	switch {
	case err != nil && exec.IsRuntimeMissing(err):
		return Probe{Reason: ProbeUnavail}, nil
	case err != nil:
		if ctx.Err() != nil {
			return Probe{}, ctx.Err()
		}
		return Probe{Reason: ProbeUnavail}, nil
	case !ok:
		return Probe{Reason: ProbeUnavail}, nil
	case readable:
		return Probe{Reason: ProbeReadable}, nil
	}
	return Probe{Available: true, Reason: ProbeOK}, nil
}
