//go:build integration

package update

// Purpose: TestUpgrade, the asserted test of P7-NODE-24: the engine over both carriers on a sim node, with a real signed release,
//          real stub agents, the real swap, the real store audit and a real slot table. One t.Run per acceptance line and carrier.
// Inputs:  INTEGRATION=1 (the verify line sets it); the go toolchain for the stubs.
// Constraints: no network beyond loopback; the sim home is a temp dir; the "job" is a slot (agent) or the busy file (ssh).

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type events struct {
	mu sync.Mutex
	l  []string
}

func (e *events) add(s string) { e.mu.Lock(); e.l = append(e.l, s); e.mu.Unlock() }
func (e *events) all() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.l...)
}
func (e *events) index(s string) int {
	for i, v := range e.all() {
		if v == s {
			return i
		}
	}
	return -1
}

// traced wraps a Node to log the calls the acceptance lines care about and to inject the "restart" of the sim.
type traced struct {
	Node
	ev                       *events
	afterInstall, afterStart func()
}

func (t *traced) Install(ctx context.Context, in InstallInput) error {
	t.ev.add("install")
	err := t.Node.Install(ctx, in)
	if err == nil && t.afterInstall != nil {
		t.afterInstall()
	}
	return err
}

func (t *traced) Restart(ctx context.Context) error {
	err := t.Node.Restart(ctx)
	if t.afterStart != nil {
		t.afterStart()
	}
	return err
}

func (t *traced) Hello(ctx context.Context, w time.Duration) (string, error) {
	v, err := t.Node.Hello(ctx, w)
	t.ev.add("hello:" + v)
	return v, err
}

// CanDowngrade passes the wrapped node's answer through (the engine asks for it by type assertion).
func (t *traced) CanDowngrade() bool {
	d, ok := t.Node.(Downgrader)
	return !ok || d.CanDowngrade()
}

func (t *traced) Probe(ctx context.Context) error { t.ev.add("probe"); return t.Node.Probe(ctx) }

// simControl is the coordinator-side view of a pull agent on the sim home; Upgrade runs the agent's own handler.
type simControl struct {
	s        *sim
	releases map[string]*release
	agentErr error
	mu       sync.Mutex
}

func (c *simControl) AgentVersion(ctx context.Context, _ string) (string, error) {
	return c.s.version(ctx)
}
func (c *simControl) ActiveJobs(context.Context, string) (int, error) {
	_, used, err := slots.Status(c.s.state())
	return used, err
}

func (c *simControl) Upgrade(_, version, _ string) error {
	var src Source = failingSource{}
	if r, ok := c.releases[version]; ok {
		src = r.source()
	}
	cur, _ := c.s.version(context.Background())
	o := localOpts(c.s, cur, version, src)
	o.AllowRestore = true // what the agent's upgrade-message handler sets: the coordinator may send it back to the binary it kept
	_, err := ApplyLocal(context.Background(), o)
	c.mu.Lock()
	if err != nil && c.agentErr == nil { // keep the first refusal; the rollback message that follows succeeds
		c.agentErr = err
	}
	c.mu.Unlock()
	return nil // the message is queued either way; the agent's refusal is not carried back (no result frame in protocol v1)
}

