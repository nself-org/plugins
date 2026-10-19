package entries

// Purpose: strict encode/decode and validation of the entry manifest.
// Inputs: JSON bytes or a Manifest. Outputs: Manifest, ErrManifest, Validate.
// Constraints: pure. Unknown fields, trailing bytes, an oversized document, a
// non-canonical namespace, an unclean or unsorted blob path and a duplicate
// path are rejected; decode then encode is stable (FuzzManifest).

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

const (
	maxManifestBytes = 8 << 20
	maxBlobs         = 4096 // one tar per spec path; Commit stats each inside the writer transaction
	maxBlobBytes     = 1 << 50
	maxPathBytes     = 1024
)

// ErrManifest wraps every manifest rejection.
var ErrManifest = errors.New("entries: invalid manifest")

var (
	hexKey    = regexp.MustCompile(`^[0-9a-f]{64}$`)
	specID    = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	attemptID = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
)

func bad(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrManifest, fmt.Sprintf(format, a...))
}

// cleanRel reports whether p is a clean relative path with no traversal.
func cleanRel(p string) bool {
	return p != "" && len(p) <= maxPathBytes && p == path.Clean(p) && p != "." && !path.IsAbs(p) &&
		p != ".." && !strings.HasPrefix(p, "../") && !strings.ContainsAny(p, "\x00\\")
}

// Validate checks every field. A bad blob path is reported through
// ErrPathOutside so Commit can refuse with path_outside.
func (m Manifest) Validate() error { return m.validate(true) }

// validate checks every field; Commit passes checkNS false so a foreign or
// malformed namespace reaches the backend, which refuses it (E711 namespace,
// with an audit row) after the attempt's own checks.
func (m Manifest) validate(checkNS bool) error {
	ns, err := authz.DecodeNamespace(m.Namespace)
	switch {
	case checkNS && (err != nil || !bytes.Equal(ns.Canonical(), m.Namespace)):
		return bad("namespace is not canonical")
	case !hexKey.MatchString(m.Key):
		return bad("key is not 64 hex digits")
	case !specID.MatchString(m.Spec):
		return bad("spec id")
	case len(m.Blobs) == 0:
		return bad("an entry needs at least one blob")
	case len(m.Blobs) > maxBlobs:
		return bad("too many blobs")
	case m.CreatedAt < 0 || m.LastUsedAt < 0 || m.Writer.Epoch < 0:
		return bad("negative time or epoch")
	case m.Writer.AttemptID != "" && !attemptID.MatchString(m.Writer.AttemptID):
		return bad("writer attempt id")
	case m.Writer.SourceTrust != "" && !model.TrustClass(m.Writer.SourceTrust).Valid():
		return bad("writer source trust")
	}
	prev, total := "", int64(0)
	for i, b := range m.Blobs {
		total += b.Size
		switch {
		case total > maxBlobBytes:
			return bad("entry larger than %d bytes", int64(maxBlobBytes))
		case !cleanRel(b.Path):
			return fmt.Errorf("%w: %w", ErrPathOutside, bad("blob path %q", b.Path))
		case i > 0 && b.Path <= prev:
			return bad("blob paths must be sorted and unique")
		case !hexKey.MatchString(b.SHA256):
			return bad("blob digest")
		case b.Size < 0 || b.Size > maxBlobBytes:
			return bad("blob size")
		}
		prev = b.Path
	}
	return nil
}

// ErrPathOutside marks a manifest rejected for a blob path outside the entry.
var ErrPathOutside = errors.New("entries: blob path outside the entry")

// EncodeManifest validates and encodes m.
func EncodeManifest(m Manifest) ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

// DecodeManifest strictly decodes and validates one manifest document.
func DecodeManifest(b []byte) (Manifest, error) {
	var m Manifest
	if len(b) > maxManifestBytes {
		return m, bad("document too large")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, bad("%v", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Manifest{}, bad("trailing data")
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}
