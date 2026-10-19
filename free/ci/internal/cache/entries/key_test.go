package entries

import (
	"encoding/hex"
	"encoding/json"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

type vector struct {
	Name      string `json:"name"`
	Namespace struct {
		Project, Trust, Protection string
		HeadRepo                   string `json:"head_repo"`
		PR                         string `json:"pr"`
	} `json:"namespace"`
	SpecID      string   `json:"spec_id"`
	SpecVersion string   `json:"spec_version"`
	Platform    string   `json:"platform"`
	Isolation   string   `json:"isolation"`
	Toolchain   []string `json:"toolchain"`
	EnvDigest   string   `json:"env_digest"`
	Paths       []string `json:"paths"`
	Lockfiles   []string `json:"lockfiles"`
	Extras      []string `json:"extras"`
	EncodedHex  string   `json:"encoded_hex"`
	Derived     string   `json:"derived"`
}

func (v vector) inputs(t *testing.T) KeyInputs {
	t.Helper()
	n := v.Namespace
	ns, err := authz.CacheNamespace(n.Project, model.TrustClass(n.Trust), authz.Protection(n.Protection), n.HeadRepo, n.PR)
	if err != nil {
		t.Fatal(err)
	}
	return KeyInputs{Namespace: ns, SpecID: v.SpecID, SpecVersion: v.SpecVersion, Platform: v.Platform, Isolation: v.Isolation,
		Toolchain: v.Toolchain, EnvDigest: v.EnvDigest, Paths: v.Paths, Lockfiles: v.Lockfiles, Extras: v.Extras}
}

var trustedNS = func() authz.Namespace {
	ns, err := authz.CacheNamespace("proj", "owner", authz.Protected, "", "")
	if err != nil {
		panic(err)
	}
	return ns
}()

// TestKeyGolden pins the encoding with vectors produced by an independent
// implementation (testdata/key_vectors.json).
func TestKeyGolden(t *testing.T) {
	raw, err := os.ReadFile("testdata/key_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vs []vector
	if err = json.Unmarshal(raw, &vs); err != nil || len(vs) < 5 {
		t.Fatalf("vectors: %d %v", len(vs), err)
	}
	for _, v := range vs {
		t.Run(v.Name, func(t *testing.T) {
			in := v.inputs(t)
			enc, err := in.encode()
			if err != nil || hex.EncodeToString(enc) != v.EncodedHex {
				t.Fatalf("encoding differs\n got %x\nwant %s (%v)", enc, v.EncodedHex, err)
			}
			if got, err := Key(in); err != nil || got != v.Derived {
				t.Fatalf("key %s want %s (%v)", got, v.Derived, err)
			}
		})
	}
}

// TestKeyBoundariesNeverShift: ("ab","c") and ("a","bc") differ in every field position.
func TestKeyBoundariesNeverShift(t *testing.T) {
	base := KeyInputs{Namespace: trustedNS, SpecID: "go", SpecVersion: "1", Platform: "linux/amd64", Isolation: "container",
		Toolchain: []string{"t"}, EnvDigest: "e", Paths: []string{"p"}, Lockfiles: []string{"l=1"}, Extras: []string{"x"}}
	scalars := []func(*KeyInputs) *string{
		func(k *KeyInputs) *string { return &k.SpecID }, func(k *KeyInputs) *string { return &k.SpecVersion },
		func(k *KeyInputs) *string { return &k.Platform }, func(k *KeyInputs) *string { return &k.Isolation },
	}
	key := func(k KeyInputs) string {
		t.Helper()
		s, err := Key(k)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	for i := 0; i+1 < len(scalars); i++ {
		a, b := base, base
		*scalars[i](&a), *scalars[i+1](&a) = "ab", "c"
		*scalars[i](&b), *scalars[i+1](&b) = "a", "bc"
		if key(a) == key(b) {
			t.Fatalf("scalar fields %d/%d collide", i, i+1)
		}
	}
	a, b := base, base
	a.Isolation, a.EnvDigest = "ab", "c"
	b.Isolation, b.EnvDigest = "a", "bc"
	if key(a) == key(b) {
		t.Fatal("isolation/env digest boundary shifted")
	}
	a, b = base, base
	a.SpecVersion, a.Platform = "", "x"
	b.SpecVersion, b.Platform = "x", ""
	if key(a) == key(b) {
		t.Fatal("empty and filled neighbours collide")
	}
	lists := []func(*KeyInputs) *[]string{
		func(k *KeyInputs) *[]string { return &k.Toolchain }, func(k *KeyInputs) *[]string { return &k.Paths },
		func(k *KeyInputs) *[]string { return &k.Lockfiles }, func(k *KeyInputs) *[]string { return &k.Extras },
	}
	for i, l := range lists {
		a, b = base, base
		*l(&a), *l(&b) = []string{"ab", "c"}, []string{"a", "bc"}
		if key(a) == key(b) {
			t.Fatalf("list %d items shift across the boundary", i)
		}
		a, b = base, base
		*l(&a), *l(&b) = []string{"ab"}, []string{"a", "b"}
		if key(a) == key(b) {
			t.Fatalf("list %d split item collides", i)
		}
		if i+1 < len(lists) {
			a, b = base, base
			*l(&a), *lists[i+1](&a), *l(&b), *lists[i+1](&b) = []string{"q"}, nil, nil, []string{"q"}
			if key(a) == key(b) {
				t.Fatalf("item moved from list %d to the next list collides", i)
			}
		}
	}
	for _, f := range []func(*KeyInputs){func(k *KeyInputs) { k.SpecID = "other" }, func(k *KeyInputs) { k.EnvDigest = "f" }, func(k *KeyInputs) {
		k.Namespace, _ = authz.CacheNamespace("proj", "untrusted", authz.Unprotected, "fork/x", "7")
	}} {
		c := base
		f(&c)
		if key(c) == key(base) {
			t.Fatal("a changed field did not change the key")
		}
	}
}

func TestKeyListsAreSetsAndPure(t *testing.T) {
	a := KeyInputs{Namespace: trustedNS, Paths: []string{"z", "a", "m"}, Toolchain: []string{"b=2", "a=1"}}
	b := KeyInputs{Namespace: trustedNS, Paths: []string{"a", "m", "z"}, Toolchain: []string{"a=1", "b=2"}}
	ka, _ := Key(a)
	kb, _ := Key(b)
	if ka != kb || len(ka) != 64 || strings.ToLower(ka) != ka {
		t.Fatalf("order must not matter and keys are 64 lowercase hex: %s %s", ka, kb)
	}
	if !reflect.DeepEqual(a.Paths, []string{"z", "a", "m"}) {
		t.Fatal("Key reordered the caller's slice")
	}
	if again, _ := Key(a); again != ka {
		t.Fatal("Key is not deterministic")
	}
}

func TestKeyRejects(t *testing.T) {
	if _, err := Key(KeyInputs{}); err == nil {
		t.Fatal("an invalid namespace was accepted")
	}
	ok := KeyInputs{Namespace: trustedNS, SpecID: strings.Repeat("x", maxFieldBytes)}
	if _, err := Key(ok); err != nil {
		t.Fatalf("a field of exactly the limit must work: %v", err)
	}
	big := KeyInputs{Namespace: trustedNS, SpecID: strings.Repeat("x", maxFieldBytes+1)}
	if _, err := Key(big); err == nil {
		t.Fatal("an oversized field was accepted")
	}
	items := make([]string, maxListItems)
	if _, err := Key(KeyInputs{Namespace: trustedNS, Paths: items}); err != nil {
		t.Fatalf("a list of exactly the limit must work: %v", err)
	}
	if _, err := Key(KeyInputs{Namespace: trustedNS, Paths: append(items, "x")}); err == nil {
		t.Fatal("an oversized list was accepted")
	}
	if _, err := Key(KeyInputs{Namespace: trustedNS, Extras: []string{strings.Repeat("x", maxFieldBytes+1)}}); err == nil {
		t.Fatal("an oversized list item was accepted")
	}
	over := make([]string, maxListItems+1)
	for name, in := range map[string]KeyInputs{
		"toolchain": {Namespace: trustedNS, Toolchain: over}, "paths": {Namespace: trustedNS, Paths: over},
		"lockfiles": {Namespace: trustedNS, Lockfiles: over}, "extras": {Namespace: trustedNS, Extras: over},
	} {
		if _, err := Key(in); err == nil {
			t.Fatalf("an oversized %s list was accepted", name)
		}
	}
	if _, err := Key(KeyInputs{Namespace: trustedNS, EnvDigest: strings.Repeat("x", maxFieldBytes+1)}); err == nil {
		t.Fatal("an oversized env digest was accepted")
	}
}

// TestKeyNoCollisionsProperty derives 10 000 keys from random tuples over an
// alphabet full of separators and finds no collision between distinct tuples.
func TestKeyNoCollisionsProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(20261018))
	alphabet := []string{"", "a", "b", "ab", "=", ",", "/", "\x00", "é", "\xff\xfe", "0000", "go", "x=1"}
	pick := func() string {
		var sb strings.Builder
		for n := rng.Intn(3); n >= 0; n-- {
			sb.WriteString(alphabet[rng.Intn(len(alphabet))])
		}
		return sb.String()
	}
	list := func() []string {
		out := make([]string, rng.Intn(4))
		for i := range out {
			out[i] = pick()
		}
		return out
	}
	seen := map[string]string{}
	for i := 0; i < 10000; i++ {
		in := KeyInputs{Namespace: trustedNS, SpecID: pick(), SpecVersion: pick(), Platform: pick(), Isolation: pick(), Toolchain: list(), EnvDigest: pick(), Paths: list(), Lockfiles: list(), Extras: list()}
		k, err := Key(in)
		if err != nil {
			t.Fatal(err)
		}
		norm := in
		for _, l := range []*[]string{&norm.Toolchain, &norm.Paths, &norm.Lockfiles, &norm.Extras} {
			*l = append([]string(nil), *l...)
			sort.Strings(*l)
		}
		tuple, _ := json.Marshal([]any{norm.SpecID, norm.SpecVersion, norm.Platform, norm.Isolation, norm.Toolchain, norm.EnvDigest, norm.Paths, norm.Lockfiles, norm.Extras})
		if prev, dup := seen[k]; dup && prev != string(tuple) {
			t.Fatalf("collision between %s and %s", prev, tuple)
		}
		seen[k] = string(tuple)
	}
	if len(seen) < 5000 {
		t.Fatalf("generator too narrow: %d distinct tuples", len(seen))
	}
}
