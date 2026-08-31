// Purpose: DB-backed handler tests for wallet and collection CRUD endpoints.
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

// ---- Wallets ----

func TestListWallets_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id").WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "user_id", "workspace_id", "address", "chain_id", "chain_name",
			"wallet_type", "ens_name", "label", "is_primary", "is_active", "created_at", "updated_at",
		}).AddRow("w1", "primary", "u1", nil, "0xabc", 1, "ethereum", nil, nil, nil, true, true, nowTime(), nowTime()))

	rec := httptest.NewRecorder()
	h.ListWallets(rec, httptest.NewRequest(http.MethodGet, "/api/v1/wallets", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestListWallets_QueryError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id").WithArgs(pgxmock.AnyArg()).
		WillReturnError(errBoom)

	rec := httptest.NewRecorder()
	h.ListWallets(rec, httptest.NewRequest(http.MethodGet, "/api/v1/wallets", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateWallet_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_web3_wallets").WithArgs(anyArgs(12)...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("w1"))

	body := `{"user_id":"u1","address":"0xabc","chain_id":1,"chain_name":"ethereum"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/wallets", strReader(body))
	h.CreateWallet(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateWallet_InvalidJSON(t *testing.T) {
	h, _ := newMockHandlers(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/wallets", strReader("bogus"))
	h.CreateWallet(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateWallet_InsertError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_web3_wallets").WithArgs(anyArgs(12)...).
		WillReturnError(errBoom)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/wallets", strReader(`{"address":"0xabc"}`))
	h.CreateWallet(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestGetWallet_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id").WithArgs("w1", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "user_id", "workspace_id", "address", "chain_id", "chain_name",
			"wallet_type", "ens_name", "label", "is_primary", "is_active", "created_at", "updated_at",
		}).AddRow("w1", "primary", "u1", nil, "0xabc", 1, "ethereum", nil, nil, nil, true, true, nowTime(), nowTime()))

	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/wallets/w1", "", "id", "w1")
	h.GetWallet(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetWallet_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id").WithArgs("missing", pgxmock.AnyArg()).
		WillReturnError(errBoom)

	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/wallets/missing", "", "id", "missing")
	h.GetWallet(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestUpdateWallet_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("UPDATE np_web3_wallets").WithArgs(anyArgs(7)...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodPut, "/api/v1/wallets/w1", `{"label":"main"}`, "id", "w1")
	h.UpdateWallet(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateWallet_InvalidJSON(t *testing.T) {
	h, _ := newMockHandlers(t)
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodPut, "/api/v1/wallets/w1", "bogus", "id", "w1")
	h.UpdateWallet(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestDeleteWallet_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_web3_wallets").WithArgs("w1", pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodDelete, "/api/v1/wallets/w1", "", "id", "w1")
	h.DeleteWallet(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestDeleteWallet_Error(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_web3_wallets").WithArgs("w1", pgxmock.AnyArg()).
		WillReturnError(errBoom)
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodDelete, "/api/v1/wallets/w1", "", "id", "w1")
	h.DeleteWallet(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

// ---- Collections ----

func TestListCollections_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, contract_address").WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "contract_address", "chain_id", "name", "slug", "token_standard",
			"is_verified", "is_spam", "is_managed", "created_at", "updated_at",
		}).AddRow("c1", "primary", "0xc", 1, "Punks", nil, nil, true, false, false, nowTime(), nowTime()))

	rec := httptest.NewRecorder()
	h.ListCollections(rec, httptest.NewRequest(http.MethodGet, "/api/v1/collections", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateCollection_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_web3_collections").WithArgs(anyArgs(10)...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("c1"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/collections", strReader(`{"contract_address":"0xc","chain_id":1,"name":"Punks"}`))
	h.CreateCollection(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetCollection_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, contract_address").WithArgs("c1", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "contract_address", "chain_id", "name", "slug", "token_standard",
			"is_verified", "is_spam", "is_managed", "created_at", "updated_at",
		}).AddRow("c1", "primary", "0xc", 1, "Punks", nil, nil, true, false, false, nowTime(), nowTime()))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/collections/c1", "", "id", "c1")
	h.GetCollection(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateCollection_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("UPDATE np_web3_collections").WithArgs(anyArgs(6)...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodPut, "/api/v1/collections/c1", `{"name":"Punks2"}`, "id", "c1")
	h.UpdateCollection(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeleteCollection_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_web3_collections").WithArgs("c1", pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodDelete, "/api/v1/collections/c1", "", "id", "c1")
	h.DeleteCollection(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}
