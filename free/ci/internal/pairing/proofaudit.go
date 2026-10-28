package pairing

// Purpose: make a failed node_key_proof visible (D-1258). A request that is not signed by the node key
//          it names is takeover probing, yet it returns before the code is claimed; it gets a rate-limited
//          audit row and alert instead of silence.
// Inputs:  the node name, source address and CSR fingerprint of the failed request.
// Outputs: at most proofAuditBurst `pairing.refuse` rows (reason node_key_proof) and `pairing.proof_failed`
//          alerts per proofAuditWindow; the count of suppressed failures rides on the next row.
// Constraints: the budget is global, so a spray of requests cannot grow the audit log; no code and no
//          key bytes are written; the clock is the CA's.

import (
	"context"
	"fmt"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pki"
)

const (
	proofAuditWindow = time.Minute
	proofAuditBurst  = 10
)

// proofAuditor is the fixed-window budget of the proof-failure rows. Its zero value is ready.
type proofAuditor struct {
	start      time.Time
	used       int
	suppressed int
}

// take reports whether a row may be written now and how many failures were dropped since the last one.
func (p *proofAuditor) take(now time.Time) (ok bool, dropped int) {
	if now.Sub(p.start) >= proofAuditWindow || now.Before(p.start) {
		p.start, p.used = now, 0
	}
	if p.used >= proofAuditBurst {
		p.suppressed++
		return false, 0
	}
	p.used++
	dropped, p.suppressed = p.suppressed, 0
	return true, dropped
}

// auditProofFailure records a failed node_key_proof, within the budget.
func (s *Service) auditProofFailure(ctx context.Context, name, src, csrFP string) {
	s.mu.Lock()
	ok, dropped := s.proofs.take(s.cfg.CA.Now())
	s.mu.Unlock()
	if !ok {
		return
	}
	_ = s.audit(ctx, "pairing.refuse", name, fmt.Sprintf("node=%s csr=%s src=%s reason=%s suppressed=%d", name, csrFP, cut(src, 64), pki.ReasonNodeKeyProof, dropped))
	s.alert(Alert{Kind: "pairing.proof_failed", NodeID: name})
}
