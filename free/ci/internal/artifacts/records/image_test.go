package records_test

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func dig(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// ociTar builds an OCI layout: a top index listing a nested index (two
// platforms) and one direct manifest.
func ociTar(t *testing.T, mutate func(files map[string][]byte)) ([]byte, string) {
	t.Helper()
	m1 := []byte(`{"schemaVersion":2,"config":{"digest":"sha256:` + strings.Repeat("1", 64) + `"}}`)
	m2 := []byte(`{"schemaVersion":2,"config":{"digest":"sha256:` + strings.Repeat("2", 64) + `"}}`)
	m3 := []byte(`{"schemaVersion":2,"config":{"digest":"sha256:` + strings.Repeat("3", 64) + `"}}`)
	nested := []byte(fmt.Sprintf(`{"schemaVersion":2,"manifests":[
	 {"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:%s","size":%d,"platform":{"os":"linux","architecture":"arm64","variant":"v8"}},
	 {"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:%s","size":%d,"platform":{"os":"linux","architecture":"amd64"}}]}`, dig(m1), len(m1), dig(m2), len(m2)))
	top := []byte(fmt.Sprintf(`{"schemaVersion":2,"manifests":[
	 {"mediaType":"application/vnd.oci.image.index.v1+json","digest":"sha256:%s","size":%d},
	 {"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:%s","size":%d}]}`, dig(nested), len(nested), dig(m3), len(m3)))
	files := map[string][]byte{"oci-layout": []byte(`{"imageLayoutVersion":"1.0.0"}`), "index.json": top,
		"blobs/sha256/" + dig(nested): nested, "blobs/sha256/" + dig(m1): m1, "blobs/sha256/" + dig(m2): m2, "blobs/sha256/" + dig(m3): m3}
	if mutate != nil {
		mutate(files)
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, name := range []string{"oci-layout", "index.json"} {
		if b, ok := files[name]; ok {
			_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(b)), Typeflag: tar.TypeReg})
			_, _ = tw.Write(b)
		}
	}
	for n, b := range files {
		if strings.HasPrefix(n, "blobs/") {
			_ = tw.WriteHeader(&tar.Header{Name: "./" + n, Mode: 0o644, Size: int64(len(b)), Typeflag: tar.TypeReg})
			_, _ = tw.Write(b)
		}
	}
	_ = tw.Close()
	return buf.Bytes(), dig(top)
}

func TestImageLayoutRecordsIndexAndPlatformDigests(t *testing.T) {
	e := newEnv(t)
	tarball, topDigest := ociTar(t, nil)
	e.write("image.tar", string(tarball))
	rec := e.commit("i1", model.ArtifactDecl{Path: "image.tar", Name: "app-image", Kind: "image", Image: &model.ArtifactImage{Ref: "registry.example/app"}})[0]
	if rec.Kind != records.KindImage || rec.Image == nil || rec.Image.IndexDigest != "sha256:"+topDigest || rec.Image.Ref != "registry.example/app" {
		t.Fatalf("image: %+v", rec.Image)
	}
	if len(rec.Image.Manifests) != 3 {
		t.Fatalf("one manifest per platform: %+v", rec.Image.Manifests)
	}
	got := map[string]bool{}
	for _, m := range rec.Image.Manifests {
		got[m.Platform] = true
		if !strings.HasPrefix(m.Digest, "sha256:") {
			t.Fatalf("digest %q", m.Digest)
		}
	}
	for _, p := range []string{"linux/arm64/v8", "linux/amd64", "unknown"} {
		if !got[p] {
			t.Fatalf("platform %s missing: %v", p, got)
		}
	}
	// it survives the store round trip
	stored, _, _ := e.st.ArtifactGet(context.Background(), rec.ID)
	if stored.Image == nil || stored.Image.IndexDigest != rec.Image.IndexDigest || len(stored.Image.Manifests) != 3 {
		t.Fatalf("stored image: %+v", stored.Image)
	}
}

