/**
 * Podcast Feed Parser
 * Parses RSS 2.0 and Atom feeds with podcast namespace extension support
 */

import { XMLParser } from 'fast-xml-parser';
import { createLogger } from '@nself/plugin-utils';
import type { ParsedFeed, ParsedEpisode, EpisodeType } from './types.js';

const logger = createLogger('podcast:parser');

const xmlParser = new XMLParser({
  ignoreAttributes: false,
  attributeNamePrefix: '@_',
  textNodeName: '#text',
  isArray: (tagName) => {
    const arrayTags = ['item', 'entry', 'category', 'link', 'podcast:chapter', 'podcast:transcript'];
    return arrayTags.includes(tagName);
  },
  parseTagValue: true,
  trimValues: true,
});

/**
 * Fetch and parse a podcast feed from a URL
 */
export async function fetchAndParseFeed(url: string): Promise<ParsedFeed> {
  logger.debug('Fetching feed', { url });

  const response = await fetch(url, {
    headers: {
      'User-Agent': 'nself-podcast/1.0.0',
      'Accept': 'application/rss+xml, application/xml, application/atom+xml, text/xml, */*',
    },
    signal: AbortSignal.timeout(30000),
  });

  if (!response.ok) {
    throw new Error(`Failed to fetch feed: HTTP ${response.status} ${response.statusText}`);
  }

  const xml = await response.text();
  return parseFeedXml(xml);
}

/**
 * Parse raw XML content into a structured feed object
 */
export function parseFeedXml(xml: string): ParsedFeed {
  let parsed: Record<string, unknown>;
  try {
    parsed = xmlParser.parse(xml) as Record<string, unknown>;
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown parse error';
    throw new Error(`Failed to parse feed XML: ${message}`);
  }

  // Detect feed type and delegate
  if (parsed.rss) {
    return parseRss(parsed.rss as Record<string, unknown>);
  }

  if (parsed.feed) {
    return parseAtom(parsed.feed as Record<string, unknown>);
  }

  // Some feeds wrap in xml declaration only
  if (parsed['rdf:RDF']) {
    return parseRdf(parsed['rdf:RDF'] as Record<string, unknown>);
  }

  throw new Error('Unrecognized feed format: expected RSS, Atom, or RDF');
}

// =========================================================================
// RSS 2.0 Parsing
// =========================================================================

function parseRss(rss: Record<string, unknown>): ParsedFeed {
  const channel = rss.channel as Record<string, unknown> | undefined;
  if (!channel) {
    throw new Error('Invalid RSS feed: missing <channel>');
  }

  const title = extractText(channel.title) ?? 'Untitled Podcast';
  const description = extractText(channel.description) ?? extractText(channel['itunes:summary']);
  const author = extractText(channel['itunes:author']) ?? extractText(channel.managingEditor);
  const language = extractText(channel.language);
  const lastBuildDate = parseDate(extractText(channel.lastBuildDate));

  // Image extraction (multiple possible sources)
  let imageUrl: string | null = null;
  const itunesImage = channel['itunes:image'] as Record<string, unknown> | undefined;
  if (itunesImage) {
    imageUrl = extractAttr(itunesImage, 'href') ?? extractText(itunesImage);
  }
  if (!imageUrl) {
    const image = channel.image as Record<string, unknown> | undefined;
    if (image) {
      imageUrl = extractText(image.url);
    }
  }

  // Categories
  const categories = extractCategories(channel);

  // Parse items (episodes)
  const items = ensureArray(channel.item);
  const episodes: ParsedEpisode[] = items.map(item => parseRssItem(item as Record<string, unknown>));

  return {
    title,
    description,
    author,
    imageUrl,
    language,
    categories,
    link: extractText(channel.link),
    lastBuildDate,
    episodes,
  };
}

