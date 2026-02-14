/**
 * Media Scanner - Filename Parser
 * Parse media filenames into structured metadata using regex patterns
 */

import type { ParsedFilename } from './types.js';

// ─── Quality Patterns ───────────────────────────────────────────────────────

const QUALITY_PATTERNS: Array<[RegExp, string]> = [
  [/\bREMUX\b/i, 'REMUX'],
  [/\bBlu-?Ray\b/i, 'BluRay'],
  [/\bBDRip\b/i, 'BDRip'],
  [/\bBRRip\b/i, 'BRRip'],
  [/\bWEB-?DL\b/i, 'WEB-DL'],
  [/\bWEB-?Rip\b/i, 'WEBRip'],
  [/\bWEB\b/i, 'WEB'],
  [/\bHDRip\b/i, 'HDRip'],
  [/\bHDTV\b/i, 'HDTV'],
  [/\bPDTV\b/i, 'PDTV'],
  [/\bSDTV\b/i, 'SDTV'],
  [/\bDVDRip\b/i, 'DVDRip'],
  [/\bDVDScr\b/i, 'DVDScr'],
  [/\bDVD\b/i, 'DVD'],
  [/\bR5\b/i, 'R5'],
  [/\bCAM\b/i, 'CAM'],
  [/\bTS\b(?![0-9])/i, 'TS'],
  [/\bTELESYNC\b/i, 'TS'],
  [/\bHC\b/i, 'HC'],
  [/\bSCR\b/i, 'SCR'],
  [/\bPPV\b/i, 'PPV'],
  [/\b(?:UHD|Ultra\.?HD)\b/i, 'UHD'],
];

// ─── Resolution Patterns ────────────────────────────────────────────────────

const RESOLUTION_PATTERNS: Array<[RegExp, string]> = [
  [/\b2160p\b/i, '2160p'],
  [/\b4K\b/i, '2160p'],
  [/\b1080p\b/i, '1080p'],
  [/\b1080i\b/i, '1080i'],
  [/\b720p\b/i, '720p'],
  [/\b576p\b/i, '576p'],
  [/\b480p\b/i, '480p'],
  [/\b360p\b/i, '360p'],
];

// ─── Codec Patterns ─────────────────────────────────────────────────────────

const CODEC_PATTERNS: Array<[RegExp, string]> = [
  [/\bx\.?265\b/i, 'x265'],
  [/\bH\.?265\b/i, 'H.265'],
  [/\bHEVC\b/i, 'HEVC'],
  [/\bx\.?264\b/i, 'x264'],
  [/\bH\.?264\b/i, 'H.264'],
  [/\bAVC\b/i, 'AVC'],
  [/\bXviD\b/i, 'XviD'],
  [/\bDivX\b/i, 'DivX'],
  [/\bVP9\b/i, 'VP9'],
  [/\bAV1\b/i, 'AV1'],
  [/\bMPEG-?2\b/i, 'MPEG-2'],
  [/\bVC-?1\b/i, 'VC-1'],
];

// ─── Audio Codec Patterns (used to delimit title parsing, not extracted) ───

const AUDIO_TAGS = [
  'DTS-HD',
  'DTS-HD.MA',
  'DTS-X',
  'DTS',
  'TrueHD',
  'Atmos',
  'DD5.1',
  'DDP5.1',
  'DDP2.0',
  'DDP',
  'DD',
  'AAC',
  'AC3',
  'EAC3',
  'FLAC',
  'LPCM',
  'MP3',
  'PCM',
  'Opus',
  '5.1',
  '7.1',
  '2.0',
];

// Build a single regex from audio tags for boundary detection
const AUDIO_TAGS_PATTERN = new RegExp(
  `\\b(?:${AUDIO_TAGS.map(t => t.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|')})\\b`,
  'i'
);

// ─── Season/Episode Patterns ────────────────────────────────────────────────

const SE_PATTERNS: Array<[RegExp, (m: RegExpMatchArray) => { season: number; episode: number }]> = [
  // S01E02, S1E2, S01E02E03 (take first episode)
  [/S(\d{1,2})E(\d{1,3})/i, (m) => ({ season: parseInt(m[1], 10), episode: parseInt(m[2], 10) })],
  // 1x02, 01x02
  [/(\d{1,2})x(\d{2,3})/i, (m) => ({ season: parseInt(m[1], 10), episode: parseInt(m[2], 10) })],
  // Season 1 Episode 2
  [/Season\s*(\d{1,2})\s*Episode\s*(\d{1,3})/i, (m) => ({ season: parseInt(m[1], 10), episode: parseInt(m[2], 10) })],
  // S01 (season only, no episode)
  [/S(\d{1,2})(?!E\d)/i, (m) => ({ season: parseInt(m[1], 10), episode: 0 })],
];

