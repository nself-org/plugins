package update

// Purpose: the verified release a node is upgraded to: the signed binary and the manifest that was verified with it
//          (hq NODE D11, P7-NODE-15's install.Release does the download and the verification).
// Inputs:  a node platform; an install.Release (pinned version, release URL, verifier).
// Outputs: Prepared: a verified binary in a private directory and its parsed manifest (protocol range included).
// Constraints: nothing is returned before install.Release has verified signature, key purpose, manifest, checksum, SBOMs and
//          provenance; the manifest read here is the very bytes that call verified (recorded by a transport wrapper, not fetched a
//          second time); the binary is never executed here.

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/install"
	"github.com/nself-org/plugins/free/ci/internal/agent/verify"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

// Prepared is a verified release for one platform.
type Prepared struct {
	// Path is the verified binary; Cleanup removes its private directory.
	Path     string
	Manifest verify.Manifest
	Cleanup  func()
}

// Source produces the verified release for a node's platform.
type Source interface {
	Prepare(ctx context.Context, p lifecycle.Platform) (Prepared, error)
}

// ReleaseSource is the production Source: the signed GitHub release (or mirror) of one pinned version.
type ReleaseSource struct{ Release install.Release }

// Prepare implements Source.
func (s ReleaseSource) Prepare(ctx context.Context, p lifecycle.Platform) (Prepared, error) {
	rec := &recorder{}
	r := s.Release
	base := r.Client
	if base == nil {
		base = &http.Client{Timeout: 5 * time.Minute}
	}
	c := *base
	rec.next = c.Transport
	c.Transport = rec
	r.Client = &c
	path, cleanup, err := r.Fetch(ctx, p)
	if err != nil {
		return Prepared{}, err
	}
	m, err := verify.ParseManifest(rec.manifest())
	if err != nil {
		cleanup()
		return Prepared{}, err
	}
	return Prepared{Path: path, Manifest: m, Cleanup: cleanup}, nil
}

// recorder keeps the body of the manifest response while passing every response through.
type recorder struct {
	next http.RoundTripper
	mu   sync.Mutex
	body []byte
}

func (r *recorder) manifest() []byte { r.mu.Lock(); defer r.mu.Unlock(); return r.body }

func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	next := r.next
	if next == nil {
		next = http.DefaultTransport
	}
	resp, err := next.RoundTrip(req)
	if err != nil || resp.StatusCode != http.StatusOK || !strings.HasSuffix(req.URL.Path, ".manifest.json") {
		return resp, err
	}
	b, rerr := io.ReadAll(io.LimitReader(resp.Body, verify.MaxManifestBytes+1))
	_ = resp.Body.Close()
	if rerr != nil {
		return nil, rerr
	}
	r.mu.Lock()
	r.body = b
	r.mu.Unlock()
	resp.Body = io.NopCloser(bytes.NewReader(b))
	return resp, nil
}
