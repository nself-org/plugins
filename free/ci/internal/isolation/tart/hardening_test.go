package tart

import (
	"archive/tar"
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestHostSSHNeverSeesJobEnvOrSecretNames(t *testing.T) {
	r := newRig(t, nil)
	envlog := filepath.Join(t.TempDir(), "ssh.env")
	t.Setenv("FAKE_SSH_ENVLOG", envlog)
	s := r.spec("e1", "sh", "-c", `echo "$API_TOKEN|$PLAIN"`)
	s.Job.Env = map[string]string{"DYLD_INSERT_LIBRARIES": "./x.dylib", "PLAIN": "p"}
	s.Job.Secrets = []string{"project/API_TOKEN"}
	s.Secrets = fakeSecrets{"API_TOKEN": "tok-value"}
	res, out := r.run(context.Background(), s)
	if res.Err != nil || !strings.Contains(out, "tok-value|p") {
		t.Fatalf("the job must still get its env inside the VM: %v %q", res.Err, out)
	}
	b, _ := os.ReadFile(envlog)
	env := string(b)
	for _, bad := range []string{"DYLD_INSERT_LIBRARIES", "PLAIN=", "API_TOKEN="} {
		if strings.Contains(env, bad) {
			t.Fatalf("the host ssh process got %q:\n%s", bad, env)
		}
	}
	if !strings.Contains(env, "NSELF_CI_VMSECRET_0=tok-value") {
		t.Fatalf("the secret value is still registered under a harmless name:\n%s", env)
	}
}

func TestBadEnvNameIsRefusedBeforeBoot(t *testing.T) {
	r := newRig(t, nil)
	s := r.spec("e2", "true")
	s.Job.Env = map[string]string{"bad-name": "v"}
	res, _ := r.run(context.Background(), s)
	if res.Err == nil || !res.Refused {
		t.Fatalf("%+v", res)
	}
	if strings.Contains(r.read("tart.log"), "clone") {
		t.Fatal("nothing may be cloned for a job that cannot run")
	}
	r.noClones()
}

func TestCollectStopsAtTheByteCap(t *testing.T) {
	old := maxCollectBytes
	maxCollectBytes = 3000
	t.Cleanup(func() { maxCollectBytes = old })
	r := newRig(t, nil)
	s := r.spec("g1", "sh", "-c", "mkdir -p dist && head -c 7000000 /dev/urandom > dist/big.bin")
	s.Job.Artifacts = []model.ArtifactDecl{{Path: "dist"}}
	res, _ := r.run(context.Background(), s)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "cap") {
		t.Fatalf("want a cap error, got %+v", res.Err)
	}
	left, _ := os.ReadDir(stateDir(r.home))
	for _, e := range left {
		t.Errorf("scratch left in the state dir: %s", e.Name())
	}
	r.noClones()
}

func TestLimitFileDropsAndCancels(t *testing.T) {
	f, _ := os.CreateTemp(t.TempDir(), "x")
	defer f.Close()
	cancelled := false
	l := &limitFile{f: f, max: 10, cancel: func() { cancelled = true }}
	l.Write([]byte("12345"))
	if cancelled {
		t.Fatal("early cancel")
	}
	n, err := l.Write([]byte("1234567890"))
	if n != 10 || err != nil || !cancelled || !l.exceeded {
		t.Fatal(n, err, cancelled)
	}
	l.Write([]byte("more"))
	if st, _ := f.Stat(); st.Size() != 10 {
		t.Fatalf("size %d", st.Size())
	}
}

func TestExtractCapsEntriesAndDepth(t *testing.T) {
	oldE, oldD := maxCollectEntries, maxCollectDepth
	maxCollectEntries, maxCollectDepth = 3, 2
	t.Cleanup(func() { maxCollectEntries, maxCollectDepth = oldE, oldD })
	many := []tar.Header{}
	for i := 0; i < 4; i++ {
		many = append(many, tar.Header{Name: fmt.Sprintf("dist/f%d", i), Typeflag: tar.TypeReg, Mode: 0o644})
	}
	root, _ := os.OpenRoot(t.TempDir())
	defer root.Close()
	if err := extractArtifacts(strings.NewReader(mkTar(many...)), root, []string{"dist"}, 1<<20); err == nil {
		t.Fatal("too many entries")
	}
	if err := extractArtifacts(strings.NewReader(mkTar(tar.Header{Name: "dist/a/b/c/d", Typeflag: tar.TypeReg})), root, []string{"dist"}, 1<<20); err == nil {
		t.Fatal("too deep")
	}
}

