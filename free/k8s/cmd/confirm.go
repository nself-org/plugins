// Purpose: the confirmation gate of `nself-k8s uninstall`.
//
// Inputs: stdin, --yes, --json.
//
// Outputs: whether the removal may run. --yes always allows it. Without --yes,
// an interactive terminal (and not --json) is asked to type the release name;
// anything else is refused.
//
// Constraints: "interactive" is stdin being a character device other than
// /dev/null (stdlib only, no terminal package). The check sits behind a
// variable so tests can drive both branches without a pty. The answer is read
// through a 4096-byte limit, so a stdin without a newline cannot grow memory.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// stdinIsTTY reports whether stdin is an interactive terminal. Tests replace it.
var stdinIsTTY = func() bool {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	// /dev/null is a character device too, and is not a person to ask.
	if null, err := os.Stat(os.DevNull); err == nil && os.SameFile(fi, null) {
		return false
	}
	return true
}

// maxConfirmBytes bounds the confirmation read. A release name is at most 53
// bytes; anything longer is not a match.
const maxConfirmBytes = 4096

// confirmRelease asks the user to type the release name and reports whether
// the answer matches it exactly.
func confirmRelease(in io.Reader, out io.Writer, release string) bool {
	fmt.Fprintf(out, "This removes the Helm release %q and its workloads. Its PersistentVolumeClaims are kept.\nType the release name to confirm: ", release)
	line, _ := bufio.NewReader(io.LimitReader(in, maxConfirmBytes)).ReadString('\n')
	return strings.TrimSpace(line) == release
}
