/**
 * DLNA Plugin Types
 * All TypeScript interfaces for DLNA/UPnP media server
 */

// ============================================================================
// Database Record Types
// ============================================================================

export interface MediaItemRecord {
  id: string;
  source_account_id: string;
  parent_id: string | null;
  object_type: ObjectType;
  upnp_class: string;
  title: string;
  file_path: string | null;
  file_size: number | null;
  mime_type: string | null;
  duration_seconds: number | null;
  resolution: string | null;
  bitrate: number | null;
  album: string | null;
  artist: string | null;
  genre: string | null;
  thumbnail_path: string | null;
  sort_order: number;
  created_at: Date;
  updated_at: Date;
  synced_at: Date;
}

export interface RendererRecord {
  id: string;
  source_account_id: string;
  usn: string;
  friendly_name: string | null;
  location: string;
  ip_address: string | null;
  device_type: string | null;
  manufacturer: string | null;
  model_name: string | null;
  last_seen_at: Date;
  created_at: Date;
  updated_at: Date;
}

// ============================================================================
// Object / Class Types
// ============================================================================

export type ObjectType = 'container' | 'item';

/** UPnP object class constants */
export const UPnPClass = {
  // Containers
  CONTAINER: 'object.container',
  CONTAINER_STORAGE: 'object.container.storageFolder',
  CONTAINER_ALBUM: 'object.container.album.musicAlbum',
  CONTAINER_PERSON: 'object.container.person.musicArtist',
  CONTAINER_GENRE: 'object.container.genre.musicGenre',
  CONTAINER_PLAYLIST: 'object.container.playlistContainer',

  // Video items
  VIDEO: 'object.item.videoItem',
  MOVIE: 'object.item.videoItem.movie',
  VIDEO_BROADCAST: 'object.item.videoItem.videoBroadcast',

  // Audio items
  AUDIO: 'object.item.audioItem',
  MUSIC_TRACK: 'object.item.audioItem.musicTrack',
  AUDIO_BROADCAST: 'object.item.audioItem.audioBroadcast',

  // Image items
  IMAGE: 'object.item.imageItem',
  PHOTO: 'object.item.imageItem.photo',
} as const;

// ============================================================================
// SSDP Types
// ============================================================================

export interface SSDPMessage {
  method: string;
  headers: Record<string, string>;
  address: string;
  port: number;
}

export interface SSDPConfig {
  address: string;
  port: number;
  uuid: string;
  friendlyName: string;
  httpPort: number;
  httpHost: string;
  advertiseInterval: number;
}

export interface DiscoveredRenderer {
  usn: string;
  location: string;
  ipAddress: string;
  deviceType: string;
  server: string;
  maxAge: number;
  lastSeen: Date;
}

// ============================================================================
// UPnP Types
// ============================================================================

export interface DeviceDescription {
  uuid: string;
  friendlyName: string;
  manufacturer: string;
  manufacturerURL: string;
  modelName: string;
  modelNumber: string;
  modelDescription: string;
  modelURL: string;
  serialNumber: string;
  presentationURL: string;
}

export interface ServiceDescription {
  serviceType: string;
  serviceId: string;
  SCPDURL: string;
  controlURL: string;
  eventSubURL: string;
}

// ============================================================================
// ContentDirectory Types
// ============================================================================

export type BrowseFlag = 'BrowseDirectChildren' | 'BrowseMetadata';

export interface BrowseRequest {
  objectId: string;
  browseFlag: BrowseFlag;
  filter: string;
  startingIndex: number;
  requestedCount: number;
  sortCriteria: string;
}

export interface BrowseResponse {
  result: string;
  numberReturned: number;
  totalMatches: number;
  updateId: number;
}

export interface SearchRequest {
  containerId: string;
  searchCriteria: string;
  filter: string;
  startingIndex: number;
  requestedCount: number;
  sortCriteria: string;
}

// ============================================================================
// DIDL-Lite Types
// ============================================================================

export interface DIDLContainer {
  id: string;
  parentId: string;
  restricted: boolean;
  childCount: number;
  title: string;
  upnpClass: string;
}

export interface DIDLItem {
  id: string;
  parentId: string;
  restricted: boolean;
  title: string;
  upnpClass: string;
  artist?: string;
  album?: string;
  genre?: string;
  resources: DIDLResource[];
  albumArtURI?: string;
}

export interface DIDLResource {
  protocolInfo: string;
  uri: string;
  size?: number;
  duration?: string;
  resolution?: string;
  bitrate?: number;
}

// ============================================================================
// SOAP Types
// ============================================================================

export interface SOAPAction {
  serviceType: string;
  actionName: string;
  arguments: Record<string, string>;
}

export interface SOAPResponse {
  actionName: string;
  serviceType: string;
  body: Record<string, string>;
}

// ============================================================================
// Connection Manager Types
// ============================================================================

export interface ProtocolInfo {
  protocol: string;
  network: string;
  contentFormat: string;
  additionalInfo: string;
}

export interface ConnectionInfo {
  connectionId: number;
  rcsId: number;
  avTransportId: number;
  protocolInfo: string;
  peerConnectionManager: string;
  peerConnectionId: number;
  direction: 'Input' | 'Output';
  status: 'OK' | 'ContentFormatMismatch' | 'InsufficientBandwidth' | 'UnreliableChannel' | 'Unknown';
}

