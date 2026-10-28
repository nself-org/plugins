//go:build container

package signing_test

// Purpose: container-level evidence for the signer's isolation (P7-CACHE-07,
// Epic CACHE D12) and its fail-closed behaviour.
// Inputs:  docker (a container job), a built nself-ci binary, a fake nself.
// Outputs: pass or fail; prerequisites missing is t.Fatal, never t.Skip.
// Constraints: unix + docker. The signer binary is the real one, started by
// the real Client through the exec funnel; the run store is a real SQLite
// store; the key lives only in the fake cli's secret directory.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/nself-org/cli/sdk/go/v2/signing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/storetest"
)

func dockerImage(t *testing.T) string {
	t.Helper()
	if out, err := osexec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Fatalf("Docker required: %v: %s", err, out)
	}
	image := os.Getenv("CI32_TEST_IMAGE")
	if image == "" {
		image = "alpine:3.19"
	}
	if err := osexec.Command("docker", "image", "inspect", image).Run(); err != nil {
		if out, perr := osexec.Command("docker", "pull", image).CombinedOutput(); perr != nil {
			t.Fatalf("image %s unavailable: %v: %s", image, perr, out)
		}
	}
	return image
}

// buildSigner compiles the real nself-ci binary into dir.
func buildSigner(t *testing.T, dir string) string {
	t.Helper()
	exe := filepath.Join(dir, "nself-ci")
	cmd := osexec.Command("go", "build", "-mod=vendor", "-o", exe, "./cmd")
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v: %s", err, out)
	}
	return exe
}

