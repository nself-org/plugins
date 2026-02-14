/**
 * Subsonic Plugin Types
 * All interfaces for database records, API responses, and configuration
 */

// ─── Database Records ────────────────────────────────────────────────────────

export interface ArtistRecord {
  id: string;
  source_account_id: string;
  name: string;
  sort_name: string | null;
  image_url: string | null;
  album_count: number;
  starred: boolean;
  starred_at: Date | null;
  created_at: Date;
  updated_at: Date;
  synced_at: Date;
}

export interface AlbumRecord {
  id: string;
  source_account_id: string;
  artist_id: string | null;
  title: string;
  sort_title: string | null;
  year: number | null;
  genre: string | null;
  cover_art_path: string | null;
  song_count: number;
  duration_seconds: number;
  play_count: number;
  starred: boolean;
  starred_at: Date | null;
  created_at: Date;
  updated_at: Date;
  synced_at: Date;
}

export interface SongRecord {
  id: string;
  source_account_id: string;
  album_id: string | null;
  artist_id: string | null;
  title: string;
  track_number: number | null;
  disc_number: number;
  year: number | null;
  genre: string | null;
  duration_seconds: number | null;
  file_path: string;
  file_size: number | null;
  bitrate: number | null;
  content_type: string | null;
  cover_art_path: string | null;
  play_count: number;
  last_played_at: Date | null;
  starred: boolean;
  starred_at: Date | null;
  created_at: Date;
  updated_at: Date;
  synced_at: Date;
}

export interface PlaylistRecord {
  id: string;
  source_account_id: string;
  name: string;
  comment: string | null;
  owner: string;
  public: boolean;
  song_count: number;
  duration_seconds: number;
  created_at: Date;
  updated_at: Date;
  synced_at: Date;
}

export interface PlaylistSongRecord {
  id: string;
  playlist_id: string;
  song_id: string;
  position: number;
  created_at: Date;
}

export interface ScrobbleRecord {
  id: string;
  source_account_id: string;
  song_id: string;
  user_name: string;
  scrobbled_at: Date;
  submission: boolean;
}

export interface MusicFolderRecord {
  id: string;
  source_account_id: string;
  name: string;
  path: string;
  created_at: Date;
}

// ─── Subsonic API Response Types ─────────────────────────────────────────────

export interface SubsonicResponse {
  'subsonic-response': {
    status: 'ok' | 'failed';
    version: string;
    type: string;
    serverVersion: string;
    openSubsonic: boolean;
    error?: SubsonicError;
    [key: string]: unknown;
  };
}

export interface SubsonicError {
  code: number;
  message: string;
}

export interface SubsonicArtist {
  id: string;
  name: string;
  albumCount?: number;
  starred?: string;
  coverArt?: string;
}

export interface SubsonicAlbum {
  id: string;
  name: string;
  artist?: string;
  artistId?: string;
  coverArt?: string;
  songCount?: number;
  duration?: number;
  playCount?: number;
  created?: string;
  starred?: string;
  year?: number;
  genre?: string;
  song?: SubsonicSong[];
}

export interface SubsonicSong {
  id: string;
  parent?: string;
  isDir: boolean;
  title: string;
  album?: string;
  artist?: string;
  track?: number;
  year?: number;
  genre?: string;
  coverArt?: string;
  size?: number;
  contentType?: string;
  suffix?: string;
  duration?: number;
  bitRate?: number;
  path?: string;
  playCount?: number;
  created?: string;
  starred?: string;
  albumId?: string;
  artistId?: string;
  type: 'music';
  isVideo: boolean;
  discNumber?: number;
}

export interface SubsonicIndex {
  name: string;
  artist: SubsonicArtist[];
}

export interface SubsonicMusicFolder {
  id: string;
  name: string;
}

export interface SubsonicPlaylist {
  id: string;
  name: string;
  comment?: string;
  owner?: string;
  public?: boolean;
  songCount: number;
  duration: number;
  created?: string;
  changed?: string;
  coverArt?: string;
  entry?: SubsonicSong[];
}

export interface SubsonicGenre {
  value: string;
  songCount: number;
  albumCount: number;
}

export interface SubsonicDirectory {
  id: string;
  name: string;
  parent?: string;
  starred?: string;
  child: SubsonicSong[];
}

