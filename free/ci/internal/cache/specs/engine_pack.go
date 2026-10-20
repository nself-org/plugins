package specs

// Stream bounded cache archives through value scanning into blob storage.

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
)

var errSizeCap = errors.New("specs: the packed cache exceeds the entry byte cap")

// capWriter counts stored bytes and fails past the cap.
type capWriter struct {
	w   io.Writer
	n   int64
	cap int64
}

func (c *capWriter) Write(p []byte) (int, error) {
	if c.n += int64(len(p)); c.n > c.cap {
		return 0, errSizeCap
	}
	return c.w.Write(p)
}

// putDir streams dir through tar and scan into the blob store. The local blob
// is a plain tar: disk is cheap and a restore is on a job's critical path, so a
// copy beats an inflate (D-1008). Compress only when a blob leaves the host.
// With inflate the scan also looks inside the gzip payloads of the files
// (Spec.InflateScan, D-1037).
func (e *Engine) putDir(ctx context.Context, base *os.Root, realm cas.Realm, rel string, budget int64, inflate bool) (string, int64, PackStats, error) {
	pr, pw := io.Pipe()
	var st PackStats
	done := make(chan struct{})
	go func() {
		defer close(done)
		var err error
		defer func() { _ = pw.CloseWithError(err) }()
		sw := newScanWriter(&capWriter{w: pw, cap: budget}, e.Scanner)
		var inf *inflater
		if inflate {
			inf = newInflater(e.Scanner)
		}
		if st, err = pack(ctx, base, rel, sw, cacheLimits(), inf); err == nil {
			err = sw.Close()
		}
	}()
	digest, size, err := e.Blobs.Put(ctx, realm, pr)
	_ = pr.CloseWithError(err)
	<-done
	return digest, size, st, err
}
