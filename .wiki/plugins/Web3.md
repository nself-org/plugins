# Web3 Plugin

Blockchain integration, NFT support, token-gated access, DAO governance, and decentralized identity

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Web3 plugin provides comprehensive blockchain integration for the nself platform. It enables wallet management, NFT tracking, token-gated access control, DAO governance, and blockchain event monitoring across multiple chains.

### Key Features

- **Multi-Chain Support** - Ethereum, Polygon, Arbitrum, Optimism, Base, and more
- **Wallet Management** - Connect, verify, and manage Web3 wallets with ENS support
- **NFT Tracking** - Track ERC-721, ERC-1155, and ERC-721A NFT ownership
- **Collection Management** - Index and manage NFT collections with metadata
- **Token Balances** - Track ERC-20 token balances with real-time prices
- **Token Gating** - Create rules for NFT ownership and token balance access control
- **DAO Governance** - Manage DAOs, proposals, and voting
- **Transaction Tracking** - Monitor blockchain transactions
- **Event Logging** - Capture and index blockchain events
- **Multi-Account Support** - `source_account_id` isolation for multi-workspace deployments

### Supported Blockchains

| Chain | Chain ID | Status |
|-------|----------|--------|
| Ethereum Mainnet | 1 | Supported |
| Polygon | 137 | Supported |
| Arbitrum One | 42161 | Supported |
| Optimism | 10 | Supported |
| Base | 8453 | Supported |

---

## Quick Start

```bash
# Install the plugin
nself plugin install web3

# Set required environment variables
export DATABASE_URL="postgresql://user:pass@localhost:5432/nself"
export WEB3_PLUGIN_PORT=3715

# Initialize database schema
nself plugin web3 init

# Start the server
nself plugin web3 server --port 3715

# Check status
nself plugin web3 status
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `WEB3_PLUGIN_PORT` | No | `3715` | HTTP server port |
| `WEB3_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server host |
| `WEB3_DEFAULT_CHAIN_ID` | No | `1` | Default blockchain chain ID (1 = Ethereum) |
| `WEB3_SUPPORTED_CHAINS` | No | `1,137,42161,10,8453` | Comma-separated chain IDs |
| `WEB3_GATE_CHECK_CACHE_TTL` | No | `300` | Token gate check cache TTL (seconds) |
| `WEB3_API_KEY` | No | - | API key for authentication (optional) |
| `WEB3_RATE_LIMIT_MAX` | No | `500` | Maximum requests per window |
| `WEB3_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window (milliseconds) |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | - | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

### Example .env File

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_DB=nself
POSTGRES_USER=nself
POSTGRES_PASSWORD=secure_password
POSTGRES_SSL=false

# Server
WEB3_PLUGIN_PORT=3715
WEB3_PLUGIN_HOST=0.0.0.0

# Web3 Configuration
WEB3_DEFAULT_CHAIN_ID=1
WEB3_SUPPORTED_CHAINS=1,137,42161,10,8453
WEB3_GATE_CHECK_CACHE_TTL=300

# Security (optional)
WEB3_API_KEY=your_api_key_here
WEB3_RATE_LIMIT_MAX=500
WEB3_RATE_LIMIT_WINDOW_MS=60000

# Logging
LOG_LEVEL=info
```

---

## CLI Commands

### Plugin Management

```bash
# Initialize database schema
nself plugin web3 init

# Start the server
nself plugin web3 server
nself plugin web3 server --port 3715 --host 0.0.0.0

# Check status and statistics
nself plugin web3 status
```

### Wallet Commands

```bash
# List all wallets
nself plugin web3 wallets list

# List wallets for a specific user
nself plugin web3 wallets list --user user123

# Filter by chain
nself plugin web3 wallets list --chain 1

# Get wallet by ID
nself plugin web3 wallets get --id <wallet-uuid>

# Get wallet by address
nself plugin web3 wallets get --address 0x1234... --chain 1
```

### NFT Commands

```bash
# List all NFTs
nself plugin web3 nfts list

# List NFTs owned by address
nself plugin web3 nfts list --owner 0x1234...

# Filter by collection
nself plugin web3 nfts list --collection <collection-uuid>

# Filter by chain
nself plugin web3 nfts list --chain 1

# Filter by token standard
nself plugin web3 nfts list --standard ERC-721

# Get NFT by ID
nself plugin web3 nfts get --id <nft-uuid>
```

### Collection Commands

```bash
# List all collections
nself plugin web3 collections list

# List verified collections only
nself plugin web3 collections list --verified

# Filter by chain
nself plugin web3 collections list --chain 1

# Get collection by ID
nself plugin web3 collections get --id <collection-uuid>

# Get collection by slug
nself plugin web3 collections get --slug bored-ape-yacht-club
```

### Token Gate Commands

```bash
# List all token gates
nself plugin web3 gates list

# Filter by workspace
nself plugin web3 gates list --workspace workspace123

# Filter by gate type
nself plugin web3 gates list --type nft_ownership

# Get gate by ID
nself plugin web3 gates get --id <gate-uuid>

# Check if user passes gate
nself plugin web3 gates check --id <gate-uuid> --user user123 --wallet 0x1234...
```

