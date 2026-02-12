/**
 * QA Validation (UPGRADE 1f)
 * Validates output quality of packaged media files
 */

import { promises as fs } from 'fs';
import { join, dirname } from 'path';
import { createLogger } from '@nself/plugin-utils';
import type { QAResult, QACheck, QAStatus } from './types.js';

const logger = createLogger('media-processing:qa');

export class QAValidator {
  /**
   * Validate all outputs in a directory
   */
  async validateOutput(outputDir: string): Promise<QAResult> {
    logger.info('Starting QA validation', { outputDir });

    const checks: QACheck[] = [];
    const issues: string[] = [];

    // Find HLS master playlist
    const masterPath = join(outputDir, 'hls', 'master.m3u8');
    let hasMaster = false;

    try {
      await fs.stat(masterPath);
      hasMaster = true;
    } catch {
      // Also check directly in output dir
      const altPath = join(outputDir, 'master.m3u8');
      try {
        await fs.stat(altPath);
        hasMaster = true;
      } catch {
        // No master playlist found
      }
    }

    if (hasMaster) {
      const actualPath = await this.findMasterPlaylist(outputDir);
      if (actualPath) {
        // Check 1: HLS playlist parses correctly
        const parseCheck = await this.checkHlsPlaylistParsing(actualPath);
        checks.push(parseCheck);
        if (parseCheck.status === 'fail') {
          issues.push(parseCheck.message);
        }

        // Check 2: All referenced segments exist
        const segmentCheck = await this.checkSegmentsExist(actualPath);
        checks.push(segmentCheck);
        if (segmentCheck.status === 'fail') {
          issues.push(segmentCheck.message);
        }

        // Check 3: EXT-X-INDEPENDENT-SEGMENTS present
        const independentCheck = await this.checkIndependentSegments(actualPath);
        checks.push(independentCheck);
        if (independentCheck.status === 'fail' || independentCheck.status === 'warn') {
          issues.push(independentCheck.message);
        }

        // Check 4: Variant playlists - segment durations within tolerance
        const durationCheck = await this.checkSegmentDurations(actualPath);
        checks.push(durationCheck);
        if (durationCheck.status === 'fail') {
          issues.push(durationCheck.message);
        }

        // Check 5: LANGUAGE attributes for audio/subtitle tracks
        const languageCheck = await this.checkLanguageAttributes(actualPath);
        checks.push(languageCheck);
        if (languageCheck.status === 'fail' || languageCheck.status === 'warn') {
          issues.push(languageCheck.message);
        }
      }
    } else {
      checks.push({
        name: 'hls_master_exists',
        status: 'warn',
        message: 'No HLS master playlist found - skipping HLS checks',
      });
    }

    // Check for DASH manifest if present
    const dashPath = join(outputDir, 'manifest.mpd');
    try {
      await fs.stat(dashPath);
      const dashCheck = await this.checkDashManifest(dashPath);
      checks.push(dashCheck);
      if (dashCheck.status === 'fail') {
        issues.push(dashCheck.message);
      }
    } catch {
      // No DASH manifest, that's fine
    }

    // Determine overall status
    const hasFailure = checks.some(c => c.status === 'fail');
    const hasWarning = checks.some(c => c.status === 'warn');
    const status: QAStatus = hasFailure ? 'fail' : hasWarning ? 'warn' : 'pass';

    const result: QAResult = {
      status,
      checks,
      issues,
      timestamp: new Date().toISOString(),
    };

    logger.info('QA validation complete', {
      status: result.status,
      totalChecks: checks.length,
      issues: issues.length,
    });

    return result;
  }

  /**
   * Find the master playlist in the output directory
   */
  private async findMasterPlaylist(outputDir: string): Promise<string | null> {
    const candidates = [
      join(outputDir, 'hls', 'master.m3u8'),
      join(outputDir, 'master.m3u8'),
    ];

    for (const path of candidates) {
      try {
        await fs.stat(path);
        return path;
      } catch {
        continue;
      }
    }

    return null;
  }

