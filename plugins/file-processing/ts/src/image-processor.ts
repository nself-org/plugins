/**
 * Image processor for nTV endpoints - sharp-based poster, sprite, and optimization operations
 */

import { createLogger } from '@nself/plugin-utils';
import sharp from 'sharp';
import ffmpeg from 'fluent-ffmpeg';
import { mkdir, readdir, stat, unlink, writeFile } from 'fs/promises';
import { tmpdir } from 'os';
import { basename, extname, join } from 'path';
import type { PosterOutput, SpriteOutput, OptimizeOutput } from './types.js';

const logger = createLogger('file-processing:image-processor');

/**
 * Generate poster thumbnails at multiple widths and formats from a source image.
 *
 * @param inputPath  - Absolute path to the source image
 * @param widths     - Target widths (height is calculated to preserve aspect ratio)
 * @param formats    - Output formats (webp, avif, jpeg, png)
 * @param outputDir  - Directory to write outputs (created if absent)
 * @returns Array of output descriptors
 */
export async function generatePosters(
  inputPath: string,
  widths: number[],
  formats: string[],
  outputDir: string,
): Promise<PosterOutput[]> {
  await mkdir(outputDir, { recursive: true });

  const outputs: PosterOutput[] = [];
  const baseName = basename(inputPath, extname(inputPath));

  for (const width of widths) {
    for (const format of formats) {
      const outFileName = `${baseName}_${width}.${format}`;
      const outPath = join(outputDir, outFileName);

      try {
        let pipeline = sharp(inputPath).resize({ width, withoutEnlargement: true });

        switch (format) {
          case 'webp':
            pipeline = pipeline.webp({ quality: 80 });
            break;
          case 'avif':
            pipeline = pipeline.avif({ quality: 65 });
            break;
          case 'jpeg':
          case 'jpg':
            pipeline = pipeline.jpeg({ quality: 80, mozjpeg: true });
            break;
          case 'png':
            pipeline = pipeline.png({ compressionLevel: 9 });
            break;
          default:
            pipeline = pipeline.webp({ quality: 80 });
        }

        await pipeline.toFile(outPath);

        const fileStat = await stat(outPath);
        outputs.push({
          width,
          format,
          path: outPath,
          size: fileStat.size,
        });
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error(`Failed to generate poster ${width}x ${format}`, { error: message });
      }
    }
  }

  return outputs;
}

/**
 * Extract frames from a video using ffmpeg, then stitch them into a sprite-sheet grid.
 * Also generates a WebVTT file mapping time ranges to sprite coordinates.
 *
 * @param inputPath  - Path to a video file (or a directory of pre-extracted frames)
 * @param grid       - Grid dimensions as "COLSxROWS" (e.g. "10x10")
 * @param thumbSize  - Individual thumbnail size as "WxH" (e.g. "320x180")
 * @param outputDir  - Directory to write sprite and VTT files
 * @returns Sprite descriptor with paths and frame count
 */
export async function generateSpriteSheet(
  inputPath: string,
  grid: string,
  thumbSize: string,
  outputDir: string,
): Promise<SpriteOutput> {
  await mkdir(outputDir, { recursive: true });

  const [cols, rows] = grid.split('x').map(Number);
  const [thumbW, thumbH] = thumbSize.split('x').map(Number);
  const maxFrames = cols * rows;

  // Step 1: Get video duration
  const duration = await getVideoDuration(inputPath);
  const interval = duration / maxFrames;

  // Step 2: Extract frames to a temp directory
  const framesDir = join(tmpdir(), `sprite-frames-${Date.now()}`);
  await mkdir(framesDir, { recursive: true });

  await extractFrames(inputPath, framesDir, interval, thumbW, thumbH, maxFrames);

  // Step 3: Read extracted frames sorted by name
  const frameFiles = (await readdir(framesDir))
    .filter((f) => f.endsWith('.jpg'))
    .sort();

  const frameCount = frameFiles.length;
  if (frameCount === 0) {
    throw new Error('No frames extracted from video');
  }

  // Step 4: Stitch frames into a sprite grid using sharp
  const actualCols = cols;
  const actualRows = Math.ceil(frameCount / actualCols);
  const spriteWidth = actualCols * thumbW;
  const spriteHeight = actualRows * thumbH;

  // Create a blank canvas
  const compositeInputs: sharp.OverlayOptions[] = [];

  for (let i = 0; i < frameCount; i++) {
    const col = i % actualCols;
    const row = Math.floor(i / actualCols);
    compositeInputs.push({
      input: join(framesDir, frameFiles[i]),
      left: col * thumbW,
      top: row * thumbH,
    });
  }

  const baseName = basename(inputPath, extname(inputPath));
  const spritePath = join(outputDir, `${baseName}_sprite.jpg`);

  await sharp({
    create: {
      width: spriteWidth,
      height: spriteHeight,
      channels: 3,
      background: { r: 0, g: 0, b: 0 },
    },
  })
    .composite(compositeInputs)
    .jpeg({ quality: 75 })
    .toFile(spritePath);

  // Step 5: Generate WebVTT
  const vttPath = join(outputDir, `${baseName}_sprite.vtt`);
  const vttLines = ['WEBVTT', ''];
  const spriteFileName = basename(spritePath);

  for (let i = 0; i < frameCount; i++) {
    const startTime = i * interval;
    const endTime = (i + 1) * interval;
    const col = i % actualCols;
    const row = Math.floor(i / actualCols);

    vttLines.push(formatVttTimestamp(startTime) + ' --> ' + formatVttTimestamp(endTime));
    vttLines.push(`${spriteFileName}#xywh=${col * thumbW},${row * thumbH},${thumbW},${thumbH}`);
    vttLines.push('');
  }

  await writeFile(vttPath, vttLines.join('\n'), 'utf-8');

  // Cleanup temp frames
  for (const f of frameFiles) {
    await unlink(join(framesDir, f)).catch(() => {});
  }

  return {
    sprite_path: spritePath,
    vtt_path: vttPath,
    frame_count: frameCount,
  };
}