### DAO Commands

```bash
# List all DAOs
nself plugin web3 daos list

# Filter by workspace
nself plugin web3 daos list --workspace workspace123

# Filter by chain
nself plugin web3 daos list --chain 1

# Get DAO by ID
nself plugin web3 daos get --id <dao-uuid>

# Get DAO by slug
nself plugin web3 daos get --slug compound
```

### Proposal Commands

```bash
# List all proposals
nself plugin web3 proposals list

# Filter by DAO
nself plugin web3 proposals list --dao <dao-uuid>

# Filter by status
nself plugin web3 proposals list --status active

# Get proposal by ID
nself plugin web3 proposals get --id <proposal-uuid>
```

### Transaction Commands

```bash
# List all transactions
nself plugin web3 transactions list

# Limit results
nself plugin web3 transactions list --limit 100

# Filter by chain
nself plugin web3 transactions list --chain 1

# Filter by from address
nself plugin web3 transactions list --from 0x1234...

# Filter by to address
nself plugin web3 transactions list --to 0x5678...

# Filter by type
nself plugin web3 transactions list --type token_transfer

# Get transaction by ID
nself plugin web3 transactions get --id <tx-uuid>

# Get transaction by hash
nself plugin web3 transactions get --hash 0xabc123... --chain 1
```

---

## REST API

### Base URL

```
http://localhost:3715
```

### Health & Status

#### GET /health
Health check endpoint.

**Response:**
```json
{
  "status": "ok",
  "plugin": "web3",
  "timestamp": "2026-02-11T12:00:00.000Z"
}
```

#### GET /ready
Readiness check (includes database connectivity).

**Response:**
```json
{
  "ready": true,
  "plugin": "web3",
  "timestamp": "2026-02-11T12:00:00.000Z"
}
```

#### GET /live
Liveness check with statistics.

**Response:**
```json
{
  "alive": true,
  "plugin": "web3",
  "version": "1.0.0",
  "uptime": 3600.5,
  "stats": {
    "total_wallets": 150,
    "verified_wallets": 120,
    "total_nfts": 5000,
    "total_collections": 50,
    "verified_collections": 30,
    "total_tokens": 100,
    "active_token_gates": 10,
    "total_gate_checks": 500,
    "passed_gate_checks": 450,
    "total_daos": 5,
    "active_proposals": 3,
    "total_votes": 250,
    "total_transactions": 10000,
    "total_events": 50000
  },
  "timestamp": "2026-02-11T12:00:00.000Z"
}
```

#### GET /v1/status
Plugin status and statistics.

**Response:**
```json
{
  "plugin": "web3",
  "version": "1.0.0",
  "status": "running",
  "stats": { /* same as /live */ },
  "timestamp": "2026-02-11T12:00:00.000Z"
}
```

### Wallets

#### GET /api/web3/wallets
List wallets with optional filters.

**Query Parameters:**
- `user_id` - Filter by user ID
- `workspace_id` - Filter by workspace ID
- `chain_id` - Filter by chain ID
- `is_active` - Filter by active status (true/false)

**Response:**
```json
{
  "data": [
    {
      "id": "uuid",
      "source_account_id": "primary",
      "user_id": "user123",
      "workspace_id": "workspace456",
      "address": "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb",
      "chain_id": 1,
      "chain_name": "Ethereum Mainnet",
      "wallet_type": "eoa",
      "ens_name": "vitalik.eth",
      "ens_avatar": "https://...",
      "label": "My Primary Wallet",
      "is_primary": true,
      "verified_at": "2026-01-15T10:30:00.000Z",
      "verification_signature": "0x...",
      "verification_message": "Sign this message...",
      "is_active": true,
      "metadata": {},
      "created_at": "2026-01-10T08:00:00.000Z",
      "updated_at": "2026-01-15T10:30:00.000Z"
    }
  ]
}
```

#### POST /api/web3/wallets
Create a new wallet.

**Request Body:**
```json
{
  "user_id": "user123",
  "workspace_id": "workspace456",
  "address": "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb",
  "chain_id": 1,
  "chain_name": "Ethereum Mainnet",
  "wallet_type": "eoa",
  "ens_name": "vitalik.eth",
  "ens_avatar": "https://...",
  "label": "My Primary Wallet",
  "is_primary": true,
  "metadata": {}
}
```

**Response:**
```json
{
  "success": true,
  "id": "wallet-uuid"
}
```

#### GET /api/web3/wallets/:id
Get wallet by ID.

#### PUT /api/web3/wallets/:id
Update wallet.

#### POST /api/web3/wallets/:id/verify
Verify wallet ownership with signature.

**Request Body:**
```json
{
  "signature": "0x...",
  "message": "Sign this message to verify ownership"
}
```

#### DELETE /api/web3/wallets/:id
Delete wallet.

