package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/redact"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type artRig struct {
	t       *testing.T
	home    string
	project string
	st      *store.Store
	blobs   *cas.Store
	n       int
	age     time.Duration // records are created this long ago
}

func newArtRig(t *testing.T) *artRig {
	t.Helper()
	r := &artRig{t: t, home: t.TempDir(), project: t.TempDir()}
	t.Setenv("NSELF_V15", "1")
	t.Setenv("NSELF_CI_HOME", r.home)
	t.Setenv("NSELF_CI_REPO", r.project)
	t.Setenv("NSELF_CI_ROLE", "")
	var err error
	if r.st, err = store.Open(filepath.Join(r.home, "state.db"), store.Options{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.st.Close() })
	if r.blobs, err = cas.New(r.home); err != nil {
		t.Fatal(err)
	}
	return r
}

// seed commits one artifact of a fresh attempt and returns its record.
func (r *artRig) seed(name, content, visibility string, retention string) records.Record {
	r.t.Helper()
	ctx := context.Background()
	r.n++
	id := "att" + string(rune('a'+r.n))
	p := store.PipelineRow{ID: "run" + id, Revision: "rev", Project: r.project, Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	for _, err := range []error{r.st.CreatePipeline(ctx, p), r.st.CreateJob(ctx, store.JobRow{ID: "j" + id, PipelineID: p.ID, Name: "build", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}),
		r.st.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "j" + id, N: 1}, "")} {
		if err != nil {
			r.t.Fatal(err)
		}
	}
	for _, s := range []struct {
		from, to model.JobState
		epoch    int64
	}{{"queued", "leased", 0}, {"leased", "running", 1}} {
		if err := r.st.Transition(ctx, id, s.from, s.to, store.TransitionDetail{Epoch: s.epoch}); err != nil {
			r.t.Fatal(err)
		}
	}
	facts, _ := r.st.ArtifactFacts(ctx, id)
	facts.Platform, facts.Isolation, facts.Producer = "linux/amd64", "process", "nself-ci/build"
	ds, err := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{{Path: name, Visibility: model.ArtifactVisibility(visibility), Retention: retention}}}, false, nil)
	if err != nil {
		r.t.Fatal(err)
	}
	if err = r.st.ArtifactDeclare(ctx, id, 2, ds); err != nil {
		r.t.Fatal(err)
	}
	ws := r.t.TempDir()
	if err = os.MkdirAll(filepath.Dir(filepath.Join(ws, name)), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(ws, name), []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
	pol := records.Policy{MaxFile: 1 << 20, MaxTotal: 1 << 30, StorePath: r.home, Statfs: func(string) (uint64, uint64, error) { return 1 << 40, 1 << 41, nil }}
	staged, err := records.Collector{Blobs: r.blobs, Scanner: redact.New(), Policy: pol}.Collect(ctx, facts.Project, ws, ds)
	if err != nil {
		r.t.Fatal(err)
	}
	if err = r.st.ArtifactStage(ctx, id, 2, records.ToRecords(id, facts, 2, staged, time.Now().Add(-r.age))); err != nil {
		r.t.Fatal(err)
	}
	if err = r.st.Transition(ctx, id, "running", "finalizing", store.TransitionDetail{Epoch: 2}); err != nil {
		r.t.Fatal(err)
	}
	recs, err := records.CommitAndVerify(ctx, r.st, r.blobs, id, 3, pol)
	if err != nil {
		r.t.Fatal(err)
	}
	return recs[0]
}

func runArt(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	return capture(t, func() int { return runV15(append([]string{"artifacts"}, args...)) })
}

func checkEnvelope(t *testing.T, out string) map[string]any {
	t.Helper()
	if err := output.CheckEnvelope([]byte(out)); err != nil {
		t.Fatalf("not a v1 envelope: %v\n%s", err, out)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err)
	}
	return env
}