/**
 * Optimize an image file: convert format, adjust quality, optionally strip EXIF.
 *
 * @param inputPath  - Path to source image
 * @param format     - Target format (webp, avif, jpeg, png)
 * @param quality    - Quality 1-100
 * @param stripExif  - Whether to strip EXIF/metadata
 * @param outputDir  - Directory for output file
 * @returns Optimization result with size savings
 */
export async function optimizeImage(
  inputPath: string,
  format: string,
  quality: number,
  stripExif: boolean,
  outputDir: string,
): Promise<OptimizeOutput> {
  await mkdir(outputDir, { recursive: true });

  const originalStat = await stat(inputPath);
  const originalSize = originalStat.size;

  const baseName = basename(inputPath, extname(inputPath));
  const outputPath = join(outputDir, `${baseName}_optimized.${format}`);

  let pipeline = sharp(inputPath);

  if (stripExif) {
    pipeline = pipeline.rotate(); // auto-rotate based on EXIF then strip
  }

  switch (format) {
    case 'webp':
      pipeline = pipeline.webp({ quality });
      break;
    case 'avif':
      pipeline = pipeline.avif({ quality });
      break;
    case 'jpeg':
    case 'jpg':
      pipeline = pipeline.jpeg({ quality, mozjpeg: true });
      break;
    case 'png':
      pipeline = pipeline.png({ compressionLevel: Math.round((100 - quality) / 11) });
      break;
    default:
      pipeline = pipeline.webp({ quality });
  }

  await pipeline.toFile(outputPath);

  const optimizedStat = await stat(outputPath);
  const optimizedSize = optimizedStat.size;
  const savingsPercent = originalSize > 0
    ? Math.round(((originalSize - optimizedSize) / originalSize) * 100)
    : 0;

  return {
    output_path: outputPath,
    original_size: originalSize,
    optimized_size: optimizedSize,
    savings_percent: savingsPercent,
  };
}

// =============================================================================
// Internal helpers
// =============================================================================

/** Get video duration in seconds via ffprobe */
function getVideoDuration(videoPath: string): Promise<number> {
  return new Promise((resolve, reject) => {
    ffmpeg.ffprobe(videoPath, (err, metadata) => {
      if (err) return reject(err);
      resolve(metadata.format.duration ?? 0);
    });
  });
}

/** Extract N frames from video at a given interval, resized to thumbW x thumbH */
function extractFrames(
  videoPath: string,
  outputDir: string,
  intervalSec: number,
  thumbW: number,
  thumbH: number,
  maxFrames: number,
): Promise<void> {
  return new Promise((resolve, reject) => {
    ffmpeg(videoPath)
      .outputOptions([
        `-vf`, `fps=1/${intervalSec},scale=${thumbW}:${thumbH}:force_original_aspect_ratio=decrease,pad=${thumbW}:${thumbH}:(ow-iw)/2:(oh-ih)/2`,
        `-frames:v`, `${maxFrames}`,
        `-q:v`, `5`,
      ])
      .output(join(outputDir, 'frame_%04d.jpg'))
      .on('end', () => resolve())
      .on('error', (err) => reject(err))
      .run();
  });
}

/** Format seconds to VTT timestamp HH:MM:SS.mmm */
function formatVttTimestamp(seconds: number): string {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  const ms = Math.round((s - Math.floor(s)) * 1000);
  return (
    String(h).padStart(2, '0') +
    ':' +
    String(m).padStart(2, '0') +
    ':' +
    String(Math.floor(s)).padStart(2, '0') +
    '.' +
    String(ms).padStart(3, '0')
  );
}
