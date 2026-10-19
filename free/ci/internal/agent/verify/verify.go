package verify

// Purpose: the one place nself-ci-agent release artifacts are verified (hq NODE D11, P7-NODE-15; P7-CACHE-10 extends it
//          with provenance). A downloaded binary is trusted only after its agent-release/v1 manifest verifies under an
//          agent-purpose key, names the pinned version and the node's target, and its sha256 equals the signed sha256.
// Inputs:  a file path, the manifest bytes and the pinned Expect (version, target); a trusted key set.
// Outputs: the verified Manifest, or an *Error with code E665 and a reason key (see codes.go).
// Constraints: every key-id, purpose and revocation decision is made by cli sdk/go/signing; this package decides nothing
//          about keys itself. The signing input is the plain string scripts/release/agent/manifest.sh signs:
//          "nself-agent-v1\n<target>\n<version>\n<sha256>\n<protocol_min>-<protocol_max>\n". Nothing is placed or executed
//          here; callers place the file only after Artifact returns nil. The production key set is keys_gen.go, generated
//          from scripts/release/keys/agent-*.pub and empty until P7-NODE-29 commits the agent key.

//go:generate bash ../../../../../scripts/release/agent/genkeys.sh keys_gen.go

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/signing"
)

// Schema is the manifest schema this package reads.
const Schema = "agent-release/v1"

// Size bounds: a manifest is a few hundred bytes; the agent is a small static Go binary.
const (
	MaxManifestBytes = 64 << 10
	MaxBinaryBytes   = 256 << 20
)

var (
	versionRe = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)
	targetRe  = regexp.MustCompile(`^(linux|darwin)-(amd64|arm64)$`)
	sha256Re  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Manifest is one signed agent-release/v1 manifest (one per target).
type Manifest struct {
	Schema      string `json:"schema"`
	Version     string `json:"version"`
	Target      string `json:"target"`
	SHA256      string `json:"sha256"`
	ProtocolMin int    `json:"protocol_min"`
	ProtocolMax int    `json:"protocol_max"`
	KeyID       string `json:"key_id"`
	Alg         string `json:"alg"`
	Sig         string `json:"sig"`
}

// Expect is what the caller pinned: the version it asked for and the node's target ("linux-amd64").
type Expect struct{ Version, Target string }

// Target spells a platform the way the release files and manifests do.
func Target(goos, goarch string) string { return goos + "-" + goarch }

