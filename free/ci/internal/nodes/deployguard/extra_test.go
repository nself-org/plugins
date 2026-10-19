package deployguard

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// A failed ssh -G or DNS lookup is not proof: a matched node stays matched, override included, until a
// full evaluation clears it.
func TestEvaluateNodeKeepsAMatchOnPartialEvidence(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	_ = r.g.Seen(ctx, r.rootA)
	r.ssh["box-alias"] = remote.Resolved{Hostname: FixtureHostV4}
	r.addNode("n1", func(c *model.Capability) {
		c.Identity.SSH = &model.CapabilitySSH{Alias: "box-alias", Hostname: "unlisted.example.test", Port: 22, User: "ci"}
		c.DeployHost = model.CapabilityDeployHost{Override: false}
	})
	if err := r.g.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if !r.deployHost("n1").Matched {
		t.Fatal("setup: not matched through ssh -G")
	}
	src := SourceInventory
	if err := r.st.NodeSetDeployHost(ctx, "n1", model.CapabilityDeployHost{Matched: true, Source: &src, Override: true}, "cli:ali", "override"); err != nil {
		t.Fatal(err)
	}
	delete(r.ssh, "box-alias") // ssh -G now fails
	res, err := r.g.EvaluateNode(ctx, "n1")
	if err != nil || !res.Matched || !res.Partial {
		t.Fatalf("%+v %v", res, err)
	}
	if dh := r.deployHost("n1"); !dh.Matched || !dh.Override {
		t.Fatalf("partial evidence cleared the match: %+v", dh)
	}
	// A complete evaluation that no longer matches clears it, override too.
	r.ssh["box-alias"] = remote.Resolved{Hostname: "elsewhere.example.test"}
	r.lookup["elsewhere.example.test"] = []netip.Addr{netip.MustParseAddr("192.0.2.9")}
	r.lookup["unlisted.example.test"] = []netip.Addr{netip.MustParseAddr("192.0.2.8")}
	if res, err = r.g.EvaluateNode(ctx, "n1"); err != nil || res.Matched || res.Partial {
		t.Fatalf("%+v %v", res, err)
	}
	if dh := r.deployHost("n1"); dh.Matched || dh.Override {
		t.Fatalf("a full evaluation did not clear: %+v", dh)
	}
	// An unmatched node with partial evidence has nothing to keep and stays unmatched.
	delete(r.ssh, "box-alias")
	if res, err = r.g.EvaluateNode(ctx, "n1"); err != nil || res.Matched || !res.Partial || r.deployHost("n1").Matched {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestResolverMarksPartialOnlyForRealLookups(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if f, _ := r.g.Resolver.Facts(ctx, Identity{Dest: "ghost.example.test"}); !f.Partial {
		t.Error("failed ssh -G and DNS did not mark Partial")
	}
	if f, _ := r.g.Resolver.Facts(ctx, Identity{Dest: "ci@" + FixtureHostV4}); f.Partial {
		t.Error("an IP literal needs no lookup")
	}
	if f, _ := r.g.Resolver.Facts(ctx, Identity{Dest: "ci@" + FixtureHostV4, Names: []string{"label"}}); f.Partial {
		t.Error("a label with no address is not partial")
	}
	r.ssh["a"] = remote.Resolved{Hostname: "h.example.test"}
	if f, _ := r.g.Resolver.Facts(ctx, Identity{Dest: "a"}); !f.Partial {
		t.Error("a resolved hostname with no address is partial")
	}
	r.ssh["same.example.test"] = remote.Resolved{Hostname: "SAME.example.test."}
	if f, _ := r.g.Resolver.Facts(ctx, Identity{Dest: "same.example.test"}); !f.Partial {
		t.Error("a hostname that ssh maps to itself still needs an address")
	}
	if res := fixtureSet().Match(Facts{Partial: true}); res.Matched || !res.Partial {
		t.Errorf("%+v", res)
	}
}

func TestLocalSetFailsClosedOnAnUnreadableSource(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	if s, err := LocalSet(ctx, home); err != nil || !s.Empty() {
		t.Fatalf("empty home: %v %v", s, err)
	}
	cfg := filepath.Join(home, "config.yaml")
	for name, text := range map[string]string{"bad yaml": "ci: [", "bad cidr": "ci:\n  never_hosts: ['203.0.113.0/33']\n"} {
		if err := os.WriteFile(cfg, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LocalSet(ctx, home); ReasonOf(err) != ReasonNoIdentity {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := os.WriteFile(cfg, []byte("ci:\n  never_hosts: ['203.0.113.0/24']\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := LocalSet(ctx, home)
	if err != nil || !s.MatchHost("203.0.113.5").Matched {
		t.Fatalf("%v %v", s.View(), err)
	}
	if err = os.Mkdir(filepath.Join(home, "state.db"), 0o700); err != nil { // exists, cannot be a database
		t.Fatal(err)
	}
	if _, err = LocalSet(ctx, home); ReasonOf(err) != ReasonNoIdentity {
		t.Fatalf("directory as state.db: %v", err)
	}
}

func TestNeverListRejectsABadCIDRAndKeepsGoodOnes(t *testing.T) {
	for _, bad := range []string{"203.0.113.0/33", "203.0.113.0/255.255.255.0", "host/24x", "203.0.113.0 /24"} {
		if _, ok, err := neverEntry(bad); err == nil || ok || ReasonOf(err) != ReasonInventory {
			t.Errorf("%q: ok=%v err=%v", bad, ok, err)
		}
	}
	for _, good := range []string{"203.0.113.0/24", "2001:db8::/32", " 10.0.0.0/8 "} {
		if e, ok, err := neverEntry(good); err != nil || !ok || e.Kind != KindPrefix {
			t.Errorf("%q: %+v %v %v", good, e, ok, err)
		}
	}
}

func TestSetNormalisesPrefixesAndZones(t *testing.T) {
	s := NewSet([]Entry{
		{KindPrefix, "::ffff:203.0.113.0/120", SourceNeverList},
		{KindHost, "198.51.100.0/24", SourceInventory},
		{KindAddress, "2001:db8:1::/48", SourceInventory},
	})
	for _, h := range []string{"203.0.113.77", "198.51.100.3", "2001:db8:1::5", "::ffff:203.0.113.9"} {
		if !s.MatchHost(h).Matched {
			t.Errorf("%s not matched", h)
		}
	}
	if s.MatchHost("203.0.114.1").Matched || s.MatchHost("2001:db8:2::1").Matched {
		t.Error("over-match")
	}
	// A resolved address that carries a zone matches the same entries.
	zoned := netip.MustParseAddr("2001:db8:1::7").WithZone("eth0")
	if !s.Match(Facts{Addrs: []netip.Addr{zoned}}).Matched {
		t.Error("zoned address missed")
	}
	if !NewSet([]Entry{{KindHost, FixtureHostV6, SourceInventory}}).Match(Facts{Addrs: []netip.Addr{netip.MustParseAddr(FixtureHostV6).WithZone("en0")}}).Matched {
		t.Error("zoned address missed an exact entry")
	}
	if got := (&Set{}); true {
		got.addPrefix(netip.MustParsePrefix("::ffff:10.0.0.0/96"), SourceInventory) // /96 of 4in6 is 0.0.0.0/0
		if v := got.View().Prefixes; len(v) != 1 || v[0] != "0.0.0.0/0" {
			t.Errorf("prefixes %v", v)
		}
	}
	if got := (&Set{}); true {
		got.addPrefix(netip.MustParsePrefix("::ffff:10.0.0.0/95"), SourceInventory) // wider than the mapped block: kept as is
		if v := got.View().Prefixes; len(v) != 1 || v[0] != "::fffe:0:0/95" {
			t.Errorf("prefixes %v", v)
		}
	}
}

func TestEmptyIsFalseWithAnySingleKind(t *testing.T) {
	if !(Set{}).Empty() {
		t.Fatal("zero set not empty")
	}
	for _, e := range []Entry{
		{KindHost, "h.example.test", ""}, {KindAddress, "10.0.0.1", ""}, {KindPrefix, "10.0.0.0/8", ""},
		{KindFingerprint, FixtureFingerprint, ""}, {KindMachineID, FixtureMachineID, ""},
	} {
		if NewSet([]Entry{e}).Empty() {
			t.Errorf("%s only: reported empty", e.Kind)
		}
	}
}

func TestCanonEdges(t *testing.T) {
	for in, want := range map[string]string{
		"@host.example.test": "host.example.test", "a@b@host.example.test": "host.example.test", "[2001:db8::1": "2001:db8::1",
		"[]": "", "[2001:db8::1]junk": "2001:db8::1", "fe80::1%": "fe80::1", "255.1.65535": "255.1.255.255",
	} {
		if got := Canon(in); got != want {
			t.Errorf("Canon(%q) = %q, want %q", in, got, want)
		}
	}
	for in, ok := range map[string]bool{
		"255.1.65535": true, "256.1.1": false, "1.2.65535": true, "1.2.65536": false, "1.16777215": true, "1.16777216": false,
		"4294967295": true, "4294967296": false, "0x7f.1": true, "1.2.3": true, "0377.0.0.1": true, "1.2.3.256": false,
	} {
		if _, got := inetAton(in); got != ok {
			t.Errorf("inetAton(%q) = %v, want %v", in, got, ok)
		}
	}
	if a, _ := inetAton("1.2.65535"); a.String() != "1.2.255.255" {
		t.Errorf("value %s", a)
	}
}

func TestFingerprintPrefixForms(t *testing.T) {
	body := FixtureFingerprint[7:]
	for in, want := range map[string]string{
		"SHA256:" + body: FixtureFingerprint, "sha256:" + body: FixtureFingerprint, body: FixtureFingerprint,
		"Sha256:" + body: "", "SHA256:": "", "sha256:": "", "": "",
	} {
		if got := NormalizeFingerprint(in); got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
}

func TestPublicKeyFingerprintNeedsTwoFields(t *testing.T) {
	fields := strings.Fields(githubKey)
	if got := PublicKeyFingerprint(fields[0] + " " + fields[1]); got != githubFP {
		t.Errorf("no comment: %q", got)
	}
}

func TestAddrsFrom(t *testing.T) {
	if _, err := addrsFrom(func() ([]net.Addr, error) { return nil, errors.New("down") }); err == nil {
		t.Fatal("error swallowed")
	}
	got, err := addrsFrom(func() ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.ParseIP("203.0.113.10"), Mask: net.CIDRMask(24, 32)}, &net.IPAddr{IP: net.ParseIP("198.51.100.1")}, &net.IPNet{IP: net.ParseIP("::ffff:192.0.2.1"), Mask: net.CIDRMask(128, 128)}}, nil
	})
	if err != nil || len(got) != 2 || got[0].String() != "203.0.113.10" || got[1].String() != "192.0.2.1" {
		t.Fatalf("%v %v", got, err)
	}
	if as, err := interfaceAddrs(); err != nil || len(as) == 0 {
		t.Fatalf("real interfaces: %v %v", as, err)
	}
}

func TestReadSmallBoundary(t *testing.T) {
	p := filepath.Join(t.TempDir(), "exact")
	if err := os.WriteFile(p, make([]byte, 1<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, err := readSmall(p); err != nil || len(b) != 1<<20 {
		t.Fatalf("exactly the cap: %v", err)
	}
}

func TestNeverListFileSizeBoundary(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	head := "ci:\n  never_hosts: [203.0.113.5]\n"
	pad := func(n int) []byte { return append([]byte(head), bytes.Repeat([]byte("#"), n-len(head))...) }
	if err := os.WriteFile(cfg, pad(1<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	if es, err := LoadNeverList(cfg); err != nil || len(es) != 1 {
		t.Fatalf("exactly the cap: %v %v", es, err)
	}
	if err := os.WriteFile(cfg, pad(1<<20+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadNeverList(cfg); err == nil {
		t.Fatal("over the cap accepted")
	}
}

func TestConfigPathDefaultsUnderTheUserHome(t *testing.T) {
	h := t.TempDir()
	t.Setenv("NSELF_CI_HOME", "")
	t.Setenv("HOME", h)
	if got := ConfigPath(""); got != filepath.Join(h, ".nself", "ci", "config.yaml") {
		t.Fatalf("%q", got)
	}
	if got := ConfigPath("/x"); got != "/x/config.yaml" {
		t.Fatalf("%q", got)
	}
}

func TestOpenRetry(t *testing.T) {
	busy := errors.New("database is locked")
	calls := 0
	open := func(failures int, err error) func(string) (*store.Store, error) {
		calls = 0
		return func(string) (*store.Store, error) {
			calls++
			if calls <= failures {
				return nil, err
			}
			return nil, nil
		}
	}
	if _, err := openRetry(open(2, busy), "p", 5*time.Second, time.Millisecond); err != nil || calls != 3 {
		t.Fatalf("busy then ok: %v after %d calls", err, calls)
	}
	if _, err := openRetry(open(1<<30, busy), "p", 30*time.Millisecond, time.Millisecond); err == nil || calls < 2 {
		t.Fatalf("always busy: %v after %d calls", err, calls)
	}
	if _, err := openRetry(open(1<<30, errors.New("corrupt")), "p", 5*time.Second, time.Millisecond); err == nil || calls != 1 {
		t.Fatalf("other error retried: %v after %d calls", err, calls)
	}
	if _, err := openRetry(open(1<<30, errors.New("SQLITE_BUSY: x")), "p", 0, time.Millisecond); err == nil || calls != 1 {
		t.Fatalf("no wait: %v after %d calls", err, calls)
	}
	if !isBusy(errors.New("SQLITE_BUSY")) || isBusy(errors.New("nope")) {
		t.Fatal("isBusy")
	}
}

func TestGuardEdges(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if (&Guard{}).actor() != "deployguard" || r.g.actor() != "cli:test" {
		t.Fatal("actor default")
	}
	// IdentityOf: the alias is the destination; with no alias the hostname is.
	c := r.capability("n", func(c *model.Capability) {
		c.Identity.SSH = &model.CapabilitySSH{Alias: "", Hostname: "h.example.test", Port: 22, User: "ci"}
	})
	if id := IdentityOf(*c); id.Dest != "h.example.test" {
		t.Errorf("dest %q", id.Dest)
	}
	c.Identity.SSH.Alias = "al"
	if id := IdentityOf(*c); id.Dest != "al" {
		t.Errorf("dest %q", id.Dest)
	}
	// A failure before any success says there is nothing cached yet.
	r.run.HelpSide = "write"
	_ = r.g.Seen(ctx, r.rootA)
	_ = r.g.Refresh(ctx)
	if w := r.g.Warnings(ctx); len(w) != 1 || !strings.Contains(w[0], "no cached entries yet") {
		t.Fatalf("warnings %v", w)
	}
	// An evaluation failure of one node is reported by Refresh and does not stop the others.
	r.run.HelpSide = "read"
	bad := store.NodeRecord{ID: "bad", Name: "bad", State: "offline", Capability: []byte(`{"not":"a capability"}`), Lifecycle: []byte(`{}`), Trust: []byte(`{}`), DeployHost: []byte(`{}`)}
	if err := r.st.NodePut(ctx, bad, "operator"); err != nil {
		t.Fatal(err)
	}
	r.addNode("good", ssh(FixtureHostV4))
	err := r.g.Refresh(ctx)
	if err == nil || !strings.Contains(err.Error(), "node bad") || !r.deployHost("good").Matched {
		t.Fatalf("refresh %v, good matched %v", err, r.deployHost("good").Matched)
	}
}

func TestRunReportsRefreshErrorsOnlyToACallback(t *testing.T) {
	r := newRig(t)
	r.run.HelpSide = "write"
	_ = r.g.Seen(context.Background(), r.rootA)
	for _, cb := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // one refresh, then stop
		var got []error
		var on func(error)
		if cb {
			on = func(e error) { got = append(got, e) }
		}
		r.g.Run(ctx, on) // must not panic with a nil callback
		if cb && len(got) != 1 {
			t.Fatalf("callback got %d errors", len(got))
		}
	}
}

func TestTerminalConfirmNeverAcceptsAnEmptyWant(t *testing.T) {
	var out bytes.Buffer
	if ok, _ := TerminalConfirm(strings.NewReader("\n"), &out, true)("p", ""); ok {
		t.Fatal("empty want confirmed")
	}
}
