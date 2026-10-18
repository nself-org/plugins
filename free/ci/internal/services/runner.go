package services

// Purpose: how the engine runs the nself and docker CLIs. Production code
// sends every call through the exec funnel (the only package that may import
// os/exec, Epic D15); tests plug a stub Runner and assert the call sequence.
// Inputs:  a Call (argv, working directory, extra environment).
// Outputs: the exit code and the redacted combined output of the call.
// Constraints: a call never inherits the caller's environment beyond a short
// allow-list; registered secrets are masked in the output before it returns.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Call is one CLI invocation.
type Call struct {
	Dir  string            // working directory; "" means a scratch directory
	Argv []string          // program and arguments
	Env  map[string]string // added to the minimal environment
	Mask []string          // secrets to hide in the output (the engine adds its own)
}

// Result is the outcome of a call.
type Result struct {
	ExitCode int
	Output   string
}

// Runner runs a Call. A non-nil error means the call could not be run at all
// (no such program, cancelled); a failing program is ExitCode != 0.
type Runner interface {
	Run(ctx context.Context, c Call) (Result, error)
}

// passEnv is the only part of the caller's environment a call receives.
var passEnv = []string{"PATH", "HOME", "USER", "LOGNAME", "LANG", "LC_ALL", "TMPDIR", "SHELL",
	"DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_CONFIG", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH", "XDG_RUNTIME_DIR", "XDG_CONFIG_HOME", "COLIMA_HOME"}

type minimalEnv struct{}

func (minimalEnv) Environment(_ context.Context, _ model.Job) ([]string, error) {
	var env []string
	for _, k := range passEnv {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env, nil
}

// bufferRedactor holds the whole output and masks the secrets once at Flush,
// so a secret that spans two read chunks is still masked.
type bufferRedactor struct {
	secrets []string
	buf     []byte
}

func (b *bufferRedactor) Redact(p []byte) []byte { b.buf = append(b.buf, p...); return nil }
func (b *bufferRedactor) Flush() []byte {
	out := maskAll(string(b.buf), b.secrets)
	b.buf = nil
	return []byte(out)
}

// maskAll replaces every non-empty secret in s.
func maskAll(s string, secrets []string) string {
	for _, sec := range secrets {
		if sec != "" {
			s = strings.ReplaceAll(s, sec, "[redacted]")
		}
	}
	return s
}

// funnelDir is the Funnel's own home below NSELF_CI_HOME. The exec funnel keeps
// process markers under its home and every job run sweeps the markers of
// coordinators it does not know as live; a separate home keeps a job's sweep
// away from a service call another job has in flight.
const funnelDir = "svc"

// ownerPrefix starts the coordinator id of every service call; the rest names
// the provision (nselfci-<id>) the call works for, or "none". KillStale asks
// the Provisioner whether that provision's owner is still alive.
const ownerPrefix = "nselfci-svc-"

// ownerOf names the provision a call belongs to from its working directory.
func ownerOf(dir string) string {
	for d := dir; d != "" && d != "." && d != string(filepath.Separator); d = filepath.Dir(d) {
		if b := filepath.Base(d); strings.HasPrefix(b, projectPrefix) {
			return b
		}
	}
	return "none"
}

// staleOwner adapts a provision-owner test to the exec sweep's checker.
type staleOwner func(owner string) bool

func (s staleOwner) Stale(_ context.Context, id string) (bool, error) {
	return strings.HasPrefix(id, ownerPrefix) && s(strings.TrimPrefix(id, ownerPrefix)), nil
}

// KillStale ends the process groups of service calls whose provision owner is
// dead (a coordinator killed with -9 leaves its `nself build` or `docker
// compose up` running; it would create resources after a Sweep removed the
// project). It runs the exec funnel's own orphan sweep through a no-op call and
// returns its error: a call it could not end must stop the teardown.
func (f *Funnel) KillStale(ctx context.Context, dead func(owner string) bool) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	ex := &ciexec.Executor{Env: minimalEnv{}, Stale: staleOwner(dead)}
	argv := []string{"true"}
	return ex.Run(ctx, ciexec.JobSpec{AttemptID: "svcsweep-" + randHex(4), CoordinatorID: ownerPrefix + "sweep",
		Job: model.Job{Run: argv}, Command: argv, Home: filepath.Join(f.Home, funnelDir)}).Err
}

// Funnel is the production Runner: one exec-funnel job per call.
type Funnel struct {
	Home string // NSELF_CI_HOME; workspaces are Home/svc/work/<call id>
	// Secrets lists values to mask in every call's output; read on each call.
	Secrets func() []string

	seq atomic.Uint64
}

// Run runs c as a process job. The program starts in c.Dir through a short
// `sh -c cd` wrapper, because the nself CLI has no project-directory flag and
// the funnel's own workspace is a fresh directory it deletes afterwards.
func (f *Funnel) Run(ctx context.Context, c Call) (Result, error) {
	if len(c.Argv) == 0 {
		return Result{}, fail(CodeProvision, "empty command")
	}
	if runtime.GOOS == "windows" {
		return Result{}, fail(CodeNoDocker, "service dependencies need a POSIX host with Docker")
	}
	id := fmt.Sprintf("svccall-%s-%d", randHex(4), f.seq.Add(1))
	secrets := append([]string(nil), c.Mask...)
	if f.Secrets != nil {
		secrets = append(secrets, f.Secrets()...)
	}
	var out struct {
		sync.Mutex
		b []byte
	}
	env := map[string]string{"NSELF_CI_SVC_DIR": c.Dir}
	for k, v := range c.Env {
		env[k] = v
	}
	ex := &ciexec.Executor{
		Env:         minimalEnv{},
		NewRedactor: func() ciexec.Redactor { return &bufferRedactor{secrets: secrets} },
	}
	argv := append([]string{"sh", "-c", `if [ -n "$NSELF_CI_SVC_DIR" ]; then cd "$NSELF_CI_SVC_DIR" || exit 97; fi; exec "$@"`, "nselfci-svc"}, c.Argv...)
	res := ex.Run(ctx, ciexec.JobSpec{
		AttemptID:     id,
		CoordinatorID: ownerPrefix + ownerOf(c.Dir),
		Job:           model.Job{Run: argv, Env: env},
		Command:       argv,
		Home:          filepath.Join(f.Home, funnelDir),
		Output:        func(p []byte) { out.Lock(); out.b = append(out.b, p...); out.Unlock() },
	})
	out.Lock()
	defer out.Unlock()
	r := Result{ExitCode: res.ExitCode, Output: string(out.b)}
	if res.Err != nil {
		return r, res.Err
	}
	return r, nil
}