#### GET /api/web3/wallets/address/:address
Get wallet by address.

**Query Parameters:**
- `chain_id` - Required chain ID

### NFTs

#### GET /api/web3/nfts
List NFTs with optional filters.

**Query Parameters:**
- `owner_address` - Filter by owner address
- `owner_user_id` - Filter by owner user ID
- `collection_id` - Filter by collection ID
- `chain_id` - Filter by chain ID
- `token_standard` - Filter by token standard (ERC-721, ERC-1155, etc.)
- `is_verified` - Filter by verified status
- `contract_address` - Filter by contract address

#### POST /api/web3/nfts
Create/index an NFT.

**Request Body:**
```json
{
  "contract_address": "0x...",
  "token_id": "1234",
  "chain_id": 1,
  "token_standard": "ERC-721",
  "owner_address": "0x...",
  "owner_user_id": "user123",
  "quantity": 1,
  "name": "Bored Ape #1234",
  "description": "A bored ape",
  "image_url": "ipfs://...",
  "animation_url": "ipfs://...",
  "external_url": "https://...",
  "attributes": [
    { "trait_type": "Background", "value": "Blue" },
    { "trait_type": "Eyes", "value": "Laser" }
  ],
  "metadata_uri": "ipfs://...",
  "metadata": {},
  "collection_id": "collection-uuid",
  "rarity_score": 95.5,
  "rarity_rank": 123,
  "minted_at": "2021-05-01T00:00:00.000Z"
}
```

#### GET /api/web3/nfts/:id
Get NFT by ID.

#### PUT /api/web3/nfts/:id
Update NFT.

#### DELETE /api/web3/nfts/:id
Delete NFT.

#### GET /api/web3/nfts/token/:contractAddress/:tokenId
Get NFT by contract address and token ID.

**Query Parameters:**
- `chain_id` - Required chain ID

### Collections

#### GET /api/web3/collections
List collections.

**Query Parameters:**
- `chain_id` - Filter by chain ID
- `token_standard` - Filter by token standard
- `is_verified` - Filter by verified status
- `is_managed` - Filter by managed status
- `workspace_id` - Filter by workspace ID

#### POST /api/web3/collections
Create/index a collection.

**Request Body:**
```json
{
  "contract_address": "0x...",
  "chain_id": 1,
  "name": "Bored Ape Yacht Club",
  "slug": "bored-ape-yacht-club",
  "description": "A collection of...",
  "image_url": "https://...",
  "banner_url": "https://...",
  "featured_image_url": "https://...",
  "website_url": "https://boredapeyachtclub.com",
  "twitter_username": "BoredApeYC",
  "discord_url": "https://discord.gg/...",
  "telegram_url": "https://t.me/...",
  "token_standard": "ERC-721",
  "total_supply": 10000,
  "workspace_id": "workspace456",
  "is_managed": true,
  "metadata": {}
}
```

#### GET /api/web3/collections/:id
Get collection by ID.

#### GET /api/web3/collections/slug/:slug
Get collection by slug.

#### GET /api/web3/collections/contract/:contractAddress
Get collection by contract address.

**Query Parameters:**
- `chain_id` - Required chain ID

#### PUT /api/web3/collections/:id
Update collection.

#### DELETE /api/web3/collections/:id
Delete collection.

#### GET /api/web3/collections/:id/nfts
Get all NFTs in a collection.

### Tokens

#### GET /api/web3/tokens
List ERC-20 tokens.

**Query Parameters:**
- `chain_id` - Filter by chain ID
- `token_type` - Filter by type (ERC-20, native)
- `is_verified` - Filter by verified status

#### POST /api/web3/tokens
Create/index a token.

**Request Body:**
```json
{
  "contract_address": "0x...",
  "chain_id": 1,
  "name": "Uniswap",
  "symbol": "UNI",
  "decimals": 18,
  "token_type": "ERC-20",
  "price_usd": 5.23,
  "logo_url": "https://...",
  "website_url": "https://uniswap.org",
  "description": "Governance token for Uniswap",
  "metadata": {}
}
```

#### GET /api/web3/tokens/:id
Get token by ID.

#### GET /api/web3/tokens/contract/:contractAddress
Get token by contract address.

**Query Parameters:**
- `chain_id` - Required chain ID

#### PUT /api/web3/tokens/:id
Update token.

#### DELETE /api/web3/tokens/:id
Delete token.

### Token Balances

#### GET /api/web3/balances
Get token balances.

**Query Parameters:**
- `wallet_address` - Filter by wallet address
- `user_id` - Filter by user ID

#### POST /api/web3/balances
Upsert token balance.

**Request Body:**
```json
{
  "wallet_address": "0x...",
  "user_id": "user123",
  "token_id": "token-uuid",
  "balance": "1000000000000000000",
  "balance_formatted": 1.0,
  "value_usd": 5.23
}
```

#### DELETE /api/web3/balances/:id
Delete token balance.

### Token Gates

#### GET /api/web3/gates
List token gates.

