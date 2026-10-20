package darwin

// Purpose: a fake macOS node for the darwin family tests: it records every argv, answers the few
// commands the family asks directly, and reads each copied file at Copy time (the family removes its
// local stage file when the step ends).
// Inputs:  scripted replies. Constraints: no network, no disk beyond the stage files, no real dscl.

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap"
)

type fakeMac struct {
	calls   [][]string
	copies  map[string]string // remote name -> file content at copy time
	uid     string
	os      string
	sudoErr bool
	users   string // reply of the list-users script
	failOn  string // a script fragment that fails
	kept    map[string]bool
	xcode   string // reply of the xcode probe
}

func newFake() *fakeMac {
	return &fakeMac{uid: "501", os: "Darwin", copies: map[string]string{}, xcode: "ok"}
}

func (n *fakeMac) run(_ context.Context, argv ...string) (string, error) {
	n.calls = append(n.calls, argv)
	joined := strings.Join(argv, " ")
	switch {
	case joined == "uname -s":
		return n.os, nil
	case joined == "id -u":
		return n.uid, nil
	case joined == "sudo -n true":
		if n.sudoErr {
			return "", errors.New("sudo: a password is required")
		}
		return "", nil
	}
	script := argv[len(argv)-1]
	if n.failOn != "" && strings.Contains(script, n.failOn) {
		return "boom\nsecond line", errors.New("exit status 1")
	}
	switch {
	case strings.Contains(script, "dscl . -list /Users | grep -E '^nself-ci-job-[0-9]+$'"):
		return n.users, nil
	case strings.Contains(script, "echo kept; exit 0"):
		for u := range n.kept {
			if strings.Contains(script, "u="+u+"\n") {
				return "kept", nil
			}
		}
		return "removed", nil
	case strings.Contains(script, "xcodebuild -license check"):
		return n.xcode, nil
	}
	return "", nil
}

func (n *fakeMac) copy(_ context.Context, local, remotePath string) error {
	b, err := os.ReadFile(local)
	if err != nil {
		return err
	}
	n.copies[remotePath] = string(b)
	return nil
}

// scripts are the `sh -c` scripts sent, in order.
func (n *fakeMac) scripts() []string {
	var out []string
	for _, c := range n.calls {
		if len(c) >= 3 && c[len(c)-3] == "sh" {
			out = append(out, c[len(c)-1])
		}
	}
	return out
}

func (n *fakeMac) options(t *testing.T, slots int) (bootstrap.Options, *[]string) {
	t.Helper()
	var logs []string
	return bootstrap.Options{Slots: slots, Run: n.run, Copy: n.copy,
		Logf: func(f string, a ...any) { logs = append(logs, strings.TrimSpace(sprintf(f, a...))) }}, &logs
}
