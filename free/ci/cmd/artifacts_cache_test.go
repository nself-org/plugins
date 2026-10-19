package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/storetest"
)

// seedCache commits one cache entry through a whole passing attempt of run
// "run<id>" and records one usage row for it.
func (r *artRig) seedCache(id, content string) entries.Manifest {
	r.t.Helper()
	ctx := context.Background()
	f := entries.Facts{Project: r.project, Trust: "owner", Protection: "protected"}
	ns, err := entries.NamespaceOf(f)
	if err != nil {
		r.t.Fatal(err)
	}
	key, err := entries.Key(entries.KeyInputs{Namespace: ns, SpecID: "go", SpecVersion: "1", Platform: "linux/amd64", Isolation: "process", Extras: []string{id}})
	if err != nil {
		r.t.Fatal(err)
	}
	es := entries.New(r.st, r.blobs, entries.Options{Home: r.home, Free: func() (int64, int64, error) { return 1 << 40, 1 << 41, nil }})
	p := store.PipelineRow{ID: "run" + id, Revision: "rev", Project: r.project, Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	steps := []error{r.st.CreatePipeline(ctx, p), r.st.CreateJob(ctx, store.JobRow{ID: "j" + id, PipelineID: p.ID, Name: "build", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}),
		r.st.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "j" + id, N: 1}, ""),
		storetest.Admit(ctx, r.st, id), r.st.Transition(ctx, id, "leased", "running", store.TransitionDetail{Epoch: 1}),
		es.RecordPlan(ctx, id, 2, f, []string{key}), es.RecordUse(ctx, entries.Use{Attempt: id, Spec: "go", Key: key, Result: "miss"}),
		r.st.Transition(ctx, id, "running", "finalizing", store.TransitionDetail{Epoch: 2}), es.RecordResult(ctx, id, 3, true)}
	for i, e := range steps {
		if e != nil {
			r.t.Fatalf("seed %s step %d: %v", id, i, e)
		}
	}
	digest, size, err := r.blobs.Put(ctx, cas.CacheRealm(string(ns.Canonical())), strings.NewReader(content))
	if err != nil {
		r.t.Fatal(err)
	}
	m := entries.Manifest{Namespace: ns.Canonical(), Key: key, Spec: "go", Blobs: []entries.Blob{{Path: "mod", SHA256: digest, Size: size}}, Writer: entries.Writer{ScannedClean: true}}
	if err := es.Commit(ctx, id, 3, ns, m); err != nil {
		r.t.Fatal(err)
	}
	return m
}

func runCache(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	return runArt(t, append([]string{"--cache"}, args...)...)
}

func data(t *testing.T, out string) any { t.Helper(); return checkEnvelope(t, out)["data"] }

