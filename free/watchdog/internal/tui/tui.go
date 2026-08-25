// Package tui provides the handful of terminal-output helpers the watchdog
// plugin needs to reproduce the exact look of `nself watchdog` from before
// extraction (CLI-R11).
//
// Purpose: the plugin is its own Go module and cannot import the CLI's
// internal/ui package (Go's internal/ visibility rule forbids it across
// module boundaries), so this file reimplements only the subset of internal/ui
// that cmd/watchdog.go actually calls: a boxed command header, success/warn
// lines, a bullet, a section header, and the color/icon primitives used
// directly by the status command's circuit-breaker rendering. Colors and
// icons are copied byte-for-byte from internal/ui/{colors,icons}.go so
// output is unchanged for a user upgrading from the in-core command to the
// plugin.
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
	Reset  = "\033[0m"
	Red    = "\033[0;31m"
	Green  = "\033[0;32m"
	Blue   = "\033[0;34m"
	yellow = "\033[0;33m"
	dim    = "\033[2m"
	bold   = "\033[1m"
)

// Icons, copied from internal/ui/icons.go.
const (
	IconSuccess = "✓" // ✓
	IconFailure = "✗" // ✗
	iconWarning = "⚠" // ⚠
	iconArrow   = "→" // →
	iconBullet  = "•" // •
)

// colorsEnabled mirrors internal/ui: off when NO_COLOR is set (a plain env
// check is enough here; the plugin does not need the CLI's full
// terminal-detection package for this).
var colorsEnabled = os.Getenv("NO_COLOR") == ""

// C wraps text in the given ANSI color code if colors are enabled. Exported
// (unlike the other helpers) because cmd/watchdog.go calls C(Red, IconFailure)
// / C(Green, IconSuccess) directly, matching the core command's use of
// ui.C(ui.Red, ui.IconFailure).
func C(color, text string) string {
	if !colorsEnabled {
		return text
	}
	return color + text + Reset
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
	fmt.Printf("%s%s%s\n", C(Blue, "╔"), C(Blue, border), C(Blue, "╗"))
	fmt.Printf("%s %s %s\n", C(Blue, "║"), padRight(C(bold, title), title, w-4), C(Blue, "║"))
	fmt.Printf("%s %s %s\n", C(Blue, "║"), padRight(C(dim, subtitle), subtitle, w-4), C(Blue, "║"))
	fmt.Printf("%s%s%s\n", C(Blue, "╚"), C(Blue, border), C(Blue, "╝"))
}

// Section prints a blank line, blue arrow, and bold section title.
func Section(title string) {
	fmt.Printf("\n%s %s\n", C(Blue, iconArrow), C(bold, title))
}

// Bullet prints an indented bullet point.
func Bullet(item string) {
	fmt.Printf("  %s %s\n", C(Blue, iconBullet), item)
}

// Success prints a green checkmark and message to stdout.
func Success(msg string) {
	fmt.Printf("%s %s\n", C(Green, IconSuccess), msg)
}

// Warn prints a yellow warning icon and message to stderr.
func Warn(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", C(yellow, iconWarning), msg)
}
