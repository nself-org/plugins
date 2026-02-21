/**
 * Request Validation Schemas
 * Zod schemas for validating API request bodies
 */

import { z } from 'zod';

// Content type enum
export const ContentTypeSchema = z.enum([
  'movie',
  'episode',
  'video',
  'audio',
  'article',
  'course',
]);

// Progress action enum
export const ProgressActionSchema = z.enum([
  'play',
  'pause',
  'seek',
  'complete',
  'resume',
]);

// Update progress request schema
export const UpdateProgressSchema = z.object({
  user_id: z.string().min(1, 'user_id is required'),
  content_type: ContentTypeSchema,
  content_id: z.string().min(1, 'content_id is required'),
  position_seconds: z.number().min(0, 'position_seconds must be >= 0'),
  duration_seconds: z.number().min(0).optional(),
  device_id: z.string().optional(),
  audio_track: z.string().optional(),
  subtitle_track: z.string().optional(),
  quality: z.string().optional(),
  metadata: z.record(z.string(), z.unknown()).optional(),
});

export type UpdateProgressInput = z.infer<typeof UpdateProgressSchema>;

// Add to watchlist request schema
export const AddToWatchlistSchema = z.object({
  user_id: z.string().min(1, 'user_id is required'),
  content_type: ContentTypeSchema,
  content_id: z.string().min(1, 'content_id is required'),
  priority: z.number().int().min(0).max(10).optional(),
  added_from: z.string().optional(),
  notes: z.string().optional(),
});

export type AddToWatchlistInput = z.infer<typeof AddToWatchlistSchema>;

// Update watchlist request schema
export const UpdateWatchlistSchema = z.object({
  priority: z.number().int().min(0).max(10).optional(),
  notes: z.string().optional(),
});

export type UpdateWatchlistInput = z.infer<typeof UpdateWatchlistSchema>;

// Add to favorites request schema
export const AddToFavoritesSchema = z.object({
  user_id: z.string().min(1, 'user_id is required'),
  content_type: ContentTypeSchema,
  content_id: z.string().min(1, 'content_id is required'),
});

export type AddToFavoritesInput = z.infer<typeof AddToFavoritesSchema>;

// Create history request schema
export const CreateHistorySchema = z.object({
  user_id: z.string().min(1, 'user_id is required'),
  content_type: ContentTypeSchema,
  content_id: z.string().min(1, 'content_id is required'),
  action: ProgressActionSchema,
  position_seconds: z.number().min(0).optional(),
  device_id: z.string().optional(),
  session_id: z.string().optional(),
});

export type CreateHistoryInput = z.infer<typeof CreateHistorySchema>;

/**
 * Validation helper that formats zod errors nicely
 */
export function formatZodError(error: z.ZodError): string {
  const errors = error.issues.map((err: z.ZodIssue) => {
    const path = err.path.join('.');
    return `${path}: ${err.message}`;
  });

  return errors.join(', ');
}