func TestImageLayoutRefusals(t *testing.T) {
	bad := map[string]func(map[string][]byte){
		"no index":        func(f map[string][]byte) { delete(f, "index.json") },
		"index not JSON":  func(f map[string][]byte) { f["index.json"] = []byte("{nope") },
		"empty manifests": func(f map[string][]byte) { f["index.json"] = []byte(`{"manifests":[]}`) },
		"bad digest": func(f map[string][]byte) {
			f["index.json"] = []byte(`{"manifests":[{"mediaType":"x","digest":"md5:abc"}]}`)
		},
		"nested missing": func(f map[string][]byte) {
			f["index.json"] = []byte(`{"manifests":[{"mediaType":"application/vnd.oci.image.index.v1+json","digest":"sha256:` + strings.Repeat("a", 64) + `"}]}`)
		},
		"nested wrong bytes": func(f map[string][]byte) {
			for n := range f {
				if strings.HasPrefix(n, "blobs/") && strings.Contains(string(f[n]), `"manifests"`) {
					f[n] = []byte(`{"manifests":[]}`)
				}
			}
		},
		"too deep": func(f map[string][]byte) { nestedIndexes(f, 4) },
	}
	for name, mut := range bad {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			tarball, _ := ociTar(t, mut)
			e.write("image.tar", string(tarball))
			_, err := collect2(e, model.ArtifactDecl{Path: "image.tar", Kind: "image"})
			if !records.IsReason(err, records.ReasonImage) {
				t.Fatalf("%v", err)
			}
		})
	}
	e := newEnv(t)
	e.write("image.tar", "this is not a tar file at all, just text padded out to look like something")
	if _, err := collect2(e, model.ArtifactDecl{Path: "image.tar", Kind: "image"}); !records.IsReason(err, records.ReasonImage) {
		t.Fatalf("not a tar: %v", err)
	}
}

func collect2(e *env, d model.ArtifactDecl) ([]records.Staged, error) {
	ds, err := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{d}}, false, nil)
	if err != nil {
		return nil, err
	}
	return e.collector().Collect(context.Background(), "proj", e.ws, ds)
}

// nestedIndexes rewrites index.json as a chain of `levels` indexes (the top one
// included); the last holds one manifest.
func nestedIndexes(f map[string][]byte, levels int) {
	for n := range f {
		if strings.HasPrefix(n, "blobs/") {
			delete(f, n)
		}
	}
	leaf := []byte(`{"manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:` + strings.Repeat("e", 64) + `","platform":{"os":"linux","architecture":"amd64"}}]}`)
	cur := leaf
	for i := 1; i < levels; i++ {
		f["blobs/sha256/"+dig(cur)] = cur
		cur = []byte(`{"manifests":[{"mediaType":"application/vnd.oci.image.index.v1+json","digest":"sha256:` + dig(cur) + `"}]}`)
	}
	f["index.json"] = cur
}

func TestImageNestingDepthBoundary(t *testing.T) {
	read := func(levels int) (*records.ImageInfo, error) {
		tarball, _ := ociTar(t, func(f map[string][]byte) { nestedIndexes(f, levels) })
		return records.ReadImageLayout(bytes.NewReader(tarball))
	}
	for levels := 1; levels <= 3; levels++ {
		if info, err := read(levels); err != nil || len(info.Manifests) != 1 {
			t.Fatalf("%d levels must read: %v", levels, err)
		}
	}
	if _, err := read(4); !records.IsReason(err, records.ReasonImage) {
		t.Fatalf("4 levels must be refused: %v", err)
	}
	if _, err := read(6); !records.IsReason(err, records.ReasonImage) {
		t.Fatalf("6 levels must be refused: %v", err)
	}
}

// rawTar writes regular entries in order.
func rawTar(entries []tar.Header, bodies [][]byte) []byte {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for i := range entries {
		h := entries[i]
		h.Typeflag, h.Mode, h.Size = tar.TypeReg, 0o644, int64(len(bodies[i]))
		_ = tw.WriteHeader(&h)
		_, _ = tw.Write(bodies[i])
	}
	_ = tw.Close()
	return buf.Bytes()
}

