//go:build tart

package tart

// Real-Tart evidence (P7-TRUST-06). Run on an Apple-silicon Mac with Tart and softnet installed:
//
//	NSELF_TEST_TART_IMAGE=<ref>@sha256:<digest> NSELF_TEST_TART_SSH_KEY=<identity file the image trusts> \
//	NSELF_TEST_LAN_ADDR=<an address on this Mac's LAN> NSELF_TEST_EXCLUDED_ADDR=<fixture exclusion address> \
//	go test -mod=vendor -count=1 -v -tags tart -timeout 30m ./internal/isolation/tart/...
//
// Every missing prerequisite is t.Fatal, never t.Skip: a tagged run that cannot prove the isolation fails.

import (
	"context"
	"os"
	osexec "os/exec"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/netscope"
)

func need(t *testing.T, name string) string {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		t.Fatalf("prerequisite missing: set %s", name)
	}
	return v
}

func realProfile(t *testing.T, withProxy bool, excluded string) *Profile {
	return realProfileIn(t, withProxy, excluded, t.TempDir())
}

func realProfileIn(t *testing.T, withProxy bool, excluded, home string) *Profile {
	t.Helper()
	if _, err := osexec.LookPath("tart"); err != nil {
		t.Fatal("prerequisite missing: tart is not installed")
	}
	image, key := need(t, "NSELF_TEST_TART_IMAGE"), need(t, "NSELF_TEST_TART_SSH_KEY")
	net := NetSource{Facts: BuildFacts(netscope.Discover, func(context.Context) (netscope.Exclusions, error) {
		if excluded == "" {
			return netscope.Exclusions{}, nil
		}
		return netscope.Exclusions{Addresses: []string{excluded}}, nil
	}, nil)}
	if withProxy {
		net.Proxy = HostProxy(nil, nil, nil)
	}
	return New("vm", Options{BaseImage: func() string { return image }, SSHKey: func() string { return key }, Home: func() string { return home }, Net: net})
}

func leftovers(t *testing.T) string {
	t.Helper()
	out, err := osexec.Command("tart", "list", "--quiet").CombinedOutput()
	if err != nil {
		t.Fatalf("tart list: %v %s", err, out)
	}
	var left []string
	for _, l := range strings.Fields(string(out)) {
		if strings.HasPrefix(l, ClonePrefix) {
			left = append(left, l)
		}
	}
	return strings.Join(left, " ")
}

func runReal(t *testing.T, p *Profile, id string, scope model.NetworkScope, script string) (exec.AttemptResult, string) {
	t.Helper()
	var out strings.Builder
	ex := exec.Executor{Isolation: isolator{p}}
	res := ex.Run(context.Background(), exec.JobSpec{
		AttemptID: id, CoordinatorID: "tart-it", Home: t.TempDir(), Network: scope, Timeout: 20 * time.Minute,
		Job: model.Job{Kind: "run", Isolation: "vm"}, Command: []string{"/bin/sh", "-c", script},
		Output: func(b []byte) { out.Write(b) },
	})
	return res, out.String()
}

func TestRealCloneRunsAndIsDeleted(t *testing.T) {
	p := realProfile(t, false, "")
	res, out := runReal(t, p, "it-1", "lan", "sw_vers -productName; echo job-ran")
	if res.Err != nil || !strings.Contains(out, "job-ran") {
		t.Fatalf("%+v\n%s", res, out)
	}
	if left := leftovers(t); left != "" {
		t.Fatalf("clones left: %s", left)
	}
}

func TestRealRestrictedCannotReachLANHostOrExcludedAddress(t *testing.T) {
	lan, excluded := need(t, "NSELF_TEST_LAN_ADDR"), need(t, "NSELF_TEST_EXCLUDED_ADDR")
	p := realProfile(t, true, excluded)
	script := `
for a in ` + lan + ` ` + excluded + `; do
  if ping -c 1 -t 3 "$a" >/dev/null 2>&1; then echo "REACHED $a"; fi
done
code=$(curl -sS -o /dev/null -w '%{http_code}' --max-time 30 https://registry.npmjs.org/ || true)
echo "registry=$code"
code=$(curl -sS -o /dev/null -w '%{http_code}' --max-time 30 https://example.com/ || true)
echo "outside=$code"
`
	res, out := runReal(t, p, "it-2", "restricted", script)
	if res.Err != nil {
		t.Fatalf("%v\n%s", res.Err, out)
	}
	if strings.Contains(out, "REACHED") {
		t.Fatalf("a forbidden address answered:\n%s", out)
	}
	if !strings.Contains(out, "registry=200") || strings.Contains(out, "outside=200") {
		t.Fatalf("allowlist: the registry must answer and a public host outside it must not:\n%s", out)
	}
	if left := leftovers(t); left != "" {
		t.Fatalf("clones left: %s", left)
	}
}

func TestRealRestrictedWithoutProxyIsUnavailable(t *testing.T) {
	p := realProfile(t, false, "")
	res, _ := runReal(t, p, "it-3", "restricted", "true")
	if res.Err == nil || !strings.Contains(res.Err.Error(), "E679") {
		t.Fatalf("want E679, got %+v", res)
	}
	if left := leftovers(t); left != "" {
		t.Fatalf("clones left: %s", left)
	}
}

func TestRealKillNineThenSweep(t *testing.T) {
	if os.Getenv("TART_REAL_HELPER") == "1" {
		p := realProfileIn(t, false, "", os.Getenv("TART_REAL_HOME"))
		runReal(t, p, "it-4", "lan", "sleep 300")
		return
	}
	need(t, "NSELF_TEST_TART_IMAGE")
	cmd := osexec.Command(os.Args[0], "-test.run=^TestRealKillNineThenSweep$")
	home := t.TempDir()
	cmd.Env = append(os.Environ(), "TART_REAL_HELPER=1", "TART_REAL_HOME="+home)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Minute) // clone, boot, ssh ready
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	p := realProfileIn(t, false, "", home)
	if err := p.Prepare(context.Background(), isolation.Attempt{ID: "it-5", CoordinatorID: "tart-it-next"}); err != nil {
		t.Fatal(err)
	}
	if left := leftovers(t); left != "" {
		t.Fatalf("the sweep left: %s", left)
	}
}
