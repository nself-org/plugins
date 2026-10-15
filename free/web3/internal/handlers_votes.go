package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (h *Handlers) ListVotes(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, proposal_id, voter_address, support, voting_power, voted_at
		 FROM np_web3_votes WHERE source_account_id=$1 ORDER BY voted_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []Vote
	for rows.Next() {
		var v Vote
		if err := rows.Scan(&v.ID, &v.SourceAccountID, &v.ProposalID, &v.VoterAddress,
			&v.Support, &v.VotingPower, &v.VotedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, v)
	}
	if items == nil {
		items = []Vote{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) CreateVote(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sa := h.sa(r)
	var id string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_web3_votes (source_account_id, proposal_id, voter_address, voter_user_id, support, voting_power, reason)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		sa, body["proposal_id"], body["voter_address"], body["voter_user_id"],
		body["support"], body["voting_power"], body["reason"]).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handlers) GetVote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var v Vote
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, proposal_id, voter_address, support, voting_power, voted_at
		 FROM np_web3_votes WHERE id=$1 AND source_account_id=$2`, id, sa).
		Scan(&v.ID, &v.SourceAccountID, &v.ProposalID, &v.VoterAddress,
			&v.Support, &v.VotingPower, &v.VotedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// ============================================================
// Transactions
// ============================================================