func TestArtifactsCacheShowVerifyPrune(t *testing.T) {
	r := newArtRig(t)
	t.Setenv("NSELF_CI_STORE_MIN_FREE", "1")
	a := r.seedCache("a1", "blob-one")
	b := r.seedCache("b1", "blob-two")

	code, out, errs := runCache(t, "list", "--json")
	list, _ := data(t, out).([]any)
	if code != 0 || len(list) != 2 || errs != "" {
		t.Fatalf("list: %d %s %s", code, out, errs)
	}
	e0 := list[0].(map[string]any)
	if e0["spec"] != "go" || e0["trust"] != "owner" || e0["protection"] != "protected" || e0["scanned_clean"] != true || e0["secrets_received"] != false {
		t.Fatalf("entry view: %v", e0)
	}
	if code, out, _ = runCache(t, "list", "--run", "runa1", "--json"); code != 0 || len(data(t, out).([]any)) != 1 || data(t, out).([]any)[0].(map[string]any)["key"] != a.Key {
		t.Fatalf("list --run: %d %s", code, out)
	}
	if code, out, _ = runCache(t, "list"); code != 0 || !strings.Contains(out, shortDigest(a.Key)) || !strings.Contains(out, shortDigest(b.Key)) {
		t.Fatalf("list text: %d %s", code, out)
	}

	code, out, _ = runCache(t, "show", "--run", "runa1", "--json")
	rows, _ := data(t, out).([]any)
	if code != 0 || len(rows) != 1 || rows[0].(map[string]any)["result"] != "miss" || rows[0].(map[string]any)["spec"] != "go" || rows[0].(map[string]any)["attempt"] != "a1" {
		t.Fatalf("show --run: %d %s", code, out)
	}
	if code, out, _ = runCache(t, "show", a.Key[:12]); code != 0 || !strings.Contains(out, "key "+a.Key) || !strings.Contains(out, "blob mod "+a.Blobs[0].SHA256[:12]) {
		t.Fatalf("show <key>: %d %s", code, out)
	}
	if code, _, errs = runCache(t, "show", "--run", "nope"); code != 1 || !strings.Contains(errs, "E713") {
		t.Fatalf("unknown run: %d %s", code, errs)
	}
	if code, _, _ = runCache(t, "show"); code != 1 {
		t.Fatalf("show without a target: %d", code)
	}

	// verify re-hashes every blob; it reports and drops nothing
	code, out, _ = runCache(t, "verify", "--json")
	if got := data(t, out).(map[string]any); code != 0 || got["checked"] != float64(2) || got["corrupt"] != float64(0) {
		t.Fatalf("verify clean: %d %v", code, got)
	}
	blobPath := func(m entries.Manifest) string {
		d := m.Blobs[0].SHA256
		return filepath.Join(r.home, "blobs", string(cas.CacheRealm(string(m.Namespace))), "sha256", d[:2], d)
	}
	if err := os.WriteFile(blobPath(a), []byte("blob-ONE"), 0o600); err != nil { // same size, other bytes
		t.Fatal(err)
	}
	if err := os.Remove(blobPath(b)); err != nil {
		t.Fatal(err)
	}
	code, out, _ = runCache(t, "verify", "--json")
	got := data(t, out).(map[string]any)
	states := map[string]string{}
	for _, c := range got["entries"].([]any) {
		states[c.(map[string]any)["key"].(string)] = c.(map[string]any)["state"].(string)
	}
	if code != 1 || got["corrupt"] != float64(2) || states[a.Key] != "corrupt" || states[b.Key] != "missing" {
		t.Fatalf("verify damaged: %d %v", code, got)
	}
	if list, _ := r.st.CacheEntries(context.Background(), ""); len(list) != 2 {
		t.Fatal("verify is a read view: it must not drop entries")
	}
	if code, out, _ = runCache(t, "verify", "--run", "runb1", "--json"); code != 1 || data(t, out).(map[string]any)["checked"] != float64(1) {
		t.Fatalf("verify --run: %d %s", code, out)
	}

	// prune: nothing is over a cap yet; with a tiny age cap both entries go
	if code, out, _ = runCache(t, "prune", "--dry-run", "--json"); code != 0 || len(data(t, out).(map[string]any)["entries"].([]any)) != 0 {
		t.Fatalf("prune dry run, no pressure: %d %s", code, out)
	}
	t.Setenv("NSELF_CI_CACHE_MAX_AGE", "1ns")
	code, out, _ = runCache(t, "prune", "--dry-run", "--json")
	dry := data(t, out).(map[string]any)
	if code != 0 || dry["dry_run"] != true || len(dry["entries"].([]any)) != 2 {
		t.Fatalf("prune dry run: %d %v", code, dry)
	}
	if list, _ := r.st.CacheEntries(context.Background(), ""); len(list) != 2 {
		t.Fatal("--dry-run removed entries")
	}
	t.Setenv("NSELF_CI_ROLE", "member")
	if code, _, errs = runCache(t, "prune"); code != 1 || !strings.Contains(errs, "admin") {
		t.Fatalf("prune as a member: %d %s", code, errs)
	}
	t.Setenv("NSELF_CI_ROLE", string(authz.RoleAdmin))
	code, out, _ = runCache(t, "prune", "--json")
	pruned := data(t, out).(map[string]any)
	if code != 0 || pruned["dry_run"] != false || pruned["entries"] != float64(2) {
		t.Fatalf("prune: %d %v", code, pruned)
	}
	if list, _ := r.st.CacheEntries(context.Background(), ""); len(list) != 0 {
		t.Fatalf("entries left after prune: %d", len(list))
	}
}

// TestArtifactsCacheRunOfAnotherProjectIsNotFound: a viewer never reads the
// usage rows of a run that belongs to another project.
func TestArtifactsCacheRunOfAnotherProjectIsNotFound(t *testing.T) {
	r := newArtRig(t)
	ctx := context.Background()
	p := store.PipelineRow{ID: "runforeign", Revision: "rev", Project: "/some/other/project", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	for i, e := range []error{r.st.CreatePipeline(ctx, p), r.st.CreateJob(ctx, store.JobRow{ID: "jforeign", PipelineID: p.ID, Name: "build", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}),
		r.st.CreateAttempt(ctx, store.AttemptRow{ID: "foreign1", JobID: "jforeign", N: 1}, ""),
		entries.New(r.st, r.blobs, entries.Options{Home: r.home}).RecordUse(ctx, entries.Use{Attempt: "foreign1", Spec: "go", Key: strings.Repeat("ab", 32), Result: "hit", Bytes: 7})} {
		if e != nil {
			t.Fatalf("seed step %d: %v", i, e)
		}
	}
	for _, args := range [][]string{{"show", "--run", "runforeign", "--json"}, {"list", "--run", "runforeign"}, {"verify", "--run", "runforeign"}} {
		code, out, errs := runCache(t, args...)
		if code != 1 || !strings.Contains(errs+out, "E713") || strings.Contains(errs+out, "foreign1") || strings.Contains(errs+out, strings.Repeat("ab", 6)) {
			t.Fatalf("%v: %d out=%q err=%q", args, code, out, errs)
		}
	}
}

func TestArtifactsCacheUsage(t *testing.T) {
	newArtRig(t)
	for _, args := range [][]string{{}, {"nope"}, {"show", "--run", "r", "extra"}} {
		if code, _, errs := runCache(t, args...); code != 1 || errs == "" {
			t.Fatalf("%v: %d %q", args, code, errs)
		}
	}
}
