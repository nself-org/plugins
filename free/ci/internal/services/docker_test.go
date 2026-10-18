//go:build docker

package services

// Purpose: the Docker-backed proof of Epic D9 / P7-CI-42: two concurrent
// disposable postgres projects stay apart, every exit path (pass, fail, cancel,
// timeout, kill -9 plus restart) leaves no nselfci- resource behind, and
// service ports bind 127.0.0.1 only.
// Inputs:  a Docker daemon and an `nself` on PATH that has `config services`
// (a cli built from nself-org/cli origin/main; the suite sets NSELF_V15=1).
// Outputs: pass or fail. These tests call t.Fatal, never t.Skip, when a
// prerequisite is missing (a skipped proof is not a proof).

import (
	"bufio"
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("CI42_HELPER") == "1" {
		helperUp() // never returns normally: the parent kills it
	}
	_ = os.Setenv("NSELF_V15", "1")
	os.Exit(m.Run())
}

// helperUp is the child of the kill -9 test: it provisions, reports, sleeps.
func helperUp() {
	home := os.Getenv("CI42_HOME")
	p := &Provisioner{Run: &Funnel{Home: home}, Home: home}
	h, err := p.Up(context.Background(), []Spec{{Name: "db", Component: "postgres"}})
	if err != nil {
		os.Stdout.WriteString("ERR " + err.Error() + "\n")
		os.Exit(3)
	}
	os.Stdout.WriteString("READY " + h.Project + "\n")
	time.Sleep(10 * time.Minute)
	os.Exit(0)
}

func dockerOut(t *testing.T, args ...string) string {
	t.Helper()
	out, err := osexec.Command("docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// need fails the test unless Docker and a v1.5 nself are usable.
func need(t *testing.T) {
	t.Helper()
	if out, err := osexec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Fatalf("Docker is required: %v\n%s", err, out)
	}
	if _, err := osexec.LookPath("nself"); err != nil {
		t.Fatalf("nself is required on PATH: %v", err)
	}
	if out, err := osexec.Command("nself", "config", "services", "list", "--json").CombinedOutput(); err != nil {
		t.Fatalf("`nself config services list --json` failed (build the cli from origin/main): %v\n%s", err, out)
	}
}

func realProv(t *testing.T) *Provisioner {
	t.Helper()
	home := t.TempDir()
	return &Provisioner{Run: &Funnel{Home: home}, Home: home, Wait: 3 * time.Minute}
}

// nselfciResources lists every container, volume and network of an nselfci- compose project.
func nselfciResources(t *testing.T) []string {
	t.Helper()
	var left []string
	for _, kind := range [][]string{
		{"ps", "-a", "--format", `{{.Names}} {{.Label "com.docker.compose.project"}}`},
		{"volume", "ls", "--format", `{{.Name}} {{.Label "com.docker.compose.project"}}`},
		{"network", "ls", "--format", `{{.Name}} {{.Label "com.docker.compose.project"}}`},
	} {
		for _, line := range strings.Split(dockerOut(t, kind...), "\n") {
			if f := strings.Fields(line); len(f) == 2 && strings.HasPrefix(f[1], projectPrefix) {
				left = append(left, kind[0]+":"+f[0])
			}
		}
	}
	return left
}

// nselfciContainers lists the containers of nselfci- compose projects.
func nselfciContainers() []string {
	out, err := osexec.Command("docker", "ps", "-a", "--format", `{{.Label "com.docker.compose.project"}}`).Output()
	if err != nil {
		return nil
	}
	var got []string
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), projectPrefix) {
			got = append(got, l)
		}
	}
	return got
}

func psql(t *testing.T, h *Handle, sql string) string {
	t.Helper()
	return dockerOut(t, "exec", h.Project+"_postgres", "psql", "-U", "nselfci", "-d", "nselfci", "-tA", "-c", sql)
}

func TestTwoConcurrentPostgres(t *testing.T) {
	need(t)
	type res struct {
		h   *Handle
		p   *Provisioner
		err error
	}
	ch := make(chan res, 2)
	for i := 0; i < 2; i++ {
		go func() {
			p := realProv(t)
			h, err := p.Up(context.Background(), []Spec{{Name: "db", Component: "postgres"}})
			ch <- res{h, p, err}
		}()
	}
	a, b := <-ch, <-ch
	for _, r := range []res{a, b} {
		if r.err != nil {
			t.Fatal(r.err)
		}
		defer func(r res) { _ = r.p.Down(context.Background(), r.h) }(r)
	}
	if a.h.Project == b.h.Project || a.h.Network == b.h.Network || a.h.Env["NSELF_CI_SERVICE_DB_PORT"] == b.h.Env["NSELF_CI_SERVICE_DB_PORT"] {
		t.Fatalf("not distinct: %s/%s %s/%s", a.h.Project, b.h.Project, a.h.Network, b.h.Network)
	}
	psql(t, a.h, "create table only_in_a(x int); insert into only_in_a values (1)")
	if got := psql(t, b.h, "select to_regclass('only_in_a') is null"); got != "t" {
		t.Fatalf("data written by one project is visible to the other: %q", got)
	}
	if got := psql(t, a.h, "select count(*) from only_in_a"); got != "1" {
		t.Fatalf("project a lost its data: %q", got)
	}
}

