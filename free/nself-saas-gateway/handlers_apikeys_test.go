package main

// handlers_apikeys_test.go — /v1/api-keys list/create/revoke over sqlmock.

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestListAPIKeysNeverReturnsSecret(t *testing.T) {
	db, mock := newSQLMock(t)
	mock.ExpectQuery(`SELECT id, tenant_id, key_prefix, name, scopes, created_at, last_used_at, revoked_at`).
		WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "tenant_id", "key_prefix", "name", "scopes", "created_at", "last_used_at", "revoked_at"}).
			AddRow("k-1", testTenant, "nsk_a1b2c3d4", "default", "full", time.Now().UTC(), nil, nil))

	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	rec := doReq(t, g, http.MethodGet, "/v1/api-keys/", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list api-keys = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		APIKeys []apiKeyDTO `json:"api_keys"`
	}
	decodeBody(t, rec, &resp)
	if len(resp.APIKeys) != 1 || resp.APIKeys[0].Prefix != "nsk_a1b2c3d4" || resp.APIKeys[0].Scope != "full" {
		t.Fatalf("api_keys = %+v", resp.APIKeys)
	}
	if strings.Contains(rec.Body.String(), `"key"`) || strings.Contains(rec.Body.String(), "key_hash") {
		t.Errorf("list response leaks key material: %s", rec.Body.String())
	}
}

func TestCreateAPIKeyReturnsSecretOnce(t *testing.T) {
	db, mock := newSQLMock(t)
	mock.ExpectQuery(`INSERT INTO np_saas_api_keys`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("k-new"))

	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	rec := doReq(t, g, http.MethodPost, "/v1/api-keys/", `{"name":"ci key","scope":"read"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create api-key = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		APIKey apiKeyDTO `json:"api_key"`
		Key    string    `json:"key"`
	}
	decodeBody(t, rec, &resp)
	if !strings.HasPrefix(resp.Key, "nsk_") || len(resp.Key) != 68 {
		t.Errorf("raw key = %q, want nsk_ + 64 hex", resp.Key)
	}
	if resp.APIKey.ID != "k-new" || resp.APIKey.Scope != "read" {
		t.Errorf("api_key = %+v", resp.APIKey)
	}
}

func TestCreateAPIKeyValidation(t *testing.T) {
	db, _ := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	if rec := doReq(t, g, http.MethodPost, "/v1/api-keys/", `{"name":""}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("empty name = %d, want 422", rec.Code)
	}
	if rec := doReq(t, g, http.MethodPost, "/v1/api-keys/", `{"name":"x","scope":"admin"}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad scope = %d, want 422", rec.Code)
	}
}

func TestDeleteAPIKeyTenantScoped(t *testing.T) {
	db, mock := newSQLMock(t)
	// Own key: one row revoked → 204.
	mock.ExpectExec(`UPDATE np_saas_api_keys SET revoked_at`).
		WithArgs("k-1", testTenant).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Foreign/unknown key: zero rows → 404.
	mock.ExpectExec(`UPDATE np_saas_api_keys SET revoked_at`).
		WithArgs("k-other", testTenant).
		WillReturnResult(sqlmock.NewResult(0, 0))

	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	if rec := doReq(t, g, http.MethodDelete, "/v1/api-keys/k-1", ""); rec.Code != http.StatusNoContent {
		t.Errorf("revoke own key = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	if rec := doReq(t, g, http.MethodDelete, "/v1/api-keys/k-other", ""); rec.Code != http.StatusNotFound {
		t.Errorf("revoke foreign key = %d, want 404", rec.Code)
	}
}
