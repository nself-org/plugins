package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (h *Handlers) ListGateChecks(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, gate_id, user_id, wallet_address, passed, failure_reason, checked_at
		 FROM np_web3_gate_checks WHERE source_account_id=$1 ORDER BY checked_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []GateCheck
	for rows.Next() {
		var gc GateCheck
		if err := rows.Scan(&gc.ID, &gc.SourceAccountID, &gc.GateID, &gc.UserID, &gc.WalletAddress,
			&gc.Passed, &gc.FailureReason, &gc.CheckedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, gc)
	}
	if items == nil {
		items = []GateCheck{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) CreateGateCheck(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sa := h.sa(r)
	var id string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_web3_gate_checks (source_account_id, gate_id, user_id, wallet_address, passed, failure_reason, evidence, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		sa, body["gate_id"], body["user_id"], body["wallet_address"], body["passed"],
		body["failure_reason"], marshalJSON(body["evidence"]), body["expires_at"]).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handlers) GetGateCheck(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var gc GateCheck
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, gate_id, user_id, wallet_address, passed, failure_reason, checked_at
		 FROM np_web3_gate_checks WHERE id=$1 AND source_account_id=$2`, id, sa).
		Scan(&gc.ID, &gc.SourceAccountID, &gc.GateID, &gc.UserID, &gc.WalletAddress,
			&gc.Passed, &gc.FailureReason, &gc.CheckedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, gc)
}

// ============================================================
// DAOs
// ============================================================