func fakeCLI(t *testing.T, secretsDir string) {
	t.Helper()
	bin := t.TempDir()
	script := `#!/bin/sh
S='` + secretsDir + `'
if [ "$1 $2 $3" = "config secrets resolve" ]; then
  case "$4" in --help) exit 0;; esac
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
}

// world is a CI home with a run whose release attempt can be moved to any state.
type world struct {
	home, project, secrets string
	st                     *store.Store
	key                    *signing.Key
	value                  string
	items                  []signing.Item
}

func newWorld(t *testing.T) *world {
	t.Helper()
	w := &world{home: t.TempDir(), project: t.TempDir(), secrets: t.TempDir()}
	var err error
	if w.st, err = store.Open(filepath.Join(w.home, "state.db"), store.Options{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.st.Close() })
	key, value, err := signing.Generate()
	if err != nil {
		t.Fatal(err)
	}
	w.key, w.value = key, string(value)
	if err := os.WriteFile(filepath.Join(w.secrets, signing.KeyName), value, 0o600); err != nil {
		t.Fatal(err)
	}
	fakeCLI(t, w.secrets)
	ctx := context.Background()
	p := store.PipelineRow{ID: "run-c", Revision: strings.Repeat("a", 40), Project: w.project, Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	for _, e := range []error{w.st.CreatePipeline(ctx, p), w.st.CreateJob(ctx, store.JobRow{ID: "job-c", PipelineID: p.ID, Name: "publish", Kind: "release", InfraMax: 1, InputDigest: "i"}),
		w.st.CreateAttempt(ctx, store.AttemptRow{ID: "att-c", JobID: "job-c", N: 1}, ""), storetest.Admit(ctx, w.st, "att-c")} {
		if e != nil {
			t.Fatal(e)
		}
	}
	for _, s := range []struct {
		from, to model.JobState
		epoch    int64
	}{{"leased", "running", 1}} {
		if err := w.st.Transition(ctx, "att-c", s.from, s.to, store.TransitionDetail{Epoch: s.epoch}); err != nil {
			t.Fatal(err)
		}
	}
	body, _ := json.Marshal(model.Evidence{Schema: "ci.evidence/v1", Revision: p.Revision, RepoID: "repo", RunID: p.ID, Result: "pass"})
	if err := w.st.RecordRunEvidence(ctx, "ev-c", p.ID, p.Revision, p.InputDigest, body); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	predicate := map[string]any{
		"buildDefinition": map[string]any{"buildType": signing.ReleaseBuildType, "externalParameters": map[string]any{"repository": "repo", "revision": p.Revision, "job": "publish"},
			"resolvedDependencies": []map[string]any{{"uri": "git+repo@" + p.Revision, "digest": map[string]string{"gitCommit": p.Revision}}}},
		"runDetails": map[string]any{"builder": map[string]any{"id": "https://nself.org/ci/builder/v1#" + signingCoordinator(w.st) + "/runner"},
			"metadata":   map[string]any{"invocationId": p.ID + "/att-c"},
			"byproducts": []map[string]any{{"name": "evidence.json", "digest": map[string]string{"sha256": hex.EncodeToString(sum[:])}}}}}
	st, _ := json.Marshal(map[string]any{"_type": "https://in-toto.io/Statement/v1", "predicateType": "https://slsa.dev/provenance/v1", "predicate": predicate,
		"subject": []map[string]any{{"name": "evidence.json", "digest": map[string]string{"sha256": hex.EncodeToString(sum[:])}}}})
	w.items = []signing.Item{item(signing.PayloadStatement, st)}
	return w
}

func (w *world) finish(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if err := w.st.Transition(ctx, "att-c", "running", "finalizing", store.TransitionDetail{Epoch: 2}); err != nil {
		t.Fatal(err)
	}
	if err := w.st.Transition(ctx, "att-c", "finalizing", "passed", store.TransitionDetail{Epoch: 3}); err != nil {
		t.Fatal(err)
	}
}

// markerExe is a stand-in signer binary that records that it was started.
func markerExe(t *testing.T, marker, body string) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "fake-signer")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho started >> '"+marker+"'\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

// TestSignerUnreachable: an unreachable, hung or half-answering signer fails the release closed.
func TestSignerUnreachable(t *testing.T) {
	w := newWorld(t)
	w.finish(t)
	partial := `{"envelopes":[{"payloadType":"` + signing.PayloadStatement + `","payload":"` + w.items[0].PayloadB64 + `","signatures":[{"keyid":"ci-release-0123456789abcdef","sig":"AAAA"`
	for name, c := range map[string]struct {
		exe     string
		timeout time.Duration
	}{
		"binary missing":   {"/nonexistent/nself-ci", 10 * time.Second},
		"hangs":            {markerExe(t, filepath.Join(t.TempDir(), "m"), "sleep 60"), 700 * time.Millisecond},
		"dies mid-answer":  {markerExe(t, filepath.Join(t.TempDir(), "m"), "printf '%s' '"+partial+"'; exit 3"), 10 * time.Second},
		"exits 0 no data":  {markerExe(t, filepath.Join(t.TempDir(), "m"), "exit 0"), 10 * time.Second},
		"killed by signal": {markerExe(t, filepath.Join(t.TempDir(), "m"), "kill -9 $$"), 10 * time.Second},
	} {
		c := signing.Client{Runner: &signing.FunnelRunner{Home: w.home}, Src: w.st, Exe: c.exe, Timeout: c.timeout, Env: []string{"NSELF_CI_HOME=" + w.home}}
		start := time.Now()
		envs, err := c.Sign(context.Background(), "run-c", w.items)
		if err == nil || envs != nil || !strings.HasPrefix(err.Error(), "E717 ") {
			t.Errorf("%s: signed or wrong error: %v %v", name, envs, err)
		}
		if time.Since(start) > 20*time.Second {
			t.Errorf("%s: took %v", name, time.Since(start))
		}
	}
}

// TestKeyNeverLeaks: a malicious container job of the run finds no key material, no signer
// process exists while it runs, and after the signer ran the planted key is in no output,
// log, store file or process argument.
func TestKeyNeverLeaks(t *testing.T) {
	image := dockerImage(t)
	w := newWorld(t)
	exe := buildSigner(t, t.TempDir())
	fragment := w.value[:40]
	marker := filepath.Join(t.TempDir(), "started")
	client := func(e string) *signing.Client {
		return &signing.Client{Runner: &signing.FunnelRunner{Home: w.home}, Src: w.st, Exe: e, Timeout: 60 * time.Second,
			Env: []string{"NSELF_CI_HOME=" + w.home, "NSELF_CI_REPO=" + w.project}}
	}

	// 1. while the malicious job runs: the coordinator refuses to start a signer.
	var jobOut strings.Builder
	var mu sync.Mutex
	job := `FRAG='` + fragment + `'
for p in "$HOME" /proc/*/environ /root /home /run /var/run /etc /tmp /secrets /age /state /ctl ` + w.home + ` ` + w.secrets + `; do
  [ -e "$p" ] || continue
  grep -rIl -e "$FRAG" -e 'BEGIN PRIVATE KEY' "$p" 2>/dev/null && echo "LEAK in $p"
done
for e in /proc/[0-9]*/environ; do tr '\0' '\n' < "$e" 2>/dev/null | grep -F "$FRAG" && echo "LEAK env $e"; done
ps 2>/dev/null | grep -i '[a]rtifacts [s]igner' && echo "LEAK signer process in the job namespace"
sleep 3; echo scan-done`
	// positive control: the same scan, over a mounted file that does hold the planted value, must report it
	ctl := t.TempDir()
	if err := os.WriteFile(filepath.Join(ctl, "planted"), []byte(w.value), 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan ciexec.AttemptResult, 1)
	go func() {
		done <- (&ciexec.Executor{Grace: time.Second}).Run(context.Background(), ciexec.JobSpec{AttemptID: "leak-job-" + strconv.Itoa(os.Getpid()), CoordinatorID: "test-coord", Home: t.TempDir(),
			Job: model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: image}}, Command: []string{"sh", "-c", job}, Mounts: []ciexec.Mount{{Host: ctl, Target: "/ctl", ReadOnly: true}},
			Output: func(b []byte) { mu.Lock(); jobOut.Write(b); mu.Unlock() }})
	}()
	time.Sleep(1 * time.Second)
	if _, err := client(markerExe(t, marker, "exit 0")).Sign(context.Background(), "run-c", w.items); !signing.IsReason(err, signing.ReasonPayloadRejected) {
		t.Fatalf("the coordinator started a signer while an attempt was running: %v", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a signer process was started while a job of the run was running")
	}
	if out, _ := osexec.Command("pgrep", "-f", exe+" artifacts signer").Output(); len(strings.TrimSpace(string(out))) != 0 {
		t.Fatalf("a signer process exists while a job runs: %s", out)
	}
	select {
	case res := <-done:
		if res.Err != nil || res.ExitCode != 0 {
			t.Fatalf("the scan job failed: %+v", res)
		}
	case <-time.After(90 * time.Second):
		t.Fatal("the scan job hung")
	}
	mu.Lock()
	out := jobOut.String()
	mu.Unlock()
	if !strings.Contains(out, "scan-done") || !strings.Contains(out, "/ctl/planted") {
		t.Fatalf("the scan did not run, or its positive control was not found:\n%s", out)
	}
	if strings.Count(out, "LEAK") != 1 || strings.Contains(out, "LEAK env") || strings.Contains(out, "LEAK signer") {
		t.Fatalf("the malicious job found key material:\n%s", out)
	}

	// 2. after the run: the real signer signs, and the key is in no sink.
	w.finish(t)
	envs, err := client(exe).Sign(context.Background(), "run-c", w.items)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	keys := []sdk.Key{{ID: w.key.ID, Purpose: sdk.PurposeCIRelease, Public: w.key.Public}}
	v, _ := sdk.NewVerifier(sdk.PurposeCIRelease, keys, nil)
	if _, _, _, err := sdk.VerifyEnvelope(v, envs[0]); err != nil {
		t.Fatalf("envelope: %v", err)
	}
	raw, _ := json.Marshal(envs)
	if strings.Contains(string(raw), fragment) {
		t.Fatal("the key is in the signer's output")
	}
	_ = w.st.Close()
	for _, root := range []string{w.home, w.project} {
		_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if b, rerr := os.ReadFile(p); rerr == nil && strings.Contains(string(b), fragment) {
				t.Errorf("the key is in %s", p)
			}
			return nil
		})
	}
	if out, _ := osexec.Command("ps", "-axo", "args").Output(); strings.Contains(string(out), fragment) {
		t.Fatal("the key is in a process argument")
	}
}

func signingCoordinator(st *store.Store) string {
	id, _ := st.AttemptCoordinator(context.Background(), "att-c")
	if id == "" {
		return "local"
	}
	return id
}
