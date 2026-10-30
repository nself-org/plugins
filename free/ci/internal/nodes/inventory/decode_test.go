package inventory

// Purpose: tests of the inventory/v1 decode, the entry checks and the two readers (P7-AUTO-04).
// Inputs:  inline documents and fake runners.
// Outputs: pass or fail.
// Constraints: no network, no processes, no sleeps (the timeout test blocks on the context only).

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const goodDoc = `schema: inventory/v1
entries:
  - id: vps1
    ssh: ci@vps1.example.test
    os: linux
    arch: amd64
    rank: 2
    labels: [gpu=none]
    pools: [mine]
    skip_repos: [/src/old]
    public_only: true
`

func TestStrictInventoryDecode(t *testing.T) {
	d, err := Decode([]byte(goodDoc))
	if err != nil || len(d.Entries) != 1 || d.Entries[0].Rank == nil || *d.Entries[0].Rank != 2 || !d.Entries[0].PublicOnly {
		t.Fatalf("good doc: %+v %v", d, err)
	}
	if _, err = Decode([]byte(`{"schema":"inventory/v1","entries":[{"id":"a","ssh":"h"}]}`)); err != nil {
		t.Fatalf("json is accepted: %v", err)
	}
	bad := map[string]string{
		"unknown entry field": "schema: inventory/v1\nentries:\n  - id: a\n    slots: 4\n",
		"unknown top field":   "schema: inventory/v1\nextra: 1\nentries: []\n",
		"wrong schema":        "schema: inventory/v2\nentries: []\n",
		"no schema":           "entries: []\n",
		"two documents":       goodDoc + "---\n" + goodDoc,
		"not a document":      "- a\n- b\n",
	}
	for name, doc := range bad {
		if _, err := Decode([]byte(doc)); err == nil || !strings.HasPrefix(err.Error(), CodeInventory) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	var many strings.Builder
	many.WriteString("schema: inventory/v1\nentries:\n")
	for i := 0; i <= MaxEntries; i++ {
		many.WriteString("  - id: n\n")
	}
	if _, err := Decode([]byte(many.String())); ReasonOf(err) != ReasonSchema {
		t.Fatalf("too many entries: %v", err)
	}
}

func TestEntryCheck(t *testing.T) {
	rank := func(n int) *int { return &n }
	cases := []struct {
		name string
		e    Entry
		ok   bool
	}{
		{"minimal", Entry{ID: "a"}, true},
		{"upper id", Entry{ID: "Abc"}, false},
		{"bad ssh", Entry{ID: "a", SSH: "h;rm"}, false},
		{"bad os", Entry{ID: "a", OS: "plan9"}, false},
		{"bad arch", Entry{ID: "a", Arch: "mips"}, false},
		{"negative rank", Entry{ID: "a", Rank: rank(-1)}, false},
		{"huge rank", Entry{ID: "a", Rank: rank(MaxRank + 1)}, false},
		{"reserved pool", Entry{ID: "a", Pools: []string{"hosted"}}, false},
		{"bad pool", Entry{ID: "a", Pools: []string{"Bad"}}, false},
		{"rank label", Entry{ID: "a", Labels: []string{"rank=1"}}, false},
		{"pool label", Entry{ID: "a", Labels: []string{"pool.x"}}, false},
		{"reserved label", Entry{ID: "a", Labels: []string{"nself.x"}}, false},
		{"empty skip", Entry{ID: "a", SkipRepos: []string{""}}, false},
		{"control skip", Entry{ID: "a", SkipRepos: []string{"a\nb"}}, false},
	}
	for _, c := range cases {
		if err := c.e.Check(); (err == nil) != c.ok {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
}

func TestCmdCapsAndTimeout(t *testing.T) {
	ok := func(out string, exit int, err error) Runner {
		return func(context.Context, []string) (string, int, error) { return out, exit, err }
	}
	small := Limits{Timeout: time.Second, MaxBytes: 64}
	if d, err := ReadCmd(context.Background(), ok("schema: inventory/v1\nentries: []\n", 0, nil), []string{"x"}, small); err != nil || len(d.Entries) != 0 {
		t.Fatalf("fits: %v", err)
	}
	if _, err := ReadCmd(context.Background(), ok(goodDoc, 0, nil), []string{"x"}, small); ReasonOf(err) != ReasonTooLarge {
		t.Fatalf("oversized: %v", err)
	}
	blocked := func(ctx context.Context, _ []string) (string, int, error) { <-ctx.Done(); return "", -1, ctx.Err() }
	if _, err := ReadCmd(context.Background(), blocked, []string{"x"}, Limits{Timeout: time.Millisecond, MaxBytes: 64}); ReasonOf(err) != ReasonTimeout {
		t.Fatalf("slow: %v", err)
	}
	if _, err := ReadCmd(context.Background(), ok("", 3, nil), []string{"x"}, small); ReasonOf(err) != ReasonCmdFailed {
		t.Fatalf("exit status: %v", err)
	}
	if _, err := ReadCmd(context.Background(), ok("", -1, context.Canceled), []string{"x"}, small); ReasonOf(err) != ReasonCmdFailed {
		t.Fatalf("not run: %v", err)
	}
	if _, err := ReadCmd(context.Background(), ok("schema: inventory/v1\nbogus: 1\n", 0, nil), []string{"x"}, small); ReasonOf(err) != ReasonDecode {
		t.Fatalf("unknown field: %v", err)
	}
	if _, err := ReadCmd(context.Background(), nil, []string{"x"}, small); ReasonOf(err) != ReasonNoRunner {
		t.Fatalf("no runner: %v", err)
	}
	if _, err := ReadCmd(context.Background(), ok("", 0, nil), nil, small); ReasonOf(err) != ReasonNoRunner {
		t.Fatalf("no argv: %v", err)
	}
	if DefaultLimits.Timeout != 10*time.Second || DefaultLimits.MaxBytes != 1<<20 {
		t.Fatalf("contract limits changed: %+v", DefaultLimits)
	}
}

func TestReadFileCap(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "inv.yaml")
	if err := os.WriteFile(p, []byte(goodDoc), 0o600); err != nil {
		t.Fatal(err)
	}
	if d, err := ReadFile(p, Limits{}); err != nil || len(d.Entries) != 1 {
		t.Fatalf("read: %v", err)
	}
	if _, err := ReadFile(p, Limits{MaxBytes: 10}); ReasonOf(err) != ReasonTooLarge {
		t.Fatalf("cap: %v", err)
	}
	if _, err := ReadFile(filepath.Join(dir, "missing.yaml"), Limits{}); ReasonOf(err) != ReasonReadFailed {
		t.Fatalf("missing: %v", err)
	}
}
