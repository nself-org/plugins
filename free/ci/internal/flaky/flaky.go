package flaky

// Purpose: find flaky checks in run history (P7-CI-40, cap:ci.diagnostics): a
// check whose result differed (pass in one run, fail in another) on identical
// inputs, or that passed on an explicit --confirm-flaky rerun after failing.
// Inputs:  decoded evidence documents, newest first (Detect), or the store (Scan).
// Outputs: Flake rows with the runs behind each one.
// Constraints: read-only and informational: a listing never edits evidence,
// never changes a result and never reads as a pass; "identical inputs" means
// the same repo, revision, tracked-changes digest, pipeline, policy, source trust and
// input digests (config, lockfiles, generated files); dirty or unbound runs are skipped; error and skip results are ignored
// (an infra error says nothing about the code); adhoc runs are ignored.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Sources of a flake.
const (
	SourceHistory      = "history"      // the result differed between runs on identical inputs
	SourceConfirmation = "confirmation" // a --confirm-flaky rerun passed after the job failed
)

// Doc is one run's evidence with the run id.
type Doc struct {
	RunID    string
	Evidence model.Evidence
}

// RunResult is one run's result for a flaky check.
type RunResult struct {
	Run    string            `json:"run"`
	Result model.CheckResult `json:"result"`
	At     string            `json:"at,omitempty"`
}

// Flake is a check that passed and failed on identical inputs.
type Flake struct {
	Check    string      `json:"check"`
	Job      string      `json:"job"`
	Pipeline string      `json:"pipeline"`
	Revision string      `json:"revision"`
	Source   string      `json:"source"`
	Passes   int         `json:"passes"`
	Fails    int         `json:"fails"`
	Runs     []RunResult `json:"runs"`
}

// groupKey names identical inputs for one check.
func groupKey(ev model.Evidence, c model.Check) string {
	in, _ := json.Marshal(ev.InputDigests)
	tracked := ""
	if ev.TrackedChangesDigest != nil {
		tracked = *ev.TrackedChangesDigest
	}
	sum := sha256.Sum256(append(in, []byte("\x00"+ev.RepoID+"\x00"+ev.Revision+"\x00"+tracked+"\x00"+ev.Pipeline.Name+"\x00"+c.ID+"\x00"+c.JobID+"\x00"+ev.PolicyDigest+"\x00"+string(ev.SourceTrust))...))
	return hex.EncodeToString(sum[:])
}

// comparable reports whether a run's inputs are fully captured: a dirty or
// unbound worktree can differ in files the digests do not cover (untracked
// files), so its result says nothing about identical inputs.
func comparable(ev model.Evidence) bool {
	return ev.WorktreeState != "dirty" && ev.Binding.State != "unbound"
}

// Detect lists the flaky checks in docs (newest first, as the store returns them).
func Detect(docs []Doc) []Flake {
	groups := map[string]*Flake{}
	seen := map[string]map[string]bool{} // group -> run ids already counted
	var order []string
	var out []Flake
	for _, d := range docs {
		ev := d.Evidence
		if ev.Pipeline.Kind == "adhoc" || !comparable(ev) {
			continue
		}
		for _, c := range ev.Checks {
			if c.Flaky != nil && c.Flaky.Source == SourceConfirmation {
				out = append(out, confirmed(d, c))
			}
			if c.Result != "pass" && c.Result != "fail" {
				continue
			}
			k := groupKey(ev, c)
			f := groups[k]
			if f == nil {
				f = &Flake{Check: c.ID, Job: c.JobID, Pipeline: ev.Pipeline.Name, Revision: ev.Revision, Source: SourceHistory}
				groups[k], seen[k] = f, map[string]bool{}
				order = append(order, k)
			}
			if seen[k][d.RunID] {
				continue
			}
			seen[k][d.RunID] = true
			f.Runs = append(f.Runs, RunResult{Run: d.RunID, Result: c.Result, At: startedAt(ev)})
			if c.Result == "pass" {
				f.Passes++
			} else {
				f.Fails++
			}
		}
	}
	for _, k := range order {
		if f := groups[k]; f.Passes > 0 && f.Fails > 0 {
			out = append(out, *f)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Check != b.Check {
			return a.Check < b.Check
		}
		if a.Revision != b.Revision {
			return a.Revision < b.Revision
		}
		return a.Source < b.Source
	})
	return out
}

func confirmed(d Doc, c model.Check) Flake {
	return Flake{Check: c.ID, Job: c.JobID, Pipeline: d.Evidence.Pipeline.Name, Revision: d.Evidence.Revision, Source: SourceConfirmation,
		Passes: c.Flaky.Passes, Fails: c.Flaky.Fails, Runs: []RunResult{{Run: d.RunID, Result: c.Result, At: startedAt(d.Evidence)}}}
}

func startedAt(ev model.Evidence) string {
	if ev.Timings.StartedAt != nil {
		return *ev.Timings.StartedAt
	}
	return ""
}