// ─── Query Types ─────────────────────────────────────────────────────────────

export interface SubsonicQueryParams {
  u?: string;
  p?: string;
  t?: string;
  s?: string;
  v?: string;
  c?: string;
  f?: string;
}

export interface AlbumListParams {
  type: 'random' | 'newest' | 'frequent' | 'recent' | 'starred' | 'alphabeticalByName' | 'alphabeticalByArtist' | 'byYear' | 'byGenre';
  size?: number;
  offset?: number;
  fromYear?: number;
  toYear?: number;
  genre?: string;
  musicFolderId?: string;
}

export interface SearchParams {
  query: string;
  artistCount?: number;
  artistOffset?: number;
  albumCount?: number;
  albumOffset?: number;
  songCount?: number;
  songOffset?: number;
  musicFolderId?: string;
}

export interface RandomSongParams {
  size?: number;
  genre?: string;
  fromYear?: number;
  toYear?: number;
  musicFolderId?: string;
}

// ─── Library Scan Types ──────────────────────────────────────────────────────

export interface ScanResult {
  songsAdded: number;
  songsUpdated: number;
  albumsCreated: number;
  artistsCreated: number;
  errors: string[];
  duration: number;
}

export interface AudioMetadata {
  title: string | null;
  artist: string | null;
  album: string | null;
  year: number | null;
  trackNumber: number | null;
  discNumber: number | null;
  genre: string | null;
  duration: number | null;
  bitrate: number | null;
  contentType: string;
  coverArt: Buffer | null;
  coverArtMime: string | null;
}

// ─── Stats ───────────────────────────────────────────────────────────────────

export interface LibraryStats {
  artists: number;
  albums: number;
  songs: number;
  playlists: number;
  scrobbles: number;
  musicFolders: number;
  totalDurationSeconds: number;
  totalFileSizeBytes: number;
  lastScanAt: Date | null;
}

// ─── Config Types ────────────────────────────────────────────────────────────

export interface SubsonicConfig {
  port: number;
  host: string;
  musicPaths: string[];
  adminPassword: string;
  transcodeEnabled: boolean;
  maxBitrate: number;
  coverArtPath: string;
  databaseHost: string;
  databasePort: number;
  databaseName: string;
  databaseUser: string;
  databasePassword: string;
  databaseSsl: boolean;
  logLevel: string;
  sourceAccountId: string;
  security: import('@nself/plugin-utils').SecurityConfig;
}

// ─── Subsonic Error Codes ────────────────────────────────────────────────────

export const SUBSONIC_ERROR_CODES = {
  GENERIC: 0,
  MISSING_PARAMETER: 10,
  CLIENT_VERSION_MISMATCH: 20,
  SERVER_VERSION_MISMATCH: 30,
  WRONG_CREDENTIALS: 40,
  NOT_AUTHORIZED: 50,
  TRIAL_EXPIRED: 60,
  NOT_FOUND: 70,
} as const;

export type SubsonicErrorCode = typeof SUBSONIC_ERROR_CODES[keyof typeof SUBSONIC_ERROR_CODES];

// ─── Supported Audio Formats ─────────────────────────────────────────────────

export const SUPPORTED_AUDIO_EXTENSIONS = new Set([
  '.mp3', '.flac', '.ogg', '.opus', '.m4a', '.aac',
  '.wav', '.wma', '.aiff', '.aif', '.ape', '.wv',
  '.dsf', '.dff', '.mka',
]);

export const EXTENSION_CONTENT_TYPES: Record<string, string> = {
  '.mp3': 'audio/mpeg',
  '.flac': 'audio/flac',
  '.ogg': 'audio/ogg',
  '.opus': 'audio/opus',
  '.m4a': 'audio/mp4',
  '.aac': 'audio/aac',
  '.wav': 'audio/wav',
  '.wma': 'audio/x-ms-wma',
  '.aiff': 'audio/aiff',
  '.aif': 'audio/aiff',
  '.ape': 'audio/ape',
  '.wv': 'audio/wavpack',
  '.dsf': 'audio/dsf',
  '.dff': 'audio/dff',
  '.mka': 'audio/x-matroska',
};

export const SUPPORTED_IMAGE_EXTENSIONS = new Set([
  '.jpg', '.jpeg', '.png', '.webp', '.gif', '.bmp',
]);
