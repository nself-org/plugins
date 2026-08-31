// Purpose: DB-backed handler tests for Proposals, Votes, Transactions, Events,
// and gate-stats/gate-invalidate endpoints.
// Inputs: httptest requests routed through Handlers methods with a mocked pgx pool.
// Outputs: asserts HTTP status codes, JSON response bodies, and SQL expectations met.
// Constraints: No real Postgres; pgxmock.PgxPoolIface satisfies internal.PgxIface.
package internal

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pashagolub/pgxmock/v3"
)

// ---- Proposals ----

func TestListProposals_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, dao_id, title, status, proposer_address").WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "dao_id", "title", "status", "proposer_address",
			"votes_for", "votes_against", "votes_abstain", "created_at", "updated_at",
		}).AddRow("p1", "primary", "d1", "Prop", "active", "0xp", "0", "0", "0", nowTime(), nowTime()))
	rec := httptest.NewRecorder()
	h.ListProposals(rec, httptest.NewRequest(http.MethodGet, "/api/v1/proposals", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateProposal_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_web3_proposals").WithArgs(anyArgs(7)...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("p1"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/proposals", strReader(`{"dao_id":"d1","title":"Prop","proposer_address":"0xp"}`))
	h.CreateProposal(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetProposal_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, dao_id, title, status, proposer_address").WithArgs("p1", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "dao_id", "title", "status", "proposer_address",
			"votes_for", "votes_against", "votes_abstain", "created_at", "updated_at",
		}).AddRow("p1", "primary", "d1", "Prop", "active", "0xp", "0", "0", "0", nowTime(), nowTime()))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/proposals/p1", "", "id", "p1")
	h.GetProposal(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateProposal_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("UPDATE np_web3_proposals").WithArgs(anyArgs(6)...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodPut, "/api/v1/proposals/p1", `{"status":"succeeded"}`, "id", "p1")
	h.UpdateProposal(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeleteProposal_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_web3_proposals").WithArgs("p1", pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodDelete, "/api/v1/proposals/p1", "", "id", "p1")
	h.DeleteProposal(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

// ---- Votes ----

func TestListVotes_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, proposal_id, voter_address, support").WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "proposal_id", "voter_address", "support", "voting_power", "voted_at",
		}).AddRow("v1", "primary", "p1", "0xv", "for", "100", nowTime()))
	rec := httptest.NewRecorder()
	h.ListVotes(rec, httptest.NewRequest(http.MethodGet, "/api/v1/votes", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateVote_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_web3_votes").WithArgs(anyArgs(7)...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("v1"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/votes", strReader(`{"proposal_id":"p1","voter_address":"0xv","support":"for","voting_power":"100"}`))
	h.CreateVote(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetVote_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, proposal_id, voter_address, support").WithArgs("v1", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "proposal_id", "voter_address", "support", "voting_power", "voted_at",
		}).AddRow("v1", "primary", "p1", "0xv", "for", "100", nowTime()))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/votes/v1", "", "id", "v1")
	h.GetVote(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetVote_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, proposal_id, voter_address, support").WithArgs("missing", pgxmock.AnyArg()).
		WillReturnError(errBoom)
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/votes/missing", "", "id", "missing")
	h.GetVote(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

// ---- Transactions ----

func TestListTransactions_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, transaction_hash, chain_id, from_address").WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "transaction_hash", "chain_id", "from_address", "to_address",
			"value", "status", "transaction_type", "created_at",
		}).AddRow("tx1", "primary", "0xhash", 1, "0xfrom", nil, "1000", nil, nil, nowTime()))
	rec := httptest.NewRecorder()
	h.ListTransactions(rec, httptest.NewRequest(http.MethodGet, "/api/v1/transactions", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateTransaction_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_web3_transactions").WithArgs(anyArgs(10)...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("tx1"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", strReader(`{"transaction_hash":"0xhash","chain_id":1,"from_address":"0xfrom","value":"1000"}`))
	h.CreateTransaction(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetTransaction_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, transaction_hash, chain_id, from_address").WithArgs("tx1", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "transaction_hash", "chain_id", "from_address", "to_address",
			"value", "status", "transaction_type", "created_at",
		}).AddRow("tx1", "primary", "0xhash", 1, "0xfrom", nil, "1000", nil, nil, nowTime()))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/transactions/tx1", "", "id", "tx1")
	h.GetTransaction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

