//go:build unix

package cloudvm

// Purpose: the unix half of the token file check (see tokenfile.go).
// Constraints: the owner is compared with tokenFileUID, a seam for the test of a foreign owner.

import (
	"os"
	"syscall"
)

var tokenFileUID = os.Getuid

// readTokenFile returns the token in path, or an E698 token_file error that names the path and
// the rule that failed, never the content.
func readTokenFile(path string) (string, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return "", fail(ReasonTokenFile, nil, "%s cannot be opened", path)
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return "", fail(ReasonTokenFile, nil, "%s cannot be checked", path)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	switch {
	case !fi.Mode().IsRegular():
		return "", fail(ReasonTokenFile, nil, "%s is not a regular file", path)
	case fi.Mode().Perm()&^0o600 != 0 || fi.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0:
		return "", fail(ReasonTokenFile, nil, "%s must be mode 0600 or stricter", path)
	case !ok || int(st.Uid) != tokenFileUID():
		return "", fail(ReasonTokenFile, nil, "%s is not owned by the coordinator's user", path)
	case fi.Size() > maxTokenBytes:
		return "", fail(ReasonTokenFile, nil, "%s is larger than %d bytes", path, maxTokenBytes)
	}
	b, ok := readCapped(f)
	if !ok {
		return "", fail(ReasonTokenFile, nil, "%s cannot be read", path)
	}
	tok, ok := tokenText(b)
	if !ok {
		return "", fail(ReasonTokenFile, nil, "%s must hold one token of printable characters", path)
	}
	return tok, nil
}
