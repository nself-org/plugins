/**
 * Web3 Plugin Database
 * Schema initialization, CRUD operations for wallets, NFTs, collections,
 * tokens, token gates, DAOs, proposals, votes, transactions, and events
 */

import { Database, createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import type {
  WalletRecord, NftRecord, CollectionRecord, TokenRecord,
  TokenBalanceRecord, TokenGateRecord, GateCheckRecord,
  DaoRecord, ProposalRecord, VoteRecord, TransactionRecord, Web3EventRecord,
  CreateWalletRequest, UpdateWalletRequest, VerifyWalletRequest,
  CreateNftRequest, UpdateNftRequest,
  CreateCollectionRequest, UpdateCollectionRequest,
  CreateTokenRequest, UpdateTokenRequest,
  UpsertTokenBalanceRequest,
  CreateTokenGateRequest, UpdateTokenGateRequest,
  CreateGateCheckRequest,
  CreateDaoRequest, UpdateDaoRequest,
  CreateProposalRequest, UpdateProposalRequest,
  CreateVoteRequest,
  CreateTransactionRequest,
  CreateWeb3EventRequest,
  WalletFilters, NftFilters, CollectionFilters, TokenFilters,
  TokenGateFilters, ProposalFilters, TransactionFilters, Web3EventFilters,
  GateCheckResult, Web3Stats,
} from './types.js';

const logger = createLogger('web3:database');

export class Web3Database {
  private db: Database;
  private sourceAccountId: string;

  constructor(sourceAccountId = 'primary') {
    const config = loadConfig();
    this.db = new Database({
      host: config.databaseHost,
      port: config.databasePort,
      database: config.databaseName,
      user: config.databaseUser,
      password: config.databasePassword,
      ssl: config.databaseSsl,
    });
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  private normalizeSourceAccountId(id: string): string {
    return id.trim().toLowerCase() || 'primary';
  }

  forSourceAccount(sourceAccountId: string): Web3Database {
    const instance = Object.create(Web3Database.prototype) as Web3Database;
    instance.db = this.db;
    instance.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
    return instance;
  }

  async connect(): Promise<void> {
    await this.db.connect();
    logger.info('Database connected');
  }

  async disconnect(): Promise<void> {
    await this.db.disconnect();
    logger.info('Database disconnected');
  }

  async query(sql: string, params?: unknown[]): Promise<{ rows: Record<string, unknown>[] }> {
    return this.db.query(sql, params);
  }

  // =========================================================================
  // Schema Initialization
  // =========================================================================

  async initializeSchema(): Promise<void> {
    logger.info('Initializing web3 schema...');

    await this.db.query(`
      CREATE TABLE IF NOT EXISTS web3_wallets (
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
      )
    `);

    await this.db.query(`
      CREATE TABLE IF NOT EXISTS web3_collections (
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
      )
    `);

    await this.db.query(`
      CREATE TABLE IF NOT EXISTS web3_nfts (
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
        collection_id UUID REFERENCES web3_collections(id) ON DELETE SET NULL,
        rarity_score DECIMAL(10,2),
        rarity_rank INTEGER,
        is_verified BOOLEAN DEFAULT false,
        is_spam BOOLEAN DEFAULT false,
        last_indexed_at TIMESTAMPTZ,
        block_number BIGINT,
        minted_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      )
    `);

    await this.db.query(`
      CREATE TABLE IF NOT EXISTS web3_tokens (
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
      )
    `);

    await this.db.query(`
      CREATE TABLE IF NOT EXISTS web3_token_balances (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        wallet_address TEXT NOT NULL,
        user_id VARCHAR(255),
        token_id UUID NOT NULL REFERENCES web3_tokens(id) ON DELETE CASCADE,
        balance TEXT NOT NULL,
        balance_formatted DECIMAL(30,8),
        value_usd DECIMAL(20,2),
        last_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      )
    `);

    await this.db.query(`
      CREATE TABLE IF NOT EXISTS web3_token_gates (
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
      )
    `);

    await this.db.query(`
      CREATE TABLE IF NOT EXISTS web3_gate_checks (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        gate_id UUID NOT NULL REFERENCES web3_token_gates(id) ON DELETE CASCADE,
        user_id VARCHAR(255) NOT NULL,
        wallet_address TEXT NOT NULL,
        passed BOOLEAN NOT NULL,
        failure_reason TEXT,
        evidence JSONB,
        expires_at TIMESTAMPTZ,
        checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      )
    `);

    await this.db.query(`
      CREATE TABLE IF NOT EXISTS web3_daos (
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
      )
    `);

    await this.db.query(`
      CREATE TABLE IF NOT EXISTS web3_proposals (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        dao_id UUID NOT NULL REFERENCES web3_daos(id) ON DELETE CASCADE,
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
      )
    `);

    await this.db.query(`
      CREATE TABLE IF NOT EXISTS web3_votes (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        proposal_id UUID NOT NULL REFERENCES web3_proposals(id) ON DELETE CASCADE,
        voter_address TEXT NOT NULL,
        voter_user_id VARCHAR(255),
        support TEXT NOT NULL CHECK (support IN ('for', 'against', 'abstain')),
        voting_power TEXT NOT NULL,
        reason TEXT,
        transaction_hash TEXT,
        block_number BIGINT,
        voted_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      )
    `);

    await this.db.query(`
      CREATE TABLE IF NOT EXISTS web3_transactions (
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
      )
    `);

    await this.db.query(`
      CREATE TABLE IF NOT EXISTS web3_events (
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
        related_nft_id UUID REFERENCES web3_nfts(id) ON DELETE SET NULL,
        related_user_id VARCHAR(255),
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      )
    `);

    // Indexes
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_wallets_source ON web3_wallets(source_account_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_wallets_user ON web3_wallets(source_account_id, user_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_wallets_address ON web3_wallets(source_account_id, address)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_wallets_chain ON web3_wallets(source_account_id, chain_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_wallets_ens ON web3_wallets(ens_name) WHERE ens_name IS NOT NULL`);

    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_collections_source ON web3_collections(source_account_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_collections_contract ON web3_collections(source_account_id, contract_address, chain_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_collections_slug ON web3_collections(source_account_id, slug)`);

    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_nfts_source ON web3_nfts(source_account_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_nfts_contract ON web3_nfts(source_account_id, contract_address, chain_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_nfts_owner ON web3_nfts(source_account_id, owner_address)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_nfts_collection ON web3_nfts(source_account_id, collection_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_nfts_attributes ON web3_nfts USING GIN(attributes)`);

    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_tokens_source ON web3_tokens(source_account_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_tokens_contract ON web3_tokens(source_account_id, contract_address, chain_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_tokens_symbol ON web3_tokens(source_account_id, symbol)`);

    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_token_balances_source ON web3_token_balances(source_account_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_token_balances_wallet ON web3_token_balances(source_account_id, wallet_address)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_token_balances_user ON web3_token_balances(source_account_id, user_id)`);

    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_token_gates_source ON web3_token_gates(source_account_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_token_gates_workspace ON web3_token_gates(source_account_id, workspace_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_token_gates_target ON web3_token_gates(source_account_id, target_type, target_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_token_gates_rules ON web3_token_gates USING GIN(rules)`);

    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_gate_checks_source ON web3_gate_checks(source_account_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_gate_checks_gate ON web3_gate_checks(source_account_id, gate_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_gate_checks_user ON web3_gate_checks(source_account_id, user_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_gate_checks_expires ON web3_gate_checks(expires_at) WHERE expires_at IS NOT NULL`);

    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_daos_source ON web3_daos(source_account_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_daos_workspace ON web3_daos(source_account_id, workspace_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_daos_slug ON web3_daos(source_account_id, slug)`);

    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_proposals_source ON web3_proposals(source_account_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_proposals_dao ON web3_proposals(source_account_id, dao_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_proposals_status ON web3_proposals(source_account_id, status)`);

    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_votes_source ON web3_votes(source_account_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_votes_proposal ON web3_votes(source_account_id, proposal_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_votes_voter ON web3_votes(source_account_id, voter_address)`);

    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_transactions_source ON web3_transactions(source_account_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_transactions_hash ON web3_transactions(source_account_id, transaction_hash)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_transactions_chain ON web3_transactions(source_account_id, chain_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_transactions_from ON web3_transactions(source_account_id, from_address)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_transactions_to ON web3_transactions(source_account_id, to_address)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_transactions_block ON web3_transactions(block_number DESC)`);

    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_events_source ON web3_events(source_account_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_events_contract ON web3_events(source_account_id, contract_address, chain_id)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_events_name ON web3_events(source_account_id, event_name)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_events_tx ON web3_events(source_account_id, transaction_hash)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_events_block ON web3_events(block_number DESC)`);
    await this.db.query(`CREATE INDEX IF NOT EXISTS idx_web3_events_data ON web3_events USING GIN(event_data)`);

    logger.success('Web3 schema initialized');
  }

  // =========================================================================
  // Wallets
  // =========================================================================

  async createWallet(data: CreateWalletRequest): Promise<string> {
    const result = await this.db.query(
      `INSERT INTO web3_wallets (source_account_id, user_id, workspace_id, address, chain_id, chain_name, wallet_type, ens_name, ens_avatar, label, is_primary, metadata)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
       RETURNING id`,
      [this.sourceAccountId, data.user_id, data.workspace_id ?? null, data.address, data.chain_id, data.chain_name,
       data.wallet_type ?? null, data.ens_name ?? null, data.ens_avatar ?? null, data.label ?? null,
       data.is_primary ?? false, data.metadata ? JSON.stringify(data.metadata) : null]
    );
    return result.rows[0].id as string;
  }

  async getWallet(id: string): Promise<WalletRecord | null> {
    const result = await this.db.query(
      `SELECT * FROM web3_wallets WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return (result.rows[0] as WalletRecord) ?? null;
  }

  async getWalletByAddress(address: string, chainId: number): Promise<WalletRecord | null> {
    const result = await this.db.query(
      `SELECT * FROM web3_wallets WHERE address = $1 AND chain_id = $2 AND source_account_id = $3`,
      [address, chainId, this.sourceAccountId]
    );
    return (result.rows[0] as WalletRecord) ?? null;
  }

  async listWallets(filters: WalletFilters = {}): Promise<WalletRecord[]> {
    const conditions = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let idx = 2;

    if (filters.user_id) { conditions.push(`user_id = $${idx++}`); params.push(filters.user_id); }
    if (filters.workspace_id) { conditions.push(`workspace_id = $${idx++}`); params.push(filters.workspace_id); }
    if (filters.chain_id !== undefined) { conditions.push(`chain_id = $${idx++}`); params.push(filters.chain_id); }
    if (filters.is_active !== undefined) { conditions.push(`is_active = $${idx++}`); params.push(filters.is_active); }

    const result = await this.db.query(
      `SELECT * FROM web3_wallets WHERE ${conditions.join(' AND ')} ORDER BY created_at DESC`,
      params
    );
    return result.rows as WalletRecord[];
  }

  async updateWallet(id: string, data: UpdateWalletRequest): Promise<boolean> {
    const fields: string[] = [];
    const params: unknown[] = [];
    let idx = 1;

    if (data.ens_name !== undefined) { fields.push(`ens_name = $${idx++}`); params.push(data.ens_name); }
    if (data.ens_avatar !== undefined) { fields.push(`ens_avatar = $${idx++}`); params.push(data.ens_avatar); }
    if (data.label !== undefined) { fields.push(`label = $${idx++}`); params.push(data.label); }
    if (data.is_primary !== undefined) { fields.push(`is_primary = $${idx++}`); params.push(data.is_primary); }
    if (data.is_active !== undefined) { fields.push(`is_active = $${idx++}`); params.push(data.is_active); }
    if (data.metadata !== undefined) { fields.push(`metadata = $${idx++}`); params.push(JSON.stringify(data.metadata)); }

    if (fields.length === 0) return false;
    fields.push('updated_at = NOW()');
    params.push(id, this.sourceAccountId);

    const result = await this.db.query(
      `UPDATE web3_wallets SET ${fields.join(', ')} WHERE id = $${idx++} AND source_account_id = $${idx}`,
      params
    );
    return (result as unknown as { rowCount: number }).rowCount > 0;
  }

  async verifyWallet(id: string, data: VerifyWalletRequest): Promise<boolean> {
    const result = await this.db.query(
      `UPDATE web3_wallets SET verified_at = NOW(), verification_signature = $1, verification_message = $2, updated_at = NOW()
       WHERE id = $3 AND source_account_id = $4`,
      [data.signature, data.message, id, this.sourceAccountId]
    );
    return (result as unknown as { rowCount: number }).rowCount > 0;
  }

  async deleteWallet(id: string): Promise<boolean> {
    const result = await this.db.query(
      `DELETE FROM web3_wallets WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return (result as unknown as { rowCount: number }).rowCount > 0;
  }

  // =========================================================================
  // NFTs
  // =========================================================================

  async createNft(data: CreateNftRequest): Promise<string> {
    const result = await this.db.query(
      `INSERT INTO web3_nfts (source_account_id, contract_address, token_id, chain_id, token_standard,
        owner_address, owner_user_id, quantity, name, description, image_url, animation_url, external_url,
        attributes, metadata_uri, metadata, collection_id, rarity_score, rarity_rank, minted_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
       RETURNING id`,
      [this.sourceAccountId, data.contract_address, data.token_id, data.chain_id, data.token_standard,
       data.owner_address, data.owner_user_id ?? null, data.quantity ?? 1,
       data.name ?? null, data.description ?? null, data.image_url ?? null,
       data.animation_url ?? null, data.external_url ?? null,
       data.attributes ? JSON.stringify(data.attributes) : null,
       data.metadata_uri ?? null, data.metadata ? JSON.stringify(data.metadata) : null,
       data.collection_id ?? null, data.rarity_score ?? null, data.rarity_rank ?? null,
       data.minted_at ?? null]
    );
    return result.rows[0].id as string;
  }

  async getNft(id: string): Promise<NftRecord | null> {
    const result = await this.db.query(
      `SELECT * FROM web3_nfts WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return (result.rows[0] as NftRecord) ?? null;
  }

  async getNftByToken(contractAddress: string, tokenId: string, chainId: number): Promise<NftRecord | null> {
    const result = await this.db.query(
      `SELECT * FROM web3_nfts WHERE contract_address = $1 AND token_id = $2 AND chain_id = $3 AND source_account_id = $4`,
      [contractAddress, tokenId, chainId, this.sourceAccountId]
    );
    return (result.rows[0] as NftRecord) ?? null;
  }

  async listNfts(filters: NftFilters = {}): Promise<NftRecord[]> {
    const conditions = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let idx = 2;

    if (filters.owner_address) { conditions.push(`owner_address = $${idx++}`); params.push(filters.owner_address); }
    if (filters.owner_user_id) { conditions.push(`owner_user_id = $${idx++}`); params.push(filters.owner_user_id); }
    if (filters.collection_id) { conditions.push(`collection_id = $${idx++}`); params.push(filters.collection_id); }
    if (filters.chain_id !== undefined) { conditions.push(`chain_id = $${idx++}`); params.push(filters.chain_id); }
    if (filters.token_standard) { conditions.push(`token_standard = $${idx++}`); params.push(filters.token_standard); }
    if (filters.is_verified !== undefined) { conditions.push(`is_verified = $${idx++}`); params.push(filters.is_verified); }
    if (filters.contract_address) { conditions.push(`contract_address = $${idx++}`); params.push(filters.contract_address); }

    const result = await this.db.query(
      `SELECT * FROM web3_nfts WHERE ${conditions.join(' AND ')} ORDER BY created_at DESC`,
      params
    );
    return result.rows as NftRecord[];
  }

  async updateNft(id: string, data: UpdateNftRequest): Promise<boolean> {
    const fields: string[] = [];
    const params: unknown[] = [];
    let idx = 1;

    if (data.owner_address !== undefined) { fields.push(`owner_address = $${idx++}`); params.push(data.owner_address); }
    if (data.owner_user_id !== undefined) { fields.push(`owner_user_id = $${idx++}`); params.push(data.owner_user_id); }
    if (data.quantity !== undefined) { fields.push(`quantity = $${idx++}`); params.push(data.quantity); }
    if (data.name !== undefined) { fields.push(`name = $${idx++}`); params.push(data.name); }
    if (data.description !== undefined) { fields.push(`description = $${idx++}`); params.push(data.description); }
    if (data.image_url !== undefined) { fields.push(`image_url = $${idx++}`); params.push(data.image_url); }
    if (data.animation_url !== undefined) { fields.push(`animation_url = $${idx++}`); params.push(data.animation_url); }
    if (data.external_url !== undefined) { fields.push(`external_url = $${idx++}`); params.push(data.external_url); }
    if (data.attributes !== undefined) { fields.push(`attributes = $${idx++}`); params.push(JSON.stringify(data.attributes)); }
    if (data.metadata_uri !== undefined) { fields.push(`metadata_uri = $${idx++}`); params.push(data.metadata_uri); }
    if (data.metadata !== undefined) { fields.push(`metadata = $${idx++}`); params.push(JSON.stringify(data.metadata)); }
    if (data.collection_id !== undefined) { fields.push(`collection_id = $${idx++}`); params.push(data.collection_id); }
    if (data.rarity_score !== undefined) { fields.push(`rarity_score = $${idx++}`); params.push(data.rarity_score); }
    if (data.rarity_rank !== undefined) { fields.push(`rarity_rank = $${idx++}`); params.push(data.rarity_rank); }
    if (data.is_verified !== undefined) { fields.push(`is_verified = $${idx++}`); params.push(data.is_verified); }
    if (data.is_spam !== undefined) { fields.push(`is_spam = $${idx++}`); params.push(data.is_spam); }

    if (fields.length === 0) return false;
    fields.push('updated_at = NOW()');
    params.push(id, this.sourceAccountId);

    const result = await this.db.query(
      `UPDATE web3_nfts SET ${fields.join(', ')} WHERE id = $${idx++} AND source_account_id = $${idx}`,
      params
    );
    return (result as unknown as { rowCount: number }).rowCount > 0;
  }

  async deleteNft(id: string): Promise<boolean> {
    const result = await this.db.query(
      `DELETE FROM web3_nfts WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return (result as unknown as { rowCount: number }).rowCount > 0;
  }

  // =========================================================================
  // Collections
  // =========================================================================

  async createCollection(data: CreateCollectionRequest): Promise<string> {
    const result = await this.db.query(
      `INSERT INTO web3_collections (source_account_id, contract_address, chain_id, name, slug, description,
        image_url, banner_url, featured_image_url, website_url, twitter_username, discord_url, telegram_url,
        token_standard, total_supply, workspace_id, is_managed, metadata)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
       RETURNING id`,
      [this.sourceAccountId, data.contract_address, data.chain_id, data.name, data.slug ?? null,
       data.description ?? null, data.image_url ?? null, data.banner_url ?? null,
       data.featured_image_url ?? null, data.website_url ?? null, data.twitter_username ?? null,
       data.discord_url ?? null, data.telegram_url ?? null, data.token_standard ?? null,
       data.total_supply ?? null, data.workspace_id ?? null, data.is_managed ?? false,
       data.metadata ? JSON.stringify(data.metadata) : null]
    );
    return result.rows[0].id as string;
  }

  async getCollection(id: string): Promise<CollectionRecord | null> {
    const result = await this.db.query(
      `SELECT * FROM web3_collections WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return (result.rows[0] as CollectionRecord) ?? null;
  }

  async getCollectionByContract(contractAddress: string, chainId: number): Promise<CollectionRecord | null> {
    const result = await this.db.query(
      `SELECT * FROM web3_collections WHERE contract_address = $1 AND chain_id = $2 AND source_account_id = $3`,
      [contractAddress, chainId, this.sourceAccountId]
    );
    return (result.rows[0] as CollectionRecord) ?? null;
  }

  async getCollectionBySlug(slug: string): Promise<CollectionRecord | null> {
    const result = await this.db.query(
      `SELECT * FROM web3_collections WHERE slug = $1 AND source_account_id = $2`,
      [slug, this.sourceAccountId]
    );
    return (result.rows[0] as CollectionRecord) ?? null;
  }

  async listCollections(filters: CollectionFilters = {}): Promise<CollectionRecord[]> {
    const conditions = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let idx = 2;

    if (filters.chain_id !== undefined) { conditions.push(`chain_id = $${idx++}`); params.push(filters.chain_id); }
    if (filters.token_standard) { conditions.push(`token_standard = $${idx++}`); params.push(filters.token_standard); }
    if (filters.is_verified !== undefined) { conditions.push(`is_verified = $${idx++}`); params.push(filters.is_verified); }
    if (filters.is_managed !== undefined) { conditions.push(`is_managed = $${idx++}`); params.push(filters.is_managed); }
    if (filters.workspace_id) { conditions.push(`workspace_id = $${idx++}`); params.push(filters.workspace_id); }

    const result = await this.db.query(
      `SELECT * FROM web3_collections WHERE ${conditions.join(' AND ')} ORDER BY created_at DESC`,
      params
    );
    return result.rows as CollectionRecord[];
  }

  async updateCollection(id: string, data: UpdateCollectionRequest): Promise<boolean> {
    const fields: string[] = [];
    const params: unknown[] = [];
    let idx = 1;

    if (data.name !== undefined) { fields.push(`name = $${idx++}`); params.push(data.name); }
    if (data.slug !== undefined) { fields.push(`slug = $${idx++}`); params.push(data.slug); }
    if (data.description !== undefined) { fields.push(`description = $${idx++}`); params.push(data.description); }
    if (data.image_url !== undefined) { fields.push(`image_url = $${idx++}`); params.push(data.image_url); }
    if (data.banner_url !== undefined) { fields.push(`banner_url = $${idx++}`); params.push(data.banner_url); }
    if (data.featured_image_url !== undefined) { fields.push(`featured_image_url = $${idx++}`); params.push(data.featured_image_url); }
    if (data.website_url !== undefined) { fields.push(`website_url = $${idx++}`); params.push(data.website_url); }
    if (data.twitter_username !== undefined) { fields.push(`twitter_username = $${idx++}`); params.push(data.twitter_username); }
    if (data.discord_url !== undefined) { fields.push(`discord_url = $${idx++}`); params.push(data.discord_url); }
    if (data.telegram_url !== undefined) { fields.push(`telegram_url = $${idx++}`); params.push(data.telegram_url); }
    if (data.total_supply !== undefined) { fields.push(`total_supply = $${idx++}`); params.push(data.total_supply); }
    if (data.floor_price !== undefined) { fields.push(`floor_price = $${idx++}`); params.push(data.floor_price); }
    if (data.floor_price_currency !== undefined) { fields.push(`floor_price_currency = $${idx++}`); params.push(data.floor_price_currency); }
    if (data.volume_total !== undefined) { fields.push(`volume_total = $${idx++}`); params.push(data.volume_total); }
    if (data.volume_24h !== undefined) { fields.push(`volume_24h = $${idx++}`); params.push(data.volume_24h); }
    if (data.owners_count !== undefined) { fields.push(`owners_count = $${idx++}`); params.push(data.owners_count); }
    if (data.is_verified !== undefined) { fields.push(`is_verified = $${idx++}`); params.push(data.is_verified); }
    if (data.is_spam !== undefined) { fields.push(`is_spam = $${idx++}`); params.push(data.is_spam); }
    if (data.metadata !== undefined) { fields.push(`metadata = $${idx++}`); params.push(JSON.stringify(data.metadata)); }

    if (fields.length === 0) return false;
    fields.push('updated_at = NOW()');
    params.push(id, this.sourceAccountId);

    const result = await this.db.query(
      `UPDATE web3_collections SET ${fields.join(', ')} WHERE id = $${idx++} AND source_account_id = $${idx}`,
      params
    );
    return (result as unknown as { rowCount: number }).rowCount > 0;
  }

  async deleteCollection(id: string): Promise<boolean> {
    const result = await this.db.query(
      `DELETE FROM web3_collections WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return (result as unknown as { rowCount: number }).rowCount > 0;
  }

  // =========================================================================
  // Tokens
  // =========================================================================

  async createToken(data: CreateTokenRequest): Promise<string> {
    const result = await this.db.query(
      `INSERT INTO web3_tokens (source_account_id, contract_address, chain_id, name, symbol, decimals,
        token_type, price_usd, logo_url, website_url, description, metadata)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
       RETURNING id`,
      [this.sourceAccountId, data.contract_address, data.chain_id, data.name, data.symbol, data.decimals,
       data.token_type, data.price_usd ?? null, data.logo_url ?? null, data.website_url ?? null,
       data.description ?? null, data.metadata ? JSON.stringify(data.metadata) : null]
    );
    return result.rows[0].id as string;
  }

  async getToken(id: string): Promise<TokenRecord | null> {
    const result = await this.db.query(
      `SELECT * FROM web3_tokens WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return (result.rows[0] as TokenRecord) ?? null;
  }

  async getTokenByContract(contractAddress: string, chainId: number): Promise<TokenRecord | null> {
    const result = await this.db.query(
      `SELECT * FROM web3_tokens WHERE contract_address = $1 AND chain_id = $2 AND source_account_id = $3`,
      [contractAddress, chainId, this.sourceAccountId]
    );
    return (result.rows[0] as TokenRecord) ?? null;
  }

  async listTokens(filters: TokenFilters = {}): Promise<TokenRecord[]> {
    const conditions = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let idx = 2;

    if (filters.chain_id !== undefined) { conditions.push(`chain_id = $${idx++}`); params.push(filters.chain_id); }
    if (filters.token_type) { conditions.push(`token_type = $${idx++}`); params.push(filters.token_type); }
    if (filters.is_verified !== undefined) { conditions.push(`is_verified = $${idx++}`); params.push(filters.is_verified); }

    const result = await this.db.query(
      `SELECT * FROM web3_tokens WHERE ${conditions.join(' AND ')} ORDER BY created_at DESC`,
      params
    );
    return result.rows as TokenRecord[];
  }

  async updateToken(id: string, data: UpdateTokenRequest): Promise<boolean> {
    const fields: string[] = [];
    const params: unknown[] = [];
    let idx = 1;

    if (data.name !== undefined) { fields.push(`name = $${idx++}`); params.push(data.name); }
    if (data.symbol !== undefined) { fields.push(`symbol = $${idx++}`); params.push(data.symbol); }
    if (data.price_usd !== undefined) { fields.push(`price_usd = $${idx++}`); params.push(data.price_usd); fields.push('price_updated_at = NOW()'); }
    if (data.logo_url !== undefined) { fields.push(`logo_url = $${idx++}`); params.push(data.logo_url); }
    if (data.website_url !== undefined) { fields.push(`website_url = $${idx++}`); params.push(data.website_url); }
    if (data.description !== undefined) { fields.push(`description = $${idx++}`); params.push(data.description); }
    if (data.is_verified !== undefined) { fields.push(`is_verified = $${idx++}`); params.push(data.is_verified); }
    if (data.is_spam !== undefined) { fields.push(`is_spam = $${idx++}`); params.push(data.is_spam); }
    if (data.metadata !== undefined) { fields.push(`metadata = $${idx++}`); params.push(JSON.stringify(data.metadata)); }

    if (fields.length === 0) return false;
    fields.push('updated_at = NOW()');
    params.push(id, this.sourceAccountId);

    const result = await this.db.query(
      `UPDATE web3_tokens SET ${fields.join(', ')} WHERE id = $${idx++} AND source_account_id = $${idx}`,
      params
    );
    return (result as unknown as { rowCount: number }).rowCount > 0;
  }

  async deleteToken(id: string): Promise<boolean> {
    const result = await this.db.query(
      `DELETE FROM web3_tokens WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return (result as unknown as { rowCount: number }).rowCount > 0;
  }

  // =========================================================================
  // Token Balances
  // =========================================================================

  async upsertTokenBalance(data: UpsertTokenBalanceRequest): Promise<string> {
    const result = await this.db.query(
      `INSERT INTO web3_token_balances (source_account_id, wallet_address, user_id, token_id, balance, balance_formatted, value_usd)
       VALUES ($1, $2, $3, $4, $5, $6, $7)
       ON CONFLICT (wallet_address, token_id) DO UPDATE SET
         balance = EXCLUDED.balance,
         balance_formatted = EXCLUDED.balance_formatted,
         value_usd = EXCLUDED.value_usd,
         last_updated_at = NOW()
       RETURNING id`,
      [this.sourceAccountId, data.wallet_address, data.user_id ?? null, data.token_id,
       data.balance, data.balance_formatted ?? null, data.value_usd ?? null]
    );
    return result.rows[0].id as string;
  }

  async getTokenBalance(walletAddress: string, tokenId: string): Promise<TokenBalanceRecord | null> {
    const result = await this.db.query(
      `SELECT * FROM web3_token_balances WHERE wallet_address = $1 AND token_id = $2 AND source_account_id = $3`,
      [walletAddress, tokenId, this.sourceAccountId]
    );
    return (result.rows[0] as TokenBalanceRecord) ?? null;
  }

  async listTokenBalances(walletAddress?: string, userId?: string): Promise<TokenBalanceRecord[]> {
    const conditions = ['tb.source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let idx = 2;

    if (walletAddress) { conditions.push(`tb.wallet_address = $${idx++}`); params.push(walletAddress); }
    if (userId) { conditions.push(`tb.user_id = $${idx++}`); params.push(userId); }

    const result = await this.db.query(
      `SELECT tb.*, t.name AS token_name, t.symbol AS token_symbol, t.decimals AS token_decimals
       FROM web3_token_balances tb
       JOIN web3_tokens t ON tb.token_id = t.id
       WHERE ${conditions.join(' AND ')}
       ORDER BY tb.value_usd DESC NULLS LAST`,
      params
    );
    return result.rows as TokenBalanceRecord[];
  }

  async deleteTokenBalance(id: string): Promise<boolean> {
    const result = await this.db.query(
      `DELETE FROM web3_token_balances WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return (result as unknown as { rowCount: number }).rowCount > 0;
  }

  // =========================================================================
  // Token Gates
  // =========================================================================

  async createTokenGate(data: CreateTokenGateRequest): Promise<string> {
    const result = await this.db.query(
      `INSERT INTO web3_token_gates (source_account_id, workspace_id, created_by, name, description,
        gate_type, rules, target_type, target_id)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
       RETURNING id`,
      [this.sourceAccountId, data.workspace_id, data.created_by, data.name, data.description ?? null,
       data.gate_type, JSON.stringify(data.rules), data.target_type, data.target_id ?? null]
    );
    return result.rows[0].id as string;
  }

  async getTokenGate(id: string): Promise<TokenGateRecord | null> {
    const result = await this.db.query(
      `SELECT * FROM web3_token_gates WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return (result.rows[0] as TokenGateRecord) ?? null;
  }

  async listTokenGates(filters: TokenGateFilters = {}): Promise<TokenGateRecord[]> {
    const conditions = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let idx = 2;

    if (filters.workspace_id) { conditions.push(`workspace_id = $${idx++}`); params.push(filters.workspace_id); }
    if (filters.gate_type) { conditions.push(`gate_type = $${idx++}`); params.push(filters.gate_type); }
    if (filters.target_type) { conditions.push(`target_type = $${idx++}`); params.push(filters.target_type); }
    if (filters.is_active !== undefined) { conditions.push(`is_active = $${idx++}`); params.push(filters.is_active); }

    const result = await this.db.query(
      `SELECT * FROM web3_token_gates WHERE ${conditions.join(' AND ')} ORDER BY created_at DESC`,
      params
    );
    return result.rows as TokenGateRecord[];
  }

  async updateTokenGate(id: string, data: UpdateTokenGateRequest): Promise<boolean> {
    const fields: string[] = [];
    const params: unknown[] = [];
    let idx = 1;

    if (data.name !== undefined) { fields.push(`name = $${idx++}`); params.push(data.name); }
    if (data.description !== undefined) { fields.push(`description = $${idx++}`); params.push(data.description); }
    if (data.gate_type !== undefined) { fields.push(`gate_type = $${idx++}`); params.push(data.gate_type); }
    if (data.rules !== undefined) { fields.push(`rules = $${idx++}`); params.push(JSON.stringify(data.rules)); }
    if (data.target_type !== undefined) { fields.push(`target_type = $${idx++}`); params.push(data.target_type); }
    if (data.target_id !== undefined) { fields.push(`target_id = $${idx++}`); params.push(data.target_id); }
    if (data.is_active !== undefined) { fields.push(`is_active = $${idx++}`); params.push(data.is_active); }

    if (fields.length === 0) return false;
    fields.push('updated_at = NOW()');
    params.push(id, this.sourceAccountId);

    const result = await this.db.query(
      `UPDATE web3_token_gates SET ${fields.join(', ')} WHERE id = $${idx++} AND source_account_id = $${idx}`,
      params
    );
    return (result as unknown as { rowCount: number }).rowCount > 0;
  }

  async deleteTokenGate(id: string): Promise<boolean> {
    const result = await this.db.query(
      `DELETE FROM web3_token_gates WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return (result as unknown as { rowCount: number }).rowCount > 0;
  }

  // =========================================================================
  // Gate Checks
  // =========================================================================

  async createGateCheck(data: CreateGateCheckRequest): Promise<string> {
    const result = await this.db.query(
      `INSERT INTO web3_gate_checks (source_account_id, gate_id, user_id, wallet_address, passed, failure_reason, evidence, expires_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
       RETURNING id`,
      [this.sourceAccountId, data.gate_id, data.user_id, data.wallet_address, data.passed,
       data.failure_reason ?? null, data.evidence ? JSON.stringify(data.evidence) : null,
       data.expires_at ?? null]
    );
    return result.rows[0].id as string;
  }

  async checkGate(gateId: string, userId: string, walletAddress: string, cacheTtl: number): Promise<GateCheckResult> {
    // Check for a cached, non-expired passing check
    const cached = await this.db.query(
      `SELECT gc.*, tg.name AS gate_name FROM web3_gate_checks gc
       JOIN web3_token_gates tg ON gc.gate_id = tg.id
       WHERE gc.gate_id = $1 AND gc.user_id = $2 AND gc.wallet_address = $3
         AND gc.source_account_id = $4 AND gc.expires_at > NOW()
       ORDER BY gc.checked_at DESC LIMIT 1`,
      [gateId, userId, walletAddress, this.sourceAccountId]
    );

    if (cached.rows.length > 0) {
      const row = cached.rows[0] as GateCheckRecord & { gate_name: string };
      return {
        passed: row.passed,
        gate_id: gateId,
        gate_name: row.gate_name,
        failure_reason: row.failure_reason,
        evidence: row.evidence,
        cached: true,
      };
    }

    // Get the gate details
    const gate = await this.getTokenGate(gateId);
    if (!gate) {
      return { passed: false, gate_id: gateId, gate_name: 'Unknown', failure_reason: 'Gate not found', evidence: null, cached: false };
    }

    if (!gate.is_active) {
      return { passed: false, gate_id: gateId, gate_name: gate.name, failure_reason: 'Gate is inactive', evidence: null, cached: false };
    }

    // Perform the check based on gate type
    let passed = false;
    let failureReason: string | null = null;
    let evidence: Record<string, unknown> | null = null;

    switch (gate.gate_type) {
      case 'nft_ownership': {
        const rules = gate.rules as { contract_address: string; chain_id: number; min_quantity?: number };
        const nfts = await this.listNfts({
          owner_address: walletAddress,
          contract_address: rules.contract_address,
          chain_id: rules.chain_id,
        });
        const totalQuantity = nfts.reduce((sum, n) => sum + (n.quantity || 1), 0);
        const minQuantity = rules.min_quantity ?? 1;
        passed = totalQuantity >= minQuantity;
        evidence = { nft_count: nfts.length, total_quantity: totalQuantity, required: minQuantity };
        if (!passed) failureReason = `Insufficient NFTs: has ${totalQuantity}, needs ${minQuantity}`;
        break;
      }
      case 'token_balance': {
        const rules = gate.rules as { token_id: string; min_balance: string };
        const balance = await this.getTokenBalance(walletAddress, rules.token_id);
        const currentBalance = BigInt(balance?.balance ?? '0');
        const minBalance = BigInt(rules.min_balance);
        passed = currentBalance >= minBalance;
        evidence = { current_balance: balance?.balance ?? '0', required_balance: rules.min_balance };
        if (!passed) failureReason = `Insufficient token balance`;
        break;
      }
      default:
        failureReason = `Unsupported gate type: ${gate.gate_type}`;
    }

    // Store the check result with TTL
    const expiresAt = new Date(Date.now() + cacheTtl * 1000).toISOString();
    await this.createGateCheck({
      gate_id: gateId,
      user_id: userId,
      wallet_address: walletAddress,
      passed,
      failure_reason: failureReason ?? undefined,
      evidence: evidence ?? undefined,
      expires_at: expiresAt,
    });

    return {
      passed,
      gate_id: gateId,
      gate_name: gate.name,
      failure_reason: failureReason,
      evidence,
      cached: false,
    };
  }

  async listGateChecks(gateId?: string, userId?: string): Promise<GateCheckRecord[]> {
    const conditions = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let idx = 2;

    if (gateId) { conditions.push(`gate_id = $${idx++}`); params.push(gateId); }
    if (userId) { conditions.push(`user_id = $${idx++}`); params.push(userId); }

    const result = await this.db.query(
      `SELECT * FROM web3_gate_checks WHERE ${conditions.join(' AND ')} ORDER BY checked_at DESC LIMIT 100`,
      params
    );
    return result.rows as GateCheckRecord[];
  }

  // =========================================================================
  // DAOs
  // =========================================================================

  async createDao(data: CreateDaoRequest): Promise<string> {
    const result = await this.db.query(
      `INSERT INTO web3_daos (source_account_id, workspace_id, name, slug, description, chain_id,
        governance_token_address, treasury_address, snapshot_space, governor_address, governor_type,
        proposal_threshold, quorum, voting_delay, voting_period, metadata)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
       RETURNING id`,
      [this.sourceAccountId, data.workspace_id ?? null, data.name, data.slug ?? null,
       data.description ?? null, data.chain_id, data.governance_token_address ?? null,
       data.treasury_address ?? null, data.snapshot_space ?? null, data.governor_address ?? null,
       data.governor_type ?? null, data.proposal_threshold ?? null, data.quorum ?? null,
       data.voting_delay ?? null, data.voting_period ?? null,
       data.metadata ? JSON.stringify(data.metadata) : null]
    );
    return result.rows[0].id as string;
  }

  async getDao(id: string): Promise<DaoRecord | null> {
    const result = await this.db.query(
      `SELECT * FROM web3_daos WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return (result.rows[0] as DaoRecord) ?? null;
  }

  async getDaoBySlug(slug: string): Promise<DaoRecord | null> {
    const result = await this.db.query(
      `SELECT * FROM web3_daos WHERE slug = $1 AND source_account_id = $2`,
      [slug, this.sourceAccountId]
    );
    return (result.rows[0] as DaoRecord) ?? null;
  }

  async listDaos(workspaceId?: string, chainId?: number, isActive?: boolean): Promise<DaoRecord[]> {
    const conditions = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let idx = 2;

    if (workspaceId) { conditions.push(`workspace_id = $${idx++}`); params.push(workspaceId); }
    if (chainId !== undefined) { conditions.push(`chain_id = $${idx++}`); params.push(chainId); }
    if (isActive !== undefined) { conditions.push(`is_active = $${idx++}`); params.push(isActive); }

    const result = await this.db.query(
      `SELECT * FROM web3_daos WHERE ${conditions.join(' AND ')} ORDER BY created_at DESC`,
      params
    );
    return result.rows as DaoRecord[];
  }

  async updateDao(id: string, data: UpdateDaoRequest): Promise<boolean> {
    const fields: string[] = [];
    const params: unknown[] = [];
    let idx = 1;

    if (data.name !== undefined) { fields.push(`name = $${idx++}`); params.push(data.name); }
    if (data.slug !== undefined) { fields.push(`slug = $${idx++}`); params.push(data.slug); }
    if (data.description !== undefined) { fields.push(`description = $${idx++}`); params.push(data.description); }
    if (data.governance_token_address !== undefined) { fields.push(`governance_token_address = $${idx++}`); params.push(data.governance_token_address); }
    if (data.treasury_address !== undefined) { fields.push(`treasury_address = $${idx++}`); params.push(data.treasury_address); }
    if (data.snapshot_space !== undefined) { fields.push(`snapshot_space = $${idx++}`); params.push(data.snapshot_space); }
    if (data.governor_address !== undefined) { fields.push(`governor_address = $${idx++}`); params.push(data.governor_address); }
    if (data.governor_type !== undefined) { fields.push(`governor_type = $${idx++}`); params.push(data.governor_type); }
    if (data.proposal_threshold !== undefined) { fields.push(`proposal_threshold = $${idx++}`); params.push(data.proposal_threshold); }
    if (data.quorum !== undefined) { fields.push(`quorum = $${idx++}`); params.push(data.quorum); }
    if (data.voting_delay !== undefined) { fields.push(`voting_delay = $${idx++}`); params.push(data.voting_delay); }
    if (data.voting_period !== undefined) { fields.push(`voting_period = $${idx++}`); params.push(data.voting_period); }
    if (data.is_active !== undefined) { fields.push(`is_active = $${idx++}`); params.push(data.is_active); }
    if (data.metadata !== undefined) { fields.push(`metadata = $${idx++}`); params.push(JSON.stringify(data.metadata)); }

    if (fields.length === 0) return false;
    fields.push('updated_at = NOW()');
    params.push(id, this.sourceAccountId);

    const result = await this.db.query(
      `UPDATE web3_daos SET ${fields.join(', ')} WHERE id = $${idx++} AND source_account_id = $${idx}`,
      params
    );
    return (result as unknown as { rowCount: number }).rowCount > 0;
  }

  async deleteDao(id: string): Promise<boolean> {
    const result = await this.db.query(
      `DELETE FROM web3_daos WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return (result as unknown as { rowCount: number }).rowCount > 0;
  }

  // =========================================================================
  // Proposals
  // =========================================================================

  async createProposal(data: CreateProposalRequest): Promise<string> {
    const result = await this.db.query(
      `INSERT INTO web3_proposals (source_account_id, dao_id, title, description, proposer_address,
        proposer_user_id, chain_proposal_id, snapshot_proposal_id, status, start_block, end_block,
        start_time, end_time, executable, metadata)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
       RETURNING id`,
      [this.sourceAccountId, data.dao_id, data.title, data.description, data.proposer_address,
       data.proposer_user_id ?? null, data.chain_proposal_id ?? null, data.snapshot_proposal_id ?? null,
       data.status ?? 'pending', data.start_block ?? null, data.end_block ?? null,
       data.start_time ?? null, data.end_time ?? null, data.executable ?? false,
       data.metadata ? JSON.stringify(data.metadata) : null]
    );
    return result.rows[0].id as string;
  }

  async getProposal(id: string): Promise<ProposalRecord | null> {
    const result = await this.db.query(
      `SELECT * FROM web3_proposals WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return (result.rows[0] as ProposalRecord) ?? null;
  }

  async listProposals(filters: ProposalFilters = {}): Promise<ProposalRecord[]> {
    const conditions = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let idx = 2;

    if (filters.dao_id) { conditions.push(`dao_id = $${idx++}`); params.push(filters.dao_id); }
    if (filters.status) { conditions.push(`status = $${idx++}`); params.push(filters.status); }
    if (filters.proposer_address) { conditions.push(`proposer_address = $${idx++}`); params.push(filters.proposer_address); }

    const result = await this.db.query(
      `SELECT * FROM web3_proposals WHERE ${conditions.join(' AND ')} ORDER BY created_at DESC`,
      params
    );
    return result.rows as ProposalRecord[];
  }

  async updateProposal(id: string, data: UpdateProposalRequest): Promise<boolean> {
    const fields: string[] = [];
    const params: unknown[] = [];
    let idx = 1;

    if (data.title !== undefined) { fields.push(`title = $${idx++}`); params.push(data.title); }
    if (data.description !== undefined) { fields.push(`description = $${idx++}`); params.push(data.description); }
    if (data.status !== undefined) { fields.push(`status = $${idx++}`); params.push(data.status); }
    if (data.votes_for !== undefined) { fields.push(`votes_for = $${idx++}`); params.push(data.votes_for); }
    if (data.votes_against !== undefined) { fields.push(`votes_against = $${idx++}`); params.push(data.votes_against); }
    if (data.votes_abstain !== undefined) { fields.push(`votes_abstain = $${idx++}`); params.push(data.votes_abstain); }
    if (data.executable !== undefined) { fields.push(`executable = $${idx++}`); params.push(data.executable); }
    if (data.executed_at !== undefined) { fields.push(`executed_at = $${idx++}`); params.push(data.executed_at); }
    if (data.execution_tx_hash !== undefined) { fields.push(`execution_tx_hash = $${idx++}`); params.push(data.execution_tx_hash); }
    if (data.metadata !== undefined) { fields.push(`metadata = $${idx++}`); params.push(JSON.stringify(data.metadata)); }

    if (fields.length === 0) return false;
    fields.push('updated_at = NOW()');
    params.push(id, this.sourceAccountId);

    const result = await this.db.query(
      `UPDATE web3_proposals SET ${fields.join(', ')} WHERE id = $${idx++} AND source_account_id = $${idx}`,
      params
    );
    return (result as unknown as { rowCount: number }).rowCount > 0;
  }

  async deleteProposal(id: string): Promise<boolean> {
    const result = await this.db.query(
      `DELETE FROM web3_proposals WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return (result as unknown as { rowCount: number }).rowCount > 0;
  }

  // =========================================================================
  // Votes
  // =========================================================================

  async createVote(data: CreateVoteRequest): Promise<string> {
    const result = await this.db.query(
      `INSERT INTO web3_votes (source_account_id, proposal_id, voter_address, voter_user_id, support,
        voting_power, reason, transaction_hash, block_number)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
       RETURNING id`,
      [this.sourceAccountId, data.proposal_id, data.voter_address, data.voter_user_id ?? null,
       data.support, data.voting_power, data.reason ?? null, data.transaction_hash ?? null,
       data.block_number ?? null]
    );
    return result.rows[0].id as string;
  }

  async listVotes(proposalId?: string, voterAddress?: string): Promise<VoteRecord[]> {
    const conditions = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let idx = 2;

    if (proposalId) { conditions.push(`proposal_id = $${idx++}`); params.push(proposalId); }
    if (voterAddress) { conditions.push(`voter_address = $${idx++}`); params.push(voterAddress); }

    const result = await this.db.query(
      `SELECT * FROM web3_votes WHERE ${conditions.join(' AND ')} ORDER BY voted_at DESC`,
      params
    );
    return result.rows as VoteRecord[];
  }

  async getVote(id: string): Promise<VoteRecord | null> {
    const result = await this.db.query(
      `SELECT * FROM web3_votes WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return (result.rows[0] as VoteRecord) ?? null;
  }

  // =========================================================================
  // Transactions
  // =========================================================================

  async createTransaction(data: CreateTransactionRequest): Promise<string> {
    const result = await this.db.query(
      `INSERT INTO web3_transactions (source_account_id, transaction_hash, chain_id, from_address, to_address,
        from_user_id, to_user_id, value, value_usd, gas_used, gas_price, block_number, block_timestamp,
        status, transaction_type, input_data, metadata)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
       RETURNING id`,
      [this.sourceAccountId, data.transaction_hash, data.chain_id, data.from_address,
       data.to_address ?? null, data.from_user_id ?? null, data.to_user_id ?? null,
       data.value, data.value_usd ?? null, data.gas_used ?? null, data.gas_price ?? null,
       data.block_number ?? null, data.block_timestamp ?? null, data.status ?? null,
       data.transaction_type ?? null, data.input_data ?? null,
       data.metadata ? JSON.stringify(data.metadata) : null]
    );
    return result.rows[0].id as string;
  }

  async getTransaction(id: string): Promise<TransactionRecord | null> {
    const result = await this.db.query(
      `SELECT * FROM web3_transactions WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return (result.rows[0] as TransactionRecord) ?? null;
  }

  async getTransactionByHash(hash: string, chainId: number): Promise<TransactionRecord | null> {
    const result = await this.db.query(
      `SELECT * FROM web3_transactions WHERE transaction_hash = $1 AND chain_id = $2 AND source_account_id = $3`,
      [hash, chainId, this.sourceAccountId]
    );
    return (result.rows[0] as TransactionRecord) ?? null;
  }

  async listTransactions(filters: TransactionFilters = {}, limit = 100, offset = 0): Promise<TransactionRecord[]> {
    const conditions = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let idx = 2;

    if (filters.chain_id !== undefined) { conditions.push(`chain_id = $${idx++}`); params.push(filters.chain_id); }
    if (filters.from_address) { conditions.push(`from_address = $${idx++}`); params.push(filters.from_address); }
    if (filters.to_address) { conditions.push(`to_address = $${idx++}`); params.push(filters.to_address); }
    if (filters.from_user_id) { conditions.push(`from_user_id = $${idx++}`); params.push(filters.from_user_id); }
    if (filters.to_user_id) { conditions.push(`to_user_id = $${idx++}`); params.push(filters.to_user_id); }
    if (filters.status) { conditions.push(`status = $${idx++}`); params.push(filters.status); }
    if (filters.transaction_type) { conditions.push(`transaction_type = $${idx++}`); params.push(filters.transaction_type); }

    params.push(limit, offset);

    const result = await this.db.query(
      `SELECT * FROM web3_transactions WHERE ${conditions.join(' AND ')} ORDER BY created_at DESC LIMIT $${idx++} OFFSET $${idx}`,
      params
    );
    return result.rows as TransactionRecord[];
  }

  // =========================================================================
  // Events
  // =========================================================================

  async createEvent(data: CreateWeb3EventRequest): Promise<string> {
    const result = await this.db.query(
      `INSERT INTO web3_events (source_account_id, event_name, contract_address, chain_id, transaction_hash,
        log_index, block_number, block_timestamp, event_data, decoded_data, related_nft_id, related_user_id)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
       RETURNING id`,
      [this.sourceAccountId, data.event_name, data.contract_address, data.chain_id,
       data.transaction_hash, data.log_index, data.block_number, data.block_timestamp ?? null,
       JSON.stringify(data.event_data), data.decoded_data ? JSON.stringify(data.decoded_data) : null,
       data.related_nft_id ?? null, data.related_user_id ?? null]
    );
    return result.rows[0].id as string;
  }

  async getEvent(id: string): Promise<Web3EventRecord | null> {
    const result = await this.db.query(
      `SELECT * FROM web3_events WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return (result.rows[0] as Web3EventRecord) ?? null;
  }

  async listEvents(filters: Web3EventFilters = {}, limit = 100, offset = 0): Promise<Web3EventRecord[]> {
    const conditions = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let idx = 2;

    if (filters.contract_address) { conditions.push(`contract_address = $${idx++}`); params.push(filters.contract_address); }
    if (filters.chain_id !== undefined) { conditions.push(`chain_id = $${idx++}`); params.push(filters.chain_id); }
    if (filters.event_name) { conditions.push(`event_name = $${idx++}`); params.push(filters.event_name); }
    if (filters.related_nft_id) { conditions.push(`related_nft_id = $${idx++}`); params.push(filters.related_nft_id); }

    params.push(limit, offset);

    const result = await this.db.query(
      `SELECT * FROM web3_events WHERE ${conditions.join(' AND ')} ORDER BY block_number DESC, log_index DESC LIMIT $${idx++} OFFSET $${idx}`,
      params
    );
    return result.rows as Web3EventRecord[];
  }

  // =========================================================================
  // Stats
  // =========================================================================

  async getStats(): Promise<Web3Stats> {
    const result = await this.db.query(
      `SELECT
        (SELECT COUNT(*) FROM web3_wallets WHERE source_account_id = $1)::int AS total_wallets,
        (SELECT COUNT(*) FROM web3_wallets WHERE source_account_id = $1 AND verified_at IS NOT NULL)::int AS verified_wallets,
        (SELECT COUNT(*) FROM web3_nfts WHERE source_account_id = $1)::int AS total_nfts,
        (SELECT COUNT(*) FROM web3_collections WHERE source_account_id = $1)::int AS total_collections,
        (SELECT COUNT(*) FROM web3_collections WHERE source_account_id = $1 AND is_verified = true)::int AS verified_collections,
        (SELECT COUNT(*) FROM web3_tokens WHERE source_account_id = $1)::int AS total_tokens,
        (SELECT COUNT(*) FROM web3_token_gates WHERE source_account_id = $1 AND is_active = true)::int AS active_token_gates,
        (SELECT COUNT(*) FROM web3_gate_checks WHERE source_account_id = $1)::int AS total_gate_checks,
        (SELECT COUNT(*) FROM web3_gate_checks WHERE source_account_id = $1 AND passed = true)::int AS passed_gate_checks,
        (SELECT COUNT(*) FROM web3_daos WHERE source_account_id = $1)::int AS total_daos,
        (SELECT COUNT(*) FROM web3_proposals WHERE source_account_id = $1 AND status = 'active')::int AS active_proposals,
        (SELECT COUNT(*) FROM web3_votes WHERE source_account_id = $1)::int AS total_votes,
        (SELECT COUNT(*) FROM web3_transactions WHERE source_account_id = $1)::int AS total_transactions,
        (SELECT COUNT(*) FROM web3_events WHERE source_account_id = $1)::int AS total_events`,
      [this.sourceAccountId]
    );
    return result.rows[0] as Web3Stats;
  }
}
