/**
 * Web3 Plugin Types
 * Interfaces for blockchain integration, NFTs, token gating, and DAO governance
 */

// =========================================================================
// Union Types
// =========================================================================

export type WalletType = 'eoa' | 'contract' | 'multisig' | 'safe';

export type TokenStandard = 'ERC-721' | 'ERC-1155' | 'ERC-721A' | 'other';

export type TokenType = 'ERC-20' | 'native';

export type GateType = 'nft_ownership' | 'token_balance' | 'token_combination' | 'custom';

export type GateTargetType = 'channel' | 'feature' | 'content' | 'role';

export type GovernorType = 'compound' | 'openzeppelin' | 'custom';

export type ProposalStatus = 'pending' | 'active' | 'canceled' | 'defeated' | 'succeeded' | 'queued' | 'expired' | 'executed';

export type VoteSupport = 'for' | 'against' | 'abstain';

export type TransactionStatus = 'pending' | 'confirmed' | 'failed';

export type TransactionType = 'transfer' | 'contract_interaction' | 'nft_transfer' | 'token_transfer' | 'swap';

// =========================================================================
// Database Records
// =========================================================================

export interface WalletRecord {
  id: string;
  source_account_id: string;
  user_id: string;
  workspace_id: string | null;
  address: string;
  chain_id: number;
  chain_name: string;
  wallet_type: WalletType | null;
  ens_name: string | null;
  ens_avatar: string | null;
  label: string | null;
  is_primary: boolean;
  verified_at: string | null;
  verification_signature: string | null;
  verification_message: string | null;
  is_active: boolean;
  metadata: Record<string, unknown> | null;
  created_at: string;
  updated_at: string;
  [key: string]: unknown;
}

export interface NftRecord {
  id: string;
  source_account_id: string;
  contract_address: string;
  token_id: string;
  chain_id: number;
  token_standard: TokenStandard;
  owner_address: string;
  owner_user_id: string | null;
  quantity: number;
  name: string | null;
  description: string | null;
  image_url: string | null;
  animation_url: string | null;
  external_url: string | null;
  attributes: Record<string, unknown> | null;
  metadata_uri: string | null;
  metadata: Record<string, unknown> | null;
  collection_id: string | null;
  rarity_score: number | null;
  rarity_rank: number | null;
  is_verified: boolean;
  is_spam: boolean;
  last_indexed_at: string | null;
  block_number: number | null;
  minted_at: string | null;
  created_at: string;
  updated_at: string;
  [key: string]: unknown;
}

export interface CollectionRecord {
  id: string;
  source_account_id: string;
  contract_address: string;
  chain_id: number;
  name: string;
  slug: string | null;
  description: string | null;
  image_url: string | null;
  banner_url: string | null;
  featured_image_url: string | null;
  website_url: string | null;
  twitter_username: string | null;
  discord_url: string | null;
  telegram_url: string | null;
  token_standard: TokenStandard | null;
  total_supply: number | null;
  floor_price: number | null;
  floor_price_currency: string | null;
  volume_total: number | null;
  volume_24h: number | null;
  owners_count: number | null;
  is_verified: boolean;
  is_spam: boolean;
  workspace_id: string | null;
  is_managed: boolean;
  metadata: Record<string, unknown> | null;
  last_indexed_at: string | null;
  created_at: string;
  updated_at: string;
  [key: string]: unknown;
}

export interface TokenRecord {
  id: string;
  source_account_id: string;
  contract_address: string;
  chain_id: number;
  name: string;
  symbol: string;
  decimals: number;
  token_type: TokenType;
  price_usd: number | null;
  price_updated_at: string | null;
  logo_url: string | null;
  website_url: string | null;
  description: string | null;
  is_verified: boolean;
  is_spam: boolean;
  metadata: Record<string, unknown> | null;
  created_at: string;
  updated_at: string;
  [key: string]: unknown;
}

export interface TokenBalanceRecord {
  id: string;
  source_account_id: string;
  wallet_address: string;
  user_id: string | null;
  token_id: string;
  balance: string;
  balance_formatted: number | null;
  value_usd: number | null;
  last_updated_at: string;
  created_at: string;
  [key: string]: unknown;
}

