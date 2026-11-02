//go:build !windows

package exec

import (
	"context"
	"errors"
	"os"
	osexec "os/exec"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

type gatedTestEnv []string

func (f gatedTestEnv) Environment(context.Context, model.Job) ([]string, error) {
	return append([]string(nil), f...), nil
}

func runEnvDump(t *testing.T, env []string, withLimit bool) ([]string, AttemptResult) {
	t.Helper()
	var out strings.Builder
	spec := JobSpec{AttemptID: "gate-env", CoordinatorID: "coord", Home: t.TempDir(), Command: []string{"env"}, Output: func(b []byte) { out.Write(b) }}
	if withLimit {
		spec.Job.Timeout = "1h"
	}
	res := (&Executor{Isolation: BaselineIsolator{}, Env: gatedTestEnv(env), GatedStart: true}).Run(context.Background(), spec)
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	sort.Strings(lines)
	return lines, res
}

// The gated wrapper is a shell; the payload's environment must still be exactly what the caller set (no PWD,
// OLDPWD, SHLVL or _ from the shell, and a PWD, OLDPWD or SHLVL the caller did set kept byte for byte).
func TestGatedStartKeepsTheEnvironmentExact(t *testing.T) {
	path := os.Getenv("PATH")
	for name, env := range map[string][]string{
		"plain":       {"PATH=" + path, "ONLY=this"},
		"caller vars": {"PATH=" + path, "PWD=/not/the/cwd", "OLDPWD=it's a 'quote'", "SHLVL=7"},
		"empty value": {"PATH=" + path, "PWD="},
	} {
		want := append(append([]string(nil), env...), "NSELF_CI_EXEC_ATTEMPT=gate-env")
		sort.Strings(want)
		for _, limit := range []bool{false, true} {
			got, res := runEnvDump(t, env, limit)
			if res.Err != nil || res.ExitCode != 0 {
				t.Fatalf("%s: gated run: %+v", name, res)
			}
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("%s (limit %v): the payload's environment is not the caller's\n--- want\n%s\n--- got\n%s", name, limit, strings.Join(want, "\n"), strings.Join(got, "\n"))
			}
		}
	}
}

// A missing or non-executable payload is still a start error, not a failure inside the wrapper.
func TestGatedStartMissingBinaryIsAStartError(t *testing.T) {
	for _, cmd := range []string{"definitely-not-a-binary-d1531", "./nope-d1531", t.TempDir()} {
		var errs [2]error
		var codes [2]int
		for i, gated := range []bool{false, true} {
			res := (&Executor{Isolation: BaselineIsolator{}, Env: gatedTestEnv{"PATH=" + os.Getenv("PATH")}, GatedStart: gated}).Run(context.Background(),
				JobSpec{AttemptID: "gate-missing", CoordinatorID: "coord", Home: t.TempDir(), Command: []string{cmd}})
			errs[i], codes[i] = res.Err, res.ExitCode
		}
		if errs[0] == nil || errs[1] == nil || codes[0] != codes[1] {
			t.Errorf("%q: ungated err=%v code=%d, gated err=%v code=%d", cmd, errs[0], codes[0], errs[1], codes[1])
		}
		var ee *osexec.Error
		if errors.As(errs[0], &ee) != errors.As(errs[1], &ee) {
			t.Errorf("%q: the error class differs: %v vs %v", cmd, errs[0], errs[1])
		}
	}
}
