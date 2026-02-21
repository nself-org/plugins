import fs from 'fs/promises';
import { createLogger } from '@nself/plugin-utils';
import type { QCResult, QCCheck, QCIssue, SubtitleCue } from './types.js';

const logger = createLogger('subtitle-manager:qc');

export class SubtitleQC {
  // ---------------------------------------------------------------------------
  // Public validation entry point
  // ---------------------------------------------------------------------------

  /**
   * Run all deterministic QC checks against a subtitle file.
   * @param subtitlePath - Path to subtitle file (.srt or .vtt)
   * @param videoDurationMs - Video duration in milliseconds (optional)
   */
  async validateSubtitle(subtitlePath: string, videoDurationMs?: number): Promise<QCResult> {
    logger.info('Running QC validation', { subtitlePath, videoDurationMs });

    const content = await fs.readFile(subtitlePath, 'utf-8');
    const isVtt = subtitlePath.toLowerCase().endsWith('.vtt');
    const cues = isVtt ? this.parseVtt(content) : this.parseSrt(content);

    const checks: QCCheck[] = [];
    const issues: QCIssue[] = [];

    // Check 1: Timestamps within [0, video_duration]
    if (videoDurationMs !== undefined) {
      const { check, cueIssues } = this.checkTimestampsInRange(cues, videoDurationMs);
      checks.push(check);
      issues.push(...cueIssues);
    }

    // Check 2: First cue within first 10 minutes (600 seconds)
    {
      const { check, cueIssues } = this.checkFirstCueEarly(cues);
      checks.push(check);
      issues.push(...cueIssues);
    }

    // Check 3: Last cue within 5 minutes of video end
    if (videoDurationMs !== undefined) {
      const { check, cueIssues } = this.checkLastCueNearEnd(cues, videoDurationMs);
      checks.push(check);
      issues.push(...cueIssues);
    }

    // Check 4: No negative cue durations
    {
      const { check, cueIssues } = this.checkNoNegativeDurations(cues);
      checks.push(check);
      issues.push(...cueIssues);
    }

    // Check 5: No massive overlap rate (>10% of cues overlap next cue)
    {
      const { check, cueIssues } = this.checkOverlapRate(cues);
      checks.push(check);
      issues.push(...cueIssues);
    }

    // Check 6: Characters per second within 5-35 CPS for each cue
    {
      const { check, cueIssues } = this.checkCPS(cues);
      checks.push(check);
      issues.push(...cueIssues);
    }

    // Check 7: Line length heuristic (no single line >80 characters - warning)
    {
      const { check, cueIssues } = this.checkLineLength(cues);
      checks.push(check);
      issues.push(...cueIssues);
    }

    // Determine overall status
    const hasErrors = issues.some(i => i.severity === 'error');
    const hasWarnings = issues.some(i => i.severity === 'warning');
    const status: QCResult['status'] = hasErrors ? 'fail' : hasWarnings ? 'warn' : 'pass';

    const totalDurationMs = cues.length > 0
      ? Math.max(...cues.map(c => c.endMs)) - Math.min(...cues.map(c => c.startMs))
      : 0;

    const result: QCResult = {
      status,
      checks,
      issues,
      cueCount: cues.length,
      totalDurationMs,
    };

    logger.info('QC validation complete', { status, cueCount: cues.length, issueCount: issues.length });
    return result;
  }

  // ---------------------------------------------------------------------------
  // SRT parser
  // ---------------------------------------------------------------------------

  parseSrt(content: string): SubtitleCue[] {
    const cues: SubtitleCue[] = [];
    const normalized = content.replace(/\r\n/g, '\n').replace(/\r/g, '\n');
    // Split on double-newline (or more) to get blocks
    const blocks = normalized.split(/\n\n+/).filter(b => b.trim().length > 0);

    for (const block of blocks) {
      const lines = block.trim().split('\n');
      if (lines.length < 2) continue;

      // Find the timestamp line (contains " --> ")
      let timestampLineIdx = -1;
      for (let i = 0; i < lines.length; i++) {
        if (lines[i].includes('-->')) {
          timestampLineIdx = i;
          break;
        }
      }
      if (timestampLineIdx < 0) continue;

      const indexStr = timestampLineIdx > 0 ? lines[0].trim() : '';
      const index = parseInt(indexStr, 10) || cues.length + 1;

      const timestamps = this.parseSrtTimestampLine(lines[timestampLineIdx]);
      if (!timestamps) continue;

      const textLines = lines.slice(timestampLineIdx + 1);
      const text = textLines.join('\n').trim();

      cues.push({
        index,
        startMs: timestamps.startMs,
        endMs: timestamps.endMs,
        text,
      });
    }

    return cues;
  }

