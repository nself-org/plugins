package adhoc

// Purpose: the whole ad-hoc path through a real Coordinator at the process level
// (no Docker needed): env policy, planted secrets, the process warning, E647 and
// the dispatch hook. The container behaviour is in container_test.go (tag container).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

const fakeSHA = "0123456789abcdef0123456789abcdef01234567"

// gitFake answers the revision, status and remote calls of a clean repository.
func gitFake(_ context.Context, argv ...string) (string, int, error) {
	j := strings.Join(argv, " ")
	switch {
	case strings.Contains(j, "rev-parse HEAD"):
		return fakeSHA + "\n", 0, nil
	case strings.Contains(j, "remote get-url"):
		return "git@github.com:acme/widgets.git\n", 0, nil
	}
	return "", 0, nil
}

type rigOut struct {
	h      *Handler
	out    *bytes.Buffer
	errw   *bytes.Buffer
	opts   cirun.Options
	home   string
	called *atomic.Int32
}

func newHandlerRig(t *testing.T, fl Flags) *rigOut {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &rigOut{out: &bytes.Buffer{}, errw: &bytes.Buffer{}, home: t.TempDir(), called: &atomic.Int32{}}
	r.opts = cirun.Options{Root: root, Home: r.home, NoStatus: true}
	r.h = &Handler{Flags: &fl, Select: has(), Git: gitFake, Stdout: r.out, Stderr: r.errw}
	return r
}

func TestProcessJobGetsOnlyTheMinimalEnvironment(t *testing.T) {
	t.Setenv("PLANTED_API_TOKEN", "synthetic-planted-value-123")
	t.Setenv("GOPATH", "/synthetic/gopath")
	r := newHandlerRig(t, Flags{Isolation: "process"})
	envFile := filepath.Join(t.TempDir(), "env.txt")
	r.opts.Adhoc = []string{"sh", "-c", "env | sort > " + envFile}
	exit, err := r.h.Adhoc(context.Background(), r.opts, r.opts.Adhoc)
	if err != nil || exit != 0 {
		t.Fatalf("exit %d err %v\n%s", exit, err, r.out)
	}
	b, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	for _, banned := range []string{"PLANTED_API_TOKEN", "synthetic-planted-value-123", "GOPATH"} {
		if strings.Contains(out, banned) {
			t.Fatalf("%q reached the job:\n%s", banned, out)
		}
	}
	for _, want := range []string{"LANG=", "TMPDIR=", "HOME="} {
		if !strings.Contains(out, want) {
			t.Errorf("job env lacks %s:\n%s", want, out)
		}
	}
	if got := strings.TrimSpace(r.errw.String()); got != ProcessWarning {
		t.Fatalf("warning line = %q", got)
	}
}

