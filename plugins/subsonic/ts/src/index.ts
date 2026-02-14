/**
 * Subsonic Plugin for nself
 * Subsonic API server for music client compatibility
 */

export { SubsonicDatabase } from './database.js';
export { createServer } from './server.js';
export { loadConfig } from './config.js';
export { authenticate, validateApiVersion } from './auth.js';
export { scanLibrary } from './library.js';
export { streamAudio, serveCoverArt, resolveCoverArtPath } from './streaming.js';
export {
  okResponse,
  errorResponse,
  artistToSubsonic,
  albumToSubsonic,
  songToSubsonic,
  playlistToSubsonic,
  musicFolderToSubsonic,
  buildArtistIndex,
  API_VERSION,
} from './response.js';
export * from './types.js';
