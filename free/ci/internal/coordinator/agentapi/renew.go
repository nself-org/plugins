package agentapi

// Purpose: POST /v1/agent/renew (D-1372): a node trades its current client certificate for the next one over
//          carrier B, so a pull node whose certificate nears its end renews without pairing again.
// Inputs:  one JSON body {"csr": <DER certificate request, base64>} on an mTLS connection that presented the
//          node's current certificate; no session is needed (the agent renews before it says hello).
// Outputs: 200 {node_id, cert, chain, root, serial, not_after} on success; a JSON refusal otherwise.
// Constraints: the body is capped at 16 KiB and decoded strictly; pki.CA.Renew decides (node kind, not expired,
//          not denied, not an ephemeral lineage) and issues a certificate for the same node id; the TLS layer
//          has already refused an expired or revoked certificate. The old certificate stays valid until its own
//          end, so a reconnect on either one works; the token gate moves the binding to the new serial on the
//          first heartbeat of the next session (transport/agent gate Reconcile), so a running job survives.

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/nself-org/plugins/free/ci/internal/pki"
)

// maxRenewBody caps a renewal request (a CSR is a few hundred bytes).
const maxRenewBody = 16 << 10

// RenewRequest is the body of POST /v1/agent/renew.
type RenewRequest struct {
	CSR []byte `json:"csr"` // DER certificate request for the next key (ECDSA P-256)
}

// RenewResponse is the reply to a successful renewal.
type RenewResponse struct {
	NodeID   string   `json:"node_id"`
	Cert     []byte   `json:"cert"`  // leaf DER
	Chain    [][]byte `json:"chain"` // leaf, node intermediate (DER)
	Root     []byte   `json:"root"`  // root DER
	Serial   string   `json:"serial"`
	NotAfter int64    `json:"not_after"` // unix seconds
}

func (s *Server) serveRenew(w http.ResponseWriter, r *http.Request) {
	q := s.begin(w, r, false)
	if q == nil {
		return
	}
	defer q.end()
	var req RenewRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRenewBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || len(req.CSR) == 0 || dec.More() {
		refuse(w, http.StatusBadRequest, pki.CodeInvalid, pki.ReasonBadCSR, "renewal request refused")
		return
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		refuse(w, http.StatusBadRequest, pki.CodeInvalid, pki.ReasonBadCSR, "renewal request refused")
		return
	}
	is, err := s.cfg.CA.Renew(q.ctx, r.TLS.PeerCertificates[0], req.CSR)
	if err != nil {
		reason := pki.ReasonOf(err)
		if reason == "" {
			s.cfg.Logf("agentapi: renewal for %s failed: %v", q.node, err)
			refuse(w, http.StatusServiceUnavailable, pki.CodeInvalid, "store", "renewal unavailable")
			return
		}
		status := http.StatusForbidden
		if reason == pki.ReasonBadCSR {
			status = http.StatusBadRequest
		}
		refuse(w, status, pki.CodeInvalid, reason, "certificate not renewed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(RenewResponse{NodeID: q.node, Cert: is.CertDER, Chain: is.Chain, Root: is.RootDER, Serial: is.Serial, NotAfter: is.NotAfter.Unix()})
}
