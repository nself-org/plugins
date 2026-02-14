/**
 * ContentDirectory SOAP Service
 * Handles UPnP ContentDirectory:1 SOAP actions (Browse, Search, GetSystemUpdateID)
 */

import { createLogger } from '@nself/plugin-utils';
import type { DlnaDatabase } from './database.js';
import type { BrowseRequest, BrowseResponse, SearchRequest, SOAPAction } from './types.js';
import { buildDIDLResponse, buildMetadataResponse, buildRootContainerXml, wrapDIDLLite } from './didl.js';
import { getSupportedProtocolInfo } from './upnp.js';

const logger = createLogger('dlna:content-directory');

/** System update ID, incremented when content changes */
let systemUpdateId = 1;

/**
 * Increment the system update ID (called after media scans)
 */
export function incrementSystemUpdateId(): void {
  systemUpdateId++;
}

/**
 * Get the current system update ID
 */
export function getSystemUpdateId(): number {
  return systemUpdateId;
}

/**
 * Escape a string for safe XML inclusion in SOAP responses
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
 * Parse a SOAP XML request body into an action object
 */
export function parseSOAPAction(xmlBody: string, soapActionHeader: string): SOAPAction {
  // Extract service type and action name from SOAPAction header
  // Format: "urn:schemas-upnp-org:service:ContentDirectory:1#Browse"
  const headerMatch = soapActionHeader.replace(/"/g, '').match(/(.+)#(.+)/);

  const serviceType = headerMatch ? headerMatch[1] : '';
  const actionName = headerMatch ? headerMatch[2] : '';

  // Parse arguments from the XML body
  const args: Record<string, string> = {};

  // Extract each argument value using regex (simple XML parsing without external deps)
  const argPatterns: Record<string, string[]> = {
    Browse: ['ObjectID', 'BrowseFlag', 'Filter', 'StartingIndex', 'RequestedCount', 'SortCriteria'],
    Search: ['ContainerID', 'SearchCriteria', 'Filter', 'StartingIndex', 'RequestedCount', 'SortCriteria'],
    GetSystemUpdateID: [],
    GetSearchCapabilities: [],
    GetSortCapabilities: [],
    GetProtocolInfo: [],
    GetCurrentConnectionIDs: [],
    GetCurrentConnectionInfo: ['ConnectionID'],
  };

  const expectedArgs = argPatterns[actionName] ?? [];

  for (const argName of expectedArgs) {
    const regex = new RegExp(`<${argName}[^>]*>([\\s\\S]*?)</${argName}>`, 'i');
    const match = xmlBody.match(regex);
    if (match) {
      // Unescape XML entities in the extracted value
      args[argName] = match[1]
        .replace(/&amp;/g, '&')
        .replace(/&lt;/g, '<')
        .replace(/&gt;/g, '>')
        .replace(/&quot;/g, '"')
        .replace(/&apos;/g, "'");
    } else {
      args[argName] = '';
    }
  }

  return { serviceType, actionName, arguments: args };
}

/**
 * Handle a ContentDirectory SOAP action
 */
export async function handleContentDirectoryAction(
  action: SOAPAction,
  db: DlnaDatabase,
  baseUrl: string
): Promise<string> {
  switch (action.actionName) {
    case 'Browse':
      return handleBrowse(action, db, baseUrl);
    case 'Search':
      return handleSearch(action, db, baseUrl);
    case 'GetSystemUpdateID':
      return handleGetSystemUpdateID(action);
    case 'GetSearchCapabilities':
      return handleGetSearchCapabilities(action);
    case 'GetSortCapabilities':
      return handleGetSortCapabilities(action);
    default:
      logger.warn('Unknown ContentDirectory action', { action: action.actionName });
      return buildSOAPFault(401, 'Invalid Action');
  }
}

/**
 * Handle a ConnectionManager SOAP action
 */
export async function handleConnectionManagerAction(
  action: SOAPAction
): Promise<string> {
  switch (action.actionName) {
    case 'GetProtocolInfo':
      return handleGetProtocolInfo(action);
    case 'GetCurrentConnectionIDs':
      return handleGetCurrentConnectionIDs(action);
    case 'GetCurrentConnectionInfo':
      return handleGetCurrentConnectionInfo(action);
    default:
      logger.warn('Unknown ConnectionManager action', { action: action.actionName });
      return buildSOAPFault(401, 'Invalid Action');
  }
}

// ---------------------------------------------------------------------------
// ContentDirectory Action Handlers
// ---------------------------------------------------------------------------

async function handleBrowse(
  action: SOAPAction,
  db: DlnaDatabase,
  baseUrl: string
): Promise<string> {
  const request: BrowseRequest = {
    objectId: action.arguments.ObjectID ?? '0',
    browseFlag: (action.arguments.BrowseFlag ?? 'BrowseDirectChildren') as BrowseRequest['browseFlag'],
    filter: action.arguments.Filter ?? '*',
    startingIndex: parseInt(action.arguments.StartingIndex ?? '0', 10),
    requestedCount: parseInt(action.arguments.RequestedCount ?? '0', 10),
    sortCriteria: action.arguments.SortCriteria ?? '',
  };

  logger.debug('Browse request', {
    objectId: request.objectId,
    flag: request.browseFlag,
    start: request.startingIndex,
    count: request.requestedCount,
  });

  let response: BrowseResponse;

  if (request.browseFlag === 'BrowseMetadata') {
    response = await browseMetadata(request, db, baseUrl);
  } else {
    response = await browseDirectChildren(request, db, baseUrl);
  }

  return buildSOAPResponse('Browse', action.serviceType, {
    Result: escapeXml(response.result),
    NumberReturned: String(response.numberReturned),
    TotalMatches: String(response.totalMatches),
    UpdateID: String(response.updateId),
  });
}

async function browseMetadata(
  request: BrowseRequest,
  db: DlnaDatabase,
  baseUrl: string
): Promise<BrowseResponse> {
  // Root container
  if (request.objectId === '0') {
    const { totalCount } = await db.listChildren(null, 0, 0);
    const result = wrapDIDLLite(buildRootContainerXml(totalCount));
    return {
      result,
      numberReturned: 1,
      totalMatches: 1,
      updateId: systemUpdateId,
    };
  }

  // Specific item/container
  const item = await db.getMediaItem(request.objectId);
  if (!item) {
    return {
      result: wrapDIDLLite(''),
      numberReturned: 0,
      totalMatches: 0,
      updateId: systemUpdateId,
    };
  }

  const childCount = item.object_type === 'container'
    ? await db.getChildCount(item.id)
    : 0;

  const result = buildMetadataResponse(item, baseUrl, childCount);

  return {
    result,
    numberReturned: 1,
    totalMatches: 1,
    updateId: systemUpdateId,
  };
}

async function browseDirectChildren(
  request: BrowseRequest,
  db: DlnaDatabase,
  baseUrl: string
): Promise<BrowseResponse> {
  const parentId = request.objectId === '0' ? null : request.objectId;
  const { items, totalCount } = await db.listChildren(
    parentId,
    request.startingIndex,
    request.requestedCount
  );

  // Get child counts for containers in the result
  const childCounts = new Map<string, number>();
  for (const item of items) {
    if (item.object_type === 'container') {
      childCounts.set(item.id, await db.getChildCount(item.id));
    }
  }

  const result = buildDIDLResponse(items, baseUrl, childCounts);

  return {
    result,
    numberReturned: items.length,
    totalMatches: totalCount,
    updateId: systemUpdateId,
  };
}

async function handleSearch(
  action: SOAPAction,
  db: DlnaDatabase,
  baseUrl: string
): Promise<string> {
  const request: SearchRequest = {
    containerId: action.arguments.ContainerID ?? '0',
    searchCriteria: action.arguments.SearchCriteria ?? '',
    filter: action.arguments.Filter ?? '*',
    startingIndex: parseInt(action.arguments.StartingIndex ?? '0', 10),
    requestedCount: parseInt(action.arguments.RequestedCount ?? '0', 10),
    sortCriteria: action.arguments.SortCriteria ?? '',
  };

  logger.debug('Search request', {
    criteria: request.searchCriteria,
    start: request.startingIndex,
    count: request.requestedCount,
  });

  const { items, totalCount } = await db.searchMediaItems(
    request.searchCriteria,
    request.startingIndex,
    request.requestedCount
  );

  const childCounts = new Map<string, number>();
  for (const item of items) {
    if (item.object_type === 'container') {
      childCounts.set(item.id, await db.getChildCount(item.id));
    }
  }

  const result = buildDIDLResponse(items, baseUrl, childCounts);

  return buildSOAPResponse('Search', action.serviceType, {
    Result: escapeXml(result),
    NumberReturned: String(items.length),
    TotalMatches: String(totalCount),
    UpdateID: String(systemUpdateId),
  });
}

function handleGetSystemUpdateID(action: SOAPAction): string {
  return buildSOAPResponse('GetSystemUpdateID', action.serviceType, {
    Id: String(systemUpdateId),
  });
}

function handleGetSearchCapabilities(action: SOAPAction): string {
  return buildSOAPResponse('GetSearchCapabilities', action.serviceType, {
    SearchCaps: 'dc:title,dc:creator,upnp:class,upnp:artist,upnp:album,upnp:genre',
  });
}

function handleGetSortCapabilities(action: SOAPAction): string {
  return buildSOAPResponse('GetSortCapabilities', action.serviceType, {
    SortCaps: 'dc:title,dc:creator,upnp:artist,upnp:album',
  });
}

// ---------------------------------------------------------------------------
// ConnectionManager Action Handlers
// ---------------------------------------------------------------------------

function handleGetProtocolInfo(action: SOAPAction): string {
  return buildSOAPResponse('GetProtocolInfo', action.serviceType, {
    Source: getSupportedProtocolInfo(),
    Sink: '',
  });
}

function handleGetCurrentConnectionIDs(action: SOAPAction): string {
  return buildSOAPResponse('GetCurrentConnectionIDs', action.serviceType, {
    ConnectionIDs: '0',
  });
}

function handleGetCurrentConnectionInfo(action: SOAPAction): string {
  return buildSOAPResponse('GetCurrentConnectionInfo', action.serviceType, {
    RcsID: '-1',
    AVTransportID: '-1',
    ProtocolInfo: '',
    PeerConnectionManager: '',
    PeerConnectionID: '-1',
    Direction: 'Output',
    Status: 'OK',
  });
}

// ---------------------------------------------------------------------------
// SOAP XML Builders
// ---------------------------------------------------------------------------

/**
 * Build a SOAP response envelope
 */
function buildSOAPResponse(
  actionName: string,
  serviceType: string,
  body: Record<string, string>
): string {
  const bodyElements = Object.entries(body)
    .map(([key, value]) => `      <${key}>${value}</${key}>`)
    .join('\n');

  return `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"
            s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <u:${actionName}Response xmlns:u="${serviceType}">
${bodyElements}
    </u:${actionName}Response>
  </s:Body>
</s:Envelope>`;
}

/**
 * Build a SOAP fault response
 */
function buildSOAPFault(errorCode: number, errorDescription: string): string {
  return `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"
            s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <s:Fault>
      <faultcode>s:Client</faultcode>
      <faultstring>UPnPError</faultstring>
      <detail>
        <UPnPError xmlns="urn:schemas-upnp-org:control-1-0">
          <errorCode>${errorCode}</errorCode>
          <errorDescription>${escapeXml(errorDescription)}</errorDescription>
        </UPnPError>
      </detail>
    </s:Fault>
  </s:Body>
</s:Envelope>`;
}
