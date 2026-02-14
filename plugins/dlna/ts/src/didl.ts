/**
 * DIDL-Lite XML Builder
 * Generates Digital Item Declaration Language XML for UPnP ContentDirectory responses
 */

import type { MediaItemRecord } from './types.js';
import { getProtocolInfo } from './types.js';

/**
 * Escape a string for safe XML inclusion
 */
function escapeXml(str: string): string {
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;');
}

/**
 * Format seconds into HH:MM:SS.000 duration string (UPnP format)
 */
function formatDuration(seconds: number): string {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = seconds % 60;
  return `${hours}:${String(minutes).padStart(2, '0')}:${String(secs).padStart(2, '0')}.000`;
}

/**
 * Build a complete DIDL-Lite XML document wrapping the provided inner elements
 */
export function wrapDIDLLite(innerXml: string): string {
  return [
    '<DIDL-Lite',
    '  xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/"',
    '  xmlns:dc="http://purl.org/dc/elements/1.1/"',
    '  xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"',
    '  xmlns:dlna="urn:schemas-dlna-org:metadata-1-0/">',
    innerXml,
    '</DIDL-Lite>',
  ].join('\n');
}

/**
 * Build DIDL-Lite XML for a container element
 */
export function buildContainerXml(
  id: string,
  parentId: string,
  title: string,
  upnpClass: string,
  childCount: number
): string {
  return [
    `<container id="${escapeXml(id)}" parentID="${escapeXml(parentId)}" restricted="true" childCount="${childCount}">`,
    `  <dc:title>${escapeXml(title)}</dc:title>`,
    `  <upnp:class>${escapeXml(upnpClass)}</upnp:class>`,
    `</container>`,
  ].join('\n');
}

/**
 * Build DIDL-Lite XML for an item element from a database record
 */
export function buildItemXml(
  item: MediaItemRecord,
  baseUrl: string
): string {
  const lines: string[] = [];

  lines.push(`<item id="${escapeXml(item.id)}" parentID="${escapeXml(item.parent_id ?? '0')}" restricted="true">`);
  lines.push(`  <dc:title>${escapeXml(item.title)}</dc:title>`);
  lines.push(`  <upnp:class>${escapeXml(item.upnp_class)}</upnp:class>`);

  // Optional metadata elements
  if (item.artist) {
    lines.push(`  <dc:creator>${escapeXml(item.artist)}</dc:creator>`);
    lines.push(`  <upnp:artist>${escapeXml(item.artist)}</upnp:artist>`);
  }
  if (item.album) {
    lines.push(`  <upnp:album>${escapeXml(item.album)}</upnp:album>`);
  }
  if (item.genre) {
    lines.push(`  <upnp:genre>${escapeXml(item.genre)}</upnp:genre>`);
  }

  // Album art / thumbnail
  if (item.thumbnail_path) {
    lines.push(`  <upnp:albumArtURI>${escapeXml(baseUrl)}/thumbnails/${escapeXml(item.id)}</upnp:albumArtURI>`);
  }

  // Resource element with protocol info
  if (item.mime_type) {
    const protocolInfo = getProtocolInfo(item.mime_type);
    const resAttrs: string[] = [`protocolInfo="${escapeXml(protocolInfo)}"`];

    if (item.file_size) {
      resAttrs.push(`size="${item.file_size}"`);
    }
    if (item.duration_seconds) {
      resAttrs.push(`duration="${formatDuration(item.duration_seconds)}"`);
    }
    if (item.resolution) {
      resAttrs.push(`resolution="${escapeXml(item.resolution)}"`);
    }
    if (item.bitrate) {
      resAttrs.push(`bitrate="${item.bitrate}"`);
    }

    const mediaUrl = `${baseUrl}/media/${item.id}`;
    lines.push(`  <res ${resAttrs.join(' ')}>${escapeXml(mediaUrl)}</res>`);
  }

  lines.push(`</item>`);
  return lines.join('\n');
}

/**
 * Build DIDL-Lite XML for the virtual root container (id "0")
 */
export function buildRootContainerXml(childCount: number): string {
  return buildContainerXml('0', '-1', 'Root', 'object.container', childCount);
}

/**
 * Build a complete DIDL-Lite response from a list of media items.
 * Containers and items are rendered appropriately.
 */
export function buildDIDLResponse(
  items: MediaItemRecord[],
  baseUrl: string,
  childCounts: Map<string, number>
): string {
  const elements: string[] = [];

  for (const item of items) {
    if (item.object_type === 'container') {
      const count = childCounts.get(item.id) ?? 0;
      elements.push(buildContainerXml(
        item.id,
        item.parent_id ?? '0',
        item.title,
        item.upnp_class,
        count
      ));
    } else {
      elements.push(buildItemXml(item, baseUrl));
    }
  }

  return wrapDIDLLite(elements.join('\n'));
}

/**
 * Build DIDL-Lite metadata for a single item (BrowseMetadata response)
 */
export function buildMetadataResponse(
  item: MediaItemRecord,
  baseUrl: string,
  childCount: number
): string {
  let inner: string;

  if (item.object_type === 'container') {
    inner = buildContainerXml(
      item.id,
      item.parent_id ?? '-1',
      item.title,
      item.upnp_class,
      childCount
    );
  } else {
    inner = buildItemXml(item, baseUrl);
  }

  return wrapDIDLLite(inner);
}
