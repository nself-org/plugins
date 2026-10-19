package discovery

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func runCmd(t *testing.T, n *memNet, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := RunDiscover(args, Env{Out: &out, Err: &errb, Guard: guardFor(n, env, t.TempDir())})
	return code, out.String(), errb.String()
}

func TestDiscoverTableAndJSON(t *testing.T) {
	n := newMemNet()
	n.injected = []Entry{{Name: "abc._nself-ci._tcp.local.", Host: "x.local.", Addrs: addr("192.168.1.40"), Port: 22, TXT: []string{"0a1b2c3d"}}}
	code, out, errs := runCmd(t, n, nil, "--timeout", "1s")
	if code != 0 || errs != "" {
		t.Fatalf("exit %d, stderr %q", code, errs)
	}
	for _, want := range []string{"0a1b2c3d", "192.168.1.40", "abc", "discovered", "not trusted and not authorized"} {
		if !strings.Contains(out, want) {
			t.Errorf("table lacks %q:\n%s", want, out)
		}
	}
	code, out, _ = runCmd(t, n, nil, "--json")
	var env struct {
		SchemaVersion string
		Command       string `json:"command"`
		Data          DiscoverData
	}
	if code != 0 || json.Unmarshal([]byte(out), &env) != nil || env.Command != "ci nodes discover" || len(env.Data.Candidates) != 1 || env.Data.Truncated != 0 || env.Data.Candidates[0].State != "discovered" || env.Data.Candidates[0].Authorized {
		t.Fatalf("bad envelope (exit %d): %s", code, out)
	}
}

func TestDiscoverNoCandidatesIsNotAFailure(t *testing.T) {
	code, out, _ := runCmd(t, newMemNet(), nil)
	if code != 0 || !strings.Contains(out, "no candidates") {
		t.Fatalf("exit %d: %s", code, out)
	}
	code, out, _ = runCmd(t, newMemNet(), nil, "--json")
	if code != 0 || !strings.Contains(out, `"candidates": []`) && !strings.Contains(out, `"candidates":[]`) {
		t.Fatalf("an empty result must be an empty list, not null: %s", out)
	}
}

func TestDiscoverDisabledIsE667WithoutANetwork(t *testing.T) {
	n := newMemNet()
	code, _, errs := runCmd(t, n, map[string]string{"NSELF_CI_DISCOVERY": "0"})
	if code != 1 || !strings.Contains(errs, "E667") || n.callCount() != 0 {
		t.Fatalf("exit %d, stderr %q, %d network calls", code, errs, n.callCount())
	}
	code, out, _ := runCmd(t, n, map[string]string{"NSELF_CI_DISCOVERY": "0"}, "--json")
	if code != 1 || !strings.Contains(out, `"E667"`) || n.callCount() != 0 {
		t.Fatalf("json: exit %d: %s", code, out)
	}
}

func TestDiscoverUsageAndNetworkFailure(t *testing.T) {
	n := newMemNet()
	if code, _, _ := runCmd(t, n, nil, "extra"); code != 1 {
		t.Fatalf("a positional argument is a usage error, exit %d", code)
	}
	if code, _, _ := runCmd(t, n, nil, "--bogus"); code != 1 {
		t.Fatalf("an unknown flag is a usage error, exit %d", code)
	}
	if code, _, _ := runCmd(t, n, nil, "--timeout", "1ms"); code != 1 || n.callCount() != 0 {
		t.Fatalf("an out-of-range timeout is refused before any socket, exit %d", code)
	}
	n.browseErr = errors.New("no multicast listeners could be started")
	code, _, errs := runCmd(t, n, nil)
	if code != 2 || !strings.Contains(errs, "browse failed") {
		t.Fatalf("a socket failure is exit 2, got %d %q", code, errs)
	}
}

func TestStripAdvertiseFlag(t *testing.T) {
	cases := []struct {
		in, rest []string
		on       bool
	}{
		{nil, []string{}, false},
		{[]string{"--advertise", "https://c:3866"}, []string{"https://c:3866"}, true},
		{[]string{"--name", "n1", "-advertise", "u"}, []string{"--name", "n1", "u"}, true},
		{[]string{"--advertise=true", "u"}, []string{"u"}, true},
		{[]string{"--advertise", "--advertise=false", "u"}, []string{"u"}, false},
		{[]string{"--expect-fingerprint", "SHA256:x", "u"}, []string{"--expect-fingerprint", "SHA256:x", "u"}, false},
		{[]string{"u", "--", "--advertise"}, []string{"u", "--", "--advertise"}, false},
		{[]string{"--advertise=0", "u"}, []string{"u"}, false},
		{[]string{"-advertise=1", "u"}, []string{"u"}, true},
		{[]string{"--advertise=maybe", "u"}, []string{"--advertise=maybe", "u"}, false},
		{[]string{"--advertiser", "u"}, []string{"--advertiser", "u"}, false},
	}
	for _, c := range cases {
		rest, on := StripAdvertiseFlag(c.in)
		if on != c.on || !slices.Equal(rest, c.rest) {
			t.Errorf("%v: got %v %v, want %v %v", c.in, rest, on, c.rest, c.on)
		}
	}
}

func TestDiscoverTableWarnsOnDuplicatesAndCuts(t *testing.T) {
	n := newMemNet()
	n.injected = []Entry{
		{Name: "a._nself-ci._tcp.local.", Addrs: addr("192.168.1.9"), Port: 22, TXT: []string{"0a1b2c3d"}},
		{Name: "b._nself-ci._tcp.local.", Addrs: addr("192.168.1.66"), Port: 22, TXT: []string{"0a1b2c3d"}},
	}
	for i := range MaxCandidates {
		n.injected = append(n.injected, Entry{Name: "j" + strconv.Itoa(i) + "._nself-ci._tcp.local.", Addrs: addr("192.168.1.2"), Port: 22})
	}
	_, out, _ := runCmd(t, n, nil)
	for _, want := range []string{"(duplicate id)", "Do not pair until", "more candidate(s) not shown"} {
		if !strings.Contains(out, want) {
			t.Errorf("table lacks %q:\n%s", want, out)
		}
	}
}

func TestDiscoverJSONFlagErrorStillEmitsAnEnvelope(t *testing.T) {
	code, out, _ := runCmd(t, newMemNet(), nil, "--json", "--timeout", "abc")
	if code != 1 || !strings.Contains(out, `"E607"`) {
		t.Fatalf("exit %d: %q", code, out)
	}
}
