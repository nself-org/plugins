# Web3 Plugin

> Blockchain integration, NFT support, token-gated access, DAO governance, and decentralized identity.

**Tier:** Free (MIT) — no license required.

## Install

```bash
nself plugin install web3
nself build
```

## Description

Blockchain integration for nSelf-powered apps. The plugin stores wallet ownership proofs, NFT holdings, ERC-20 token balances, DAO proposals and votes, and records blockchain transactions that the app has observed. It provides a token-gating primitive so an app can express rules like "holders of collection X, or at least 100 of token Y, pass this gate."

Multi-chain support is configurable via `WEB3_SUPPORTED_CHAINS` (EVM chain IDs). Gate-check results are cached by TTL to avoid repeated RPC calls for the same wallet. Webhooks fire on wallet connection, NFT transfer, gate evaluation, DAO proposal lifecycle events, and confirmed transactions.

Wallet verification uses a signed-nonce flow: the app requests a nonce, the user signs it with their wallet, and the plugin verifies the signature against the claimed address. Once verified, the wallet binding is stored and can be reused for downstream gates and DAO voting without re-prompting the user.

## Configuration

| Env Var | Required | Default | Description |
|---------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | Main Postgres connection |
| `PORT` | No | `3128` | Service port |
| `WEB3_RPC_URL_<CHAIN_ID>` | Yes* | — | RPC endpoint per chain (e.g. `WEB3_RPC_URL_1=https://mainnet.infura.io/v3/KEY`). Required for on-chain reads. |
| `WEB3_DEFAULT_CHAIN_ID` | No | `1` | Default EVM chain id (1 = Ethereum mainnet) |
| `WEB3_SUPPORTED_CHAINS` | No | — | Comma-separated chain ids to accept |
| `WEB3_GATE_CHECK_CACHE_TTL` | No | `300` | Seconds to cache gate-check results |
| `WEB3_API_KEY` | No | — | Upstream RPC provider key (Infura / Alchemy) |
| `WEB3_RATE_LIMIT_MAX` | No | — | Requests per window |
| `WEB3_RATE_LIMIT_WINDOW_MS` | No | — | Rate-limit window in ms |

Reference vault credentials. Never hardcode secrets.

## Ports

- Default port: `3128`
- Bound to `127.0.0.1` per nSelf service-binding rules; reach via Nginx, never directly.

## Database Schema

Tables created (prefix `np_web3_`):

- `np_web3_wallets` — verified wallet bindings per user
- `np_web3_nfts` — NFT holdings indexed per wallet
- `np_web3_collections` — NFT collection metadata
- `np_web3_tokens` — ERC-20 token registry
- `np_web3_token_balances` — wallet balances per token
- `np_web3_token_gates` — token-gate rule definitions
- `np_web3_gate_checks` — cached gate-check evaluations
- `np_web3_daos` — DAO registry
- `np_web3_proposals` — DAO proposals lifecycle
- `np_web3_votes` — votes cast on proposals
- `np_web3_transactions` — observed blockchain transactions
- `np_web3_events` — outbound event log

All tables use `source_account_id` for multi-app isolation.

## REST API

Public endpoints. Internal admin routes are excluded from this surface.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness probe |
| POST | `/wallets/connect` | Record a connected wallet |
| POST | `/wallets/verify` | Verify wallet ownership via signature |
| GET | `/nfts/:wallet` | List NFT holdings for a wallet |
| GET | `/tokens/:wallet` | List ERC-20 balances for a wallet |
| POST | `/token-gates` | Create a token-gate rule |
| POST | `/token-gates/:id/check` | Evaluate the gate for a wallet |
| GET | `/daos/:id/proposals` | List DAO proposals |
| POST | `/daos/:id/proposals/:proposal/vote` | Cast a vote |

## Webhooks

| Event | Description |
|-------|-------------|
| `wallet.connected` | Wallet connected to the app |
| `wallet.verified` | Wallet ownership verified via signature |
| `nft.transferred` | NFT ownership transferred |
| `nft.minted` | NFT minted |
| `collection.indexed` | Collection indexed and ready for queries |
| `token_gate.passed` | Token gate check passed |
| `token_gate.failed` | Token gate check failed |
| `proposal.created` | DAO proposal created |
| `proposal.executed` | DAO proposal executed |
| `vote.cast` | DAO vote cast |
| `transaction.confirmed` | Transaction confirmed on chain |

## Examples

Verify a wallet ownership signature:

```bash
curl -X POST -H 'Authorization: Bearer $TOKEN' \
  https://api.example.com/web3/wallets/verify \
  -d '{"address":"0xabc...","signature":"0x...","nonce":"n_xxx"}'
```

Check a token gate:

```bash
curl -X POST -H 'Authorization: Bearer $TOKEN' \
  https://api.example.com/web3/token-gates/gate_xxx/check \
  -d '{"wallet":"0xabc..."}'
```

Cast a DAO vote:

```bash
curl -X POST -H 'Authorization: Bearer $TOKEN' \
  https://api.example.com/web3/daos/dao_xxx/proposals/prop_123/vote \
  -d '{"choice":"yes","wallet":"0xabc..."}'
```

List NFT holdings for a wallet:

```bash
curl -H 'Authorization: Bearer $TOKEN' \
  https://api.example.com/web3/nfts/0xabc...
```

## Source

MIT licensed, source included in this repository: [`free/web3/`](https://github.com/nself-org/plugins/tree/main/free/web3)

## See Also

- [[plugin-auth]] — pair with auth for unified user + wallet identity
- [[plugin-entitlements]] — feature gating across subscriptions and token gates
- [[plugin-notify]] — multi-channel notification delivery for proposal and gate events
- [[Pricing]] — tier comparison
- [[Plugins]] — full plugin index

← [[Plugins]] | [[Home]] →
