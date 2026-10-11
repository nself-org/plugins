//go:build !unix

package legacy

import "os/exec"

// killProcessGroupOnCancel is a no-op off unix: the context cancel kills only
// the script itself, and WaitDelay still bounds the wait for its pipes.
func killProcessGroupOnCancel(c *exec.Cmd) {}
