package notify

// Purpose: the Event every sink receives (P7-AUTO-18). One shape for stdout, webhook and
// command sinks, so a receiver parses one document.
// Inputs: a finished run's ids, result and counts (Run), or a digest's counts.
// Outputs: Event (JSON), already redacted and bounded by Redacted.
// Constraints: an event carries ids, counts, result and a pointer to the run, never log
// text, diagnostics or job output. Every string passes the shared redactor and is clipped;
// the failed-job list is capped, so the body stays far under MaxBody.

import (
	"strings"
	"unicode"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

// Kind names what happened.
type Kind string

// The four event kinds.
const (
	KindFailure    Kind = "failure"     // a run failed (once per repo, sha and context)
	KindRecovery   Kind = "recovery"    // a ref turned green after a red run
	KindErrorStuck Kind = "error_stuck" // an infrastructure error ended the run after its retries
	KindDigest     Kind = "digest"      // the daily summary
)

// Kinds lists every kind, the values notify.quiet accepts.
func Kinds() []Kind { return []Kind{KindFailure, KindRecovery, KindErrorStuck, KindDigest} }

// Valid reports whether k is one of the four kinds.
func (k Kind) Valid() bool {
	for _, v := range Kinds() {
		if k == v {
			return true
		}
	}
	return false
}

// Limits that keep an event small.
const (
	MaxBody       = 16 << 10 // the webhook body cap
	maxField      = 200      // characters of any string field
	maxFailedJobs = 10
)

// Summary is the digest's counts.
type Summary struct {
	Since  string `json:"since"`
	Until  string `json:"until"`
	Runs   int    `json:"runs"`
	Passed int    `json:"passed"`
	Failed int    `json:"failed"`
	Errors int    `json:"infra_errors"`
	Queued int    `json:"queued"`
}

// Event is one notification.
type Event struct {
	Kind           Kind          `json:"kind"`
	Time           string        `json:"ts"`
	Message        string        `json:"message"`
	Repo           string        `json:"repo,omitempty"`
	Ref            string        `json:"ref,omitempty"`
	Revision       string        `json:"revision,omitempty"`
	Context        string        `json:"context,omitempty"`
	RunID          string        `json:"run_id,omitempty"`
	Result         string        `json:"result,omitempty"`
	EvidenceDigest string        `json:"evidence_digest,omitempty"`
	Counts         *model.Counts `json:"counts,omitempty"`
	FailedJobs     []string      `json:"failed_jobs,omitempty"`
	Show           string        `json:"show,omitempty"` // the command that opens the run
	Digest         *Summary      `json:"digest,omitempty"`
}

// Redacted returns a copy with every string masked by the shared redactor, control
// characters dropped and lengths bounded.
func (e Event) Redacted() Event {
	s := scrubber{r: redact.New()}
	e.Message, e.Repo, e.Ref, e.Revision = s.clip(e.Message), s.clip(e.Repo), s.clip(e.Ref), s.clip(e.Revision)
	e.Context, e.RunID, e.Result, e.EvidenceDigest, e.Show = s.clip(e.Context), s.clip(e.RunID), s.clip(e.Result), s.clip(e.EvidenceDigest), s.clip(e.Show)
	if len(e.FailedJobs) > maxFailedJobs {
		e.FailedJobs = e.FailedJobs[:maxFailedJobs]
	}
	jobs := make([]string, 0, len(e.FailedJobs))
	for _, j := range e.FailedJobs {
		jobs = append(jobs, s.clip(j))
	}
	e.FailedJobs = jobs
	if len(jobs) == 0 {
		e.FailedJobs = nil
	}
	return e
}

type scrubber struct{ r *redact.Redactor }

func (s scrubber) clip(in string) string {
	f := s.r.NewFilter()
	out := string(f.Redact([]byte(in))) + string(f.Flush())
	out = strings.Map(func(r rune) rune {
		// < > & grow to six bytes in JSON and U+2028/2029 are escaped too: they would break the 16 KiB bound
		if unicode.IsControl(r) || r == '<' || r == '>' || r == '&' || r == '\u2028' || r == '\u2029' {
			return ' '
		}
		return r
	}, strings.TrimSpace(out))
	if r := []rune(out); len(r) > maxField {
		out = string(r[:maxField-1]) + "…"
	}
	return out
}
