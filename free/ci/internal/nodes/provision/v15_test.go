package provision

// Purpose: the V15 behaviour of provision and verify (P7-NODE-06): the pinned runner with its sha256 check
// before extraction, the --json envelopes, the registry record and the dual-mode rule (v1.4 unchanged).
// Inputs:  fake executors; a local fixture server and stub sudo/sha256sum for the real script under bash.
// Constraints: obviously fake tokens only; nothing leaves the machine except loopback.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func fixturePin(url, sha string) *RunnerPin {
	return &RunnerPin{Version: "9.9.9", Sources: map[string]RunnerSource{
		"amd64": {URL: url, SHA256: sha}, "arm64": {URL: url, SHA256: sha}}}
}

func runnerTarball(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{
		"config.sh": "#!/bin/sh\nprintf '%s\\n' \"${ACTIONS_RUNNER_INPUT_TOKEN-unset}\" > ran-config.log\n",
		"svc.sh":    "#!/bin/sh\nexit 0\n",
	} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write([]byte(body))
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

// scriptRig is a bash host: stub sudo (drops -u USER), a sha256sum when the OS has none, a fixture server.
type scriptRig struct {
	root, url string
	body      []byte
}

func newScriptRig(t *testing.T) *scriptRig {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	r := &scriptRig{root: t.TempDir(), body: runnerTarball(t)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(r.body) }))
	t.Cleanup(srv.Close)
	r.url = srv.URL + "/actions-runner.tar.gz"
	bin := t.TempDir()
	stubs := map[string]string{"sudo": `case "$1" in chown) exit 0;; -u) shift 2;; esac; exec "$@"`}
	if _, err := exec.LookPath("sha256sum"); err != nil {
		stubs["sha256sum"] = `exec shasum -a 256 "$@"`
	}
	for name, body := range stubs {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return r
}

func (r *scriptRig) provision(pin *RunnerPin) (string, error) {
	opts := tokenOpts(r.root)
	opts.Runner = pin
	return installRunnerInstance(context.Background(), LocalExecutor{}, testManifest(), opts, 1)
}

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func TestV15TamperedRunnerAbortsBeforeExtraction(t *testing.T) {
	r := newScriptRig(t)
	_, err := r.provision(fixturePin(r.url, strings.Repeat("0", 64)))
	if bootstrap.ReasonOf(err) != bootstrap.ReasonChecksum || !strings.Contains(err.Error(), "E669") {
		t.Fatalf("err = %v, want E669 checksum", err)
	}
	inst := filepath.Join(r.root, "runner-1")
	for _, gone := range []string{"config.sh", "svc.sh", "actions-runner.tar.gz", "ran-config.log"} {
		if _, serr := os.Stat(filepath.Join(inst, gone)); serr == nil {
			t.Errorf("%s exists after a checksum mismatch: the tarball was extracted or kept", gone)
		}
	}
	if strings.Contains(err.Error(), fakeRegToken) {
		t.Fatal("the error carries the token")
	}
}

func TestV15PinnedRunnerInstallsAndKeepsTokenOffArgv(t *testing.T) {
	r := newScriptRig(t)
	out, err := r.provision(fixturePin(r.url, sum(r.body)))
	if err != nil {
		t.Fatalf("pinned install: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(filepath.Join(r.root, "runner-1", "ran-config.log")); string(got) != fakeRegToken+"\n" {
		t.Fatalf("config.sh saw %q", got)
	}
}

func TestV15UnknownArchitectureIsRefused(t *testing.T) {
	r := newScriptRig(t)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "uname"), []byte("#!/bin/sh\necho riscv64\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, err := r.provision(fixturePin(r.url, sum(r.body)))
	if bootstrap.ReasonOf(err) != bootstrap.ReasonRunnerArch {
		t.Fatalf("err = %v", err)
	}
}

func TestRunnerBlockV14AndV15(t *testing.T) {
	ex := &fakeExecutor{label: "h", fallback: fakeResponse{out: "ok"}}
	opts := tokenOpts("/opt/x")
	if _, err := installRunnerInstance(context.Background(), ex, testManifest(), opts, 1); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ex.commands[0], "releases/latest") || strings.Contains(ex.commands[0], "sha256sum") {
		t.Fatal("v1.4 script changed")
	}
	opts.Runner = fixturePin("https://example.test/r.tgz", strings.Repeat("a", 64))
	if _, err := installRunnerInstance(context.Background(), ex, testManifest(), opts, 1); err != nil {
		t.Fatal(err)
	}
	v15 := ex.commands[1]
	for _, want := range []string{"sha256sum -c -", strings.Repeat("a", 64), "https://example.test/r.tgz"} {
		if !strings.Contains(v15, want) {
			t.Errorf("v15 script lacks %q", want)
		}
	}
	if strings.Contains(v15, "releases/latest") || strings.Index(v15, "sha256sum") > strings.Index(v15, "tar xzf") {
		t.Error("v15 script still asks for the latest release or extracts before the check")
	}
	if strings.Contains(v15, fakeRegToken) {
		t.Error("token in the script")
	}
}