**Query Parameters:**
- `workspace_id` - Filter by workspace ID
- `gate_type` - Filter by type (nft_ownership, token_balance, etc.)
- `target_type` - Filter by target type (channel, feature, content, role)
- `is_active` - Filter by active status

#### POST /api/web3/gates
Create a token gate.

**Request Body:**
```json
{
  "workspace_id": "workspace456",
  "created_by": "user123",
  "name": "Premium Channel Access",
  "description": "Requires Bored Ape NFT",
  "gate_type": "nft_ownership",
  "rules": {
    "contract_address": "0x...",
    "chain_id": 1,
    "min_quantity": 1
  },
  "target_type": "channel",
  "target_id": "channel789"
}
```

#### GET /api/web3/gates/:id
Get gate by ID.

#### PUT /api/web3/gates/:id
Update gate.

#### DELETE /api/web3/gates/:id
Delete gate.

#### POST /api/web3/gates/:id/check
Check if user passes gate.

**Request Body:**
```json
{
  "user_id": "user123",
  "wallet_address": "0x..."
}
```

**Response:**
```json
{
  "passed": true,
  "gate_id": "gate-uuid",
  "gate_name": "Premium Channel Access",
  "failure_reason": null,
  "evidence": {
    "nft_count": 2,
    "total_quantity": 2,
    "required": 1
  },
  "cached": false
}
```

#### GET /api/web3/gates/:id/checks
Get gate check history.

**Query Parameters:**
- `user_id` - Filter by user ID

### DAOs

#### GET /api/web3/daos
List DAOs.

**Query Parameters:**
- `workspace_id` - Filter by workspace ID
- `chain_id` - Filter by chain ID
- `is_active` - Filter by active status

#### POST /api/web3/daos
Create a DAO.

**Request Body:**
```json
{
  "workspace_id": "workspace456",
  "name": "My DAO",
  "slug": "my-dao",
  "description": "A decentralized autonomous organization",
  "chain_id": 1,
  "governance_token_address": "0x...",
  "treasury_address": "0x...",
  "snapshot_space": "my-dao.eth",
  "governor_address": "0x...",
  "governor_type": "compound",
  "proposal_threshold": "100000000000000000000",
  "quorum": "1000000000000000000000",
  "voting_delay": 13140,
  "voting_period": 40320,
  "metadata": {}
}
```

#### GET /api/web3/daos/:id
Get DAO by ID.

#### GET /api/web3/daos/slug/:slug
Get DAO by slug.

#### PUT /api/web3/daos/:id
Update DAO.

#### DELETE /api/web3/daos/:id
Delete DAO.

#### GET /api/web3/daos/:id/proposals
Get all proposals for a DAO.

**Query Parameters:**
- `status` - Filter by proposal status

### Proposals

#### GET /api/web3/proposals
List proposals.

**Query Parameters:**
- `dao_id` - Filter by DAO ID
- `status` - Filter by status (pending, active, canceled, defeated, succeeded, queued, expired, executed)
- `proposer_address` - Filter by proposer address

#### POST /api/web3/proposals
Create a proposal.

**Request Body:**
```json
{
  "dao_id": "dao-uuid",
  "title": "Proposal: Increase treasury allocation",
  "description": "This proposal seeks to...",
  "proposer_address": "0x...",
  "proposer_user_id": "user123",
  "chain_proposal_id": "1",
  "snapshot_proposal_id": "0x...",
  "status": "pending",
  "start_block": 12345678,
  "end_block": 12385998,
  "start_time": "2026-02-15T00:00:00.000Z",
  "end_time": "2026-02-22T00:00:00.000Z",
  "executable": true,
  "metadata": {}
}
```

#### GET /api/web3/proposals/:id
Get proposal by ID.

#### PUT /api/web3/proposals/:id
Update proposal.

#### DELETE /api/web3/proposals/:id
Delete proposal.

#### GET /api/web3/proposals/:id/votes
Get all votes for a proposal.

### Votes

#### GET /api/web3/votes
List votes.

**Query Parameters:**
- `proposal_id` - Filter by proposal ID
- `voter_address` - Filter by voter address

#### POST /api/web3/votes
Cast a vote.

**Request Body:**
```json
{
  "proposal_id": "proposal-uuid",
  "voter_address": "0x...",
  "voter_user_id": "user123",
  "support": "for",
  "voting_power": "100000000000000000000",
  "reason": "I support this proposal because...",
  "transaction_hash": "0x...",
  "block_number": 12350000
}
```

#### GET /api/web3/votes/:id
Get vote by ID.

### Transactions

#### GET /api/web3/transactions
List transactions.

**Query Parameters:**
- `chain_id` - Filter by chain ID
- `from_address` - Filter by from address
- `to_address` - Filter by to address
- `from_user_id` - Filter by from user ID
- `to_user_id` - Filter by to user ID
- `status` - Filter by status (pending, confirmed, failed)
- `transaction_type` - Filter by type (transfer, contract_interaction, nft_transfer, token_transfer, swap)
- `limit` - Result limit (default: 100)
- `offset` - Result offset (default: 0)