  private parseSrtTimestampLine(line: string): { startMs: number; endMs: number } | null {
    // Format: 00:01:23,456 --> 00:01:25,789
    const match = line.match(
      /(\d{1,2}):(\d{2}):(\d{2})[,.](\d{3})\s*-->\s*(\d{1,2}):(\d{2}):(\d{2})[,.](\d{3})/,
    );
    if (!match) return null;

    const startMs =
      parseInt(match[1], 10) * 3600000 +
      parseInt(match[2], 10) * 60000 +
      parseInt(match[3], 10) * 1000 +
      parseInt(match[4], 10);

    const endMs =
      parseInt(match[5], 10) * 3600000 +
      parseInt(match[6], 10) * 60000 +
      parseInt(match[7], 10) * 1000 +
      parseInt(match[8], 10);

    return { startMs, endMs };
  }

  // ---------------------------------------------------------------------------
  // WebVTT parser
  // ---------------------------------------------------------------------------

  parseVtt(content: string): SubtitleCue[] {
    const cues: SubtitleCue[] = [];
    const normalized = content.replace(/\r\n/g, '\n').replace(/\r/g, '\n');

    // Remove WEBVTT header and any metadata blocks
    const headerEnd = normalized.indexOf('\n\n');
    if (headerEnd < 0) return cues;
    const body = normalized.substring(headerEnd + 2);

    const blocks = body.split(/\n\n+/).filter(b => b.trim().length > 0);

    let cueIndex = 1;
    for (const block of blocks) {
      const lines = block.trim().split('\n');
      if (lines.length < 2) continue;

      // Find the timestamp line
      let timestampLineIdx = -1;
      for (let i = 0; i < lines.length; i++) {
        if (lines[i].includes('-->')) {
          timestampLineIdx = i;
          break;
        }
      }
      if (timestampLineIdx < 0) continue;

      const timestamps = this.parseVttTimestampLine(lines[timestampLineIdx]);
      if (!timestamps) continue;

      const textLines = lines.slice(timestampLineIdx + 1);
      const text = textLines.join('\n').trim();

      cues.push({
        index: cueIndex++,
        startMs: timestamps.startMs,
        endMs: timestamps.endMs,
        text,
      });
    }

    return cues;
  }

  private parseVttTimestampLine(line: string): { startMs: number; endMs: number } | null {
    // Format: 00:01:23.456 --> 00:01:25.789  or  01:23.456 --> 01:25.789
    const match = line.match(
      /(?:(\d{1,2}):)?(\d{2}):(\d{2})\.(\d{3})\s*-->\s*(?:(\d{1,2}):)?(\d{2}):(\d{2})\.(\d{3})/,
    );
    if (!match) return null;

    const startMs =
      (parseInt(match[1] || '0', 10)) * 3600000 +
      parseInt(match[2], 10) * 60000 +
      parseInt(match[3], 10) * 1000 +
      parseInt(match[4], 10);

    const endMs =
      (parseInt(match[5] || '0', 10)) * 3600000 +
      parseInt(match[6], 10) * 60000 +
      parseInt(match[7], 10) * 1000 +
      parseInt(match[8], 10);

    return { startMs, endMs };
  }

  // ---------------------------------------------------------------------------
  // QC check implementations
  // ---------------------------------------------------------------------------

