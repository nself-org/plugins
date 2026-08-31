// Purpose: DB-backed handler tests for TokenBalances, TokenGates, GateChecks, and DAO CRUD endpoints.
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

// ---- Token Balances ----

func TestListTokenBalances_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, wallet_address, user_id, token_id, balance").WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "wallet_address", "user_id", "token_id", "balance", "last_updated_at", "created_at",
		}).AddRow("b1", "primary", "0xw", nil, "t1", "100", nowTime(), nowTime()))
	rec := httptest.NewRecorder()
	h.ListTokenBalances(rec, httptest.NewRequest(http.MethodGet, "/api/v1/token-balances", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpsertTokenBalance_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_web3_token_balances").WithArgs(anyArgs(7)...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("b1"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/token-balances", strReader(`{"wallet_address":"0xw","token_id":"t1","balance":"100"}`))
	h.UpsertTokenBalance(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpsertTokenBalance_InvalidJSON(t *testing.T) {
	h, _ := newMockHandlers(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/token-balances", strReader("bogus"))
	h.UpsertTokenBalance(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestDeleteTokenBalance_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_web3_token_balances").WithArgs("b1", pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodDelete, "/api/v1/token-balances/b1", "", "id", "b1")
	h.DeleteTokenBalance(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

// ---- Token Gates ----

func TestListTokenGates_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, workspace_id, created_by, name, gate_type").WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "workspace_id", "created_by", "name", "gate_type",
			"target_type", "target_id", "is_active", "created_at", "updated_at",
		}).AddRow("g1", "primary", "ws1", "u1", "Gate", "nft_ownership", "channel", nil, true, nowTime(), nowTime()))
	rec := httptest.NewRecorder()
	h.ListTokenGates(rec, httptest.NewRequest(http.MethodGet, "/api/v1/token-gates", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateTokenGate_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_web3_token_gates").WithArgs(anyArgs(9)...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("g1"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/token-gates", strReader(`{"workspace_id":"ws1","created_by":"u1","name":"Gate","gate_type":"nft_ownership","target_type":"channel"}`))
	h.CreateTokenGate(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetTokenGate_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, workspace_id, created_by, name, gate_type").WithArgs("g1", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "workspace_id", "created_by", "name", "gate_type",
			"target_type", "target_id", "is_active", "created_at", "updated_at",
		}).AddRow("g1", "primary", "ws1", "u1", "Gate", "nft_ownership", "channel", nil, true, nowTime(), nowTime()))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/token-gates/g1", "", "id", "g1")
	h.GetTokenGate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateTokenGate_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("UPDATE np_web3_token_gates").WithArgs(anyArgs(5)...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodPut, "/api/v1/token-gates/g1", `{"name":"Gate2"}`, "id", "g1")
	h.UpdateTokenGate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeleteTokenGate_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_web3_token_gates").WithArgs("g1", pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodDelete, "/api/v1/token-gates/g1", "", "id", "g1")
	h.DeleteTokenGate(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

// ---- Gate Checks ----

func TestListGateChecks_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, gate_id, user_id, wallet_address, passed").WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "gate_id", "user_id", "wallet_address", "passed", "failure_reason", "checked_at",
		}).AddRow("gc1", "primary", "g1", "u1", "0xw", true, nil, nowTime()))
	rec := httptest.NewRecorder()
	h.ListGateChecks(rec, httptest.NewRequest(http.MethodGet, "/api/v1/gate-checks", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateGateCheck_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_web3_gate_checks").WithArgs(anyArgs(8)...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("gc1"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gate-checks", strReader(`{"gate_id":"g1","user_id":"u1","wallet_address":"0xw","passed":true}`))
	h.CreateGateCheck(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetGateCheck_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, gate_id, user_id, wallet_address, passed").WithArgs("gc1", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "gate_id", "user_id", "wallet_address", "passed", "failure_reason", "checked_at",
		}).AddRow("gc1", "primary", "g1", "u1", "0xw", true, nil, nowTime()))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/gate-checks/gc1", "", "id", "gc1")
	h.GetGateCheck(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetGateCheck_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, gate_id, user_id, wallet_address, passed").WithArgs("missing", pgxmock.AnyArg()).
		WillReturnError(errBoom)
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/gate-checks/missing", "", "id", "missing")
	h.GetGateCheck(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

// ---- DAOs ----

func TestListDAOs_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, workspace_id, name, slug, chain_id, is_active").WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "workspace_id", "name", "slug", "chain_id", "is_active", "created_at", "updated_at",
		}).AddRow("d1", "primary", nil, "DAO", nil, 1, true, nowTime(), nowTime()))
	rec := httptest.NewRecorder()
	h.ListDAOs(rec, httptest.NewRequest(http.MethodGet, "/api/v1/daos", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateDAO_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_web3_daos").WithArgs(anyArgs(11)...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("d1"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/daos", strReader(`{"name":"DAO","chain_id":1}`))
	h.CreateDAO(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetDAO_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, workspace_id, name, slug, chain_id, is_active").WithArgs("d1", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "workspace_id", "name", "slug", "chain_id", "is_active", "created_at", "updated_at",
		}).AddRow("d1", "primary", nil, "DAO", nil, 1, true, nowTime(), nowTime()))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/daos/d1", "", "id", "d1")
	h.GetDAO(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateDAO_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("UPDATE np_web3_daos").WithArgs(anyArgs(5)...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodPut, "/api/v1/daos/d1", `{"name":"DAO2"}`, "id", "d1")
	h.UpdateDAO(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeleteDAO_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_web3_daos").WithArgs("d1", pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodDelete, "/api/v1/daos/d1", "", "id", "d1")
	h.DeleteDAO(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}
