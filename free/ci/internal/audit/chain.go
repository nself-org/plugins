package audit

// Purpose: the hash chain over the core audit table (Epic D20): head = sha256(prev_head || canonical row),
// where the canonical row is trust.Canonical over seq, ts, actor, action, target and detail.
// Inputs:  the previous head (nil for the first row) and the row.
// Outputs: the new head as a store.AuditChain; the signature column stays empty (TRUST-20 signs checkpoints).
// Constraints: pure and deterministic; a genesis row hashes with an empty prev; the chain detects an edit
//          that was not recomputed, not a writer who recomputes every later row (the TEAM checkpoint does).

import (
	"crypto/sha256"

	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// Hash returns the head of row chained to prev.
func Hash(prev []byte, row store.AuditRow) ([]byte, error) {
	doc, err := trust.Canonical(map[string]any{
		"seq": row.Seq, "ts": row.TS, "actor": row.Actor, "action": row.Action, "target": row.Target, "detail": row.Detail,
	})
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	h.Write(prev)
	h.Write(doc)
	return h.Sum(nil), nil
}

// Chain is the store.AuditSealer of this package.
func Chain(prev []byte, row store.AuditRow) (store.AuditChain, error) {
	head, err := Hash(prev, row)
	return store.AuditChain{Head: head}, err
}