// ParseManifest reads one manifest strictly: a single JSON object, no unknown fields, canonical field shapes.
func ParseManifest(data []byte) (Manifest, error) {
	var m Manifest
	if len(data) == 0 || len(data) > MaxManifestBytes {
		return m, fail(ReasonMalformedManifest, nil, "manifest size %d is outside 1..%d", len(data), MaxManifestBytes)
	}
	if dup := duplicateKey(data); dup {
		return m, fail(ReasonMalformedManifest, nil, "manifest repeats a field")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fail(ReasonMalformedManifest, nil, "manifest is not a valid agent-release object")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Manifest{}, fail(ReasonMalformedManifest, nil, "manifest has data after the object")
	}
	if m.KeyID == "" || m.Sig == "" {
		return Manifest{}, fail(ReasonMissingSignature, nil, "manifest carries no signature")
	}
	if err := m.validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// duplicateKey reports whether the top-level JSON object repeats a field name, compared case-insensitively because encoding/json
// matches field names that way and would keep the last one silently.
// Input that is not an object is left to the strict decode that follows.
func duplicateKey(data []byte) bool {
	dec := json.NewDecoder(bytes.NewReader(data))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	for dec.More() {
		k, err := dec.Token()
		name, ok := k.(string)
		if err != nil || !ok {
			return false
		}
		folded := strings.ToLower(name)
		if seen[folded] {
			return true
		}
		seen[folded] = true
		var skip json.RawMessage
		if err = dec.Decode(&skip); err != nil {
			return false
		}
	}
	return false
}

func (m Manifest) validate() error {
	switch {
	case m.Schema != Schema:
		return fail(ReasonMalformedManifest, nil, "schema %q is not %s", clip(m.Schema), Schema)
	case !versionRe.MatchString(m.Version):
		return fail(ReasonMalformedManifest, nil, "version %q is not a plain version", clip(m.Version))
	case !targetRe.MatchString(m.Target):
		return fail(ReasonMalformedManifest, nil, "target %q is not <os>-<arch>", clip(m.Target))
	case !sha256Re.MatchString(m.SHA256):
		return fail(ReasonMalformedManifest, nil, "sha256 is not 64 lower-case hex characters")
	case m.ProtocolMin < 1 || m.ProtocolMax < m.ProtocolMin:
		return fail(ReasonMalformedManifest, nil, "protocol range %d-%d is not valid", m.ProtocolMin, m.ProtocolMax)
	case m.Alg != "ed25519":
		return fail(ReasonMalformedManifest, nil, "alg %q is not ed25519", clip(m.Alg))
	}
	return nil
}

func clip(s string) string {
	if len(s) > 48 {
		return s[:48] + "..."
	}
	return s
}

// SigningInput is the byte string the release signs. It refuses a manifest whose fields are not in canonical shape,
// so no field can carry a newline that shifts the others.
func SigningInput(m Manifest) ([]byte, error) {
	if err := m.validate(); err != nil {
		return nil, err
	}
	return fmt.Appendf(nil, "nself-agent-v1\n%s\n%s\n%s\n%d-%d\n", m.Target, m.Version, m.SHA256, m.ProtocolMin, m.ProtocolMax), nil
}

// Verifier verifies agent artifacts against a fixed trusted key set.
type Verifier struct {
	sv    *signing.Verifier
	nkeys int
}

// New builds a Verifier over keys and a revoked-id list. The keys should be agent-purpose keys with derived ids; a key of
// another purpose in the set is refused at verify time with wrong_purpose.
func New(keys []signing.Key, revoked []string, opts ...signing.Option) (*Verifier, error) {
	sv, err := signing.NewVerifier(signing.PurposeAgent, keys, revoked, opts...)
	if err != nil {
		return nil, fail(ReasonUnknownKey, err, "trusted key set is not usable")
	}
	return &Verifier{sv: sv, nkeys: len(keys)}, nil
}

// Default is the Verifier over the compiled-in key set (keys_gen.go). With no key compiled in, every Artifact call fails
// with no_trusted_key.
func Default() (*Verifier, error) {
	keys := make([]signing.Key, 0, len(generatedKeys))
	for _, e := range generatedKeys {
		pub, err := base64.StdEncoding.Strict().DecodeString(e.Public)
		if err != nil || len(pub) != ed25519.PublicKeySize {
			return nil, fail(ReasonUnknownKey, nil, "compiled-in key %s is not a 32-byte key", e.ID)
		}
		keys = append(keys, signing.Key{ID: e.ID, Purpose: signing.PurposeAgent, Public: pub})
	}
	return New(keys, generatedRevoked)
}

// HasKeys reports whether any key is trusted.
func (v *Verifier) HasKeys() bool { return v != nil && v.nkeys > 0 }

// Ready is nil when a key is trusted, and the no_trusted_key error (E665, naming --agent-binary) otherwise. Callers use it to
// refuse before downloading anything.
func (v *Verifier) Ready() error {
	if v.HasKeys() {
		return nil
	}
	return fail(ReasonNoTrustedKey, nil, "no agent signing key is built in; pass --agent-binary <path> to install a binary you trust")
}

// ValidVersion reports whether s is a plain release version (1.5.0 or 1.5.0-rc.1).
func ValidVersion(s string) bool { return versionRe.MatchString(s) }

// Artifact verifies the file at path against the manifest bytes and the pinned Expect. It returns the manifest only when the
// signature verifies, the signed version and target are the pinned ones, and the file's sha256 is the signed sha256.
func (v *Verifier) Artifact(path string, manifest []byte, want Expect) (Manifest, error) {
	if err := v.Ready(); err != nil {
		return Manifest{}, err
	}
	m, err := ParseManifest(manifest)
	if err != nil {
		return Manifest{}, err
	}
	sig, derr := signing.DecodeSig(m.Sig)
	if derr != nil {
		return Manifest{}, fail(ReasonMalformedSig, derr, "signature is not canonical base64")
	}
	input, err := SigningInput(m)
	if err != nil {
		return Manifest{}, err
	}
	if err := v.sv.Verify(input, signing.Signature{KeyID: m.KeyID, Sig: sig}); err != nil {
		return Manifest{}, signingFailure(err, m.KeyID)
	}
	if m.Version != want.Version {
		return Manifest{}, fail(ReasonVersionMismatch, nil, "manifest is for version %s, the pinned version is %s", m.Version, clip(want.Version))
	}
	if m.Target != want.Target {
		return Manifest{}, fail(ReasonTargetMismatch, nil, "manifest is for %s, the node is %s", m.Target, clip(want.Target))
	}
	sum, err := fileSHA256(path)
	if err != nil {
		return Manifest{}, err
	}
	if sum != m.SHA256 {
		return Manifest{}, fail(ReasonChecksumMismatch, nil, "file sha256 %s is not the signed sha256", sum)
	}
	return m, nil
}

// signingFailure maps an sdk/go/signing sentinel to a reason key.
func signingFailure(err error, keyID string) *Error {
	reasons := []struct {
		sentinel error
		reason   string
	}{
		{signing.ErrRevoked, ReasonRevoked}, {signing.ErrUnknownKey, ReasonUnknownKey}, {signing.ErrWrongPurpose, ReasonWrongPurpose},
		{signing.ErrWrongScope, ReasonWrongPurpose}, {signing.ErrExpired, ReasonKeyNotValid}, {signing.ErrNotYetValid, ReasonKeyNotValid},
		{signing.ErrBadSignature, ReasonBadSignature}, {signing.ErrMalformed, ReasonMalformedManifest}, // a bad key id; the signature text was checked before
	}
	for _, r := range reasons {
		if errors.Is(err, r.sentinel) {
			return fail(r.reason, err, "key %s: %s", clip(keyID), r.reason)
		}
	}
	return fail(ReasonBadSignature, err, "key %s did not verify", clip(keyID))
}

func fileSHA256(path string) (string, error) {
	// A symlink is refused: the bytes read must be the bytes at the path the caller then uses.
	if lst, err := os.Lstat(path); err != nil || !lst.Mode().IsRegular() {
		return "", fail(ReasonUnreadable, err, "artifact is not a regular file (symlinks are refused)")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fail(ReasonUnreadable, err, "cannot open the artifact")
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() == 0 || st.Size() > MaxBinaryBytes {
		return "", fail(ReasonUnreadable, err, "artifact is not a regular file of 1..%d bytes", MaxBinaryBytes)
	}
	h := sha256.New()
	if _, err = io.Copy(h, io.LimitReader(f, MaxBinaryBytes+1)); err != nil {
		return "", fail(ReasonUnreadable, err, "cannot read the artifact")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
