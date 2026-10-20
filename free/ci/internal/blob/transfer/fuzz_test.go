package transfer

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// FuzzReceiver drives a Receiver with arbitrary announces and writes. Whatever the input, it must
// not panic, and a blob may be visible in the realm only when its bytes hash to the announced digest.
func FuzzReceiver(f *testing.F) {
	good := pattern(300)
	f.Add(sum(good), int64(300), good, uint8(100), uint8(0))
	f.Add(sum(good), int64(300), good, uint8(30), uint8(2))
	f.Add("zz", int64(5), []byte("hello"), uint8(2), uint8(1))
	f.Add(sum([]byte("a")), int64(-1), []byte("a"), uint8(0), uint8(3))
	f.Add(sum([]byte("abc")), int64(3), []byte("abd"), uint8(1), uint8(0))
	f.Fuzz(func(t *testing.T, sha string, size int64, data []byte, split uint8, op uint8) {
		store, err := cas.New(t.TempDir())
		if err != nil {
			t.Skip()
		}
		r := &Receiver{Store: store, Realm: cas.ArtifactRealm("p"), Lease: "l", Limits: Limits{MaxFile: 1 << 20}, Live: func(context.Context) error { return nil }}
		w, err := r.Open(context.Background(), transport.BlobHeader{Stream: StreamArtifact, SHA256: sha, Size: size})
		if err != nil {
			return
		}
		cut := 0
		if len(data) > 0 {
			cut = int(split) % (len(data) + 1)
		}
		_, _ = w.Write(data[:cut])
		_, _ = w.Write(data[cut:])
		switch op % 4 {
		case 0:
			_ = w.Close()
		case 1:
			_ = w.Verify(sha)
		case 2:
			_ = w.Abort()
			_ = w.Close()
		case 3:
			_ = w.Verify("other")
		}
		if info, err := store.Stat(r.Realm, sha); err == nil {
			if sum(data) != sha || info.Size() != int64(len(data)) {
				t.Fatalf("blob %s is visible but the bytes written hash to %s (%d B)", sha, sum(data), len(data))
			}
		}
	})
}