// ─── Year Pattern ───────────────────────────────────────────────────────────

const YEAR_PATTERN = /(?:^|[.\s(])(\d{4})(?=[.\s)]|$)/;

// ─── Release Group Pattern ──────────────────────────────────────────────────

const GROUP_PATTERN = /-([A-Za-z0-9]+)(?:\[.*\])?$/;

/**
 * Parse a media filename into structured metadata.
 *
 * Handles patterns such as:
 *   Show.Name.S01E02.720p.BluRay.x264-GROUP
 *   Movie.Title.2023.1080p.WEB-DL.DDP5.1.H.264-GROUP
 *   The.Wire.S01E01.The.Target.HDTV.XviD-LOL
 *   Breaking.Bad.S05E16.Felina.1080p.BluRay.x265.HEVC-GROUP
 *   Movie.Name.(2019).4K.REMUX.2160p.UHD.BluRay-GROUP
 *   Some.Movie.2021.PROPER.REPACK.720p.WEB-DL.x264-GROUP
 *   Anime.Show.S01E01.1080p.WEB.H.264-GROUP
 *   The.Office.US.S02E05.HDTV.XviD-LOL
 *   movie-title-2020-1080p-web-dl.mkv
 *   Show Name - S03E12 - Episode Title.mkv
 *   Movie.Title.EXTENDED.2022.1080p.BluRay.x264-GROUP
 *   Some.Show.S01E01E02.720p.HDTV.x264-GROUP (multi-episode)
 *   Title.Of.Movie.1999.REMASTERED.BluRay.1080p.DTS-HD.MA.5.1.AVC.REMUX-GROUP
 *   An.Anime.S01.1080p.BluRay.x265-GROUP (season pack)
 *   Movie.2024.German.DL.1080p.BluRay.x264-GROUP
 *   Show.Name.2024.S01E01.Episode.Title.2160p.AMZN.WEB-DL.DDP5.1.H.265-GROUP
 *   The.Movie.2023.IMAX.HYBRID.2160p.UHD.BluRay.REMUX.HDR.HEVC.Atmos-GROUP
 *   Movie_Title_2022_1080p_WEB-DL_x264.mp4
 *   [SubGroup] Anime Title - 01 (1080p) [ABCD1234].mkv
 */
export function parseFilename(filename: string): ParsedFilename {
  // Strip file extension
  let name = filename.replace(/\.[a-zA-Z0-9]{2,4}$/, '');

  // Handle bracket-prefixed anime/fansub naming: [Group] Title - 01 (1080p)
  const bracketGroupMatch = name.match(/^\[([^\]]+)\]\s*/);
  let fansubGroup: string | null = null;
  if (bracketGroupMatch) {
    fansubGroup = bracketGroupMatch[1];
    name = name.substring(bracketGroupMatch[0].length);
  }

  // Remove trailing bracket hashes like [ABCD1234]
  name = name.replace(/\s*\[[A-Fa-f0-9]{8}\]\s*$/, '');

  // Normalize separators: replace underscores and " - " with dots
  const normalized = name
    .replace(/_/g, '.')
    .replace(/\s+-\s+/g, '.')
    .replace(/\s+/g, '.');

  // Extract season/episode
  let season: number | null = null;
  let episode: number | null = null;
  let seMatchIndex = -1;

  for (const [pattern, extractor] of SE_PATTERNS) {
    const match = normalized.match(pattern);
    if (match && match.index !== undefined) {
      const result = extractor(match);
      season = result.season;
      episode = result.episode === 0 ? null : result.episode;
      seMatchIndex = match.index;
      break;
    }
  }

  // Extract year
  let year: number | null = null;
  const yearMatch = normalized.match(YEAR_PATTERN);
  if (yearMatch) {
    const yearValue = parseInt(yearMatch[1], 10);
    if (yearValue >= 1920 && yearValue <= new Date().getFullYear() + 1) {
      year = yearValue;
    }
  }

  // Extract quality
  let quality: string | null = null;
  for (const [pattern, label] of QUALITY_PATTERNS) {
    if (pattern.test(normalized)) {
      quality = label;
      break;
    }
  }

  // Extract resolution
  let resolution: string | null = null;
  for (const [pattern, label] of RESOLUTION_PATTERNS) {
    if (pattern.test(normalized)) {
      resolution = label;
      break;
    }
  }

  // Extract codec
  let codec: string | null = null;
  for (const [pattern, label] of CODEC_PATTERNS) {
    if (pattern.test(normalized)) {
      codec = label;
      break;
    }
  }

  // Extract release group
  let group: string | null = null;
  const groupMatch = normalized.match(GROUP_PATTERN);
  if (groupMatch) {
    group = groupMatch[1];
  } else if (fansubGroup) {
    group = fansubGroup;
  }

  // Extract title: everything before the first metadata indicator
  const title = extractTitle(normalized, {
    year,
    yearMatch,
    seMatchIndex,
    resolution,
    quality,
    codec,
  });

  return {
    title,
    year,
    season,
    episode,
    quality,
    resolution,
    codec,
    group,
  };
}

