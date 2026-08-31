-- Purpose: Create all np_web3_* tables with source_account_id isolation and RLS.
-- Inputs: None (first migration).
-- Outputs: 12 tables, indexes, RLS policies.
-- Constraints: np_ prefix required; source_account_id isolates multi-app installs.
-- SPORT: web3 plugin schema v1.

BEGIN;

-- ============================================================
-- Tables
-- ============================================================

CREATE TABLE IF NOT EXISTS np_web3_wallets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    user_id VARCHAR(255) NOT NULL,
    workspace_id VARCHAR(255),
    address TEXT NOT NULL,
    chain_id INTEGER NOT NULL,
    chain_name TEXT NOT NULL,
    wallet_type TEXT CHECK (wallet_type IN ('eoa', 'contract', 'multisig', 'safe')),
    ens_name TEXT,
    ens_avatar TEXT,
    label TEXT,
    is_primary BOOLEAN DEFAULT false,
    verified_at TIMESTAMPTZ,
    verification_signature TEXT,
    verification_message TEXT,
    is_active BOOLEAN DEFAULT true,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_web3_collections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    contract_address TEXT NOT NULL,
    chain_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    slug TEXT,
    description TEXT,
    image_url TEXT,
    banner_url TEXT,
    featured_image_url TEXT,
    website_url TEXT,
    twitter_username TEXT,
    discord_url TEXT,
    telegram_url TEXT,
    token_standard TEXT CHECK (token_standard IN ('ERC-721', 'ERC-1155', 'ERC-721A', 'other')),
    total_supply BIGINT,
    floor_price DECIMAL(20,8),
    floor_price_currency TEXT,
    volume_total DECIMAL(20,8),
    volume_24h DECIMAL(20,8),
    owners_count INTEGER,
    is_verified BOOLEAN DEFAULT false,
    is_spam BOOLEAN DEFAULT false,
    workspace_id VARCHAR(255),
    is_managed BOOLEAN DEFAULT false,
    metadata JSONB,
    last_indexed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_web3_nfts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    contract_address TEXT NOT NULL,
    token_id TEXT NOT NULL,
    chain_id INTEGER NOT NULL,
    token_standard TEXT NOT NULL CHECK (token_standard IN ('ERC-721', 'ERC-1155', 'ERC-721A', 'other')),
    owner_address TEXT NOT NULL,
    owner_user_id VARCHAR(255),
    quantity BIGINT DEFAULT 1,
    name TEXT,
    description TEXT,
    image_url TEXT,
    animation_url TEXT,
    external_url TEXT,
    attributes JSONB,
    metadata_uri TEXT,
    metadata JSONB,
    collection_id UUID REFERENCES np_web3_collections(id) ON DELETE SET NULL,
    rarity_score DECIMAL(10,2),
    rarity_rank INTEGER,
    is_verified BOOLEAN DEFAULT false,
    is_spam BOOLEAN DEFAULT false,
    last_indexed_at TIMESTAMPTZ,
    block_number BIGINT,
    minted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_web3_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    contract_address TEXT NOT NULL,
    chain_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    symbol TEXT NOT NULL,
    decimals INTEGER NOT NULL,
    token_type TEXT NOT NULL CHECK (token_type IN ('ERC-20', 'native')),
    price_usd DECIMAL(20,8),
    price_updated_at TIMESTAMPTZ,
    logo_url TEXT,
    website_url TEXT,
    description TEXT,
    is_verified BOOLEAN DEFAULT false,
    is_spam BOOLEAN DEFAULT false,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_web3_token_balances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    wallet_address TEXT NOT NULL,
    user_id VARCHAR(255),
    token_id UUID NOT NULL REFERENCES np_web3_tokens(id) ON DELETE CASCADE,
    balance TEXT NOT NULL,
    balance_formatted DECIMAL(30,8),
    value_usd DECIMAL(20,2),
    last_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_web3_token_gates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    workspace_id VARCHAR(255) NOT NULL,
    created_by VARCHAR(255) NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    gate_type TEXT NOT NULL CHECK (gate_type IN ('nft_ownership', 'token_balance', 'token_combination', 'custom')),
    rules JSONB NOT NULL,
    target_type TEXT NOT NULL CHECK (target_type IN ('channel', 'feature', 'content', 'role')),
    target_id VARCHAR(255),
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_web3_gate_checks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    gate_id UUID NOT NULL REFERENCES np_web3_token_gates(id) ON DELETE CASCADE,
    user_id VARCHAR(255) NOT NULL,
    wallet_address TEXT NOT NULL,
    passed BOOLEAN NOT NULL,
    failure_reason TEXT,
    evidence JSONB,
    expires_at TIMESTAMPTZ,
    checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_web3_daos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    workspace_id VARCHAR(255),
    name TEXT NOT NULL,
    slug TEXT,
    description TEXT,
    chain_id INTEGER NOT NULL,
    governance_token_address TEXT,
    treasury_address TEXT,
    snapshot_space TEXT,
    governor_address TEXT,
    governor_type TEXT CHECK (governor_type IN ('compound', 'openzeppelin', 'custom')),
    proposal_threshold TEXT,
    quorum TEXT,
    voting_delay BIGINT,
    voting_period BIGINT,
    is_active BOOLEAN DEFAULT true,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_web3_proposals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    dao_id UUID NOT NULL REFERENCES np_web3_daos(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    proposer_address TEXT NOT NULL,
    proposer_user_id VARCHAR(255),
    chain_proposal_id TEXT,
    snapshot_proposal_id TEXT,
    status TEXT NOT NULL CHECK (status IN ('pending', 'active', 'canceled', 'defeated', 'succeeded', 'queued', 'expired', 'executed')),
    start_block BIGINT,
    end_block BIGINT,
    start_time TIMESTAMPTZ,
    end_time TIMESTAMPTZ,
    votes_for TEXT DEFAULT '0',
    votes_against TEXT DEFAULT '0',
    votes_abstain TEXT DEFAULT '0',
    executable BOOLEAN DEFAULT false,
    executed_at TIMESTAMPTZ,
    execution_tx_hash TEXT,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_web3_votes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    proposal_id UUID NOT NULL REFERENCES np_web3_proposals(id) ON DELETE CASCADE,
    voter_address TEXT NOT NULL,
    voter_user_id VARCHAR(255),
    support TEXT NOT NULL CHECK (support IN ('for', 'against', 'abstain')),
    voting_power TEXT NOT NULL,
    reason TEXT,
    transaction_hash TEXT,
    block_number BIGINT,
    voted_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_web3_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    transaction_hash TEXT NOT NULL,
    chain_id INTEGER NOT NULL,
    from_address TEXT NOT NULL,
    to_address TEXT,
    from_user_id VARCHAR(255),
    to_user_id VARCHAR(255),
    value TEXT NOT NULL,
    value_usd DECIMAL(20,2),
    gas_used BIGINT,
    gas_price TEXT,
    block_number BIGINT,
    block_timestamp TIMESTAMPTZ,
    status TEXT CHECK (status IN ('pending', 'confirmed', 'failed')),
    transaction_type TEXT CHECK (transaction_type IN ('transfer', 'contract_interaction', 'nft_transfer', 'token_transfer', 'swap')),
    input_data TEXT,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_web3_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    event_name TEXT NOT NULL,
    contract_address TEXT NOT NULL,
    chain_id INTEGER NOT NULL,
    transaction_hash TEXT NOT NULL,
    log_index INTEGER NOT NULL,
    block_number BIGINT NOT NULL,
    block_timestamp TIMESTAMPTZ,
    event_data JSONB NOT NULL,
    decoded_data JSONB,
    related_nft_id UUID REFERENCES np_web3_nfts(id) ON DELETE SET NULL,
    related_user_id VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- Indexes
-- ============================================================

CREATE INDEX IF NOT EXISTS idx_np_web3_wallets_source ON np_web3_wallets(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_wallets_user ON np_web3_wallets(source_account_id, user_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_wallets_address ON np_web3_wallets(source_account_id, address);
CREATE INDEX IF NOT EXISTS idx_np_web3_wallets_chain ON np_web3_wallets(source_account_id, chain_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_wallets_ens ON np_web3_wallets(ens_name) WHERE ens_name IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_np_web3_collections_source ON np_web3_collections(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_collections_contract ON np_web3_collections(source_account_id, contract_address, chain_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_collections_slug ON np_web3_collections(source_account_id, slug);
CREATE INDEX IF NOT EXISTS idx_np_web3_nfts_source ON np_web3_nfts(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_nfts_contract ON np_web3_nfts(source_account_id, contract_address, chain_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_nfts_owner ON np_web3_nfts(source_account_id, owner_address);
CREATE INDEX IF NOT EXISTS idx_np_web3_nfts_collection ON np_web3_nfts(source_account_id, collection_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_nfts_attributes ON np_web3_nfts USING GIN(attributes);
CREATE INDEX IF NOT EXISTS idx_np_web3_tokens_source ON np_web3_tokens(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_tokens_contract ON np_web3_tokens(source_account_id, contract_address, chain_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_tokens_symbol ON np_web3_tokens(source_account_id, symbol);
CREATE INDEX IF NOT EXISTS idx_np_web3_token_balances_source ON np_web3_token_balances(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_token_balances_wallet ON np_web3_token_balances(source_account_id, wallet_address);
CREATE INDEX IF NOT EXISTS idx_np_web3_token_balances_user ON np_web3_token_balances(source_account_id, user_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_token_gates_source ON np_web3_token_gates(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_token_gates_workspace ON np_web3_token_gates(source_account_id, workspace_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_token_gates_target ON np_web3_token_gates(source_account_id, target_type, target_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_token_gates_rules ON np_web3_token_gates USING GIN(rules);
CREATE INDEX IF NOT EXISTS idx_np_web3_gate_checks_source ON np_web3_gate_checks(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_gate_checks_gate ON np_web3_gate_checks(source_account_id, gate_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_gate_checks_user ON np_web3_gate_checks(source_account_id, user_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_gate_checks_expires ON np_web3_gate_checks(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_np_web3_daos_source ON np_web3_daos(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_daos_workspace ON np_web3_daos(source_account_id, workspace_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_daos_slug ON np_web3_daos(source_account_id, slug);
CREATE INDEX IF NOT EXISTS idx_np_web3_proposals_source ON np_web3_proposals(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_proposals_dao ON np_web3_proposals(source_account_id, dao_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_proposals_status ON np_web3_proposals(source_account_id, status);
CREATE INDEX IF NOT EXISTS idx_np_web3_votes_source ON np_web3_votes(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_votes_proposal ON np_web3_votes(source_account_id, proposal_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_votes_voter ON np_web3_votes(source_account_id, voter_address);
CREATE INDEX IF NOT EXISTS idx_np_web3_transactions_source ON np_web3_transactions(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_transactions_hash ON np_web3_transactions(source_account_id, transaction_hash);
CREATE INDEX IF NOT EXISTS idx_np_web3_transactions_chain ON np_web3_transactions(source_account_id, chain_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_transactions_from ON np_web3_transactions(source_account_id, from_address);
CREATE INDEX IF NOT EXISTS idx_np_web3_transactions_to ON np_web3_transactions(source_account_id, to_address);
CREATE INDEX IF NOT EXISTS idx_np_web3_transactions_block ON np_web3_transactions(block_number DESC);
CREATE INDEX IF NOT EXISTS idx_np_web3_events_source ON np_web3_events(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_events_contract ON np_web3_events(source_account_id, contract_address, chain_id);
CREATE INDEX IF NOT EXISTS idx_np_web3_events_name ON np_web3_events(source_account_id, event_name);
CREATE INDEX IF NOT EXISTS idx_np_web3_events_tx ON np_web3_events(source_account_id, transaction_hash);
CREATE INDEX IF NOT EXISTS idx_np_web3_events_block ON np_web3_events(block_number DESC);
CREATE INDEX IF NOT EXISTS idx_np_web3_events_data ON np_web3_events USING GIN(event_data);

-- ============================================================
-- Row-Level Security (source_account_id isolation)
-- Convention: Multi-App Isolation per multi-tenant-conventions.md
-- ============================================================

ALTER TABLE np_web3_wallets ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_web3_collections ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_web3_nfts ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_web3_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_web3_token_balances ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_web3_token_gates ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_web3_gate_checks ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_web3_daos ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_web3_proposals ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_web3_votes ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_web3_transactions ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_web3_events ENABLE ROW LEVEL SECURITY;

-- Service role bypasses RLS; app role enforces source_account_id isolation.
-- Policies created only if the nself_app role exists; skipped otherwise.

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nself_app') THEN
    EXECUTE $p$
      CREATE POLICY np_web3_wallets_isolation ON np_web3_wallets
        USING (source_account_id = current_setting('nself.source_account_id', true));
      CREATE POLICY np_web3_collections_isolation ON np_web3_collections
        USING (source_account_id = current_setting('nself.source_account_id', true));
      CREATE POLICY np_web3_nfts_isolation ON np_web3_nfts
        USING (source_account_id = current_setting('nself.source_account_id', true));
      CREATE POLICY np_web3_tokens_isolation ON np_web3_tokens
        USING (source_account_id = current_setting('nself.source_account_id', true));
      CREATE POLICY np_web3_token_balances_isolation ON np_web3_token_balances
        USING (source_account_id = current_setting('nself.source_account_id', true));
      CREATE POLICY np_web3_token_gates_isolation ON np_web3_token_gates
        USING (source_account_id = current_setting('nself.source_account_id', true));
      CREATE POLICY np_web3_gate_checks_isolation ON np_web3_gate_checks
        USING (source_account_id = current_setting('nself.source_account_id', true));
      CREATE POLICY np_web3_daos_isolation ON np_web3_daos
        USING (source_account_id = current_setting('nself.source_account_id', true));
      CREATE POLICY np_web3_proposals_isolation ON np_web3_proposals
        USING (source_account_id = current_setting('nself.source_account_id', true));
      CREATE POLICY np_web3_votes_isolation ON np_web3_votes
        USING (source_account_id = current_setting('nself.source_account_id', true));
      CREATE POLICY np_web3_transactions_isolation ON np_web3_transactions
        USING (source_account_id = current_setting('nself.source_account_id', true));
      CREATE POLICY np_web3_events_isolation ON np_web3_events
        USING (source_account_id = current_setting('nself.source_account_id', true));
    $p$;
  END IF;
END;
$$;

COMMIT;
