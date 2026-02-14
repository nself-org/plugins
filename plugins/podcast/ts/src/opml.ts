/**
 * OPML Import/Export
 * Parse and generate OPML XML for podcast feed subscriptions
 */

import { XMLParser, XMLBuilder } from 'fast-xml-parser';
import type { OpmlOutline, OpmlDocument, FeedRecord } from './types.js';

const opmlParser = new XMLParser({
  ignoreAttributes: false,
  attributeNamePrefix: '@_',
  textNodeName: '#text',
  isArray: (tagName) => tagName === 'outline',
  parseTagValue: true,
  trimValues: true,
});

const opmlBuilder = new XMLBuilder({
  ignoreAttributes: false,
  attributeNamePrefix: '@_',
  format: true,
  indentBy: '  ',
  suppressEmptyNode: true,
  suppressBooleanAttributes: false,
});

// =========================================================================
// OPML Import (Parse)
// =========================================================================

/**
 * Parse an OPML string into a list of feed URLs with metadata
 */
export function parseOpml(opmlContent: string): OpmlOutline[] {
  let parsed: Record<string, unknown>;
  try {
    parsed = opmlParser.parse(opmlContent) as Record<string, unknown>;
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown parse error';
    throw new Error(`Failed to parse OPML: ${message}`);
  }

  const opml = parsed.opml as Record<string, unknown> | undefined;
  if (!opml) {
    throw new Error('Invalid OPML: missing <opml> root element');
  }

  const body = opml.body as Record<string, unknown> | undefined;
  if (!body) {
    throw new Error('Invalid OPML: missing <body> element');
  }

  const outlines = extractOutlines(body);
  return flattenFeedOutlines(outlines);
}

/**
 * Extract feed URLs from parsed OPML outlines
 * Returns only outlines that have xmlUrl (actual feed subscriptions)
 */
export function extractFeedUrls(outlines: OpmlOutline[]): Array<{ url: string; title: string }> {
  const feeds: Array<{ url: string; title: string }> = [];

  for (const outline of outlines) {
    if (outline.xmlUrl) {
      feeds.push({
        url: outline.xmlUrl,
        title: outline.title ?? outline.text ?? outline.xmlUrl,
      });
    }
    if (outline.children) {
      feeds.push(...extractFeedUrls(outline.children));
    }
  }

  return feeds;
}

function extractOutlines(parent: Record<string, unknown>): OpmlOutline[] {
  const rawOutlines = parent.outline;
  if (!rawOutlines) return [];

  const outlineArray = Array.isArray(rawOutlines) ? rawOutlines : [rawOutlines];
  return outlineArray.map((outline) => {
    const obj = outline as Record<string, unknown>;
    const result: OpmlOutline = {
      text: extractAttr(obj, 'text') ?? extractAttr(obj, 'title') ?? 'Untitled',
      title: extractAttr(obj, 'title') ?? undefined,
      type: extractAttr(obj, 'type') ?? undefined,
      xmlUrl: extractAttr(obj, 'xmlUrl') ?? undefined,
      htmlUrl: extractAttr(obj, 'htmlUrl') ?? undefined,
    };

    // Recursively extract nested outlines (groups)
    if (obj.outline) {
      result.children = extractOutlines(obj);
    }

    return result;
  });
}

function flattenFeedOutlines(outlines: OpmlOutline[]): OpmlOutline[] {
  return outlines;
}

function extractAttr(obj: Record<string, unknown>, attr: string): string | null {
  const value = obj[`@_${attr}`];
  if (value === null || value === undefined) return null;
  return String(value).trim() || null;
}

// =========================================================================
// OPML Export (Generate)
// =========================================================================

/**
 * Generate OPML XML from a list of feed records
 */
export function generateOpml(feeds: FeedRecord[], title = 'nself Podcast Subscriptions'): string {
  const dateCreated = new Date().toUTCString();

  const outlines = feeds.map(feed => ({
    '@_text': feed.title ?? feed.url,
    '@_title': feed.title ?? feed.url,
    '@_type': 'rss',
    '@_xmlUrl': feed.url,
  }));

  const opmlDoc = {
    '?xml': {
      '@_version': '1.0',
      '@_encoding': 'UTF-8',
    },
    opml: {
      '@_version': '2.0',
      head: {
        title,
        dateCreated,
      },
      body: {
        outline: outlines,
      },
    },
  };

  const xml = opmlBuilder.build(opmlDoc) as string;

  // Ensure proper XML declaration at the top
  if (!xml.startsWith('<?xml')) {
    return `<?xml version="1.0" encoding="UTF-8"?>\n${xml}`;
  }

  return xml;
}

/**
 * Parse OPML and return structured document
 */
export function parseOpmlDocument(opmlContent: string): OpmlDocument {
  let parsed: Record<string, unknown>;
  try {
    parsed = opmlParser.parse(opmlContent) as Record<string, unknown>;
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown parse error';
    throw new Error(`Failed to parse OPML: ${message}`);
  }

  const opml = parsed.opml as Record<string, unknown> | undefined;
  if (!opml) {
    throw new Error('Invalid OPML: missing <opml> root element');
  }

  const head = opml.head as Record<string, unknown> | undefined;
  const body = opml.body as Record<string, unknown> | undefined;

  if (!body) {
    throw new Error('Invalid OPML: missing <body> element');
  }

  return {
    title: head ? String(head.title ?? 'Untitled') : 'Untitled',
    dateCreated: head ? String(head.dateCreated ?? '') : '',
    outlines: extractOutlines(body),
  };
}
