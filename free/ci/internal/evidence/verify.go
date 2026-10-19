package evidence

// Purpose: judge one evidence document for one revision and map the verdict
// to the `ci evidence verify` exit codes (Epic D11): 0 current bound passing
// evidence, 1 the evidence fails, 10 stale, 11 invalid, 12 unbound.
// Inputs:  a decoded or raw document, the full revision it must describe and
// the digests of the inputs at that revision (nil when they are unknown).
// Outputs: a Verdict holding the report `verify --json` prints.
// Constraints: pure; freshness is recomputed here and the document's own
// `freshness` is never trusted; unknown inputs are stale, never current;
// adhoc and drill evidence never gates; a document that does not pass
// attest.Verify is invalid whatever else it says.

import (
	"encoding/json"
	"io/fs"
	"regexp"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Exit codes of `ci evidence verify`.
const (
	ExitOK       = 0
	ExitFails    = 1
	ExitStale    = 10
	ExitInvalid  = 11
	ExitUnbound  = 12
	ExitInternal = 2
)

// Class is what kind of evidence a document is.
type Class string

// Evidence classes (P7-DEPL delta 8).
const (
	ClassDefault Class = "default"
	ClassNamed   Class = "named"
	ClassDrill   Class = "drill"
	ClassAdhoc   Class = "adhoc"
	ClassRelease Class = "release"
)

// Report is the data of `ci evidence verify --json`.
type Report struct {
	Revision    string          `json:"revision"`
	Result      model.Result    `json:"result"`
	Class       Class           `json:"class"`
	Binding     model.Binding   `json:"binding"`
	Freshness   model.Freshness `json:"freshness"`
	Quarantined bool            `json:"quarantined"`
	Evidence    model.Evidence  `json:"evidence"`
}

// Verdict is a Report with the exit code and the reasons behind it.
type Verdict struct {
	Report   Report
	Exit     int
	Reasons  []attest.Reason
	Warnings []string
}

// ClassOf names the class of a document: release when a passing release-kind
// job is present, otherwise the pipeline kind.
func ClassOf(ev model.Evidence) Class {
	for _, c := range ev.Checks {
		if c.Kind == "release" && c.Result == "pass" {
			return ClassRelease
		}
	}
	switch ev.Pipeline.Kind {
	case "named":
		return ClassNamed
	case "drill":
		return ClassDrill
	case "adhoc":
		return ClassAdhoc
	}
	return ClassDefault
}

// Judge decides what ev is worth for revision want. current is the digest set
// of the inputs at want; nil means the inputs could not be read.
func Judge(ev model.Evidence, want string, current *model.InputDigests) Verdict {
	fresh := model.Freshness{State: "unknown"}
	reason := "current inputs were not supplied"
	fresh.Reason = &reason
	if current != nil {
		fresh = attest.Fresh(ev, *current)
	}
	ev.Freshness = fresh
	v := Verdict{Report: Report{Revision: want, Result: ev.Result, Class: ClassOf(ev), Binding: ev.Binding, Freshness: fresh, Evidence: ev}}
	add := func(exit int, r attest.Reason) {
		if v.Exit == 0 || exitRank(exit) < exitRank(v.Exit) {
			v.Exit = exit
		}
		v.Reasons = append(v.Reasons, r)
	}
	if rs := attest.Consistent(ev); len(rs) > 0 {
		for _, r := range rs {
			add(ExitInvalid, r)
		}
	}
	if ev.Revision != want {
		add(ExitInvalid, attest.ReasonRevisionMismatch)
	}
	if ev.Binding.State != "bound" || ev.WorktreeState != "clean" {
		add(ExitUnbound, attest.ReasonUnbound)
	}
	if fresh.State != "current" {
		add(ExitStale, attest.ReasonStale)
	}
	if k := ev.Pipeline.Kind; k == "adhoc" || k == "drill" { // the pipeline kind, not the class: a release check must not hide it
		add(ExitFails, attest.ReasonAdhoc)
	}
	if ev.Selection.Mode != "full" { // an affected-only pass proves less than the required policy
		add(ExitFails, attest.ReasonSelectionPartial)
	}
	if ev.Result != "pass" {
		add(ExitFails, attest.ReasonNotPassing)
	}
	return v
}

// exitRank orders the exit codes by how fundamental the problem is: invalid,
// then unbound, then stale, then a failing result.
func exitRank(exit int) int {
	switch exit {
	case ExitInvalid:
		return 0
	case ExitUnbound:
		return 1
	case ExitStale:
		return 2
	case ExitFails:
		return 3
	}
	return 4
}

// VerifyDocument checks a raw document against want. recorded returns the
// document this machine stored for the same run (nil when it knows none, or
// when the function is nil): a document that differs from it was edited and
// is invalid. The returned error is set, with exit 11, when the document
// cannot be decoded at all.
func VerifyDocument(doc []byte, want string, current *model.InputDigests, recorded func(model.Evidence) *model.Evidence) (Verdict, error) {
	ev, err := attest.Verify(doc)
	if err != nil {
		return Verdict{Exit: ExitInvalid}, fail(CodeRejected, ExitInvalid, "%s", firstLine(err.Error()))
	}
	v := Judge(ev, want, current)
	var stored *model.Evidence
	if recorded != nil {
		stored = recorded(ev)
	}
	if stored != nil && !sameDocument(ev, *stored) {
		v.Exit = ExitInvalid
		v.Reasons = append(v.Reasons, attest.ReasonInvalid)
		v.Warnings = append(v.Warnings, "the document differs from the evidence this machine recorded for run "+ev.RunID)
	}
	if stored == nil {
		authenticate(&v, doc, ev, want, current)
	}
	return v, nil
}

// authenticate settles what a document is worth when this machine holds no
// record of its run. Nothing in the file proves who wrote it, so a verdict of
// "pass" is refused (exit 11) unless a registered verifier vouches for a
// signed-remote document. A document that already fails, is stale or is
// unbound keeps that exit: forging can only make a document look better.
func authenticate(v *Verdict, doc []byte, ev model.Evidence, want string, current *model.InputDigests) {
	if ev.Producer.Kind == "signed-remote" {
		g := attest.Gate{RepoID: ev.RepoID, Revision: want, PolicyDigest: ev.PolicyDigest, MinTrust: ev.SourceTrust, FullSelection: true, Current: current}
		for _, c := range ev.Checks {
			if c.Required {
				g.RequiredJobs = append(g.RequiredJobs, c.JobID)
			}
		}
		for _, r := range attest.GateAcceptDocument(doc, g) {
			if r == attest.ReasonSignatureUnverifiable || r == attest.ReasonInvalid {
				v.reject(r, "the signature of run "+ev.RunID+" cannot be verified here")
				return
			}
		}
		return
	}
	why := "this machine has no record of run " + ev.RunID + ": the document's origin is not authenticated"
	if ev.Producer.Kind == "self-reported" {
		why = "self-reported evidence is not admitted"
	}
	if v.Exit == ExitOK {
		v.reject(attest.ReasonInvalid, why)
		return
	}
	v.Warnings = append(v.Warnings, why)
}

// reject turns the verdict into an invalid-evidence one.
func (v *Verdict) reject(r attest.Reason, warning string) {
	v.Exit = ExitInvalid
	v.Reasons = append(v.Reasons, r)
	v.Warnings = append(v.Warnings, warning)
}

// sameDocument compares two documents ignoring the read-time freshness.
func sameDocument(a, b model.Evidence) bool {
	a.Freshness, b.Freshness = model.Freshness{}, model.Freshness{}
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(x) == string(y)
}

// MissingInputs lists the lockfile and generated paths the document binds
// that do not exist in the tree of the revision.
func MissingInputs(ev model.Evidence, tree fs.FS) []string {
	var out []string
	for _, group := range [][]model.DigestRef{ev.InputDigests.Lockfiles, ev.InputDigests.Generated} {
		for _, r := range group {
			if _, err := fs.Stat(tree, r.Path); err != nil {
				out = append(out, r.Path)
			}
		}
	}
	return out
}

// MarkMissing fails the verdict closed: evidence that binds an input the
// revision does not have is invalid (CACHE delta 4).
func (v *Verdict) MarkMissing(paths []string) {
	if len(paths) == 0 {
		return
	}
	v.Exit = ExitInvalid
	v.Reasons = append(v.Reasons, attest.ReasonInvalid)
	v.Warnings = append(v.Warnings, "input absent at the revision: "+strings.Join(paths, ", "))
}

// ParseRevision accepts only a full lower-case commit id; a prefix or a ref
// name is E628.
func ParseRevision(s string) (string, error) {
	if !fullRevision.MatchString(s) {
		return "", fail(CodeUsage, ExitFails, "%q is not a full commit sha (use $(git rev-parse HEAD))", s)
	}
	return s, nil
}

var fullRevision = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
