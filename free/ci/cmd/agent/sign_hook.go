package main

// Purpose: install the evidence signer of the agent session (P7-TRUST-20, Epic D13): a result frame's
// evidence is signed with the node key over attest's payload ("nself-ci-evidence/v1\n" + canonical JSON
// without signature and freshness, producer kind signed-remote), under the key id ci-node-<16 hex>. The
// coordinator turns the frame into signed-remote evidence and verifies it with attest.RunnerVerifier.
// Inputs:  the agent state dir: the node key (node.key) and the marker file `signing-eligible`.
// Outputs: session.SetResultSigner(...) when the node is signing eligible; nothing otherwise.
// Constraints: registered as a PreStart hook so serve.go stays as it is; only `serve` signs. The marker
//          is created by the operator or by provisioning (P7-NODE-06) on a node whose jobs run under a
//          separate UID, user namespace or container, so a job cannot read the node key; it must be a
//          regular file, not a symlink, with no group or other write bit. Without it results carry no
//          signature. The coordinator's registry record (signing_eligible) decides regardless.

import (
	"context"
	"os"
	"os/user"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/hygiene"
)

// signingMarker is the file that makes a node sign its results.
const signingMarker = "signing-eligible"

// setResultSigner is session.SetResultSigner; tests replace it.
var setResultSigner = session.SetResultSigner

func init() { must(RegisterPreStart("sign-evidence", signPreStart)) }

// evidenceSigner signs result evidence with the node key.
type evidenceSigner struct{ key nodekey.Signer }

// KeyID is the evidence key id.
func (s evidenceSigner) KeyID() string { return attest.EvidenceKeyID(s.key.KeyID()) }

// Sign signs the evidence bytes of a result frame.
func (s evidenceSigner) Sign(evidence []byte) ([]byte, error) {
	return attest.SignDocumentRaw(evidence, s.key)
}

// signingEligible reports whether the state dir carries a usable marker.
func signingEligible(state string) bool {
	info, err := os.Lstat(filepath.Join(state, signingMarker))
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o022 == 0
}

// newEvidenceSigner returns the signer of a signing-eligible state dir; ok is false without the marker.
func newEvidenceSigner(state string) (s evidenceSigner, ok bool, err error) {
	if !signingEligible(state) {
		return s, false, nil
	}
	key, err := nodekey.LoadOrCreate(state)
	if err != nil {
		return s, false, err
	}
	return evidenceSigner{key: key}, true, nil
}

// lookupJobUser resolves the job user of slot 0 (tests replace it).
var lookupJobUser = func(name string) (uid int, err error) {
	u, err := user.Lookup(name)
	if err != nil {
		return 0, err
	}
	var n int
	for _, c := range u.Uid {
		if c < '0' || c > '9' {
			return 0, os.ErrInvalid
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// uidSeparationProven reports whether this agent is the system agent user, its jobs run as another
// existing user through the job shim, and the state dir and node key are private to the agent's own uid
// (owner is the agent, no group or other access), so a job cannot read the key. Any doubt is false.
func uidSeparationProven(state string) bool {
	if systemShim(0) == nil || shimUserName() == "" {
		return false
	}
	self := os.Geteuid()
	if self <= 0 { // root is no separation proof
		return false
	}
	job, err := lookupJobUser(hygiene.JobUserName(0))
	if err != nil || job <= 0 || job == self {
		return false
	}
	for _, p := range []string{state, filepath.Join(state, "node.key")} {
		info, err := os.Lstat(p)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
			return false
		}
		if !ownedBySelf(info) {
			return false
		}
	}
	return true
}

// provisionMarker writes the signing-eligible marker (0600, create-exclusive, O_EXCL refuses an existing symlink)
// when uid separation is proven and the marker is absent. The node key is created first, so the
// separation check covers the key the signer will use.
func provisionMarker(state string) error {
	if _, err := os.Lstat(filepath.Join(state, signingMarker)); err == nil {
		return nil
	}
	if systemShim(0) == nil { // not the system agent user: no separation to prove, and no key is made for it
		return nil
	}
	if _, err := nodekey.LoadOrCreate(state); err != nil {
		return err
	}
	if !uidSeparationProven(state) {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(state, signingMarker), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	return f.Close()
}

func signPreStart(_ context.Context, command string) error {
	if command != "serve" {
		return nil
	}
	state, err := stateDir()
	if err != nil {
		return err
	}
	if err = provisionMarker(state); err != nil {
		return err
	}
	s, ok, err := newEvidenceSigner(state)
	if err != nil {
		return err
	}
	if ok {
		setResultSigner(s)
	}
	return nil
}