function parseRssItem(item: Record<string, unknown>): ParsedEpisode {
  const title = extractText(item.title) ?? 'Untitled Episode';

  // GUID: prefer <guid>, fall back to <link> or <enclosure url>
  let guid = extractText(item.guid);
  if (!guid) {
    const guidObj = item.guid as Record<string, unknown> | undefined;
    guid = guidObj ? extractText(guidObj['#text']) : null;
  }
  if (!guid) {
    guid = extractText(item.link);
  }

  // Description: try multiple sources
  const description =
    extractText(item['content:encoded']) ??
    extractText(item.description) ??
    extractText(item['itunes:summary']);

  // Publication date
  const pubDate = parseDate(extractText(item.pubDate));

  // Duration
  const durationSeconds = parseDuration(extractText(item['itunes:duration']));

  // Enclosure
  const enclosure = item.enclosure as Record<string, unknown> | undefined;
  let enclosureUrl: string | null = null;
  let enclosureType: string | null = null;
  let enclosureLength: number | null = null;
  if (enclosure) {
    enclosureUrl = extractAttr(enclosure, 'url');
    enclosureType = extractAttr(enclosure, 'type');
    const lengthStr = extractAttr(enclosure, 'length');
    enclosureLength = lengthStr ? parseInt(lengthStr, 10) || null : null;
  }

  // If no enclosure, check for media:content
  if (!enclosureUrl) {
    const mediaContent = item['media:content'] as Record<string, unknown> | undefined;
    if (mediaContent) {
      enclosureUrl = extractAttr(mediaContent, 'url');
      enclosureType = extractAttr(mediaContent, 'type');
    }
  }

  // Fall back: use link as guid if still missing
  if (!guid) {
    guid = enclosureUrl ?? `${title}-${pubDate?.toISOString() ?? 'unknown'}`;
  }

  // Season/episode numbers
  const seasonNumber = parseIntOrNull(extractText(item['itunes:season']));
  const episodeNumber = parseIntOrNull(extractText(item['itunes:episode']));

  // Episode type
  const rawType = extractText(item['itunes:episodeType']);
  const episodeType = parseEpisodeType(rawType);

  // Podcast namespace extensions
  const chaptersUrl = extractPodcastChaptersUrl(item);
  const transcriptUrl = extractPodcastTranscriptUrl(item);

  // Episode image
  let imageUrl: string | null = null;
  const itunesImage = item['itunes:image'] as Record<string, unknown> | undefined;
  if (itunesImage) {
    imageUrl = extractAttr(itunesImage, 'href') ?? extractText(itunesImage);
  }

  return {
    guid,
    title,
    description,
    pubDate,
    durationSeconds,
    enclosureUrl,
    enclosureType,
    enclosureLength,
    seasonNumber,
    episodeNumber,
    episodeType,
    chaptersUrl,
    transcriptUrl,
    imageUrl,
  };
}

// =========================================================================
// Atom Feed Parsing
// =========================================================================

function parseAtom(feed: Record<string, unknown>): ParsedFeed {
  const title = extractText(feed.title) ?? 'Untitled Podcast';
  const description = extractText(feed.subtitle) ?? extractText(feed.summary);
  const author = extractAtomAuthor(feed);
  const language = extractAttr(feed, 'xml:lang');

  // Image
  let imageUrl: string | null = null;
  const logo = extractText(feed.logo);
  const icon = extractText(feed.icon);
  imageUrl = logo ?? icon;

  // Link
  const link = extractAtomLink(feed, 'alternate');

  // Categories
  const categories: string[] = [];
  const cats = ensureArray(feed.category);
  for (const cat of cats) {
    const catObj = cat as Record<string, unknown>;
    const term = extractAttr(catObj, 'term') ?? extractText(catObj);
    if (term) categories.push(term);
  }

  // Entries (episodes)
  const entries = ensureArray(feed.entry);
  const episodes: ParsedEpisode[] = entries.map(entry => parseAtomEntry(entry as Record<string, unknown>));

  return {
    title,
    description,
    author,
    imageUrl,
    language,
    categories,
    link,
    lastBuildDate: parseDate(extractText(feed.updated)),
    episodes,
  };
}

