//go:build unix

package legacy

import (
	"os/exec"
	"syscall"
)

// killProcessGroupOnCancel starts c in its own process group and makes a
// context cancel kill the whole group, so a timed-out repo gate script does
// not leave its children (gremlins, go test) running.
func killProcessGroupOnCancel(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		return syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
}
