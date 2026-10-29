package main

// Purpose: tests for `nself ci artifacts key|key init|key rotate` and the
// hidden `artifacts signer` (P7-CACHE-07). A fake `nself` shell script stands
// in for the cli: it keeps secrets in a directory, answers `config secrets
// resolve` with the v1 envelope and `config secrets set` from stdin.
// Inputs:  none. Outputs: pass or fail.
// Constraints: unix only (the signer's stdin FIFO and sh wrapper); the signer
// runs in process through an in-process StdioRunner, but the cli calls go
// through the real exec funnel and a real FIFO.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/storetest"
)

// fakeNself writes the fake cli into a bin directory on PATH and returns the
// directory its secrets live in.
func fakeNself(t *testing.T) string {
	t.Helper()
	bin, secretsDir := t.TempDir(), t.TempDir()
	script := `#!/bin/sh
S='` + secretsDir + `'
if [ "$1 $2 $3" = "config secrets set" ] || [ "$1 $2" = "secrets set" ]; then
  [ "$1" = config ] && shift
  shift 2
  case "$1" in --help) exit 0;; esac
  [ "$1" = "--env" ] && [ "$2" = "release" ] || exit 9
  cat > "$S/$3"
  exit 0
fi
if [ "$1 $2 $3" = "config secrets resolve" ]; then
  case "$4" in --help) exit 0;; esac
  # $4 --project $5 P $6 --for-attempt $7 A $8 --json $9 REF
  ref="$9"; att="$7"; name="${ref#release/}"
  if [ ! -f "$S/$name" ]; then printf '{"error":{"code":"E541"}}\n'; exit 1; fi
  printf '{"data":{"schema":"cli.secret-resolve/v1","attempt_id":"%s","values":[{"ref":"%s","env_name":"%s","value":"%s","source":"age:release"}]}}\n' "$att" "$ref" "$name" "$(cat "$S/$name")"
  exit 0
fi
exit 8
`
	if err := os.WriteFile(filepath.Join(bin, "nself"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return secretsDir
}

// inProc runs the signer in this process: the same code `nself-ci artifacts
// signer` runs, without a second binary.
type inProc struct{ calls []string }

func (p *inProc) RunStdio(_ context.Context, c signing.StdioCall) ([]byte, int, error) {
	mode := c.Argv[len(c.Argv)-1]
	p.calls = append(p.calls, mode)
	var out bytes.Buffer
	signerIn, signerOut = bytes.NewReader(c.Stdin), &out
	defer func() { signerIn, signerOut = nil, nil }()
	code := artifactsSigner([]string{mode})
	return out.Bytes(), code, nil
}

func useInProcSigner(t *testing.T) *inProc {
	t.Helper()
	t.Setenv("HOME", t.TempDir()) // the key lock lives under the user's home
	p := &inProc{}
	old := keyClient
	keyClient = func() (*signing.Client, error) { return &signing.Client{Runner: p, Exe: "nself-ci"}, nil }
	t.Cleanup(func() { keyClient = old })
	return p
}

func runKey(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	old := keyConfirmIn
	f, _ := os.CreateTemp(t.TempDir(), "in")
	_, _ = f.WriteString(stdin)
	_, _ = f.Seek(0, 0)
	keyConfirmIn = f
	defer func() { keyConfirmIn = old; _ = f.Close() }()
	return runArt(t, append([]string{"key"}, args...)...)
}

func dataOf(t *testing.T, out string) map[string]any {
	t.Helper()
	d, _ := checkEnvelope(t, out)["data"].(map[string]any)
	if d == nil {
		t.Fatalf("no data in %s", out)
	}
	return d
}

func TestArtifactsKeyInit(t *testing.T) {
	r := newArtRig(t)
	secretsDir := fakeNself(t)
	proc := useInProcSigner(t)

	// no key yet: the public call names `key init`
	if code, out, errs := runKey(t, ""); code != 2 || !strings.Contains(out+errs, "key_missing") && !strings.Contains(out+errs, "key init") {
		t.Fatalf("key before init: %d %q %q", code, out, errs)
	}
	code, out, errs := runKey(t, "", "init", "--json")
	if code != 0 {
		t.Fatalf("init: %d %q %q", code, out, errs)
	}
	d := dataOf(t, out)
	keyID, pub, line := d["key_id"].(string), d["public_key"].(string), d["trust_line"].(string)
	if !strings.HasPrefix(keyID, "ci-release-") || line != keyID+" "+pub || d["old_key_id"] != nil {
		t.Fatalf("%v", d)
	}
	stored, err := os.ReadFile(filepath.Join(secretsDir, signing.KeyName))
	if err != nil || len(stored) == 0 {
		t.Fatalf("the key was not stored by `config secrets set`: %v", err)
	}
	for _, sink := range []string{out, errs} {
		if strings.Contains(sink, string(stored)) || strings.Contains(sink, string(stored)[:30]) {
			t.Fatal("the key value reached stdout or stderr")
		}
	}
	// the stored key reads back through `config secrets resolve`: `key` prints the same line
	code, out, _ = runKey(t, "")
	if code != 0 || strings.TrimSpace(out) != line {
		t.Fatalf("key: %d %q want %q", code, out, line)
	}
	code, out, _ = runKey(t, "", "--json")
	if code != 0 || dataOf(t, out)["trust_line"] != line {
		t.Fatalf("key --json: %d %q", code, out)
	}
	// the trust line builds a trust file that verifies a signed fixture
	keys, err := sdk.ParseKeysFile(strings.NewReader(line+"\n"), sdk.PurposeCIRelease)
	if err != nil || len(keys) != 1 {
		t.Fatalf("trust file: %v", err)
	}
	v, err := sdk.NewVerifier(sdk.PurposeCIRelease, keys, nil)
	if err != nil {
		t.Fatal(err)
	}
	env := signFixture(t, r)
	if _, _, ids, err := sdk.VerifyEnvelope(v, env); err != nil || ids[0] != keyID {
		t.Fatalf("a release signed by the signer does not verify under the printed trust line: %v %v", err, ids)
	}
	// a second init is refused and stores nothing
	before, _ := os.ReadFile(filepath.Join(secretsDir, signing.KeyName))
	code, out, errs = runKey(t, "", "init", "--json")
	after, _ := os.ReadFile(filepath.Join(secretsDir, signing.KeyName))
	if code != 2 || !strings.Contains(out, "secret_exists") || string(before) != string(after) {
		t.Fatalf("second init: %d %q %q", code, out, errs)
	}
	if len(proc.calls) == 0 {
		t.Fatal("the signer was not used")
	}
}

func TestArtifactsKeyInitNeedsASecretsSetCli(t *testing.T) {
	newArtRig(t)
	bin := t.TempDir()
	// a cli that has `config secrets resolve` but no `set` in either spelling
	_ = os.WriteFile(filepath.Join(bin, "nself"), []byte("#!/bin/sh\nif [ \"$4\" = --help ] && [ \"$3\" = resolve ]; then exit 0; fi\nprintf '{\"error\":{\"code\":\"E541\"}}'; exit 1\n"), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	useInProcSigner(t)
	if code, out, _ := runKey(t, "", "init", "--json"); code != 2 || !strings.Contains(out, "cli_too_old") {
		t.Fatalf("%d %q", code, out)
	}
}

func TestArtifactsKeyRotate(t *testing.T) {
	r := newArtRig(t)
	secretsDir := fakeNself(t)
	useInProcSigner(t)
	if code, out, errs := runKey(t, "", "init"); code != 0 {
		t.Fatalf("%d %q %q", code, out, errs)
	}
	oldLine, _, _ := runKeyLine(t)
	oldID := strings.Fields(oldLine)[0]
	oldEnv := signFixture(t, r)
	oldValue, _ := os.ReadFile(filepath.Join(secretsDir, signing.KeyName))

	// without confirmation nothing changes
	if code, _, errs := runKey(t, "n\n", "rotate"); code != 1 || !strings.Contains(errs, "not confirmed") {
		t.Fatalf("declined rotate: %d %q", code, errs)
	}
	if code, _, _ := runKey(t, "", "rotate"); code != 1 {
		t.Fatalf("rotate with no answer: %d", code)
	}
	if v, _ := os.ReadFile(filepath.Join(secretsDir, signing.KeyName)); string(v) != string(oldValue) {
		t.Fatal("a declined rotation changed the key")
	}
	if code, _, _ := runKey(t, "", "--yes"); code != 1 {
		t.Fatal("--yes without rotate was accepted")
	}

	code, out, errs := runKey(t, "y\n", "rotate", "--json")
	if code != 0 {
		t.Fatalf("rotate: %d %q %q", code, out, errs)
	}
	d := dataOf(t, out)
	newLine := d["trust_line"].(string)
	if d["old_key_id"] != oldID || strings.Fields(newLine)[0] == oldID {
		t.Fatalf("%v (old %s)", d, oldID)
	}
	if v, _ := os.ReadFile(filepath.Join(secretsDir, signing.KeyName)); string(v) == string(oldValue) {
		t.Fatal("rotate did not replace the stored key")
	}
	// text output: new trust line on stdout, the old id for .revoked on stderr
	code, out, errs = runKey(t, "", "rotate", "--yes")
	if code != 0 || len(strings.Fields(out)) != 2 || !strings.Contains(errs, ".revoked") || !strings.Contains(errs, strings.Fields(newLine)[0]) {
		t.Fatalf("rotate --yes: %d %q %q", code, out, errs)
	}
	// a release signed before the rotation fails once the old id is revoked
	oldKeys, _ := sdk.ParseKeysFile(strings.NewReader(oldLine+"\n"+newLine+"\n"), sdk.PurposeCIRelease)
	trusted, _ := sdk.NewVerifier(sdk.PurposeCIRelease, oldKeys, nil)
	if _, _, _, err := sdk.VerifyEnvelope(trusted, oldEnv); err != nil {
		t.Fatalf("the old release stopped verifying before it was revoked: %v", err)
	}
	revoked, _ := sdk.ParseRevokedFile(strings.NewReader(oldID + "\n"))
	rv, _ := sdk.NewVerifier(sdk.PurposeCIRelease, oldKeys, revoked)
	if _, _, _, err := sdk.VerifyEnvelope(rv, oldEnv); err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("a release signed with the revoked key verified: %v", err)
	}
	// rotate without a key says so
	_ = os.Remove(filepath.Join(secretsDir, signing.KeyName))
	if code, out, _ := runKey(t, "", "rotate", "--yes", "--json"); code != 2 || !strings.Contains(out, "key_missing") {
		t.Fatalf("%d %q", code, out)
	}
}

func runKeyLine(t *testing.T) (string, string, string) {
	t.Helper()
	code, out, errs := runKey(t, "")
	if code != 0 {
		t.Fatalf("key: %d %q %q", code, out, errs)
	}
	return strings.TrimSpace(out), out, errs
}

// Two processes with different CI homes but one project serialize on one lock in the user's home.
func TestSignerStoreLocksPerProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	s, err := signerStore(root)
	if err != nil {
		t.Fatal(err)
	}
	st, ok := s.(*signing.CLIStore)
	if !ok || st.LockDir != filepath.Join(home, ".nself", "locks") || st.Project != root {
		t.Fatalf("%+v", st)
	}
	t.Setenv("NSELF_CI_HOME", t.TempDir())
	a, err := st.Lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer a()
	t.Setenv("NSELF_CI_HOME", t.TempDir()) // another CI home, the same project
	s2, _ := signerStore(root)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := s2.Lock(ctx); err == nil {
		t.Fatal("a second CI home took the project's key lock")
	}
	other, _ := signerStore(t.TempDir()) // another project locks independently
	u, err := other.Lock(context.Background())
	if err != nil {
		t.Fatalf("another project: %v", err)
	}
	u()
	if left, _ := filepath.Glob(filepath.Join(root, ".secrets", "*")); len(left) != 0 {
		t.Fatalf("the lock file is in the project: %v", left)
	}
}

func TestArtifactsKeyUsage(t *testing.T) {
	newArtRig(t)
	for _, args := range [][]string{{"bogus"}, {"init", "rotate"}, {"init", "--yes"}} {
		if code, _, errs := runKey(t, "", args...); code != 1 || !strings.Contains(errs, "usage:") {
			t.Errorf("%v: %d %q", args, code, errs)
		}
	}
	if code, _, errs := runArt(t, "key", "--nope"); code != 1 {
		t.Errorf("bad flag: %d %q", code, errs)
	}
}

// signedRun creates a finished release run in the rig's store and returns a
// signer request over a statement whose one subject is the run's evidence.
func signedRun(t *testing.T, r *artRig) signing.Request {
	t.Helper()
	ctx := context.Background()
	p := store.PipelineRow{ID: "run-rel", Revision: strings.Repeat("a", 40), Project: r.project, Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	steps := []error{r.st.CreatePipeline(ctx, p), r.st.CreateJob(ctx, store.JobRow{ID: "job-rel", PipelineID: p.ID, Name: "publish", Kind: "release", Idempotent: false, InfraMax: 1, InputDigest: "i"}),
		r.st.CreateAttempt(ctx, store.AttemptRow{ID: "att-rel", JobID: "job-rel", N: 1}, ""), storetest.Admit(ctx, r.st, "att-rel")}
	for _, s := range []struct {
		from, to model.JobState
		epoch    int64
	}{{"leased", "running", 1}, {"running", "finalizing", 2}, {"finalizing", "passed", 3}} {
		steps = append(steps, r.st.Transition(ctx, "att-rel", s.from, s.to, store.TransitionDetail{Epoch: s.epoch}))
	}
	body, _ := json.Marshal(model.Evidence{Schema: "ci.evidence/v1", Revision: p.Revision, RepoID: "repo", RunID: p.ID, Result: "pass"})
	steps = append(steps, r.st.RecordRunEvidence(ctx, "ev-1", p.ID, p.Revision, p.InputDigest, body))
	for _, err := range steps {
		if err != nil {
			t.Fatal(err)
		}
	}
	sum := sha256.Sum256(body)
	predicate := map[string]any{
		"buildDefinition": map[string]any{"buildType": signing.ReleaseBuildType, "externalParameters": map[string]any{"repository": "repo", "revision": p.Revision, "job": "publish"},
			"resolvedDependencies": []map[string]any{{"uri": "git+repo@" + p.Revision, "digest": map[string]string{"gitCommit": p.Revision}}}},
		"runDetails": map[string]any{"builder": map[string]any{"id": "https://nself.org/ci/builder/v1#local/runner"},
			"metadata":   map[string]any{"invocationId": p.ID + "/att-rel"},
			"byproducts": []map[string]any{{"name": "evidence.json", "digest": map[string]string{"sha256": hex.EncodeToString(sum[:])}}}}}
	st, _ := json.Marshal(map[string]any{"_type": "https://in-toto.io/Statement/v1", "predicateType": "https://slsa.dev/provenance/v1", "predicate": predicate,
		"subject": []map[string]any{{"name": "evidence.json", "digest": map[string]string{"sha256": hex.EncodeToString(sum[:])}}}})
	return signing.Request{RunID: p.ID, Items: []signing.Item{{PayloadType: signing.PayloadStatement, PayloadB64: b64(st)}}}
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// signFixture signs a statement of a finished run through the signer, over the real store.
func signFixture(t *testing.T, r *artRig) sdk.Envelope {
	t.Helper()
	req := signedRun(t, r)
	raw, _ := json.Marshal(req)
	signerIn = bytes.NewReader(raw)
	defer func() { signerIn = nil }()
	code, out, errs := runArt(t, "signer", "--stdio")
	if code != 0 {
		t.Fatalf("signer: %d %q %q", code, out, errs)
	}
	var resp signing.Response
	if err := json.Unmarshal([]byte(out), &resp); err != nil || len(resp.Envelopes) != 1 {
		t.Fatalf("%v %q", err, out)
	}
	return resp.Envelopes[0]
}

func TestArtifactsSignerRefusals(t *testing.T) {
	r := newArtRig(t)
	fakeNself(t)
	signerRefuses := func(name string, stdin []byte, reason string) {
		t.Helper()
		signerIn = bytes.NewReader(stdin)
		defer func() { signerIn = nil }()
		code, out, _ := runArt(t, "signer", "--stdio")
		var resp signing.Response
		if code != 2 || json.Unmarshal([]byte(out), &resp) != nil || resp.Error == nil || resp.Error.Code != "E717" || resp.Error.Reason != reason || len(resp.Envelopes) != 0 {
			t.Fatalf("%s: exit %d, %q", name, code, out)
		}
	}
	item := func(typ string) signing.Item { return signing.Item{PayloadType: typ, PayloadB64: b64([]byte("{}"))} }
	req := func(runID string, items ...signing.Item) []byte {
		b, _ := json.Marshal(signing.Request{RunID: runID, Items: items})
		return b
	}
	signerRefuses("a payload type outside the two allowed", req("run-x", item("text/plain")), signing.ReasonPayloadRejected)
	signerRefuses("a cosign bundle", req("run-x", item("application/vnd.dev.sigstore.bundle+json")), signing.ReasonPayloadRejected)
	signerRefuses("an oversize request", append([]byte(`{"run_id":"r","items":[{"payload_type":"`+signing.PayloadStatement+`","payload_b64":"`), bytes.Repeat([]byte("A"), signing.MaxRequestBytes)...), signing.ReasonPayloadRejected)
	signerRefuses("garbage", []byte("not a request"), signing.ReasonPayloadRejected)
	signerRefuses("an unknown run", req("run-nope", item(signing.PayloadStatement)), signing.ReasonPayloadRejected)
	// a run with an attempt that has not finished: the rig's seed leaves one finalizing
	r.seed("a.txt", "x", "project", "keep")
	signerRefuses("a non-terminal run", req("run"+"attb", item(signing.PayloadStatement)), signing.ReasonPayloadRejected)
	// no key has been created: a request that passes every check still signs nothing
	fix := signedRun(t, r)
	raw, _ := json.Marshal(fix)
	signerRefuses("a run whose release key is not set", raw, signing.ReasonKeyMissing)
	// a run of another project than the one this signer serves is never signed
	t.Setenv("NSELF_CI_REPO", t.TempDir())
	signerRefuses("a run of another project", raw, signing.ReasonPayloadRejected)
	t.Setenv("NSELF_CI_REPO", r.project)
	// a TEAM coordinator never resolves the release key, in any signer mode
	t.Setenv("NSELF_CI_COORDINATOR_MODE", "team")
	signerRefuses("a TEAM coordinator", raw, signing.ReasonSignerFailed)
	for _, mode := range []string{"--public", "--init", "--rotate"} {
		signerIn = bytes.NewReader(nil)
		code, out, _ := runArt(t, "signer", mode)
		signerIn = nil
		if code != 2 || !strings.Contains(out, "TEAM") {
			t.Fatalf("%s on a TEAM coordinator: %d %q", mode, code, out)
		}
	}
	if code, out, errs := runKey(t, "", "--json"); code != 2 || !strings.Contains(out+errs, "TEAM") {
		t.Fatalf("key on a TEAM coordinator: %d %q %q", code, out, errs)
	}
	t.Setenv("NSELF_CI_COORDINATOR_MODE", "")
	// usage
	if code, _, errs := runArt(t, "signer"); code != 1 || !strings.Contains(errs, "started by the coordinator") {
		t.Fatalf("signer without a mode: %d %q", code, errs)
	}
	if code, _, _ := runArt(t, "signer", "--stdio", "--public"); code != 1 {
		t.Fatal("two modes accepted")
	}
	if code, _, _ := runArt(t, "signer", "--bogus"); code != 1 {
		t.Fatal("unknown flag accepted")
	}
}