  /**
   * Check 1: HLS master playlist parses correctly
   */
  private async checkHlsPlaylistParsing(masterPath: string): Promise<QACheck> {
    try {
      const content = await fs.readFile(masterPath, 'utf-8');
      const lines = content.split('\n').map(l => l.trim()).filter(l => l.length > 0);

      // Must start with #EXTM3U
      if (!lines[0] || lines[0] !== '#EXTM3U') {
        return {
          name: 'hls_playlist_parse',
          status: 'fail',
          message: 'Master playlist does not start with #EXTM3U header',
        };
      }

      // Must have at least one #EXT-X-STREAM-INF
      const streamInfLines = lines.filter(l => l.startsWith('#EXT-X-STREAM-INF'));
      if (streamInfLines.length === 0) {
        return {
          name: 'hls_playlist_parse',
          status: 'fail',
          message: 'Master playlist has no #EXT-X-STREAM-INF entries',
        };
      }

      // Each STREAM-INF should have BANDWIDTH
      for (const line of streamInfLines) {
        if (!line.includes('BANDWIDTH=')) {
          return {
            name: 'hls_playlist_parse',
            status: 'fail',
            message: 'Stream variant missing BANDWIDTH attribute',
          };
        }
      }

      return {
        name: 'hls_playlist_parse',
        status: 'pass',
        message: `Master playlist valid with ${streamInfLines.length} variant(s)`,
        details: { variants: streamInfLines.length },
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return {
        name: 'hls_playlist_parse',
        status: 'fail',
        message: `Failed to read master playlist: ${message}`,
      };
    }
  }

  /**
   * Check 2: All referenced segments and variant playlists exist on disk
   */
  private async checkSegmentsExist(masterPath: string): Promise<QACheck> {
    try {
      const content = await fs.readFile(masterPath, 'utf-8');
      const lines = content.split('\n').map(l => l.trim()).filter(l => l.length > 0);
      const baseDir = dirname(masterPath);

      let missingFiles: string[] = [];
      let totalChecked = 0;

      // Check variant playlist files referenced in master
      for (const line of lines) {
        if (!line.startsWith('#') && line.endsWith('.m3u8')) {
          totalChecked++;
          const variantPath = join(baseDir, line);
          try {
            await fs.stat(variantPath);

            // Also check segments referenced in each variant
            const variantMissing = await this.checkVariantSegments(variantPath);
            missingFiles = missingFiles.concat(variantMissing);
            totalChecked += variantMissing.length; // approximate
          } catch {
            missingFiles.push(line);
          }
        }
      }

      if (missingFiles.length > 0) {
        return {
          name: 'segments_exist',
          status: 'fail',
          message: `${missingFiles.length} referenced file(s) missing on disk`,
          details: { missing: missingFiles.slice(0, 10) },
        };
      }

      return {
        name: 'segments_exist',
        status: 'pass',
        message: `All referenced files exist (${totalChecked} checked)`,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return {
        name: 'segments_exist',
        status: 'fail',
        message: `Failed to check segments: ${message}`,
      };
    }
  }

  /**
   * Check segments referenced in a variant playlist
   */
  private async checkVariantSegments(variantPath: string): Promise<string[]> {
    const missing: string[] = [];

    try {
      const content = await fs.readFile(variantPath, 'utf-8');
      const lines = content.split('\n').map(l => l.trim()).filter(l => l.length > 0);
      const baseDir = dirname(variantPath);

      for (const line of lines) {
        if (!line.startsWith('#') && (line.endsWith('.ts') || line.endsWith('.m4s') || line.endsWith('.mp4'))) {
          const segPath = join(baseDir, line);
          try {
            await fs.stat(segPath);
          } catch {
            missing.push(line);
          }
        }
      }
    } catch {
      // Can't read variant, already reported
    }

    return missing;
  }

  /**
   * Check 3: EXT-X-INDEPENDENT-SEGMENTS present in master or variant playlists
   */
  private async checkIndependentSegments(masterPath: string): Promise<QACheck> {
    try {
      const content = await fs.readFile(masterPath, 'utf-8');

      if (content.includes('#EXT-X-INDEPENDENT-SEGMENTS')) {
        return {
          name: 'independent_segments',
          status: 'pass',
          message: 'EXT-X-INDEPENDENT-SEGMENTS is present in master playlist',
        };
      }

      // Also check in variant playlists
      const lines = content.split('\n').map(l => l.trim()).filter(l => l.length > 0);
      const baseDir = dirname(masterPath);

      for (const line of lines) {
        if (!line.startsWith('#') && line.endsWith('.m3u8')) {
          try {
            const variantContent = await fs.readFile(join(baseDir, line), 'utf-8');
            if (variantContent.includes('#EXT-X-INDEPENDENT-SEGMENTS')) {
              return {
                name: 'independent_segments',
                status: 'pass',
                message: 'EXT-X-INDEPENDENT-SEGMENTS found in variant playlist(s)',
              };
            }
          } catch {
            continue;
          }
        }
      }

      return {
        name: 'independent_segments',
        status: 'warn',
        message: 'EXT-X-INDEPENDENT-SEGMENTS not found - recommended for ABR streaming',
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return {
        name: 'independent_segments',
        status: 'fail',
        message: `Failed to check independent segments: ${message}`,
      };
    }
  }

  /**
   * Check 4: Segment durations within tolerance of target
   */
  private async checkSegmentDurations(masterPath: string): Promise<QACheck> {
    try {
      const content = await fs.readFile(masterPath, 'utf-8');
      const lines = content.split('\n').map(l => l.trim()).filter(l => l.length > 0);
      const baseDir = dirname(masterPath);

      const toleranceSeconds = 0.5;
      let outOfTolerance = 0;
      let totalSegments = 0;
      let targetDuration: number | null = null;

      // Check first variant playlist for segment durations
      for (const line of lines) {
        if (!line.startsWith('#') && line.endsWith('.m3u8')) {
          try {
            const variantContent = await fs.readFile(join(baseDir, line), 'utf-8');
            const variantLines = variantContent.split('\n').map(l => l.trim());

            // Find target duration
            for (const vLine of variantLines) {
              const targetMatch = vLine.match(/#EXT-X-TARGETDURATION:(\d+)/);
              if (targetMatch) {
                targetDuration = parseInt(targetMatch[1], 10);
                break;
              }
            }

            // Check actual durations
            for (const vLine of variantLines) {
              const durationMatch = vLine.match(/#EXTINF:([\d.]+)/);
              if (durationMatch) {
                totalSegments++;
                const duration = parseFloat(durationMatch[1]);
                if (targetDuration !== null && Math.abs(duration - targetDuration) > toleranceSeconds) {
                  // Allow last segment to be shorter
                  outOfTolerance++;
                }
              }
            }

            // Only check one variant playlist
            break;
          } catch {
            continue;
          }
        }
      }

      if (totalSegments === 0) {
        return {
          name: 'segment_durations',
          status: 'warn',
          message: 'No segments found to check durations',
        };
      }

      // Last segment is allowed to be shorter, so subtract 1 from out-of-tolerance
      const adjusted = Math.max(0, outOfTolerance - 1);

      if (adjusted > 0 && adjusted > totalSegments * 0.1) {
        return {
          name: 'segment_durations',
          status: 'fail',
          message: `${adjusted} of ${totalSegments} segments exceed ${toleranceSeconds}s tolerance from target duration ${targetDuration}s`,
          details: { outOfTolerance: adjusted, total: totalSegments, targetDuration },
        };
      }

      if (adjusted > 0) {
        return {
          name: 'segment_durations',
          status: 'warn',
          message: `${adjusted} of ${totalSegments} segments slightly outside tolerance (acceptable for short content)`,
          details: { outOfTolerance: adjusted, total: totalSegments, targetDuration },
        };
      }

      return {
        name: 'segment_durations',
        status: 'pass',
        message: `All ${totalSegments} segments within ${toleranceSeconds}s of target duration ${targetDuration}s`,
        details: { total: totalSegments, targetDuration },
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return {
        name: 'segment_durations',
        status: 'fail',
        message: `Failed to check segment durations: ${message}`,
      };
    }
  }

  /**
   * Check 5: LANGUAGE attributes present for audio/subtitle tracks
   */
  private async checkLanguageAttributes(masterPath: string): Promise<QACheck> {
    try {
      const content = await fs.readFile(masterPath, 'utf-8');
      const lines = content.split('\n').map(l => l.trim()).filter(l => l.length > 0);

      // Look for EXT-X-MEDIA tags (audio/subtitle renditions)
      const mediaLines = lines.filter(l => l.startsWith('#EXT-X-MEDIA'));

      if (mediaLines.length === 0) {
        // No separate audio/subtitle renditions - check if stream-inf only
        const streamInfLines = lines.filter(l => l.startsWith('#EXT-X-STREAM-INF'));
        if (streamInfLines.length > 0) {
          return {
            name: 'language_attributes',
            status: 'pass',
            message: 'No separate audio/subtitle renditions (muxed audio) - LANGUAGE check not applicable',
          };
        }

        return {
          name: 'language_attributes',
          status: 'warn',
          message: 'No media renditions found to check LANGUAGE attributes',
        };
      }

      const missingLanguage: string[] = [];
      for (const line of mediaLines) {
        if (!line.includes('LANGUAGE=')) {
          // Extract TYPE and NAME for reporting
          const typeMatch = line.match(/TYPE=([A-Z]+)/);
          const nameMatch = line.match(/NAME="([^"]+)"/);
          missingLanguage.push(`${typeMatch?.[1] ?? 'UNKNOWN'}:${nameMatch?.[1] ?? 'unnamed'}`);
        }
      }

      if (missingLanguage.length > 0) {
        return {
          name: 'language_attributes',
          status: 'warn',
          message: `${missingLanguage.length} media rendition(s) missing LANGUAGE attribute`,
          details: { missing: missingLanguage },
        };
      }

      return {
        name: 'language_attributes',
        status: 'pass',
        message: `All ${mediaLines.length} media rendition(s) have LANGUAGE attributes`,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return {
        name: 'language_attributes',
        status: 'fail',
        message: `Failed to check language attributes: ${message}`,
      };
    }
  }

  /**
   * Basic DASH manifest check
   */
  private async checkDashManifest(dashPath: string): Promise<QACheck> {
    try {
      const content = await fs.readFile(dashPath, 'utf-8');

      if (!content.includes('<MPD')) {
        return {
          name: 'dash_manifest',
          status: 'fail',
          message: 'DASH manifest does not contain <MPD root element',
        };
      }

      if (!content.includes('<Period')) {
        return {
          name: 'dash_manifest',
          status: 'fail',
          message: 'DASH manifest has no <Period> elements',
        };
      }

      return {
        name: 'dash_manifest',
        status: 'pass',
        message: 'DASH manifest appears valid',
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return {
        name: 'dash_manifest',
        status: 'fail',
        message: `Failed to read DASH manifest: ${message}`,
      };
    }
  }
}