func manifestIndex() []byte {
	return []byte(`{"manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:` + strings.Repeat("a", 64) + `"}]}`)
}

func TestImageSizeAndCountBoundaries(t *testing.T) {
	const small = 1 << 20
	pad := func(b []byte, n int) []byte {
		return append(append([]byte(nil), b...), bytes.Repeat([]byte(" "), n-len(b))...)
	}
	// index.json of exactly 1 MiB is read; one byte more is skipped (no index)
	if _, err := records.ReadImageLayout(bytes.NewReader(rawTar([]tar.Header{{Name: "index.json"}}, [][]byte{pad(manifestIndex(), small)}))); err != nil {
		t.Fatalf("1 MiB index: %v", err)
	}
	if _, err := records.ReadImageLayout(bytes.NewReader(rawTar([]tar.Header{{Name: "index.json"}}, [][]byte{pad(manifestIndex(), small+1)}))); !records.IsReason(err, records.ReasonImage) {
		t.Fatalf("1 MiB + 1 index: %v", err)
	}
	// entry count: 65536 entries read, 65537 refused
	many := func(n int) []byte {
		hs := make([]tar.Header, n)
		bs := make([][]byte, n)
		hs[0], bs[0] = tar.Header{Name: "index.json"}, manifestIndex()
		for i := 1; i < n; i++ {
			hs[i] = tar.Header{Name: fmt.Sprintf("f/%d", i)}
		}
		return rawTar(hs, bs)
	}
	if _, err := records.ReadImageLayout(bytes.NewReader(many(1 << 16))); err != nil {
		t.Fatalf("65536 entries: %v", err)
	}
	if _, err := records.ReadImageLayout(bytes.NewReader(many(1<<16 + 1))); !records.IsReason(err, records.ReasonImage) {
		t.Fatalf("65537 entries: %v", err)
	}
	// small blobs held: 32 x 1 MiB is read, one more byte is refused
	blobs := func(extra int) []byte {
		hs := []tar.Header{{Name: "index.json"}}
		bs := [][]byte{manifestIndex()}
		for i := 0; i < 32; i++ {
			hs = append(hs, tar.Header{Name: fmt.Sprintf("blobs/sha256/%064x", i)})
			bs = append(bs, bytes.Repeat([]byte{byte(i)}, small))
		}
		if extra > 0 {
			hs = append(hs, tar.Header{Name: fmt.Sprintf("blobs/sha256/%064x", 99)})
			bs = append(bs, make([]byte, extra))
		}
		return rawTar(hs, bs)
	}
	if _, err := records.ReadImageLayout(bytes.NewReader(blobs(0))); err != nil {
		t.Fatalf("32 MiB held: %v", err)
	}
	if _, err := records.ReadImageLayout(bytes.NewReader(blobs(1))); !records.IsReason(err, records.ReasonImage) {
		t.Fatalf("32 MiB + 1 held: %v", err)
	}
}

func TestImageManifestOrder(t *testing.T) {
	d := func(c string) string { return `"sha256:` + strings.Repeat(c, 64) + `"` }
	m := func(c, arch string) string {
		return `{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":` + d(c) + `,"platform":{"os":"linux","architecture":"` + arch + `"}}`
	}
	idx := []byte(`{"manifests":[` + strings.Join([]string{m("9", "arm64"), m("2", "amd64"), m("1", "amd64"), m("5", "386")}, ",") + `]}`)
	info, err := records.ReadImageLayout(bytes.NewReader(rawTar([]tar.Header{{Name: "index.json"}}, [][]byte{idx})))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, mf := range info.Manifests {
		got = append(got, mf.Platform+"="+mf.Digest[7:8])
	}
	if want := "linux/386=5 linux/amd64=1 linux/amd64=2 linux/arm64=9"; strings.Join(got, " ") != want {
		t.Fatalf("order\n got %v\nwant %s", got, want)
	}
}
