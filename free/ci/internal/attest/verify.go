package attest

// Purpose: local verification of an evidence document: schema, strict decode
// and internal consistency (revision format, binding against worktree state,
// producer against signature kind, counts, result against checks). It proves
// the document is well formed and does not contradict itself; it does not
// prove who wrote it (a runner signature needs the remote verifier).
// Inputs:  the raw JSON document, or a decoded model.Evidence.
// Outputs: the decoded evidence, or a *Rejection carrying E685 reason keys.
// Constraints: pure; bounded input; the document's own `freshness` is never
// consulted; unknown fields and trailing data are refused.

import (
	"bytes"
	"encoding/json"
	"regexp"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// MaxDocument bounds the size of a document Verify will parse.
const MaxDocument = 16 << 20

var revisionRe = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)

var (
	schemaOnce  sync.Once
	schemaBytes []byte
	schemaErr   error
)

func evidenceSchema() ([]byte, error) {
	schemaOnce.Do(func() {
		all, err := model.Schemas()
		if err != nil {
			schemaErr = err
			return
		}
		schemaBytes = all["ci-evidence.v1.schema.json"]
	})
	return schemaBytes, schemaErr
}

// Verify checks a raw ci.evidence/v1 document and returns it decoded.
func Verify(doc []byte) (model.Evidence, error) {
	var ev model.Evidence
	if len(doc) == 0 || len(doc) > MaxDocument {
		return ev, Reject([]Reason{ReasonInvalid}, "document is empty or too large")
	}
	schema, err := evidenceSchema()
	if err != nil {
		return ev, Reject([]Reason{ReasonInvalid}, "evidence schema unavailable")
	}
	if err := model.ValidateJSON(schema, doc); err != nil {
		return ev, Reject([]Reason{ReasonInvalid}, err.Error())
	}
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ev); err != nil {
		return model.Evidence{}, Reject([]Reason{ReasonInvalid}, err.Error())
	}
	set := reasonSet{}
	set.add(consistency(ev)...)
	if ev.Producer.Kind == "signed-remote" || ev.Signature.Kind == "runner-ed25519" {
		set.add(verifySignature(doc, ev)...)
	}
	if reasons := set.list(); len(reasons) > 0 {
		return model.Evidence{}, Reject(reasons, "")
	}
	return ev, nil
}

// Consistent reports the reasons a decoded document contradicts itself.
func Consistent(ev model.Evidence) []Reason { return consistency(ev) }

func consistency(ev model.Evidence) []Reason {
	set := reasonSet{}
	if ev.Schema != "ci.evidence/v1" || !revisionRe.MatchString(ev.Revision) || !enumsValid(ev) {
		return []Reason{ReasonInvalid}
	}
	if !bindingConsistent(ev) || !producerConsistent(ev) || !countsConsistent(ev) {
		set.add(ReasonInvalid)
	}
	if ev.Result == "pass" && model.Verdict(ev.Checks) != "pass" {
		set.add(ReasonInvalid) // a pass the checks do not support
	}
	return set.list()
}

func enumsValid(ev model.Evidence) bool {
	if !ev.SourceTrust.Valid() || !ev.Trigger.Valid() || !ev.Pipeline.Kind.Valid() || !ev.Selection.Mode.Valid() ||
		!ev.Result.Valid() || !ev.Binding.State.Valid() || !ev.WorktreeState.Valid() ||
		!ev.Producer.Kind.Valid() || !ev.Signature.Kind.Valid() {
		return false
	}
	for _, c := range ev.Checks {
		if !c.Result.Valid() {
			return false
		}
	}
	return true
}

// bindingConsistent: bound means a clean worktree, no tracked changes and no
// reason; a dirty worktree is never bound; unbound names its reason.
func bindingConsistent(ev model.Evidence) bool {
	switch ev.Binding.State {
	case "bound":
		return ev.WorktreeState == "clean" && ev.TrackedChangesDigest == nil && ev.Binding.Reason == nil
	default:
		return ev.Binding.Reason != nil
	}
}

// producerConsistent ties the producer kind to the signature kind.
func producerConsistent(ev model.Evidence) bool {
	s := ev.Signature
	if ev.Producer.Kind == "signed-remote" {
		return s.Kind == "runner-ed25519" && s.KeyID != nil && *s.KeyID != "" && s.Value != nil && *s.Value != ""
	}
	return s.Kind == "coordinator-recorded" && s.KeyID == nil && s.Value == nil
}

func countsConsistent(ev model.Evidence) bool {
	var c model.Counts
	for _, check := range ev.Checks {
		switch check.Result {
		case "pass":
			c.Pass++
		case "fail":
			c.Fail++
		case "skip":
			c.Skip++
		case "error":
			c.Error++
		}
	}
	c.Total = len(ev.Checks)
	return c == ev.Counts
}
