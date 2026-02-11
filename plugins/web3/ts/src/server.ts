/**
 * Web3 Plugin Server
 * HTTP server for blockchain integration, NFTs, token gating, and DAO governance
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { Web3Database } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreateWalletRequest, UpdateWalletRequest, VerifyWalletRequest,
  CreateNftRequest, UpdateNftRequest,
  CreateCollectionRequest, UpdateCollectionRequest,
  CreateTokenRequest, UpdateTokenRequest,
  UpsertTokenBalanceRequest,
  CreateTokenGateRequest, UpdateTokenGateRequest,
  CreateDaoRequest, UpdateDaoRequest,
  CreateProposalRequest, UpdateProposalRequest,
  CreateVoteRequest,
  CreateTransactionRequest,
  CreateWeb3EventRequest,
  TokenStandard, TokenType, GateType, GateTargetType,
  ProposalStatus, TransactionStatus, TransactionType,
} from './types.js';

const logger = createLogger('web3:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  const db = new Web3Database();
  await db.connect();
  await db.initializeSchema();

  const app = Fastify({ logger: false, bodyLimit: 10 * 1024 * 1024 });
  await app.register(cors, { origin: true, credentials: true });

  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 500,
    fullConfig.security.rateLimitWindowMs ?? 60000
  );
  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  if (fullConfig.security.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.security.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  function scopedDb(request: unknown): Web3Database {
    return (request as Record<string, unknown>).scopedDb as Web3Database;
  }

  // =========================================================================
  // Health Checks
  // =========================================================================

  app.get('/health', async () => ({ status: 'ok', plugin: 'web3', timestamp: new Date().toISOString() }));

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'web3', timestamp: new Date().toISOString() };
    } catch {
      return reply.status(503).send({ ready: false, plugin: 'web3', error: 'Database unavailable' });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return { alive: true, plugin: 'web3', version: '1.0.0', uptime: process.uptime(), stats, timestamp: new Date().toISOString() };
  });

  // =========================================================================
  // Wallets
  // =========================================================================

  app.get('/api/web3/wallets', async (request) => {
    const { user_id, workspace_id, chain_id, is_active } = request.query as Record<string, string | undefined>;
    const wallets = await scopedDb(request).listWallets({
      user_id, workspace_id,
      chain_id: chain_id ? Number(chain_id) : undefined,
      is_active: is_active !== undefined ? is_active === 'true' : undefined,
    });
    return { data: wallets };
  });

  app.post('/api/web3/wallets', async (request, reply) => {
    const body = request.body as CreateWalletRequest;
    if (!body.user_id || !body.address || !body.chain_id || !body.chain_name) {
      return reply.status(400).send({ error: 'user_id, address, chain_id, and chain_name are required' });
    }
    const id = await scopedDb(request).createWallet(body);
    return { success: true, id };
  });

  app.get('/api/web3/wallets/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const wallet = await scopedDb(request).getWallet(id);
    if (!wallet) return reply.status(404).send({ error: 'Wallet not found' });
    return wallet;
  });

  app.put('/api/web3/wallets/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const updated = await scopedDb(request).updateWallet(id, request.body as UpdateWalletRequest);
    if (!updated) return reply.status(404).send({ error: 'Wallet not found' });
    return { success: true };
  });

  app.post('/api/web3/wallets/:id/verify', async (request, reply) => {
    const { id } = request.params as { id: string };
    const body = request.body as VerifyWalletRequest;
    if (!body.signature || !body.message) {
      return reply.status(400).send({ error: 'signature and message are required' });
    }
    const verified = await scopedDb(request).verifyWallet(id, body);
    if (!verified) return reply.status(404).send({ error: 'Wallet not found' });
    return { success: true };
  });

  app.delete('/api/web3/wallets/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deleteWallet(id);
    if (!deleted) return reply.status(404).send({ error: 'Wallet not found' });
    return { success: true };
  });

  app.get('/api/web3/wallets/address/:address', async (request, reply) => {
    const { address } = request.params as { address: string };
    const { chain_id } = request.query as { chain_id?: string };
    if (!chain_id) return reply.status(400).send({ error: 'chain_id query parameter is required' });
    const wallet = await scopedDb(request).getWalletByAddress(address, Number(chain_id));
    if (!wallet) return reply.status(404).send({ error: 'Wallet not found' });
    return wallet;
  });

  // =========================================================================
  // NFTs
  // =========================================================================

  app.get('/api/web3/nfts', async (request) => {
    const { owner_address, owner_user_id, collection_id, chain_id, token_standard, is_verified, contract_address } =
      request.query as Record<string, string | undefined>;
    const nfts = await scopedDb(request).listNfts({
      owner_address, owner_user_id, collection_id,
      chain_id: chain_id ? Number(chain_id) : undefined,
      token_standard: token_standard as TokenStandard,
      is_verified: is_verified !== undefined ? is_verified === 'true' : undefined,
      contract_address,
    });
    return { data: nfts };
  });

  app.post('/api/web3/nfts', async (request, reply) => {
    const body = request.body as CreateNftRequest;
    if (!body.contract_address || !body.token_id || !body.chain_id || !body.token_standard || !body.owner_address) {
      return reply.status(400).send({ error: 'contract_address, token_id, chain_id, token_standard, and owner_address are required' });
    }
    const id = await scopedDb(request).createNft(body);
    return { success: true, id };
  });

  app.get('/api/web3/nfts/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const nft = await scopedDb(request).getNft(id);
    if (!nft) return reply.status(404).send({ error: 'NFT not found' });
    return nft;
  });

  app.put('/api/web3/nfts/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const updated = await scopedDb(request).updateNft(id, request.body as UpdateNftRequest);
    if (!updated) return reply.status(404).send({ error: 'NFT not found' });
    return { success: true };
  });

  app.delete('/api/web3/nfts/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deleteNft(id);
    if (!deleted) return reply.status(404).send({ error: 'NFT not found' });
    return { success: true };
  });

  app.get('/api/web3/nfts/token/:contractAddress/:tokenId', async (request, reply) => {
    const { contractAddress, tokenId } = request.params as { contractAddress: string; tokenId: string };
    const { chain_id } = request.query as { chain_id?: string };
    if (!chain_id) return reply.status(400).send({ error: 'chain_id query parameter is required' });
    const nft = await scopedDb(request).getNftByToken(contractAddress, tokenId, Number(chain_id));
    if (!nft) return reply.status(404).send({ error: 'NFT not found' });
    return nft;
  });

  // =========================================================================
  // Collections
  // =========================================================================

  app.get('/api/web3/collections', async (request) => {
    const { chain_id, token_standard, is_verified, is_managed, workspace_id } =
      request.query as Record<string, string | undefined>;
    const collections = await scopedDb(request).listCollections({
      chain_id: chain_id ? Number(chain_id) : undefined,
      token_standard: token_standard as TokenStandard,
      is_verified: is_verified !== undefined ? is_verified === 'true' : undefined,
      is_managed: is_managed !== undefined ? is_managed === 'true' : undefined,
      workspace_id,
    });
    return { data: collections };
  });

  app.post('/api/web3/collections', async (request, reply) => {
    const body = request.body as CreateCollectionRequest;
    if (!body.contract_address || !body.chain_id || !body.name) {
      return reply.status(400).send({ error: 'contract_address, chain_id, and name are required' });
    }
    const id = await scopedDb(request).createCollection(body);
    return { success: true, id };
  });

  app.get('/api/web3/collections/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const collection = await scopedDb(request).getCollection(id);
    if (!collection) return reply.status(404).send({ error: 'Collection not found' });
    return collection;
  });

  app.get('/api/web3/collections/slug/:slug', async (request, reply) => {
    const { slug } = request.params as { slug: string };
    const collection = await scopedDb(request).getCollectionBySlug(slug);
    if (!collection) return reply.status(404).send({ error: 'Collection not found' });
    return collection;
  });

  app.get('/api/web3/collections/contract/:contractAddress', async (request, reply) => {
    const { contractAddress } = request.params as { contractAddress: string };
    const { chain_id } = request.query as { chain_id?: string };
    if (!chain_id) return reply.status(400).send({ error: 'chain_id query parameter is required' });
    const collection = await scopedDb(request).getCollectionByContract(contractAddress, Number(chain_id));
    if (!collection) return reply.status(404).send({ error: 'Collection not found' });
    return collection;
  });

  app.put('/api/web3/collections/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const updated = await scopedDb(request).updateCollection(id, request.body as UpdateCollectionRequest);
    if (!updated) return reply.status(404).send({ error: 'Collection not found' });
    return { success: true };
  });

  app.delete('/api/web3/collections/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deleteCollection(id);
    if (!deleted) return reply.status(404).send({ error: 'Collection not found' });
    return { success: true };
  });

  app.get('/api/web3/collections/:id/nfts', async (request) => {
    const { id } = request.params as { id: string };
    const nfts = await scopedDb(request).listNfts({ collection_id: id });
    return { data: nfts };
  });

  // =========================================================================
  // Tokens
  // =========================================================================

  app.get('/api/web3/tokens', async (request) => {
    const { chain_id, token_type, is_verified } = request.query as Record<string, string | undefined>;
    const tokens = await scopedDb(request).listTokens({
      chain_id: chain_id ? Number(chain_id) : undefined,
      token_type: token_type as TokenType,
      is_verified: is_verified !== undefined ? is_verified === 'true' : undefined,
    });
    return { data: tokens };
  });

  app.post('/api/web3/tokens', async (request, reply) => {
    const body = request.body as CreateTokenRequest;
    if (!body.contract_address || !body.chain_id || !body.name || !body.symbol || body.decimals === undefined || !body.token_type) {
      return reply.status(400).send({ error: 'contract_address, chain_id, name, symbol, decimals, and token_type are required' });
    }
    const id = await scopedDb(request).createToken(body);
    return { success: true, id };
  });

  app.get('/api/web3/tokens/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const token = await scopedDb(request).getToken(id);
    if (!token) return reply.status(404).send({ error: 'Token not found' });
    return token;
  });

  app.get('/api/web3/tokens/contract/:contractAddress', async (request, reply) => {
    const { contractAddress } = request.params as { contractAddress: string };
    const { chain_id } = request.query as { chain_id?: string };
    if (!chain_id) return reply.status(400).send({ error: 'chain_id query parameter is required' });
    const token = await scopedDb(request).getTokenByContract(contractAddress, Number(chain_id));
    if (!token) return reply.status(404).send({ error: 'Token not found' });
    return token;
  });

  app.put('/api/web3/tokens/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const updated = await scopedDb(request).updateToken(id, request.body as UpdateTokenRequest);
    if (!updated) return reply.status(404).send({ error: 'Token not found' });
    return { success: true };
  });

  app.delete('/api/web3/tokens/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deleteToken(id);
    if (!deleted) return reply.status(404).send({ error: 'Token not found' });
    return { success: true };
  });

  // =========================================================================
  // Token Balances
  // =========================================================================

  app.get('/api/web3/balances', async (request) => {
    const { wallet_address, user_id } = request.query as { wallet_address?: string; user_id?: string };
    const balances = await scopedDb(request).listTokenBalances(wallet_address, user_id);
    return { data: balances };
  });

  app.post('/api/web3/balances', async (request, reply) => {
    const body = request.body as UpsertTokenBalanceRequest;
    if (!body.wallet_address || !body.token_id || !body.balance) {
      return reply.status(400).send({ error: 'wallet_address, token_id, and balance are required' });
    }
    const id = await scopedDb(request).upsertTokenBalance(body);
    return { success: true, id };
  });

  app.delete('/api/web3/balances/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deleteTokenBalance(id);
    if (!deleted) return reply.status(404).send({ error: 'Balance not found' });
    return { success: true };
  });

  // =========================================================================
  // Token Gates
  // =========================================================================

  app.get('/api/web3/gates', async (request) => {
    const { workspace_id, gate_type, target_type, is_active } = request.query as Record<string, string | undefined>;
    const gates = await scopedDb(request).listTokenGates({
      workspace_id,
      gate_type: gate_type as GateType,
      target_type: target_type as GateTargetType,
      is_active: is_active !== undefined ? is_active === 'true' : undefined,
    });
    return { data: gates };
  });

  app.post('/api/web3/gates', async (request, reply) => {
    const body = request.body as CreateTokenGateRequest;
    if (!body.workspace_id || !body.created_by || !body.name || !body.gate_type || !body.rules || !body.target_type) {
      return reply.status(400).send({ error: 'workspace_id, created_by, name, gate_type, rules, and target_type are required' });
    }
    const id = await scopedDb(request).createTokenGate(body);
    return { success: true, id };
  });

  app.get('/api/web3/gates/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const gate = await scopedDb(request).getTokenGate(id);
    if (!gate) return reply.status(404).send({ error: 'Token gate not found' });
    return gate;
  });

  app.put('/api/web3/gates/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const updated = await scopedDb(request).updateTokenGate(id, request.body as UpdateTokenGateRequest);
    if (!updated) return reply.status(404).send({ error: 'Token gate not found' });
    return { success: true };
  });

  app.delete('/api/web3/gates/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deleteTokenGate(id);
    if (!deleted) return reply.status(404).send({ error: 'Token gate not found' });
    return { success: true };
  });

  app.post('/api/web3/gates/:id/check', async (request, reply) => {
    const { id } = request.params as { id: string };
    const { user_id, wallet_address } = request.body as { user_id: string; wallet_address: string };
    if (!user_id || !wallet_address) {
      return reply.status(400).send({ error: 'user_id and wallet_address are required' });
    }
    const result = await scopedDb(request).checkGate(id, user_id, wallet_address, fullConfig.gateCheckCacheTtl);
    return result;
  });

  app.get('/api/web3/gates/:id/checks', async (request) => {
    const { id } = request.params as { id: string };
    const { user_id } = request.query as { user_id?: string };
    const checks = await scopedDb(request).listGateChecks(id, user_id);
    return { data: checks };
  });

  // =========================================================================
  // DAOs
  // =========================================================================

  app.get('/api/web3/daos', async (request) => {
    const { workspace_id, chain_id, is_active } = request.query as Record<string, string | undefined>;
    const daos = await scopedDb(request).listDaos(
      workspace_id,
      chain_id ? Number(chain_id) : undefined,
      is_active !== undefined ? is_active === 'true' : undefined,
    );
    return { data: daos };
  });

  app.post('/api/web3/daos', async (request, reply) => {
    const body = request.body as CreateDaoRequest;
    if (!body.name || !body.chain_id) {
      return reply.status(400).send({ error: 'name and chain_id are required' });
    }
    const id = await scopedDb(request).createDao(body);
    return { success: true, id };
  });

  app.get('/api/web3/daos/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const dao = await scopedDb(request).getDao(id);
    if (!dao) return reply.status(404).send({ error: 'DAO not found' });
    return dao;
  });

  app.get('/api/web3/daos/slug/:slug', async (request, reply) => {
    const { slug } = request.params as { slug: string };
    const dao = await scopedDb(request).getDaoBySlug(slug);
    if (!dao) return reply.status(404).send({ error: 'DAO not found' });
    return dao;
  });

  app.put('/api/web3/daos/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const updated = await scopedDb(request).updateDao(id, request.body as UpdateDaoRequest);
    if (!updated) return reply.status(404).send({ error: 'DAO not found' });
    return { success: true };
  });

  app.delete('/api/web3/daos/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deleteDao(id);
    if (!deleted) return reply.status(404).send({ error: 'DAO not found' });
    return { success: true };
  });

  // =========================================================================
  // Proposals
  // =========================================================================

  app.get('/api/web3/proposals', async (request) => {
    const { dao_id, status, proposer_address } = request.query as Record<string, string | undefined>;
    const proposals = await scopedDb(request).listProposals({
      dao_id, status: status as ProposalStatus, proposer_address,
    });
    return { data: proposals };
  });

  app.post('/api/web3/proposals', async (request, reply) => {
    const body = request.body as CreateProposalRequest;
    if (!body.dao_id || !body.title || !body.description || !body.proposer_address) {
      return reply.status(400).send({ error: 'dao_id, title, description, and proposer_address are required' });
    }
    const id = await scopedDb(request).createProposal(body);
    return { success: true, id };
  });

  app.get('/api/web3/proposals/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const proposal = await scopedDb(request).getProposal(id);
    if (!proposal) return reply.status(404).send({ error: 'Proposal not found' });
    return proposal;
  });

  app.put('/api/web3/proposals/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const updated = await scopedDb(request).updateProposal(id, request.body as UpdateProposalRequest);
    if (!updated) return reply.status(404).send({ error: 'Proposal not found' });
    return { success: true };
  });

  app.delete('/api/web3/proposals/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deleteProposal(id);
    if (!deleted) return reply.status(404).send({ error: 'Proposal not found' });
    return { success: true };
  });

  app.get('/api/web3/proposals/:id/votes', async (request) => {
    const { id } = request.params as { id: string };
    const votes = await scopedDb(request).listVotes(id);
    return { data: votes };
  });

  app.get('/api/web3/daos/:id/proposals', async (request) => {
    const { id } = request.params as { id: string };
    const { status } = request.query as { status?: string };
    const proposals = await scopedDb(request).listProposals({ dao_id: id, status: status as ProposalStatus });
    return { data: proposals };
  });

  // =========================================================================
  // Votes
  // =========================================================================

  app.post('/api/web3/votes', async (request, reply) => {
    const body = request.body as CreateVoteRequest;
    if (!body.proposal_id || !body.voter_address || !body.support || !body.voting_power) {
      return reply.status(400).send({ error: 'proposal_id, voter_address, support, and voting_power are required' });
    }
    const id = await scopedDb(request).createVote(body);
    return { success: true, id };
  });

  app.get('/api/web3/votes', async (request) => {
    const { proposal_id, voter_address } = request.query as { proposal_id?: string; voter_address?: string };
    const votes = await scopedDb(request).listVotes(proposal_id, voter_address);
    return { data: votes };
  });

  app.get('/api/web3/votes/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const vote = await scopedDb(request).getVote(id);
    if (!vote) return reply.status(404).send({ error: 'Vote not found' });
    return vote;
  });

  // =========================================================================
  // Transactions
  // =========================================================================

  app.get('/api/web3/transactions', async (request) => {
    const { chain_id, from_address, to_address, from_user_id, to_user_id, status, transaction_type, limit = '100', offset = '0' } =
      request.query as Record<string, string | undefined>;
    const transactions = await scopedDb(request).listTransactions({
      chain_id: chain_id ? Number(chain_id) : undefined,
      from_address, to_address, from_user_id, to_user_id,
      status: status as TransactionStatus,
      transaction_type: transaction_type as TransactionType,
    }, Number(limit), Number(offset));
    return { data: transactions };
  });

  app.post('/api/web3/transactions', async (request, reply) => {
    const body = request.body as CreateTransactionRequest;
    if (!body.transaction_hash || !body.chain_id || !body.from_address || !body.value) {
      return reply.status(400).send({ error: 'transaction_hash, chain_id, from_address, and value are required' });
    }
    const id = await scopedDb(request).createTransaction(body);
    return { success: true, id };
  });

  app.get('/api/web3/transactions/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const tx = await scopedDb(request).getTransaction(id);
    if (!tx) return reply.status(404).send({ error: 'Transaction not found' });
    return tx;
  });

  app.get('/api/web3/transactions/hash/:hash', async (request, reply) => {
    const { hash } = request.params as { hash: string };
    const { chain_id } = request.query as { chain_id?: string };
    if (!chain_id) return reply.status(400).send({ error: 'chain_id query parameter is required' });
    const tx = await scopedDb(request).getTransactionByHash(hash, Number(chain_id));
    if (!tx) return reply.status(404).send({ error: 'Transaction not found' });
    return tx;
  });

  // =========================================================================
  // Events
  // =========================================================================

  app.get('/api/web3/events', async (request) => {
    const { contract_address, chain_id, event_name, related_nft_id, limit = '100', offset = '0' } =
      request.query as Record<string, string | undefined>;
    const events = await scopedDb(request).listEvents({
      contract_address,
      chain_id: chain_id ? Number(chain_id) : undefined,
      event_name, related_nft_id,
    }, Number(limit), Number(offset));
    return { data: events };
  });

  app.post('/api/web3/events', async (request, reply) => {
    const body = request.body as CreateWeb3EventRequest;
    if (!body.event_name || !body.contract_address || !body.chain_id || !body.transaction_hash || body.log_index === undefined || !body.block_number || !body.event_data) {
      return reply.status(400).send({ error: 'event_name, contract_address, chain_id, transaction_hash, log_index, block_number, and event_data are required' });
    }
    const id = await scopedDb(request).createEvent(body);
    return { success: true, id };
  });

  app.get('/api/web3/events/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const event = await scopedDb(request).getEvent(id);
    if (!event) return reply.status(404).send({ error: 'Event not found' });
    return event;
  });

  // =========================================================================
  // Stats / Status
  // =========================================================================

  app.get('/v1/status', async (request) => {
    const stats = await scopedDb(request).getStats();
    return { plugin: 'web3', version: '1.0.0', status: 'running', stats, timestamp: new Date().toISOString() };
  });

  // Graceful shutdown
  const shutdown = async () => {
    logger.info('Shutting down server...');
    await app.close();
    await db.disconnect();
    process.exit(0);
  };

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);

  return { app, db, config: fullConfig, shutdown };
}

export async function startServer(config?: Partial<Config>): Promise<void> {
  const { app, config: fullConfig } = await createServer(config);
  await app.listen({ port: fullConfig.port, host: fullConfig.host });
  logger.success(`Web3 plugin listening on ${fullConfig.host}:${fullConfig.port}`);
}
