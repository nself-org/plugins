// Package tui provides the handful of terminal-output helpers the mail
// plugin needs to reproduce the exact look of `nself mail` from before
// extraction (CLI-R11).
//
// Purpose: the plugin is its own Go module and cannot import the CLI's
// internal/ui package (Go's internal/ visibility rule forbids it across
// module boundaries), so this file reimplements only the subset of
// internal/ui that cmd/mail_*.go actually calls: the success/warn message
// lines and the box-drawing table renderer. Colors, icons, and the table
// layout algorithm are copied byte-for-byte from internal/ui/{colors,
// icons,messages,table}.go so output is unchanged for a user upgrading from
// the in-core command to the plugin.
//
// Inputs: plain strings (Success/Warn) or column headers/rows (Table).
//
// Outputs: writes to stdout (success, table) or stderr (warnings).
//
// Constraints: no dependency beyond the standard library.
package tui

import (
	"fmt"
	"os"
	"strings"
)

const (
	reset  = "\033[0m"
	green  = "\033[0;32m"
	yellow = "\033[0;33m"
)

const (
	iconSuccess = "✓" // check mark
	iconWarning = "⚠" // warning triangle
)

var colorsEnabled = os.Getenv("NO_COLOR") == ""

func c(color, text string) string {
	if !colorsEnabled {
		return text
	}
	return color + text + reset
}

// Success prints a green checkmark and message to stdout.
func Success(msg string) {
	fmt.Printf("%s %s\n", c(green, iconSuccess), msg)
}

// Warn prints a yellow warning icon and message to stderr.
func Warn(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", c(yellow, iconWarning), msg)
}

// Table renders data as a box-drawing table to stdout. Copied byte-for-byte
// from internal/ui/table.go.
type Table struct {
	Headers []string
	Rows    [][]string
	Widths  []int
}

// NewTable creates a new table with the given column headers.
func NewTable(headers ...string) *Table {
	return &Table{Headers: headers}
}

// AddRow appends a row of values to the table.
func (t *Table) AddRow(values ...string) {
	t.Rows = append(t.Rows, values)
}

// Render outputs the table with box-drawing characters to stdout.
func (t *Table) Render() {
	t.calcWidths()
	t.printBorder("┌", "┬", "┐")
	t.printRow(t.Headers)
	t.printBorder("├", "┼", "┤")
	for _, row := range t.Rows {
		t.printRow(row)
	}
	t.printBorder("└", "┴", "┘")
}

func (t *Table) calcWidths() {
	cols := len(t.Headers)
	t.Widths = make([]int, cols)
	for i, h := range t.Headers {
		if len(h) > t.Widths[i] {
			t.Widths[i] = len(h)
		}
	}
	for _, row := range t.Rows {
		for i := 0; i < cols && i < len(row); i++ {
			if len(row[i]) > t.Widths[i] {
				t.Widths[i] = len(row[i])
			}
		}
	}
	for i := range t.Widths {
		t.Widths[i] += 2
	}
}

func (t *Table) printBorder(left, mid, right string) {
	var b strings.Builder
	b.WriteString(left)
	for i, w := range t.Widths {
		b.WriteString(strings.Repeat("─", w))
		if i < len(t.Widths)-1 {
			b.WriteString(mid)
		}
	}
	b.WriteString(right)
	fmt.Println(b.String())
}

func (t *Table) printRow(values []string) {
	var b strings.Builder
	b.WriteString("│")
	for i, w := range t.Widths {
		val := ""
		if i < len(values) {
			val = values[i]
		}
		b.WriteString(" ")
		b.WriteString(fmt.Sprintf("%-*s", w-2, val))
		b.WriteString(" │")
	}
	fmt.Println(b.String())
}
