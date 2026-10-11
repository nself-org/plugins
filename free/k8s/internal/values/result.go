// Purpose: the structured outcome of one `values` run, its failure kinds, and
// its human rendering.
//
// Inputs: filled by Run.
//
// Outputs: Result (JSON-tagged: the data member of the --json envelope),
// Error (a failure with a Kind), WriteText (the human lines, byte-identical to
// the output before the --json work).
//
// Constraints: no secret values. Error.Error() is the inner text unchanged.
package values

import (
	"fmt"
	"io"
)

// Modes of a Result.
const (
	ModeGenerate = "generate"
	ModeCheck    = "check"
)

// Failure kinds of an Error.
const (
	KindCompose = "compose" // docker compose config could not be resolved
	KindMissing = "missing" // --check without an existing values.yaml
	KindInvalid = "invalid" // the existing values.yaml cannot be read as values
	KindParity  = "parity"  // --check found differences
	KindOther   = "other"   // routes, mapping or write failure
)

// Error is a failed values run.
type Error struct {
	Kind string
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// Item is one named entry with a reason (an unsupported service, an unmapped route).
type Item struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// Result is what a run did.
type Result struct {
	Mode           string        `json:"mode"`
	ValuesFile     string        `json:"values_file"`
	SecretsFile    string        `json:"secrets_file,omitempty"`
	Mapped         int           `json:"mapped"`
	Unsupported    []Unsupported `json:"-"`
	UnmappedRoutes []Unsupported `json:"-"`
	Services       int           `json:"services"`
	Diffs          []Diff        `json:"-"`
}

// Items converts an Unsupported list to Items (never nil, so JSON has []).
func Items(list []Unsupported) []Item {
	out := make([]Item, 0, len(list))
	for _, u := range list {
		out = append(out, Item{Name: u.Name, Reason: u.Reason})
	}
	return out
}

// WriteText prints the human report: the files written and what could not be
// mapped, or the parity differences and the parity-ok line.
func (r *Result) WriteText(w io.Writer) {
	if r.Mode == ModeCheck {
		for _, d := range r.Diffs {
			fmt.Fprintln(w, d)
		}
		if len(r.Diffs) == 0 {
			fmt.Fprintf(w, "parity ok: %d compose service(s) match %s\n", r.Services, r.ValuesFile)
		}
		return
	}
	fmt.Fprintf(w, "wrote %s and %s (%d mapped, %d unsupported)\n", r.ValuesFile, r.SecretsFile, r.Mapped, len(r.Unsupported))
	for _, u := range r.Unsupported {
		fmt.Fprintf(w, "unsupported: %s: %s\n", u.Name, u.Reason)
	}
	for _, u := range r.UnmappedRoutes {
		fmt.Fprintf(w, "unmapped route: %s: %s\n", u.Name, u.Reason)
	}
}
