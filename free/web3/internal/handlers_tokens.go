package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (h *Handlers) ListTokens(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, contract_address, chain_id, name, symbol, decimals, token_type,
		        is_verified, is_spam, created_at, updated_at
		 FROM np_web3_tokens WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []Token
	for rows.Next() {
		var t Token
		if err := rows.Scan(&t.ID, &t.SourceAccountID, &t.ContractAddress, &t.ChainID, &t.Name,
			&t.Symbol, &t.Decimals, &t.TokenType, &t.IsVerified, &t.IsSpam,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, t)
	}
	if items == nil {
		items = []Token{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) CreateToken(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sa := h.sa(r)
	var id string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_web3_tokens (source_account_id, contract_address, chain_id, name, symbol, decimals, token_type, logo_url, description)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		sa, body["contract_address"], body["chain_id"], body["name"], body["symbol"],
		body["decimals"], body["token_type"], body["logo_url"], body["description"]).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handlers) GetToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var t Token
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, contract_address, chain_id, name, symbol, decimals, token_type,
		        is_verified, is_spam, created_at, updated_at
		 FROM np_web3_tokens WHERE id=$1 AND source_account_id=$2`, id, sa).
		Scan(&t.ID, &t.SourceAccountID, &t.ContractAddress, &t.ChainID, &t.Name,
			&t.Symbol, &t.Decimals, &t.TokenType, &t.IsVerified, &t.IsSpam,
			&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *Handlers) UpdateToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	_, err := h.pool.Exec(r.Context(),
		`UPDATE np_web3_tokens SET name=$3, symbol=$4, logo_url=$5, is_verified=$6, updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa, body["name"], body["symbol"], body["logo_url"], body["is_verified"])
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("update: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (h *Handlers) DeleteToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	if _, err := h.pool.Exec(r.Context(),
		`DELETE FROM np_web3_tokens WHERE id=$1 AND source_account_id=$2`, id, sa); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Token Balances
// ============================================================
