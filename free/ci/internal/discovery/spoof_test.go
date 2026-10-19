package discovery

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func addr(s string) []netip.Addr { return []netip.Addr{netip.MustParseAddr(s)} }

// TestSpoofedRecordsAreOnlyCandidates: whatever a host on the LAN publishes comes back as a row in state
// discovered, never authorized, with terminal-safe strings; two responders claiming one name are both
// listed.
func TestSpoofedRecordsAreOnlyCandidates(t *testing.T) {
	n := newMemNet()
	n.injected = []Entry{
		{Name: "twin._nself-ci._tcp.local.", Host: "a.local.", Addrs: addr("192.168.1.30"), Port: 22, TXT: []string{"deadbeef"}},
		{Name: "twin._nself-ci._tcp.local.", Host: "b.local.", Addrs: addr("192.168.1.31"), Port: 22, TXT: []string{"deadbeef"}},
		{Name: "twin._nself-ci._tcp.local.", Host: "b.local.", Addrs: addr("192.168.1.31"), Port: 22, TXT: []string{"deadbeef"}}, // an exact repeat
		{Name: "evil\x1b[2J\r\n._nself-ci._tcp.local.", Host: "h\x07ost.local.", Addrs: addr("10.0.0.9"), Port: 70000, TXT: []string{"ADMIN", "not-an-id", "0123abcd"}},
		{Name: "mcast._nself-ci._tcp.local.", Host: "m.local.", Addrs: append(addr("224.0.0.1"), netip.MustParseAddr("127.0.0.1"), netip.IPv4Unspecified()), Port: 22},
	}
	cs, err := guardFor(n, nil, t.TempDir()).Browse(context.Background(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 4 {
		t.Fatalf("want 4 candidates (two twins, evil, mcast), got %d: %+v", len(cs), cs)
	}
	twins := 0
	for _, c := range cs {
		if c.State != StateDiscovered || c.Authorized {
			t.Fatalf("a candidate must be discovered and unauthorized: %+v", c)
		}
		if c.Instance == "twin" {
			twins++
		}
		for _, s := range append([]string{c.Instance, c.Host, c.WindowID}, c.Addresses...) {
			for _, r := range s {
				if r < 0x20 || r == 0x7f || r == 0x1b {
					t.Fatalf("control character in %q", s)
				}
			}
		}
		if strings.HasPrefix(c.Instance, "evil") {
			if c.WindowID != "0123abcd" || c.Port != 0 || c.Instance != "evil??2J??" || c.Host != "h?ost.local" {
				t.Fatalf("spoofed fields not cleaned: %+v", c)
			}
		}
		if c.Instance == "mcast" && len(c.Addresses) != 0 {
			t.Fatalf("multicast, loopback and unspecified addresses must be dropped: %v", c.Addresses)
		}
	}
	if twins != 2 {
		t.Fatalf("two responders claiming one name must both be listed, got %d", twins)
	}
	b, _ := json.Marshal(cs[0])
	if !strings.Contains(string(b), `"state":"discovered"`) || !strings.Contains(string(b), `"authorized":false`) {
		t.Fatalf("--json must show discovered and authorized false: %s", b)
	}
}

func TestCandidateListIsCapped(t *testing.T) {
	n := newMemNet()
	for i := range 3 * MaxCandidates {
		n.injected = append(n.injected, Entry{Name: "n" + strconv.Itoa(i) + "._nself-ci._tcp.local.", Addrs: addr("192.168.1.2"), Port: 22})
	}
	cs, err := guardFor(n, nil, t.TempDir()).Browse(context.Background(), time.Second)
	if err != nil || len(cs) != MaxCandidates {
		t.Fatalf("want %d candidates, got %d (%v)", MaxCandidates, len(cs), err)
	}
}

func goFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "vendor" || d.Name() == "testdata") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(p, ".go") {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func parse(t *testing.T, path string) *ast.File {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// TestImportsCannotAuthorize: no code path of this package can authorize or pair a node from a record:
// its non-test files import only the standard library, the mDNS library, yaml, the error-code registry
// and the envelope writer. The registry, store, pairing and PKI packages are all absent.
func TestImportsCannotAuthorize(t *testing.T) {
	allowed := map[string]bool{
		"github.com/hashicorp/mdns":                           true,
		"gopkg.in/yaml.v3":                                    true,
		"github.com/nself-org/plugins/free/ci/internal/model": true,
		"github.com/nself-org/cli/sdk/go/v2/output":           true,
	}
	files, _ := filepath.Glob("*.go")
	for _, p := range files {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		for _, im := range parse(t, p).Imports {
			path, _ := strconv.Unquote(im.Path.Value)
			if !strings.Contains(strings.SplitN(path, "/", 2)[0], ".") { // standard library
				continue
			}
			if !allowed[path] {
				t.Errorf("%s imports %s: discovery must not reach anything that authorizes or pairs a node", p, path)
			}
		}
	}
}

// TestOnlyMDNSNetImportsTheLibrary: the hashicorp/mdns library is imported by exactly one file, and the
// Net methods that open sockets are called from exactly one file, the Guard.
func TestOnlyMDNSNetImportsTheLibrary(t *testing.T) {
	var all []string
	for _, dir := range []string{"internal", "cmd", "test", "architecture"} {
		if _, err := os.Stat(filepath.Join("..", "..", dir)); err == nil {
			all = append(all, goFiles(t, filepath.Join("..", "..", dir))...)
		}
	}
	for _, p := range all { // tools/deps pins the module for go mod vendor and is not a call site
		for _, im := range parse(t, p).Imports {
			if strings.Contains(im.Path.Value, "hashicorp/mdns") && filepath.ToSlash(p) != "../../internal/discovery/mdnsnet.go" && !strings.HasSuffix(p, "_test.go") {
				t.Errorf("%s imports the mDNS library outside the entry-point guard's socket layer", p)
			}
		}
	}
	files, _ := filepath.Glob("*.go")
	for _, p := range files {
		if strings.HasSuffix(p, "_test.go") || p == "guard.go" {
			continue
		}
		src, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, call := range []string{".Browse(", ".Advertise(", ".LocalAddrs("} {
			if strings.Contains(string(src), call) && p != "mdnsnet.go" && p != "cli.go" {
				t.Errorf("%s calls %s: only guard.go may reach the network layer", p, call)
			}
		}
	}
}