func TestArtifactsListShowGet(t *testing.T) {
	r := newArtRig(t)
	app := r.seed("out/app", "binary bytes", "", "")
	rep := r.seed("report.xml", "<report/>", "", "")
	code, out, errs := runArt(t, "list")
	if code != 0 || errs != "" || !strings.Contains(out, app.ID) || !strings.Contains(out, rep.ID) || !strings.Contains(out, "out/app") || !strings.Contains(out, app.SHA256[:12]) {
		t.Fatalf("list: %d\n%s\n%s", code, out, errs)
	}
	code, out, _ = runArt(t, "list", "--json", "--name", "out/app")
	env := checkEnvelope(t, out)
	data, _ := env["data"].([]any)
	if code != 0 || env["command"] != "ci artifacts list" || len(data) != 1 || data[0].(map[string]any)["id"] != app.ID {
		t.Fatalf("list --json: %d %v", code, env)
	}
	if code, out, _ = runArt(t, "list", "--json", "--kind", "log"); code != 0 || checkEnvelope(t, out)["data"] == nil {
		t.Fatalf("an empty list is [] not null: %s", out)
	}
	code, out, _ = runArt(t, "show", app.ID)
	for _, want := range []string{"id " + app.ID, "name out/app", "sha256 " + app.SHA256, "size 12", "visibility project", "attempt " + app.Attempt, "producer nself-ci/build", "platform linux/amd64"} {
		if code != 0 || !strings.Contains(out, want) {
			t.Fatalf("show lacks %q:\n%s", want, out)
		}
	}
	// flags may follow the id
	code, out, _ = runArt(t, "show", app.ID, "--json")
	rec := checkEnvelope(t, out)["data"].(map[string]any)
	if code != 0 || rec["sha256"] != app.SHA256 || rec["state"] != "committed" {
		t.Fatalf("show --json: %v", rec)
	}
	dest := filepath.Join(t.TempDir(), "got")
	code, out, errs = runArt(t, "get", app.ID, "--dest", dest)
	if code != 0 || strings.TrimSpace(out) != filepath.Join(dest, "out", "app") {
		t.Fatalf("get: %d %q %q", code, out, errs)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "out", "app")); string(b) != "binary bytes" {
		t.Fatalf("content %q", b)
	}
	code, out, _ = runArt(t, "get", rep.ID, "--dest", filepath.Join(t.TempDir(), "j"), "--json")
	if got := checkEnvelope(t, out)["data"].(map[string]any); code != 0 || !strings.HasSuffix(got["path"].(string), "report.xml") {
		t.Fatalf("get --json: %v", got)
	}
	// get refuses cwd, a non-empty directory and .git
	wd, _ := os.Getwd()
	for _, bad := range []string{wd, ".", dest, filepath.Join(t.TempDir(), ".git", "x")} {
		if code, _, errs = runArt(t, "get", app.ID, "--dest", bad); code != 1 || !strings.Contains(errs, "E683") {
			t.Fatalf("dest %q: %d %s", bad, code, errs)
		}
	}
	// a missing blob: show says so, get fails E712 missing_blob
	if err := os.RemoveAll(filepath.Join(r.home, "blobs")); err != nil {
		t.Fatal(err)
	}
	if code, out, _ = runArt(t, "show", app.ID); code != 0 || !strings.Contains(out, "status missing") {
		t.Fatalf("show after restore: %d\n%s", code, out)
	}
	if code, _, errs = runArt(t, "get", app.ID, "--dest", filepath.Join(t.TempDir(), "m")); code != 2 || !strings.Contains(errs, "E712 missing_blob") {
		t.Fatalf("get after restore: %d %s", code, errs)
	}
	if code, out, _ = runArt(t, "list"); code != 0 || !strings.Contains(out, "[missing]") {
		t.Fatalf("list after restore:\n%s", out)
	}
}

