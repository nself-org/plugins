/**
 * Configuration loader for Meetings plugin
 */

import * as dotenv from 'dotenv';
import { MeetingsConfig } from './types.js';

// Load environment variables
dotenv.config();

export function loadConfig(): MeetingsConfig {
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
      port: parseInt(process.env.MEETINGS_PLUGIN_PORT ?? process.env.PORT ?? '3710', 10),
      host: process.env.HOST ?? '0.0.0.0',
    },

    calendar: {
      default_timezone: process.env.MEETINGS_DEFAULT_TIMEZONE ?? 'UTC',
      business_hours_start: process.env.MEETINGS_BUSINESS_HOURS_START ?? '09:00',
      business_hours_end: process.env.MEETINGS_BUSINESS_HOURS_END ?? '17:00',
      slot_duration_minutes: parseInt(process.env.MEETINGS_SLOT_DURATION ?? '30', 10),
      suggestion_window_days: parseInt(process.env.MEETINGS_SUGGESTION_WINDOW ?? '30', 10),
    },

    rooms: {
      default_buffer_minutes: parseInt(process.env.MEETINGS_DEFAULT_BUFFER ?? '15', 10),
      max_advance_booking_days: parseInt(process.env.MEETINGS_MAX_ADVANCE_BOOKING ?? '90', 10),
      auto_release_minutes: parseInt(process.env.MEETINGS_AUTO_RELEASE ?? '15', 10),
    },

    sync: {
      sync_interval_minutes: parseInt(process.env.MEETINGS_SYNC_INTERVAL ?? '15', 10),
      google: {
        client_id: process.env.GOOGLE_CALENDAR_CLIENT_ID ?? '',
        client_secret: process.env.GOOGLE_CALENDAR_CLIENT_SECRET ?? '',
        redirect_uri: process.env.GOOGLE_CALENDAR_REDIRECT_URI ?? 'http://localhost:3710/api/v1/sync/google/callback',
      },
      outlook: {
        client_id: process.env.OUTLOOK_CALENDAR_CLIENT_ID ?? '',
        client_secret: process.env.OUTLOOK_CALENDAR_CLIENT_SECRET ?? '',
        redirect_uri: process.env.OUTLOOK_CALENDAR_REDIRECT_URI ?? 'http://localhost:3710/api/v1/sync/outlook/callback',
      },
    },

    reminders: {
      default_reminder_minutes: [15, 60, 1440],
      max_reminders_per_event: parseInt(process.env.MEETINGS_MAX_REMINDERS ?? '5', 10),
    },
  };
}

export const config = loadConfig();
