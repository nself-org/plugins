//go:build !linux

package hygiene

// mountsUnder has no mountinfo outside Linux; the device check in RemoveConfined still applies.
func mountsUnder(string) ([]string, bool) { return nil, true }
