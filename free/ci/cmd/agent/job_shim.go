package main

// Purpose: `nself-ci-agent job-shim` (P7-NODE-06, Epic D8): the one command the agent runs through sudo
// as a job user (`sudo -n -u nself-ci-job-<n> /opt/nself-ci-agent/bin/nself-ci-agent job-shim`) to
// start one job step under that user's UID, so a job never runs as the agent and cannot read the
// agent's node key or age identity.
// Inputs:  one JSON line on stdin: {"argv":[...],"dir":"/abs","env":["K=V",...]}; every byte after the
// newline stays on stdin for the job.
// Outputs: none; the process becomes argv[0] (execve) with exactly the given environment.
// Constraints: argv comes from the lease and is never handed to a shell; the shim refuses to run as root
// or as any account that is not nself-ci-job-<n> (exit 1), reads the request a byte at a time with a
// 1 MiB cap, accepts no unknown field, keeps nothing of sudo's environment, and changes into dir only
// when it is absolute. A malformed request is exit 2.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"regexp"
	"strings"
	"syscall"
)

const maxShimRequest = 1 << 20

var jobUserName = regexp.MustCompile(`^nself-ci-job-[1-9][0-9]{0,2}$`)

func init() {
	must(RegisterSubcommand("job-shim", "run one job step as this job user (started through sudo by the agent)", runJobShim))
}

// shimRequest is the line the agent writes.
type shimRequest struct {
	Argv []string `json:"argv"`
	Dir  string   `json:"dir"`
	Env  []string `json:"env"`
}

// readShimRequest reads one line of at most max bytes, a byte at a time.
func readShimRequest(r io.Reader, max int) ([]byte, error) {
	var line []byte
	var b [1]byte
	for len(line) <= max {
		n, err := r.Read(b[:])
		if n == 1 {
			if b[0] == '\n' {
				return line, nil
			}
			line = append(line, b[0])
		}
		if err != nil {
			return nil, errors.New("no job request on stdin")
		}
	}
	return nil, errors.New("job request too long")
}

// parseShimRequest decodes and checks a request.
func parseShimRequest(raw []byte) (shimRequest, error) {
	var req shimRequest
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return req, fmt.Errorf("bad job request: %w", err)
	}
	if len(req.Argv) == 0 || req.Argv[0] == "" {
		return req, errors.New("bad job request: argv is empty")
	}
	for _, a := range req.Argv {
		if strings.ContainsRune(a, 0) {
			return req, errors.New("bad job request: argv holds a NUL byte")
		}
	}
	for _, e := range req.Env {
		k, _, ok := strings.Cut(e, "=")
		if !ok || k == "" || strings.ContainsRune(e, 0) {
			return req, errors.New("bad job request: env entries are KEY=VALUE")
		}
	}
	if req.Dir != "" && (req.Dir[0] != '/' || strings.ContainsRune(req.Dir, 0)) {
		return req, errors.New("bad job request: dir must be an absolute path")
	}
	return req, nil
}

// shimUserOK reports whether the process runs as a job user and not as root.
func shimUserOK(euid int, name string) bool {
	return euid > 0 && jobUserName.MatchString(name)
}

func runJobShim(_ context.Context, args []string, sio IO) int {
	if len(args) != 0 {
		fmt.Fprintln(sio.Stderr, "usage: nself-ci-agent job-shim   (one JSON request line on stdin)")
		return 2
	}
	name := ""
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	if !shimUserOK(os.Geteuid(), name) {
		fmt.Fprintln(sio.Stderr, "error: job-shim runs only as a job user (nself-ci-job-<n>), through the agent's sudo rule")
		return 1
	}
	raw, err := readShimRequest(sio.Stdin, maxShimRequest)
	if err != nil {
		fmt.Fprintln(sio.Stderr, "error:", err)
		return 2
	}
	req, err := parseShimRequest(raw)
	if err != nil {
		fmt.Fprintln(sio.Stderr, "error:", err)
		return 2
	}
	return execShimRequest(req, sio)
}

// execShimRequest replaces the process with the requested command.
func execShimRequest(req shimRequest, sio IO) int {
	path, err := lookCommand(req.Argv[0])
	if err != nil {
		fmt.Fprintln(sio.Stderr, "error:", err)
		return 127
	}
	dir := req.Dir
	if dir == "" {
		dir = "/" // never the agent's inherited cwd (its 0700 state directory)
	}
	if err = os.Chdir(dir); err != nil {
		fmt.Fprintln(sio.Stderr, "error: cannot enter the job directory:", err)
		return 126
	}
	err = syscall.Exec(path, req.Argv, req.Env) // only returns on failure
	fmt.Fprintln(sio.Stderr, "error: exec:", err)
	return 126
}