function parseAtomEntry(entry: Record<string, unknown>): ParsedEpisode {
  const title = extractText(entry.title) ?? 'Untitled Episode';
  const guid = extractText(entry.id) ?? title;
  const description = extractText(entry.content) ?? extractText(entry.summary);
  const pubDate = parseDate(extractText(entry.published) ?? extractText(entry.updated));

  // Look for enclosure-like link
  let enclosureUrl: string | null = null;
  let enclosureType: string | null = null;
  let enclosureLength: number | null = null;

  const links = ensureArray(entry.link);
  for (const linkItem of links) {
    const linkObj = linkItem as Record<string, unknown>;
    const rel = extractAttr(linkObj, 'rel');
    if (rel === 'enclosure') {
      enclosureUrl = extractAttr(linkObj, 'href');
      enclosureType = extractAttr(linkObj, 'type');
      const lengthStr = extractAttr(linkObj, 'length');
      enclosureLength = lengthStr ? parseInt(lengthStr, 10) || null : null;
      break;
    }
  }

  const durationSeconds = parseDuration(extractText(entry['itunes:duration']));
  const seasonNumber = parseIntOrNull(extractText(entry['itunes:season']));
  const episodeNumber = parseIntOrNull(extractText(entry['itunes:episode']));
  const episodeType = parseEpisodeType(extractText(entry['itunes:episodeType']));
  const chaptersUrl = extractPodcastChaptersUrl(entry);
  const transcriptUrl = extractPodcastTranscriptUrl(entry);

  let imageUrl: string | null = null;
  const itunesImage = entry['itunes:image'] as Record<string, unknown> | undefined;
  if (itunesImage) {
    imageUrl = extractAttr(itunesImage, 'href') ?? extractText(itunesImage);
  }

  return {
    guid,
    title,
    description,
    pubDate,
    durationSeconds,
    enclosureUrl,
    enclosureType,
    enclosureLength,
    seasonNumber,
    episodeNumber,
    episodeType,
    chaptersUrl,
    transcriptUrl,
    imageUrl,
  };
}

// =========================================================================
// RDF (RSS 1.0) Parsing
// =========================================================================

function parseRdf(rdf: Record<string, unknown>): ParsedFeed {
  const channel = rdf.channel as Record<string, unknown> | undefined;
  const title = channel ? extractText(channel.title) ?? 'Untitled Podcast' : 'Untitled Podcast';
  const description = channel ? extractText(channel.description) : null;

  const items = ensureArray(rdf.item);
  const episodes: ParsedEpisode[] = items.map(item => {
    const itemObj = item as Record<string, unknown>;
    const itemTitle = extractText(itemObj.title) ?? 'Untitled Episode';
    const guid = extractText(itemObj.link) ?? extractAttr(itemObj, 'rdf:about') ?? itemTitle;
    return {
      guid,
      title: itemTitle,
      description: extractText(itemObj.description),
      pubDate: parseDate(extractText(itemObj['dc:date'])),
      durationSeconds: null,
      enclosureUrl: null,
      enclosureType: null,
      enclosureLength: null,
      seasonNumber: null,
      episodeNumber: null,
      episodeType: 'full' as EpisodeType,
      chaptersUrl: null,
      transcriptUrl: null,
      imageUrl: null,
    };
  });

  return {
    title,
    description,
    author: channel ? extractText(channel['dc:creator']) : null,
    imageUrl: null,
    language: channel ? extractText(channel['dc:language']) : null,
    categories: [],
    link: channel ? extractText(channel.link) : null,
    lastBuildDate: null,
    episodes,
  };
}

// =========================================================================
// Helper Functions
// =========================================================================

function extractText(value: unknown): string | null {
  if (value === null || value === undefined) return null;
  if (typeof value === 'string') return value.trim() || null;
  if (typeof value === 'number') return String(value);
  if (typeof value === 'object') {
    const obj = value as Record<string, unknown>;
    if ('#text' in obj) return extractText(obj['#text']);
    if ('toString' in obj) {
      const str = String(obj);
      return str === '[object Object]' ? null : str.trim() || null;
    }
  }
  return null;
}

function extractAttr(obj: Record<string, unknown>, attr: string): string | null {
  const value = obj[`@_${attr}`];
  if (value === null || value === undefined) return null;
  return String(value).trim() || null;
}

function ensureArray(value: unknown): unknown[] {
  if (!value) return [];
  if (Array.isArray(value)) return value;
  return [value];
}

