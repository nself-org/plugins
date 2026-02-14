/**
 * DLNA Plugin for nself
 * DLNA/UPnP media server with SSDP discovery, ContentDirectory, and HTTP streaming
 */

export { DlnaDatabase } from './database.js';
export { SSDPServer } from './ssdp.js';
export { MediaScanner } from './media-scanner.js';
export { createServer } from './server.js';
export { loadConfig, getLocalIpAddress } from './config.js';
export {
  generateDeviceDescription,
  generateContentDirectorySCPD,
  generateConnectionManagerSCPD,
  getSupportedProtocolInfo,
} from './upnp.js';
export {
  parseSOAPAction,
  handleContentDirectoryAction,
  handleConnectionManagerAction,
  getSystemUpdateId,
  incrementSystemUpdateId,
} from './content-directory.js';
export {
  wrapDIDLLite,
  buildContainerXml,
  buildItemXml,
  buildRootContainerXml,
  buildDIDLResponse,
  buildMetadataResponse,
} from './didl.js';
export { handleMediaStream, handleThumbnailStream } from './streaming.js';
export * from './types.js';