#### POST /api/web3/transactions
Index a transaction.

**Request Body:**
```json
{
  "transaction_hash": "0x...",
  "chain_id": 1,
  "from_address": "0x...",
  "to_address": "0x...",
  "from_user_id": "user123",
  "to_user_id": "user456",
  "value": "1000000000000000000",
  "value_usd": 3500.00,
  "gas_used": 21000,
  "gas_price": "50000000000",
  "block_number": 12345678,
  "block_timestamp": "2026-02-11T12:00:00.000Z",
  "status": "confirmed",
  "transaction_type": "transfer",
  "input_data": "0x",
  "metadata": {}
}
```

#### GET /api/web3/transactions/:id
Get transaction by ID.

#### GET /api/web3/transactions/hash/:hash
Get transaction by hash.

**Query Parameters:**
- `chain_id` - Required chain ID

### Events

#### GET /api/web3/events
List blockchain events.

**Query Parameters:**
- `contract_address` - Filter by contract address
- `chain_id` - Filter by chain ID
- `event_name` - Filter by event name
- `related_nft_id` - Filter by related NFT ID
- `limit` - Result limit (default: 100)
- `offset` - Result offset (default: 0)

#### POST /api/web3/events
Index a blockchain event.

**Request Body:**
```json
{
  "event_name": "Transfer",
  "contract_address": "0x...",
  "chain_id": 1,
  "transaction_hash": "0x...",
  "log_index": 0,
  "block_number": 12345678,
  "block_timestamp": "2026-02-11T12:00:00.000Z",
  "event_data": {
    "from": "0x...",
    "to": "0x...",
    "tokenId": "1234"
  },
  "decoded_data": {
    "from": "0x...",
    "to": "0x...",
    "token_id": "1234"
  },
  "related_nft_id": "nft-uuid",
  "related_user_id": "user123"
}
```

#### GET /api/web3/events/:id
Get event by ID.

---

## Webhook Events

The Web3 plugin emits webhook events for key blockchain activities:

| Event | Description | Payload |
|-------|-------------|---------|
| `wallet.connected` | Wallet connected to platform | `{ wallet_id, user_id, address, chain_id }` |
| `wallet.verified` | Wallet verified with signature | `{ wallet_id, user_id, address, verified_at }` |
| `nft.transferred` | NFT ownership transferred | `{ nft_id, from_address, to_address, token_id, contract_address }` |
| `nft.minted` | NFT minted | `{ nft_id, owner_address, token_id, contract_address }` |
| `collection.indexed` | Collection indexed | `{ collection_id, contract_address, name }` |
| `token_gate.passed` | User passed token gate check | `{ gate_id, user_id, wallet_address, evidence }` |
| `token_gate.failed` | User failed token gate check | `{ gate_id, user_id, wallet_address, failure_reason }` |
| `proposal.created` | DAO proposal created | `{ proposal_id, dao_id, title, proposer_address }` |
| `proposal.executed` | DAO proposal executed | `{ proposal_id, dao_id, execution_tx_hash }` |
| `vote.cast` | DAO vote cast | `{ vote_id, proposal_id, voter_address, support }` |
| `transaction.confirmed` | Transaction confirmed on-chain | `{ transaction_id, hash, chain_id, status }` |

---

## Database Schema

### web3_wallets

Stores connected Web3 wallets.

```sql
CREATE TABLE web3_wallets (
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

CREATE INDEX idx_web3_wallets_source ON web3_wallets(source_account_id);
CREATE INDEX idx_web3_wallets_user ON web3_wallets(source_account_id, user_id);
CREATE INDEX idx_web3_wallets_address ON web3_wallets(source_account_id, address);
CREATE INDEX idx_web3_wallets_chain ON web3_wallets(source_account_id, chain_id);
CREATE INDEX idx_web3_wallets_ens ON web3_wallets(ens_name) WHERE ens_name IS NOT NULL;
```

**Columns:**
| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | gen_random_uuid() | Primary key |
| `source_account_id` | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| `user_id` | VARCHAR(255) | No | - | User ID owning the wallet |
| `workspace_id` | VARCHAR(255) | Yes | - | Workspace ID |
| `address` | TEXT | No | - | Wallet address (0x...) |
| `chain_id` | INTEGER | No | - | Blockchain chain ID |
| `chain_name` | TEXT | No | - | Human-readable chain name |
| `wallet_type` | TEXT | Yes | - | Wallet type (eoa, contract, multisig, safe) |
| `ens_name` | TEXT | Yes | - | ENS name (e.g., vitalik.eth) |
| `ens_avatar` | TEXT | Yes | - | ENS avatar URL |
| `label` | TEXT | Yes | - | User-defined label |
| `is_primary` | BOOLEAN | No | false | Primary wallet flag |
| `verified_at` | TIMESTAMPTZ | Yes | - | Verification timestamp |
| `verification_signature` | TEXT | Yes | - | Signature proof |
| `verification_message` | TEXT | Yes | - | Message that was signed |
| `is_active` | BOOLEAN | No | true | Active status |
| `metadata` | JSONB | Yes | - | Additional metadata |
| `created_at` | TIMESTAMPTZ | No | NOW() | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | No | NOW() | Last update timestamp |

