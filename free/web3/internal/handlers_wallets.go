package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (h *Handlers) ListWallets(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, user_id, workspace_id, address, chain_id, chain_name,
		        wallet_type, ens_name, label, is_primary, is_active, created_at, updated_at
		 FROM np_web3_wallets WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []Wallet
	for rows.Next() {
		var wl Wallet
		if err := rows.Scan(&wl.ID, &wl.SourceAccountID, &wl.UserID, &wl.WorkspaceID,
			&wl.Address, &wl.ChainID, &wl.ChainName, &wl.WalletType, &wl.EnsName,
			&wl.Label, &wl.IsPrimary, &wl.IsActive, &wl.CreatedAt, &wl.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, wl)
	}
	if items == nil {
		items = []Wallet{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) CreateWallet(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sa := h.sa(r)
	var id string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_web3_wallets (source_account_id, user_id, workspace_id, address, chain_id, chain_name,
		  wallet_type, ens_name, ens_avatar, label, is_primary, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`,
		sa, body["user_id"], body["workspace_id"], body["address"], body["chain_id"], body["chain_name"],
		body["wallet_type"], body["ens_name"], body["ens_avatar"], body["label"],
		body["is_primary"], marshalJSON(body["metadata"])).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handlers) GetWallet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var wl Wallet
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, user_id, workspace_id, address, chain_id, chain_name,
		        wallet_type, ens_name, label, is_primary, is_active, created_at, updated_at
		 FROM np_web3_wallets WHERE id=$1 AND source_account_id=$2`, id, sa).
		Scan(&wl.ID, &wl.SourceAccountID, &wl.UserID, &wl.WorkspaceID,
			&wl.Address, &wl.ChainID, &wl.ChainName, &wl.WalletType, &wl.EnsName,
			&wl.Label, &wl.IsPrimary, &wl.IsActive, &wl.CreatedAt, &wl.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, wl)
}

func (h *Handlers) UpdateWallet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	_, err := h.pool.Exec(r.Context(),
		`UPDATE np_web3_wallets SET label=$3, ens_name=$4, ens_avatar=$5, is_primary=$6, is_active=$7, updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa, body["label"], body["ens_name"], body["ens_avatar"], body["is_primary"], body["is_active"])
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("update: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (h *Handlers) DeleteWallet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	if _, err := h.pool.Exec(r.Context(),
		`DELETE FROM np_web3_wallets WHERE id=$1 AND source_account_id=$2`, id, sa); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Collections
// ============================================================
