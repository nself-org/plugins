package drill

// Purpose: read the ci.evidence document a drill prints and decide how the job
// ends (P7-CI-02). The exit code is the result; when the drill also prints one
// ci.evidence/v1 document on its own line, that document is validated and its
// checks are summarised in the job's check (findings, excerpt).
// Inputs:  the drill's redacted output.
// Outputs: a run.OutcomeChange.
// Constraints: a document that is present but invalid ends the job error
// (E640) whatever the exit code; a process that exits 0 with a nested result
// other than pass never ends passed; no document at all leaves the exit code
// alone; the last matching line wins (stderr noise may come before it).

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

const marker = `"ci.evidence/v1"`

// Document finds the last line of output that looks like a ci.evidence
// document. ok is false when there is none.
func Document(output string, truncated bool) (line string, ok bool) {
	lines := strings.Split(output, "\n")
	if truncated && len(lines) > 0 {
		lines = lines[1:] // the first line may be cut in half
	}
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if strings.HasPrefix(l, "{") && strings.Contains(l, marker) {
			return l, true
		}
	}
	return "", false
}

// prettyDocument matches a pretty-printed document: the schema key on a line of
// its own, which Document cannot read.
var prettyDocument = regexp.MustCompile(`(?m)^\s*"schema"\s*:\s*"ci\.evidence/v1"`)

// Judge decides the end of one drill attempt from its output.
func Judge(output string, truncated bool) (cirun.OutcomeChange, bool) {
	line, ok := Document(output, truncated)
	if !ok {
		if prettyDocument.MatchString(output) {
			e := fail(CodeInvalid, "the drill printed a ci.evidence document over several lines; print it as one line of compact JSON")
			return cirun.OutcomeChange{Fail: true, FailureClass: "code", Err: e, Excerpt: e.Error()}, true
		}
		return cirun.OutcomeChange{}, false
	}
	ev, err := attest.Verify([]byte(line))
	if err != nil {
		e := fail(CodeInvalid, "the drill printed an invalid ci.evidence document: %v", err)
		return cirun.OutcomeChange{Fail: true, FailureClass: "code", Err: e, Excerpt: e.Error()}, true
	}
	ch := cirun.OutcomeChange{Findings: ev.Counts.Fail + ev.Counts.Error, Excerpt: summary(ev)}
	switch ev.Result {
	case "pass":
	case "fail":
		ch.Fail, ch.FailureClass, ch.Detail = true, "code", "the drill's nested evidence result is fail"
	default: // error, cancelled: the drill did not get an answer
		ch.Fail, ch.FailureClass, ch.Detail = true, "infra", fmt.Sprintf("the drill's nested evidence result is %s", ev.Result)
	}
	return ch, true
}

// PassedPrefix starts the last line of the excerpt: the ids of the nested checks that passed, space separated
// (P7-TRUST-25 reads it back as the per-level record of the isolation drill). It is last because the evidence
// excerpt keeps the tail when it is cut.
const PassedPrefix = "passed: "

// maxPassedIDs bounds the passed line so a drill with a huge document cannot fill the excerpt.
const maxPassedIDs = 64

// summary lists the nested checks that did not pass, then the totals, then the ids that passed.
func summary(ev model.Evidence) string {
	var b strings.Builder
	fmt.Fprintf(&b, "nested evidence: %s (pass %d, fail %d, skip %d, error %d)\n", ev.Result, ev.Counts.Pass, ev.Counts.Fail, ev.Counts.Skip, ev.Counts.Error)
	for _, c := range ev.Checks {
		if c.Result == "pass" || c.Result == "skip" {
			continue
		}
		fmt.Fprintf(&b, "  %s %s", c.Result, c.ID)
		if why := firstNonEmpty(string(c.Reason), firstLine(c.Excerpt)); why != "" {
			fmt.Fprintf(&b, ": %s", why)
		}
		b.WriteByte('\n')
	}
	var passed []string
	for _, c := range ev.Checks {
		if c.Result == "pass" && len(passed) < maxPassedIDs && !strings.ContainsAny(c.ID, " \t\r\n") {
			passed = append(passed, c.ID)
		}
	}
	if len(passed) > 0 {
		b.WriteString(PassedPrefix + strings.Join(passed, " ") + "\n")
	}
	return b.String()
}

// hook is the run.OutcomeHook of drill jobs.
type hook struct{}

func init() { cirun.RegisterOutcome(hook{}) }

func (hook) Wants(job model.Job) bool { return job.Kind == "drill" }

func (hook) Outcome(_ context.Context, f cirun.OutcomeFacts) (cirun.OutcomeChange, bool) {
	return Judge(f.Output, f.Truncated)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// firstLine is the first line of s, clipped to 200 bytes.
func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	if len(line) > 200 {
		line = line[:200]
	}
	return line
}
