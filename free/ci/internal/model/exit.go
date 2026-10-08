package model

import "strings"

// ExitCode maps a command outcome to its public process exit status.
type ExitCode struct {
	Command string
	Outcome string
	Code    int
}

// ExitCodes is the v1 table used by generated reference documentation.
var ExitCodes = []ExitCode{
	{"ci", "pass", 0}, {"ci", "required check failed", 1}, {"ci", "usage error", 1},
	{"ci", "infrastructure", 2}, {"ci", "authorization", 3},
	{"ci evidence verify", "current bound pass", 0}, {"ci evidence verify", "evidence fails", 1},
	{"ci evidence verify", "stale", 10}, {"ci evidence verify", "invalid", 11}, {"ci evidence verify", "unbound", 12},
	{"ci evidence --pipeline", "none overdue", 0}, {"ci evidence --pipeline", "overdue", 10},
	{"ci doctor", "clean", 0}, {"ci doctor", "check failed", 10}, {"ci doctor", "warnings only", 12},
}

// ExitMarkdown renders the table for the reference generator.
func ExitMarkdown() string {
	var b strings.Builder
	b.WriteString("| Command | Outcome | Exit |\n|---|---|---:|\n")
	for _, row := range ExitCodes {
		b.WriteString("| " + row.Command + " | " + row.Outcome + " | " + countText(row.Code) + " |\n")
	}
	return b.String()
}