function extractCategories(channel: Record<string, unknown>): string[] {
  const categories: string[] = [];

  // itunes:category (can be nested)
  const itunesCats = ensureArray(channel['itunes:category']);
  for (const cat of itunesCats) {
    const catObj = cat as Record<string, unknown>;
    const text = extractAttr(catObj, 'text');
    if (text) categories.push(text);

    // Subcategories
    const subCats = ensureArray(catObj['itunes:category']);
    for (const subCat of subCats) {
      const subText = extractAttr(subCat as Record<string, unknown>, 'text');
      if (subText) categories.push(subText);
    }
  }

  // Plain category elements
  const plainCats = ensureArray(channel.category);
  for (const cat of plainCats) {
    const text = extractText(cat);
    if (text) categories.push(text);
  }

  return [...new Set(categories)];
}

function extractAtomAuthor(feed: Record<string, unknown>): string | null {
  const author = feed.author as Record<string, unknown> | undefined;
  if (author) {
    return extractText(author.name) ?? extractText(author.email);
  }
  return extractText(feed['itunes:author']);
}

function extractAtomLink(feed: Record<string, unknown>, rel: string): string | null {
  const links = ensureArray(feed.link);
  for (const link of links) {
    if (typeof link === 'string') return link;
    const linkObj = link as Record<string, unknown>;
    const linkRel = extractAttr(linkObj, 'rel');
    if (linkRel === rel || (!linkRel && rel === 'alternate')) {
      return extractAttr(linkObj, 'href');
    }
  }
  return null;
}

function extractPodcastChaptersUrl(item: Record<string, unknown>): string | null {
  const chapters = item['podcast:chapters'];
  if (!chapters) return null;
  const arr = ensureArray(chapters);
  for (const ch of arr) {
    const chObj = ch as Record<string, unknown>;
    const url = extractAttr(chObj, 'url');
    if (url) return url;
  }
  return null;
}

function extractPodcastTranscriptUrl(item: Record<string, unknown>): string | null {
  const transcripts = item['podcast:transcript'];
  if (!transcripts) return null;
  const arr = ensureArray(transcripts);
  for (const tr of arr) {
    const trObj = tr as Record<string, unknown>;
    const url = extractAttr(trObj, 'url');
    if (url) return url;
  }
  return null;
}

/**
 * Parse duration string to seconds.
 * Supports: "HH:MM:SS", "MM:SS", "SSS" (plain seconds), or "1h2m3s" formats
 */
export function parseDuration(value: string | null): number | null {
  if (!value) return null;
  const trimmed = value.trim();
  if (!trimmed) return null;

  // Plain number (seconds)
  if (/^\d+$/.test(trimmed)) {
    return parseInt(trimmed, 10);
  }

  // HH:MM:SS or MM:SS
  const colonParts = trimmed.split(':');
  if (colonParts.length === 3) {
    const hours = parseInt(colonParts[0], 10) || 0;
    const minutes = parseInt(colonParts[1], 10) || 0;
    const seconds = parseInt(colonParts[2], 10) || 0;
    return hours * 3600 + minutes * 60 + seconds;
  }
  if (colonParts.length === 2) {
    const minutes = parseInt(colonParts[0], 10) || 0;
    const seconds = parseInt(colonParts[1], 10) || 0;
    return minutes * 60 + seconds;
  }

  // "1h2m3s" format
  const hmsMatch = trimmed.match(/(?:(\d+)\s*h)?\s*(?:(\d+)\s*m)?\s*(?:(\d+)\s*s)?/i);
  if (hmsMatch && (hmsMatch[1] || hmsMatch[2] || hmsMatch[3])) {
    const hours = parseInt(hmsMatch[1] ?? '0', 10);
    const minutes = parseInt(hmsMatch[2] ?? '0', 10);
    const seconds = parseInt(hmsMatch[3] ?? '0', 10);
    return hours * 3600 + minutes * 60 + seconds;
  }

  return null;
}

/**
 * Parse an RFC 2822 or ISO 8601 date string
 */
function parseDate(value: string | null): Date | null {
  if (!value) return null;
  const date = new Date(value);
  if (isNaN(date.getTime())) return null;
  return date;
}

function parseIntOrNull(value: string | null): number | null {
  if (!value) return null;
  const num = parseInt(value, 10);
  return isNaN(num) ? null : num;
}

function parseEpisodeType(value: string | null): EpisodeType {
  if (!value) return 'full';
  const lower = value.toLowerCase();
  if (lower === 'trailer') return 'trailer';
  if (lower === 'bonus') return 'bonus';
  return 'full';
}
