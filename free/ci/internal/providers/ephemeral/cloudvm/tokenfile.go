package cloudvm

// Purpose: read the provider token from burst.cloudvm.token_file at call time (CI D26, review F8).
// Inputs:  the file path from the spend policy. Outputs: the token text.
// Constraints: the file must be a regular file, mode 0600 or stricter, owned by the coordinator's
// user, at most 4096 bytes; it is opened without following a final symlink and checked on the open
// descriptor, so there is no check-then-open gap; the token is one run of printable ASCII (a
// trailing newline is dropped); the value is never cached, never logged and never part of an error.

import (
	"io"
	"strings"
)

const maxTokenBytes = 4096

// tokenText trims one trailing line ending and checks the token is one run of printable ASCII.
func tokenText(b []byte) (string, bool) {
	s := strings.TrimRight(string(b), "\r\n")
	if s == "" {
		return "", false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x21 || s[i] > 0x7e {
			return "", false
		}
	}
	return s, true
}

func readCapped(r io.Reader) ([]byte, bool) {
	b, err := io.ReadAll(io.LimitReader(r, maxTokenBytes+1))
	return b, err == nil && len(b) <= maxTokenBytes
}
