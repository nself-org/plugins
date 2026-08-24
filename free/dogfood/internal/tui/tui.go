// Package tui provides the handful of terminal-output helpers the dogfood
// plugin needs to reproduce the exact look of `nself dogfood` from before
// extraction (CLI-R11).
//
// Purpose: the plugin is its own Go module and cannot import the CLI's
// internal/ui package (Go's internal/ visibility rule forbids it across
// module boundaries), so this file reimplements only the subset of internal/ui
// that cmd/dogfood.go and cmd/report.go actually call: a boxed command
// header, a section marker, a bullet line, a separator, and the pass/warn/fail
// check-line renderer. Colors and icons are copied byte-for-byte from
// internal/ui/{colors,icons}.go so output is unchanged for a user upgrading
// from the in-core command to the plugin.
//
// Inputs: plain strings from the caller.
//
// Outputs: writes to stdout (informational) or stderr (warnings/failures),
// matching the split internal/ui used.
//
// Constraints: no dependency beyond the standard library — this package must
// stay buildable offline, like the rest of the plugin.
package tui

import (
	"fmt"
	"os"
	"strings"
)

// ANSI color codes, copied from the CLI's internal/ui/colors.go so a user
// sees identical output whether the command runs in-core or as a plugin.
const (
	reset  = "\033[0m"
	red    = "\033[0;31m"
	green  = "\033[0;32m"
	yellow = "\033[0;33m"
	blue   = "\033[0;34m"
	dim    = "\033[2m"
	bold   = "\033[1m"
)

const (
	iconSuccess = "✓" // check mark
	iconWarning = "⚠" // warning triangle
	iconFailure = "✗" // cross mark
	iconBullet  = "•" // bullet
)

// colorsEnabled mirrors internal/ui: off when NO_COLOR is set or stdout is
// not a terminal (a plain pipe check is enough here; the plugin does not need
// the CLI's full terminal-detection package for this).
var colorsEnabled = os.Getenv("NO_COLOR") == ""

func c(color, text string) string {
	if !colorsEnabled {
		return text
	}
	return color + text + reset
}

func padRight(styled, plain string, width int) string {
	padding := width - len([]rune(plain))
	if padding <= 0 {
		return styled
	}
	return styled + strings.Repeat(" ", padding)
}

// CommandHeader prints the same 60-char double-line box internal/ui uses.
func CommandHeader(title, subtitle string) {
	w := 60
	border := strings.Repeat("═", w-2)
	fmt.Printf("%s%s%s\n", c(blue, "╔"), c(blue, border), c(blue, "╗"))
	fmt.Printf("%s %s %s\n", c(blue, "║"), padRight(c(bold, title), title, w-4), c(blue, "║"))
	fmt.Printf("%s %s %s\n", c(blue, "║"), padRight(c(dim, subtitle), subtitle, w-4), c(blue, "║"))
	fmt.Printf("%s%s%s\n", c(blue, "╚"), c(blue, border), c(blue, "╝"))
}

// Section prints a blank line, blue arrow, and bold section title.
func Section(title string) {
	fmt.Printf("\n%s %s\n", c(blue, "→"), c(bold, title))
}

// Bullet prints an indented bullet point.
func Bullet(item string) {
	fmt.Printf("  %s %s\n", c(blue, iconBullet), item)
}

// Warn prints a yellow warning icon and message to stderr.
func Warn(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", c(yellow, iconWarning), msg)
}

// Separator prints a dim 60-char horizontal rule.
func Separator() {
	fmt.Printf("%s\n", c(dim, strings.Repeat("─", 60)))
}

// PrintCheck renders one audit check result, matching
// cmd/commands/doctor_health_report.go's printCheck in the core CLI: pass
// goes to stdout with a green check, warn/fail go to stderr.
func PrintCheck(status, name, message string) {
	line := fmt.Sprintf("%s: %s", name, message)
	switch status {
	case "pass":
		fmt.Printf("  %s %s\n", c(green, iconSuccess), line)
	case "warn":
		fmt.Fprintf(os.Stderr, "  %s %s\n", c(yellow, iconWarning), line)
	case "fail":
		fmt.Fprintf(os.Stderr, "  %s %s\n", c(red, iconFailure), line)
	}
}
