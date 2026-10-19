package signing

// Purpose: re-check an in-toto statement payload (the release provenance)
// before it is signed (Epic CACHE D12: statement subjects likewise).
// Inputs:  payload bytes and the run's facts.
// Outputs: nil when every subject is a committed, byte-verified digest of the
// run, or the run's stored evidence.
// Constraints: strict JSON (no duplicate or case-variant keys, nothing after the
// statement); each subject is the run's evidence under its manifest name or a
// committed record under its own name; statement type v1 and the SLSA provenance predicate type of
// the release manifest contract; at most maxSubjects subjects; each subject
// has a name and exactly one sha256 digest; the first failure is returned.

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
)

const (
	statementType = "https://in-toto.io/Statement/v1"
	maxSubjects   = 4096
)

var sha256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)

type statement struct {
	Type          string          `json:"_type"`
	PredicateType string          `json:"predicateType"`
	Subject       []statementSubj `json:"subject"`
	Predicate     json.RawMessage `json:"predicate"`
}

type statementSubj struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

// checkStatement validates one in-toto statement payload.
func (f *runFacts) checkStatement(ctx context.Context, payload []byte) error {
	if err := strictJSON(payload, "statement"); err != nil {
		return err
	}
	var st statement
	if err := json.NewDecoder(bytes.NewReader(payload)).Decode(&st); err != nil {
		return Failf(ReasonPayloadRejected, "statement: not a JSON in-toto statement")
	}
	if st.Type != statementType || st.PredicateType != manifest.ProvenancePredicate || len(st.Predicate) == 0 {
		return Failf(ReasonPayloadRejected, "statement: type or predicateType is not the v1 provenance statement")
	}
	if len(st.Subject) == 0 || len(st.Subject) > maxSubjects {
		return Failf(ReasonPayloadRejected, "statement: it needs 1 to %d subjects", maxSubjects)
	}
	for _, s := range st.Subject {
		d, ok := s.Digest["sha256"]
		if s.Name == "" || !ok || len(s.Digest) != 1 || !sha256Re.MatchString(d) {
			return Failf(ReasonPayloadRejected, "statement: subject %q needs a name and one sha256 digest", echo(s.Name))
		}
		ok, err := f.subjectBacked(ctx, s.Name, d)
		if err != nil {
			return err
		}
		if !ok {
			return Failf(ReasonDigestNotCommitted, "statement: subject %q (digest %s) is not a committed record of this run under that name", echo(s.Name), echo(d))
		}
	}
	return nil
}

// subjectBacked reports whether a subject is the run's evidence under its
// manifest name, or a committed, verified record of the run with this digest
// and this name (or this name under artifacts/, the release layout).
func (f *runFacts) subjectBacked(ctx context.Context, name, digest string) (bool, error) {
	if digest == f.evDigest && name == manifest.EvidencePath {
		return true, nil
	}
	names := []string{name}
	if rest, ok := strings.CutPrefix(name, "artifacts/"); ok {
		names = append(names, rest)
	}
	for _, n := range names {
		ok, err := f.match(ctx, n, "", digest, func(records.Record) bool { return true })
		if err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}
