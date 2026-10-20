//go:build cachesec

package cachesec

// Purpose: TM-14 on the remote carriers: a node (honest agents stage safely, so these are nodes that bypass the
// stager) ships a crafted cache archive or names a crafted artifact path. The coordinator validates what it
// receives with the restore sanitiser before an entry is committed.
// Inputs: crafted tar.gz bytes and artifact refs sent by a scripted node over the in-process ssh carrier.
// Outputs: registered cases of the g3 rows of matrix.yaml.
// Constraints: synthetic names and payloads; the coordinator handler is production code; a refused save leaves
// no entry (the unreferenced staging blob is not reachable and is swept by cas.Sweep).

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// g3CraftedTgz builds a gzip tar whose single entry is hdr (with body).
func g3CraftedTgz(t *testing.T, hdr tar.Header, body string) []byte {
	t.Helper()
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	hdr.Size = int64(len(body))
	if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != 0 {
		hdr.Size = 0
	}
	if err := tw.WriteHeader(&hdr); err != nil {
		t.Fatal(err)
	}
	if hdr.Size > 0 {
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// g3CraftedSave: a node ships the crafted archive and its ref; the coordinator finalizes. The honest control is
// the same node script with a well-formed archive on a fresh world.
func g3CraftedSave(t *testing.T, crafted []byte) Observed {
	good := g3Tgz(t, map[string]string{"a.txt": "SYNTHETIC honest payload"})
	g3ControlSave(t, "ssh", g3Main, good)
	w := newWorld(t)
	r := newG3Rig(t, w, "craft1", g3Main, g3Plan(g3NS(t, g3Main), "rw", g3Key), model.Job{Kind: "test", Run: []string{"true"}})
	n := &g3node{steps: []g3step{g3Send("cache", crafted), g3CacheRef(g3Key, [2]any{"mod", crafted})}}
	s := r.Sink()
	g3Run(t, "ssh", r, n, s)
	ferr := r.cache.Finalize(context.Background(), 2*time.Second)
	t.Logf("finalize: %v; audit: %v", ferr, w.AuditActions())
	var ref *entries.Refused
	if !errors.As(ferr, &ref) || ref.Reason != entries.ReasonBlobInvalid {
		t.Fatalf("the save is not also refused as E711 blob_invalid: %v", ferr)
	}
	got := g3Refused(t, ferr)
	got.Audit = w.AuditOf("cache.remote.refused")
	got.Landed = r.Landed()
	return got
}

func init() {
	register("C14-SSH-01", func(t *testing.T) Observed {
		return g3CraftedSave(t, g3CraftedTgz(t, tar.Header{Name: "../../escape.txt", Mode: 0o644, Typeflag: tar.TypeReg}, "SYNTHETIC"))
	})
	register("C14-SSH-02", func(t *testing.T) Observed {
		return g3CraftedSave(t, g3CraftedTgz(t, tar.Header{Name: "link", Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}, ""))
	})
	register("C14-SSH-03", func(t *testing.T) Observed {
		return g3CraftedSave(t, g3CraftedTgz(t, tar.Header{Name: "hard", Mode: 0o644, Typeflag: tar.TypeLink, Linkname: "../../outside"}, ""))
	})
	register("C14-SSH-04", func(t *testing.T) Observed {
		return g3CraftedSave(t, g3CraftedTgz(t, tar.Header{Name: "pipe", Mode: 0o644, Typeflag: tar.TypeFifo}, ""))
	})
	// a ratio bomb: 96 MiB of zeros in a few hundred KiB of gzip
	register("C14-SSH-05", func(t *testing.T) Observed {
		var out bytes.Buffer
		gz, _ := gzip.NewWriterLevel(&out, gzip.BestCompression)
		tw := tar.NewWriter(gz)
		const n = 96 << 20
		if err := tw.WriteHeader(&tar.Header{Name: "zeros.bin", Mode: 0o644, Size: n, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := io.CopyN(tw, zeroReader{}, n); err != nil {
			t.Fatal(err)
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		return g3CraftedSave(t, out.Bytes())
	})
	// a partial archive: a valid archive cut in half (the gzip stream ends early)
	register("C15-SSH-01", func(t *testing.T) Observed {
		whole := g3CraftedTgz(t, tar.Header{Name: "big.txt", Mode: 0o644, Typeflag: tar.TypeReg}, strings.Repeat("SYNTHETIC partial payload ", 4000))
		return g3CraftedSave(t, whole[:len(whole)/2])
	})
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}
