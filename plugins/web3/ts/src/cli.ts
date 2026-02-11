#!/usr/bin/env node
/**
 * Web3 Plugin CLI
 * Command-line interface for the Web3 plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { Web3Database } from './database.js';
import { createServer } from './server.js';

const logger = createLogger('web3:cli');

const program = new Command();

program
  .name('nself-web3')
  .description('Web3 plugin for nself - blockchain integration, NFTs, token gating, and DAO governance')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig();
      const db = new Web3Database();
      await db.connect();
      await db.initializeSchema();
      await db.disconnect();
      logger.success('Database schema initialized');
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Init failed', { error: message });
      process.exit(1);
    }
  });

// Server command
program
  .command('server')
  .description('Start the web3 server')
  .option('-p, --port <port>', 'Server port', '3715')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });
      const { app } = await createServer(config);
      await app.listen({ port: config.port, host: config.host });
      logger.success(`Web3 plugin listening on ${config.host}:${config.port}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Status command
program
  .command('status')
  .description('Show web3 status and statistics')
  .action(async () => {
    try {
      loadConfig();
      const db = new Web3Database();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nWeb3 Status');
      console.log('====================');
      console.log(`Wallets:              ${stats.total_wallets}`);
      console.log(`Verified Wallets:     ${stats.verified_wallets}`);
      console.log(`NFTs:                 ${stats.total_nfts}`);
      console.log(`Collections:          ${stats.total_collections}`);
      console.log(`Verified Collections: ${stats.verified_collections}`);
      console.log(`Tokens:               ${stats.total_tokens}`);
      console.log(`Active Token Gates:   ${stats.active_token_gates}`);
      console.log(`Gate Checks:          ${stats.total_gate_checks}`);
      console.log(`Passed Gate Checks:   ${stats.passed_gate_checks}`);
      console.log(`DAOs:                 ${stats.total_daos}`);
      console.log(`Active Proposals:     ${stats.active_proposals}`);
      console.log(`Total Votes:          ${stats.total_votes}`);
      console.log(`Transactions:         ${stats.total_transactions}`);
      console.log(`Events:               ${stats.total_events}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Wallets command
program
  .command('wallets')
  .description('Manage web3 wallets')
  .argument('[action]', 'Action: list, get', 'list')
  .option('--id <id>', 'Wallet ID (for get)')
  .option('-u, --user <id>', 'Filter by user ID')
  .option('--chain <chainId>', 'Filter by chain ID')
  .option('--address <address>', 'Wallet address (for get)')
  .action(async (action, options) => {
    try {
      loadConfig();
      const db = new Web3Database();
      await db.connect();

      switch (action) {
        case 'list': {
          const wallets = await db.listWallets({
            user_id: options.user,
            chain_id: options.chain ? Number(options.chain) : undefined,
            is_active: true,
          });
          console.log(`\nWallets (${wallets.length}):`);
          console.log('-'.repeat(120));
          for (const w of wallets) {
            const verified = w.verified_at ? 'Verified' : 'Unverified';
            const ens = w.ens_name ? ` (${w.ens_name})` : '';
            console.log(`${w.id} | ${w.address}${ens} | ${w.chain_name} (${w.chain_id}) | ${w.wallet_type ?? 'eoa'} | ${verified}`);
          }
          break;
        }
        case 'get': {
          let wallet = null;
          if (options.id) {
            wallet = await db.getWallet(options.id);
          } else if (options.address && options.chain) {
            wallet = await db.getWalletByAddress(options.address, Number(options.chain));
          } else {
            logger.error('ID or (address + chain) required');
            process.exit(1);
          }
          if (!wallet) { logger.error('Wallet not found'); process.exit(1); }
          console.log(JSON.stringify(wallet, null, 2));
          break;
        }
        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// NFTs command
program
  .command('nfts')
  .description('Browse and manage NFTs')
  .argument('[action]', 'Action: list, get', 'list')
  .option('--id <id>', 'NFT ID')
  .option('--owner <address>', 'Filter by owner address')
  .option('--collection <id>', 'Filter by collection ID')
  .option('--chain <chainId>', 'Filter by chain ID')
  .option('--standard <standard>', 'Filter by token standard')
  .action(async (action, options) => {
    try {
      loadConfig();
      const db = new Web3Database();
      await db.connect();

      switch (action) {
        case 'list': {
          const nfts = await db.listNfts({
            owner_address: options.owner,
            collection_id: options.collection,
            chain_id: options.chain ? Number(options.chain) : undefined,
            token_standard: options.standard,
          });
          console.log(`\nNFTs (${nfts.length}):`);
          console.log('-'.repeat(120));
          for (const n of nfts) {
            const name = n.name ?? `#${n.token_id}`;
            console.log(`${n.id} | ${name} | ${n.contract_address.slice(0, 10)}... | ${n.token_standard} | Chain ${n.chain_id} | Owner: ${n.owner_address.slice(0, 10)}...`);
          }
          break;
        }
        case 'get': {
          if (!options.id) { logger.error('NFT ID required (--id)'); process.exit(1); }
          const nft = await db.getNft(options.id);
          if (!nft) { logger.error('NFT not found'); process.exit(1); }
          console.log(JSON.stringify(nft, null, 2));
          break;
        }
        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Collections command
program
  .command('collections')
  .description('Manage NFT collections')
  .argument('[action]', 'Action: list, get', 'list')
  .option('--id <id>', 'Collection ID')
  .option('--slug <slug>', 'Collection slug')
  .option('--chain <chainId>', 'Filter by chain ID')
  .option('--verified', 'Only verified collections')
  .action(async (action, options) => {
    try {
      loadConfig();
      const db = new Web3Database();
      await db.connect();

      switch (action) {
        case 'list': {
          const collections = await db.listCollections({
            chain_id: options.chain ? Number(options.chain) : undefined,
            is_verified: options.verified ? true : undefined,
          });
          console.log(`\nCollections (${collections.length}):`);
          console.log('-'.repeat(120));
          for (const c of collections) {
            const verified = c.is_verified ? 'Verified' : '';
            const supply = c.total_supply ? `Supply: ${c.total_supply}` : '';
            const floor = c.floor_price ? `Floor: ${c.floor_price} ${c.floor_price_currency ?? ''}` : '';
            console.log(`${c.id} | ${c.name} | ${c.contract_address.slice(0, 10)}... | Chain ${c.chain_id} | ${supply} | ${floor} | ${verified}`);
          }
          break;
        }
        case 'get': {
          let collection = null;
          if (options.id) collection = await db.getCollection(options.id);
          else if (options.slug) collection = await db.getCollectionBySlug(options.slug);
          else { logger.error('ID or slug required'); process.exit(1); }
          if (!collection) { logger.error('Collection not found'); process.exit(1); }
          console.log(JSON.stringify(collection, null, 2));
          break;
        }
        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Token Gates command
program
  .command('gates')
  .description('Manage token gates')
  .argument('[action]', 'Action: list, get, check', 'list')
  .option('--id <id>', 'Gate ID')
  .option('-w, --workspace <id>', 'Workspace ID')
  .option('--type <type>', 'Gate type filter')
  .option('-u, --user <id>', 'User ID (for check)')
  .option('--wallet <address>', 'Wallet address (for check)')
  .action(async (action, options) => {
    try {
      const config = loadConfig();
      const db = new Web3Database();
      await db.connect();

      switch (action) {
        case 'list': {
          const gates = await db.listTokenGates({
            workspace_id: options.workspace,
            gate_type: options.type,
            is_active: true,
          });
          console.log(`\nToken Gates (${gates.length}):`);
          console.log('-'.repeat(100));
          for (const g of gates) {
            console.log(`${g.id} | ${g.name} | ${g.gate_type} | Target: ${g.target_type} | ${g.is_active ? 'Active' : 'Inactive'}`);
          }
          break;
        }
        case 'get': {
          if (!options.id) { logger.error('Gate ID required (--id)'); process.exit(1); }
          const gate = await db.getTokenGate(options.id);
          if (!gate) { logger.error('Token gate not found'); process.exit(1); }
          console.log(JSON.stringify(gate, null, 2));
          break;
        }
        case 'check': {
          if (!options.id) { logger.error('Gate ID required (--id)'); process.exit(1); }
          if (!options.user) { logger.error('User ID required (--user)'); process.exit(1); }
          if (!options.wallet) { logger.error('Wallet address required (--wallet)'); process.exit(1); }
          const result = await db.checkGate(options.id, options.user, options.wallet, config.gateCheckCacheTtl);
          console.log('\nGate Check Result');
          console.log('=================');
          console.log(`Gate:    ${result.gate_name}`);
          console.log(`Passed:  ${result.passed ? 'Yes' : 'No'}`);
          console.log(`Cached:  ${result.cached ? 'Yes' : 'No'}`);
          if (result.failure_reason) console.log(`Reason:  ${result.failure_reason}`);
          if (result.evidence) console.log(`Evidence: ${JSON.stringify(result.evidence)}`);
          break;
        }
        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// DAOs command
program
  .command('daos')
  .description('Manage DAOs')
  .argument('[action]', 'Action: list, get', 'list')
  .option('--id <id>', 'DAO ID')
  .option('--slug <slug>', 'DAO slug')
  .option('-w, --workspace <id>', 'Workspace ID')
  .option('--chain <chainId>', 'Filter by chain ID')
  .action(async (action, options) => {
    try {
      loadConfig();
      const db = new Web3Database();
      await db.connect();

      switch (action) {
        case 'list': {
          const daos = await db.listDaos(
            options.workspace,
            options.chain ? Number(options.chain) : undefined,
            true,
          );
          console.log(`\nDAOs (${daos.length}):`);
          console.log('-'.repeat(100));
          for (const d of daos) {
            const governor = d.governor_type ? ` (${d.governor_type})` : '';
            const snapshot = d.snapshot_space ? ` [Snapshot: ${d.snapshot_space}]` : '';
            console.log(`${d.id} | ${d.name} | Chain ${d.chain_id}${governor}${snapshot}`);
          }
          break;
        }
        case 'get': {
          let dao = null;
          if (options.id) dao = await db.getDao(options.id);
          else if (options.slug) dao = await db.getDaoBySlug(options.slug);
          else { logger.error('ID or slug required'); process.exit(1); }
          if (!dao) { logger.error('DAO not found'); process.exit(1); }
          console.log(JSON.stringify(dao, null, 2));
          break;
        }
        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Proposals command
program
  .command('proposals')
  .description('Manage DAO proposals')
  .argument('[action]', 'Action: list, get', 'list')
  .option('--id <id>', 'Proposal ID')
  .option('--dao <id>', 'DAO ID')
  .option('--status <status>', 'Filter by status')
  .action(async (action, options) => {
    try {
      loadConfig();
      const db = new Web3Database();
      await db.connect();

      switch (action) {
        case 'list': {
          const proposals = await db.listProposals({
            dao_id: options.dao,
            status: options.status,
          });
          console.log(`\nProposals (${proposals.length}):`);
          console.log('-'.repeat(120));
          for (const p of proposals) {
            const endTime = p.end_time ? new Date(p.end_time).toLocaleDateString() : 'N/A';
            console.log(`${p.id} | ${p.title.slice(0, 50)} | ${p.status} | For: ${p.votes_for} Against: ${p.votes_against} | Ends: ${endTime}`);
          }
          break;
        }
        case 'get': {
          if (!options.id) { logger.error('Proposal ID required (--id)'); process.exit(1); }
          const proposal = await db.getProposal(options.id);
          if (!proposal) { logger.error('Proposal not found'); process.exit(1); }
          console.log(JSON.stringify(proposal, null, 2));
          break;
        }
        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Transactions command
program
  .command('transactions')
  .description('View blockchain transactions')
  .argument('[action]', 'Action: list, get', 'list')
  .option('--id <id>', 'Transaction ID')
  .option('--hash <hash>', 'Transaction hash')
  .option('--chain <chainId>', 'Chain ID')
  .option('--from <address>', 'From address')
  .option('--to <address>', 'To address')
  .option('--type <type>', 'Transaction type')
  .option('-l, --limit <limit>', 'Result limit', '50')
  .action(async (action, options) => {
    try {
      loadConfig();
      const db = new Web3Database();
      await db.connect();

      switch (action) {
        case 'list': {
          const txs = await db.listTransactions({
            chain_id: options.chain ? Number(options.chain) : undefined,
            from_address: options.from,
            to_address: options.to,
            transaction_type: options.type,
          }, Number(options.limit));
          console.log(`\nTransactions (${txs.length}):`);
          console.log('-'.repeat(140));
          for (const tx of txs) {
            const valueUsd = tx.value_usd ? ` ($${tx.value_usd})` : '';
            console.log(`${tx.id} | ${tx.transaction_hash.slice(0, 16)}... | ${tx.status ?? 'unknown'} | ${tx.from_address.slice(0, 10)}... -> ${(tx.to_address ?? 'contract').slice(0, 10)}... | ${tx.value}${valueUsd}`);
          }
          break;
        }
        case 'get': {
          let tx = null;
          if (options.id) {
            tx = await db.getTransaction(options.id);
          } else if (options.hash && options.chain) {
            tx = await db.getTransactionByHash(options.hash, Number(options.chain));
          } else {
            logger.error('ID or (hash + chain) required');
            process.exit(1);
          }
          if (!tx) { logger.error('Transaction not found'); process.exit(1); }
          console.log(JSON.stringify(tx, null, 2));
          break;
        }
        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
