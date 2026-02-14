/**
 * Subsonic Response Builder
 * Builds correctly formatted Subsonic API responses in JSON format.
 * All responses are wrapped in { "subsonic-response": { ... } }
 */

import type {
  SubsonicArtist,
  SubsonicAlbum,
  SubsonicSong,
  SubsonicIndex,
  SubsonicMusicFolder,
  SubsonicPlaylist,
  SubsonicGenre,
  SubsonicDirectory,
  ArtistRecord,
  AlbumRecord,
  SongRecord,
  PlaylistRecord,
  MusicFolderRecord,
  SubsonicErrorCode,
} from './types.js';

const API_VERSION = '1.16.1';
const SERVER_TYPE = 'nself-subsonic';
const SERVER_VERSION = '1.0.0';

// ─── Response Wrapper ────────────────────────────────────────────────────────

export function okResponse(data: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    'subsonic-response': {
      status: 'ok',
      version: API_VERSION,
      type: SERVER_TYPE,
      serverVersion: SERVER_VERSION,
      openSubsonic: true,
      ...data,
    },
  };
}

export function errorResponse(code: SubsonicErrorCode, message: string): Record<string, unknown> {
  return {
    'subsonic-response': {
      status: 'failed',
      version: API_VERSION,
      type: SERVER_TYPE,
      serverVersion: SERVER_VERSION,
      openSubsonic: true,
      error: { code, message },
    },
  };
}

// ─── Record to Subsonic Type Mapping ─────────────────────────────────────────

export function artistToSubsonic(artist: ArtistRecord): SubsonicArtist {
  const result: SubsonicArtist = {
    id: artist.id,
    name: artist.name,
    albumCount: artist.album_count,
  };
  if (artist.starred && artist.starred_at) {
    result.starred = artist.starred_at.toISOString();
  }
  if (artist.image_url) {
    result.coverArt = `ar-${artist.id}`;
  }
  return result;
}

export function albumToSubsonic(album: AlbumRecord, artistName?: string): SubsonicAlbum {
  const result: SubsonicAlbum = {
    id: album.id,
    name: album.title,
    songCount: album.song_count,
    duration: album.duration_seconds,
    playCount: album.play_count,
    created: album.created_at.toISOString(),
  };
  if (album.artist_id) {
    result.artistId = album.artist_id;
  }
  if (artistName) {
    result.artist = artistName;
  }
  if (album.cover_art_path) {
    result.coverArt = `al-${album.id}`;
  }
  if (album.starred && album.starred_at) {
    result.starred = album.starred_at.toISOString();
  }
  if (album.year) {
    result.year = album.year;
  }
  if (album.genre) {
    result.genre = album.genre;
  }
  return result;
}

export function songToSubsonic(
  song: SongRecord,
  artistName?: string,
  albumTitle?: string
): SubsonicSong {
  const suffix = song.file_path.split('.').pop() ?? '';
  const result: SubsonicSong = {
    id: song.id,
    parent: song.album_id ?? undefined,
    isDir: false,
    title: song.title,
    type: 'music',
    isVideo: false,
  };
  if (albumTitle) {
    result.album = albumTitle;
  }
  if (artistName) {
    result.artist = artistName;
  }
  if (song.album_id) {
    result.albumId = song.album_id;
  }
  if (song.artist_id) {
    result.artistId = song.artist_id;
  }
  if (song.track_number !== null) {
    result.track = song.track_number;
  }
  if (song.disc_number !== null && song.disc_number !== undefined) {
    result.discNumber = song.disc_number;
  }
  if (song.year) {
    result.year = song.year;
  }
  if (song.genre) {
    result.genre = song.genre;
  }
  if (song.cover_art_path || song.album_id) {
    result.coverArt = song.cover_art_path ? `so-${song.id}` : `al-${song.album_id}`;
  }
  if (song.file_size) {
    result.size = Number(song.file_size);
  }
  if (song.content_type) {
    result.contentType = song.content_type;
  }
  if (suffix) {
    result.suffix = suffix;
  }
  if (song.duration_seconds !== null) {
    result.duration = song.duration_seconds;
  }
  if (song.bitrate) {
    result.bitRate = song.bitrate;
  }
  result.path = song.file_path;
  if (song.play_count > 0) {
    result.playCount = song.play_count;
  }
  result.created = song.created_at.toISOString();
  if (song.starred && song.starred_at) {
    result.starred = song.starred_at.toISOString();
  }
  return result;
}

export function playlistToSubsonic(playlist: PlaylistRecord): SubsonicPlaylist {
  return {
    id: playlist.id,
    name: playlist.name,
    comment: playlist.comment ?? undefined,
    owner: playlist.owner,
    public: playlist.public,
    songCount: playlist.song_count,
    duration: playlist.duration_seconds,
    created: playlist.created_at.toISOString(),
    changed: playlist.updated_at.toISOString(),
  };
}

export function musicFolderToSubsonic(folder: MusicFolderRecord): SubsonicMusicFolder {
  return {
    id: folder.id,
    name: folder.name,
  };
}

// ─── Index Building ──────────────────────────────────────────────────────────

export function buildArtistIndex(artists: ArtistRecord[]): SubsonicIndex[] {
  const indexMap = new Map<string, SubsonicArtist[]>();

  for (const artist of artists) {
    const sortKey = (artist.sort_name ?? artist.name).toUpperCase();
    const firstChar = sortKey.charAt(0);
    const letter = /^[A-Z]/.test(firstChar) ? firstChar : '#';

    if (!indexMap.has(letter)) {
      indexMap.set(letter, []);
    }
    indexMap.get(letter)!.push(artistToSubsonic(artist));
  }

  const indices: SubsonicIndex[] = [];
  const sortedKeys = Array.from(indexMap.keys()).sort((a, b) => {
    if (a === '#') return 1;
    if (b === '#') return -1;
    return a.localeCompare(b);
  });

  for (const key of sortedKeys) {
    indices.push({
      name: key,
      artist: indexMap.get(key)!,
    });
  }

  return indices;
}

// ─── Directory Building ──────────────────────────────────────────────────────

export function buildDirectory(
  id: string,
  name: string,
  children: SubsonicSong[],
  parentId?: string
): SubsonicDirectory {
  return {
    id,
    name,
    parent: parentId,
    child: children,
  };
}

// ─── Genre Building ──────────────────────────────────────────────────────────

export function buildGenre(value: string, songCount: number, albumCount: number): SubsonicGenre {
  return { value, songCount, albumCount };
}

// ─── Convenience exports ─────────────────────────────────────────────────────

export { API_VERSION, SERVER_TYPE, SERVER_VERSION };
