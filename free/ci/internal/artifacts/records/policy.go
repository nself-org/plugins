package records

// Purpose: the byte caps, the free-disk floor and the TEAM project share that
// bound artifact writes (Epic CACHE D5/D6).
// Inputs:  environment (NSELF_CI_ARTIFACT_MAX_FILE, NSELF_CI_ARTIFACT_MAX_BYTES,
// NSELF_CI_STORE_MIN_FREE, NSELF_CI_PROJECT_SHARE), the store usage, an
// injectable statfs and clock.
// Outputs: Policy, its checks and the 80 % warning text.
// Constraints: a malformed value is an error (E712 config), never a silent
// default; Statfs is injected in tests; TEAM mode is the caller's switch.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Defaults of the caps.
const (
	DefaultMaxFile      = int64(2) << 30
	DefaultMaxTotal     = int64(20) << 30
	defaultFloorBytes   = int64(5) << 30
	defaultFloorPercent = 10
	defaultShare        = 50
	warnPercent         = 80
)

// Usage is what the ledger holds now; bytes count committed and staged records.
type Usage struct {
	Total, Release, Project, ProjectRelease int64
}

// Policy bounds one artifact write.
type Policy struct {
	MaxFile, MaxTotal int64
	MinFreeBytes      int64 // 0 with MinFreePercent 0: the default floor
	MinFreePercent    int
	ProjectShare      int // percent of each cap one project may hold (TEAM only)
	Team              bool
	StorePath         string // directory whose volume the floor is measured on
	Statfs            func(path string) (free, total uint64, err error)
	Warn              func(string)
}

// PolicyFromEnv reads the caps; get is os.Getenv in production.
func PolicyFromEnv(get func(string) string) (Policy, error) {
	p := Policy{MaxFile: DefaultMaxFile, MaxTotal: DefaultMaxTotal, ProjectShare: defaultShare}
	for _, f := range []struct {
		name string
		dst  *int64
	}{{"NSELF_CI_ARTIFACT_MAX_FILE", &p.MaxFile}, {"NSELF_CI_ARTIFACT_MAX_BYTES", &p.MaxTotal}} {
		if v := get(f.name); v != "" {
			n, err := parseSize(v)
			if err != nil || n <= 0 || strings.HasSuffix(v, "%") {
				return p, failf(ReasonConfig, "%s=%q is not a positive size", f.name, v)
			}
			*f.dst = n
		}
	}
	if v := get("NSELF_CI_STORE_MIN_FREE"); v != "" {
		if strings.HasSuffix(v, "%") {
			n, err := strconv.Atoi(strings.TrimSuffix(v, "%"))
			if err != nil || n < 0 || n > 100 {
				return p, failf(ReasonConfig, "NSELF_CI_STORE_MIN_FREE=%q is not a size or percent", v)
			}
			p.MinFreePercent = n
		} else {
			n, err := parseSize(v)
			if err != nil || n < 0 {
				return p, failf(ReasonConfig, "NSELF_CI_STORE_MIN_FREE=%q is not a size or percent", v)
			}
			p.MinFreeBytes = n
		}
	}
	if v := get("NSELF_CI_PROJECT_SHARE"); v != "" {
		n, err := strconv.Atoi(strings.TrimSuffix(v, "%"))
		if err != nil || n < 1 || n > 100 {
			return p, failf(ReasonConfig, "NSELF_CI_PROJECT_SHARE=%q is not a percent of 1-100", v)
		}
		p.ProjectShare = n
	}
	return p, nil
}

var sizeUnits = []struct {
	suffix string
	mult   int64
}{{"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10}, {"GB", 1e9}, {"MB", 1e6}, {"KB", 1e3}, {"B", 1}}

// parseSize reads "123", "5GiB" or "512MB" (a bare number is bytes).
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	mult := int64(1)
	for _, u := range sizeUnits {
		if strings.HasSuffix(s, u.suffix) {
			s, mult = strings.TrimSpace(strings.TrimSuffix(s, u.suffix)), u.mult
			break
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 || n > (1<<62)/mult {
		return 0, fmt.Errorf("bad size")
	}
	return n * mult, nil
}

func statfs(p Policy) func(string) (uint64, uint64, error) {
	if p.Statfs != nil {
		return p.Statfs
	}
	return volumeSpace
}

// floor is the free-space floor on a volume of total bytes: the larger of the
// configured bytes and percent; with neither set, the larger of 5 GiB and 10 %.
func (p Policy) floor(total uint64) uint64 {
	b, pct := p.MinFreeBytes, p.MinFreePercent
	if b == 0 && pct == 0 {
		b, pct = defaultFloorBytes, defaultFloorPercent
	}
	return max(uint64(b), total/100*uint64(pct))
}

// CheckDisk refuses a write when the volume of StorePath holds less than the
// floor plus the bytes about to land. An unreadable volume refuses too.
func (p Policy) CheckDisk(add int64) error {
	free, total, err := statfs(p)(p.StorePath)
	if err != nil {
		return failf(ReasonDiskFloor, "cannot read free space of %s: %v", p.StorePath, err)
	}
	add = max(add, 0)
	if floor := p.floor(total); free < floor+uint64(add) {
		return failf(ReasonDiskFloor, "free space %d B is below the floor %d B (+%d B to write); free disk or lower NSELF_CI_STORE_MIN_FREE", free, floor, add)
	}
	return nil
}

// CheckFile refuses one file above NSELF_CI_ARTIFACT_MAX_FILE.
func (p Policy) CheckFile(name string, size int64) error {
	if size > p.MaxFile {
		return failf(ReasonFileCap, "%s is %d B, above the per-file cap %d B (NSELF_CI_ARTIFACT_MAX_FILE)", name, size, p.MaxFile)
	}
	return nil
}

// CheckCaps decides whether add more bytes fit. Release bytes count and are
// never auto-evicted, so a full store names `prune --release`; at 80 % the
// returned warning names it too.
func (p Policy) CheckCaps(u Usage, add int64) (warning string, err error) {
	hint := "run `nself ci artifacts prune`"
	if u.Release > 0 {
		hint = "run `nself ci artifacts prune --release` (release artifacts are never removed automatically)"
	}
	if p.Team {
		if share := p.MaxTotal / 100 * int64(p.ProjectShare); u.Project+add > share {
			return "", failf(ReasonSizeCap, "project holds %d B of its %d B share (NSELF_CI_PROJECT_SHARE=%d%%); %s", u.Project, share, p.ProjectShare, hint)
		}
	}
	if u.Total+add > p.MaxTotal {
		return "", failf(ReasonSizeCap, "artifact store holds %d B of %d B (NSELF_CI_ARTIFACT_MAX_BYTES); %s", u.Total, p.MaxTotal, hint)
	}
	if pct := int(float64(u.Total+add) * 100 / float64(p.MaxTotal)); pct >= warnPercent {
		return fmt.Sprintf("artifact store is at %d%% of its cap; %s", pct, hint), nil
	}
	return "", nil
}

// StoreDir is the CI home (state.db and blobs live under it): home, else
// NSELF_CI_HOME, else ~/.nself/ci.
func StoreDir(home string) string {
	if home != "" {
		return home
	}
	if h := os.Getenv("NSELF_CI_HOME"); h != "" {
		return h
	}
	d, _ := os.UserHomeDir()
	return filepath.Join(d, ".nself", "ci")
}
