package ui

// Purpose: the output helpers `nself infra server` needs beyond Info and Warn:
// the command header box, the success line, indented JSON and the box table.
// Inputs: strings, a value to marshal, table headers and rows.
// Outputs: the same bytes core's internal/ui produced for `nself server`
// (P7-CANON-12 parity), Success/header/table/JSON on stdout.
// Constraints: a deliberate copy of core's header.go, json.go, table.go and the
// Success line (a plugin must not import cli/internal/*); colour follows the
// same rule as ui.go (off when NO_COLOR is set or stdout is not a terminal).

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const (
	green    = "\033[0;32m"
	bold     = "\033[1m"
	dim      = "\033[2m"
	iconDone = "✓"
)

// Success prints a green checkmark and message to stdout.
func Success(msg string) {
	fmt.Printf("%s %s\n", c(green, iconDone), msg)
}

// CommandHeader prints a 60-char wide double-line box with title and subtitle.
func CommandHeader(title, subtitle string) {
	w := 60
	border := strings.Repeat("═", w-2)

	fmt.Printf("%s%s%s\n", c(blue, "╔"), c(blue, border), c(blue, "╗"))
	fmt.Printf("%s %s %s\n", c(blue, "║"), padRight(c(bold, title), title, w-4), c(blue, "║"))
	fmt.Printf("%s %s %s\n", c(blue, "║"), padRight(c(dim, subtitle), subtitle, w-4), c(blue, "║"))
	fmt.Printf("%s%s%s\n", c(blue, "╚"), c(blue, border), c(blue, "╝"))
}

// padRight pads a styled string to width based on the plain text length.
func padRight(styled, plain string, width int) string {
	padding := width - len([]rune(plain))
	if padding <= 0 {
		return styled
	}
	return styled + strings.Repeat(" ", padding)
}

// PrintJSON marshals v to indented JSON (2-space indent) and writes it to stdout.
func PrintJSON(v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}
	_, err = fmt.Fprintln(os.Stdout, string(data))
	return err
}

// Table renders data as a box-drawing table to stdout.
type Table struct {
	Headers []string
	Rows    [][]string
	Widths  []int
}

// NewTable creates a new table with the given column headers.
func NewTable(headers ...string) *Table { return &Table{Headers: headers} }

// AddRow appends a row of values to the table.
func (t *Table) AddRow(values ...string) { t.Rows = append(t.Rows, values) }

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

// calcWidths computes column widths as max(header, longest value) + 2 padding.
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
		fmt.Fprintf(&b, "%-*s", w-2, val)
		b.WriteString(" │")
	}
	fmt.Println(b.String())
}
