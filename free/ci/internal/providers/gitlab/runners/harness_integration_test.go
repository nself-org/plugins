//go:build integration

package runners_test

// Purpose: the container leg shared by the integration tests of the GitLab launcher: it runs a job the way
// the agent does for a lease that carries a gitlab-runner-token credential (the agent binary mounted
// read-only AS THE ENTRYPOINT, because the gitlab-runner image's own entrypoint is a launcher; the command
// starts at the credential wrapper; the credential on the job's stdin) through the executor and the hardened
// container profile, and samples what the node and the Docker daemon show while it runs.
// Inputs:  a credential, an image and a command; a Docker daemon (a missing one is a failure, never a skip).
// Outputs: the job's output, its result, `docker inspect` of the job container and the node's ps output.
// Constraints: the agent binary is built for the daemon's architecture into a temp dir and mounted at
//          /usr/local/bin/nself-ci-agent (cmd/agent's wrapCredential does the same; its own test proves the
//          argv, the entrypoint, the mount and the stdin). The synthetic credential exists nowhere else. The
//          gVisor leg needs runsc: a Linux host without it fails, any other host logs that it did not run.

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"strconv"
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
	agentPath = "/usr/local/bin/nself-ci-agent"
	// credKind is the lease credential kind of a GitLab runner.
	credKind = "gitlab-runner-token"
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
	cmd.Dir = filepath.Join("..", "..", "..", "..")
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
	// HomeRead and HomeHits are the entries of the state directory read while the job ran and the files
	// that held the credential.
	HomeRead int
	HomeHits []string
}

// scanTree counts the entries under root and lists the files that contain needle.
func scanTree(root, needle string) (read int, hits []string) {
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		read++
		if d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if b, rerr := os.ReadFile(p); rerr == nil && strings.Contains(string(b), needle) {
			hits = append(hits, p)
		}
		return nil
	})
	return read, hits
}

// legOptions are the knobs of one run.
type legOptions struct {
	ID        string
	Image     string
	Cred      string
	Command   []string
	Sandboxed bool
	Limit     time.Duration
	Masks     []string // extra masks besides the credential (what cmd/agent's partialMasks adds)
	Network   model.NetworkScope
}

// runLeg runs one container leg; ran is false when the host cannot run that level (logged, not a skip).
func runLeg(t *testing.T, o legOptions) (jobRun, bool) {
	t.Helper()
	id := o.ID + "-" + strconv.Itoa(os.Getpid()) // container names are global to the daemon: two runs at once must not meet
	agent := buildAgent(t)
	home := t.TempDir()
	out := &outBuf{}
	var iso interface {
		Wrap(exec.JobSpec) (exec.JobSpec, error)
	}
	level := model.Isolation("container")
	if o.Sandboxed {
		g := gvisor.New(gvisor.Options{})
		if av := g.Probe(context.Background()); !av.Available() {
			if runtime.GOOS == "linux" { // the gVisor leg is the one the design rests on: a Linux host without runsc fails
				t.Fatalf("gVisor (runsc registered with Docker) is required on Linux: %+v", av)
			}
			t.Logf("gVisor leg not run on this host (%s): %+v", runtime.GOOS, av)
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
	netw := o.Network
	if netw == "" {
		netw = "none"
	}
	spec := exec.JobSpec{AttemptID: id, CoordinatorID: "host07", Home: home, Output: out.write,
		Command:    append([]string{"cred-wrapper", credKind, "--"}, o.Command...),
		Entrypoint: agentPath, Mounts: []exec.Mount{{Host: agent, Target: agentPath, ReadOnly: true}},
		Job:   model.Job{Isolation: level, Container: &model.ContainerConfig{Image: o.Image}},
		Stdin: io.MultiReader(bytes.NewReader([]byte(o.Cred)), strings.NewReader("\n")), Masks: append([]string{o.Cred}, o.Masks...), RunnerRoot: true,
		CPU: 1, MemoryMB: 2048, Network: netw, Timeout: o.Limit}
	e := &exec.Executor{Isolation: wrapIsolator{iso}, Grace: 2 * time.Second, NewRedactor: exec.NewOutputRedactor}
	t.Cleanup(func() { _ = exec.RuntimeRemove(context.Background(), "docker", "nself-ci-"+id) })

	run := jobRun{Home: home, Level: string(level)}
	sampled, finished := make(chan struct{}), make(chan struct{})
	go func() { // sample the node every 150 ms for as long as the job is alive, and keep every sample
		defer close(sampled)
		var inspect, ps strings.Builder
		defer func() { run.Inspect, run.Ps = inspect.String(), ps.String() }()
		for end := time.Now().Add(o.Limit); time.Now().Before(end); time.Sleep(150 * time.Millisecond) {
			select {
			case <-finished:
				return
			default:
			}
			b, err := osexec.Command("docker", "inspect", "nself-ci-"+id).Output()
			if err != nil || len(b) < 10 {
				continue
			}
			if inspect.Len() < 1<<20 {
				inspect.Write(b)
			}
			p, _ := osexec.Command("ps", "-axww", "-o", "args=").Output()
			if ps.Len() < 4<<20 {
				ps.Write(p)
			}
			read, hits := scanTree(home, o.Cred)
			run.HomeRead = max(run.HomeRead, read)
			run.HomeHits = append(run.HomeHits, hits...)
		}
	}()
	run.Res = e.Run(context.Background(), spec)
	close(finished)
	<-sampled
	run.Out = out.String()
	return run, true
}
