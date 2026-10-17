package plan

// Purpose: map the config loader's dependency diagnostics to E616 at the
// plan command boundary. The loader validates before the planner runs and
// reports a cycle or an undefined dependency as E620; Epic D14 names those
// two defects E616, with the jobs in the message.
// Inputs:  located problems as plain values (file, line, JSON pointer,
// message), passed by the caller so this package never imports config.
// Outputs: an *Error with code E616 that names the jobs and keeps every
// location, plus any other problems unchanged after them; nil when no
// problem is a dependency defect, so every other config error stays E620.
// Constraints: pure; no re-reading of the config file. The loader does not
// report the undefined target, so the message names the dependent job and
// the dependency index from the pointer.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Diagnostic is one located config problem.
type Diagnostic struct {
	File    string
	Line    int
	Pointer string
	Message string
}

func (d Diagnostic) String() string {
	out := d.File
	if d.Line > 0 {
		out += ":" + strconv.Itoa(d.Line)
	}
	if d.Pointer != "" {
		out += ": " + d.Pointer
	}
	return out + ": " + d.Message
}

const (
	cyclePrefix   = "dependency cycle: "
	undefinedText = "depends on a job that is not defined"
)

var depPointer = regexp.MustCompile(`^/jobs/(.+)/deps/(\d+)$`)

// DAGError returns the E616 error for the dependency defects in ds, or nil.
func DAGError(ds []Diagnostic) *Error {
	var dag, other []string
	names := map[string]bool{}
	for _, d := range ds {
		switch {
		case strings.HasPrefix(d.Message, cyclePrefix):
			dag = append(dag, d.String())
			for _, j := range strings.Split(strings.TrimPrefix(d.Message, cyclePrefix), " -> ") {
				names[strings.TrimSpace(j)] = true
			}
		case strings.Contains(d.Message, undefinedText) && depPointer.MatchString(d.Pointer):
			m := depPointer.FindStringSubmatch(d.Pointer)
			job := strings.NewReplacer("~1", "/", "~0", "~").Replace(m[1])
			names[job] = true
			dag = append(dag, fmt.Sprintf("%s (dependency %s of job %s)", d.String(), m[2], job))
		default:
			other = append(other, d.String())
		}
	}
	if len(dag) == 0 {
		return nil
	}
	msg := strings.Join(dag, "\n")
	if len(other) > 0 {
		msg += "\nother config problems:\n" + strings.Join(other, "\n")
	}
	return fail(CodeCycle, msg, keys(names))
}
