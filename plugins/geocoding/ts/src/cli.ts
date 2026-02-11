#!/usr/bin/env node
/**
 * Geocoding Plugin CLI
 * Command-line interface for the Geocoding plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { GeocodingDatabase } from './database.js';
import { createServer } from './server.js';

const logger = createLogger('geocoding:cli');

const program = new Command();

program
  .name('nself-geocoding')
  .description('Geocoding and location services plugin for nself')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      const config = loadConfig();
      const db = new GeocodingDatabase(undefined, 'primary', config.cacheTtlDays);
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
  .description('Start the API server')
  .option('-p, --port <port>', 'Server port', '3203')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });
      const server = await createServer(config);
      await server.start();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Geocode command
program
  .command('geocode')
  .description('Forward geocode an address')
  .argument('<address>', 'Address to geocode')
  .action(async (address) => {
    try {
      const config = loadConfig();
      const db = new GeocodingDatabase(undefined, 'primary', config.cacheTtlDays);
      await db.connect();

      // Check cache first
      const cached = await db.getCachedGeocodeAnyProvider('forward', address);
      if (cached) {
        console.log('\nGeocoding Result (cached):');
        console.log('=========================');
        console.log(`  Address:    ${cached.formatted_address ?? address}`);
        console.log(`  Lat:        ${cached.lat}`);
        console.log(`  Lng:        ${cached.lng}`);
        console.log(`  City:       ${cached.city ?? 'N/A'}`);
        console.log(`  State:      ${cached.state ?? 'N/A'} (${cached.state_code ?? ''})`);
        console.log(`  Country:    ${cached.country ?? 'N/A'} (${cached.country_code ?? ''})`);
        console.log(`  Postal:     ${cached.postal_code ?? 'N/A'}`);
        console.log(`  Provider:   ${cached.provider}`);
        console.log(`  Accuracy:   ${cached.accuracy ?? 'N/A'}`);
        console.log(`  Hit count:  ${cached.hit_count}`);
      } else {
        logger.info('No cached result found. External provider integration pending.');
        logger.info('Configure GEOCODING_PROVIDERS and run the server for live geocoding.');
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Geocode failed', { error: message });
      process.exit(1);
    }
  });

// Reverse command
program
  .command('reverse')
  .description('Reverse geocode coordinates')
  .argument('<lat>', 'Latitude')
  .argument('<lng>', 'Longitude')
  .action(async (lat, lng) => {
    try {
      const config = loadConfig();
      const db = new GeocodingDatabase(undefined, 'primary', config.cacheTtlDays);
      await db.connect();

      const queryText = `${lat},${lng}`;
      const cached = await db.getCachedGeocodeAnyProvider('reverse', queryText);

      if (cached) {
        console.log('\nReverse Geocoding Result (cached):');
        console.log('==================================');
        console.log(`  Address:    ${cached.formatted_address ?? 'N/A'}`);
        console.log(`  Lat:        ${cached.lat}`);
        console.log(`  Lng:        ${cached.lng}`);
        console.log(`  City:       ${cached.city ?? 'N/A'}`);
        console.log(`  State:      ${cached.state ?? 'N/A'}`);
        console.log(`  Country:    ${cached.country ?? 'N/A'}`);
        console.log(`  Provider:   ${cached.provider}`);
      } else {
        logger.info('No cached result found. External provider integration pending.');
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Reverse geocode failed', { error: message });
      process.exit(1);
    }
  });

// Search command
program
  .command('search')
  .description('Search for places')
  .argument('<query>', 'Search query')
  .option('--near <coords>', 'Near coordinates (lat,lng)')
  .option('--radius <meters>', 'Search radius in meters', '5000')
  .option('--category <category>', 'Filter by category')
  .option('-l, --limit <limit>', 'Number of results', '20')
  .action(async (query, options) => {
    try {
      const config = loadConfig();
      const db = new GeocodingDatabase(undefined, 'primary', config.cacheTtlDays);
      await db.connect();

      let lat: number | undefined;
      let lng: number | undefined;

      if (options.near) {
        const [latStr, lngStr] = options.near.split(',');
        lat = parseFloat(latStr);
        lng = parseFloat(lngStr);
      }

      const places = await db.searchPlaces({
        query,
        lat,
        lng,
        radius: parseInt(options.radius, 10),
        category: options.category,
        limit: parseInt(options.limit, 10),
      });

      console.log('\nPlace Search Results:');
      console.log('-'.repeat(100));
      for (const place of places) {
        console.log(
          `${place.name.padEnd(35)} | ` +
          `${(place.category ?? '').padEnd(15)} | ` +
          `${place.lat.toFixed(4)}, ${place.lng.toFixed(4)} | ` +
          `${place.rating ? `${place.rating}/5` : 'N/A'}`
        );
      }
      console.log(`\nTotal: ${places.length}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Search failed', { error: message });
      process.exit(1);
    }
  });

// Batch command
program
  .command('batch')
  .description('Batch geocode addresses from file')
  .argument('<file>', 'CSV file with addresses')
  .action(async (file) => {
    try {
      logger.info(`Batch geocoding from file: ${file}`);
      logger.info('Batch file processing will be implemented with the jobs plugin integration');

      // Placeholder for file reading and batch processing
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Batch geocode failed', { error: message });
      process.exit(1);
    }
  });

// Geofences command
program
  .command('geofences')
  .description('Manage geofences')
  .argument('[action]', 'Action: list, create', 'list')
  .option('--name <name>', 'Geofence name')
  .option('--lat <lat>', 'Center latitude')
  .option('--lng <lng>', 'Center longitude')
  .option('--radius <meters>', 'Radius in meters')
  .action(async (action, options) => {
    try {
      const config = loadConfig();
      const db = new GeocodingDatabase(undefined, 'primary', config.cacheTtlDays);
      await db.connect();

      switch (action) {
        case 'list': {
          const geofences = await db.listGeofences();
          console.log('\nGeofences:');
          console.log('-'.repeat(100));
          for (const fence of geofences) {
            console.log(
              `${fence.id.substring(0, 8)}... | ` +
              `${fence.name.padEnd(25)} | ` +
              `${fence.fence_type.padEnd(8)} | ` +
              `${fence.center_lat.toFixed(4)}, ${fence.center_lng.toFixed(4)} | ` +
              `${fence.radius_meters ? `${fence.radius_meters}m` : 'polygon'} | ` +
              `${fence.active ? 'ACTIVE' : 'INACTIVE'}`
            );
          }
          console.log(`\nTotal: ${geofences.length}`);
          break;
        }

        case 'create': {
          if (!options.name || !options.lat || !options.lng) {
            logger.error('Name, lat, and lng are required for creating a geofence');
            process.exit(1);
          }

          const geofence = await db.createGeofence({
            name: options.name,
            center_lat: parseFloat(options.lat),
            center_lng: parseFloat(options.lng),
            radius_meters: options.radius ? parseFloat(options.radius) : 500,
          });

          logger.success(`Geofence created: ${geofence.id}`);
          console.log(JSON.stringify(geofence, null, 2));
          break;
        }

        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Geofences command failed', { error: message });
      process.exit(1);
    }
  });

// Cache command
program
  .command('cache')
  .description('Manage geocoding cache')
  .argument('[action]', 'Action: stats, clear', 'stats')
  .option('--days <days>', 'Clear entries older than N days')
  .action(async (action, options) => {
    try {
      const config = loadConfig();
      const db = new GeocodingDatabase(undefined, 'primary', config.cacheTtlDays);
      await db.connect();

      switch (action) {
        case 'stats': {
          const stats = await db.getCacheStats();
          console.log('\nCache Statistics:');
          console.log('=================');
          console.log(`  Total entries:     ${stats.total_entries}`);
          console.log(`  Active entries:    ${stats.active_entries}`);
          console.log(`  Expired entries:   ${stats.expired_entries}`);
          console.log(`  Total hits:        ${stats.total_hits}`);
          console.log(`  Avg hits/entry:    ${stats.avg_hits_per_entry}`);
          console.log(`  Reuse rate:        ${stats.reuse_percentage}%`);

          if (Object.keys(stats.by_query_type).length > 0) {
            console.log('\n  By Query Type:');
            for (const [type, count] of Object.entries(stats.by_query_type)) {
              console.log(`    ${type}: ${count}`);
            }
          }

          if (Object.keys(stats.by_provider).length > 0) {
            console.log('\n  By Provider:');
            for (const [provider, count] of Object.entries(stats.by_provider)) {
              console.log(`    ${provider}: ${count}`);
            }
          }
          break;
        }

        case 'clear': {
          const days = options.days ? parseInt(options.days, 10) : undefined;
          const cleared = await db.clearCache(days);
          logger.success(`Cleared ${cleared} cache entries${days ? ` older than ${days} days` : ''}`);
          break;
        }

        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Cache command failed', { error: message });
      process.exit(1);
    }
  });

// Status command
program
  .command('status')
  .description('Show overall statistics')
  .action(async () => {
    try {
      const config = loadConfig();
      const db = new GeocodingDatabase(undefined, 'primary', config.cacheTtlDays);
      await db.connect();

      const stats = await db.getPluginStats();

      console.log('\nGeocoding Plugin Status');
      console.log('=======================');
      console.log(`  Cache entries:       ${stats.total_cache_entries}`);
      console.log(`  Cache hit rate:      ${stats.cache_hit_rate}%`);
      console.log(`  Geofences (total):   ${stats.total_geofences}`);
      console.log(`  Geofences (active):  ${stats.active_geofences}`);
      console.log(`  Geofence events:     ${stats.total_geofence_events}`);
      console.log(`  Places:              ${stats.total_places}`);

      if (Object.keys(stats.by_provider).length > 0) {
        console.log('\n  Cache by Provider:');
        for (const [provider, count] of Object.entries(stats.by_provider)) {
          console.log(`    ${provider}: ${count}`);
        }
      }

      console.log(`\n  Config:`);
      console.log(`    Providers:   ${config.providers.join(', ')}`);
      console.log(`    Cache TTL:   ${config.cacheTtlDays} days`);
      console.log(`    Max batch:   ${config.maxBatchSize}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
