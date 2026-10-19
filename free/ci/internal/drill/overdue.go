package drill

// Purpose: the overdue report of `nself ci evidence --pipeline <name>`
// (P7-CI-02, the PipelineReporter hook of P7-CI-38). A scheduled pipeline that
// stops running leaves no failing check, so staleness is read from evidence
// age: every job of the pipeline needs a passing, commit-bound run no older
// than the pipeline's max_age (Constitution 8.3).
// Inputs:  the pipeline name, the repo's .nself/ci.yaml and the local run store.
// Outputs: an evidence.PipelineReport; Overdue makes the command exit 10.
// Constraints: only runs of this repository count; a job that never passed is overdue (reason says so); a pipeline
// without max_age is never overdue; unbound (dirty) passes do not count; the
// repo is NSELF_CI_REPO or the working directory, the store is the one
// `ci runs` reads; reading never creates a store.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/config"
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// JobStatus is the freshness of one job of a pipeline.
type JobStatus struct {
	Job      string  `json:"job"`
	LastPass *string `json:"last_pass"`
	Age      string  `json:"age,omitempty"`
	Overdue  bool    `json:"overdue"`
	Code     string  `json:"code,omitempty"`
	Reason   string  `json:"reason,omitempty"`
}

// Detail is the reporter's description of a pipeline.
type Detail struct {
	MaxAge string      `json:"max_age"`
	Jobs   []JobStatus `json:"jobs"`
}

// Source supplies what a report needs; tests plug a fake.
type Source interface {
	// Pipeline returns the pipeline with its drill jobs expanded.
	Pipeline(ctx context.Context, name string) (model.PipelineDef, error)
	// LastPasses returns, per job id, when it last passed in a run of the pipeline.
	LastPasses(ctx context.Context, name string, jobs []string) (map[string]time.Time, error)
}

// Reporter implements evidence.PipelineReporter.
type Reporter struct {
	Source Source
	Now    func() time.Time
}

func init() { evidence.RegisterPipelineReporter("*", Reporter{Source: localSource{}, Now: time.Now}) }

// Report builds the overdue report of one pipeline.
func (r Reporter) Report(ctx context.Context, name string) (evidence.PipelineReport, error) {
	def, err := r.Source.Pipeline(ctx, name)
	if err != nil {
		return evidence.PipelineReport{}, err
	}
	rep := evidence.PipelineReport{Pipeline: name}
	d := Detail{MaxAge: def.MaxAge, Jobs: []JobStatus{}}
	var maxAge time.Duration
	if def.MaxAge != "" {
		if maxAge, err = time.ParseDuration(def.MaxAge); err != nil || maxAge <= 0 {
			return rep, fail(CodeInvalid, "pipeline %q has max_age %q, which is not a positive duration", name, def.MaxAge)
		}
	}
	jobs := append([]string(nil), def.Jobs...)
	sort.Strings(jobs)
	passes, err := r.Source.LastPasses(ctx, name, jobs)
	if err != nil {
		return rep, err
	}
	now := r.Now()
	for _, j := range jobs {
		s := JobStatus{Job: j}
		if at, ok := passes[j]; ok {
			stamp := at.UTC().Format(time.RFC3339)
			s.LastPass, s.Age = &stamp, now.Sub(at).Round(time.Minute).String()
			if maxAge > 0 && now.Sub(at) > maxAge {
				s.Overdue, s.Code = true, CodeOverdue
				s.Reason = fmt.Sprintf("last pass %s ago, older than max_age %s", s.Age, maxAge)
			}
		} else if maxAge > 0 {
			s.Overdue, s.Code, s.Reason = true, CodeOverdue, "no passing run recorded on this machine"
		}
		rep.Overdue = rep.Overdue || s.Overdue
		d.Jobs = append(d.Jobs, s)
	}
	rep.Detail = d
	return rep, nil
}

// localSource reads the repo config and the run store of this machine.
type localSource struct{}

func repoRoot() (string, error) {
	if r := os.Getenv("NSELF_CI_REPO"); r != "" {
		return filepath.Abs(r)
	}
	return os.Getwd()
}

func (localSource) Pipeline(_ context.Context, name string) (model.PipelineDef, error) {
	root, err := repoRoot()
	if err != nil {
		return model.PipelineDef{}, err
	}
	loaded, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		return model.PipelineDef{}, err
	}
	if _, ok := loaded.Config.Pipelines[name]; !ok {
		return model.PipelineDef{}, fail(CodeInvalid, "unknown pipeline %q", name)
	}
	cfg, err := Expand(root, loaded.Config, name, "")
	if err != nil {
		return model.PipelineDef{}, err
	}
	return cfg.Pipelines[name], nil
}

// maxRuns bounds how far back LastPasses reads.
const maxRuns = 1000

func (localSource) LastPasses(ctx context.Context, name string, jobs []string) (map[string]time.Time, error) {
	out := map[string]time.Time{}
	root, err := repoRoot()
	if err != nil {
		return nil, err
	}
	path := evidence.StorePath()
	if !evidence.StoreExists(path) {
		return out, nil
	}
	st, err := cirun.OpenStore(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = st.Close() }()
	want := map[string]bool{}
	for _, j := range jobs {
		want[j] = true
	}
	var cursor int64
	for seen := 0; seen < maxRuns && len(out) < len(want); {
		page, err := st.History(ctx, cursor, 100)
		if err != nil {
			return nil, err
		}
		if len(page.Pipelines) == 0 {
			break
		}
		seen += len(page.Pipelines)
		cursor = page.NextCreatedAt
		for _, row := range page.Pipelines {
			if !sameDir(row.Project, root) { // the store is shared by every repo on this machine
				continue
			}
			docs, err := evidence.ForRun(ctx, st, row)
			if err != nil {
				return nil, err
			}
			for _, doc := range docs {
				notePasses(out, want, name, doc)
			}
		}
	}
	return out, nil
}

// sameDir reports whether two paths name the same directory.
func sameDir(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

// notePasses records the newest bound pass per wanted job of a run document.
func notePasses(out map[string]time.Time, want map[string]bool, name string, doc evidence.Stored) {
	ev := doc.Evidence
	if ev.Pipeline.Name != name || ev.Binding.State != "bound" {
		return
	}
	at := time.Unix(0, doc.Created)
	if ev.Timings.FinishedAt != nil {
		if t, err := time.Parse(time.RFC3339, *ev.Timings.FinishedAt); err == nil {
			at = t
		}
	}
	for _, c := range ev.Checks {
		if c.Result == "pass" && want[c.JobID] && at.After(out[c.JobID]) {
			out[c.JobID] = at
		}
	}
}
