#!/usr/bin/env node
/**
 * Media Processing Plugin CLI
 * Command-line interface for the media processing plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { MediaProcessingDatabase } from './database.js';
import { startServer } from './server.js';
import { FFmpegClient } from './ffmpeg.js';
import type { CreateJobInput } from './types.js';

const logger = createLogger('media-processing:cli');

const program = new Command();

program
  .name('nself-media-processing')
  .description('Media processing plugin for nself - FFmpeg-based encoding and streaming')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig();
      const db = new MediaProcessingDatabase();
      await db.connect();
      await db.initializeSchema();

      logger.info('Schema initialized successfully');

      // Create a default profile if none exists
      const profiles = await db.listEncodingProfiles();
      if (profiles.length === 0) {
        await db.createEncodingProfile({
          name: 'default',
          description: 'Default encoding profile with 1080p, 720p, and 480p',
          is_default: true,
        });
        logger.info('Created default encoding profile');
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Initialization failed', { error: message });
      process.exit(1);
    }
  });

// Server command
program
  .command('server')
  .description('Start HTTP API server')
  .option('-p, --port <port>', 'Server port', '3019')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      await startServer(config);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Status command
program
  .command('status')
  .description('Show plugin status and statistics')
  .action(async () => {
    try {
      loadConfig();
      const db = new MediaProcessingDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nMedia Processing Plugin Status');
      console.log('==============================\n');
      console.log(`Total Jobs:      ${stats.totalJobs}`);
      console.log(`Pending:         ${stats.pendingJobs}`);
      console.log(`Running:         ${stats.runningJobs}`);
      console.log(`Completed:       ${stats.completedJobs}`);
      console.log(`Failed:          ${stats.failedJobs}`);
      console.log(`\nProfiles:        ${stats.profiles}`);

      if (stats.averageProcessingTimeSeconds) {
        console.log(`Avg Process Time: ${Math.round(stats.averageProcessingTimeSeconds)}s`);
      }

      if (stats.lastJobCompletedAt) {
        console.log(`Last Completed:  ${stats.lastJobCompletedAt.toISOString()}`);
      }

      console.log(`\nTotal Duration:  ${Math.round(stats.totalDurationSeconds)}s`);
      console.log(`Total Size:      ${(stats.totalFileSizeBytes / (1024 * 1024 * 1024)).toFixed(2)} GB`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Submit job command
program
  .command('submit <url>')
  .description('Submit a new encoding job')
  .option('-p, --profile <id>', 'Encoding profile ID')
  .option('-t, --type <type>', 'Input type (file, url, s3)', 'file')
  .option('--priority <priority>', 'Job priority', '0')
  .option('-o, --output <path>', 'Output base path')
  .action(async (url: string, options) => {
    try {
      loadConfig();
      const db = new MediaProcessingDatabase();
      await db.connect();

      const jobInput: CreateJobInput = {
        input_url: url,
        input_type: options.type as 'file' | 'url' | 's3',
        profile_id: options.profile,
        priority: parseInt(options.priority, 10),
        output_base_path: options.output,
      };

      const job = await db.createJob(jobInput);

      console.log('\nJob submitted successfully!');
      console.log('==========================');
      console.log(`Job ID:      ${job.id}`);
      console.log(`Input:       ${job.input_url}`);
      console.log(`Input Type:  ${job.input_type}`);
      console.log(`Status:      ${job.status}`);
      console.log(`Priority:    ${job.priority}`);
      console.log(`Created:     ${job.created_at.toISOString()}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Job submission failed', { error: message });
      process.exit(1);
    }
  });

// List jobs command
program
  .command('jobs')
  .description('List encoding jobs')
  .option('-s, --status <status>', 'Filter by status')
  .option('-l, --limit <limit>', 'Number of jobs to show', '20')
  .action(async (options) => {
    try {
      loadConfig();
      const db = new MediaProcessingDatabase();
      await db.connect();

      const jobs = await db.listJobs(
        options.status,
        parseInt(options.limit, 10)
      );

      console.log(`\nEncoding Jobs (${jobs.length})`);
      console.log('================================================');

      for (const job of jobs) {
        console.log(`\nID:       ${job.id}`);
        console.log(`Status:   ${job.status} (${job.progress.toFixed(1)}%)`);
        console.log(`Input:    ${job.input_url.substring(0, 60)}${job.input_url.length > 60 ? '...' : ''}`);
        console.log(`Created:  ${job.created_at.toISOString()}`);

        if (job.error_message) {
          console.log(`Error:    ${job.error_message}`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list jobs', { error: message });
      process.exit(1);
    }
  });

// List profiles command
program
  .command('profiles')
  .description('List encoding profiles')
  .action(async () => {
    try {
      loadConfig();
      const db = new MediaProcessingDatabase();
      await db.connect();

      const profiles = await db.listEncodingProfiles();

      console.log(`\nEncoding Profiles (${profiles.length})`);
      console.log('================================================');

      for (const profile of profiles) {
        console.log(`\nID:          ${profile.id}`);
        console.log(`Name:        ${profile.name}${profile.is_default ? ' (default)' : ''}`);
        console.log(`Container:   ${profile.container}`);
        console.log(`Video Codec: ${profile.video_codec}`);
        console.log(`Audio Codec: ${profile.audio_codec}`);
        console.log(`Preset:      ${profile.preset}`);
        console.log(`Resolutions: ${profile.resolutions.map(r => r.label).join(', ')}`);
        console.log(`HLS:         ${profile.hls_enabled ? 'Yes' : 'No'}`);
        console.log(`Thumbnails:  ${profile.thumbnail_enabled ? 'Yes' : 'No'}`);
        console.log(`Subtitles:   ${profile.subtitle_extract ? 'Yes' : 'No'}`);
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list profiles', { error: message });
      process.exit(1);
    }
  });

// Analyze media command
program
  .command('analyze <input>')
  .description('Analyze media file and show metadata')
  .action(async (input: string) => {
    try {
      const config = loadConfig();
      const ffmpeg = new FFmpegClient(config);

      console.log('\nAnalyzing media file...');
      console.log('======================\n');

      const metadata = await ffmpeg.probe(input);

      console.log(`Format:     ${metadata.format}`);
      console.log(`Duration:   ${metadata.duration ? Math.round(metadata.duration) : 'N/A'}s`);
      console.log(`Bitrate:    ${metadata.bitrate ? Math.round(metadata.bitrate / 1000) : 'N/A'} kbps`);
      console.log(`Size:       ${metadata.size ? (metadata.size / (1024 * 1024)).toFixed(2) : 'N/A'} MB`);

      if (metadata.streams) {
        console.log(`\nStreams (${metadata.streams.length}):`);
        console.log('-------------------');

        for (const stream of metadata.streams) {
          console.log(`\nType:       ${stream.codec_type}`);
          console.log(`Codec:      ${stream.codec_name}`);

          if (stream.codec_type === 'video') {
            console.log(`Resolution: ${stream.width}x${stream.height}`);
          } else if (stream.codec_type === 'audio') {
            console.log(`Channels:   ${stream.channels}`);
            console.log(`Sample Rate: ${stream.sample_rate}`);
          } else if (stream.codec_type === 'subtitle') {
            console.log(`Language:   ${stream.language ?? 'unknown'}`);
          }
        }
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Analysis failed', { error: message });
      process.exit(1);
    }
  });

// Statistics command
program
  .command('stats')
  .description('Show detailed processing statistics')
  .action(async () => {
    try {
      loadConfig();
      const db = new MediaProcessingDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nMedia Processing Statistics');
      console.log('===========================\n');

      console.log('Jobs:');
      console.log(`  Total:     ${stats.totalJobs}`);
      console.log(`  Pending:   ${stats.pendingJobs}`);
      console.log(`  Running:   ${stats.runningJobs}`);
      console.log(`  Completed: ${stats.completedJobs}`);
      console.log(`  Failed:    ${stats.failedJobs}`);

      const successRate = stats.totalJobs > 0
        ? ((stats.completedJobs / stats.totalJobs) * 100).toFixed(1)
        : '0.0';

      console.log(`\nSuccess Rate: ${successRate}%`);

      console.log('\nProcessing:');
      if (stats.averageProcessingTimeSeconds) {
        console.log(`  Avg Time:  ${Math.round(stats.averageProcessingTimeSeconds)}s`);
      }
      console.log(`  Total Time: ${(stats.totalDurationSeconds / 3600).toFixed(2)} hours`);

      console.log('\nStorage:');
      console.log(`  Total Size: ${(stats.totalFileSizeBytes / (1024 * 1024 * 1024)).toFixed(2)} GB`);

      if (stats.completedJobs > 0) {
        const avgSize = stats.totalFileSizeBytes / stats.completedJobs / (1024 * 1024);
        console.log(`  Avg Size:   ${avgSize.toFixed(2)} MB per job`);
      }

      console.log(`\nProfiles:     ${stats.profiles}`);

      if (stats.lastJobCompletedAt) {
        console.log(`\nLast Completed: ${stats.lastJobCompletedAt.toISOString()}`);
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get statistics', { error: message });
      process.exit(1);
    }
  });

program.parse();
