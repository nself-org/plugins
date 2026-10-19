package entries_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
)

func TestRecordUseStoresValidatedRowsOnly(t *testing.T) {
	e := newEnv(t, envOpts{})
	key := strings.Repeat("a", 64)
	for _, res := range []string{"hit", "miss", "disabled", "readonly", "denied", "refused", "saved"} {
		if err := e.es.RecordUse(e.ctx, entries.Use{Attempt: "u1", Spec: "go", Key: key, Result: res, Bytes: 12, MS: 7}); err != nil {
			t.Fatalf("%s: %v", res, err)
		}
		e.advance(time.Second)
	}
	if err := e.es.RecordUse(e.ctx, entries.Use{Attempt: "u1", Spec: "pnpm", Result: "miss"}); err != nil {
		t.Fatalf("a use without a key (cache disabled) is valid: %v", err)
	}
	rows, err := e.es.Usage(e.ctx, "u1")
	if err != nil || len(rows) != 8 {
		t.Fatalf("rows %d %v", len(rows), err)
	}
	if rows[0].Result != "hit" || rows[0].Bytes != 12 || rows[0].MS != 7 || rows[0].Spec != "go" || rows[0].Key != key || rows[5].Result != "refused" || rows[6].Result != "saved" || rows[7].Key != "" {
		t.Fatalf("rows %+v", rows)
	}
	if rows[1].TS-rows[0].TS != int64(time.Second) {
		t.Fatalf("the backend must stamp the time: %d %d", rows[0].TS, rows[1].TS)
	}
	for name, u := range map[string]entries.Use{
		"bad attempt": {Attempt: "u 1", Spec: "go", Result: "hit"}, "empty attempt": {Spec: "go", Result: "hit"},
		"bad spec": {Attempt: "u1", Spec: "GOFLAGS=-v", Result: "hit"}, "bad key": {Attempt: "u1", Spec: "go", Key: "zz", Result: "hit"},
		"bad result": {Attempt: "u1", Spec: "go", Result: "restored"}, "negative bytes": {Attempt: "u1", Spec: "go", Result: "hit", Bytes: -1},
		"negative ms": {Attempt: "u1", Spec: "go", Result: "hit", MS: -1},
	} {
		if err := e.es.RecordUse(e.ctx, u); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if other, _ := e.es.Usage(e.ctx, "nobody"); len(other) != 0 {
		t.Fatal("usage leaked across attempts")
	}
}

// TestNoEnvOrSecretValueReachesTheDatabase plants an env value and a secret
// value, runs a full save, restore, usage and prune cycle, and greps the whole
// database file set. A positive control proves the grep sees what is written.
func TestNoEnvOrSecretValueReachesTheDatabase(t *testing.T) {
	const planted = "planted-env-value-7f3a9c1e-DO-NOT-STORE"
	t.Setenv("GOFLAGS", "-ldflags=-X=main.token="+planted)
	t.Setenv("NSELF_PLANTED_SECRET", planted)
	e := newEnv(t, envOpts{})
	m := e.save("g1", trusted, "k", "tarball bytes")
	e.lookup(trusted, m.Key)
	if err := e.es.RecordUse(e.ctx, entries.Use{Attempt: "g1", Spec: "go", Key: m.Key, Result: "hit", Bytes: 1, MS: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.es.Prune(e.ctx, 0); err != nil {
		t.Fatal(err)
	}
	enc, err := entries.EncodeManifest(m)
	if err != nil || strings.Contains(string(enc), planted) {
		t.Fatalf("manifest holds the planted value: %v", err)
	}
	grep := func() (hits, files int) {
		for _, suffix := range []string{"", "-wal", "-shm"} {
			b, err := os.ReadFile(filepath.Join(e.home, "state.db"+suffix))
			if err != nil {
				continue
			}
			files++
			if strings.Contains(string(b), planted) {
				hits++
			}
		}
		return hits, files
	}
	if hits, files := grep(); hits != 0 || files == 0 {
		t.Fatalf("planted value found in %d of %d database files", hits, files)
	}
	if err := e.st.AppendAudit(e.ctx, "test", "control", "t", planted); err != nil {
		t.Fatal(err)
	}
	if hits, _ := grep(); hits == 0 {
		t.Fatal("positive control failed: the grep cannot see database content")
	}
}

func TestConcurrentSavesLookupsAndPrunes(t *testing.T) {
	e := newEnv(t, envOpts{env: map[string]string{"NSELF_CI_CACHE_MAX_BYTES": "200"}})
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			f := trusted
			f.Project = "proj" + string(rune('a'+i))
			id := "cc" + string(rune('a'+i))
			key := e.key(f, id)
			fin := e.attempt(id, f, []string{key}, true)
			m := e.manifest(f, key, strings.Repeat("d", 30+i))
			if err := e.es.Commit(e.ctx, id, fin, e.ns(f), m); err != nil {
				errs <- err
			}
			for j := 0; j < 5; j++ {
				if _, _, err := e.es.Lookup(e.ctx, e.ns(f), key, "rw-own"); err != nil {
					errs <- err
				}
				if _, err := e.es.Prune(e.ctx, time.Second); err != nil {
					errs <- err
				}
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if rep, err := e.es.Verify(e.ctx); err != nil || rep.Dropped != 0 {
		t.Fatalf("a concurrent prune left a damaged entry: %+v %v", rep, err)
	}
}
