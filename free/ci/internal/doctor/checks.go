package doctor

// Purpose: the local checks CI-D001–CI-D009 and the entry point that runs
// them. Each check has a stable id and a remediation a person can act on.
// Inputs:  Deps (environment, working directory, process runner, store
// opener) and the repository root.
// Outputs: the ordered checks.
// Constraints: see the package comment. CI-D001 toolchain (one entry per
// detected stack), CI-D002 Docker, CI-D003 status posting (gh auth and GitHub
// remote), CI-D004 store opens, CI-D005 store version, CI-D006
// .nself/ci.yaml validates, CI-D007 no leftover .ci.yaml, CI-D008
// .nself/ci.yaml not git-ignored. CI-D090–CI-D099 are placeholders and
// framework notices (sections.go).

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// CaptureFunc runs argv through the exec funnel and returns its combined
// output and exit code; err is set only when the process could not run.
type CaptureFunc func(ctx context.Context, argv ...string) (string, int, error)

// Deps are the process facts the checks read; tests replace them.
type Deps struct {
	Getenv    func(string) string
	Getwd     func() (string, error)
	UserHome  func() (string, error)
	Capture   CaptureFunc
	OpenStore func(path string) error
}

// DefaultDeps reads the real process.
func DefaultDeps() Deps {
	return Deps{
		Getenv: os.Getenv, Getwd: os.Getwd, UserHome: os.UserHomeDir,
		Capture: report.Capture,
		OpenStore: func(path string) error {
			st, err := store.Open(path, store.Options{})
			if err != nil {
				return err
			}
			return st.Close()
		},
	}
}

// RepoRoot resolves the repository: the flag value, then NSELF_CI_REPO, then
// the working directory.
func RepoRoot(flagValue string, d Deps) (string, error) {
	if flagValue == "" {
		flagValue = d.Getenv("NSELF_CI_REPO")
	}
	if flagValue != "" {
		return filepath.Abs(flagValue)
	}
	return d.Getwd()
}

// StorePath is the state database the run commands use.
func StorePath(d Deps) (string, error) {
	home := d.Getenv("NSELF_CI_HOME")
	if home == "" {
		user, err := d.UserHome()
		if err != nil {
			return "", err
		}
		home = filepath.Join(user, ".nself", "ci")
	}
	return filepath.Join(home, "state.db"), nil
}

// RunLocal runs the local checks for the repository at root.
func RunLocal(ctx context.Context, root string, d Deps) []Check {
	configCheck, cfg := checkConfig(root)
	var out []Check
	out = append(out, checkToolchains(ctx, root, cfg, d)...)
	out = append(out, checkDocker(ctx, cfg, d))
	out = append(out, checkStatusPosting(ctx, root, d))
	out = append(out, checkStore(d)...)
	out = append(out, configCheck, checkLeftover(root), checkIgnored(ctx, root, d))
	sortChecks(out)
	return out
}

// requiredJobs returns the sorted ids of the configured jobs that must pass
// (required defaults to true) and satisfy need.
func requiredJobs(cfg *model.PipelineConfig, need func(model.Job) bool) []string {
	if cfg == nil {
		return nil
	}
	var ids []string
	for id, j := range cfg.Jobs {
		if (j.Required == nil || *j.Required) && need(j) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}