// ============================================================================
// Media Scanner Types
// ============================================================================

export interface ScanResult {
  totalFiles: number;
  newFiles: number;
  updatedFiles: number;
  removedFiles: number;
  errors: string[];
  duration: number;
}

export interface MediaFileInfo {
  filePath: string;
  fileName: string;
  fileSize: number;
  mimeType: string;
  upnpClass: string;
  modifiedAt: Date;
}

// ============================================================================
// Server Types
// ============================================================================

export interface DlnaServerStatus {
  plugin: string;
  version: string;
  status: string;
  friendlyName: string;
  uuid: string;
  httpPort: number;
  ssdpPort: number;
  mediaItems: number;
  renderers: number;
  mediaPaths: string[];
  uptime: number;
  timestamp: string;
}

// ============================================================================
// MIME Type Map
// ============================================================================

export const MIME_TYPES: Record<string, string> = {
  // Video
  '.mp4': 'video/mp4',
  '.mkv': 'video/x-matroska',
  '.avi': 'video/x-msvideo',
  '.mov': 'video/quicktime',
  '.wmv': 'video/x-ms-wmv',
  '.flv': 'video/x-flv',
  '.webm': 'video/webm',
  '.m4v': 'video/mp4',
  '.ts': 'video/mp2t',
  '.m2ts': 'video/mp2t',
  '.3gp': 'video/3gpp',
  '.ogv': 'video/ogg',
  '.mpg': 'video/mpeg',
  '.mpeg': 'video/mpeg',

  // Audio
  '.mp3': 'audio/mpeg',
  '.flac': 'audio/flac',
  '.wav': 'audio/wav',
  '.aac': 'audio/aac',
  '.ogg': 'audio/ogg',
  '.wma': 'audio/x-ms-wma',
  '.m4a': 'audio/mp4',
  '.opus': 'audio/opus',
  '.aiff': 'audio/aiff',
  '.alac': 'audio/mp4',

  // Image
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.png': 'image/png',
  '.gif': 'image/gif',
  '.bmp': 'image/bmp',
  '.webp': 'image/webp',
  '.tiff': 'image/tiff',
  '.tif': 'image/tiff',
  '.svg': 'image/svg+xml',
};

/**
 * Supported DLNA protocol info strings for each MIME type
 */
export const DLNA_PROFILES: Record<string, string> = {
  'video/mp4': 'http-get:*:video/mp4:DLNA.ORG_PN=AVC_MP4_MP_SD_AAC_MULT5;DLNA.ORG_OP=01;DLNA.ORG_FLAGS=01500000000000000000000000000000',
  'video/x-matroska': 'http-get:*:video/x-matroska:*',
  'video/x-msvideo': 'http-get:*:video/x-msvideo:*',
  'video/quicktime': 'http-get:*:video/quicktime:*',
  'video/x-ms-wmv': 'http-get:*:video/x-ms-wmv:DLNA.ORG_PN=WMVHIGH_FULL',
  'video/webm': 'http-get:*:video/webm:*',
  'video/mpeg': 'http-get:*:video/mpeg:DLNA.ORG_PN=MPEG_PS_NTSC',
  'video/mp2t': 'http-get:*:video/mp2t:*',
  'audio/mpeg': 'http-get:*:audio/mpeg:DLNA.ORG_PN=MP3;DLNA.ORG_OP=01',
  'audio/flac': 'http-get:*:audio/flac:*',
  'audio/wav': 'http-get:*:audio/wav:*',
  'audio/aac': 'http-get:*:audio/aac:DLNA.ORG_PN=AAC_ISO',
  'audio/mp4': 'http-get:*:audio/mp4:*',
  'audio/ogg': 'http-get:*:audio/ogg:*',
  'image/jpeg': 'http-get:*:image/jpeg:DLNA.ORG_PN=JPEG_LRG;DLNA.ORG_OP=01',
  'image/png': 'http-get:*:image/png:DLNA.ORG_PN=PNG_LRG;DLNA.ORG_OP=01',
  'image/gif': 'http-get:*:image/gif:*',
  'image/bmp': 'http-get:*:image/bmp:*',
};

/**
 * Get DLNA protocol info for a MIME type.
 * Falls back to a generic http-get protocol info if no profile exists.
 */
export function getProtocolInfo(mimeType: string): string {
  return DLNA_PROFILES[mimeType] ?? `http-get:*:${mimeType}:*`;
}

/**
 * Determine the UPnP class from a MIME type
 */
export function getUpnpClassForMime(mimeType: string): string {
  if (mimeType.startsWith('video/')) return UPnPClass.VIDEO;
  if (mimeType.startsWith('audio/')) return UPnPClass.MUSIC_TRACK;
  if (mimeType.startsWith('image/')) return UPnPClass.PHOTO;
  return 'object.item';
}

/**
 * Determine the media category from a MIME type
 */
export function getMediaCategory(mimeType: string): 'Video' | 'Audio' | 'Image' | 'Unknown' {
  if (mimeType.startsWith('video/')) return 'Video';
  if (mimeType.startsWith('audio/')) return 'Audio';
  if (mimeType.startsWith('image/')) return 'Image';
  return 'Unknown';
}