### web3_collections

Stores NFT collection metadata.

```sql
CREATE TABLE web3_collections (
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

CREATE INDEX idx_web3_collections_source ON web3_collections(source_account_id);
CREATE INDEX idx_web3_collections_contract ON web3_collections(source_account_id, contract_address, chain_id);
CREATE INDEX idx_web3_collections_slug ON web3_collections(source_account_id, slug);
```

### web3_nfts

Stores NFT ownership and metadata.

```sql
CREATE TABLE web3_nfts (
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
);

CREATE INDEX idx_web3_nfts_source ON web3_nfts(source_account_id);
CREATE INDEX idx_web3_nfts_contract ON web3_nfts(source_account_id, contract_address, chain_id);
CREATE INDEX idx_web3_nfts_owner ON web3_nfts(source_account_id, owner_address);
CREATE INDEX idx_web3_nfts_collection ON web3_nfts(source_account_id, collection_id);
CREATE INDEX idx_web3_nfts_attributes ON web3_nfts USING GIN(attributes);
```

### web3_tokens

Stores ERC-20 token information.

```sql
CREATE TABLE web3_tokens (
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

CREATE INDEX idx_web3_tokens_source ON web3_tokens(source_account_id);
CREATE INDEX idx_web3_tokens_contract ON web3_tokens(source_account_id, contract_address, chain_id);
CREATE INDEX idx_web3_tokens_symbol ON web3_tokens(source_account_id, symbol);
```

### web3_token_balances

Stores token balances for wallets.

```sql
CREATE TABLE web3_token_balances (
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
);

CREATE INDEX idx_web3_token_balances_source ON web3_token_balances(source_account_id);
CREATE INDEX idx_web3_token_balances_wallet ON web3_token_balances(source_account_id, wallet_address);
CREATE INDEX idx_web3_token_balances_user ON web3_token_balances(source_account_id, user_id);
```

### web3_token_gates

Stores token-gated access rules.

```sql
CREATE TABLE web3_token_gates (
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

CREATE INDEX idx_web3_token_gates_source ON web3_token_gates(source_account_id);
CREATE INDEX idx_web3_token_gates_workspace ON web3_token_gates(source_account_id, workspace_id);
CREATE INDEX idx_web3_token_gates_target ON web3_token_gates(source_account_id, target_type, target_id);
CREATE INDEX idx_web3_token_gates_rules ON web3_token_gates USING GIN(rules);
```

### web3_gate_checks

Stores token gate check results (with caching).

```sql
CREATE TABLE web3_gate_checks (
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
);

CREATE INDEX idx_web3_gate_checks_source ON web3_gate_checks(source_account_id);
CREATE INDEX idx_web3_gate_checks_gate ON web3_gate_checks(source_account_id, gate_id);
CREATE INDEX idx_web3_gate_checks_user ON web3_gate_checks(source_account_id, user_id);
CREATE INDEX idx_web3_gate_checks_expires ON web3_gate_checks(expires_at) WHERE expires_at IS NOT NULL;
```

### web3_daos

Stores DAO information.

```sql
CREATE TABLE web3_daos (
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

CREATE INDEX idx_web3_daos_source ON web3_daos(source_account_id);
CREATE INDEX idx_web3_daos_workspace ON web3_daos(source_account_id, workspace_id);
CREATE INDEX idx_web3_daos_slug ON web3_daos(source_account_id, slug);
```

### web3_proposals

Stores DAO proposals.

```sql
CREATE TABLE web3_proposals (
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
);

CREATE INDEX idx_web3_proposals_source ON web3_proposals(source_account_id);
CREATE INDEX idx_web3_proposals_dao ON web3_proposals(source_account_id, dao_id);
CREATE INDEX idx_web3_proposals_status ON web3_proposals(source_account_id, status);
```

### web3_votes

Stores DAO votes.

```sql
CREATE TABLE web3_votes (
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
);

CREATE INDEX idx_web3_votes_source ON web3_votes(source_account_id);
CREATE INDEX idx_web3_votes_proposal ON web3_votes(source_account_id, proposal_id);
CREATE INDEX idx_web3_votes_voter ON web3_votes(source_account_id, voter_address);
```

### web3_transactions

Stores blockchain transactions.

```sql
CREATE TABLE web3_transactions (
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

CREATE INDEX idx_web3_transactions_source ON web3_transactions(source_account_id);
CREATE INDEX idx_web3_transactions_hash ON web3_transactions(source_account_id, transaction_hash);
CREATE INDEX idx_web3_transactions_chain ON web3_transactions(source_account_id, chain_id);
CREATE INDEX idx_web3_transactions_from ON web3_transactions(source_account_id, from_address);
CREATE INDEX idx_web3_transactions_to ON web3_transactions(source_account_id, to_address);
CREATE INDEX idx_web3_transactions_block ON web3_transactions(block_number DESC);
```