func TestLoopbackOnly(t *testing.T) {
	need(t)
	p := realProv(t)
	h, err := p.Up(context.Background(), []Spec{{Name: "db", Component: "postgres"}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Down(context.Background(), h) }()
	ports := dockerOut(t, "port", h.Project+"_postgres")
	if !strings.Contains(ports, "127.0.0.1:"+h.Env["NSELF_CI_SERVICE_DB_PORT"]) || strings.Contains(ports, "0.0.0.0") || strings.Contains(ports, "[::]") {
		t.Fatalf("service must bind 127.0.0.1 only:\n%s", ports)
	}
	if n := strings.Count(dockerOut(t, "ps", "-q", "--filter", "label=com.docker.compose.project="+h.Project), "\n") + 1; n != 1 {
		t.Fatalf("a postgres-only job must start exactly one container, got %d", n)
	}
}

func TestTeardownEveryExit(t *testing.T) {
	need(t)
	empty := func(when string) {
		t.Helper()
		if left := nselfciResources(t); len(left) != 0 {
			t.Fatalf("%s: left behind: %v", when, left)
		}
	}
	specs := []Spec{{Name: "db", Component: "postgres"}}
	t.Run("pass and fail", func(t *testing.T) { // the job outcome does not change teardown
		for range 2 {
			p := realProv(t)
			h, err := p.Up(context.Background(), specs)
			if err != nil {
				t.Fatal(err)
			}
			if err := p.Down(context.Background(), h); err != nil {
				t.Fatal(err)
			}
		}
		empty("after pass/fail")
	})
	t.Run("cancel", func(t *testing.T) {
		// Cancel once the project's container exists, whatever the host speed:
		// postgres still needs seconds to answer, so the Up is always mid-wait.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			for deadline := time.Now().Add(3 * time.Minute); time.Now().Before(deadline) && len(nselfciContainers()) == 0; {
				time.Sleep(100 * time.Millisecond)
			}
			cancel()
		}()
		if _, err := realProv(t).Up(ctx, specs); err == nil {
			t.Fatal("expected the cancelled Up to fail")
		}
		empty("after cancel")
	})
	t.Run("timeout", func(t *testing.T) {
		// Shorter than any Up (eight --help probes alone take longer on a warm host).
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()
		if _, err := realProv(t).Up(ctx, specs); err == nil {
			t.Fatal("expected the timed-out Up to fail")
		}
		empty("after timeout")
	})
	t.Run("kill -9 and restart", func(t *testing.T) {
		home := t.TempDir()
		cmd := osexec.Command(os.Args[0], "-test.run=^$")
		cmd.Env = append(os.Environ(), "CI42_HELPER=1", "CI42_HOME="+home)
		out, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		line, _ := bufio.NewReader(out).ReadString('\n')
		if !strings.HasPrefix(line, "READY ") {
			_ = cmd.Process.Kill()
			t.Fatalf("helper did not provision: %q", line)
		}
		if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
			t.Fatal(err)
		}
		_ = cmd.Wait()
		if len(nselfciResources(t)) == 0 {
			t.Fatal("the killed coordinator left nothing to sweep: the test proves nothing")
		}
		restarted := &Provisioner{Run: &Funnel{Home: home}, Home: home}
		n, err := restarted.Sweep(context.Background())
		if err != nil || n != 1 {
			t.Fatalf("swept %d, err %v", n, err)
		}
		empty("after kill -9 and sweep")
		if entries, _ := filepath.Glob(filepath.Join(home, "work", "nselfci-*")); len(entries) != 0 {
			t.Fatalf("work files left: %v", entries)
		}
	})
}

// TestOptionalRedis proves an optional component: enabled, started alone,
// loopback-bound, answering, and gone after Down.
func TestOptionalRedis(t *testing.T) {
	need(t)
	p := realProv(t)
	h, err := p.Up(context.Background(), []Spec{{Name: "cache", Component: "redis"}})
	if err != nil {
		t.Fatal(err)
	}
	ports := dockerOut(t, "port", h.Project+"_redis")
	if !strings.Contains(ports, "127.0.0.1:"+h.Env["NSELF_CI_SERVICE_CACHE_PORT"]) || strings.Contains(ports, "0.0.0.0") {
		t.Fatalf("redis must bind 127.0.0.1 only:\n%s", ports)
	}
	if n := len(strings.Fields(dockerOut(t, "ps", "-q", "--filter", "label=com.docker.compose.project="+h.Project))); n != 1 {
		t.Fatalf("a redis-only job must start exactly one container, got %d", n)
	}
	if err := p.Down(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	if left := nselfciResources(t); len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
}
