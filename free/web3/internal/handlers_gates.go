package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (h *Handlers) ListTokenGates(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, workspace_id, created_by, name, gate_type,
		        target_type, target_id, is_active, created_at, updated_at
		 FROM np_web3_token_gates WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []TokenGate
	for rows.Next() {
		var tg TokenGate
		if err := rows.Scan(&tg.ID, &tg.SourceAccountID, &tg.WorkspaceID, &tg.CreatedBy, &tg.Name,
			&tg.GateType, &tg.TargetType, &tg.TargetID, &tg.IsActive,
			&tg.CreatedAt, &tg.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, tg)
	}
	if items == nil {
		items = []TokenGate{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) CreateTokenGate(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sa := h.sa(r)
	var id string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_web3_token_gates (source_account_id, workspace_id, created_by, name, description,
		  gate_type, rules, target_type, target_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		sa, body["workspace_id"], body["created_by"], body["name"], body["description"],
		body["gate_type"], marshalJSON(body["rules"]), body["target_type"], body["target_id"]).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handlers) GetTokenGate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var tg TokenGate
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, workspace_id, created_by, name, gate_type,
		        target_type, target_id, is_active, created_at, updated_at
		 FROM np_web3_token_gates WHERE id=$1 AND source_account_id=$2`, id, sa).
		Scan(&tg.ID, &tg.SourceAccountID, &tg.WorkspaceID, &tg.CreatedBy, &tg.Name,
			&tg.GateType, &tg.TargetType, &tg.TargetID, &tg.IsActive,
			&tg.CreatedAt, &tg.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, tg)
}

func (h *Handlers) UpdateTokenGate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	_, err := h.pool.Exec(r.Context(),
		`UPDATE np_web3_token_gates SET name=$3, description=$4, is_active=$5, updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa, body["name"], body["description"], body["is_active"])
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("update: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (h *Handlers) DeleteTokenGate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	if _, err := h.pool.Exec(r.Context(),
		`DELETE FROM np_web3_token_gates WHERE id=$1 AND source_account_id=$2`, id, sa); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Gate Checks
// ============================================================