### web3_events

Stores blockchain events.

```sql
CREATE TABLE web3_events (
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
);

CREATE INDEX idx_web3_events_source ON web3_events(source_account_id);
CREATE INDEX idx_web3_events_contract ON web3_events(source_account_id, contract_address, chain_id);
CREATE INDEX idx_web3_events_name ON web3_events(source_account_id, event_name);
CREATE INDEX idx_web3_events_tx ON web3_events(source_account_id, transaction_hash);
CREATE INDEX idx_web3_events_block ON web3_events(block_number DESC);
CREATE INDEX idx_web3_events_data ON web3_events USING GIN(event_data);
```

---

## Examples

### Example 1: Connect and Verify Wallet

```bash
# CLI: Check existing wallets
nself plugin web3 wallets list --user user123

# API: Connect wallet
curl -X POST http://localhost:3715/api/web3/wallets \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user123",
    "address": "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb",
    "chain_id": 1,
    "chain_name": "Ethereum Mainnet",
    "wallet_type": "eoa",
    "label": "My MetaMask Wallet"
  }'

# API: Verify wallet with signature
curl -X POST http://localhost:3715/api/web3/wallets/<wallet-id>/verify \
  -H "Content-Type: application/json" \
  -d '{
    "signature": "0x1234...",
    "message": "Sign this message to verify ownership of 0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb"
  }'
```

### Example 2: Create Token Gate for Premium Channel

```bash
# Create NFT ownership gate
curl -X POST http://localhost:3715/api/web3/gates \
  -H "Content-Type: application/json" \
  -d '{
    "workspace_id": "workspace456",
    "created_by": "admin123",
    "name": "Premium Channel Access",
    "description": "Requires Bored Ape Yacht Club NFT",
    "gate_type": "nft_ownership",
    "rules": {
      "contract_address": "0xBC4CA0EdA7647A8aB7C2061c2E118A18a936f13D",
      "chain_id": 1,
      "min_quantity": 1
    },
    "target_type": "channel",
    "target_id": "channel789"
  }'

# Check if user passes gate
curl -X POST http://localhost:3715/api/web3/gates/<gate-id>/check \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user123",
    "wallet_address": "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb"
  }'
```

### Example 3: Track NFT Collection

```sql
-- Get all NFTs in a collection
SELECT
  n.id,
  n.token_id,
  n.name,
  n.owner_address,
  n.rarity_rank,
  n.image_url,
  c.name AS collection_name
FROM web3_nfts n
JOIN web3_collections c ON n.collection_id = c.id
WHERE c.slug = 'bored-ape-yacht-club'
  AND n.source_account_id = 'primary'
ORDER BY n.rarity_rank ASC
LIMIT 100;

-- Get NFT ownership by user
SELECT
  u.name AS user_name,
  COUNT(n.id) AS nft_count,
  JSON_AGG(
    JSON_BUILD_OBJECT(
      'collection', c.name,
      'token_id', n.token_id,
      'name', n.name,
      'image_url', n.image_url
    )
  ) AS nfts
FROM web3_nfts n
JOIN web3_collections c ON n.collection_id = c.id
JOIN users u ON n.owner_user_id = u.id
WHERE n.source_account_id = 'primary'
GROUP BY u.id, u.name
ORDER BY nft_count DESC;
```

### Example 4: DAO Proposal Voting

```bash
# Create a proposal
curl -X POST http://localhost:3715/api/web3/proposals \
  -H "Content-Type: application/json" \
  -d '{
    "dao_id": "dao-uuid",
    "title": "Increase Treasury Allocation for Marketing",
    "description": "This proposal seeks to allocate 100,000 tokens...",
    "proposer_address": "0x...",
    "proposer_user_id": "user123",
    "status": "active",
    "start_time": "2026-02-15T00:00:00.000Z",
    "end_time": "2026-02-22T00:00:00.000Z"
  }'

# Cast a vote
curl -X POST http://localhost:3715/api/web3/votes \
  -H "Content-Type: application/json" \
  -d '{
    "proposal_id": "proposal-uuid",
    "voter_address": "0x...",
    "voter_user_id": "user456",
    "support": "for",
    "voting_power": "10000000000000000000000",
    "reason": "I support this proposal because..."
  }'
```

```sql
-- Get voting results
SELECT
  p.title,
  p.status,
  p.votes_for,
  p.votes_against,
  p.votes_abstain,
  COUNT(v.id) AS total_votes,
  COUNT(DISTINCT v.voter_address) AS unique_voters
FROM web3_proposals p
LEFT JOIN web3_votes v ON p.id = v.proposal_id
WHERE p.dao_id = 'dao-uuid'
  AND p.source_account_id = 'primary'
GROUP BY p.id, p.title, p.status, p.votes_for, p.votes_against, p.votes_abstain;
```

