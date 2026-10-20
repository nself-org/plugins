package remote

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/transport"
)

func TestRemoteRefusalsAreAudited(t *testing.T) {
	for _, name := range []string{"not_declared", "missing_blob", "stored_size", "transfer_digest"} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, jobWith("out/a"))
			data := pattern(100)
			var err error
			switch name {
			case "not_declared":
				err = r.lease.ArtifactRef(context.Background(), ref("other", data))
			case "missing_blob":
				err = r.lease.ArtifactRef(context.Background(), ref("out/a", data))
			case "stored_size":
				if err = r.upload(data); err != nil {
					t.Fatal(err)
				}
				v := ref("out/a", data)
				v.Size--
				err = r.lease.ArtifactRef(context.Background(), v)
			case "transfer_digest":
				w, e := r.lease.Blob(context.Background(), transport.BlobHeader{Stream: "artifact", SHA256: sum(data), Size: 100})
				if e != nil {
					t.Fatal(e)
				}
				if _, e = w.Write(make([]byte, 100)); e != nil {
					t.Fatal(e)
				}
				err = w.Close()
			}
			if err == nil {
				t.Fatal("expected refusal")
			}
			if n := r.audits("artifact.remote.refused"); n != 1 {
				t.Fatalf("got %d refusal audits, want 1", n)
			}
		})
	}
}
