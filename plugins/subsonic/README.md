# Subsonic Plugin

Subsonic API server for music client compatibility. Provides a full Subsonic REST API implementation that allows popular music clients (Symfonium, Ultrasonic, play:Sub, Amperfy, DSub) to browse, search, stream, and manage your music library.

## Quick Start

```bash
# Install dependencies
cd plugins/subsonic/ts
pnpm install

# Build
pnpm run build

# Initialize database
pnpm run start -- init

# Scan your music library
pnpm run start -- scan

# Start the server
pnpm run start -- server
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `SUBSONIC_PORT` | `3024` | Server port |
| `SUBSONIC_MUSIC_PATHS` | `/media/music` | Comma-separated music directory paths |
| `SUBSONIC_ADMIN_PASSWORD` | `admin` | Password for Subsonic API authentication |
| `SUBSONIC_TRANSCODE_ENABLED` | `true` | Enable audio transcoding via ffmpeg |
| `SUBSONIC_MAX_BITRATE` | `320` | Maximum bitrate (kbps) for transcoding |
| `SUBSONIC_COVER_ART_PATH` | `/data/subsonic/covers` | Directory for extracted cover art |
| `DATABASE_URL` | - | PostgreSQL connection string |
| `POSTGRES_HOST` | `localhost` | Database host |
| `POSTGRES_PORT` | `5432` | Database port |
| `POSTGRES_DB` | `nself` | Database name |
| `POSTGRES_USER` | `postgres` | Database user |
| `POSTGRES_PASSWORD` | - | Database password |

## Client Setup

Point your Subsonic-compatible client to:

- **Server URL**: `http://your-server:3024`
- **Username**: Any value (e.g., `admin`)
- **Password**: Value of `SUBSONIC_ADMIN_PASSWORD`

### Tested Clients

- Symfonium (Android)
- Ultrasonic (Android)
- play:Sub (iOS)
- Amperfy (iOS)
- DSub (Android)
- Clementine (Desktop)
- Strawberry (Desktop)

## CLI Commands

```bash
nself-subsonic server           # Start the Subsonic API server
nself-subsonic init             # Initialize database schema
nself-subsonic scan             # Scan music library and index files
nself-subsonic status           # Show library statistics
nself-subsonic artists          # List artists
nself-subsonic albums           # List albums
nself-subsonic playlists        # Manage playlists
nself-subsonic genres           # List genres
```

## REST API Endpoints

### System

| Endpoint | Description |
|----------|-------------|
| `GET /rest/ping.view` | Server alive check |
| `GET /rest/getLicense.view` | License status |
| `GET /health` | Health check (non-Subsonic) |

### Browsing

| Endpoint | Description |
|----------|-------------|
| `GET /rest/getMusicFolders.view` | List music libraries |
| `GET /rest/getIndexes.view` | Artist index (A-Z grouped) |
| `GET /rest/getArtists.view` | All artists grouped |
| `GET /rest/getArtist.view?id=` | Artist detail with albums |
| `GET /rest/getMusicDirectory.view?id=` | Browse directory contents |
| `GET /rest/getAlbumList2.view?type=` | Album lists (random, newest, etc.) |
| `GET /rest/getAlbum.view?id=` | Album detail with songs |
| `GET /rest/getSong.view?id=` | Single song detail |
| `GET /rest/getGenres.view` | List all genres |

### Search

| Endpoint | Description |
|----------|-------------|
| `GET /rest/search3.view?query=` | Search artists, albums, and songs |

### Playback

| Endpoint | Description |
|----------|-------------|
| `GET /rest/stream.view?id=` | Stream audio file |
| `GET /rest/getCoverArt.view?id=` | Get cover art image |
| `GET /rest/scrobble.view?id=` | Record a play |
| `GET /rest/getRandomSongs.view` | Get random songs |

### Favorites

| Endpoint | Description |
|----------|-------------|
| `GET /rest/star.view` | Star an item |
| `GET /rest/unstar.view` | Unstar an item |
| `GET /rest/getStarred2.view` | List all starred items |

### Playlists

| Endpoint | Description |
|----------|-------------|
| `GET /rest/getPlaylists.view` | List all playlists |
| `GET /rest/getPlaylist.view?id=` | Get playlist with songs |
| `GET /rest/createPlaylist.view?name=` | Create a playlist |
| `GET /rest/updatePlaylist.view?playlistId=` | Update a playlist |
| `GET /rest/deletePlaylist.view?id=` | Delete a playlist |

## Database Schema

### Tables

| Table | Description |
|-------|-------------|
| `np_sub_artists` | Artist records |
| `np_sub_albums` | Album records |
| `np_sub_songs` | Song/track records with file paths |
| `np_sub_playlists` | Playlist metadata |
| `np_sub_playlist_songs` | Playlist-song associations |
| `np_sub_scrobbles` | Play history |
| `np_sub_music_folders` | Configured music directories |

All tables use `source_account_id` for multi-app isolation.

## Audio Streaming

- Direct file streaming with range request support for seeking
- Optional transcoding via ffmpeg to reduce bitrate
- Supports MP3, FLAC, OGG, Opus, M4A, AAC, WAV, WMA, AIFF, APE, WavPack, DSF, DFF

## Cover Art

- Automatically extracts embedded cover art during library scan
- Falls back to folder images (cover.jpg, folder.png, etc.)
- Serves resized images when `size` parameter is provided (requires ffmpeg)

## Authentication

Supports all Subsonic authentication methods:

- **Plaintext**: `p=mypassword`
- **Hex-encoded**: `p=enc:6d7970617373776f7264`
- **Token+Salt** (API 1.13.0+): `t=md5(password+salt)&s=salt`