  private checkTimestampsInRange(
    cues: SubtitleCue[],
    videoDurationMs: number,
  ): { check: QCCheck; cueIssues: QCIssue[] } {
    const cueIssues: QCIssue[] = [];
    let allInRange = true;

    for (const cue of cues) {
      if (cue.startMs < 0 || cue.endMs < 0) {
        allInRange = false;
        cueIssues.push({
          severity: 'error',
          check: 'timestamps_in_range',
          cueIndex: cue.index,
          message: `Cue ${cue.index} has negative timestamp: start=${cue.startMs}ms, end=${cue.endMs}ms`,
        });
      }
      if (cue.endMs > videoDurationMs + 5000) {
        // Allow 5s tolerance past video duration
        allInRange = false;
        cueIssues.push({
          severity: 'error',
          check: 'timestamps_in_range',
          cueIndex: cue.index,
          message: `Cue ${cue.index} extends beyond video duration: endMs=${cue.endMs} > videoDuration=${videoDurationMs}ms`,
        });
      }
    }

    return {
      check: {
        name: 'timestamps_in_range',
        passed: allInRange,
        message: allInRange
          ? `All ${cues.length} cues within video duration`
          : `${cueIssues.length} cue(s) have out-of-range timestamps`,
      },
      cueIssues,
    };
  }

  private checkFirstCueEarly(
    cues: SubtitleCue[],
  ): { check: QCCheck; cueIssues: QCIssue[] } {
    const cueIssues: QCIssue[] = [];
    const maxFirstCueMs = 600_000; // 10 minutes

    if (cues.length === 0) {
      cueIssues.push({
        severity: 'error',
        check: 'first_cue_early',
        message: 'No cues found in subtitle file',
      });
      return {
        check: { name: 'first_cue_early', passed: false, message: 'No cues found' },
        cueIssues,
      };
    }

    const firstCue = cues[0];
    const passed = firstCue.startMs <= maxFirstCueMs;
    if (!passed) {
      cueIssues.push({
        severity: 'error',
        check: 'first_cue_early',
        cueIndex: firstCue.index,
        message: `First cue starts at ${firstCue.startMs}ms (${(firstCue.startMs / 60000).toFixed(1)} min), expected within first 10 minutes`,
      });
    }

    return {
      check: {
        name: 'first_cue_early',
        passed,
        message: passed
          ? `First cue at ${firstCue.startMs}ms`
          : `First cue too late at ${firstCue.startMs}ms`,
      },
      cueIssues,
    };
  }

  private checkLastCueNearEnd(
    cues: SubtitleCue[],
    videoDurationMs: number,
  ): { check: QCCheck; cueIssues: QCIssue[] } {
    const cueIssues: QCIssue[] = [];
    const maxGapMs = 300_000; // 5 minutes

    if (cues.length === 0) {
      return {
        check: { name: 'last_cue_near_end', passed: false, message: 'No cues found' },
        cueIssues: [{ severity: 'error', check: 'last_cue_near_end', message: 'No cues found' }],
      };
    }

    const lastCue = cues[cues.length - 1];
    const gap = videoDurationMs - lastCue.endMs;
    const passed = gap <= maxGapMs;

    if (!passed) {
      cueIssues.push({
        severity: 'error',
        check: 'last_cue_near_end',
        cueIndex: lastCue.index,
        message: `Last cue ends at ${lastCue.endMs}ms, ${(gap / 60000).toFixed(1)} min before video end (${videoDurationMs}ms). Max gap: 5 min`,
      });
    }

    return {
      check: {
        name: 'last_cue_near_end',
        passed,
        message: passed
          ? `Last cue ends ${(gap / 1000).toFixed(1)}s before video end`
          : `Last cue ends ${(gap / 60000).toFixed(1)} min before video end`,
      },
      cueIssues,
    };
  }

  private checkNoNegativeDurations(
    cues: SubtitleCue[],
  ): { check: QCCheck; cueIssues: QCIssue[] } {
    const cueIssues: QCIssue[] = [];

    for (const cue of cues) {
      if (cue.endMs < cue.startMs) {
        cueIssues.push({
          severity: 'error',
          check: 'no_negative_durations',
          cueIndex: cue.index,
          message: `Cue ${cue.index} has negative duration: start=${cue.startMs}ms > end=${cue.endMs}ms`,
        });
      }
    }

    const passed = cueIssues.length === 0;
    return {
      check: {
        name: 'no_negative_durations',
        passed,
        message: passed
          ? 'No negative durations found'
          : `${cueIssues.length} cue(s) have negative durations`,
      },
      cueIssues,
    };
  }