// ---- Events ----

func TestListEvents_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, event_name, contract_address, chain_id").WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "event_name", "contract_address", "chain_id",
			"transaction_hash", "block_number", "created_at",
		}).AddRow("e1", "primary", "Transfer", "0xc", 1, "0xhash", int64(100), nowTime()))
	rec := httptest.NewRecorder()
	h.ListEvents(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateEvent_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_web3_events").WithArgs(anyArgs(9)...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("e1"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strReader(`{"event_name":"Transfer","contract_address":"0xc","chain_id":1,"transaction_hash":"0xhash","block_number":100}`))
	h.CreateEvent(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetEvent_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, event_name, contract_address, chain_id").WithArgs("e1", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "event_name", "contract_address", "chain_id",
			"transaction_hash", "block_number", "created_at",
		}).AddRow("e1", "primary", "Transfer", "0xc", 1, "0xhash", int64(100), nowTime()))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/events/e1", "", "id", "e1")
	h.GetEvent(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetEvent_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, event_name, contract_address, chain_id").WithArgs("missing", pgxmock.AnyArg()).
		WillReturnError(errBoom)
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/events/missing", "", "id", "missing")
	h.GetEvent(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

// ---- Gate Stats / Invalidate ----

func TestGetGateStats_ByGateID(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT").WithArgs(pgxmock.AnyArg(), "g1").
		WillReturnRows(pgxmock.NewRows([]string{"total_checks", "passed_checks", "failed_checks", "last_checked_at"}).
			AddRow(10, 8, 2, nowTime()))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gate-stats?gate_id=g1", nil)
	h.GetGateStats(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetGateStats_ByGateID_Error(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT").WithArgs(pgxmock.AnyArg(), "g1").WillReturnError(errBoom)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gate-stats?gate_id=g1", nil)
	h.GetGateStats(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestGetGateStats_Aggregate(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT").WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"total_checks", "passed_checks", "failed_checks", "last_checked_at"}).
			AddRow(0, 0, 0, nil))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gate-stats", nil)
	h.GetGateStats(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetGateStats_Aggregate_Error(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT").WithArgs(pgxmock.AnyArg()).WillReturnError(errBoom)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gate-stats", nil)
	h.GetGateStats(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestInvalidateGate_ByGateID(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_web3_gate_checks").WithArgs(pgxmock.AnyArg(), "g1").
		WillReturnResult(pgxmock.NewResult("DELETE", 3))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gate-invalidate", strReader(`{"gate_id":"g1"}`))
	h.InvalidateGate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestInvalidateGate_ByWalletAddress(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_web3_gate_checks").WithArgs(pgxmock.AnyArg(), "0xw").
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gate-invalidate", strReader(`{"wallet_address":"0xw"}`))
	h.InvalidateGate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestInvalidateGate_MissingScope(t *testing.T) {
	h, _ := newMockHandlers(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gate-invalidate", strReader(`{}`))
	h.InvalidateGate(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestInvalidateGate_InvalidJSON(t *testing.T) {
	h, _ := newMockHandlers(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gate-invalidate", strReader("bogus"))
	h.InvalidateGate(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestInvalidateGate_DeleteError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_web3_gate_checks").WithArgs(pgxmock.AnyArg(), "g1").
		WillReturnError(errBoom)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gate-invalidate", strReader(`{"gate_id":"g1"}`))
	h.InvalidateGate(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}
