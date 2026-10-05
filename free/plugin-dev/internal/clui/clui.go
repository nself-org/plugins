// Package clui prints the core `nself` console messages the author commands
// use, so the ported output matches core byte for byte.
//
// Purpose: temporary copy of the parts of cli internal/ui that the plugin
// author commands use (Success, Error, Warn, Info, Dimmed, Table). Ports must
// not import cli internals (CANON D11); this is deleted with the core code at
// P7-SHIP-09.
// Inputs:  message strings.
// Outputs: stdout (Success, Info, Dimmed, Table) and stderr (Warn, Error).
// Constraints: colour only when NO_COLOR is unset and stdout is a terminal,
// decided by an ioctl exactly like core (golang.org/x/term); standard library
// only.
package clui

import (
	"fmt"
	"os"
	"strings"
)

const (
	reset  = "\033[0m"
	dim    = "\033[2m"
	red    = "\033[0;31m"
	green  = "\033[0;32m"
	yellow = "\033[0;33m"
	blue   = "\033[0;34m"

	iconSuccess = "✓"
	iconFailure = "✗"
	iconWarning = "⚠"
	iconInfo    = "ℹ"
)

// colorsEnabled mirrors core: NO_COLOR unset and stdout is a terminal. A
// variable so tests can drive both branches.
var colorsEnabled = os.Getenv("NO_COLOR") == "" && isTerminalFd(os.Stdout.Fd())

// c wraps text in the colour code when colours are enabled.
func c(color, text string) string {
	if !colorsEnabled {
		return text
	}
	return color + text + reset
}

// Success prints a green check and message to stdout.
func Success(msg string) { fmt.Printf("%s %s\n", c(green, iconSuccess), msg) }

// Error prints a red cross and message to stderr.
func Error(msg string) { fmt.Fprintf(os.Stderr, "%s %s\n", c(red, iconFailure), msg) }

// Warn prints a yellow warning icon and message to stderr.
func Warn(msg string) { fmt.Fprintf(os.Stderr, "%s %s\n", c(yellow, iconWarning), msg) }

// Info prints a blue info icon and message to stdout.
func Info(msg string) { fmt.Printf("%s %s\n", c(blue, iconInfo), msg) }

// Dimmed prints dim text to stdout.
func Dimmed(msg string) { fmt.Printf("%s\n", c(dim, msg)) }

// Infof, Successf, Dimmedf and Warnf format and print.
func Infof(format string, a ...any)    { Info(fmt.Sprintf(format, a...)) }
func Successf(format string, a ...any) { Success(fmt.Sprintf(format, a...)) }
func Dimmedf(format string, a ...any)  { Dimmed(fmt.Sprintf(format, a...)) }
func Warnf(format string, a ...any)    { Warn(fmt.Sprintf(format, a...)) }

// Table renders a box-drawing table to stdout (core ui.Table).
type Table struct {
	headers []string
	rows    [][]string
}

// NewTable creates a table with the given column headers.
func NewTable(headers ...string) *Table { return &Table{headers: headers} }

// AddRow appends a row.
func (t *Table) AddRow(values ...string) { t.rows = append(t.rows, values) }

// Render prints the table.
func (t *Table) Render() {
	w := make([]int, len(t.headers))
	for i, h := range t.headers {
		w[i] = len(h)
	}
	for _, r := range t.rows {
		for i := 0; i < len(w) && i < len(r); i++ {
			if len(r[i]) > w[i] {
				w[i] = len(r[i])
			}
		}
	}
	for i := range w {
		w[i] += 2
	}
	border := func(l, m, r string) {
		var b strings.Builder
		b.WriteString(l)
		for i, n := range w {
			b.WriteString(strings.Repeat("─", n))
			if i < len(w)-1 {
				b.WriteString(m)
			}
		}
		b.WriteString(r)
		fmt.Println(b.String())
	}
	row := func(v []string) {
		var b strings.Builder
		b.WriteString("│")
		for i, n := range w {
			val := ""
			if i < len(v) {
				val = v[i]
			}
			b.WriteString(" ")
			fmt.Fprintf(&b, "%-*s", n-2, val)
			b.WriteString(" │")
		}
		fmt.Println(b.String())
	}
	border("┌", "┬", "┐")
	row(t.headers)
	border("├", "┼", "┤")
	for _, r := range t.rows {
		row(r)
	}
	border("└", "┴", "┘")
}
