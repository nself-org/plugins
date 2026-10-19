//go:build windows

package hygiene

// RemoveConfined is not supported on Windows (Epic D35, gate G3): there is no openat/unlinkat.
func RemoveConfined(_, _ string, _ func(string, ...any)) error {
	return &Error{Code: CodeLeak, Reason: ReasonOSUnsupp, Message: "confined removal is not supported on this OS"}
}
