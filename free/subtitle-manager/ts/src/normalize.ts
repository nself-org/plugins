import fs from 'fs/promises';
import path from 'path';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('subtitle-manager:normalize');

export type SubtitleFormat = 'srt' | 'vtt' | 'ass' | 'ssa' | 'unknown';

export class SubtitleNormalizer {
  // ---------------------------------------------------------------------------
  // Public API
  // ---------------------------------------------------------------------------

  /**
   * Converts any supported subtitle format to valid WebVTT.
   * @param inputPath - Path to the source subtitle file
   * @param outputPath - Output path (defaults to inputPath with .vtt extension)
   * @returns Path to the normalized WebVTT file
   */
  async normalizeToWebVTT(inputPath: string, outputPath?: string): Promise<string> {
    logger.info('Normalizing subtitle to WebVTT', { inputPath });

    const rawBuffer = await fs.readFile(inputPath);
    const content = this.normalizeEncoding(rawBuffer);
    const format = this.detectFormat(content);

    logger.debug('Detected subtitle format', { format });

    let vttContent: string;
    switch (format) {
      case 'srt':
        vttContent = this.srtToWebVTT(content);
        break;
      case 'vtt':
        // Already WebVTT, just normalize encoding/line endings
        vttContent = this.cleanWebVTT(content);
        break;
      case 'ass':
      case 'ssa':
        vttContent = this.assToWebVTT(content);
        break;
      default:
        throw new Error(`Unsupported subtitle format: ${format}. Supported: srt, vtt, ass, ssa`);
    }

    const resolvedOutput = outputPath || this.replaceExtension(inputPath, '.vtt');
    const outputDir = path.dirname(resolvedOutput);
    await fs.mkdir(outputDir, { recursive: true });
    await fs.writeFile(resolvedOutput, vttContent, 'utf-8');

    logger.info('WebVTT normalization complete', { outputPath: resolvedOutput });
    return resolvedOutput;
  }

  // ---------------------------------------------------------------------------
  // Format detection
  // ---------------------------------------------------------------------------

  /**
   * Detect subtitle format from file content.
   */
  detectFormat(content: string): SubtitleFormat {
    const trimmed = content.trim();

    // WebVTT starts with WEBVTT
    if (/^WEBVTT/i.test(trimmed)) {
      return 'vtt';
    }

    // ASS/SSA has [Script Info] section
    if (/\[Script Info\]/i.test(trimmed)) {
      // ASS has "ScriptType: v4.00+" while SSA has "ScriptType: v4.00"
      if (/ScriptType:\s*v4\.00\+/i.test(trimmed)) {
        return 'ass';
      }
      return 'ssa';
    }

    // SRT has numeric index followed by timestamp with comma separator
    // Look for the pattern: digits \n digits:digits:digits,digits --> digits:digits:digits,digits
    if (/^\d+\s*\n\d{1,2}:\d{2}:\d{2}[,.]\d{3}\s*-->\s*\d{1,2}:\d{2}:\d{2}[,.]\d{3}/m.test(trimmed)) {
      return 'srt';
    }

    // Also detect SRT where the first block might not start at index 1
    if (/\d{1,2}:\d{2}:\d{2}[,.]\d{3}\s*-->\s*\d{1,2}:\d{2}:\d{2}[,.]\d{3}/.test(trimmed)) {
      return 'srt';
    }

    return 'unknown';
  }

  // ---------------------------------------------------------------------------
  // SRT -> WebVTT
  // ---------------------------------------------------------------------------

