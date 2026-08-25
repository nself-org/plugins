// Package ui provides the handful of styled-print helpers this plugin's
// `claw migrate` command uses.
//
// Constraints: cli/internal/ui is 1521 lines across the whole core CLI's
// output surface (progress bars, spinners, tables, ...) and is unreachable
// from this plugin module. Only Section/Info/Success/Dimmed/C are used here
// — copied narrowly rather than ported wholesale, matching the precedent
// set by the k8s and dogfood plugins' own internal/tui packages.
package ui

import "fmt"

// ANSI color codes (subset of cli/internal/ui/colors.go).
const (
	Green = "\033[0;32m"
	reset = "\033[0m"
	dim   = "\033[2m"
)

// IconSuccess mirrors cli/internal/ui/icons.go.
const IconSuccess = "✓" // checkmark

// C wraps text in an ANSI color code and reset.
func C(color, text string) string {
	return color + text + reset
}

// Section prints a section header.
func Section(title string) {
	fmt.Printf("\n%s\n", title)
}

// Info prints an informational line.
func Info(msg string) {
	fmt.Println(msg)
}

// Success prints a checkmark-prefixed success line.
func Success(msg string) {
	fmt.Printf("%s %s\n", C(Green, IconSuccess), msg)
}

// Dimmed prints a de-emphasized line.
func Dimmed(msg string) {
	fmt.Println(C(dim, msg))
}
