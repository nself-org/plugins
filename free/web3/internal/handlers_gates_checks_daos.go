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

func (h *Handlers) ListDAOs(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, workspace_id, name, slug, chain_id, is_active, created_at, updated_at
		 FROM np_web3_daos WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []DAO
	for rows.Next() {
		var d DAO
		if err := rows.Scan(&d.ID, &d.SourceAccountID, &d.WorkspaceID, &d.Name, &d.Slug,
			&d.ChainID, &d.IsActive, &d.CreatedAt, &d.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, d)
	}
	if items == nil {
		items = []DAO{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) CreateDAO(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sa := h.sa(r)
	var id string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_web3_daos (source_account_id, workspace_id, name, slug, description, chain_id,
		  governance_token_address, treasury_address, snapshot_space, governor_address, governor_type)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		sa, body["workspace_id"], body["name"], body["slug"], body["description"], body["chain_id"],
		body["governance_token_address"], body["treasury_address"], body["snapshot_space"],
		body["governor_address"], body["governor_type"]).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handlers) GetDAO(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var d DAO
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, workspace_id, name, slug, chain_id, is_active, created_at, updated_at
		 FROM np_web3_daos WHERE id=$1 AND source_account_id=$2`, id, sa).
		Scan(&d.ID, &d.SourceAccountID, &d.WorkspaceID, &d.Name, &d.Slug,
			&d.ChainID, &d.IsActive, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *Handlers) UpdateDAO(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	_, err := h.pool.Exec(r.Context(),
		`UPDATE np_web3_daos SET name=$3, description=$4, is_active=$5, updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa, body["name"], body["description"], body["is_active"])
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("update: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (h *Handlers) DeleteDAO(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	if _, err := h.pool.Exec(r.Context(),
		`DELETE FROM np_web3_daos WHERE id=$1 AND source_account_id=$2`, id, sa); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