func TestSecretInArgvNeverReachesTheStoredRecords(t *testing.T) {
	secret := "synthetic" + "-argv-" + "value-987" // assembled: fake, and no scanner-shaped literal
	r := newHandlerRig(t, Flags{Isolation: "process"})
	argv := []string{"sh", "-c", `echo "got $0"`, "--token=" + secret}
	r.opts.Adhoc = argv
	// the secret is printed by the job itself, so stdout and the stored log carry it unless masked
	exit, err := r.h.Adhoc(context.Background(), r.opts, []string{"sh", "-c", "echo visible-marker; echo " + secret + "; echo " + secret + " >&2; true", "--token=" + secret})
	if err != nil || exit != 0 {
		t.Fatalf("exit %d err %v\n%s", exit, err, r.out)
	}
	if strings.Contains(r.out.String(), secret) {
		t.Fatalf("secret in the output:\n%s", r.out)
	}
	files, positive := 0, false
	err = filepath.WalkDir(r.home, func(p string, d os.DirEntry, werr error) error {
		if werr != nil || d.IsDir() {
			return werr
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		files++
		positive = positive || bytes.Contains(b, []byte("visible-marker"))
		if bytes.Contains(b, []byte(secret)) {
			t.Errorf("secret found in stored file %s", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 || !positive {
		t.Fatalf("positive control failed: %d files read, job output stored = %v", files, positive)
	}
}

func TestUntrustedWithoutTheSandboxRunsNothing(t *testing.T) {
	r := newHandlerRig(t, Flags{Trust: "untrusted"})
	marker := filepath.Join(t.TempDir(), "ran")
	r.opts.Adhoc = []string{"sh", "-c", "touch " + marker}
	exit, err := r.h.Adhoc(context.Background(), r.opts, r.opts.Adhoc)
	if exit == 0 || codeOf(err) != CodeTrustIsolation {
		t.Fatalf("exit %d err %v, want %s", exit, err, CodeTrustIsolation)
	}
	if _, serr := os.Stat(marker); serr == nil {
		t.Fatal("the command ran")
	}
}

func TestNoDockerNoFlagIsE646(t *testing.T) {
	r := newHandlerRig(t, Flags{})
	r.opts.Adhoc = []string{"true"}
	exit, err := r.h.Adhoc(context.Background(), r.opts, r.opts.Adhoc)
	if exit == 0 || codeOf(err) != CodeNoIsolation || !strings.Contains(err.Error(), "--isolation process") {
		t.Fatalf("exit %d err %v", exit, err)
	}
}

func TestAdhocAttemptReachesTheDispatchHook(t *testing.T) {
	saved := cirun.Dispatch
	defer func() { cirun.Dispatch = saved }()
	var got cirun.AttemptSpec
	cirun.Dispatch = func(_ context.Context, s cirun.AttemptSpec) (cirun.Admitted, error) {
		got = s
		return cirun.Admitted{}, errors.New("refused by the test dispatcher")
	}
	r := newHandlerRig(t, Flags{Isolation: "process"})
	r.opts.Adhoc = []string{"true"}
	_, _ = r.h.Adhoc(context.Background(), r.opts, r.opts.Adhoc)
	if got.Pipeline == nil || got.Pipeline.Kind != "adhoc" || got.Job == nil || got.Job.Run[0] != "true" {
		t.Fatalf("the dispatch hook saw %+v", got)
	}
}

func TestAdhocPipelineKindIsAdhoc(t *testing.T) {
	pl, err := buildPlan([]string{"go", "test", "./..."}, Mode{Level: "container", Network: "none"}, "example.invalid/img@sha256:"+strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	if pl.Pipeline.Kind != "adhoc" || pl.Pipeline.Name != "adhoc" {
		t.Fatalf("pipeline %+v", pl.Pipeline)
	}
}

type fakeClassifier struct{ calls int }

func (f *fakeClassifier) Classify(context.Context, cirun.TrustRequest) (model.TrustClass, error) {
	f.calls++
	return "untrusted", nil
}

type failIsolation struct{}

func (failIsolation) Prepare(context.Context, *ciexec.JobSpec) error {
	return errors.New("test isolation stops before any container starts")
}

// A classifier that says untrusted must not make the coordinator refuse a job that the
// handler routes to the sandbox: the run gets as far as the isolation hook.
func TestUntrustedVerdictReachesTheIsolationHook(t *testing.T) {
	cls := &fakeClassifier{}
	r := newHandlerRig(t, Flags{})
	r.h.Select = has("sandboxed-container")
	r.h.Hooks.Trust = cls
	r.h.Hooks.Isolation = failIsolation{}
	r.h.Hooks.Image = alpineStub{}
	r.opts.Adhoc = []string{"true"}
	_, err := r.h.Adhoc(context.Background(), r.opts, r.opts.Adhoc)
	if cirun.CodeOf(err) == cirun.CodeTrust {
		t.Fatalf("the coordinator refused the sandboxed untrusted job: %v", err)
	}
	if cls.calls != 1 {
		t.Fatalf("classifier called %d times, want once (the handler)", cls.calls)
	}
	if !strings.Contains(r.out.String()+errString(err), "test isolation stops") {
		t.Fatalf("the run did not reach the isolation hook: %v\n%s", err, r.out)
	}
}

type alpineStub struct{}

func (alpineStub) Image(context.Context, cirun.AttemptInfo) (string, error) {
	return "example.invalid/img@sha256:" + strings.Repeat("a", 64), nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestShortSecretValueIsReported(t *testing.T) {
	r := newHandlerRig(t, Flags{Isolation: "process"})
	argv := []string{"true", "--token=abc"}
	r.opts.Adhoc = argv
	if _, err := r.h.Adhoc(context.Background(), r.opts, argv); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.errw.String(), "shorter than the redaction floor") {
		t.Fatalf("no note about the unmaskable value:\n%s", r.errw)
	}
	if strings.Contains(r.errw.String(), "abc") {
		t.Fatal("the note repeats the value")
	}
}

// recordEvidence wraps the builder the handler installed and keeps what it returns.
type recordEvidence struct {
	cirun.EvidenceBuilder
	got *model.Evidence
}

func (r recordEvidence) Complete(ctx context.Context, begin any, f cirun.EvidenceFacts, ev model.Evidence) (model.Evidence, error) {
	out, err := r.EvidenceBuilder.Complete(ctx, begin, f, ev)
	*r.got = out
	return out, err
}

// The evidence the real builder produces for an ad-hoc run: kind adhoc, one check whose tool is the
// redacted argv, the level actually used and the network scope.
func TestAdhocEvidenceThroughTheRealBuilder(t *testing.T) {
	secret := "synthetic" + "-evidence-" + "4242" // assembled: fake, and no scanner-shaped literal
	r := newHandlerRig(t, Flags{Isolation: "process"})
	var ev model.Evidence
	r.h.Tune = func(c *cirun.Coordinator) { c.Hooks.Evidence = recordEvidence{c.Hooks.Evidence, &ev} }
	argv := []string{"true", "--api-key=" + secret}
	r.opts.Adhoc = argv
	if exit, err := r.h.Adhoc(context.Background(), r.opts, argv); err != nil || exit != 0 {
		t.Fatalf("exit %d err %v\n%s", exit, err, r.out)
	}
	if ev.Pipeline.Kind != "adhoc" || ev.Runner.Isolation != "process" || len(ev.Checks) != 1 || ev.ToolVersions[KeyNetwork] != "lan" {
		t.Fatalf("evidence = %+v", ev)
	}
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte(secret)) {
		t.Fatal("the secret reached the evidence")
	}
}

type oddClassifier struct{ verdict model.TrustClass }

func (o oddClassifier) Classify(context.Context, cirun.TrustRequest) (model.TrustClass, error) {
	return o.verdict, nil
}

// A classifier that returns nothing usable must not be read as owner.
func TestInvalidClassifierVerdictRunsNothing(t *testing.T) {
	r := newHandlerRig(t, Flags{Isolation: "process"})
	r.h.Hooks.Trust = oddClassifier{verdict: "wat"}
	marker := filepath.Join(t.TempDir(), "ran")
	argv := []string{"sh", "-c", "touch " + marker}
	r.opts.Adhoc = argv
	exit, err := r.h.Adhoc(context.Background(), r.opts, argv)
	if exit == 0 || err == nil {
		t.Fatalf("exit %d err %v", exit, err)
	}
	if _, serr := os.Stat(marker); serr == nil {
		t.Fatal("the command ran")
	}
}

func TestAdhocRetainsClassifiedTrustForSecrets(t *testing.T) {
	r := newHandlerRig(t, Flags{Trust: "collaborator", Isolation: "sandboxed-container"})
	r.h.Select = has("sandboxed-container")
	r.h.Hooks.Isolation = failIsolation{}
	r.h.Hooks.Image = alpineStub{}
	var got model.TrustClass
	r.h.Tune = func(c *cirun.Coordinator) { got = c.Opts.SecretsTrust }
	r.opts.Adhoc = []string{"true"}
	_, _ = r.h.Adhoc(context.Background(), r.opts, r.opts.Adhoc)
	if got != "collaborator" {
		t.Fatalf("secrets saw trust %q", got)
	}
}

func TestAdhocPersistsRequestedSourceTrust(t *testing.T) {
	for _, class := range []string{"internal", "collaborator", "untrusted"} {
		t.Run(class, func(t *testing.T) {
			r := newHandlerRig(t, Flags{Trust: class})
			r.h.Select = has("container", "sandboxed-container")
			r.h.Hooks.Isolation = failIsolation{} // stop before Docker; pipeline must already be persisted
			r.h.Hooks.Image = alpineStub{}
			r.opts.Adhoc = []string{"true"}
			_, err := r.h.Adhoc(context.Background(), r.opts, r.opts.Adhoc)
			if cirun.CodeOf(err) == cirun.CodeTrust {
				t.Fatalf("admission failed: %v", err)
			}
			if !strings.Contains(r.out.String()+errString(err), "test isolation stops") {
				t.Fatal("did not reach isolation hook")
			}
			st, err := cirun.OpenStore(filepath.Join(r.home, "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			rows, err := st.PipelinesByRevision(context.Background(), fakeSHA)
			if err != nil || len(rows) != 1 {
				t.Fatalf("rows=%v err=%v", rows, err)
			}
			if rows[0].SourceTrust != class {
				t.Fatalf("stored source trust=%s want=%s", rows[0].SourceTrust, class)
			}
		})
	}
}

func TestOperatorTypedRecordsLowerTrust(t *testing.T) {
	for _, class := range []model.TrustClass{"owner", "internal", "collaborator", "untrusted"} {
		o := operatorTyped{trust: class}
		if got, _ := o.Classify(context.Background(), cirun.TrustRequest{}); got != "owner" {
			t.Fatalf("admission=%s", got)
		}
		if got := o.RecordedTrust(); got != class {
			t.Fatalf("recorded=%s want=%s", got, class)
		}
	}
}

// Under NSELF_V15 the dispatcher places the job on the local runner, which accepts only
// owner. The recorded trust stays what the operator asked for; the placement uses the
// admitted class, so a lower class must still run.
func TestAdhocV15PlacesLowerTrustJobs(t *testing.T) {
	for _, class := range []string{"internal", "collaborator", "untrusted"} {
		t.Run(class, func(t *testing.T) {
			t.Setenv("NSELF_V15", "1")
			fl := Flags{Isolation: "process", Trust: class}
			if class != "internal" {
				fl.Isolation = "sandboxed-container"
			}
			r := newHandlerRig(t, fl)
			r.h.Select = has("container", "sandboxed-container")
			r.h.Hooks.Isolation = failIsolation{} // placement happens before isolation; stop before Docker
			r.h.Hooks.Image = alpineStub{}
			r.opts.Adhoc = []string{"true"}
			exit, err := r.h.Adhoc(context.Background(), r.opts, r.opts.Adhoc)
			out := r.out.String() + errString(err)
			if placed := strings.Contains(out, "test isolation stops") || (err == nil && exit == 0); !placed {
				t.Fatalf("job was not placed under v1.5: %v\n%s", err, out)
			}
			st, err := cirun.OpenStore(filepath.Join(r.home, "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			rows, err := st.PipelinesByRevision(context.Background(), fakeSHA)
			if err != nil || len(rows) != 1 || rows[0].SourceTrust != class {
				t.Fatalf("rows=%v err=%v want stored trust %s", rows, err, class)
			}
		})
	}
}
