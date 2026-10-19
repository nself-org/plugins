package specs

// Purpose: a blob kept in the local store is a plain tar, so a restore is a copy
// and not a gunzip (D-1008 round 3); a gzip blob (older saves, or one a remote
// store hands back) still restores.

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
)

func TestLocalBlobIsAPlainTarAndRestores(t *testing.T) {
	e := newEnv(t)
	src := t.TempDir()
	putFile(t, filepath.Join(src, "d", "a.txt"), "alpha alpha alpha alpha")
	base, err := os.OpenRoot(src)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = base.Close() }()
	realm := cas.CacheRealm("ns")
	digest, _, _, err := e.eng.putDir(e.ctx, base, realm, "d", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := e.blobs.Open(realm, digest)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > 1 && raw[0] == 0x1f && raw[1] == 0x8b {
		t.Fatal("the local blob is gzip; a restore would pay a decompression")
	}
	if _, err := tar.NewReader(bytes.NewReader(raw)).Next(); err != nil {
		t.Fatalf("the local blob is not a tar: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "out")
	if _, err := unpackTo(t, bytes.NewReader(raw), dst, cacheLimits(), UnpackOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := getFile(t, filepath.Join(dst, "a.txt")); got != "alpha alpha alpha alpha" {
		t.Fatalf("restored %q", got)
	}
}

func TestPlainTarObeysTheSanitiserLimits(t *testing.T) {
	lim := authz.DefaultLimits()
	lim.MaxTotalBytes = 1 << 20
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	body := bytes.Repeat([]byte("ab"), 200<<10)
	for _, n := range []string{"a", "b", "c"} {
		_ = tw.WriteHeader(&tar.Header{Name: n, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(body))})
		_, _ = tw.Write(body)
	}
	_ = tw.Close()
	_, err := unpackTo(t, bytes.NewReader(buf.Bytes()), t.TempDir()+"/d", lim, UnpackOptions{})
	var pe *authz.PathError
	if !errors.As(err, &pe) || pe.Reason != authz.SizeTotal {
		t.Fatalf("err %v", err)
	}
}