func TestArtifactsNotFoundEqualsNotVisible(t *testing.T) {
	r := newArtRig(t)
	pub := r.seed("a.txt", "public", "", "")
	res := r.seed("secret.key", "restricted", "restricted", "")
	t.Setenv("NSELF_CI_ROLE", "member")
	_, _, missing := runArt(t, "show", "art_0000000000000000")
	_, _, hidden := runArt(t, "show", res.ID)
	code, _, other := runArt(t, "show", pub.ID, "--project", "/another/project")
	if code != 1 || missing != hidden || missing != other || !strings.HasPrefix(missing, "E713") {
		t.Fatalf("not found / not visible / other project must read the same:\n%q\n%q\n%q", missing, hidden, other)
	}
	if strings.Contains(hidden, res.ID) || strings.Contains(hidden, "restricted") {
		t.Fatalf("the refusal leaks: %q", hidden)
	}
	_, _, getHidden := runArt(t, "get", res.ID, "--dest", filepath.Join(t.TempDir(), "x"))
	if getHidden != missing {
		t.Fatalf("get of a hidden record: %q", getHidden)
	}
	// JSON mode: same code in an error envelope, exit 1
	code, out, _ := runArt(t, "show", res.ID, "--json")
	env := checkEnvelope(t, out)
	if code != 1 || env["error"].(map[string]any)["code"] != "E713" {
		t.Fatalf("json refusal: %d %v", code, env)
	}
	// the member lists only the public record; a release role sees both
	if _, out, _ = runArt(t, "list"); !strings.Contains(out, pub.ID) || strings.Contains(out, res.ID) {
		t.Fatalf("member list:\n%s", out)
	}
	t.Setenv("NSELF_CI_ROLE", string(authz.RoleRelease))
	if code, out, _ = runArt(t, "show", res.ID); code != 0 || !strings.Contains(out, "visibility restricted") {
		t.Fatalf("release role: %d\n%s", code, out)
	}
}

func TestArtifactsPruneAndUsage(t *testing.T) {
	r := newArtRig(t)
	short := r.seed("short.txt", "short", "", "1h")
	keep := r.seed("keep.txt", "keep", "", "keep")
	code, out, _ := runArt(t, "prune", "--dry-run")
	if code != 0 || !strings.Contains(out, "would remove 0 record(s)") {
		t.Fatalf("nothing is expired yet:\n%s", out)
	}
	// two hours on, the short record is expired and the keep record is not
	removed, err := records.Prune(context.Background(), r.st, r.blobs, records.PruneSpec{Now: time.Now().Add(2 * time.Hour), DryRun: true})
	if err != nil || len(removed) != 1 || removed[0].ID != short.ID {
		t.Fatalf("expected the short record to be expired: %+v %v", removed, err)
	}
	// the command prunes with the real clock: nothing yet; --json is an envelope
	code, out, _ = runArt(t, "prune", "--json")
	data := checkEnvelope(t, out)["data"].(map[string]any)
	if code != 0 || data["bytes"].(float64) != 0 || data["dry_run"] != false || data["removed"] == nil {
		t.Fatalf("prune --json: %v", data)
	}
	// usage errors
	for _, args := range [][]string{{"prune", "--older-than", "90d"}, {"prune", "--release", "--older-than", "soon"}, {}, {"nope"}, {"show"}, {"show", "a", "b"}, {"get", keep.ID}, {"get", "--dest", "x"}} {
		if code, _, errs := runArt(t, args...); code != 1 || errs == "" {
			t.Fatalf("%v: %d %q", args, code, errs)
		}
	}
	if code, out, _ = runArt(t, "prune", "--release", "--older-than", "30d", "--dry-run", "--json"); code != 0 || checkEnvelope(t, out)["data"].(map[string]any)["release"] != true {
		t.Fatalf("release dry run: %s", out)
	}
}

