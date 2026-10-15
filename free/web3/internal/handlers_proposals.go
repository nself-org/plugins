package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (h *Handlers) ListProposals(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, dao_id, title, status, proposer_address,
		        votes_for, votes_against, votes_abstain, created_at, updated_at
		 FROM np_web3_proposals WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []Proposal
	for rows.Next() {
		var p Proposal
		if err := rows.Scan(&p.ID, &p.SourceAccountID, &p.DaoID, &p.Title, &p.Status, &p.ProposerAddress,
			&p.VotesFor, &p.VotesAgainst, &p.VotesAbstain, &p.CreatedAt, &p.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, p)
	}
	if items == nil {
		items = []Proposal{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) CreateProposal(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sa := h.sa(r)
	var id string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_web3_proposals (source_account_id, dao_id, title, description, proposer_address,
		  proposer_user_id, status)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		sa, body["dao_id"], body["title"], body["description"], body["proposer_address"],
		body["proposer_user_id"], body["status"]).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handlers) GetProposal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var p Proposal
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, dao_id, title, status, proposer_address,
		        votes_for, votes_against, votes_abstain, created_at, updated_at
		 FROM np_web3_proposals WHERE id=$1 AND source_account_id=$2`, id, sa).
		Scan(&p.ID, &p.SourceAccountID, &p.DaoID, &p.Title, &p.Status, &p.ProposerAddress,
			&p.VotesFor, &p.VotesAgainst, &p.VotesAbstain, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handlers) UpdateProposal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	_, err := h.pool.Exec(r.Context(),
		`UPDATE np_web3_proposals SET status=$3, votes_for=$4, votes_against=$5, votes_abstain=$6, updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa, body["status"], body["votes_for"], body["votes_against"], body["votes_abstain"])
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("update: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (h *Handlers) DeleteProposal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	if _, err := h.pool.Exec(r.Context(),
		`DELETE FROM np_web3_proposals WHERE id=$1 AND source_account_id=$2`, id, sa); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Votes
// ============================================================
