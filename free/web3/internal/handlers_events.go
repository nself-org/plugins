package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (h *Handlers) ListEvents(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, event_name, contract_address, chain_id,
		        transaction_hash, block_number, created_at
		 FROM np_web3_events WHERE source_account_id=$1 ORDER BY block_number DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []Web3Event
	for rows.Next() {
		var ev Web3Event
		if err := rows.Scan(&ev.ID, &ev.SourceAccountID, &ev.EventName, &ev.ContractAddress, &ev.ChainID,
			&ev.TransactionHash, &ev.BlockNumber, &ev.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, ev)
	}
	if items == nil {
		items = []Web3Event{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) CreateEvent(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sa := h.sa(r)
	var id string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_web3_events (source_account_id, event_name, contract_address, chain_id,
		  transaction_hash, log_index, block_number, event_data, related_user_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		sa, body["event_name"], body["contract_address"], body["chain_id"],
		body["transaction_hash"], body["log_index"], body["block_number"],
		marshalJSON(body["event_data"]), body["related_user_id"]).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handlers) GetEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var ev Web3Event
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, event_name, contract_address, chain_id,
		        transaction_hash, block_number, created_at
		 FROM np_web3_events WHERE id=$1 AND source_account_id=$2`, id, sa).
		Scan(&ev.ID, &ev.SourceAccountID, &ev.EventName, &ev.ContractAddress, &ev.ChainID,
			&ev.TransactionHash, &ev.BlockNumber, &ev.CreatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// ============================================================
// Helper
// ============================================================

func marshalJSON(v any) []byte {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
