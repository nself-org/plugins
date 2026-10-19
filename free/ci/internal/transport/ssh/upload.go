package ssh

// Purpose: coordinator to agent transfers: the checkout bundle as chunk frames
// and the value-bearing secrets frame.
// Inputs:  the bundle file made by checkout.Create; the SecretSource.
// Outputs: chunk frames on stream "checkout" (base64, in order, final marked);
// one secrets frame.
// Constraints: a chunk is at most 192 KiB raw (about 256 KiB framed, far under
// the 1 MiB frame limit); secret values are sent only when the coordinator is
// PERSONAL and the carrier policy allows them, are never logged or put in an
// error, and are dropped from memory when the call returns.

import (
	"context"
	"encoding/base64"
	"io"
	"os"

	"github.com/nself-org/plugins/free/ci/internal/agent/checkout"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

const chunkSize = 192 << 10

func (s *sess) upload(ctx context.Context, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return newErr(CodeTransport, ReasonCheckout, "cannot read the checkout bundle")
	}
	defer func() { _ = f.Close() }()
	total := s.lease.Checkout.Size
	buf := make([]byte, chunkSize)
	var off int64
	for off < total {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := io.ReadFull(f, buf)
		if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
			return newErr(CodeTransport, ReasonCheckout, "cannot read the checkout bundle")
		}
		if n == 0 {
			return newErr(CodeTransport, ReasonCheckout, "the checkout bundle is shorter than announced")
		}
		c := protocol.Chunk{Stream: checkout.Stream, Offset: off, Data: base64.StdEncoding.EncodeToString(buf[:n]), Final: off+int64(n) >= total}
		if err := s.send("chunk", c, true); err != nil {
			return s.lostErr(err)
		}
		off += int64(n)
	}
	return nil
}

// sendSecrets sends the lease's secret values once, in memory only.
func (s *sess) sendSecrets(ctx context.Context) error {
	if !s.d.Personal || s.d.Secrets == nil || len(s.lease.Secrets) == 0 {
		return nil
	}
	vals, err := s.d.Secrets(ctx, s.lease)
	if err != nil {
		return newErr(CodeTransport, ReasonHandshake, "secret values for lease %s could not be resolved", s.lease.LeaseID)
	}
	defer func() {
		for i := range vals {
			vals[i].Value = ""
		}
	}()
	if len(vals) == 0 {
		return nil
	}
	if err := s.send("secrets", protocol.Secrets{LeaseID: s.lease.LeaseID, Values: vals}, true); err != nil {
		return s.lostErr(err)
	}
	return nil
}
