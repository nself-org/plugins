package tart

import (
	"context"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// D-1355: the limits of the small-scope proof and the slot ledger.

const netstatHeader = "Proto Recv-Q Send-Q  Local Address          Foreign Address        (state)      rhiwat shiwat    pid   epid state    options\n"

func TestParseListenersKeepsOnlyWildcardTCPListeners(t *testing.T) {
	out := netstatHeader +
		"tcp4       0      0  *.5900                 *.*                    LISTEN      131072 131072   300      0 0x0100 0x00000106\n" +
		"tcp46      0      0  *.7000                 *.*                    LISTEN      131072 131072   301      0 0x0100 0x00000106\n" +
		"tcp6       0      0  ::.9100                *.*                    LISTEN      131072 131072   302      0 0x0100 0x00000106\n" +
		"tcp4       0      0  127.0.0.1.8000         *.*                    LISTEN      131072 131072   303      0 0x0100 0x00000106\n" +
		"tcp4       0      0  192.168.64.1.3128      *.*                    LISTEN      131072 131072   304      0 0x0100 0x00000106\n" +
		"tcp4       0      0  192.168.1.20.50000     1.2.3.4.443            ESTABLISHED 131072 131072   305      0 0x0100 0x00000106\n"
	var ports []string
	for _, l := range parseListeners([]byte(out)) {
		ports = append(ports, l.Port)
	}
	if got := strings.Join(ports, ","); got != "5900,7000,9100" {
		t.Fatalf("wildcard listeners: %s", got)
	}
}

func TestHostListenerOutsideTheCheckedPortsRefusesScopesBelowLan(t *testing.T) {
	r := newRig(t, func(o *Options) {
		o.Net.Facts = func(context.Context) (netFacts, error) { return facts(), nil }
	})
	list := filepath.Join(t.TempDir(), "netstat")
	if err := os.WriteFile(list, []byte("tcp4  0  0  *.9100  *.*  LISTEN  131072 131072 1 0 0x0100 0x00000106\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_NETSTAT_FILE", list)
	s := r.spec("l1", "true")
	s.Network = "none"
	_, err := r.p.Wrap(s)
	if !isScopeErr(err) || !strings.Contains(err.Error(), "9100") {
		t.Fatalf("a wildcard listener on a port the check does not name must refuse the scope: %v", err)
	}
	s.Network = "lan"
	if _, err := r.p.Wrap(s); err != nil {
		t.Fatalf("lan does not depend on the host's listeners: %v", err)
	}
	r.p.slots.release(false)
	t.Setenv("FAKE_NETSTAT_FILE", "")
	t.Setenv("FAKE_NETSTAT_FAIL", "1")
	s.Network = "none"
	if _, err := r.p.Wrap(s); !isScopeErr(err) {
		t.Fatalf("a scan that cannot run must fail closed: %v", err)
	}
}

func TestIPv6ChecksAreInTheScript(t *testing.T) {
	c := planChecks(softnetPlan{Scope: "none"}, gw, nil, facts())
	script := string(c.script())
	for _, want := range []string{"png6 ::1 &", "png6 2606:4700:4700::1111 &", "rt6 2606:4700:4700::1111 &"} {
		if !strings.Contains(script, want) {
			t.Fatalf("script lacks %q:\n%s", want, script)
		}
	}
}

func TestSlotLedgerCountsAnotherProcessBootingClone(t *testing.T) {
	r := newRig(t, nil)
	// another coordinator process (its own ledger) claimed a marker and is booting: Tart does not list the clone yet
	other := New("vm", func() Options { o := r.p.opts; o.Slots = NewSlots(); return o }())
	c, err := claimMarker(stateDir(r.home), CloneName("peer1"), marker{Name: CloneName("peer1")})
	if err != nil {
		t.Fatal(err)
	}
	defer c.release()
	av := other.Probe(context.Background())
	if av.Facts["free_slots"] != "1" || av.Facts["booting_other"] != "1" {
		t.Fatalf("a locked marker with no listed clone holds a slot: %+v", av.Facts)
	}
	// this process's own claim is not "another process": the ledger already counts it
	r.p.slots.hold(CloneName("peer1"))
	if av := r.p.probe(context.Background(), true); av.Facts["booting_other"] != "0" {
		t.Fatalf("a held marker is not a peer: %+v", av.Facts)
	}
}

func TestJobExit255IsTheJobsAndADroppedSessionIsInfra(t *testing.T) {
	r := newRig(t, nil)
	res, out := r.run(context.Background(), r.spec("x1", "sh", "-c", "exit 255"))
	if res.ExitCode != 255 || res.Err != nil || res.Attempt.FailureClass != "code" {
		t.Fatalf("the job's own exit 255 is a code failure: %+v\n%s", res, out)
	}
	r.noClones()
	t.Setenv("FAKE_SSH_DROP", "1")
	res, out = r.run(context.Background(), r.spec("x2", "sh", "-c", "exit 0"))
	if res.Err == nil || res.Attempt.FailureClass != "infra" {
		t.Fatalf("a dropped ssh session (exit 255, no status line) is an infra failure: %+v\n%s", res, out)
	}
	r.noClones()
}

func TestStatusLineSurvivesChunkingIsStrippedAndNotForged(t *testing.T) {
	var got strings.Builder
	st, err := newExitStatus(func(b []byte) { got.Write(b) })
	if err != nil {
		t.Fatal(err)
	}
	stream := "job says hi\nand \nnself-ci:exit:guess:1\n more" + "\n" + st.prefix + "255\n"
	for i := 0; i < len(stream); i += 3 {
		st.write([]byte(stream[i:min(i+3, len(stream))]))
	}
	st.write([]byte("\nnself-ci"))
	code, ok := st.flush()
	if !ok || code != 255 {
		t.Fatalf("status %d %v", code, ok)
	}
	if want := "job says hi\nand \nnself-ci:exit:guess:1\n more" + "\nnself-ci"; got.String() != want {
		t.Fatalf("output %q, want %q", got.String(), want)
	}
}

// slowDelete is a Tool whose `tart delete` waits for release, like a clone that takes minutes to go.
type slowDelete struct {
	Tool
	release chan struct{}
}

func (s slowDelete) Run(ctx context.Context, argv []string, stdin io.Reader, out io.Writer) (Result, error) {
	if len(argv) > 1 && argv[1] == "delete" {
		select {
		case <-s.release:
		case <-ctx.Done():
		}
	}
	return s.Tool.Run(ctx, argv, stdin, out)
}

func TestOrphanSweepDoesNotBlockTheProbe(t *testing.T) {
	release := make(chan struct{})
	r := newRig(t, func(o *Options) {
		inner := NewExecTool
		o.NewTool = func(c, h string) Tool { return slowDelete{Tool: inner(c, h), release: release} }
	})
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
		c.unlockKeep()
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
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		vms, _ := listVMs(context.Background(), tool, r.p.opts.Tart)
		if f, o := vmCounts(vms); f+o == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fake VMs did not start")
		}
	}
	done := make(chan struct{})
	go func() { r.p.Probe(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the probe waited for the orphan sweep (it must stay inside its 30 s bound)")
	}
	close(release)
	r.p.bg.Wait()
	if av := r.p.probe(context.Background(), true); !av.Available() || av.Facts["free_slots"] != "2" {
		t.Fatalf("the background sweep frees the slots: %+v", av)
	}
	r.noClones()
}
