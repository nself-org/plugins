//go:build integration

package runnerjob_test

// Purpose: the container leg shared by the integration tests of the launcher: it runs a job exactly as
// the agent does for a lease that carries a runner credential (cred-wrapper from a read-only agent mount,
// the credential on the job's stdin) through the executor and the hardened container profile, and samples
// what the node and the Docker daemon show while it runs.
// Inputs:  a credential, an image and a command; a Docker daemon (a missing one is a failure, never a skip).
// Outputs: the job's output, its result, `docker inspect` of the job container and the node's ps output.
// Constraints: the agent binary is built for the daemon's architecture into a temp dir and mounted
//          read-only at /usr/local/bin/nself-ci-agent (cmd/agent's wrapCredential does the same; its own
//          test proves the argv, the mount and the stdin). The synthetic credential exists nowhere else.

import (
	"bytes"
	"context"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation/container"
	"github.com/nself-org/plugins/free/ci/internal/isolation/gvisor"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

const (
	stubImage = "busybox:1.37.0@sha256:bdf57e528e45e4433820e045b29b4597825a1c9e38353532d90a01445013f82e"
	realImage = "ghcr.io/actions/actions-runner@sha256:4ffadc0002b2581327e06101fc8c06cd189232baf79fe561fac9caeb76f5e807"
	agentPath = "/usr/local/bin/nself-ci-agent"
)

func docker(t *testing.T, args ...string) string {
	t.Helper()
	out, err := osexec.Command("docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func needDocker(t *testing.T, images ...string) {
	t.Helper()
	if out, err := osexec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Fatalf("a Docker daemon is required: %v\n%s", err, out)
	}
	for _, img := range images {
		docker(t, "pull", "-q", img)
	}
}

// buildAgent builds nself-ci-agent for the daemon's architecture.
func buildAgent(t *testing.T) string {
	t.Helper()
	arch := map[string]string{"x86_64": "amd64", "aarch64": "arm64"}[strings.TrimSpace(docker(t, "info", "--format", "{{.Architecture}}"))]
	if arch == "" {
		t.Fatal("unknown daemon architecture")
	}
	out := filepath.Join(t.TempDir(), "nself-ci-agent")
	cmd := osexec.Command("go", "build", "-mod=vendor", "-o", out, "./cmd/agent")
	cmd.Dir = filepath.Join("..", "..", "..")
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build agent: %v\n%s", err, b)
	}
	return out
}

type wrapIsolator struct {
	p interface {
		Wrap(exec.JobSpec) (exec.JobSpec, error)
	}
}

func (w wrapIsolator) Prepare(_ context.Context, s *exec.JobSpec) error {
	wrapped, err := w.p.Wrap(*s)
	*s = wrapped
	return err
}

type outBuf struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (c *outBuf) write(b []byte) { c.mu.Lock(); c.buf.Write(b); c.mu.Unlock() }
func (c *outBuf) String() string { c.mu.Lock(); defer c.mu.Unlock(); return c.buf.String() }

// jobRun is what one run of the container leg showed.
type jobRun struct {
	Out     string
	Res     exec.AttemptResult
	Inspect string // docker inspect of the job container, sampled while it ran
	Ps      string // the node's process list, sampled while it ran
	Home    string // the agent's state directory of the attempt
	Level   string
	// HomeRead and HomeHits are the files of the state directory read while the job ran and the ones
	// that held the credential.
	HomeRead int
	HomeHits []string
}

// runLeg runs command in image under the container profile (or gVisor when asked and available) with the
// credential on the job's stdin and the wrapper in front.
func runLeg(t *testing.T, id, image, kind, cred string, command []string, sandboxed bool, limit time.Duration) (jobRun, bool) {
	t.Helper()
	agent := buildAgent(t)
	home := t.TempDir()
	out := &outBuf{}
	var iso interface {
		Wrap(exec.JobSpec) (exec.JobSpec, error)
	}
	level := model.Isolation("container")
	if sandboxed {
		g := gvisor.New(gvisor.Options{})
		if av := g.Probe(context.Background()); !av.Available() {
			t.Logf("gVisor leg not run on this host: %+v", av)
			return jobRun{}, false
		}
		iso, level = g, "sandboxed-container"
	} else {
		p := container.New(container.Options{})
		if av := p.Probe(context.Background()); !av.Available() {
			t.Fatalf("the container profile is unavailable: %+v", av)
		}
		iso = p
	}
	spec := exec.JobSpec{AttemptID: id, CoordinatorID: "host19", Home: home, Output: out.write,
		Command: append([]string{agentPath, "cred-wrapper", kind, "--"}, command...),
		Mounts:  []exec.Mount{{Host: agent, Target: agentPath, ReadOnly: true}},
		Job:     model.Job{Isolation: level, Container: &model.ContainerConfig{Image: image}},
		Stdin:   io.MultiReader(bytes.NewReader([]byte(cred)), strings.NewReader("\n")), Masks: []string{cred},
		CPU: 1, MemoryMB: 512, Network: "none", Timeout: limit}
	e := &exec.Executor{Isolation: wrapIsolator{iso}, Grace: 2 * time.Second, NewRedactor: exec.NewOutputRedactor}
	t.Cleanup(func() { _ = exec.RuntimeRemove(context.Background(), "docker", "nself-ci-"+id) })

	run := jobRun{Home: home, Level: string(level)}
	sampled, finished := make(chan struct{}), make(chan struct{})
	go func() { // sample the node while the job is alive
		defer close(sampled)
		for end := time.Now().Add(limit); time.Now().Before(end); time.Sleep(150 * time.Millisecond) {
			select {
			case <-finished:
				return
			default:
			}
			b, err := osexec.Command("docker", "inspect", "nself-ci-"+id).Output()
			if err == nil && len(b) > 10 {
				run.Inspect = string(b)
				ps, _ := osexec.Command("ps", "-axww", "-o", "args=").Output()
				run.Ps = string(ps)
				run.HomeRead, run.HomeHits = scanTree(home, cred)
				return
			}
		}
	}()
	run.Res = e.Run(context.Background(), spec)
	close(finished)
	<-sampled
	run.Out = out.String()
	return run, true
}
