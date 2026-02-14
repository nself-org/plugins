/**
 * Configuration loader for the recommendation-engine plugin
 */

import * as dotenv from 'dotenv';
import { RecommendationConfig } from './types.js';

dotenv.config();

export function loadConfig(): RecommendationConfig {
  const collaborativeWeight = parseFloat(process.env.COLLABORATIVE_WEIGHT ?? '0.6');
  const contentWeight = parseFloat(process.env.CONTENT_WEIGHT ?? '0.4');

  // Validate weights sum to ~1.0
  const totalWeight = collaborativeWeight + contentWeight;
  if (Math.abs(totalWeight - 1.0) > 0.01) {
    throw new Error(
      `COLLABORATIVE_WEIGHT (${collaborativeWeight}) + CONTENT_WEIGHT (${contentWeight}) = ${totalWeight}, must sum to 1.0`
    );
  }

  return {
    database: {
      host: process.env.POSTGRES_HOST ?? 'localhost',
      port: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
      database: process.env.POSTGRES_DB ?? 'nself',
      user: process.env.POSTGRES_USER ?? 'postgres',
      password: process.env.POSTGRES_PASSWORD ?? '',
      ssl: process.env.POSTGRES_SSL === 'true',
    },

    server: {
      port: parseInt(process.env.RECOMMENDATION_PORT ?? process.env.PORT ?? '5004', 10),
      host: process.env.HOST ?? '0.0.0.0',
    },

    redis: {
      url: process.env.REDIS_URL ?? 'redis://localhost:6379',
      enabled: !!process.env.REDIS_URL,
    },

    engine: {
      collaborativeWeight,
      contentWeight,
      cacheTtlSeconds: parseInt(process.env.CACHE_TTL_SECONDS ?? '3600', 10),
      rebuildIntervalHours: parseInt(process.env.REBUILD_INTERVAL_HOURS ?? '24', 10),
      minInteractionsForCollaborative: parseInt(process.env.MIN_INTERACTIONS_FOR_COLLABORATIVE ?? '5', 10),
    },
  };
}

export const config = loadConfig();
