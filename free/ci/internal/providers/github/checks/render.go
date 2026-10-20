package checks

// Purpose: render the gate table (the check run's output text) and the failing-check annotations.
// Inputs:  the published evidence and projection of a run.
// Outputs: deterministic Markdown (golden-tested) and a bounded annotation list.
// Constraints: cell text is made safe for a Markdown table (pipes, newlines, control characters,
// backticks); the text stays under MaxText bytes; annotations come only from a failing check's
// excerpt lines of the form `path:line[:col]: message` with a relative path, so a location is
// never invented. The evidence is the PUBLISHED document (publication profile applied), so an
// excerpt the profile removed yields no annotation.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Limits.
const (
	MaxText        = 60000 // GitHub allows 65535 bytes of output text
	MaxAnnotations = 200   // across all pages
	PageSize       = 50    // GitHub's limit per request
	maxCell        = 160
	maxMessage     = 500
)

// Annotation is one file-level note on a failing check.
type Annotation struct {
	Path    string `json:"path"`
	Start   int    `json:"start_line"`
	End     int    `json:"end_line"`
	Level   string `json:"annotation_level"`
	Message string `json:"message"`
	Title   string `json:"title"`
}

func cell(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '|':
			b.WriteString(`\|`)
		case r == '`':
			b.WriteByte('\'')
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteByte(' ')
		case unicode.IsControl(r):
		default:
			b.WriteRune(r)
		}
	}
	return truncRunes(strings.TrimSpace(b.String()), maxCell)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// Table renders the gate table for a run.
func Table(ev model.Evidence, proj model.ProjectionResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**%s**\n\n", cell(proj.Summary))
	b.WriteString("| Check | Result | Required | Class | Findings | Next step |\n|---|---|---|---|---|---|\n")
	notShown := 0
	for i, c := range ev.Checks {
		class := string(c.FailureClass)
		step := c.Remediation
		if step == "" {
			step = c.Diagnosis
		}
		if c.Flaky != nil {
			step = strings.TrimSpace(step + " (flaky: " + strconv.Itoa(c.Flaky.Passes) + " pass, " + strconv.Itoa(c.Flaky.Fails) + " fail on identical inputs)")
		}
		row := fmt.Sprintf("| `%s` | %s | %s | %s | %d | %s |\n", cell(c.ID), cell(string(c.Result)), yesNo(c.Required), cell(class), c.Findings, cell(step))
		if b.Len()+len(row) > MaxText-200 {
			notShown = len(ev.Checks) - i
			break
		}
		b.WriteString(row)
	}
	if notShown > 0 {
		fmt.Fprintf(&b, "\n%d more checks are not shown; see the run evidence.\n", notShown)
	}
	return b.String()
}

// Title is the check run's title line.
func Title(proj model.ProjectionResult) string { return truncRunes(cell(proj.Summary), 120) }

var location = regexp.MustCompile(`^\s*(?:\./)?([A-Za-z0-9_@+][A-Za-z0-9_./@+-]*):([0-9]{1,7})(?::[0-9]{1,5})?:\s*(\S.*)$`)

func safePath(p string) bool {
	return len(p) <= 256 && (strings.Contains(p, "/") || strings.Contains(p, ".")) &&
		!strings.HasPrefix(p, "/") && !strings.Contains(p, "..") && !strings.Contains(p, "//") && !strings.HasSuffix(p, "/")
}

// Annotations extracts the located failures of the failing checks, at most MaxAnnotations.
func Annotations(ev model.Evidence) []Annotation {
	var out []Annotation
	seen := map[string]bool{}
	for _, c := range ev.Checks {
		level := "failure"
		switch c.Result {
		case "fail":
		case "error":
			level = "warning"
		default:
			continue
		}
		for _, line := range strings.Split(c.Excerpt, "\n") {
			m := location.FindStringSubmatch(line)
			if m == nil || !safePath(m[1]) {
				continue
			}
			n, err := strconv.Atoi(m[2])
			if err != nil || n < 1 {
				continue
			}
			msg := truncRunes(strings.TrimSpace(strings.Map(func(r rune) rune {
				if unicode.IsControl(r) && r != '\t' {
					return -1
				}
				return r
			}, m[3])), maxMessage)
			key := c.ID + "\x00" + m[1] + "\x00" + m[2] + "\x00" + msg
			if seen[key] || !utf8.ValidString(msg) {
				continue
			}
			seen[key] = true
			out = append(out, Annotation{Path: m[1], Start: n, End: n, Level: level, Message: msg, Title: truncRunes(c.ID, 255)})
			if len(out) == MaxAnnotations {
				return out
			}
		}
	}
	return out
}

// Pages splits annotations into requests of at most PageSize.
func Pages(a []Annotation) [][]Annotation {
	var out [][]Annotation
	for len(a) > 0 {
		n := min(PageSize, len(a))
		out = append(out, a[:n])
		a = a[n:]
	}
	return out
}
