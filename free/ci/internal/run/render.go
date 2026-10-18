package run

// Purpose: what a run tells the user. The coordinator emits Events to a
// Renderer; the human renderer is registered here, the JSON renderers are
// added by P7-CI-38 through RegisterRenderer.
// Inputs:  Events and the final Summary. Outputs: text on a writer.
// Constraints: a job that did not run always gets a reason line; the status
// line is exactly `nself-ci: status not posted: <reason>` when nothing was
// posted; output tails are bounded; a renderer never fails the run.

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

// Job end states.
const (
	StatePassed    = "passed"
	StateFailed    = "failed"
	StateSkipped   = "skipped"
	StateCancelled = "cancelled"
)

// JobOutcome is the final record of one planned job.
type JobOutcome struct {
	ID, Name, Check, Tool               string
	Layer                               int
	Kind                                model.JobKind
	Required, AllowFailure, Substantive bool
	State                               string
	Reason                              model.Reason
	Detail                              string
	Code                                string
	FailureClass                        model.FailureClass
	ExitCode                            int
	AttemptID                           string
	N                                   int
	Duration                            time.Duration
	Tail                                string
}

// Event is one thing that happened during a run.
type Event struct {
	Type  string // queued | planned | started | log | finished
	RunID string // set by the coordinator on every event
	Job   string
	Text  string
	Out   *JobOutcome
	Plan  *plan.Plan
}

// Summary is the end of a run.
type Summary struct {
	RunID    string
	Result   model.Result
	Jobs     []JobOutcome
	Skipped  []plan.Skipped
	Status   string         // the status line, already formatted
	Evidence string         // evidence digest
	Doc      model.Evidence // the run evidence document, for renderers that print it
	Notes    []string       // recovery, hook and storage problems; never change the verdict
}

// Renderer turns Events and the Summary into output.
type Renderer interface {
	Event(Event)
	Done(Summary)
}

var (
	rendererMu sync.Mutex
	renderers  = map[string]func(io.Writer, Options) Renderer{}
)

// RegisterRenderer adds a renderer by name (P7-CI-38 adds "json").
func RegisterRenderer(name string, f func(io.Writer, Options) Renderer) {
	rendererMu.Lock()
	defer rendererMu.Unlock()
	renderers[name] = f
}

// stampRun fills RunID on every event a renderer receives.
type stamped struct {
	Renderer
	runID string
}

func (s stamped) Event(e Event) { e.RunID = s.runID; s.Renderer.Event(e) }

func stampRun(r Renderer, runID string) Renderer { return stamped{r, runID} }

// NewRenderer builds the renderer o selects: JSON when --json and a JSON
// renderer is registered, otherwise human.
func NewRenderer(w io.Writer, o Options) Renderer {
	rendererMu.Lock()
	defer rendererMu.Unlock()
	name := o.Renderer
	if name == "" {
		name = "human"
		if o.JSON && renderers["json"] != nil {
			name = "json"
		}
	}
	if f := renderers[name]; f != nil {
		return f(w, o)
	}
	return renderers["human"](w, o)
}

func init() {
	RegisterRenderer("human", func(w io.Writer, o Options) Renderer { return &human{w: w, stream: o.Stream} })
}

type human struct {
	mu     sync.Mutex
	w      io.Writer
	stream bool
}

func (h *human) Event(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch e.Type {
	case "log":
		if h.stream {
			fmt.Fprintf(h.w, "[%s] %s\n", e.Job, strings.TrimRight(e.Text, "\r\n"))
		}
	case "finished":
		o := e.Out
		switch o.State {
		case StatePassed:
			fmt.Fprintf(h.w, "  pass     %s (%s)\n", o.ID, o.Duration.Round(time.Millisecond))
		case StateFailed:
			fmt.Fprintf(h.w, "  FAIL     %s (%s)\n", o.ID, failText(*o))
			if o.Tail != "" && !h.stream {
				for _, l := range strings.Split(strings.TrimRight(o.Tail, "\n"), "\n") {
					fmt.Fprintf(h.w, "           | %s\n", l)
				}
			}
		default:
			fmt.Fprintf(h.w, "  %-8s %s: %s\n", o.State, o.ID, reasonText(*o))
		}
	}
}

func failText(o JobOutcome) string {
	switch {
	case o.Code != "":
		return o.Code + ": " + o.Detail
	case o.FailureClass == "timeout":
		return "timeout"
	case o.ExitCode == 0 && o.Detail != "":
		return o.Detail
	default:
		return fmt.Sprintf("exit %d", o.ExitCode)
	}
}

func reasonText(o JobOutcome) string {
	r := string(o.Reason)
	if r == "" {
		r = "not run"
	}
	if o.Detail != "" {
		r += " (" + o.Detail + ")"
	}
	return r
}

func (h *human) Done(s Summary) {
	h.mu.Lock()
	defer h.mu.Unlock()
	skipped := append([]plan.Skipped(nil), s.Skipped...)
	sort.Slice(skipped, func(i, j int) bool { return skipped[i].Job < skipped[j].Job })
	for _, k := range skipped {
		fmt.Fprintf(h.w, "  skipped  %s: %s\n", k.Job, k.Reason)
	}
	pass, total := 0, 0
	for _, j := range s.Jobs {
		total++
		if j.State == StatePassed {
			pass++
		}
	}
	fmt.Fprintf(h.w, "nself-ci: %s (%d/%d jobs passed) run %s\n", s.Result, pass, total, s.RunID)
	for _, n := range s.Notes {
		fmt.Fprintln(h.w, "nself-ci: note: "+n)
	}
	if s.Status != "" {
		fmt.Fprintln(h.w, s.Status)
	}
}
