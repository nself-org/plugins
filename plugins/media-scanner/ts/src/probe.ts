/**
 * Media Scanner - FFprobe Wrapper
 * Extract media information using ffprobe via child_process
 */

import { execFile } from 'node:child_process';
import { access, constants } from 'node:fs/promises';
import { createLogger } from '@nself/plugin-utils';
import type { MediaInfo, FFprobeOutput, FFprobeStream } from './types.js';

const logger = createLogger('media-scanner:probe');

/** Maximum execution time for ffprobe (30 seconds) */
const PROBE_TIMEOUT_MS = 30_000;

/** Default ffprobe binary path */
const FFPROBE_BIN = process.env.FFPROBE_PATH ?? 'ffprobe';

/**
 * Run ffprobe on a media file and return structured media information.
 */
export async function probeFile(filePath: string): Promise<MediaInfo> {
  // Verify file exists and is readable
  await access(filePath, constants.R_OK);

  const output = await runFFprobe(filePath);
  return parseFFprobeOutput(output);
}

/**
 * Execute ffprobe and return the parsed JSON output.
 */
function runFFprobe(filePath: string): Promise<FFprobeOutput> {
  return new Promise((resolve, reject) => {
    const args = [
      '-v', 'quiet',
      '-print_format', 'json',
      '-show_format',
      '-show_streams',
      filePath,
    ];

    execFile(FFPROBE_BIN, args, { timeout: PROBE_TIMEOUT_MS, maxBuffer: 10 * 1024 * 1024 }, (error, stdout, stderr) => {
      if (error) {
        const message = error.message || 'ffprobe execution failed';
        logger.error('FFprobe failed', { path: filePath, error: message, stderr });
        reject(new Error(`FFprobe failed for ${filePath}: ${message}`));
        return;
      }

      try {
        const parsed = JSON.parse(stdout) as FFprobeOutput;
        if (!parsed.format || !parsed.streams) {
          reject(new Error(`FFprobe returned incomplete data for ${filePath}`));
          return;
        }
        resolve(parsed);
      } catch (parseError) {
        const msg = parseError instanceof Error ? parseError.message : 'JSON parse error';
        logger.error('Failed to parse ffprobe output', { path: filePath, error: msg });
        reject(new Error(`Failed to parse ffprobe output for ${filePath}: ${msg}`));
      }
    });
  });
}

/**
 * Parse raw ffprobe JSON output into a structured MediaInfo object.
 */
function parseFFprobeOutput(output: FFprobeOutput): MediaInfo {
  const videoStreams = output.streams.filter(s => s.codec_type === 'video');
  const audioStreams = output.streams.filter(s => s.codec_type === 'audio');
  const subtitleStreams = output.streams.filter(s => s.codec_type === 'subtitle');

  // Primary video stream
  const primaryVideo = videoStreams[0];

  // Duration from format (more reliable) or video stream
  const durationSeconds = parseDuration(output.format.duration)
    ?? parseDuration(primaryVideo?.duration);

  // Video resolution
  let videoResolution: string | null = null;
  if (primaryVideo?.width && primaryVideo?.height) {
    videoResolution = `${primaryVideo.width}x${primaryVideo.height}`;
  }

  // Video bitrate: prefer stream-level, fall back to format-level
  const videoBitrate = parseBitrate(primaryVideo?.bit_rate)
    ?? parseBitrate(output.format.bit_rate);

  // Audio languages
  const audioLanguages = extractLanguages(audioStreams);
  const subtitleLanguages = extractLanguages(subtitleStreams);

  return {
    duration_seconds: durationSeconds ?? 0,
    video_codec: primaryVideo?.codec_name ?? null,
    video_resolution: videoResolution,
    video_bitrate: videoBitrate,
    audio_tracks: audioStreams.length,
    audio_languages: audioLanguages,
    subtitle_tracks: subtitleStreams.length,
    subtitle_languages: subtitleLanguages,
  };
}

/**
 * Parse a duration string (seconds as a string) into a number.
 */
function parseDuration(value: string | undefined): number | null {
  if (!value) return null;
  const num = parseFloat(value);
  if (isNaN(num) || num <= 0) return null;
  return Math.round(num * 100) / 100;
}

/**
 * Parse a bitrate string into an integer (bits per second).
 */
function parseBitrate(value: string | undefined): number | null {
  if (!value) return null;
  const num = parseInt(value, 10);
  if (isNaN(num) || num <= 0) return null;
  return num;
}

/**
 * Extract unique language tags from streams.
 */
function extractLanguages(streams: FFprobeStream[]): string[] {
  const languages = new Set<string>();
  for (const stream of streams) {
    const lang = stream.tags?.language;
    if (lang && lang !== 'und' && lang !== 'unk') {
      languages.add(lang);
    }
  }
  return Array.from(languages);
}

/**
 * Check if ffprobe is available on the system.
 */
export function checkFFprobeAvailable(): Promise<boolean> {
  return new Promise((resolve) => {
    execFile(FFPROBE_BIN, ['-version'], { timeout: 5000 }, (error) => {
      resolve(!error);
    });
  });
}