export interface TokenGateRecord {
  id: string;
  source_account_id: string;
  workspace_id: string;
  created_by: string;
  name: string;
  description: string | null;
  gate_type: GateType;
  rules: Record<string, unknown>;
  target_type: GateTargetType;
  target_id: string | null;
  is_active: boolean;
  created_at: string;
  updated_at: string;
  [key: string]: unknown;
}

export interface GateCheckRecord {
  id: string;
  source_account_id: string;
  gate_id: string;
  user_id: string;
  wallet_address: string;
  passed: boolean;
  failure_reason: string | null;
  evidence: Record<string, unknown> | null;
  expires_at: string | null;
  checked_at: string;
  [key: string]: unknown;
}

export interface DaoRecord {
  id: string;
  source_account_id: string;
  workspace_id: string | null;
  name: string;
  slug: string | null;
  description: string | null;
  chain_id: number;
  governance_token_address: string | null;
  treasury_address: string | null;
  snapshot_space: string | null;
  governor_address: string | null;
  governor_type: GovernorType | null;
  proposal_threshold: string | null;
  quorum: string | null;
  voting_delay: number | null;
  voting_period: number | null;
  is_active: boolean;
  metadata: Record<string, unknown> | null;
  created_at: string;
  updated_at: string;
  [key: string]: unknown;
}

export interface ProposalRecord {
  id: string;
  source_account_id: string;
  dao_id: string;
  title: string;
  description: string;
  proposer_address: string;
  proposer_user_id: string | null;
  chain_proposal_id: string | null;
  snapshot_proposal_id: string | null;
  status: ProposalStatus;
  start_block: number | null;
  end_block: number | null;
  start_time: string | null;
  end_time: string | null;
  votes_for: string;
  votes_against: string;
  votes_abstain: string;
  executable: boolean;
  executed_at: string | null;
  execution_tx_hash: string | null;
  metadata: Record<string, unknown> | null;
  created_at: string;
  updated_at: string;
  [key: string]: unknown;
}

export interface VoteRecord {
  id: string;
  source_account_id: string;
  proposal_id: string;
  voter_address: string;
  voter_user_id: string | null;
  support: VoteSupport;
  voting_power: string;
  reason: string | null;
  transaction_hash: string | null;
  block_number: number | null;
  voted_at: string;
  [key: string]: unknown;
}

export interface TransactionRecord {
  id: string;
  source_account_id: string;
  transaction_hash: string;
  chain_id: number;
  from_address: string;
  to_address: string | null;
  from_user_id: string | null;
  to_user_id: string | null;
  value: string;
  value_usd: number | null;
  gas_used: number | null;
  gas_price: string | null;
  block_number: number | null;
  block_timestamp: string | null;
  status: TransactionStatus | null;
  transaction_type: TransactionType | null;
  input_data: string | null;
  metadata: Record<string, unknown> | null;
  created_at: string;
  [key: string]: unknown;
}

export interface Web3EventRecord {
  id: string;
  source_account_id: string;
  event_name: string;
  contract_address: string;
  chain_id: number;
  transaction_hash: string;
  log_index: number;
  block_number: number;
  block_timestamp: string | null;
  event_data: Record<string, unknown>;
  decoded_data: Record<string, unknown> | null;
  related_nft_id: string | null;
  related_user_id: string | null;
  created_at: string;
  [key: string]: unknown;
}

// =========================================================================
// Request Types
// =========================================================================

export interface CreateWalletRequest {
  user_id: string;
  workspace_id?: string;
  address: string;
  chain_id: number;
  chain_name: string;
  wallet_type?: WalletType;
  ens_name?: string;
  ens_avatar?: string;
  label?: string;
  is_primary?: boolean;
  metadata?: Record<string, unknown>;
}

export interface UpdateWalletRequest {
  ens_name?: string;
  ens_avatar?: string;
  label?: string;
  is_primary?: boolean;
  is_active?: boolean;
  metadata?: Record<string, unknown>;
}

export interface VerifyWalletRequest {
  signature: string;
  message: string;
}

