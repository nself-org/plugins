package internal

// Purpose: Gate stats and cache-invalidation handlers for np_web3_gate_checks.
// Inputs: source_account_id (header), gateId query param, invalidation body.
// Outputs: JSON stats aggregation or deletion count.
// Constraints: source_account_id isolation on every query; no cross-account leakage.
// SPORT: GET /api/v1/gate-stats, POST /api/v1/gate-invalidate

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// GateStats holds aggregated check statistics for one or all gates.
type GateStats struct {
	GateID        *string   `json:"gate_id,omitempty"`
	TotalChecks   int       `json:"total_checks"`
	PassedChecks  int       `json:"passed_checks"`
	FailedChecks  int       `json:"failed_checks"`
	PassRate      float64   `json:"pass_rate"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
}

// GetGateStats handles GET /api/v1/gate-stats
// Query params: gate_id (optional) — if absent, returns aggregate across all gates for the account.
func (h *Handlers) GetGateStats(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	gateID := r.URL.Query().Get("gate_id")

	if gateID != "" {
		var stats GateStats
		stats.GateID = &gateID
		err := h.pool.QueryRow(r.Context(),
			`SELECT
			    COUNT(*) AS total_checks,
			    COUNT(*) FILTER (WHERE passed) AS passed_checks,
			    COUNT(*) FILTER (WHERE NOT passed) AS failed_checks,
			    MAX(checked_at) AS last_checked_at
			 FROM np_web3_gate_checks
			 WHERE source_account_id=$1 AND gate_id=$2::uuid`,
			sa, gateID,
		).Scan(&stats.TotalChecks, &stats.PassedChecks, &stats.FailedChecks, &stats.LastCheckedAt)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
			return
		}
		if stats.TotalChecks > 0 {
			stats.PassRate = float64(stats.PassedChecks) / float64(stats.TotalChecks)
		}
		writeJSON(w, http.StatusOK, stats)
		return
	}

	// Aggregate across all gates for this account.
	var stats GateStats
	err := h.pool.QueryRow(r.Context(),
		`SELECT
		    COUNT(*) AS total_checks,
		    COUNT(*) FILTER (WHERE passed) AS passed_checks,
		    COUNT(*) FILTER (WHERE NOT passed) AS failed_checks,
		    MAX(checked_at) AS last_checked_at
		 FROM np_web3_gate_checks
		 WHERE source_account_id=$1`,
		sa,
	).Scan(&stats.TotalChecks, &stats.PassedChecks, &stats.FailedChecks, &stats.LastCheckedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	if stats.TotalChecks > 0 {
		stats.PassRate = float64(stats.PassedChecks) / float64(stats.TotalChecks)
	}
	writeJSON(w, http.StatusOK, stats)
}

// GateInvalidateRequest carries invalidation scope.
// Exactly one of GateID or WalletAddress must be set.
type GateInvalidateRequest struct {
	GateID        string `json:"gate_id"`
	WalletAddress string `json:"wallet_address"`
}

// InvalidateGate handles POST /api/v1/gate-invalidate
// Body: { "gate_id": "<uuid>" } or { "wallet_address": "<address>" }
// Deletes cached np_web3_gate_checks rows matching the scope.
func (h *Handlers) InvalidateGate(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	var req GateInvalidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	if req.GateID == "" && req.WalletAddress == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "one of gate_id or wallet_address is required"})
		return
	}

	var deleted int64

	if req.GateID != "" {
		tag, err := h.pool.Exec(r.Context(),
			`DELETE FROM np_web3_gate_checks
			 WHERE source_account_id=$1 AND gate_id=$2::uuid`,
			sa, req.GateID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
			return
		}
		deleted = tag.RowsAffected()
	} else {
		tag, err := h.pool.Exec(r.Context(),
			`DELETE FROM np_web3_gate_checks
			 WHERE source_account_id=$1 AND wallet_address=$2`,
			sa, req.WalletAddress)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
			return
		}
		deleted = tag.RowsAffected()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":         true,
		"deleted_entries": deleted,
	})
}
