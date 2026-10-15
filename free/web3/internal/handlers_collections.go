package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (h *Handlers) ListCollections(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, contract_address, chain_id, name, slug, token_standard,
		        is_verified, is_spam, is_managed, created_at, updated_at
		 FROM np_web3_collections WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []Collection
	for rows.Next() {
		var c Collection
		if err := rows.Scan(&c.ID, &c.SourceAccountID, &c.ContractAddress, &c.ChainID, &c.Name,
			&c.Slug, &c.TokenStandard, &c.IsVerified, &c.IsSpam, &c.IsManaged,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, c)
	}
	if items == nil {
		items = []Collection{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) CreateCollection(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sa := h.sa(r)
	var id string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_web3_collections (source_account_id, contract_address, chain_id, name, slug,
		  description, image_url, token_standard, workspace_id, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		sa, body["contract_address"], body["chain_id"], body["name"], body["slug"],
		body["description"], body["image_url"], body["token_standard"],
		body["workspace_id"], marshalJSON(body["metadata"])).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handlers) GetCollection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var c Collection
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, contract_address, chain_id, name, slug, token_standard,
		        is_verified, is_spam, is_managed, created_at, updated_at
		 FROM np_web3_collections WHERE id=$1 AND source_account_id=$2`, id, sa).
		Scan(&c.ID, &c.SourceAccountID, &c.ContractAddress, &c.ChainID, &c.Name,
			&c.Slug, &c.TokenStandard, &c.IsVerified, &c.IsSpam, &c.IsManaged,
			&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handlers) UpdateCollection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	_, err := h.pool.Exec(r.Context(),
		`UPDATE np_web3_collections SET name=$3, description=$4, image_url=$5, is_verified=$6, updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa, body["name"], body["description"], body["image_url"], body["is_verified"])
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("update: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (h *Handlers) DeleteCollection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	if _, err := h.pool.Exec(r.Context(),
		`DELETE FROM np_web3_collections WHERE id=$1 AND source_account_id=$2`, id, sa); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// NFTs
// ============================================================
