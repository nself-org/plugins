package entries

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func goodManifest() Manifest {
	return Manifest{Namespace: trustedNS.Canonical(), Key: strings.Repeat("a", 64), Spec: "go",
		Blobs:     []Blob{{Path: "a/mod.tar.gz", SHA256: strings.Repeat("b", 64), Size: 7}, {Path: "z.tar.gz", SHA256: strings.Repeat("c", 64), Size: 0}},
		Writer:    Writer{AttemptID: "att-1", Epoch: 3, SourceTrust: "owner", ScannedClean: true},
		CreatedAt: 5, LastUsedAt: 6}
}

func TestManifestRoundTrip(t *testing.T) {
	m := goodManifest()
	b, err := EncodeManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeManifest(b)
	if err != nil || !reflect.DeepEqual(got, m) {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	if m.Bytes() != 7 {
		t.Fatalf("Bytes() = %d", m.Bytes())
	}
}

func TestManifestRejects(t *testing.T) {
	mutate := func(f func(*Manifest)) Manifest { m := goodManifest(); f(&m); return m }
	cases := map[string]Manifest{
		"foreign namespace bytes": mutate(func(m *Manifest) { m.Namespace = []byte("nope") }),
		"empty namespace":         mutate(func(m *Manifest) { m.Namespace = nil }),
		"non canonical namespace": mutate(func(m *Manifest) { m.Namespace = append(m.Namespace, 0) }),
		"short key":               mutate(func(m *Manifest) { m.Key = "abc" }),
		"upper key":               mutate(func(m *Manifest) { m.Key = strings.ToUpper(m.Key) }),
		"bad spec":                mutate(func(m *Manifest) { m.Spec = "Go Cache" }),
		"negative created":        mutate(func(m *Manifest) { m.CreatedAt = -1 }),
		"negative used":           mutate(func(m *Manifest) { m.LastUsedAt = -1 }),
		"negative epoch":          mutate(func(m *Manifest) { m.Writer.Epoch = -1 }),
		"bad attempt":             mutate(func(m *Manifest) { m.Writer.AttemptID = "a b" }),
		"bad trust":               mutate(func(m *Manifest) { m.Writer.SourceTrust = "root" }),
		"unsorted blobs":          mutate(func(m *Manifest) { m.Blobs[0].Path, m.Blobs[1].Path = m.Blobs[1].Path, m.Blobs[0].Path }),
		"duplicate blob":          mutate(func(m *Manifest) { m.Blobs[1].Path = m.Blobs[0].Path }),
		"bad digest":              mutate(func(m *Manifest) { m.Blobs[0].SHA256 = "xyz" }),
		"negative size":           mutate(func(m *Manifest) { m.Blobs[0].Size = -1 }),
		"huge size":               mutate(func(m *Manifest) { m.Blobs[0].Size = maxBlobBytes + 1 }),
	}
	for name, m := range cases {
		if err := m.Validate(); !errors.Is(err, ErrManifest) || errors.Is(err, ErrPathOutside) {
			t.Errorf("%s: %v", name, err)
		}
		if _, err := EncodeManifest(m); err == nil {
			t.Errorf("%s: encoded", name)
		}
	}
	for _, p := range []string{"", ".", "..", "../x", "a/../../x", "/abs", "a//b", "a/./b", "a/", "a\\b", "a\x00b", strings.Repeat("p", maxPathBytes+1)} {
		m := mutate(func(m *Manifest) { m.Blobs = []Blob{{Path: p, SHA256: strings.Repeat("b", 64)}} })
		if err := m.Validate(); !errors.Is(err, ErrPathOutside) || !errors.Is(err, ErrManifest) {
			t.Errorf("path %q: %v", p, err)
		}
	}
	ok := mutate(func(m *Manifest) {
		m.Blobs = []Blob{{Path: strings.Repeat("p", maxPathBytes), SHA256: strings.Repeat("b", 64), Size: maxBlobBytes}}
	})
	if err := ok.Validate(); err != nil {
		t.Errorf("limits must be inclusive: %v", err)
	}
	total := mutate(func(m *Manifest) {
		m.Blobs = []Blob{{Path: "a", SHA256: strings.Repeat("b", 64), Size: maxBlobBytes}, {Path: "b", SHA256: strings.Repeat("c", 64), Size: 1}}
	})
	if err := total.Validate(); !errors.Is(err, ErrManifest) {
		t.Errorf("an entry over the total limit: %v", err)
	}
	many := mutate(func(m *Manifest) { m.Blobs = make([]Blob, maxBlobs+1) })
	if err := many.Validate(); !errors.Is(err, ErrManifest) {
		t.Errorf("too many blobs: %v", err)
	}
}

func TestDecodeManifestStrict(t *testing.T) {
	good, _ := json.Marshal(goodManifest())
	withExtra := bytes.Replace(good, []byte(`{`), []byte(`{"extra":1,`), 1)
	for name, in := range map[string][]byte{
		"unknown field": withExtra, "trailing document": append(append([]byte(nil), good...), []byte(` {}`)...),
		"trailing garbage": append(append([]byte(nil), good...), 'x'), "empty": nil, "not json": []byte("nope"),
		"wrong type":   []byte(`{"key":5}`),
		"oversized":    bytes.Repeat([]byte(" "), maxManifestBytes+1),
		"null":         []byte("null"),
		"array":        []byte("[]"),
		"duplicate ns": []byte(`{"namespace":"AAAA","namespace":"AAAA"}`),
	} {
		if _, err := DecodeManifest(in); !errors.Is(err, ErrManifest) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := DecodeManifest(append(good, '\n', ' ')); err != nil {
		t.Errorf("trailing whitespace is fine: %v", err)
	}
}

func FuzzManifest(f *testing.F) {
	good, _ := json.Marshal(goodManifest())
	f.Add(good)
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"namespace":"AAAA","key":"","blobs":[{"path":"../x"}]}`))
	f.Add(append([]byte(nil), good[:len(good)/2]...))
	f.Fuzz(func(t *testing.T, b []byte) {
		m, err := DecodeManifest(b)
		if err != nil {
			if !errors.Is(err, ErrManifest) {
				t.Fatalf("decode error without ErrManifest: %v", err)
			}
			return
		}
		enc, err := EncodeManifest(m)
		if err != nil {
			t.Fatalf("a decoded manifest does not encode: %v", err)
		}
		again, err := DecodeManifest(enc)
		if err != nil || !reflect.DeepEqual(again, m) {
			t.Fatalf("encode/decode is not stable: %v", err)
		}
	})
}

func TestDecodeManifestSizeLimitIsInclusive(t *testing.T) {
	good, _ := json.Marshal(goodManifest())
	pad := func(n int) []byte {
		return append(append([]byte(nil), good...), bytes.Repeat([]byte(" "), n-len(good))...)
	}
	if _, err := DecodeManifest(pad(maxManifestBytes)); err != nil {
		t.Fatalf("a document of exactly the limit must decode: %v", err)
	}
	if _, err := DecodeManifest(pad(maxManifestBytes + 1)); !errors.Is(err, ErrManifest) {
		t.Fatalf("a document over the limit: %v", err)
	}
}
