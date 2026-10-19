package discovery

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

// TestNoSocketUnlessAsked: with the default config nothing touches the network until a command calls
// Browse or Advertise; with discovery off not even then, and the refusal is E667 with a reason key.
func TestNoSocketUnlessAsked(t *testing.T) {
	n := newMemNet()
	g := guardFor(n, nil, t.TempDir())
	if n.callCount() != 0 {
		t.Fatal("building a Guard touched the network")
	}
	if err := g.Allowed(); err != nil || n.callCount() != 0 {
		t.Fatalf("Allowed must be a pure check: %v, %d calls", err, n.callCount())
	}
	if _, err := g.Browse(context.Background(), time.Second); err != nil || n.browses != 1 {
		t.Fatalf("an explicit browse must run: %v", err)
	}

	cases := []struct {
		name, reason string
		env          map[string]string
		config       string
	}{
		{"env zero", ReasonEnvOff, map[string]string{"NSELF_CI_DISCOVERY": "0"}, ""},
		{"env off", ReasonEnvOff, map[string]string{"NSELF_CI_DISCOVERY": "off"}, ""},
		{"nested off", ReasonConfigOff, nil, "ci:\n  discovery: off\n"},
		{"nested false", ReasonConfigOff, nil, "ci:\n  discovery: false\n"},
		{"flat off", ReasonConfigOff, nil, "ci.discovery: off\n"},
		{"quoted no", ReasonConfigOff, nil, "ci:\n  discovery: \"no\"\n"},
		{"not yaml", ReasonConfigInvalid, nil, "ci: [unclosed\n"},
		{"mapping value", ReasonConfigInvalid, nil, "ci:\n  discovery: {enabled: false}\n"},
		{"typo value", ReasonConfigInvalid, nil, "ci:\n  discovery: of\n"},
		{"list value", ReasonConfigInvalid, nil, "ci.discovery: [a]\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			off := newMemNet()
			home := t.TempDir()
			if c.config != "" {
				home = writeConfig(t, c.config)
			}
			g := guardFor(off, c.env, home)
			if _, err := g.Browse(context.Background(), time.Second); ReasonOf(err) != c.reason {
				t.Fatalf("Browse: want E667 %s, got %v", c.reason, err)
			}
			if _, err := g.Advertise(context.Background(), time.Minute, SSHPort, nil); ReasonOf(err) != c.reason {
				t.Fatalf("Advertise: want E667 %s, got %v", c.reason, err)
			}
			if off.callCount() != 0 {
				t.Fatalf("%d network calls while discovery is off", off.callCount())
			}
			if e := (&Error{}); !strings.Contains(g.Allowed().Error(), "E667") || e.ErrCode() != "E667" {
				t.Fatal("the refusal must carry E667")
			}
		})
	}
}

func TestConfigOnAndUnrelatedKeysStayOn(t *testing.T) {
	for _, body := range []string{"ci:\n  discovery: on\n", "ci:\n  never_hosts: [a]\n", "other: 1\n", ""} {
		g := guardFor(newMemNet(), nil, writeConfig(t, body))
		if err := g.Allowed(); err != nil {
			t.Fatalf("%q must leave discovery on: %v", body, err)
		}
	}
}