func TestRunnerPinValidation(t *testing.T) {
	for _, bad := range []*RunnerPin{
		{}, {Version: "1", Sources: map[string]RunnerSource{"amd64": {URL: "ftp://x", SHA256: strings.Repeat("a", 64)}}},
		{Version: "1", Sources: map[string]RunnerSource{"amd64": {URL: "https://x/y", SHA256: "short"}}},
		{Version: "1", Sources: map[string]RunnerSource{"amd64": {URL: "https://x/y z", SHA256: strings.Repeat("a", 64)}}},
	} {
		if bad.Validate() == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

// ---- command level -------------------------------------------------------------------------------

type v15Rig struct {
	ex       *fakeExecutor
	out, err bytes.Buffer
	recorded []RecordRequest
	deps     Deps
}

func newV15Rig(t *testing.T, v15 bool) *v15Rig {
	t.Helper()
	if v15 {
		t.Setenv("NSELF_V15", "1")
	} else {
		t.Setenv("NSELF_V15", "")
	}
	r := &v15Rig{ex: &fakeExecutor{label: "ci@host-1", fallback: fakeResponse{out: "ok"}}}
	r.deps = Deps{Out: &r.out, Err: &r.err, Getenv: func(string) string { return "" }, Ctx: context.Background(),
		Executors: func([]string, string) []Executor { return []Executor{r.ex} },
		RunnerPin: func() (RunnerPin, error) {
			return *fixturePin("https://example.test/r.tgz", strings.Repeat("b", 64)), nil
		},
		Record: func(_ context.Context, req RecordRequest) (string, error) {
			r.recorded = append(r.recorded, req)
			return RunnerNodeID(req.Host), nil
		}}
	return r
}

type envelope struct {
	SchemaVersion string          `json:"schema_version"`
	Command       string          `json:"command"`
	Data          json.RawMessage `json:"data"`
	Error         *struct {
		Code     string `json:"code"`
		Message  string `json:"message"`
		ExitCode int    `json:"exit_code"`
		Class    string `json:"class"`
	} `json:"error"`
}

func (r *v15Rig) envelope(t *testing.T) envelope {
	t.Helper()
	var e envelope
	dec := json.NewDecoder(bytes.NewReader(r.out.Bytes()))
	if err := dec.Decode(&e); err != nil {
		t.Fatalf("stdout is not one envelope: %v\n%s", err, r.out.String())
	}
	if dec.More() {
		t.Fatalf("stdout holds more than one document:\n%s", r.out.String())
	}
	if e.SchemaVersion != "1" {
		t.Fatalf("schema_version = %q", e.SchemaVersion)
	}
	return e
}

var provArgs = []string{"--json", "--host", "ci@host-1", "--github-url", "https://github.com/nself-org/example", "--token", fakeRegToken, "--labels", "gpu"}

func TestV15ProvisionJSONEnvelopeAndRecord(t *testing.T) {
	r := newV15Rig(t, true)
	if code := RunProvision(provArgs, r.deps); code != 0 {
		t.Fatalf("exit %d\n%s%s", code, r.out.String(), r.err.String())
	}
	e := r.envelope(t)
	if e.Command != "ci nodes provision" || e.Error != nil {
		t.Fatalf("envelope = %+v", e)
	}
	var d ProvisionData
	if err := json.Unmarshal(e.Data, &d); err != nil || d.Host != "ci@host-1" || d.Instances != 1 || d.Node != "gh-runner-host-1" || len(d.Steps) < 5 {
		t.Fatalf("data = %+v err=%v", d, err)
	}
	if strings.Contains(r.out.String()+r.err.String(), fakeRegToken) {
		t.Fatal("token in the output")
	}
	if len(r.recorded) != 1 || r.recorded[0].Host != "ci@host-1" || r.recorded[0].Labels[0] != "gpu" {
		t.Fatalf("recorded = %+v", r.recorded)
	}
	if !strings.Contains(strings.Join(r.ex.commands, "\n"), "sha256sum -c -") {
		t.Fatal("V15 provision did not use the pinned runner")
	}
}

func TestV15ProvisionTextModeStillRecords(t *testing.T) {
	r := newV15Rig(t, true)
	args := append([]string(nil), provArgs[1:]...)
	if code := RunProvision(args, r.deps); code != 0 || !strings.Contains(r.out.String(), "Provision complete.") || len(r.recorded) != 1 {
		t.Fatalf("exit %d out=%q recorded=%d", code, r.out.String(), len(r.recorded))
	}
}

func TestV14ProvisionIsUnchanged(t *testing.T) {
	r := newV15Rig(t, false)
	if code := RunProvision(provArgs, r.deps); code != 1 || r.out.Len() != 0 || !strings.Contains(r.err.String(), "does not support --json") {
		t.Fatalf("--json in v1.4: exit %d out=%q err=%q", code, r.out.String(), r.err.String())
	}
	r = newV15Rig(t, false)
	if code := RunProvision(provArgs[1:], r.deps); code != 0 {
		t.Fatalf("exit %d: %s", code, r.err.String())
	}
	if len(r.recorded) != 0 || strings.Contains(strings.Join(r.ex.commands, "\n"), "sha256sum") || !strings.Contains(strings.Join(r.ex.commands, "\n"), "releases/latest") {
		t.Fatal("v1.4 provision recorded a node or used the pin")
	}
}

func TestV15ProvisionErrorEnvelopes(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		prep     func(*v15Rig)
		code     string
		wantExit int
	}{
		{"missing github url", []string{"--json", "--token", "x"}, nil, "E651", 1},
		{"two hosts", []string{"--json", "--host", "a", "--host", "b", "--github-url", "u", "--token", "x"}, nil, "E651", 1},
		{"a failing step", provArgs, func(r *v15Rig) { r.ex.when("apt-get", "nope", errors.New("exit status 100")) }, "E607", 2},
		{"checksum mismatch", provArgs, func(r *v15Rig) { r.ex.when("sha256sum", markChecksum, errors.New("exit status 69")) }, "E669", 2},
		{"no pin wired", provArgs, func(r *v15Rig) { r.deps.RunnerPin = nil }, "E607", 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newV15Rig(t, true)
			if c.prep != nil {
				c.prep(r)
			}
			code := RunProvision(c.args, r.deps)
			e := r.envelope(t)
			if code != c.wantExit || e.Error == nil || e.Error.Code != c.code || e.Error.ExitCode != code || strings.Contains(r.out.String(), fakeRegToken) {
				t.Fatalf("exit %d envelope %+v", code, e)
			}
			if len(r.recorded) != 0 {
				t.Fatal("a failed provision was recorded")
			}
		})
	}
}

