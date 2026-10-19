package enroll

// Purpose: the proof of possession of the Ed25519 node key at enrollment (Opus review F1). The node key
//          is public by design (every hello carries it), so naming it proves nothing: the joining node
//          signs a transcript with the private half and the coordinator verifies it BEFORE the code is
//          consumed, for new and existing node names alike.
// Inputs:  the coordinator id (from the hello reply), the request's client time, the CSR key
//          fingerprint and the node key id.
// Outputs: the bytes to sign.
// Constraints: domain-separated (a signature made here is valid for nothing else), bound to this
//          coordinator, this CSR key and this request time (the 120 s skew window bounds replay, and a
//          replayed request still needs a live single-use code and an approver decision).

import "strconv"

// Domain is the signature domain tag.
const Domain = "nself-ci-enroll-v1"

// Transcript is the message the node key signs in a phase-enroll request.
func Transcript(coordinatorID string, clientTime int64, csrFingerprint, nodeKeyID string) []byte {
	return []byte(Domain + "\n" + coordinatorID + "\n" + strconv.FormatInt(clientTime, 10) + "\n" + csrFingerprint + "\n" + nodeKeyID)
}
