package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type Handlers struct {
	pool PgxIface
}

func NewHandlers(pool PgxIface) *Handlers {
	return &Handlers{pool: pool}
}

func (h *Handlers) sa(r *http.Request) string {
	if v := r.Header.Get("X-Source-Account-ID"); v != "" {
		return v
	}
	return "primary"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// Health / Ready

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "plugin": "web3"})
}

func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := h.pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// ============================================================
// Wallets
// ============================================================

func (h *Handlers) ListWallets(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, user_id, workspace_id, address, chain_id, chain_name,
		        wallet_type, ens_name, label, is_primary, is_active, created_at, updated_at
		 FROM np_web3_wallets WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []Wallet
	for rows.Next() {
		var wl Wallet
		if err := rows.Scan(&wl.ID, &wl.SourceAccountID, &wl.UserID, &wl.WorkspaceID,
			&wl.Address, &wl.ChainID, &wl.ChainName, &wl.WalletType, &wl.EnsName,
			&wl.Label, &wl.IsPrimary, &wl.IsActive, &wl.CreatedAt, &wl.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, wl)
	}
	if items == nil {
		items = []Wallet{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) CreateWallet(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sa := h.sa(r)
	var id string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_web3_wallets (source_account_id, user_id, workspace_id, address, chain_id, chain_name,
		  wallet_type, ens_name, ens_avatar, label, is_primary, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`,
		sa, body["user_id"], body["workspace_id"], body["address"], body["chain_id"], body["chain_name"],
		body["wallet_type"], body["ens_name"], body["ens_avatar"], body["label"],
		body["is_primary"], marshalJSON(body["metadata"])).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handlers) GetWallet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var wl Wallet
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, user_id, workspace_id, address, chain_id, chain_name,
		        wallet_type, ens_name, label, is_primary, is_active, created_at, updated_at
		 FROM np_web3_wallets WHERE id=$1 AND source_account_id=$2`, id, sa).
		Scan(&wl.ID, &wl.SourceAccountID, &wl.UserID, &wl.WorkspaceID,
			&wl.Address, &wl.ChainID, &wl.ChainName, &wl.WalletType, &wl.EnsName,
			&wl.Label, &wl.IsPrimary, &wl.IsActive, &wl.CreatedAt, &wl.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, wl)
}

func (h *Handlers) UpdateWallet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	_, err := h.pool.Exec(r.Context(),
		`UPDATE np_web3_wallets SET label=$3, ens_name=$4, ens_avatar=$5, is_primary=$6, is_active=$7, updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa, body["label"], body["ens_name"], body["ens_avatar"], body["is_primary"], body["is_active"])
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("update: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (h *Handlers) DeleteWallet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	if _, err := h.pool.Exec(r.Context(),
		`DELETE FROM np_web3_wallets WHERE id=$1 AND source_account_id=$2`, id, sa); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Collections
// ============================================================

func (h *Handlers) ListCollections(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, contract_address, chain_id, name, slug, token_standard,
		        is_verified, is_spam, is_managed, created_at, updated_at
		 FROM np_web3_collections WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []Collection
	for rows.Next() {
		var c Collection
		if err := rows.Scan(&c.ID, &c.SourceAccountID, &c.ContractAddress, &c.ChainID, &c.Name,
			&c.Slug, &c.TokenStandard, &c.IsVerified, &c.IsSpam, &c.IsManaged,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, c)
	}
	if items == nil {
		items = []Collection{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) CreateCollection(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sa := h.sa(r)
	var id string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_web3_collections (source_account_id, contract_address, chain_id, name, slug,
		  description, image_url, token_standard, workspace_id, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		sa, body["contract_address"], body["chain_id"], body["name"], body["slug"],
		body["description"], body["image_url"], body["token_standard"],
		body["workspace_id"], marshalJSON(body["metadata"])).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handlers) GetCollection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var c Collection
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, contract_address, chain_id, name, slug, token_standard,
		        is_verified, is_spam, is_managed, created_at, updated_at
		 FROM np_web3_collections WHERE id=$1 AND source_account_id=$2`, id, sa).
		Scan(&c.ID, &c.SourceAccountID, &c.ContractAddress, &c.ChainID, &c.Name,
			&c.Slug, &c.TokenStandard, &c.IsVerified, &c.IsSpam, &c.IsManaged,
			&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handlers) UpdateCollection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	_, err := h.pool.Exec(r.Context(),
		`UPDATE np_web3_collections SET name=$3, description=$4, image_url=$5, is_verified=$6, updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa, body["name"], body["description"], body["image_url"], body["is_verified"])
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("update: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (h *Handlers) DeleteCollection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	if _, err := h.pool.Exec(r.Context(),
		`DELETE FROM np_web3_collections WHERE id=$1 AND source_account_id=$2`, id, sa); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// NFTs
// ============================================================

func (h *Handlers) ListNFTs(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, contract_address, token_id, chain_id, token_standard,
		        owner_address, quantity, name, is_verified, is_spam, created_at, updated_at
		 FROM np_web3_nfts WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []NFT
	for rows.Next() {
		var n NFT
		if err := rows.Scan(&n.ID, &n.SourceAccountID, &n.ContractAddress, &n.TokenID, &n.ChainID,
			&n.TokenStandard, &n.OwnerAddress, &n.Quantity, &n.Name,
			&n.IsVerified, &n.IsSpam, &n.CreatedAt, &n.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, n)
	}
	if items == nil {
		items = []NFT{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) CreateNFT(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sa := h.sa(r)
	var id string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_web3_nfts (source_account_id, contract_address, token_id, chain_id, token_standard,
		  owner_address, owner_user_id, quantity, name, description, image_url, metadata_uri, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`,
		sa, body["contract_address"], body["token_id"], body["chain_id"], body["token_standard"],
		body["owner_address"], body["owner_user_id"], body["quantity"], body["name"],
		body["description"], body["image_url"], body["metadata_uri"],
		marshalJSON(body["metadata"])).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handlers) GetNFT(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var n NFT
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, contract_address, token_id, chain_id, token_standard,
		        owner_address, quantity, name, is_verified, is_spam, created_at, updated_at
		 FROM np_web3_nfts WHERE id=$1 AND source_account_id=$2`, id, sa).
		Scan(&n.ID, &n.SourceAccountID, &n.ContractAddress, &n.TokenID, &n.ChainID,
			&n.TokenStandard, &n.OwnerAddress, &n.Quantity, &n.Name,
			&n.IsVerified, &n.IsSpam, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (h *Handlers) UpdateNFT(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	_, err := h.pool.Exec(r.Context(),
		`UPDATE np_web3_nfts SET owner_address=$3, name=$4, description=$5, is_verified=$6, updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa, body["owner_address"], body["name"], body["description"], body["is_verified"])
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("update: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (h *Handlers) DeleteNFT(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	if _, err := h.pool.Exec(r.Context(),
		`DELETE FROM np_web3_nfts WHERE id=$1 AND source_account_id=$2`, id, sa); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Tokens
// ============================================================

func (h *Handlers) ListTokens(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, contract_address, chain_id, name, symbol, decimals, token_type,
		        is_verified, is_spam, created_at, updated_at
		 FROM np_web3_tokens WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []Token
	for rows.Next() {
		var t Token
		if err := rows.Scan(&t.ID, &t.SourceAccountID, &t.ContractAddress, &t.ChainID, &t.Name,
			&t.Symbol, &t.Decimals, &t.TokenType, &t.IsVerified, &t.IsSpam,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, t)
	}
	if items == nil {
		items = []Token{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) CreateToken(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sa := h.sa(r)
	var id string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_web3_tokens (source_account_id, contract_address, chain_id, name, symbol, decimals, token_type, logo_url, description)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		sa, body["contract_address"], body["chain_id"], body["name"], body["symbol"],
		body["decimals"], body["token_type"], body["logo_url"], body["description"]).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handlers) GetToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var t Token
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, contract_address, chain_id, name, symbol, decimals, token_type,
		        is_verified, is_spam, created_at, updated_at
		 FROM np_web3_tokens WHERE id=$1 AND source_account_id=$2`, id, sa).
		Scan(&t.ID, &t.SourceAccountID, &t.ContractAddress, &t.ChainID, &t.Name,
			&t.Symbol, &t.Decimals, &t.TokenType, &t.IsVerified, &t.IsSpam,
			&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *Handlers) UpdateToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	_, err := h.pool.Exec(r.Context(),
		`UPDATE np_web3_tokens SET name=$3, symbol=$4, logo_url=$5, is_verified=$6, updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa, body["name"], body["symbol"], body["logo_url"], body["is_verified"])
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("update: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (h *Handlers) DeleteToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	if _, err := h.pool.Exec(r.Context(),
		`DELETE FROM np_web3_tokens WHERE id=$1 AND source_account_id=$2`, id, sa); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Token Balances
// ============================================================

func (h *Handlers) ListTokenBalances(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, wallet_address, user_id, token_id, balance, last_updated_at, created_at
		 FROM np_web3_token_balances WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []TokenBalance
	for rows.Next() {
		var tb TokenBalance
		if err := rows.Scan(&tb.ID, &tb.SourceAccountID, &tb.WalletAddress, &tb.UserID,
			&tb.TokenID, &tb.Balance, &tb.LastUpdatedAt, &tb.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, tb)
	}
	if items == nil {
		items = []TokenBalance{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) UpsertTokenBalance(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sa := h.sa(r)
	var id string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_web3_token_balances (source_account_id, wallet_address, user_id, token_id, balance, balance_formatted, value_usd)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (source_account_id, wallet_address, token_id) DO UPDATE
		   SET balance=EXCLUDED.balance, balance_formatted=EXCLUDED.balance_formatted,
		       value_usd=EXCLUDED.value_usd, last_updated_at=NOW()
		 RETURNING id`,
		sa, body["wallet_address"], body["user_id"], body["token_id"], body["balance"],
		body["balance_formatted"], body["value_usd"]).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("upsert: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (h *Handlers) DeleteTokenBalance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	if _, err := h.pool.Exec(r.Context(),
		`DELETE FROM np_web3_token_balances WHERE id=$1 AND source_account_id=$2`, id, sa); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Token Gates
// ============================================================

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

// ============================================================
// DAOs
// ============================================================

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

// ============================================================
// Proposals
// ============================================================

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

func (h *Handlers) ListTransactions(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, transaction_hash, chain_id, from_address, to_address,
		        value, status, transaction_type, created_at
		 FROM np_web3_transactions WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []Transaction
	for rows.Next() {
		var tx Transaction
		if err := rows.Scan(&tx.ID, &tx.SourceAccountID, &tx.TransactionHash, &tx.ChainID,
			&tx.FromAddress, &tx.ToAddress, &tx.Value, &tx.Status, &tx.TransactionType,
			&tx.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, tx)
	}
	if items == nil {
		items = []Transaction{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) CreateTransaction(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sa := h.sa(r)
	var id string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_web3_transactions (source_account_id, transaction_hash, chain_id, from_address,
		  to_address, from_user_id, to_user_id, value, status, transaction_type)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		sa, body["transaction_hash"], body["chain_id"], body["from_address"], body["to_address"],
		body["from_user_id"], body["to_user_id"], body["value"], body["status"],
		body["transaction_type"]).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handlers) GetTransaction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var tx Transaction
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, transaction_hash, chain_id, from_address, to_address,
		        value, status, transaction_type, created_at
		 FROM np_web3_transactions WHERE id=$1 AND source_account_id=$2`, id, sa).
		Scan(&tx.ID, &tx.SourceAccountID, &tx.TransactionHash, &tx.ChainID,
			&tx.FromAddress, &tx.ToAddress, &tx.Value, &tx.Status, &tx.TransactionType,
			&tx.CreatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, tx)
}

// ============================================================
// Events
// ============================================================

func (h *Handlers) ListEvents(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, event_name, contract_address, chain_id,
		        transaction_hash, block_number, created_at
		 FROM np_web3_events WHERE source_account_id=$1 ORDER BY block_number DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []Web3Event
	for rows.Next() {
		var ev Web3Event
		if err := rows.Scan(&ev.ID, &ev.SourceAccountID, &ev.EventName, &ev.ContractAddress, &ev.ChainID,
			&ev.TransactionHash, &ev.BlockNumber, &ev.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, ev)
	}
	if items == nil {
		items = []Web3Event{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) CreateEvent(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sa := h.sa(r)
	var id string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_web3_events (source_account_id, event_name, contract_address, chain_id,
		  transaction_hash, log_index, block_number, event_data, related_user_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		sa, body["event_name"], body["contract_address"], body["chain_id"],
		body["transaction_hash"], body["log_index"], body["block_number"],
		marshalJSON(body["event_data"]), body["related_user_id"]).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handlers) GetEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var ev Web3Event
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, event_name, contract_address, chain_id,
		        transaction_hash, block_number, created_at
		 FROM np_web3_events WHERE id=$1 AND source_account_id=$2`, id, sa).
		Scan(&ev.ID, &ev.SourceAccountID, &ev.EventName, &ev.ContractAddress, &ev.ChainID,
			&ev.TransactionHash, &ev.BlockNumber, &ev.CreatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// ============================================================
// Helper
// ============================================================

func marshalJSON(v any) []byte {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