  /**
   * Convert SRT content to valid WebVTT.
   */
  srtToWebVTT(content: string): string {
    const lines: string[] = ['WEBVTT', ''];
    const normalized = content.replace(/\r\n/g, '\n').replace(/\r/g, '\n');
    const blocks = normalized.split(/\n\n+/).filter(b => b.trim().length > 0);

    for (const block of blocks) {
      const blockLines = block.trim().split('\n');
      if (blockLines.length < 2) continue;

      // Find timestamp line
      let timestampLineIdx = -1;
      for (let i = 0; i < blockLines.length; i++) {
        if (blockLines[i].includes('-->')) {
          timestampLineIdx = i;
          break;
        }
      }
      if (timestampLineIdx < 0) continue;

      // Convert timestamp: replace commas with dots
      const timestampLine = blockLines[timestampLineIdx].replace(/,/g, '.');

      // Collect text lines after the timestamp
      const textLines = blockLines.slice(timestampLineIdx + 1);
      // Strip basic SRT formatting tags but keep <b>, <i>, <u> which are valid in VTT
      const cleanedText = textLines
        .map(line => this.cleanSrtTags(line))
        .join('\n');

      lines.push(timestampLine);
      lines.push(cleanedText);
      lines.push('');
    }

    return lines.join('\n');
  }