func (c *simControl) WaitHello(ctx context.Context, _ string, _ time.Time, wait time.Duration) (string, error) {
	end := time.Now().Add(wait)
	for {
		if v, err := c.s.version(ctx); err == nil {
			return v, nil
		} else if !time.Now().Before(end) {
			return "", fmt.Errorf("E656: no hello within %s: %w", wait, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type rig struct {
	t       *testing.T
	carrier string
	s       *sim
	st      *store.Store
	ev      *events
	ctl     *simControl
	state   string // "online" or "draining"
	probes  int
	node    *traced
}

func newRig(t *testing.T, carrier, installed string, releases ...*release) *rig {
	t.Helper()
	st, err := store.Open(t.TempDir()+"/state.db", store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r := &rig{t: t, carrier: carrier, s: newSim(t, installed), st: st, ev: &events{}, state: "online"}
	drain := func(context.Context) (func(context.Context) error, error) {
		was := r.state == "online"
		r.state = "draining"
		if !was {
			return nil, nil
		}
		return func(context.Context) error { r.state = "online"; return nil }, nil
	}
	probe := func(context.Context) error { r.probes++; return nil }
	var inner Node
	if carrier == "ssh" {
		inner = &SSHNode{NodeID: "n1", Plat: hostPlatform, Run: r.s.Run, Copy: r.s.Copy, DrainFn: drain, ProbeFn: probe, HelloPoll: 5 * time.Millisecond}
	} else {
		r.ctl = &simControl{s: r.s, releases: map[string]*release{}}
		for _, rel := range releases {
			r.ctl.releases[rel.Version] = rel
		}
		inner = &AgentNode{NodeID: "n1", Plat: hostPlatform, Ctl: r.ctl, DrainFn: drain, ProbeFn: probe}
	}
	r.node = &traced{Node: inner, ev: r.ev}
	return r
}

// breakNewAgent makes the 1.5.0 agent fail `version` once it is "restarted".
func (r *rig) breakNewAgent() {
	hook := func() { r.s.flag("broken-1.5.0", "x") }
	if r.carrier == "ssh" {
		r.node.afterStart = hook
	} else {
		r.node.afterInstall = hook
	}
}

func (r *rig) run(rel *release, version string, mut func(*Request)) (Result, error) {
	rq := Request{Version: version, ManifestURL: "https://example.invalid/m.json", DrainWait: 5 * time.Second, HelloWait: 300 * time.Millisecond, Poll: 10 * time.Millisecond}
	if mut != nil {
		mut(&rq)
	}
	e := Engine{Source: rel.source(), Audit: StoreAuditor{Store: r.st, Actor: "cli:test"}}
	return e.Run(context.Background(), r.node, rq)
}

func (r *rig) audit() []string {
	rows, err := r.st.AuditEntries(context.Background(), 0, 100)
	if err != nil {
		r.t.Fatal(err)
	}
	var out []string
	for _, row := range rows {
		out = append(out, strings.TrimPrefix(row.Action, "node.upgrade."))
	}
	return out
}

// startJob occupies the node for d and logs "job_end" when it finishes on its own.
func (r *rig) startJob(d time.Duration) {
	if r.carrier == "ssh" {
		r.s.flag("busy", "1")
		tm := time.AfterFunc(d, func() {
			_ = os.WriteFile(filepath.Join(r.s.state(), "busy"), []byte("0"), 0o600)
			r.ev.add("job_end")
		})
		r.t.Cleanup(func() { tm.Stop() })
		return
	}
	tbl, err := slots.Open(r.s.state(), 2)
	if err != nil {
		r.t.Fatal(err)
	}
	sl, err := tbl.Acquire(slots.Self("c1", "lease-1"))
	if err != nil {
		r.t.Fatal(err)
	}
	tm := time.AfterFunc(d, func() { _ = tbl.Release(sl); r.ev.add("job_end") })
	r.t.Cleanup(func() { tm.Stop() })
}

func TestUpgrade(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("INTEGRATION=1 runs the sim-node upgrade")
	}
	for _, carrier := range []string{"ssh", "agent"} {
		t.Run(carrier, func(t *testing.T) {
			t.Run("hello_at_the_new_version_and_a_fresh_probe", func(t *testing.T) {
				rel := newRelease(t, "1.5.0", 1, 1)
				r := newRig(t, carrier, "1.4.0", rel)
				res, err := r.run(rel, "1.5.0", nil)
				if err != nil || res.RolledBack || installed(t, r.s) != "1.5.0" || r.state != "online" || r.probes != 1 {
					t.Fatalf("res %+v err %v version %s state %s probes %d", res, err, installed(t, r.s), r.state, r.probes)
				}
				if r.ev.index("hello:1.5.0") < 0 || r.ev.index("hello:1.5.0") > r.ev.index("probe") || r.ev.index("install") > r.ev.index("hello:1.5.0") {
					t.Fatalf("order %v", r.ev.all())
				}
				if got := strings.Join(r.audit(), ","); got != "verify,drain,wait,install,restart,hello,probe,enable" {
					t.Fatalf("audit %s", got)
				}
			})
			t.Run("running_job_waits_for_drain_and_is_not_interrupted", func(t *testing.T) {
				rel := newRelease(t, "1.5.0", 1, 1)
				r := newRig(t, carrier, "1.4.0", rel)
				r.startJob(250 * time.Millisecond)
				if _, err := r.run(rel, "1.5.0", nil); err != nil {
					t.Fatal(err)
				}
				if r.ev.index("job_end") < 0 || r.ev.index("job_end") > r.ev.index("install") || installed(t, r.s) != "1.5.0" {
					t.Fatalf("the install did not wait for the job: %v", r.ev.all())
				}
			})
			t.Run("a_job_that_outlasts_the_drain_wait_blocks_the_upgrade_with_running_job", func(t *testing.T) {
				rel := newRelease(t, "1.5.0", 1, 1)
				r := newRig(t, carrier, "1.4.0", rel)
				r.startJob(2 * time.Second)
				_, err := r.run(rel, "1.5.0", func(q *Request) { q.DrainWait = 100 * time.Millisecond })
				if ReasonOf(err) != ReasonRunningJob || installed(t, r.s) != "1.4.0" || r.state != "online" || r.ev.index("install") >= 0 {
					t.Fatalf("err %v version %s state %s events %v", err, installed(t, r.s), r.state, r.ev.all())
				}
			})
			t.Run("failing_health_check_restores_the_previous_binary_and_enables_the_old_version", func(t *testing.T) {
				rel := newRelease(t, "1.5.0", 1, 1)
				r := newRig(t, carrier, "1.4.0", rel)
				r.breakNewAgent()
				res, err := r.run(rel, "1.5.0", nil)
				if err == nil || !strings.HasPrefix(err.Error(), "E656:") || !res.RolledBack || installed(t, r.s) != "1.4.0" || r.state != "online" {
					t.Fatalf("res %+v err %v version %s state %s", res, err, installed(t, r.s), r.state)
				}
				if !strings.Contains(strings.Join(r.audit(), ","), "rollback") || r.ev.index("hello:1.4.0") < 0 {
					t.Fatalf("audit %v events %v", r.audit(), r.ev.all())
				}
			})
			t.Run("past_a_registered_coordinators_ceiling_is_refused_with_E666_ceiling", func(t *testing.T) {
				rel := newRelease(t, "2.0.0", 2, 2)
				r := newRig(t, carrier, "1.4.0", rel)
				if err := session.RecordWelcome(r.s.state(), session.Coordinator{ID: "c1"}, protocol.Welcome{Version: 1}, time.Now()); err != nil {
					t.Fatal(err)
				}
				_, err := r.run(rel, "2.0.0", nil)
				if installed(t, r.s) != "1.4.0" || r.state != "online" {
					t.Fatalf("version %s state %s", installed(t, r.s), r.state)
				}
				if carrier == "ssh" {
					if ReasonOf(err) != ReasonCeiling || !strings.HasPrefix(err.Error(), "E666:") {
						t.Fatalf("err %v", err)
					}
					return
				}
				// the pull agent enforces its own ceiling and refuses; the coordinator sees a hello that never changes
				if err == nil || ReasonOf(r.ctl.agentErr) != ReasonCeiling {
					t.Fatalf("err %v agent %v", err, r.ctl.agentErr)
				}
			})
			t.Run("downgrade_is_refused_without_the_flag_and_audited_with_it", func(t *testing.T) {
				rel := newRelease(t, "1.4.0", 1, 1)
				r := newRig(t, carrier, "1.5.0", rel)
				_, err := r.run(rel, "1.4.0", nil)
				if ReasonOf(err) != ReasonDowngrade || !strings.HasPrefix(err.Error(), "E666:") || installed(t, r.s) != "1.5.0" {
					t.Fatalf("err %v version %s", err, installed(t, r.s))
				}
				res, err := r.run(rel, "1.4.0", func(q *Request) { q.AllowDowngrade = true })
				if carrier == "agent" {
					// the upgrade message carries no downgrade permission: a pull agent is downgraded on the node itself
					if ReasonOf(err) != ReasonDowngrade || installed(t, r.s) != "1.5.0" || r.ev.index("install") >= 0 || r.state != "online" {
						t.Fatalf("err %v version %s events %v", err, installed(t, r.s), r.ev.all())
					}
					return
				}
				if err != nil || res.Direction != Downgrade || installed(t, r.s) != "1.4.0" {
					t.Fatalf("res %+v err %v version %s", res, err, installed(t, r.s))
				}
				if !strings.Contains(strings.Join(r.audit(), ","), "downgrade") {
					t.Fatalf("no downgrade audit row: %v", r.audit())
				}
			})
		})
	}
}

// sshOnly holds the cases that exercise the ssh install script itself.
func TestUpgradeSSHScript(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("INTEGRATION=1 runs the sim-node upgrade")
	}
	t.Run("a_job_that_starts_during_the_copy_blocks_the_swap", func(t *testing.T) {
		rel := newRelease(t, "1.5.0", 1, 1)
		r := newRig(t, "ssh", "1.4.0", rel)
		n := r.node.Node.(*SSHNode)
		n.Copy = func(ctx context.Context, tg remote.Target, local, remotePath string) error {
			err := r.s.Copy(ctx, tg, local, remotePath)
			r.s.flag("busy", "1") // another coordinator leases a job while the file travels
			return err
		}
		_, err := r.run(rel, "1.5.0", nil)
		if ReasonOf(err) != ReasonRunningJob || installed(t, r.s) != "1.4.0" || r.state != "online" {
			t.Fatalf("err %v version %s state %s", err, installed(t, r.s), r.state)
		}
		if _, serr := os.Stat(r.s.bin() + NewSuffix); serr == nil {
			t.Fatal("the staged copy was left on the node")
		}
	})
	t.Run("a_stale_prev_is_never_restored", func(t *testing.T) {
		r := newRig(t, "ssh", "1.4.0")
		n := r.node.Node.(*SSHNode)
		if _, err := n.Version(context.Background()); err != nil { // records 1.4.0 as the version before the upgrade
			t.Fatal(err)
		}
		b, _ := os.ReadFile(stubBinary(t, "1.3.0"))
		_ = os.WriteFile(r.s.bin()+PrevSuffix, b, 0o755)
		r.s.flag("bin/nself-ci-agent"+PrevVersionSuffix, "1.3.0\n")
		if err := n.Restore(context.Background()); err != nil || installed(t, r.s) != "1.4.0" {
			t.Fatalf("err %v version %s: an older .prev replaced the installed agent", err, installed(t, r.s))
		}
	})
	t.Run("the_installed_file_keeps_the_mode_job_users_need", func(t *testing.T) {
		rel := newRelease(t, "1.5.0", 1, 1)
		r := newRig(t, "ssh", "1.4.0", rel)
		if _, err := r.run(rel, "1.5.0", nil); err != nil {
			t.Fatal(err)
		}
		if st, _ := os.Stat(r.s.bin()); st.Mode().Perm() != 0o755 {
			t.Fatalf("mode %v", st.Mode().Perm())
		}
	})
}

// bannerRun wraps a node's Run so every answer starts with a login banner that mentions the script's markers.
func bannerRun(run func(context.Context, remote.Target, ...string) (string, error)) func(context.Context, remote.Target, ...string) (string, error) {
	return func(ctx context.Context, tg remote.Target, argv ...string) (string, error) {
		out, err := run(ctx, tg, argv...)
		return "Welcome. BUSY SWAPPED RESTORED NOPREV\n" + out, err
	}
}

func TestUpgradeSSHFailures(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("INTEGRATION=1 runs the sim-node upgrade")
	}
	t.Run("a_login_banner_cannot_fake_a_marker", func(t *testing.T) {
		rel := newRelease(t, "1.5.0", 1, 1)
		r := newRig(t, "ssh", "1.4.0", rel)
		n := r.node.Node.(*SSHNode)
		n.Run = bannerRun(r.s.Run)
		if _, err := r.run(rel, "1.5.0", nil); err != nil || installed(t, r.s) != "1.5.0" || r.state != "online" {
			t.Fatalf("err %v version %s state %s", err, installed(t, r.s), r.state)
		}
	})
	t.Run("an_ssh_that_drops_after_the_swap_is_rolled_back", func(t *testing.T) {
		rel := newRelease(t, "1.5.0", 1, 1)
		r := newRig(t, "ssh", "1.4.0", rel)
		n := r.node.Node.(*SSHNode)
		n.SettleWait = 200 * time.Millisecond
		n.HelloPoll = 5 * time.Millisecond
		n.Run = func(ctx context.Context, tg remote.Target, argv ...string) (string, error) {
			out, err := r.s.Run(ctx, tg, argv...)
			if err == nil && strings.Contains(out, "SWAPPED") {
				return "", errors.New("connection reset by peer") // the script finished on the node; the answer never arrived
			}
			return out, err
		}
		res, err := r.run(rel, "1.5.0", nil)
		if err == nil || !res.RolledBack || installed(t, r.s) != "1.4.0" || r.state != "online" {
			t.Fatalf("res %+v err %v version %s state %s", res, err, installed(t, r.s), r.state)
		}
	})
	t.Run("a_restore_that_cannot_move_the_file_is_an_error", func(t *testing.T) {
		r := newRig(t, "ssh", "1.4.0")
		n := r.node.Node.(*SSHNode)
		if _, err := n.Version(context.Background()); err != nil {
			t.Fatal(err)
		}
		r.s.flag("bin/nself-ci-agent"+PrevVersionSuffix, "1.4.0\n") // a marker for this version, but no .prev file
		if err := n.Restore(context.Background()); err == nil {
			t.Fatal("Restore reported success although mv failed")
		}
	})
	t.Run("a_half_copied_staged_file_never_replaces_the_agent", func(t *testing.T) {
		rel := newRelease(t, "1.5.0", 1, 1)
		r := newRig(t, "ssh", "1.4.0", rel)
		n := r.node.Node.(*SSHNode)
		n.Copy = func(ctx context.Context, tg remote.Target, local, remotePath string) error {
			b, _ := os.ReadFile(local)
			return os.WriteFile(filepath.Join(r.s.Home, remotePath), b[:len(b)/2], 0o600) // cut off
		}
		_, err := r.run(rel, "1.5.0", nil)
		if err == nil || installed(t, r.s) != "1.4.0" || r.state != "online" {
			t.Fatalf("err %v version %s state %s", err, installed(t, r.s), r.state)
		}
	})
}