export interface CreateNftRequest {
  contract_address: string;
  token_id: string;
  chain_id: number;
  token_standard: TokenStandard;
  owner_address: string;
  owner_user_id?: string;
  quantity?: number;
  name?: string;
  description?: string;
  image_url?: string;
  animation_url?: string;
  external_url?: string;
  attributes?: Record<string, unknown>;
  metadata_uri?: string;
  metadata?: Record<string, unknown>;
  collection_id?: string;
  rarity_score?: number;
  rarity_rank?: number;
  minted_at?: string;
}

export interface UpdateNftRequest {
  owner_address?: string;
  owner_user_id?: string;
  quantity?: number;
  name?: string;
  description?: string;
  image_url?: string;
  animation_url?: string;
  external_url?: string;
  attributes?: Record<string, unknown>;
  metadata_uri?: string;
  metadata?: Record<string, unknown>;
  collection_id?: string;
  rarity_score?: number;
  rarity_rank?: number;
  is_verified?: boolean;
  is_spam?: boolean;
}

export interface CreateCollectionRequest {
  contract_address: string;
  chain_id: number;
  name: string;
  slug?: string;
  description?: string;
  image_url?: string;
  banner_url?: string;
  featured_image_url?: string;
  website_url?: string;
  twitter_username?: string;
  discord_url?: string;
  telegram_url?: string;
  token_standard?: TokenStandard;
  total_supply?: number;
  workspace_id?: string;
  is_managed?: boolean;
  metadata?: Record<string, unknown>;
}

export interface UpdateCollectionRequest {
  name?: string;
  slug?: string;
  description?: string;
  image_url?: string;
  banner_url?: string;
  featured_image_url?: string;
  website_url?: string;
  twitter_username?: string;
  discord_url?: string;
  telegram_url?: string;
  total_supply?: number;
  floor_price?: number;
  floor_price_currency?: string;
  volume_total?: number;
  volume_24h?: number;
  owners_count?: number;
  is_verified?: boolean;
  is_spam?: boolean;
  metadata?: Record<string, unknown>;
}

export interface CreateTokenRequest {
  contract_address: string;
  chain_id: number;
  name: string;
  symbol: string;
  decimals: number;
  token_type: TokenType;
  price_usd?: number;
  logo_url?: string;
  website_url?: string;
  description?: string;
  metadata?: Record<string, unknown>;
}

export interface UpdateTokenRequest {
  name?: string;
  symbol?: string;
  price_usd?: number;
  logo_url?: string;
  website_url?: string;
  description?: string;
  is_verified?: boolean;
  is_spam?: boolean;
  metadata?: Record<string, unknown>;
}

export interface UpsertTokenBalanceRequest {
  wallet_address: string;
  user_id?: string;
  token_id: string;
  balance: string;
  balance_formatted?: number;
  value_usd?: number;
}

export interface CreateTokenGateRequest {
  workspace_id: string;
  created_by: string;
  name: string;
  description?: string;
  gate_type: GateType;
  rules: Record<string, unknown>;
  target_type: GateTargetType;
  target_id?: string;
}

export interface UpdateTokenGateRequest {
  name?: string;
  description?: string;
  gate_type?: GateType;
  rules?: Record<string, unknown>;
  target_type?: GateTargetType;
  target_id?: string;
  is_active?: boolean;
}

export interface CreateGateCheckRequest {
  gate_id: string;
  user_id: string;
  wallet_address: string;
  passed: boolean;
  failure_reason?: string;
  evidence?: Record<string, unknown>;
  expires_at?: string;
}

export interface CreateDaoRequest {
  workspace_id?: string;
  name: string;
  slug?: string;
  description?: string;
  chain_id: number;
  governance_token_address?: string;
  treasury_address?: string;
  snapshot_space?: string;
  governor_address?: string;
  governor_type?: GovernorType;
  proposal_threshold?: string;
  quorum?: string;
  voting_delay?: number;
  voting_period?: number;
  metadata?: Record<string, unknown>;
}

