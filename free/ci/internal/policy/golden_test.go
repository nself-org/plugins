package policy

// Purpose: golden of the Resolved shape for the plain private-owner case, so a
// change of a default, a source or the digest recipe is a visible diff.
// Inputs: testdata/golden/private-owner-defaults.json.
// Constraints: regenerate with UPDATE_GOLDEN=1 go test -run TestGolden ./internal/policy/.

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestGoldenPrivateOwnerDefaults(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main", root)
	r := &Resolver{Keys: testKeys(t), Run: testRunner}
	got, err := r.Resolve(context.Background(), Repo{Root: root, Visibility: "private", Class: "owner"}, Revision{})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := json.MarshalIndent(map[string]any{"values": got.Values, "sources": got.Sources, "fetch": got.Fetch, "digest": got.Digest}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	doc = append(doc, '\n')
	const path = "testdata/golden/private-owner-defaults.json"
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata/golden", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, doc, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(doc) {
		t.Fatalf("golden drift:\n%s", doc)
	}
}
