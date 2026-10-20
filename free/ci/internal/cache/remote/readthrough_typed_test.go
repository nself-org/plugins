package remote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
)

func TestReadThroughDigestRefusalIsTyped(t *testing.T) {
	for _, data := range []string{"bad", "short"} {
		t.Run(data, func(t *testing.T) {
			c, err := NewReadThrough(t.TempDir(), 100)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256([]byte("correct"))
			f, err := c.Open(context.Background(), "realm", hex.EncodeToString(sum[:]), 7, func(_ context.Context, w io.Writer) error {
				_, err := io.WriteString(w, data)
				return err
			})
			if f != nil {
				_ = f.Close()
				t.Fatal("corrupt copy returned")
			}
			var refusal *transfer.Error
			if !errors.As(err, &refusal) || refusal.Reason != transfer.ReasonDigest {
				t.Fatalf("want typed digest refusal, got %T: %v", err, err)
			}
		})
	}
}
