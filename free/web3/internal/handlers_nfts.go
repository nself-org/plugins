package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (h *Handlers) ListNFTs(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, contract_address, token_id, chain_id, token_standard,
		        owner_address, quantity, name, is_verified, is_spam, created_at, updated_at
		 FROM np_web3_nfts WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []NFT
	for rows.Next() {
		var n NFT
		if err := rows.Scan(&n.ID, &n.SourceAccountID, &n.ContractAddress, &n.TokenID, &n.ChainID,
			&n.TokenStandard, &n.OwnerAddress, &n.Quantity, &n.Name,
			&n.IsVerified, &n.IsSpam, &n.CreatedAt, &n.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, n)
	}
	if items == nil {
		items = []NFT{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) CreateNFT(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sa := h.sa(r)
	var id string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_web3_nfts (source_account_id, contract_address, token_id, chain_id, token_standard,
		  owner_address, owner_user_id, quantity, name, description, image_url, metadata_uri, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`,
		sa, body["contract_address"], body["token_id"], body["chain_id"], body["token_standard"],
		body["owner_address"], body["owner_user_id"], body["quantity"], body["name"],
		body["description"], body["image_url"], body["metadata_uri"],
		marshalJSON(body["metadata"])).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handlers) GetNFT(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var n NFT
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, contract_address, token_id, chain_id, token_standard,
		        owner_address, quantity, name, is_verified, is_spam, created_at, updated_at
		 FROM np_web3_nfts WHERE id=$1 AND source_account_id=$2`, id, sa).
		Scan(&n.ID, &n.SourceAccountID, &n.ContractAddress, &n.TokenID, &n.ChainID,
			&n.TokenStandard, &n.OwnerAddress, &n.Quantity, &n.Name,
			&n.IsVerified, &n.IsSpam, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (h *Handlers) UpdateNFT(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	_, err := h.pool.Exec(r.Context(),
		`UPDATE np_web3_nfts SET owner_address=$3, name=$4, description=$5, is_verified=$6, updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa, body["owner_address"], body["name"], body["description"], body["is_verified"])
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("update: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (h *Handlers) DeleteNFT(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	if _, err := h.pool.Exec(r.Context(),
		`DELETE FROM np_web3_nfts WHERE id=$1 AND source_account_id=$2`, id, sa); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Tokens
// ============================================================
