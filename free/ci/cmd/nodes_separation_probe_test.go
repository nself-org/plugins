package main

// Purpose: the system-node separation proof (D-1230): uid_separation is true only when the job chain
// ran as nself-ci-job-1 AND the job user got EACCES on the node key; every single failure is false
// with a named reason, and systemInstall records exactly what the probe proved.
// The probe forces the C locale, needs cat's exit code 1 as well as its text, and quotes its request.
// Inputs:  a fake node runner that answers the two probe scripts; sh for the quoting check.
// Constraints: no ssh, no sudo.

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

type probeAnswer struct {
	out string
	err error
}

const eacces = "cat: /var/lib/nself-ci-agent/node.key: Permission denied"

// probeRunner answers the chain probe (id -un) and the key probe (cat) and records the scripts.
func probeRunner(chain, key probeAnswer, scripts *[]string) func(context.Context, ...string) (string, error) {
	return func(_ context.Context, argv ...string) (string, error) {
		if len(argv) != 3 || argv[0] != "sh" || argv[1] != "-c" {
			return "", errors.New("unexpected argv")
		}
		if scripts != nil {
			*scripts = append(*scripts, argv[2])
		}
		switch {
		case strings.Contains(argv[2], `"/usr/bin/id"`):
			return chain.out, chain.err
		case strings.Contains(argv[2], `"/bin/cat"`):
			return key.out, key.err
		}
		return "", nil
	}
}

var (
	chainOK = probeAnswer{out: "rc=0\nnself-ci-job-1\n"}
	keyOK   = probeAnswer{out: "rc=1\n" + eacces + "\n"}
)

func TestSeparationProbeNeedsBothProofs(t *testing.T) {
	var scripts []string
	got := probeSeparation(context.Background(), probeRunner(chainOK, keyOK, &scripts), "linux")
	if !got.OK || got.Reason != shimProbeOK {
		t.Fatalf("both proofs: %+v", got)
	}
	if len(scripts) != 2 {
		t.Fatalf("want the chain and the key probe, got %d scripts", len(scripts))
	}
	chain := "| sudo -n -u nself-ci sudo -n -u nself-ci-job-1 /opt/nself-ci-agent/bin/nself-ci-agent job-shim 2>&1"
	for _, s := range scripts {
		if !strings.Contains(s, chain) || strings.ContainsRune(s, '\\') {
			t.Fatalf("probe does not run the job chain exactly (or holds a backslash):\n%s", s)
		}
	}
	if !strings.Contains(scripts[1], `"argv":["/bin/cat","/var/lib/nself-ci-agent/node.key"]`) {
		t.Fatalf("key probe reads the wrong path:\n%s", scripts[1])
	}
}

func TestSeparationProbeEachFailureIsFalseWithAReason(t *testing.T) {
	cases := []struct {
		name       string
		chain, key probeAnswer
		reason     string
	}{
		{"sudo refused", probeAnswer{out: "rc=1\nsudo: a password is required\n"}, keyOK, shimProbeChainFailed},
		{"shim refused", probeAnswer{out: "rc=1\nerror: job-shim runs only as a job user\n"}, keyOK, shimProbeChainFailed},
		{"ran as the agent", probeAnswer{out: "rc=0\nnself-ci\n"}, keyOK, shimProbeWrongUser},
		{"ran as root", probeAnswer{out: "rc=0\nroot\n"}, keyOK, shimProbeWrongUser},
		{"chain transport error", probeAnswer{err: errors.New("ssh: no route")}, keyOK, shimProbeUnreadable},
		{"chain no rc line", probeAnswer{out: "nself-ci-job-1\n"}, keyOK, shimProbeUnreadable},
		{"key readable", chainOK, probeAnswer{out: "rc=0\n-----BEGIN KEY-----\n"}, shimProbeKeyReadable},
		{"key ENOENT", chainOK, probeAnswer{out: "rc=1\ncat: /var/lib/nself-ci-agent/node.key: No such file or directory\n"}, shimProbeKeyNoEACCES},
		{"key failed otherwise", chainOK, probeAnswer{out: "rc=1\nsudo: a password is required\n"}, shimProbeKeyNoEACCES},
		{"key EACCES text with the shim's exit code", chainOK, probeAnswer{out: "rc=2\n" + eacces + "\n"}, shimProbeKeyNoEACCES},
		{"key EACCES text, rc 126", chainOK, probeAnswer{out: "rc=126\n" + eacces + "\n"}, shimProbeKeyNoEACCES},
		{"key EACCES on another path", chainOK, probeAnswer{out: "rc=1\ncat: /tmp/x: Permission denied\n"}, shimProbeKeyNoEACCES},
		{"key transport error", chainOK, probeAnswer{err: errors.New("ssh: broken pipe")}, shimProbeUnreadable},
		{"empty node", probeAnswer{}, probeAnswer{}, shimProbeUnreadable},
	}
	for _, c := range cases {
		got := probeSeparation(context.Background(), probeRunner(c.chain, c.key, nil), "linux")
		if got.OK || got.Reason != c.reason {
			t.Errorf("%s: got %+v, want false with %s", c.name, got, c.reason)
		}
	}
}

