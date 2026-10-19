package enroll

// Purpose: the proof of possession of the Ed25519 node key at enrollment (Opus review F1). The node key
//          is public by design (every hello carries it), so naming it proves nothing: the joining node
//          signs a transcript with the private half and the coordinator verifies it BEFORE the code is
//          consumed, for new and existing node names alike.
// Inputs:  the coordinator id (from the hello reply), the fingerprint of the root the node confirmed, the
//          node name, the request's client time, the CSR key fingerprint and the node key id.
// Outputs: the bytes to sign.
// Constraints: domain-separated (a signature made here is valid for nothing else), bound to this
//          coordinator and its root (the default coordinator id is the constant "local", so the id alone
//          does not name one), the node name, this CSR key and this request time (the 120 s skew window
//          bounds replay, and a replayed request still needs a live single-use code and an approver
//          decision). Every field is length-prefixed: no field can carry another's separator.

import (
	"strconv"
	"strings"
)

// Domain is the signature domain tag.
const Domain = "nself-ci-enroll-v1"

// Transcript is the message the node key signs in a phase-enroll request. The node name is folded to
// lower case, as the coordinator does.
func Transcript(coordinatorID, rootFingerprint, nodeName string, clientTime int64, csrFingerprint, nodeKeyID string) []byte {
	var b strings.Builder
	for _, f := range []string{Domain, coordinatorID, rootFingerprint, strings.ToLower(nodeName), strconv.FormatInt(clientTime, 10), csrFingerprint, nodeKeyID} {
		b.WriteString(strconv.Itoa(len(f)))
		b.WriteByte(':')
		b.WriteString(f)
	}
	return []byte(b.String())
}
