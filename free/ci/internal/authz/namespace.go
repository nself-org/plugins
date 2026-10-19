package authz

// Purpose: the cache namespace (ADR 0025 §7, R-SEC-19, R-SEC-30). A namespace
// is a struct whose fields are compared on every access; its canonical bytes
// are a stored manifest field, never only a hash input.
// Inputs:  project id, trust class, branch protection, head repo, PR.
// Outputs: Namespace, CacheNamespace, Namespace.Canonical/Equal, DecodeNamespace.
// Constraints: pure. An untrusted job always gets a per-(head repo, PR)
// namespace, so two PRs never share one; a trusted class never carries a head
// repo or PR, so it can never equal an untrusted namespace. Unknown trust
// decodes to untrusted (most restrictive).

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// Protection says whether the ref the job runs for is protected.
type Protection string

const (
	// Protected is a protected branch (or tag) of the project's own repo.
	Protected Protection = "protected"
	// Unprotected is every other ref.
	Unprotected Protection = "unprotected"
)

// encodingTag versions the canonical encoding; a change bumps it so old
// stored namespaces never compare equal to new ones.
const encodingTag = "nself-ci-cache-ns/1"

// Namespace identifies one cache realm owner. Compare with Equal, never with
// a hash: the store keeps the Canonical bytes and every access re-derives and
// compares the fields.
type Namespace struct {
	Project    string
	Trust      model.TrustClass
	Protection Protection
	HeadRepo   string // untrusted only
	PR         string // untrusted only
}

// untrusted reports whether n is an untrusted-class namespace.
func (n Namespace) untrusted() bool { return n.Trust == "untrusted" }

// Valid reports whether n is well formed: a project, a known trust class, a
// known protection, and head repo plus PR exactly when the class is untrusted.
func (n Namespace) Valid() bool { return n.check() == nil }

func (n Namespace) check() error {
	switch {
	case n.Project == "":
		return errors.New("project is empty")
	case trust.RankTrust(n.Trust) < 0:
		return fmt.Errorf("unknown trust class %q", n.Trust)
	case n.Protection != Protected && n.Protection != Unprotected:
		return fmt.Errorf("unknown protection %q", n.Protection)
	case n.untrusted() && (n.HeadRepo == "" || n.PR == ""):
		return errors.New("untrusted namespace needs head repo and PR")
	case !n.untrusted() && (n.HeadRepo != "" || n.PR != ""):
		return errors.New("trusted namespace carries no head repo or PR")
	case n.untrusted() && n.Protection == Protected:
		return errors.New("untrusted namespace is never protected")
	}
	return nil
}

// Equal reports whether the two namespaces are the same field for field. An
// invalid namespace equals nothing, not even itself.
func (n Namespace) Equal(o Namespace) bool { return n == o && n.Valid() }

// CacheNamespace derives the namespace of a job. class is the trust class the
// coordinator recorded for the job's source (never a value the job supplies);
// an unknown class is untrusted. For an untrusted job headRepo and pr are
// required: their absence is an error (E684), not a shared bucket. For a
// trusted job they are ignored and cleared. A protected ref only counts as
// protected for a trusted class; any other protection value reads unprotected.
func CacheNamespace(project string, class model.TrustClass, protection Protection, headRepo, pr string) (Namespace, error) {
	class = trust.DecodeTrust(string(class))
	n := Namespace{Project: project, Trust: class, Protection: Unprotected}
	if n.untrusted() {
		n.HeadRepo, n.PR = headRepo, pr
	} else if protection == Protected {
		n.Protection = Protected
	}
	if err := n.check(); err != nil {
		return Namespace{}, &Denied{Reason: ReasonNamespace, Detail: err.Error()}
	}
	return n, nil
}

// Canonical returns the length-prefixed encoding (R-SEC-30): the tag, then
// each field as a 4-byte big-endian length followed by its bytes. The result
// is injective: no two distinct namespaces encode alike.
func (n Namespace) Canonical() []byte {
	var out []byte
	for _, f := range []string{encodingTag, n.Project, string(n.Trust), string(n.Protection), n.HeadRepo, n.PR} {
		out = binary.BigEndian.AppendUint32(out, uint32(len(f)))
		out = append(out, f...)
	}
	return out
}

// DecodeNamespace parses Canonical bytes. It rejects trailing bytes, a wrong
// tag, a truncated field and an invalid namespace.
func DecodeNamespace(b []byte) (Namespace, error) {
	var fields []string
	for len(b) > 0 {
		if len(b) < 4 {
			return Namespace{}, errors.New("truncated length")
		}
		size := binary.BigEndian.Uint32(b)
		b = b[4:]
		if uint64(size) > uint64(len(b)) {
			return Namespace{}, errors.New("truncated field")
		}
		fields = append(fields, string(b[:size]))
		b = b[size:]
	}
	if len(fields) != 6 || fields[0] != encodingTag {
		return Namespace{}, errors.New("not a cache namespace encoding")
	}
	n := Namespace{Project: fields[1], Trust: model.TrustClass(fields[2]), Protection: Protection(fields[3]), HeadRepo: fields[4], PR: fields[5]}
	if err := n.check(); err != nil {
		return Namespace{}, err
	}
	return n, nil
}