func TestOrphansThatFillBothSlotsAreSweptByTheProbe(t *testing.T) {
	r := newRig(t, nil)
	tool := r.p.opts.NewTool("c", r.home)
	var procs []*osexec.Cmd
	for _, n := range []string{"nself-ci-orphan1", "nself-ci-orphan2"} {
		if _, err := tool.Run(context.Background(), []string{r.p.opts.Tart, "clone", testImage, n}, nil, nil); err != nil {
			t.Fatal(err)
		}
		c, err := claimMarker(stateDir(r.home), n, marker{Name: n})
		if err != nil {
			t.Fatal(err)
		}
		c.unlockKeep() // its owner died
		cmd := osexec.Command(r.p.opts.Tart, "run", "--no-graphics", n)
		cmd.Env = append(os.Environ(), "FAKE_TART_DIR="+r.dir)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		procs = append(procs, cmd)
	}
	t.Cleanup(func() {
		for _, c := range procs {
			_ = c.Process.Kill()
			_, _ = c.Process.Wait()
		}
	})
	deadline := time.Now().Add(10 * time.Second)
	for {
		vms, _ := listVMs(context.Background(), tool, r.p.opts.Tart)
		if f, o := vmCounts(vms); f+o == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fake VMs did not start")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if av := r.p.Probe(context.Background()); av.Available() {
		t.Fatalf("the probe answers before the sweep ends: %+v", av)
	}
	r.p.bg.Wait()
	av := r.p.probe(context.Background(), true)
	if !av.Available() || av.Facts["free_slots"] != "2" {
		t.Fatalf("two orphans must not block the runner: %+v", av)
	}
	r.noClones()
}

func TestClaimDetectsAReplacedMarker(t *testing.T) {
	dir := t.TempDir()
	c, err := claimMarker(dir, "nself-ci-r", marker{})
	if err != nil {
		t.Fatal(err)
	}
	if !samePath(c.f, c.path) {
		t.Fatal("fresh claim")
	}
	os.Remove(c.path) // a sweep unlinked it
	if samePath(c.f, c.path) {
		t.Fatal("an unlinked marker must be detected")
	}
	c.unlockKeep()
}

func TestOldScratchIsSwept(t *testing.T) {
	r := newRig(t, nil)
	dir := stateDir(r.home)
	os.MkdirAll(dir, 0o700)
	old, fresh := filepath.Join(dir, "nself-ci-x.out-111"), filepath.Join(dir, "nself-ci-y.out-222")
	os.WriteFile(old, []byte("vm data"), 0o600)
	os.WriteFile(fresh, []byte("vm data"), 0o600)
	past := time.Now().Add(-48 * time.Hour)
	os.Chtimes(old, past, past)
	r.p.Sweep(context.Background(), r.p.opts.NewTool("c", r.home), dir)
	if _, err := os.Stat(old); err == nil {
		t.Fatal("old scratch survived")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("fresh scratch may belong to a running collect")
	}
}

func TestScratchRuleKeepsMarkersOfOddAttemptIDs(t *testing.T) {
	r := newRig(t, nil)
	dir := stateDir(r.home)
	c, err := claimMarker(dir, "nself-ci-x.out-1", marker{Name: "nself-ci-x.out-1"})
	if err != nil {
		t.Fatal(err)
	}
	c.unlockKeep()
	past := time.Now().Add(-72 * time.Hour)
	os.Chtimes(c.path, past, past)
	tool := r.p.opts.NewTool("c", r.home)
	if _, err := tool.Run(context.Background(), []string{r.p.opts.Tart, "clone", testImage, "nself-ci-x.out-1"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if gone := r.p.Sweep(context.Background(), tool, dir); len(gone) != 1 {
		t.Fatalf("an attempt id containing .out- is a marker, not scratch: %v", gone)
	}
	r.noClones()
}

func TestProxyNeedsTheVMAddress(t *testing.T) {
	start := HostProxy(nil, nil, nil)
	if _, err := start(context.Background(), ProxyRequest{Scope: "restricted", Gateway: gw}); !isScopeErr(err) {
		t.Fatalf("an unfiltered proxy must not start: %v", err)
	}
}

func TestNoneScopeFailsWhenTheFactsCannotBeReadForTheProof(t *testing.T) {
	calls := 0
	r := newRig(t, func(o *Options) {
		o.Net = NetSource{Facts: func(context.Context) (netFacts, error) {
			calls++
			if calls > 1 {
				return netFacts{}, errTest
			}
			return facts(), nil
		}}
	})
	t.Setenv("FAKE_SSH_NETCHECK", "1")
	t.Setenv("FAKE_SSH_UP", "192.168.64.1")
	s := r.spec("n9", "true")
	s.Network = "none"
	res, _ := r.run(context.Background(), s)
	if res.Err == nil {
		t.Fatal("a proof with fewer targets must not pass")
	}
	r.noClones()
}
