package main

// Purpose: the `cache` contributor registered in this binary, through a real world build.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/cache/locality"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

type cacheLedger struct {
	entries map[string][]entries.Manifest
	rows    []store.CacheLocalityRow
}

func (l cacheLedger) CacheEntries(_ context.Context, key string) ([]entries.Manifest, error) {
	return l.entries[key], nil
}

func (l cacheLedger) CacheLocality(context.Context, string, string, string) ([]store.CacheLocalityRow, error) {
	return l.rows, nil
}

func capability(t *testing.T, name, id string) model.Capability {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "internal", "model", "testdata", "capability", "valid", name+".valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	c.Identity.ID = id
	return c
}

// TestWorldCacheFacts: the local machine holds the entry, node B holds a read-through copy, node C
// holds nothing, and a job without a spec is unknown; the world digest is stable across two builds.
func TestWorldCacheFacts(t *testing.T) {
	ctx := context.Background()
	const image = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	now := time.Unix(1_000_000, 0)
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	local := capability(t, "laptop", "local")
	nodeB, nodeC := capability(t, "lan-mac", "01J00000000000000000000002"), capability(t, "vps", "01J00000000000000000000003")
	for _, c := range []model.Capability{nodeB, nodeC} {
		doc, e := json.Marshal(c)
		if e != nil {
			t.Fatal(e)
		}
		if err = st.NodePut(ctx, store.NodeRecord{ID: c.Identity.ID, Name: c.Identity.Name, State: "online", Capability: doc}, "operator"); err != nil {
			t.Fatal(err)
		}
	}
	facts := entries.Facts{Project: "/work/proj", Trust: "owner", Protection: "protected"}
	detected := []detect.Fact{{Kind: "stack", Name: "go", Path: "go.mod", Confidence: detect.Certain},
		{Kind: "lockfile", Name: "go", Path: "go.sum", SHA256: strings.Repeat("c3", 32), Confidence: detect.Certain}}
	ns, err := entries.NamespaceOf(facts)
	if err != nil {
		t.Fatal(err)
	}
	realm := base64.RawURLEncoding.EncodeToString(ns.Canonical())
	ledger := cacheLedger{entries: map[string][]entries.Manifest{}}
	keyOn := map[string]string{}
	for _, platform := range []string{"darwin/arm64", "linux/arm64"} {
		mat, e := specs.Resolve(ctx, specs.Input{Spec: mustSpec(t, "go"), Platform: platform, Isolation: "process", Image: image, Facts: detected})
		if e != nil {
			t.Fatal(e)
		}
		key, e := mat.Key(ns)
		if e != nil {
			t.Fatal(e)
		}
		keyOn[platform] = key
		ledger.entries[key] = []entries.Manifest{{Namespace: ns.Canonical(), Key: key, Blobs: []entries.Blob{{Path: "mod", SHA256: strings.Repeat("0", 64), Size: 4096}}}}
	}
	ledger.rows = []store.CacheLocalityRow{{Node: nodeB.Identity.ID, Realm: realm, Key: keyOn["darwin/arm64"], Bytes: 4096, LastSeen: now}}
	unbind := locality.Bind(func(context.Context) (locality.Inputs, error) {
		return locality.Inputs{Ledger: ledger, Jobs: []locality.Job{
			{Key: "build[linux]", Facts: facts, Detected: detected, Image: image},
			{Key: "docs", Facts: facts, Image: image},
		}}, nil
	})
	defer unbind()
	deps := world.Deps{Store: st, Local: &local, Clock: func() time.Time { return now },
		Policy: func(context.Context, string) (sched.Policy, error) { return sched.Policy{}, nil }}
	w, digest, err := world.Build(ctx, deps, sched.Pipeline{ID: "pipe"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for runner, jobs := range w.Cache {
		for job, f := range jobs {
			s := "nil"
			if f.MissBytes != nil {
				s = fmt.Sprint(*f.MissBytes)
			}
			got[runner+"/"+job] = s + "/" + f.Confidence
		}
	}
	want := map[string]string{
		"local/build[linux]": "0/known", "local/docs": "nil/unknown",
		nodeB.Identity.ID + "/build[linux]": "0/known", nodeB.Identity.ID + "/docs": "nil/unknown",
		nodeC.Identity.ID + "/build[linux]": "4096/known", nodeC.Identity.ID + "/docs": "nil/unknown",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) { // fmt prints maps sorted by key
		t.Fatalf("cache facts\n got %v\nwant %v", got, want)
	}
	_, again, err := world.Build(ctx, deps, sched.Pipeline{ID: "pipe"})
	if err != nil || again != digest {
		t.Fatalf("digest not stable: %q vs %q (%v)", digest, again, err)
	}
}

func mustSpec(t *testing.T, id string) specs.Spec {
	t.Helper()
	sp, ok := specs.Get(id)
	if !ok {
		t.Fatalf("spec %s not registered", id)
	}
	return sp
}