  private checkOverlapRate(
    cues: SubtitleCue[],
  ): { check: QCCheck; cueIssues: QCIssue[] } {
    const cueIssues: QCIssue[] = [];
    let overlapCount = 0;

    for (let i = 0; i < cues.length - 1; i++) {
      if (cues[i].endMs > cues[i + 1].startMs) {
        overlapCount++;
        // Only log first few overlaps to keep issues manageable
        if (cueIssues.length < 20) {
          cueIssues.push({
            severity: 'warning',
            check: 'overlap_rate',
            cueIndex: cues[i].index,
            message: `Cue ${cues[i].index} overlaps with cue ${cues[i + 1].index}: end=${cues[i].endMs}ms > nextStart=${cues[i + 1].startMs}ms`,
          });
        }
      }
    }

    const overlapRate = cues.length > 1 ? overlapCount / (cues.length - 1) : 0;
    const passed = overlapRate <= 0.10;

    if (!passed) {
      // Promote severity to error when threshold exceeded
      for (const issue of cueIssues) {
        issue.severity = 'error';
      }
    }

    return {
      check: {
        name: 'overlap_rate',
        passed,
        message: passed
          ? `Overlap rate: ${(overlapRate * 100).toFixed(1)}% (${overlapCount} of ${cues.length} cues)`
          : `Excessive overlap rate: ${(overlapRate * 100).toFixed(1)}% (${overlapCount} of ${cues.length} cues) exceeds 10% threshold`,
      },
      cueIssues,
    };
  }

  private checkCPS(
    cues: SubtitleCue[],
  ): { check: QCCheck; cueIssues: QCIssue[] } {
    const cueIssues: QCIssue[] = [];
    const minCPS = 5;
    const maxCPS = 35;
    let outOfBounds = 0;

    for (const cue of cues) {
      const durationSec = (cue.endMs - cue.startMs) / 1000;
      if (durationSec <= 0) continue; // skip invalid durations (caught by other check)

      // Strip HTML/formatting tags for character count
      const plainText = cue.text.replace(/<[^>]+>/g, '').replace(/\{[^}]+\}/g, '');
      const charCount = plainText.length;
      if (charCount === 0) continue;

      const cps = charCount / durationSec;
      if (cps < minCPS || cps > maxCPS) {
        outOfBounds++;
        if (cueIssues.length < 20) {
          cueIssues.push({
            severity: 'warning',
            check: 'cps_bounds',
            cueIndex: cue.index,
            message: `Cue ${cue.index} has ${cps.toFixed(1)} CPS (${charCount} chars / ${durationSec.toFixed(1)}s). Expected ${minCPS}-${maxCPS} CPS`,
          });
        }
      }
    }

    const passed = outOfBounds === 0;
    return {
      check: {
        name: 'cps_bounds',
        passed,
        message: passed
          ? 'All cues within CPS bounds (5-35)'
          : `${outOfBounds} cue(s) outside CPS bounds (5-35)`,
      },
      cueIssues,
    };
  }

  private checkLineLength(
    cues: SubtitleCue[],
  ): { check: QCCheck; cueIssues: QCIssue[] } {
    const cueIssues: QCIssue[] = [];
    const maxLineLength = 80;

    for (const cue of cues) {
      const lines = cue.text.split('\n');
      for (const line of lines) {
        const plainLine = line.replace(/<[^>]+>/g, '').replace(/\{[^}]+\}/g, '');
        if (plainLine.length > maxLineLength) {
          cueIssues.push({
            severity: 'warning',
            check: 'line_length',
            cueIndex: cue.index,
            message: `Cue ${cue.index} has line with ${plainLine.length} chars (max ${maxLineLength})`,
          });
          break; // one warning per cue is enough
        }
      }
    }

    const passed = cueIssues.length === 0;
    return {
      check: {
        name: 'line_length',
        passed,
        message: passed
          ? `All lines within ${maxLineLength} character limit`
          : `${cueIssues.length} cue(s) have lines exceeding ${maxLineLength} characters`,
      },
      cueIssues,
    };
  }
}