  /**
   * Clean SRT-specific tags, keep WebVTT-compatible ones.
   */
  private cleanSrtTags(line: string): string {
    // Keep <b>, <i>, <u> and their closing tags (valid in WebVTT)
    // Remove font tags and other HTML
    let cleaned = line;
    // Remove <font ...> and </font>
    cleaned = cleaned.replace(/<\/?font[^>]*>/gi, '');
    // Remove position/alignment tags like {\an8}
    cleaned = cleaned.replace(/\{\\an?\d+\}/gi, '');
    // Remove other ASS-style overrides that sometimes appear in SRT
    cleaned = cleaned.replace(/\{\\[^}]+\}/g, '');
    return cleaned;
  }

  // ---------------------------------------------------------------------------
  // ASS/SSA -> WebVTT
  // ---------------------------------------------------------------------------

  /**
   * Convert ASS/SSA content to valid WebVTT.
   */
  assToWebVTT(content: string): string {
    const lines: string[] = ['WEBVTT', ''];
    const normalized = content.replace(/\r\n/g, '\n').replace(/\r/g, '\n');

    // Find the [Events] section
    const eventsMatch = normalized.match(/\[Events\]\s*\n([\s\S]*?)(?:\n\[|$)/i);
    if (!eventsMatch) {
      logger.warn('No [Events] section found in ASS/SSA file');
      return lines.join('\n');
    }

    const eventsBlock = eventsMatch[1];
    const eventLines = eventsBlock.split('\n').filter(l => l.trim().length > 0);

    // Parse Format line to find column positions
    let formatLine = eventLines.find(l => /^Format:/i.test(l.trim()));
    let formatColumns: string[] = [];
    if (formatLine) {
      formatColumns = formatLine
        .replace(/^Format:\s*/i, '')
        .split(',')
        .map(c => c.trim().toLowerCase());
    } else {
      // Default ASS format
      formatColumns = ['layer', 'start', 'end', 'style', 'name', 'marginl', 'marginr', 'marginv', 'effect', 'text'];
    }

    const startIdx = formatColumns.indexOf('start');
    const endIdx = formatColumns.indexOf('end');
    const textIdx = formatColumns.indexOf('text');

    if (startIdx < 0 || endIdx < 0 || textIdx < 0) {
      logger.warn('Could not find required columns in ASS/SSA Format line');
      return lines.join('\n');
    }

    // Parse Dialogue lines
    const dialogueLines = eventLines.filter(l => /^Dialogue:/i.test(l.trim()));
    let cueIndex = 1;

    for (const dialogue of dialogueLines) {
      const parts = dialogue.replace(/^Dialogue:\s*/i, '');
      // Split by comma, but the text field (last) may contain commas
      const columns = this.splitAssDialogue(parts, formatColumns.length);
      if (columns.length < formatColumns.length) continue;

      const startTime = columns[startIdx];
      const endTime = columns[endIdx];
      const rawText = columns[textIdx];

      const vttStart = this.assTimestampToVtt(startTime);
      const vttEnd = this.assTimestampToVtt(endTime);
      if (!vttStart || !vttEnd) continue;

      // Strip ASS tags and convert to plain text
      const cleanText = this.stripAssTags(rawText);
      if (cleanText.trim().length === 0) continue;

      lines.push(`${vttStart} --> ${vttEnd}`);
      lines.push(cleanText);
      lines.push('');
      cueIndex++;
    }

    return lines.join('\n');
  }

  /**
   * Split ASS dialogue line respecting that the last field (Text) may contain commas.
   */
  private splitAssDialogue(line: string, columnCount: number): string[] {
    const result: string[] = [];
    let remaining = line;

    for (let i = 0; i < columnCount - 1; i++) {
      const commaIdx = remaining.indexOf(',');
      if (commaIdx < 0) break;
      result.push(remaining.substring(0, commaIdx).trim());
      remaining = remaining.substring(commaIdx + 1);
    }

    // The rest is the Text field
    result.push(remaining.trim());
    return result;
  }

  /**
   * Convert ASS timestamp (H:MM:SS.cc) to WebVTT (HH:MM:SS.mmm).
   */
  private assTimestampToVtt(timestamp: string): string | null {
    const match = timestamp.trim().match(/(\d+):(\d{2}):(\d{2})\.(\d{2,3})/);
    if (!match) return null;

    const hours = match[1].padStart(2, '0');
    const minutes = match[2];
    const seconds = match[3];
    // ASS uses centiseconds (2 digits), VTT uses milliseconds (3 digits)
    let ms = match[4];
    if (ms.length === 2) {
      ms = ms + '0'; // Convert centiseconds to milliseconds
    }

    return `${hours}:${minutes}:${seconds}.${ms}`;
  }

  /**
   * Strip all ASS override tags from text.
   */
  private stripAssTags(text: string): string {
    let cleaned = text;
    // Remove override blocks like {\b1}, {\i0}, {\pos(x,y)}, etc.
    cleaned = cleaned.replace(/\{\\[^}]*\}/g, '');
    // Convert \N to newline (ASS line break)
    cleaned = cleaned.replace(/\\N/g, '\n');
    // Convert \n (soft line break) to space
    cleaned = cleaned.replace(/\\n/g, ' ');
    // Remove \h (hard space)
    cleaned = cleaned.replace(/\\h/g, ' ');
    return cleaned.trim();
  }

  // ---------------------------------------------------------------------------
  // Encoding normalization
  // ---------------------------------------------------------------------------

  /**
   * Detect encoding, strip BOM, and normalize to UTF-8 with \n line endings.
   */
  normalizeEncoding(buffer: Buffer): string {
    let content: string;

    // Check for BOM markers and decode accordingly
    if (buffer.length >= 3 && buffer[0] === 0xEF && buffer[1] === 0xBB && buffer[2] === 0xBF) {
      // UTF-8 BOM
      content = buffer.subarray(3).toString('utf-8');
    } else if (buffer.length >= 2 && buffer[0] === 0xFF && buffer[1] === 0xFE) {
      // UTF-16 LE BOM
      content = buffer.subarray(2).toString('utf16le');
    } else if (buffer.length >= 2 && buffer[0] === 0xFE && buffer[1] === 0xFF) {
      // UTF-16 BE BOM - swap bytes to LE then decode
      const swapped = Buffer.alloc(buffer.length - 2);
      for (let i = 2; i < buffer.length - 1; i += 2) {
        swapped[i - 2] = buffer[i + 1];
        swapped[i - 1] = buffer[i];
      }
      content = swapped.toString('utf16le');
    } else {
      // Assume UTF-8 (covers ASCII and most modern files)
      content = buffer.toString('utf-8');
    }

    // Normalize line endings to \n
    content = content.replace(/\r\n/g, '\n').replace(/\r/g, '\n');

    return content;
  }

  // ---------------------------------------------------------------------------
  // Helpers
  // ---------------------------------------------------------------------------

  /**
   * Clean existing WebVTT content (normalize encoding, fix whitespace).
   */
  private cleanWebVTT(content: string): string {
    const lines = content.split('\n');
    // Ensure WEBVTT header
    if (!lines[0].startsWith('WEBVTT')) {
      lines.unshift('WEBVTT');
    }
    return lines.join('\n');
  }

  private replaceExtension(filePath: string, newExt: string): string {
    const parsed = path.parse(filePath);
    return path.join(parsed.dir, parsed.name + newExt);
  }
}