func TestV15VerifyJSONEnvelopeAndV14Raw(t *testing.T) {
	r := newV15Rig(t, true)
	// the fake host fails the dependency checks, so verify exits 1 and still prints its data
	if code := RunVerify([]string{"--json", "--host", "ci@host-1"}, r.deps); code != 1 {
		t.Fatalf("exit %d out=%s", code, r.out.String())
	}
	e := r.envelope(t)
	var d VerifyData
	if e.Command != "ci nodes verify" || json.Unmarshal(e.Data, &d) != nil || len(d.Hosts) != 1 || d.Hosts[0].Host != "ci@host-1" || d.Drift == nil {
		t.Fatalf("envelope = %s", r.out.String())
	}
	bad := newV15Rig(t, true)
	bad.ex.when("echo reachable", "", errors.New("connection refused"))
	if code := RunVerify([]string{"--json", "--host", "ci@host-1"}, bad.deps); code != 1 {
		t.Fatalf("an unreachable host exits %d", code)
	}
	if e = bad.envelope(t); e.Error != nil {
		t.Fatal("verify problems must still print the data envelope")
	}
	old := newV15Rig(t, false)
	if code := RunVerify([]string{"--json", "--host", "ci@host-1"}, old.deps); code != 1 {
		t.Fatalf("exit %d", code)
	}
	var raw []HostReport
	if err := json.Unmarshal(old.out.Bytes(), &raw); err != nil || len(raw) != 1 {
		t.Fatalf("v1.4 --json is not the raw []HostReport: %v\n%s", err, old.out.String())
	}
}

// ---- the registry record ---------------------------------------------------------------------------

func TestRunnerCapabilityIsAProviderNodeNoLeaseReaches(t *testing.T) {
	c := RunnerCapability(RecordRequest{Host: "ci@Host.Example", Labels: []string{"gpu"}}, time.Unix(1_700_000_000, 0).UTC())
	if err := model.ValidateCapability(c); err != nil {
		t.Fatal(err)
	}
	if c.Identity.ID != "gh-runner-host.example" || c.Trust.Accepts.Value != nil || c.Trust.Accepts.Source != "provider" {
		t.Fatalf("identity %q accepts %+v", c.Identity.ID, c.Trust.Accepts)
	}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var back model.Capability
	if err = json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("the stored document does not read back: %v", err)
	}
	if bytes.Contains(raw, []byte(`"accepts":{"value":[`)) {
		t.Fatal("accepts holds classes")
	}
	decl := sched.DeclarationFromCapability(back, false, trust.Personal)
	for _, class := range []model.TrustClass{"owner", "collaborator", "internal", "untrusted"} {
		reasons := trust.Admissible(trust.JobFacts{Trust: class}, decl, trust.Effective{})
		found := false
		for _, r := range reasons {
			found = found || r == trust.TrustNotAccepted
		}
		if !found {
			t.Errorf("a %s job is admitted to a provisioned runner: %v", class, reasons)
		}
	}
}

func TestRunnerNodeID(t *testing.T) {
	for in, want := range map[string]string{"ci@h1": "gh-runner-h1", "H_2": "gh-runner-h_2", "u@10.0.0.5": "gh-runner-10.0.0.5", "u@[::1]": "gh-runner-1"} {
		if got := RunnerNodeID(in); got != want {
			t.Errorf("RunnerNodeID(%q) = %q, want %q", in, got, want)
		}
	}
}
