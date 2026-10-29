package netscope

// Purpose: stage a sidecar-readable copy without widening the agent binary's permissions.
// Inputs: a regular executable file. Outputs: a private staged path and idempotent cleanup.
// Constraints: copy before publishing the path; cleanup on errors; no source mode changes.

import (
	"io"
	"os"
	"path/filepath"
)

func stageProxyBinary(source string) (string, func(), error) {
	in, err := os.Open(source)
	if err != nil {
		return "", nil, scopeFailed("cannot read the proxy binary")
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return "", nil, scopeFailed("the proxy binary is not a regular file")
	}
	dir, err := os.MkdirTemp("", "nself-ci-proxy-")
	if err != nil {
		return "", nil, err
	}
	remove := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, "nself-ci")
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		remove()
		return "", nil, err
	}
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Chmod(0755)
	}
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		remove()
		return "", nil, err
	}
	return path, remove, nil
}