interface TitleExtractionContext {
  year: number | null;
  yearMatch: RegExpMatchArray | null;
  seMatchIndex: number;
  resolution: string | null;
  quality: string | null;
  codec: string | null;
}

function extractTitle(normalized: string, ctx: TitleExtractionContext): string {
  // Find the earliest metadata boundary
  const boundaries: number[] = [];

  // Season/episode location
  if (ctx.seMatchIndex >= 0) {
    boundaries.push(ctx.seMatchIndex);
  }

  // Year location
  if (ctx.yearMatch && ctx.yearMatch.index !== undefined) {
    // The year match may have a leading separator; adjust index
    const yearStart = normalized.indexOf(String(ctx.year), ctx.yearMatch.index);
    if (yearStart >= 0) {
      boundaries.push(yearStart);
    }
  }

  // Resolution location
  if (ctx.resolution) {
    const resIndex = normalized.search(new RegExp(`\\b${escapeRegex(ctx.resolution)}\\b`, 'i'));
    if (resIndex >= 0) {
      boundaries.push(resIndex);
    }
  }

  // Quality location
  if (ctx.quality) {
    const qualIndex = normalized.search(new RegExp(`\\b${escapeRegex(ctx.quality)}\\b`, 'i'));
    if (qualIndex >= 0) {
      boundaries.push(qualIndex);
    }
  }

  // Audio tag location
  const audioMatch = normalized.match(AUDIO_TAGS_PATTERN);
  if (audioMatch && audioMatch.index !== undefined) {
    boundaries.push(audioMatch.index);
  }

  // Common noise words that indicate title has ended
  const noisePatterns = [
    /\bPROPER\b/i,
    /\bREPACK\b/i,
    /\bINTERNAL\b/i,
    /\bEXTENDED\b/i,
    /\bUNRATED\b/i,
    /\bDIRECTORS\.?CUT\b/i,
    /\bIMAX\b/i,
    /\bHYBRID\b/i,
    /\bREMASTERED\b/i,
    /\bGerman\b/i,
    /\bFrench\b/i,
    /\bSpanish\b/i,
    /\bItalian\b/i,
    /\bMULTi\b/i,
    /\bDL\b(?=\.)/i,
    /\bHDR\b/i,
    /\bHDR10\b/i,
    /\bDV\b/i,
    /\bDoVi\b/i,
    /\bDolby\.?Vision\b/i,
    /\bAMZN\b/i,
    /\bNF\b/i,
    /\bDSNP\b/i,
    /\bHMAX\b/i,
    /\bATVP\b/i,
    /\bPMTP\b/i,
  ];

  for (const pattern of noisePatterns) {
    const noiseMatch = normalized.match(pattern);
    if (noiseMatch && noiseMatch.index !== undefined) {
      boundaries.push(noiseMatch.index);
    }
  }

  // Codec location
  if (ctx.codec) {
    const codecIndex = normalized.search(new RegExp(`\\b${escapeRegex(ctx.codec)}\\b`, 'i'));
    if (codecIndex >= 0) {
      boundaries.push(codecIndex);
    }
  }

  let titleEnd: number;
  if (boundaries.length > 0) {
    titleEnd = Math.min(...boundaries);
  } else {
    titleEnd = normalized.length;
  }

  let title = normalized.substring(0, titleEnd);

  // Clean up the title
  title = title
    .replace(/\./g, ' ')       // dots to spaces
    .replace(/\s+/g, ' ')      // collapse whitespace
    .replace(/[()[\]]/g, '')   // remove brackets/parens
    .trim();

  // Remove trailing dash or hyphen
  title = title.replace(/[-\s]+$/, '');

  // Lowercase and strip punctuation for normalization
  title = normalizeTitle(title);

  return title;
}

/**
 * Normalize a title: lowercase, strip punctuation (except apostrophes within words),
 * collapse whitespace.
 */
export function normalizeTitle(title: string): string {
  return title
    .toLowerCase()
    .replace(/[^\w\s']/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
}

function escapeRegex(str: string): string {
  return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}
