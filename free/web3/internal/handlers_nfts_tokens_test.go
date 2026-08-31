// Purpose: DB-backed handler tests for NFT and Token CRUD endpoints.
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

// ---- NFTs ----

func TestListNFTs_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, contract_address, token_id").WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "contract_address", "token_id", "chain_id", "token_standard",
			"owner_address", "quantity", "name", "is_verified", "is_spam", "created_at", "updated_at",
		}).AddRow("n1", "primary", "0xc", "1", 1, "ERC-721", "0xowner", int64(1), nil, false, false, nowTime(), nowTime()))
	rec := httptest.NewRecorder()
	h.ListNFTs(rec, httptest.NewRequest(http.MethodGet, "/api/v1/nfts", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestListNFTs_QueryError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, contract_address, token_id").WithArgs(pgxmock.AnyArg()).
		WillReturnError(errBoom)
	rec := httptest.NewRecorder()
	h.ListNFTs(rec, httptest.NewRequest(http.MethodGet, "/api/v1/nfts", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateNFT_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_web3_nfts").WithArgs(anyArgs(13)...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("n1"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nfts", strReader(`{"contract_address":"0xc","token_id":"1","chain_id":1}`))
	h.CreateNFT(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateNFT_InvalidJSON(t *testing.T) {
	h, _ := newMockHandlers(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nfts", strReader("bogus"))
	h.CreateNFT(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestGetNFT_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, contract_address, token_id").WithArgs("n1", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "contract_address", "token_id", "chain_id", "token_standard",
			"owner_address", "quantity", "name", "is_verified", "is_spam", "created_at", "updated_at",
		}).AddRow("n1", "primary", "0xc", "1", 1, "ERC-721", "0xowner", int64(1), nil, false, false, nowTime(), nowTime()))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/nfts/n1", "", "id", "n1")
	h.GetNFT(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetNFT_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, contract_address, token_id").WithArgs("missing", pgxmock.AnyArg()).
		WillReturnError(errBoom)
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/nfts/missing", "", "id", "missing")
	h.GetNFT(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestUpdateNFT_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("UPDATE np_web3_nfts").WithArgs(anyArgs(6)...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodPut, "/api/v1/nfts/n1", `{"name":"Punk #1"}`, "id", "n1")
	h.UpdateNFT(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeleteNFT_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_web3_nfts").WithArgs("n1", pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodDelete, "/api/v1/nfts/n1", "", "id", "n1")
	h.DeleteNFT(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

// ---- Tokens ----

func TestListTokens_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, contract_address, chain_id, name, symbol").WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "contract_address", "chain_id", "name", "symbol", "decimals", "token_type",
			"is_verified", "is_spam", "created_at", "updated_at",
		}).AddRow("t1", "primary", "0xt", 1, "Token", "TOK", 18, "ERC-20", false, false, nowTime(), nowTime()))
	rec := httptest.NewRecorder()
	h.ListTokens(rec, httptest.NewRequest(http.MethodGet, "/api/v1/tokens", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateToken_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_web3_tokens").WithArgs(anyArgs(9)...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("t1"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tokens", strReader(`{"contract_address":"0xt","chain_id":1,"name":"Token","symbol":"TOK","decimals":18,"token_type":"ERC-20"}`))
	h.CreateToken(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetToken_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, contract_address, chain_id, name, symbol").WithArgs("t1", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "contract_address", "chain_id", "name", "symbol", "decimals", "token_type",
			"is_verified", "is_spam", "created_at", "updated_at",
		}).AddRow("t1", "primary", "0xt", 1, "Token", "TOK", 18, "ERC-20", false, false, nowTime(), nowTime()))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/tokens/t1", "", "id", "t1")
	h.GetToken(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateToken_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("UPDATE np_web3_tokens").WithArgs(anyArgs(6)...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodPut, "/api/v1/tokens/t1", `{"name":"Token2"}`, "id", "t1")
	h.UpdateToken(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeleteToken_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_web3_tokens").WithArgs("t1", pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodDelete, "/api/v1/tokens/t1", "", "id", "t1")
	h.DeleteToken(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}
