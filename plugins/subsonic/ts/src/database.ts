/**
 * Subsonic Plugin Database
 * Schema initialization, CRUD operations, and query methods
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  ArtistRecord,
  AlbumRecord,
  SongRecord,
  PlaylistRecord,
  PlaylistSongRecord,
  ScrobbleRecord,
  MusicFolderRecord,
  LibraryStats,
} from './types.js';

const logger = createLogger('subsonic:database');

const SCHEMA_SQL = `
CREATE TABLE IF NOT EXISTS np_sub_music_folders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
    name TEXT NOT NULL,
    path TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_sub_artists (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
    name TEXT NOT NULL,
    sort_name TEXT,
    image_url TEXT,
    album_count INTEGER DEFAULT 0,
    starred BOOLEAN DEFAULT FALSE,
    starred_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    synced_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_sub_albums (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
    artist_id UUID REFERENCES np_sub_artists(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    sort_title TEXT,
    year INTEGER,
    genre TEXT,
    cover_art_path TEXT,
    song_count INTEGER DEFAULT 0,
    duration_seconds INTEGER DEFAULT 0,
    play_count INTEGER DEFAULT 0,
    starred BOOLEAN DEFAULT FALSE,
    starred_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    synced_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_sub_songs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
    album_id UUID REFERENCES np_sub_albums(id) ON DELETE CASCADE,
    artist_id UUID REFERENCES np_sub_artists(id) ON DELETE SET NULL,
    title TEXT NOT NULL,
    track_number INTEGER,
    disc_number INTEGER DEFAULT 1,
    year INTEGER,
    genre TEXT,
    duration_seconds INTEGER,
    file_path TEXT NOT NULL,
    file_size BIGINT,
    bitrate INTEGER,
    content_type VARCHAR(50),
    cover_art_path TEXT,
    play_count INTEGER DEFAULT 0,
    last_played_at TIMESTAMPTZ,
    starred BOOLEAN DEFAULT FALSE,
    starred_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    synced_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_sub_playlists (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
    name TEXT NOT NULL,
    comment TEXT,
    owner VARCHAR(255) NOT NULL DEFAULT 'admin',
    public BOOLEAN DEFAULT FALSE,
    song_count INTEGER DEFAULT 0,
    duration_seconds INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    synced_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_sub_playlist_songs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    playlist_id UUID NOT NULL REFERENCES np_sub_playlists(id) ON DELETE CASCADE,
    song_id UUID NOT NULL REFERENCES np_sub_songs(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_sub_scrobbles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
    song_id UUID NOT NULL REFERENCES np_sub_songs(id) ON DELETE CASCADE,
    user_name VARCHAR(255) NOT NULL,
    scrobbled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    submission BOOLEAN DEFAULT TRUE
);

CREATE INDEX IF NOT EXISTS idx_np_sub_songs_album ON np_sub_songs(album_id);
CREATE INDEX IF NOT EXISTS idx_np_sub_songs_artist ON np_sub_songs(artist_id);
CREATE INDEX IF NOT EXISTS idx_np_sub_albums_artist ON np_sub_albums(artist_id);
CREATE INDEX IF NOT EXISTS idx_np_sub_artists_name ON np_sub_artists(source_account_id, sort_name);
CREATE INDEX IF NOT EXISTS idx_np_sub_songs_starred ON np_sub_songs(source_account_id, starred) WHERE starred = TRUE;
CREATE INDEX IF NOT EXISTS idx_np_sub_albums_starred ON np_sub_albums(source_account_id, starred) WHERE starred = TRUE;
CREATE INDEX IF NOT EXISTS idx_np_sub_artists_starred ON np_sub_artists(source_account_id, starred) WHERE starred = TRUE;
CREATE INDEX IF NOT EXISTS idx_np_sub_playlist_songs_pos ON np_sub_playlist_songs(playlist_id, position);
CREATE INDEX IF NOT EXISTS idx_np_sub_songs_file_path ON np_sub_songs(file_path);
CREATE INDEX IF NOT EXISTS idx_np_sub_songs_genre ON np_sub_songs(source_account_id, genre);
CREATE INDEX IF NOT EXISTS idx_np_sub_scrobbles_song ON np_sub_scrobbles(song_id);
CREATE INDEX IF NOT EXISTS idx_np_sub_albums_year ON np_sub_albums(source_account_id, year);
`;

export class SubsonicDatabase {
  private db: Database;
  private sourceAccountId: string;

  constructor(sourceAccountId = 'primary') {
    this.db = createDatabase();
    this.sourceAccountId = sourceAccountId;
  }

  async connect(): Promise<void> {
    await this.db.connect();
  }

  async disconnect(): Promise<void> {
    await this.db.disconnect();
  }

  async initializeSchema(): Promise<void> {
    await this.db.executeSqlFile(SCHEMA_SQL);
    logger.info('Database schema initialized');
  }

  forSourceAccount(sourceAccountId: string): SubsonicDatabase {
    const instance = new SubsonicDatabase(sourceAccountId);
    instance.db = this.db;
    return instance;
  }

  // ─── Music Folders ───────────────────────────────────────────────────────

  async getMusicFolders(): Promise<MusicFolderRecord[]> {
    const result = await this.db.query<MusicFolderRecord>(
      `SELECT * FROM np_sub_music_folders WHERE source_account_id = $1 ORDER BY name`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  async upsertMusicFolder(name: string, path: string): Promise<MusicFolderRecord> {
    const result = await this.db.query<MusicFolderRecord>(
      `INSERT INTO np_sub_music_folders (source_account_id, name, path)
       VALUES ($1, $2, $3)
       ON CONFLICT DO NOTHING
       RETURNING *`,
      [this.sourceAccountId, name, path]
    );
    if (result.rows[0]) return result.rows[0];

    const existing = await this.db.queryOne<MusicFolderRecord>(
      `SELECT * FROM np_sub_music_folders
       WHERE source_account_id = $1 AND path = $2`,
      [this.sourceAccountId, path]
    );
    return existing!;
  }

  async getMusicFolderById(id: string): Promise<MusicFolderRecord | null> {
    return this.db.queryOne<MusicFolderRecord>(
      `SELECT * FROM np_sub_music_folders WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  // ─── Artists ─────────────────────────────────────────────────────────────

  async getOrCreateArtist(name: string): Promise<ArtistRecord> {
    const existing = await this.db.queryOne<ArtistRecord>(
      `SELECT * FROM np_sub_artists
       WHERE source_account_id = $1 AND LOWER(name) = LOWER($2)`,
      [this.sourceAccountId, name]
    );
    if (existing) return existing;

    const sortName = name.replace(/^(the|a|an)\s+/i, '').trim();
    const result = await this.db.query<ArtistRecord>(
      `INSERT INTO np_sub_artists (source_account_id, name, sort_name)
       VALUES ($1, $2, $3)
       RETURNING *`,
      [this.sourceAccountId, name, sortName]
    );
    return result.rows[0];
  }

  async getArtistById(id: string): Promise<ArtistRecord | null> {
    return this.db.queryOne<ArtistRecord>(
      `SELECT * FROM np_sub_artists WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async listArtists(): Promise<ArtistRecord[]> {
    const result = await this.db.query<ArtistRecord>(
      `SELECT * FROM np_sub_artists
       WHERE source_account_id = $1
       ORDER BY sort_name ASC`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  async getStarredArtists(): Promise<ArtistRecord[]> {
    const result = await this.db.query<ArtistRecord>(
      `SELECT * FROM np_sub_artists
       WHERE source_account_id = $1 AND starred = TRUE
       ORDER BY sort_name ASC`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  async updateArtistAlbumCount(artistId: string): Promise<void> {
    await this.db.execute(
      `UPDATE np_sub_artists
       SET album_count = (
         SELECT COUNT(*) FROM np_sub_albums
         WHERE artist_id = $1 AND source_account_id = $2
       ), updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [artistId, this.sourceAccountId]
    );
  }

  async starArtist(id: string): Promise<void> {
    await this.db.execute(
      `UPDATE np_sub_artists SET starred = TRUE, starred_at = NOW(), updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async unstarArtist(id: string): Promise<void> {
    await this.db.execute(
      `UPDATE np_sub_artists SET starred = FALSE, starred_at = NULL, updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  // ─── Albums ──────────────────────────────────────────────────────────────

  async getOrCreateAlbum(
    title: string,
    artistId: string | null,
    year: number | null,
    genre: string | null
  ): Promise<AlbumRecord> {
    const existing = await this.db.queryOne<AlbumRecord>(
      `SELECT * FROM np_sub_albums
       WHERE source_account_id = $1 AND LOWER(title) = LOWER($2)
         AND (artist_id = $3 OR ($3 IS NULL AND artist_id IS NULL))`,
      [this.sourceAccountId, title, artistId]
    );
    if (existing) return existing;

    const sortTitle = title.replace(/^(the|a|an)\s+/i, '').trim();
    const result = await this.db.query<AlbumRecord>(
      `INSERT INTO np_sub_albums (source_account_id, artist_id, title, sort_title, year, genre)
       VALUES ($1, $2, $3, $4, $5, $6)
       RETURNING *`,
      [this.sourceAccountId, artistId, title, sortTitle, year, genre]
    );
    return result.rows[0];
  }

  async getAlbumById(id: string): Promise<AlbumRecord | null> {
    return this.db.queryOne<AlbumRecord>(
      `SELECT * FROM np_sub_albums WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async getAlbumsByArtist(artistId: string): Promise<AlbumRecord[]> {
    const result = await this.db.query<AlbumRecord>(
      `SELECT * FROM np_sub_albums
       WHERE artist_id = $1 AND source_account_id = $2
       ORDER BY year ASC NULLS LAST, sort_title ASC`,
      [artistId, this.sourceAccountId]
    );
    return result.rows;
  }

  async listAlbums(
    type: string,
    size: number,
    offset: number,
    fromYear?: number,
    toYear?: number,
    genre?: string
  ): Promise<AlbumRecord[]> {
    let orderBy: string;
    let whereExtra = '';
    const params: unknown[] = [this.sourceAccountId, size, offset];

    switch (type) {
      case 'random':
        orderBy = 'RANDOM()';
        break;
      case 'newest':
        orderBy = 'a.created_at DESC';
        break;
      case 'frequent':
        orderBy = 'a.play_count DESC';
        break;
      case 'recent':
        orderBy = 'a.updated_at DESC';
        break;
      case 'starred':
        whereExtra = ' AND a.starred = TRUE';
        orderBy = 'a.starred_at DESC';
        break;
      case 'alphabeticalByName':
        orderBy = 'a.sort_title ASC';
        break;
      case 'alphabeticalByArtist':
        orderBy = 'art.sort_name ASC, a.sort_title ASC';
        break;
      case 'byYear':
        if (fromYear !== undefined) {
          params.push(fromYear);
          whereExtra += ` AND a.year >= $${params.length}`;
        }
        if (toYear !== undefined) {
          params.push(toYear);
          whereExtra += ` AND a.year <= $${params.length}`;
        }
        orderBy = 'a.year ASC';
        break;
      case 'byGenre':
        if (genre) {
          params.push(genre);
          whereExtra += ` AND LOWER(a.genre) = LOWER($${params.length})`;
        }
        orderBy = 'a.sort_title ASC';
        break;
      default:
        orderBy = 'a.sort_title ASC';
    }

    const result = await this.db.query<AlbumRecord>(
      `SELECT a.* FROM np_sub_albums a
       LEFT JOIN np_sub_artists art ON art.id = a.artist_id
       WHERE a.source_account_id = $1${whereExtra}
       ORDER BY ${orderBy}
       LIMIT $2 OFFSET $3`,
      params
    );
    return result.rows;
  }

  async updateAlbumStats(albumId: string): Promise<void> {
    await this.db.execute(
      `UPDATE np_sub_albums
       SET song_count = sub.cnt,
           duration_seconds = sub.dur,
           updated_at = NOW()
       FROM (
         SELECT COUNT(*) as cnt, COALESCE(SUM(duration_seconds), 0) as dur
         FROM np_sub_songs
         WHERE album_id = $1 AND source_account_id = $2
       ) sub
       WHERE id = $1 AND source_account_id = $2`,
      [albumId, this.sourceAccountId]
    );
  }

  async updateAlbumCoverArt(albumId: string, coverArtPath: string): Promise<void> {
    await this.db.execute(
      `UPDATE np_sub_albums SET cover_art_path = $3, updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [albumId, this.sourceAccountId, coverArtPath]
    );
  }

  async starAlbum(id: string): Promise<void> {
    await this.db.execute(
      `UPDATE np_sub_albums SET starred = TRUE, starred_at = NOW(), updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async unstarAlbum(id: string): Promise<void> {
    await this.db.execute(
      `UPDATE np_sub_albums SET starred = FALSE, starred_at = NULL, updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async getStarredAlbums(): Promise<AlbumRecord[]> {
    const result = await this.db.query<AlbumRecord>(
      `SELECT * FROM np_sub_albums
       WHERE source_account_id = $1 AND starred = TRUE
       ORDER BY starred_at DESC`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  // ─── Songs ───────────────────────────────────────────────────────────────

  async getSongByFilePath(filePath: string): Promise<SongRecord | null> {
    return this.db.queryOne<SongRecord>(
      `SELECT * FROM np_sub_songs
       WHERE file_path = $1 AND source_account_id = $2`,
      [filePath, this.sourceAccountId]
    );
  }

  async upsertSong(song: Omit<SongRecord, 'id' | 'created_at' | 'updated_at' | 'synced_at' | 'play_count' | 'last_played_at' | 'starred' | 'starred_at'>): Promise<SongRecord> {
    const existing = await this.getSongByFilePath(song.file_path);
    if (existing) {
      await this.db.execute(
        `UPDATE np_sub_songs SET
           album_id = $3, artist_id = $4, title = $5, track_number = $6,
           disc_number = $7, year = $8, genre = $9, duration_seconds = $10,
           file_size = $11, bitrate = $12, content_type = $13, cover_art_path = $14,
           updated_at = NOW(), synced_at = NOW()
         WHERE id = $1 AND source_account_id = $2`,
        [
          existing.id, this.sourceAccountId,
          song.album_id, song.artist_id, song.title, song.track_number,
          song.disc_number, song.year, song.genre, song.duration_seconds,
          song.file_size, song.bitrate, song.content_type, song.cover_art_path,
        ]
      );
      return { ...existing, ...song, updated_at: new Date(), synced_at: new Date() };
    }

    const result = await this.db.query<SongRecord>(
      `INSERT INTO np_sub_songs (
         source_account_id, album_id, artist_id, title, track_number,
         disc_number, year, genre, duration_seconds, file_path,
         file_size, bitrate, content_type, cover_art_path
       ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
       RETURNING *`,
      [
        this.sourceAccountId, song.album_id, song.artist_id, song.title, song.track_number,
        song.disc_number, song.year, song.genre, song.duration_seconds, song.file_path,
        song.file_size, song.bitrate, song.content_type, song.cover_art_path,
      ]
    );
    return result.rows[0];
  }

  async getSongById(id: string): Promise<SongRecord | null> {
    return this.db.queryOne<SongRecord>(
      `SELECT * FROM np_sub_songs WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async getSongsByAlbum(albumId: string): Promise<SongRecord[]> {
    const result = await this.db.query<SongRecord>(
      `SELECT * FROM np_sub_songs
       WHERE album_id = $1 AND source_account_id = $2
       ORDER BY disc_number ASC, track_number ASC`,
      [albumId, this.sourceAccountId]
    );
    return result.rows;
  }

  async getRandomSongs(
    size: number,
    genre?: string,
    fromYear?: number,
    toYear?: number
  ): Promise<SongRecord[]> {
    let where = 'source_account_id = $1';
    const params: unknown[] = [this.sourceAccountId];

    if (genre) {
      params.push(genre);
      where += ` AND LOWER(genre) = LOWER($${params.length})`;
    }
    if (fromYear !== undefined) {
      params.push(fromYear);
      where += ` AND year >= $${params.length}`;
    }
    if (toYear !== undefined) {
      params.push(toYear);
      where += ` AND year <= $${params.length}`;
    }

    params.push(size);
    const result = await this.db.query<SongRecord>(
      `SELECT * FROM np_sub_songs WHERE ${where} ORDER BY RANDOM() LIMIT $${params.length}`,
      params
    );
    return result.rows;
  }

  async starSong(id: string): Promise<void> {
    await this.db.execute(
      `UPDATE np_sub_songs SET starred = TRUE, starred_at = NOW(), updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async unstarSong(id: string): Promise<void> {
    await this.db.execute(
      `UPDATE np_sub_songs SET starred = FALSE, starred_at = NULL, updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async getStarredSongs(): Promise<SongRecord[]> {
    const result = await this.db.query<SongRecord>(
      `SELECT * FROM np_sub_songs
       WHERE source_account_id = $1 AND starred = TRUE
       ORDER BY starred_at DESC`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  async incrementPlayCount(songId: string): Promise<void> {
    await this.db.execute(
      `UPDATE np_sub_songs
       SET play_count = play_count + 1, last_played_at = NOW(), updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [songId, this.sourceAccountId]
    );

    // Also update album play count
    await this.db.execute(
      `UPDATE np_sub_albums
       SET play_count = play_count + 1, updated_at = NOW()
       WHERE id = (SELECT album_id FROM np_sub_songs WHERE id = $1)
         AND source_account_id = $2`,
      [songId, this.sourceAccountId]
    );
  }

  // ─── Search ──────────────────────────────────────────────────────────────

  async searchArtists(query: string, limit: number, offset: number): Promise<ArtistRecord[]> {
    const pattern = `%${query}%`;
    const result = await this.db.query<ArtistRecord>(
      `SELECT * FROM np_sub_artists
       WHERE source_account_id = $1 AND name ILIKE $2
       ORDER BY sort_name ASC
       LIMIT $3 OFFSET $4`,
      [this.sourceAccountId, pattern, limit, offset]
    );
    return result.rows;
  }

  async searchAlbums(query: string, limit: number, offset: number): Promise<AlbumRecord[]> {
    const pattern = `%${query}%`;
    const result = await this.db.query<AlbumRecord>(
      `SELECT * FROM np_sub_albums
       WHERE source_account_id = $1 AND title ILIKE $2
       ORDER BY sort_title ASC
       LIMIT $3 OFFSET $4`,
      [this.sourceAccountId, pattern, limit, offset]
    );
    return result.rows;
  }

  async searchSongs(query: string, limit: number, offset: number): Promise<SongRecord[]> {
    const pattern = `%${query}%`;
    const result = await this.db.query<SongRecord>(
      `SELECT * FROM np_sub_songs
       WHERE source_account_id = $1 AND title ILIKE $2
       ORDER BY title ASC
       LIMIT $3 OFFSET $4`,
      [this.sourceAccountId, pattern, limit, offset]
    );
    return result.rows;
  }

  // ─── Genres ──────────────────────────────────────────────────────────────

  async getGenres(): Promise<Array<{ value: string; songCount: number; albumCount: number }>> {
    const result = await this.db.query<{ genre: string; song_count: string; album_count: string }>(
      `SELECT
         g.genre,
         COALESCE(s.cnt, 0) as song_count,
         COALESCE(a.cnt, 0) as album_count
       FROM (
         SELECT DISTINCT genre FROM np_sub_songs WHERE source_account_id = $1 AND genre IS NOT NULL
         UNION
         SELECT DISTINCT genre FROM np_sub_albums WHERE source_account_id = $1 AND genre IS NOT NULL
       ) g
       LEFT JOIN (
         SELECT genre, COUNT(*) as cnt FROM np_sub_songs WHERE source_account_id = $1 AND genre IS NOT NULL GROUP BY genre
       ) s ON LOWER(s.genre) = LOWER(g.genre)
       LEFT JOIN (
         SELECT genre, COUNT(*) as cnt FROM np_sub_albums WHERE source_account_id = $1 AND genre IS NOT NULL GROUP BY genre
       ) a ON LOWER(a.genre) = LOWER(g.genre)
       ORDER BY g.genre ASC`,
      [this.sourceAccountId]
    );
    return result.rows.map(r => ({
      value: r.genre,
      songCount: parseInt(r.song_count, 10),
      albumCount: parseInt(r.album_count, 10),
    }));
  }

  // ─── Playlists ───────────────────────────────────────────────────────────

  async getPlaylists(): Promise<PlaylistRecord[]> {
    const result = await this.db.query<PlaylistRecord>(
      `SELECT * FROM np_sub_playlists
       WHERE source_account_id = $1
       ORDER BY name ASC`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  async getPlaylistById(id: string): Promise<PlaylistRecord | null> {
    return this.db.queryOne<PlaylistRecord>(
      `SELECT * FROM np_sub_playlists WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async getPlaylistSongs(playlistId: string): Promise<SongRecord[]> {
    const result = await this.db.query<SongRecord>(
      `SELECT s.* FROM np_sub_songs s
       JOIN np_sub_playlist_songs ps ON ps.song_id = s.id
       WHERE ps.playlist_id = $1
       ORDER BY ps.position ASC`,
      [playlistId]
    );
    return result.rows;
  }

  async createPlaylist(name: string, songIds: string[], owner = 'admin'): Promise<PlaylistRecord> {
    const result = await this.db.query<PlaylistRecord>(
      `INSERT INTO np_sub_playlists (source_account_id, name, owner)
       VALUES ($1, $2, $3)
       RETURNING *`,
      [this.sourceAccountId, name, owner]
    );
    const playlist = result.rows[0];

    if (songIds.length > 0) {
      await this.addSongsToPlaylist(playlist.id, songIds);
    }

    return (await this.getPlaylistById(playlist.id))!;
  }

  async updatePlaylist(
    playlistId: string,
    updates: { name?: string; comment?: string; isPublic?: boolean; songIdsToAdd?: string[]; songIndicesToRemove?: number[] }
  ): Promise<void> {
    if (updates.name !== undefined) {
      await this.db.execute(
        `UPDATE np_sub_playlists SET name = $3, updated_at = NOW()
         WHERE id = $1 AND source_account_id = $2`,
        [playlistId, this.sourceAccountId, updates.name]
      );
    }
    if (updates.comment !== undefined) {
      await this.db.execute(
        `UPDATE np_sub_playlists SET comment = $3, updated_at = NOW()
         WHERE id = $1 AND source_account_id = $2`,
        [playlistId, this.sourceAccountId, updates.comment]
      );
    }
    if (updates.isPublic !== undefined) {
      await this.db.execute(
        `UPDATE np_sub_playlists SET public = $3, updated_at = NOW()
         WHERE id = $1 AND source_account_id = $2`,
        [playlistId, this.sourceAccountId, updates.isPublic]
      );
    }

    // Remove songs by index (do removals before additions)
    if (updates.songIndicesToRemove && updates.songIndicesToRemove.length > 0) {
      // Sort indices descending to avoid position shift issues
      const sorted = [...updates.songIndicesToRemove].sort((a, b) => b - a);
      for (const idx of sorted) {
        await this.db.execute(
          `DELETE FROM np_sub_playlist_songs
           WHERE playlist_id = $1 AND position = $2`,
          [playlistId, idx]
        );
      }
      // Re-number remaining positions
      await this.reorderPlaylistSongs(playlistId);
    }

    // Add songs
    if (updates.songIdsToAdd && updates.songIdsToAdd.length > 0) {
      await this.addSongsToPlaylist(playlistId, updates.songIdsToAdd);
    }

    // Update playlist stats
    await this.updatePlaylistStats(playlistId);
  }

  async deletePlaylist(id: string): Promise<void> {
    await this.db.execute(
      `DELETE FROM np_sub_playlists WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  private async addSongsToPlaylist(playlistId: string, songIds: string[]): Promise<void> {
    // Get current max position
    const maxPos = await this.db.queryOne<{ max: number | null }>(
      `SELECT MAX(position) as max FROM np_sub_playlist_songs WHERE playlist_id = $1`,
      [playlistId]
    );
    let position = (maxPos?.max ?? -1) + 1;

    for (const songId of songIds) {
      await this.db.execute(
        `INSERT INTO np_sub_playlist_songs (playlist_id, song_id, position)
         VALUES ($1, $2, $3)`,
        [playlistId, songId, position]
      );
      position++;
    }
  }

  private async reorderPlaylistSongs(playlistId: string): Promise<void> {
    const songs = await this.db.query<PlaylistSongRecord>(
      `SELECT * FROM np_sub_playlist_songs WHERE playlist_id = $1 ORDER BY position ASC`,
      [playlistId]
    );
    for (let i = 0; i < songs.rows.length; i++) {
      if (songs.rows[i].position !== i) {
        await this.db.execute(
          `UPDATE np_sub_playlist_songs SET position = $2 WHERE id = $1`,
          [songs.rows[i].id, i]
        );
      }
    }
  }

  private async updatePlaylistStats(playlistId: string): Promise<void> {
    await this.db.execute(
      `UPDATE np_sub_playlists
       SET song_count = sub.cnt,
           duration_seconds = sub.dur,
           updated_at = NOW()
       FROM (
         SELECT
           COUNT(*) as cnt,
           COALESCE(SUM(s.duration_seconds), 0) as dur
         FROM np_sub_playlist_songs ps
         JOIN np_sub_songs s ON s.id = ps.song_id
         WHERE ps.playlist_id = $1
       ) sub
       WHERE id = $1`,
      [playlistId]
    );
  }

  // ─── Scrobbles ───────────────────────────────────────────────────────────

  async addScrobble(songId: string, userName: string, submission: boolean, time?: Date): Promise<void> {
    await this.db.execute(
      `INSERT INTO np_sub_scrobbles (source_account_id, song_id, user_name, submission, scrobbled_at)
       VALUES ($1, $2, $3, $4, $5)`,
      [this.sourceAccountId, songId, userName, submission, time ?? new Date()]
    );
  }

  // ─── Directory Browsing ──────────────────────────────────────────────────

  async getMusicDirectoryContents(id: string): Promise<{ type: 'artist' | 'album'; record: ArtistRecord | AlbumRecord; songs: SongRecord[] }> {
    // Check if it's an artist
    const artist = await this.getArtistById(id);
    if (artist) {
      const albums = await this.getAlbumsByArtist(id);
      // Return albums as "songs" (children) in the directory response
      return { type: 'artist', record: artist, songs: albums as unknown as SongRecord[] };
    }

    // Check if it's an album
    const album = await this.getAlbumById(id);
    if (album) {
      const songs = await this.getSongsByAlbum(id);
      return { type: 'album', record: album, songs };
    }

    throw new Error('Directory not found');
  }

  // ─── Stats ───────────────────────────────────────────────────────────────

  async getStats(): Promise<LibraryStats> {
    const [artists, albums, songs, playlists, scrobbles, musicFolders, totalDuration, totalSize, lastScan] = await Promise.all([
      this.db.countScoped('np_sub_artists', this.sourceAccountId),
      this.db.countScoped('np_sub_albums', this.sourceAccountId),
      this.db.countScoped('np_sub_songs', this.sourceAccountId),
      this.db.countScoped('np_sub_playlists', this.sourceAccountId),
      this.db.countScoped('np_sub_scrobbles', this.sourceAccountId),
      this.db.countScoped('np_sub_music_folders', this.sourceAccountId),
      this.db.queryOne<{ total: string }>(
        `SELECT COALESCE(SUM(duration_seconds), 0) as total
         FROM np_sub_songs WHERE source_account_id = $1`,
        [this.sourceAccountId]
      ),
      this.db.queryOne<{ total: string }>(
        `SELECT COALESCE(SUM(file_size), 0) as total
         FROM np_sub_songs WHERE source_account_id = $1`,
        [this.sourceAccountId]
      ),
      this.db.getLastSyncTimeScoped('np_sub_songs', this.sourceAccountId),
    ]);

    return {
      artists,
      albums,
      songs,
      playlists,
      scrobbles,
      musicFolders,
      totalDurationSeconds: parseInt(totalDuration?.total ?? '0', 10),
      totalFileSizeBytes: parseInt(totalSize?.total ?? '0', 10),
      lastScanAt: lastScan,
    };
  }

  // ─── Cleanup ─────────────────────────────────────────────────────────────

  async removeOrphanedSongs(validFilePaths: Set<string>): Promise<number> {
    const allSongs = await this.db.query<{ id: string; file_path: string }>(
      `SELECT id, file_path FROM np_sub_songs WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    let removed = 0;
    for (const song of allSongs.rows) {
      if (!validFilePaths.has(song.file_path)) {
        await this.db.execute(
          `DELETE FROM np_sub_songs WHERE id = $1 AND source_account_id = $2`,
          [song.id, this.sourceAccountId]
        );
        removed++;
      }
    }

    return removed;
  }

  async removeOrphanedAlbums(): Promise<number> {
    const result = await this.db.execute(
      `DELETE FROM np_sub_albums
       WHERE source_account_id = $1
         AND id NOT IN (SELECT DISTINCT album_id FROM np_sub_songs WHERE album_id IS NOT NULL AND source_account_id = $1)`,
      [this.sourceAccountId]
    );
    return result;
  }

  async removeOrphanedArtists(): Promise<number> {
    const result = await this.db.execute(
      `DELETE FROM np_sub_artists
       WHERE source_account_id = $1
         AND id NOT IN (SELECT DISTINCT artist_id FROM np_sub_songs WHERE artist_id IS NOT NULL AND source_account_id = $1)
         AND id NOT IN (SELECT DISTINCT artist_id FROM np_sub_albums WHERE artist_id IS NOT NULL AND source_account_id = $1)`,
      [this.sourceAccountId]
    );
    return result;
  }

  /** Expose underlying database for raw queries */
  get raw(): Database {
    return this.db;
  }
}
