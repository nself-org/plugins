package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (h *Handlers) ListTokenBalances(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, wallet_address, user_id, token_id, balance, last_updated_at, created_at
		 FROM np_web3_token_balances WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []TokenBalance
	for rows.Next() {
		var tb TokenBalance
		if err := rows.Scan(&tb.ID, &tb.SourceAccountID, &tb.WalletAddress, &tb.UserID,
			&tb.TokenID, &tb.Balance, &tb.LastUpdatedAt, &tb.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, tb)
	}
	if items == nil {
		items = []TokenBalance{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) UpsertTokenBalance(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sa := h.sa(r)
	var id string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_web3_token_balances (source_account_id, wallet_address, user_id, token_id, balance, balance_formatted, value_usd)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (source_account_id, wallet_address, token_id) DO UPDATE
		   SET balance=EXCLUDED.balance, balance_formatted=EXCLUDED.balance_formatted,
		       value_usd=EXCLUDED.value_usd, last_updated_at=NOW()
		 RETURNING id`,
		sa, body["wallet_address"], body["user_id"], body["token_id"], body["balance"],
		body["balance_formatted"], body["value_usd"]).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("upsert: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (h *Handlers) DeleteTokenBalance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	if _, err := h.pool.Exec(r.Context(),
		`DELETE FROM np_web3_token_balances WHERE id=$1 AND source_account_id=$2`, id, sa); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Token Gates
// ============================================================
