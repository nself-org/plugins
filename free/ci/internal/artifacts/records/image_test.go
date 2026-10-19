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
		"too deep": func(f map[string][]byte) {
			a := []byte(`{"manifests":[{"mediaType":"application/vnd.oci.image.index.v1+json","digest":"sha256:%s"}]}`)
			b := []byte(`{"manifests":[{"mediaType":"application/vnd.oci.image.index.v1+json","digest":"sha256:%s"}]}`)
			c := []byte(`{"manifests":[{"mediaType":"application/vnd.oci.image.index.v1+json","digest":"sha256:%s"}]}`)
			d := []byte(`{"manifests":[]}`)
			dd := dig(d)
			c = []byte(fmt.Sprintf(string(c), dd))
			cd := dig(c)
			b = []byte(fmt.Sprintf(string(b), cd))
			bd := dig(b)
			a = []byte(fmt.Sprintf(string(a), bd))
			f["index.json"] = a
			f["blobs/sha256/"+bd], f["blobs/sha256/"+cd], f["blobs/sha256/"+dd] = b, c, d
		},
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
