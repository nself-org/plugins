#!/usr/bin/env node
/**
 * CLI for recommendation-engine plugin
 */

import { createLogger } from '@nself/plugin-utils';
import { config } from './config.js';
import { db } from './database.js';
import { RecommendationEngine } from './engine.js';

const logger = createLogger('recommendation:cli');

const command = process.argv[2];

async function main() {
  switch (command) {
    case 'init':
      await init();
      break;
    case 'server':
      await startServer();
      break;
    case 'status':
      await showStatus();
      break;
    case 'rebuild':
      await triggerRebuild();
      break;
    case 'recommendations':
      await getRecommendations();
      break;
    case 'similar':
      await getSimilar();
      break;
    case 'stats':
      await showStats();
      break;
    default:
      showHelp();
      break;
  }
}

async function init() {
  logger.info('Initializing recommendation engine...');

  try {
    await db.initializeSchema();
    logger.info('Database schema initialized');

    const stats = await db.getStats();
    logger.info('Current state:');
    logger.info(`  Users: ${stats.total_users}`);
    logger.info(`  Items: ${stats.total_items}`);
    logger.info(`  Cached recommendations: ${stats.active_cached_recommendations}`);
    logger.info(`  Similar pairs: ${stats.total_similar_pairs}`);

    logger.info('Configuration:');
    logger.info(`  Port: ${config.server.port}`);
    logger.info(`  Collaborative weight: ${config.engine.collaborativeWeight}`);
    logger.info(`  Content-based weight: ${config.engine.contentWeight}`);
    logger.info(`  Cache TTL: ${config.engine.cacheTtlSeconds}s`);
    logger.info(`  Rebuild interval: ${config.engine.rebuildIntervalHours}h`);
    logger.info(`  Min interactions for CF: ${config.engine.minInteractionsForCollaborative}`);
    logger.info(`  Redis: ${config.redis.enabled ? 'enabled' : 'disabled'}`);

    logger.info('Initialization complete!');
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Initialization failed', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

async function startServer() {
  logger.info('Starting recommendation engine server...');
  await import('./server.js');
}

async function showStatus() {
  try {
    const modelState = await db.getModelState();

    if (!modelState) {
      logger.info('Model Status: Not initialized');
      logger.info('  Run "nself-recommendation-engine rebuild" to build the model');
    } else {
      logger.info('Model Status:');
      logger.info(`  Ready: ${modelState.model_ready ? 'yes' : 'no'}`);
      logger.info(`  Last rebuild: ${modelState.last_rebuild ? new Date(modelState.last_rebuild).toLocaleString() : 'never'}`);
      logger.info(`  Items: ${modelState.item_count}`);
      logger.info(`  Users: ${modelState.user_count}`);
      if (modelState.rebuild_duration_seconds !== null) {
        logger.info(`  Rebuild duration: ${modelState.rebuild_duration_seconds.toFixed(1)}s`);
      }
    }

    const stats = await db.getStats();
    logger.info('');
    logger.info('Database Stats:');
    logger.info(`  User profiles: ${stats.total_users}`);
    logger.info(`  Item profiles: ${stats.total_items}`);
    logger.info(`  Active cached recommendations: ${stats.active_cached_recommendations}`);
    logger.info(`  Similar item pairs: ${stats.total_similar_pairs}`);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Error getting status', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

async function triggerRebuild() {
  logger.info('Triggering model rebuild...');

  try {
    const engine = new RecommendationEngine(db);
    await engine.initialize();

    const result = await engine.rebuild();

    if (result.started) {
      logger.success('Model rebuild completed', {
        duration: `${result.estimated_time_seconds}s`,
      });
    } else {
      logger.warn('Rebuild was skipped (already in progress)');
    }

    await engine.shutdown();
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Rebuild failed', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

async function getRecommendations() {
  const userId = process.argv[3];
  if (!userId) {
    logger.error('Usage: nself-recommendation-engine recommendations <user_id> [limit]');
    process.exit(1);
  }

  const limit = parseInt(process.argv[4] ?? '10', 10);

  try {
    const engine = new RecommendationEngine(db);
    await engine.initialize();

    const recommendations = await engine.getRecommendations(userId, limit);

    if (recommendations.length === 0) {
      logger.info('No recommendations found. Try populating item data and rebuilding the model.');
    } else {
      logger.info(`Recommendations for ${userId} (${recommendations.length}):`);
      for (const rec of recommendations) {
        logger.info(`  [${rec.score.toFixed(3)}] ${rec.title} (${rec.type ?? 'unknown'})`);
        logger.info(`    ${rec.reason}`);
      }
    }

    await engine.shutdown();
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Error getting recommendations', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

async function getSimilar() {
  const mediaId = process.argv[3];
  if (!mediaId) {
    logger.error('Usage: nself-recommendation-engine similar <media_id> [limit]');
    process.exit(1);
  }

  const limit = parseInt(process.argv[4] ?? '10', 10);

  try {
    const engine = new RecommendationEngine(db);
    await engine.initialize();

    const similar = await engine.getSimilarItems(mediaId, limit);

    if (similar.length === 0) {
      logger.info('No similar items found. Ensure items are loaded and model is rebuilt.');
    } else {
      logger.info(`Items similar to ${mediaId} (${similar.length}):`);
      for (const item of similar) {
        logger.info(`  [${item.similarity_score.toFixed(3)}] ${item.title} (${item.type ?? 'unknown'})`);
      }
    }

    await engine.shutdown();
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Error getting similar items', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

async function showStats() {
  try {
    const stats = await db.getStats();
    logger.info('Recommendation Engine Stats:');
    logger.info(`  User profiles: ${stats.total_users}`);
    logger.info(`  Item profiles: ${stats.total_items}`);
    logger.info(`  Active cached recommendations: ${stats.active_cached_recommendations}`);
    logger.info(`  Similar item pairs: ${stats.total_similar_pairs}`);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Error getting stats', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

function showHelp() {
  logger.info('nself-recommendation-engine - Hybrid recommendation engine CLI');
  logger.info('');
  logger.info('Usage:');
  logger.info('  nself-recommendation-engine <command> [options]');
  logger.info('');
  logger.info('Commands:');
  logger.info('  init                          Initialize and verify setup');
  logger.info('  server                        Start the recommendation server');
  logger.info('  status                        Show model and system status');
  logger.info('  rebuild                       Trigger a model rebuild');
  logger.info('  recommendations <user_id>     Get recommendations for a user');
  logger.info('  similar <media_id>            Get similar items for a media item');
  logger.info('  stats                         Show database statistics');
  logger.info('');
  logger.info('Environment:');
  logger.info('  RECOMMENDATION_PORT           Server port (default: 5004)');
  logger.info('  DATABASE_URL                  PostgreSQL connection URL');
  logger.info('  REDIS_URL                     Redis connection URL (optional)');
  logger.info('  COLLABORATIVE_WEIGHT          Weight for CF (default: 0.6)');
  logger.info('  CONTENT_WEIGHT                Weight for CB (default: 0.4)');
  logger.info('  CACHE_TTL_SECONDS             Cache TTL (default: 3600)');
  logger.info('  REBUILD_INTERVAL_HOURS        Rebuild interval (default: 24)');
  logger.info('  MIN_INTERACTIONS_FOR_COLLABORATIVE  Min interactions for CF (default: 5)');
  logger.info('');
  logger.info('For full functionality, use the nself plugin commands:');
  logger.info('  nself plugin recommendation-engine <action>');
}

main().catch((error) => {
  const message = error instanceof Error ? error.message : 'Unknown error';
  logger.error('Fatal error', { error: message });
  process.exit(1);
});
