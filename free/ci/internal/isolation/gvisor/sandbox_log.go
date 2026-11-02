package gvisor

// Purpose: tell a sandbox killed by its process limit from a job that exited with the same code.
// Inputs:  the --debug-log value of the runsc runtime entry, the attempt's workspace path, the attempt start.
// Outputs: the runsc log line that shows the death, or "".
// Constraints: under gVisor the container's pids limit is a host cgroup limit on the sentry, so a fork bomb
// ends with the sentry failing to create a stub process: it exits 2 and the job prints nothing. Only runsc's
// own boot log says so (seen on the GitHub ubuntu runner, gVisor 20261012.0):
//   handlePtraceSyscallRequest failed: unable to create a stub process: creating stub process: resource temporarily unavailable
//   panic: Unknown syscall 57 error: createStub: failed to get clone
//   panic: Unknown syscall 57 error: createSysmsgThread: failed to get clone
// A boot log is the attempt's when it mounted the attempt's workspace. A script that merely runs `exit 2`
// leaves no such line, so it stays a code failure. Without a configured debug log nothing is claimed.

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxBootLog bounds the size of a log that is read.
const maxBootLog = 256 << 20

// deathSignatures are the runsc log fragments that show the stub-process (thread) limit was hit.
var deathSignatures = []string{
	"unable to create a stub process: creating stub process: resource temporarily unavailable",
	"failed to get clone", // createStub / createSysmsgThread: the clone syscall (57) was refused
}

// debugLogOf returns the --debug-log value of the runtime arguments ("" when none).
func debugLogOf(args []string) string {
	for i := 0; i < len(args); i++ {
		name, value, has := splitFlag(args[i])
		if name != "debug-log" {
			continue
		}
		if !has && i+1 < len(args) {
			value = args[i+1]
		}
		return value
	}
	return ""
}

// sandboxDeath returns the boot-log line showing that the sandbox that mounted workspace died from the
// process limit, or "" (no log configured, no log of this attempt, or no such line in it).
func sandboxDeath(logPath, workspace string, began time.Time) string {
	if logPath == "" || workspace == "" {
		return ""
	}
	dir, prefix := logPath, ""
	if !strings.HasSuffix(logPath, "/") {
		dir, prefix = filepath.Dir(logPath), filepath.Base(logPath)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, prefix) || !strings.Contains(name, ".boot.") {
			continue
		}
		info, err := e.Info()
		if err != nil || info.Size() > maxBootLog || info.ModTime().Before(began.Add(-time.Second)) {
			continue
		}
		if line := deathLine(filepath.Join(dir, name), workspace); line != "" {
			return line + " (" + name + ")"
		}
	}
	return ""
}

// deathLine scans one boot log: the first signature line, only if the log is of workspace's sandbox.
func deathLine(file, workspace string) string {
	f, err := os.Open(file)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	mine, line := false, ""
	for sc.Scan() {
		text := sc.Text()
		if !mine && strings.Contains(text, workspace) {
			mine = true
		}
		if line == "" {
			for _, sig := range deathSignatures {
				if strings.Contains(text, sig) {
					line = strings.TrimSpace(text)
					if len(line) > 240 {
						line = line[:240]
					}
					break
				}
			}
		}
	}
	if !mine {
		return ""
	}
	return line
}

// bootLogNote says what the runsc log directory held for an attempt that showed no death: the readings that
// explain a miss ("none configured", how many boot logs were fresh, how many of them mounted the workspace).
func bootLogNote(logPath, workspace string, began time.Time) string {
	if logPath == "" {
		return "runsc log: no --debug-log configured"
	}
	dir, prefix := logPath, ""
	if !strings.HasSuffix(logPath, "/") {
		dir, prefix = filepath.Dir(logPath), filepath.Base(logPath)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "runsc log: " + dir + " unreadable (" + err.Error() + ")"
	}
	fresh, mine := 0, 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, prefix) || !strings.Contains(name, ".boot.") {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().Before(began.Add(-time.Second)) {
			continue
		}
		fresh++
		if bootLogMounts(filepath.Join(dir, name), workspace) {
			mine++
		}
	}
	return fmt.Sprintf("runsc log: %s has %d fresh boot logs, %d of this attempt's workspace, no death line", dir, fresh, mine)
}

// bootLogMounts reports whether the boot log mentions workspace.
func bootLogMounts(file, workspace string) bool {
	f, err := os.Open(file)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		if strings.Contains(sc.Text(), workspace) {
			return true
		}
	}
	return false
}