### Example 5: Token Balance Tracking

```sql
-- Get top token holders
SELECT
  tb.wallet_address,
  w.ens_name,
  t.symbol,
  tb.balance_formatted,
  tb.value_usd,
  tb.last_updated_at
FROM web3_token_balances tb
JOIN web3_tokens t ON tb.token_id = t.id
LEFT JOIN web3_wallets w ON tb.wallet_address = w.address
WHERE t.symbol = 'UNI'
  AND tb.source_account_id = 'primary'
ORDER BY tb.balance_formatted DESC
LIMIT 100;

-- Get user's token portfolio
SELECT
  t.name,
  t.symbol,
  tb.balance_formatted,
  tb.value_usd,
  t.price_usd,
  (tb.balance_formatted * t.price_usd) AS calculated_value
FROM web3_token_balances tb
JOIN web3_tokens t ON tb.token_id = t.id
WHERE tb.user_id = 'user123'
  AND tb.source_account_id = 'primary'
ORDER BY tb.value_usd DESC;
```

---

## Troubleshooting

### Common Issues

#### "Database connection failed"

```
Error: Connection refused
```

**Solutions:**
1. Verify PostgreSQL is running
2. Check `DATABASE_URL` or individual `POSTGRES_*` variables
3. Test connection: `psql $DATABASE_URL -c "SELECT 1"`
4. Verify network connectivity

```bash
# Check PostgreSQL status
pg_isready -h localhost -p 5432

# Test connection
psql "postgresql://user:pass@localhost:5432/nself" -c "SELECT version();"
```

#### "Invalid chain ID"

```
Error: Chain ID 999 not supported
```

**Solution:** Verify chain ID is in `WEB3_SUPPORTED_CHAINS` environment variable.

```bash
# Check supported chains
echo $WEB3_SUPPORTED_CHAINS

# Add chain if needed
export WEB3_SUPPORTED_CHAINS=1,137,42161,10,8453,999
```

#### "Token gate check failed"

```
Error: Gate check failed: Insufficient NFTs
```

**Solutions:**
1. Verify wallet address is correct
2. Check that NFTs are indexed in the database
3. Verify gate rules are correct
4. Check if cache is stale (wait for TTL expiration or clear cache)

```bash
# Check user's NFTs
nself plugin web3 nfts list --owner 0x...

# Check gate configuration
nself plugin web3 gates get --id <gate-uuid>

# Perform fresh gate check
curl -X POST http://localhost:3715/api/web3/gates/<gate-id>/check \
  -H "Content-Type: application/json" \
  -d '{"user_id": "user123", "wallet_address": "0x..."}'
```

#### "ENS resolution failed"

```
Error: Could not resolve ENS name
```

**Solution:** ENS resolution requires an Ethereum node connection. This feature is optional and can be populated manually.

```bash
# Set ENS name manually via API
curl -X PUT http://localhost:3715/api/web3/wallets/<wallet-id> \
  -H "Content-Type: application/json" \
  -d '{
    "ens_name": "vitalik.eth",
    "ens_avatar": "https://..."
  }'
```

#### "Wallet verification failed"

```
Error: Invalid signature
```

**Solutions:**
1. Ensure signature is for the correct address
2. Verify message matches exactly (including whitespace)
3. Check that signature is properly formatted (0x prefix)
4. Use EIP-191 or EIP-712 standard for signing

```typescript
// Example wallet verification flow
const message = `Sign this message to verify ownership of ${address}`;
const signature = await signer.signMessage(message);

// Submit to API
await fetch('/api/web3/wallets/${walletId}/verify', {
  method: 'POST',
  body: JSON.stringify({ message, signature })
});
```

### Debug Mode

Enable debug logging for detailed troubleshooting:

```bash
LOG_LEVEL=debug nself plugin web3 server
```

### Health Checks

```bash
# Check plugin health
curl http://localhost:3715/health

# Check database connectivity
curl http://localhost:3715/ready

# Check detailed status
curl http://localhost:3715/v1/status
```

---

## Security Considerations

### API Key Authentication

When `WEB3_API_KEY` is set, all API requests require authentication:

```bash
curl -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:3715/api/web3/wallets
```

### Wallet Verification

Always verify wallet ownership before granting access:

1. Generate unique message for user
2. User signs with their wallet
3. Verify signature matches address
4. Mark wallet as verified with timestamp

### Token Gate Security

- Cache TTL prevents frequent on-chain checks
- Evidence stored for audit trail
- Gate checks are non-mutating (read-only)
- Failed checks include detailed reason

### Rate Limiting

Built-in rate limiting protects against abuse:
- Default: 500 requests per 60 seconds
- Per-IP tracking
- Configurable via `WEB3_RATE_LIMIT_MAX` and `WEB3_RATE_LIMIT_WINDOW_MS`

---

*Last Updated: February 11, 2026*
*Plugin Version: 1.0.0*
*nself Version: 0.4.8+*