export interface UpdateDaoRequest {
  name?: string;
  slug?: string;
  description?: string;
  governance_token_address?: string;
  treasury_address?: string;
  snapshot_space?: string;
  governor_address?: string;
  governor_type?: GovernorType;
  proposal_threshold?: string;
  quorum?: string;
  voting_delay?: number;
  voting_period?: number;
  is_active?: boolean;
  metadata?: Record<string, unknown>;
}

export interface CreateProposalRequest {
  dao_id: string;
  title: string;
  description: string;
  proposer_address: string;
  proposer_user_id?: string;
  chain_proposal_id?: string;
  snapshot_proposal_id?: string;
  status?: ProposalStatus;
  start_block?: number;
  end_block?: number;
  start_time?: string;
  end_time?: string;
  executable?: boolean;
  metadata?: Record<string, unknown>;
}

export interface UpdateProposalRequest {
  title?: string;
  description?: string;
  status?: ProposalStatus;
  votes_for?: string;
  votes_against?: string;
  votes_abstain?: string;
  executable?: boolean;
  executed_at?: string;
  execution_tx_hash?: string;
  metadata?: Record<string, unknown>;
}

export interface CreateVoteRequest {
  proposal_id: string;
  voter_address: string;
  voter_user_id?: string;
  support: VoteSupport;
  voting_power: string;
  reason?: string;
  transaction_hash?: string;
  block_number?: number;
}

export interface CreateTransactionRequest {
  transaction_hash: string;
  chain_id: number;
  from_address: string;
  to_address?: string;
  from_user_id?: string;
  to_user_id?: string;
  value: string;
  value_usd?: number;
  gas_used?: number;
  gas_price?: string;
  block_number?: number;
  block_timestamp?: string;
  status?: TransactionStatus;
  transaction_type?: TransactionType;
  input_data?: string;
  metadata?: Record<string, unknown>;
}

export interface CreateWeb3EventRequest {
  event_name: string;
  contract_address: string;
  chain_id: number;
  transaction_hash: string;
  log_index: number;
  block_number: number;
  block_timestamp?: string;
  event_data: Record<string, unknown>;
  decoded_data?: Record<string, unknown>;
  related_nft_id?: string;
  related_user_id?: string;
}

// =========================================================================
// Query / Filter Types
// =========================================================================

export interface WalletFilters {
  user_id?: string;
  workspace_id?: string;
  chain_id?: number;
  is_active?: boolean;
}

export interface NftFilters {
  owner_address?: string;
  owner_user_id?: string;
  collection_id?: string;
  chain_id?: number;
  token_standard?: TokenStandard;
  is_verified?: boolean;
  contract_address?: string;
}

export interface CollectionFilters {
  chain_id?: number;
  token_standard?: TokenStandard;
  is_verified?: boolean;
  is_managed?: boolean;
  workspace_id?: string;
}

export interface TokenFilters {
  chain_id?: number;
  token_type?: TokenType;
  is_verified?: boolean;
}

export interface TokenGateFilters {
  workspace_id?: string;
  gate_type?: GateType;
  target_type?: GateTargetType;
  is_active?: boolean;
}

export interface ProposalFilters {
  dao_id?: string;
  status?: ProposalStatus;
  proposer_address?: string;
}

export interface TransactionFilters {
  chain_id?: number;
  from_address?: string;
  to_address?: string;
  from_user_id?: string;
  to_user_id?: string;
  status?: TransactionStatus;
  transaction_type?: TransactionType;
}

export interface Web3EventFilters {
  contract_address?: string;
  chain_id?: number;
  event_name?: string;
  related_nft_id?: string;
}

// =========================================================================
// Result Types
// =========================================================================

export interface GateCheckResult {
  passed: boolean;
  gate_id: string;
  gate_name: string;
  failure_reason: string | null;
  evidence: Record<string, unknown> | null;
  cached: boolean;
}

export interface Web3Stats {
  total_wallets: number;
  verified_wallets: number;
  total_nfts: number;
  total_collections: number;
  verified_collections: number;
  total_tokens: number;
  active_token_gates: number;
  total_gate_checks: number;
  passed_gate_checks: number;
  total_daos: number;
  active_proposals: number;
  total_votes: number;
  total_transactions: number;
  total_events: number;
  [key: string]: unknown;
}
