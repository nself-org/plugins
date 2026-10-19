package logs

// Purpose: where stored logs live and how their files are named.
// Layout:  <home>/logs/<run>/<attempt>/active.jsonl        the file being written
//          <home>/logs/<run>/<attempt>/seg-<first>-<last>-<bytes>.jsonl.gz
//                                                       a rotated, gzipped segment
//          (first and last are 12-digit line numbers, bytes the uncompressed size).
// Inputs:  the state directory (NSELF_CI_HOME or ~/.nself/ci), run and attempt ids.
// Outputs: paths and parsed segment names.
// Constraints: ids are validated before they touch a path (no separators, no
// dots-only names); directories are 0700 and files 0600, so nothing here is
// readable by another user.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	dirMode    = 0o700
	fileMode   = 0o600
	activeName = "active.jsonl"
	segPrefix  = "seg-"
	segSuffix  = ".jsonl.gz"
)

var validID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// ValidID reports whether s may name a run or an attempt on disk.
func ValidID(s string) bool { return validID.MatchString(s) && !strings.Contains(s, "..") }

// Home resolves the state directory the way the coordinator does: home, else
// NSELF_CI_HOME, else ~/.nself/ci. It is "" when no home directory is known.
func Home(home string) string {
	if home == "" {
		home = os.Getenv("NSELF_CI_HOME")
	}
	if home == "" {
		user, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		home = filepath.Join(user, ".nself", "ci")
	}
	return home
}

// Root is the directory holding every run's logs.
func Root(home string) string { return filepath.Join(Home(home), "logs") }

// RunDir is the directory of one run's attempt logs.
func RunDir(home, run string) string { return filepath.Join(Root(home), run) }

// AttemptDir is the directory of one attempt's files.
func AttemptDir(home, run, attempt string) string { return filepath.Join(RunDir(home, run), attempt) }

// segment is one rotated file.
type segment struct {
	Path        string
	First, Last int64
	Bytes       int64
}

func segmentName(first, last, raw int64) string {
	return fmt.Sprintf("%s%012d-%012d-%d%s", segPrefix, first, last, raw, segSuffix)
}

// parseSegment reads the name of a rotated file.
func parseSegment(name string) (segment, bool) {
	rest, ok := strings.CutPrefix(name, segPrefix)
	if !ok {
		return segment{}, false
	}
	rest, ok = strings.CutSuffix(rest, segSuffix)
	if !ok {
		return segment{}, false
	}
	var s segment
	if n, err := fmt.Sscanf(rest, "%d-%d-%d", &s.First, &s.Last, &s.Bytes); err != nil || n != 3 || s.First < 1 || s.Last < s.First {
		return segment{}, false
	}
	return s, true
}

// segments lists the rotated files in an attempt directory, oldest first.
func segments(dir string) ([]segment, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []segment
	for _, e := range ents {
		if s, ok := parseSegment(e.Name()); ok && e.Type().IsRegular() {
			s.Path = filepath.Join(dir, e.Name())
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].First < out[j].First })
	return out, nil
}

// Attempts lists the attempt ids that have a stored log directory in a run directory.
func Attempts(runDir string) ([]string, error) {
	ents, err := os.ReadDir(runDir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() && ValidID(e.Name()) {
			out = append(out, e.Name())
		}
	}
	return out, nil
}