func TestBusyBoxEACCESCounts(t *testing.T) {
	key := probeAnswer{out: "rc=1\ncat: can't open '/var/lib/nself-ci-agent/node.key': Permission denied\n"}
	if got := probeSeparation(context.Background(), probeRunner(chainOK, key, nil), "linux"); !got.OK {
		t.Fatalf("busybox EACCES: %+v", got)
	}
	mac := probeAnswer{out: "rc=1\ncat: /var/db/nself-ci-agent/node.key: Permission denied\n"}
	if got := probeSeparation(context.Background(), probeRunner(chainOK, mac, nil), "darwin"); !got.OK {
		t.Fatalf("macOS EACCES: %+v", got)
	}
	if got := probeSeparation(context.Background(), probeRunner(chainOK, keyOK, nil), "darwin"); got.OK {
		t.Fatal("a Linux key path proved a macOS node")
	}
}

func TestSeparationProbeForcesTheCLocale(t *testing.T) {
	script := shimProbeScript([]string{"/bin/cat", "/x"})
	if !strings.HasPrefix(script, "set -u\nLC_ALL=C\nLANG=C\nexport LC_ALL LANG\n") {
		t.Fatalf("the probe does not force the C locale before sudo:\n%s", script)
	}
	if !strings.Contains(script, `"env":["LC_ALL=C","LANG=C","PATH=/usr/bin:/bin"]`) {
		t.Fatalf("the job's env does not carry the C locale:\n%s", script)
	}
}

func TestSeparationProbeQuotesTheRequest(t *testing.T) {
	for _, s := range []string{`{"argv":["it's"]}`, "''", "a'; exit 7; echo '", `$(id) ` + "`id`"} {
		q := shQuote(s)
		if strings.ContainsRune(q, '\\') {
			t.Fatalf("quoting %q used a backslash: %s", s, q)
		}
		out, err := exec.Command("sh", "-c", "printf %s "+q).Output()
		if err != nil || string(out) != s {
			t.Fatalf("sh read %q back as %q (%v)", s, out, err)
		}
	}
	script := shimProbeScript([]string{"/bin/echo", "it's"})
	if !strings.Contains(script, `"argv":["/bin/echo","it'"'"'s"]`) {
		t.Fatalf("a quote in the request is not closed and reopened:\n%s", script)
	}
}

type userns struct{}

func (userns) Detect(context.Context, func(context.Context, ...string) (string, error), lifecycle.Platform) lifecycle.SeparationFacts {
	return lifecycle.SeparationFacts{UIDSeparation: true, UserNS: true, Reason: "userns.ok"}
}

func TestShimSeparationRecordsOnlyTheProof(t *testing.T) {
	ctx := context.Background()
	failed := shimSeparation{inner: userns{}, result: probeResult{Reason: shimProbeChainFailed}}
	if f := failed.Detect(ctx, nil, lifecycle.Platform{OS: "linux"}); f.UIDSeparation || !f.UserNS || f.Reason != shimProbeChainFailed {
		t.Fatalf("a failed probe on a userns node: %+v", f)
	}
	proven := shimSeparation{inner: bareSeparation{}, result: probeResult{OK: true, Reason: shimProbeOK}}
	if f := proven.Detect(ctx, nil, lifecycle.Platform{OS: "linux"}); !f.UIDSeparation || f.UserNS || f.Reason != shimProbeOK {
		t.Fatalf("a proven node: %+v", f)
	}
}

func TestSystemInstallRecordsTheProvenNodeSeparated(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(bin, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	d, _, errb := fakeDeps(t, "Linux")
	inner := d.Run
	probe := probeRunner(chainOK, keyOK, nil)
	d.Run = func(ctx context.Context, tg remote.Target, argv ...string) (string, error) {
		if len(argv) == 3 && strings.Contains(argv[2], " job-shim 2>&1") {
			return probe(ctx, argv...)
		}
		return inner(ctx, tg, argv...)
	}
	fs, _ := addFlags(t, "--system", "--slots", "2")
	req := lifecycle.InstallRequest{NodeID: "sys-proven", Platform: lifecycle.Platform{OS: "linux", Arch: "amd64"}, Dir: ".nself/ci-agent", Source: fakeSource{bin}, Flags: fs}
	if err := systemInstall(context.Background(), d, req); err != nil {
		t.Fatal(err)
	}
	defer installed.Delete("sys-proven")
	if f := d.Separation.Detect(context.Background(), nil, lifecycle.Platform{OS: "linux"}); !f.UIDSeparation || f.Reason != shimProbeOK {
		t.Fatalf("a proven system node: %+v (stderr %q)", f, errb.String())
	}
	if !strings.Contains(errb.String(), "recorded uid-separated") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestSystemInstallWithoutProofStaysUnseparated(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(bin, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	d, _, errb := fakeDeps(t, "Linux")
	fs, _ := addFlags(t, "--system")
	req := lifecycle.InstallRequest{NodeID: "sys-unproven", Platform: lifecycle.Platform{OS: "linux", Arch: "amd64"}, Dir: ".nself/ci-agent", Source: fakeSource{bin}, Flags: fs}
	if err := systemInstall(context.Background(), d, req); err != nil {
		t.Fatal(err)
	}
	defer installed.Delete("sys-unproven")
	if f := d.Separation.Detect(context.Background(), nil, lifecycle.Platform{OS: "linux"}); f.UIDSeparation || f.Reason != shimProbeUnreadable {
		t.Fatalf("an unproven system node: %+v", f)
	}
	if !strings.Contains(errb.String(), "separation not proven ("+shimProbeUnreadable+")") {
		t.Fatalf("stderr = %q", errb.String())
	}
}