func TestAdvertiseWindowPrivacyAndStop(t *testing.T) {
	n := newMemNet()
	end := make(chan time.Time)
	g := guardFor(n, nil, t.TempDir())
	g.After = func(time.Duration) <-chan time.Time { return end }
	a, err := g.Advertise(context.Background(), time.Minute, SSHPort, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc := n.advertised[0]
	if !regexp.MustCompile(`^[0-9a-f]{8}$`).MatchString(a.WindowID) || len(svc.TXT) != 1 || svc.TXT[0] != a.WindowID {
		t.Fatalf("the TXT must be the window id only: %q / %v", a.WindowID, svc.TXT)
	}
	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(svc.Instance) || !regexp.MustCompile(`^[0-9a-f]{16}\.local\.$`).MatchString(svc.HostName) {
		t.Fatalf("instance and host must be random hex: %q %q", svc.Instance, svc.HostName)
	}
	host, _ := os.Hostname()
	for _, v := range []string{svc.Instance, svc.HostName, svc.TXT[0]} {
		if host != "" && len(host) > 3 && strings.Contains(strings.ToLower(v), strings.ToLower(host)) {
			t.Fatalf("%q carries the host name", v)
		}
	}
	if svc.Type != ServiceType || svc.Port != SSHPort || len(svc.IPs) == 0 {
		t.Fatalf("unexpected service %+v", svc)
	}
	select {
	case <-a.Done():
		t.Fatal("stopped before the window ended")
	default:
	}
	end <- time.Now() // the window ends
	select {
	case <-a.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("advertisement did not stop at window end")
	}
	a.Stop() // idempotent
	if n.stopCount() != 1 {
		t.Fatalf("the responder must be stopped exactly once, got %d", n.stopCount())
	}
}

func TestAdvertiseFreshIdentityPerWindow(t *testing.T) {
	n := newMemNet()
	g := guardFor(n, nil, t.TempDir())
	seen := map[string]bool{}
	for range 20 {
		a, err := g.Advertise(context.Background(), time.Minute, SSHPort, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range []string{a.WindowID, a.Instance, a.HostName} {
			if seen[v] {
				t.Fatalf("%q repeated across windows", v)
			}
			seen[v] = true
		}
		a.Stop()
	}
	if n.stopCount() != 20 {
		t.Fatalf("every window must stop its responder, got %d", n.stopCount())
	}
}

func TestAdvertiseStopsWhenContextEnds(t *testing.T) {
	n := newMemNet()
	ctx, cancel := context.WithCancel(context.Background())
	a, err := guardFor(n, nil, t.TempDir()).Advertise(ctx, time.Minute, SSHPort, nil)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-a.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("cancelling the context must end the advertisement")
	}
}

func TestAdvertiseRealWindowEnd(t *testing.T) {
	n := newMemNet()
	a, err := guardFor(n, nil, t.TempDir()).Advertise(context.Background(), 30*time.Millisecond, SSHPort, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-a.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("a 30 ms window must end by itself")
	}
}

func TestAdvertiseRejectsBadInput(t *testing.T) {
	for name, f := range map[string]func(g *Guard) error{
		"no window": func(g *Guard) error { _, err := g.Advertise(context.Background(), 0, SSHPort, nil); return err },
		"too long": func(g *Guard) error {
			_, err := g.Advertise(context.Background(), MaxWindow+1, SSHPort, nil)
			return err
		},
		"bad port": func(g *Guard) error { _, err := g.Advertise(context.Background(), time.Minute, 0, nil); return err },
		"no addresses": func(g *Guard) error {
			g.Net.(*memNet).local = nil
			_, err := g.Advertise(context.Background(), time.Minute, 22, nil)
			return err
		},
	} {
		n := newMemNet()
		if err := f(guardFor(n, nil, t.TempDir())); err == nil {
			t.Fatalf("%s: want an error", name)
		}
		if len(n.advertised) != 0 {
			t.Fatalf("%s: a refused advertisement must not start a responder", name)
		}
	}
}

func TestBrowseTimeoutBounds(t *testing.T) {
	n := newMemNet()
	g := guardFor(n, nil, t.TempDir())
	for _, d := range []time.Duration{0, time.Millisecond, 2 * time.Minute, 30 * time.Second} {
		if _, err := g.Browse(context.Background(), d); err == nil {
			t.Fatalf("timeout %s must be refused", d)
		}
	}
	if n.callCount() != 0 {
		t.Fatal("a refused timeout must not open a socket")
	}
}

// TestUnknownConfigLocationFailsClosed: with no home at all the switch cannot be read, so discovery stays off.
func TestUnknownConfigLocationFailsClosed(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("NSELF_CI_HOME", "")
	n := newMemNet()
	g := &Guard{Net: n, Getenv: func(string) string { return "" }}
	if _, err := g.Browse(context.Background(), time.Second); ReasonOf(err) != ReasonConfigInvalid || n.callCount() != 0 {
		t.Fatalf("want E667 config_invalid and no network call, got %v / %d", err, n.callCount())
	}
}

func TestConfigThatIsNotARegularFileFailsClosed(t *testing.T) {
	home := t.TempDir()
	if err := os.Symlink("/dev/zero", filepath.Join(home, "config.yaml")); err != nil {
		t.Skip(err)
	}
	if err := guardFor(newMemNet(), nil, home).Allowed(); ReasonOf(err) != ReasonConfigInvalid {
		t.Fatalf("a device as config must be refused, got %v", err)
	}
	big := writeConfig(t, "ci:\n  discovery: on\n"+strings.Repeat("# pad\n", 200000))
	if err := guardFor(newMemNet(), nil, big).Allowed(); ReasonOf(err) != ReasonConfigInvalid {
		t.Fatalf("an oversized config must be refused, got %v", err)
	}
}
