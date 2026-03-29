/**
 * subtitle-manager plugin — HTTP API tests
 *
 * Uses node:test + node:assert (zero external dependencies).
 * Passes stub implementations of the database and external clients
 * directly to the SubtitleManagerServer constructor — no Postgres needed.
 */

import { describe, it, before, after, mock } from 'node:test';
import assert from 'node:assert/strict';

// ---------------------------------------------------------------------------
// Mock external dependencies before importing the server module
// ---------------------------------------------------------------------------

// Mock OpenSubtitles client — avoid real network calls
await mock.module('../src/opensubtitles-client.js', {
  namedExports: {
    OpenSubtitlesClient: class {
      constructor(_apiKey: string | undefined) {}
      async searchByQuery(_query: string, _langs: string[]) { return []; }
      async searchByHash(_hash: string, _size: number, _langs: string[]) { return []; }
      async downloadSubtitle(_fileId: number): Promise<Buffer | null> { return null; }
    },
  },
});

// Mock subtitle sync — requires alass/ffsubsync binaries
await mock.module('../src/sync.js', {
  namedExports: {
    SubtitleSynchronizer: class {
      constructor(_config: unknown) {}
      async syncSubtitle(_video: string, _sub: string, _out: string) {
        return { offsetMs: 0, tool: 'none', success: true };
      }
    },
  },
});

// Mock QC — requires file system access to .srt files
await mock.module('../src/qc.js', {
  namedExports: {
    SubtitleQC: class {
      async validateSubtitle(_path: string) {
        return { status: 'ok', checks: {}, issues: [], cueCount: 0, totalDurationMs: 0 };
      }
    },
  },
});

// Mock normalizer — requires ffmpeg
await mock.module('../src/normalize.js', {
  namedExports: {
    SubtitleNormalizer: class {
      async normalizeToWebVTT(inputPath: string) {
        return inputPath.replace('.srt', '.vtt');
      }
    },
  },
});

const { SubtitleManagerServer } = await import('../src/server.js');

// ---------------------------------------------------------------------------
// Stub database
// ---------------------------------------------------------------------------

const mockDatabase = {
  connect: async () => {},
  disconnect: async () => {},
  searchSubtitles: async () => [],
  listDownloads: async () => ({ downloads: [], total: 0 }),
  getStats: async () => ({ total_downloads: 0, total_size_bytes: 0, languages: [] }),
  getDownloadByMediaId: async () => null,
  insertDownload: async (data: Record<string, unknown>) => ({ id: 'dl-1', ...data }),
  insertQCResult: async () => ({ id: 'qc-1' }),
  updateDownloadQC: async () => {},
  deleteDownload: async () => true,
};

// ---------------------------------------------------------------------------
// Test suite
// ---------------------------------------------------------------------------

describe('subtitle-manager plugin', () => {
  const TEST_PORT = 47301;
  let server: InstanceType<typeof SubtitleManagerServer>;
  const baseUrl = `http://127.0.0.1:${TEST_PORT}`;

  before(async () => {
    const config = {
      database_url: 'postgresql://unused:unused@127.0.0.1:5432/unused',
      port: TEST_PORT,
      opensubtitles_api_key: undefined,
      subtitle_storage_path: '/tmp/test-subtitles',
      log_level: 'error',
      alass_path: 'alass',
      ffsubsync_path: 'ffsubsync',
    };
    server = new SubtitleManagerServer(config as never, mockDatabase as never);
    await server.initialize();
    await server.start();
  });

  after(async () => {
    await server.stop();
  });

  it('GET /health returns 200 with status ok', async () => {
    const res = await fetch(`${baseUrl}/health`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.equal(body.status, 'ok');
    assert.equal(body.plugin, 'subtitle-manager');
  });

  it('GET /v1/subtitles without media_id returns error', async () => {
    const res = await fetch(`${baseUrl}/v1/subtitles`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    // Returns { error: 'media_id query parameter is required' } with 200 status
    assert.ok(body.error);
  });

  it('GET /v1/subtitles with media_id returns empty list', async () => {
    const res = await fetch(`${baseUrl}/v1/subtitles?media_id=movie-123`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(Array.isArray(body.subtitles));
  });

  it('POST /v1/search missing query returns 400', async () => {
    const res = await fetch(`${baseUrl}/v1/search`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({}),
    });
    assert.equal(res.status, 400);
  });

  it('GET /v1/stats returns stats object', async () => {
    const res = await fetch(`${baseUrl}/v1/stats`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.ok('stats' in body);
  });
});