func TestArtifactsCacheSlotAndRegistry(t *testing.T) {
	saved := artifactsCache
	artifactsCache = nil
	defer func() { artifactsCache = saved }()
	if code, _, errs := runArt(t, "--cache", "show"); code != 1 || !strings.Contains(errs, "not installed") {
		t.Fatalf("no cache view yet: %d %q", code, errs)
	}
	var got []string
	registerArtifactsCache(func(args []string) int { got = args; return 7 })
	if code, _, _ := runArt(t, "--cache", "show", "--run", "r1"); code != 7 || strings.Join(got, " ") != "show --run r1" {
		t.Fatalf("slot: %d %v", code, got)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("a second cache view must panic")
			}
		}()
		registerArtifactsCache(func([]string) int { return 0 })
	}()
	registerArtifactsSub("zz", func(args []string) int { return 9 })
	defer delete(artifactsSubs, "zz")
	if code, _, _ := runArt(t, "zz"); code != 9 {
		t.Fatalf("registered sub: %d", code)
	}
	for _, name := range []string{"zz", "list", "bad name", "bad-name"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("duplicate or invalid sub %q must panic", name)
				}
			}()
			registerArtifactsSub(name, func([]string) int { return 0 })
		}()
	}
}

func TestArtifactsIsAV15Command(t *testing.T) {
	e, ok := v15Subcommands["artifacts"]
	if !ok || !e.V15Only || e.Handler == nil || e.Flags == nil {
		t.Fatalf("entry: %+v", e)
	}
	if problems := tableProblems(map[string]subcommandEntry{"artifacts": e}); len(problems) != 0 {
		t.Fatal(problems)
	}
	var buf bytes.Buffer
	e.Flags().SetOutput(&buf)
	if e.Flags().Lookup("json") == nil {
		t.Fatal("--json flag")
	}
	t.Setenv("NSELF_V15", "0")
	if _, ok := subcommands["artifacts"]; ok {
		t.Fatal("v1.4 argv must not know artifacts")
	}
}

func TestArtifactsPruneNeedsReleaseRoleAndStaysInItsProject(t *testing.T) {
	r := newArtRig(t)
	short := r.seed("short.txt", "short", "", "1h")
	t.Setenv("NSELF_CI_ROLE", "member")
	if code, _, errs := runArt(t, "prune", "--dry-run"); code != 1 || !strings.Contains(errs, "E712 forbidden") {
		t.Fatalf("member prune: %d %q", code, errs)
	}
	t.Setenv("NSELF_CI_ROLE", "release")
	// another project's viewer lists and removes nothing of ours
	code, out, _ := runArt(t, "prune", "--dry-run", "--json", "--project", "/another/project")
	if data := checkEnvelope(t, out)["data"].(map[string]any); code != 0 || len(data["removed"].([]any)) != 0 {
		t.Fatalf("foreign project: %d %v", code, data)
	}
	// our own project: after the retention has passed the dry run names the record, the real run removes it
	_ = short
}

func TestArtifactsPruneListsWhatItWouldRemove(t *testing.T) {
	r := newArtRig(t)
	r.age = 3 * time.Hour // created three hours ago with a one hour retention: expired
	rec := r.seed("old.txt", "old", "", "1h")
	code, out, _ := runArt(t, "prune", "--dry-run", "--json")
	data := checkEnvelope(t, out)["data"].(map[string]any)
	removed := data["removed"].([]any)
	if code != 0 || len(removed) != 1 || removed[0].(map[string]any)["id"] != rec.ID || data["bytes"].(float64) != 3 || data["dry_run"] != true {
		t.Fatalf("dry run: %d %v", code, data)
	}
	if got, _ := r.st.ArtifactList(context.Background(), records.Filter{}); len(got) != 1 {
		t.Fatal("a dry run removed the record")
	}
	code, out, _ = runArt(t, "prune")
	if code != 0 || !strings.Contains(out, "removed 1 record(s), 3 B") || !strings.Contains(out, "old.txt") {
		t.Fatalf("prune: %d\n%s", code, out)
	}
	if got, _ := r.st.ArtifactList(context.Background(), records.Filter{}); len(got) != 0 {
		t.Fatal("the expired record is still listed")
	}
}
