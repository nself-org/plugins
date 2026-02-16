/**
 * IGDB API Client
 * Rate-limited client for the IGDB (Internet Game Database) API via Twitch OAuth2.
 * Enforces a maximum of 4 requests per second using a token bucket algorithm.
 */

import axios, { type AxiosInstance } from 'axios';
import { createLogger } from '@nself/plugin-utils';
import type { IgdbAuthResponse, IgdbGame } from './types.js';

const logger = createLogger('retro-gaming:igdb');

// Platform name mapping: our internal names -> IGDB platform IDs
const PLATFORM_TO_IGDB: Record<string, number[]> = {
  nes: [18],       // Nintendo Entertainment System
  snes: [19],      // Super Nintendo Entertainment System
  gb: [33],        // Game Boy
  gbc: [22],       // Game Boy Color
  gba: [24],       // Game Boy Advance
  genesis: [29],   // Sega Mega Drive/Genesis
  n64: [4],        // Nintendo 64
  ps1: [7],        // PlayStation
  arcade: [52],    // Arcade
  sms: [64],       // Sega Master System
  gamegear: [35],  // Sega Game Gear
  nds: [20],       // Nintendo DS
  psp: [38],       // PlayStation Portable
  atari2600: [59], // Atari 2600
  tg16: [86],      // TurboGrafx-16
};

/**
 * Token bucket rate limiter.
 * Allows up to `maxTokens` requests per second, refilling at `refillRate` tokens/sec.
 * Callers call `acquire()` which resolves when a token is available.
 */
class TokenBucketRateLimiter {
  private tokens: number;
  private readonly maxTokens: number;
  private readonly refillRate: number; // tokens per millisecond
  private lastRefill: number;
  private waitQueue: Array<() => void> = [];
  private drainTimer: ReturnType<typeof setTimeout> | null = null;

  constructor(maxTokensPerSecond: number) {
    this.maxTokens = maxTokensPerSecond;
    this.tokens = maxTokensPerSecond;
    this.refillRate = maxTokensPerSecond / 1000; // per ms
    this.lastRefill = Date.now();
  }

  private refill(): void {
    const now = Date.now();
    const elapsed = now - this.lastRefill;
    const tokensToAdd = elapsed * this.refillRate;
    this.tokens = Math.min(this.maxTokens, this.tokens + tokensToAdd);
    this.lastRefill = now;
  }

  async acquire(): Promise<void> {
    this.refill();

    if (this.tokens >= 1) {
      this.tokens -= 1;
      return;
    }

    // Wait until a token is available
    return new Promise<void>((resolve) => {
      this.waitQueue.push(resolve);
      this.scheduleDrain();
    });
  }

  private scheduleDrain(): void {
    if (this.drainTimer !== null) return;

    // Compute how long until the next token is available
    const msUntilToken = Math.ceil((1 - this.tokens) / this.refillRate);

    this.drainTimer = setTimeout(() => {
      this.drainTimer = null;
      this.refill();

      while (this.waitQueue.length > 0 && this.tokens >= 1) {
        this.tokens -= 1;
        const next = this.waitQueue.shift();
        if (next) next();
      }

      // If there are still waiters, schedule again
      if (this.waitQueue.length > 0) {
        this.scheduleDrain();
      }
    }, msUntilToken);
  }

  destroy(): void {
    if (this.drainTimer !== null) {
      clearTimeout(this.drainTimer);
      this.drainTimer = null;
    }
    // Resolve any pending waiters so they don't hang forever
    for (const resolve of this.waitQueue) {
      resolve();
    }
    this.waitQueue = [];
  }
}

export class IgdbClient {
  private clientId: string;
  private clientSecret: string;
  private apiUrl: string;
  private oauthUrl: string;
  private accessToken: string | null = null;
  private tokenExpiresAt: number = 0;
  private http: AxiosInstance;
  private rateLimiter: TokenBucketRateLimiter;

  constructor(
    clientId: string,
    clientSecret: string,
    apiUrl: string = 'https://api.igdb.com/v4',
    oauthUrl: string = 'https://id.twitch.tv/oauth2/token'
  ) {
    this.clientId = clientId;
    this.clientSecret = clientSecret;
    this.apiUrl = apiUrl;
    this.oauthUrl = oauthUrl;

    this.http = axios.create({
      baseURL: this.apiUrl,
      timeout: 15000,
    });

    // IGDB allows 4 requests per second
    this.rateLimiter = new TokenBucketRateLimiter(4);
  }

  /**
   * Check if the client has credentials configured
   */
  isConfigured(): boolean {
    return this.clientId.length > 0 && this.clientSecret.length > 0;
  }

  /**
   * Authenticate with the Twitch OAuth2 endpoint to get an IGDB access token
   */
  async authenticate(): Promise<void> {
    if (!this.isConfigured()) {
      throw new Error('IGDB client ID and secret are required. Set IGDB_CLIENT_ID and IGDB_CLIENT_SECRET.');
    }

    // If we have a valid token, skip
    if (this.accessToken && Date.now() < this.tokenExpiresAt - 60000) {
      return;
    }

    logger.info('Authenticating with Twitch/IGDB...');

    const response = await axios.post<IgdbAuthResponse>(
      this.oauthUrl,
      null,
      {
        params: {
          client_id: this.clientId,
          client_secret: this.clientSecret,
          grant_type: 'client_credentials',
        },
      }
    );

    this.accessToken = response.data.access_token;
    this.tokenExpiresAt = Date.now() + (response.data.expires_in * 1000);

    logger.info('IGDB authentication successful', {
      expiresIn: response.data.expires_in,
    });
  }

  /**
   * Make a rate-limited request to the IGDB API
   */
  private async request<T>(endpoint: string, body: string): Promise<T[]> {
    await this.authenticate();
    await this.rateLimiter.acquire();

    const response = await this.http.post<T[]>(endpoint, body, {
      headers: {
        'Client-ID': this.clientId,
        'Authorization': `Bearer ${this.accessToken}`,
        'Content-Type': 'text/plain',
      },
    });

    return response.data;
  }

  /**
   * Search for games by title and optionally platform.
   * Uses IGDB's search endpoint with fuzzy matching.
   */
  async searchGames(title: string, platform?: string): Promise<IgdbGame[]> {
    if (!this.isConfigured()) {
      logger.warn('IGDB not configured, skipping search');
      return [];
    }

    let query = `search "${title.replace(/"/g, '\\"')}"; fields name,summary,first_release_date,cover.url,screenshots.url,genres.name,involved_companies.company.name,involved_companies.publisher,involved_companies.developer,platforms.name,platforms.abbreviation; limit 10;`;

    if (platform) {
      const igdbPlatformIds = PLATFORM_TO_IGDB[platform.toLowerCase()];
      if (igdbPlatformIds && igdbPlatformIds.length > 0) {
        query = `search "${title.replace(/"/g, '\\"')}"; fields name,summary,first_release_date,cover.url,screenshots.url,genres.name,involved_companies.company.name,involved_companies.publisher,involved_companies.developer,platforms.name,platforms.abbreviation; where platforms = (${igdbPlatformIds.join(',')}); limit 10;`;
      }
    }

    try {
      const results = await this.request<IgdbGame>('/games', query);
      logger.debug(`IGDB search for "${title}": ${results.length} results`);
      return results;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('IGDB search failed', { error: message, title });
      return [];
    }
  }

  /**
   * Get full game details by IGDB ID
   */
  async getGameDetails(igdbId: number): Promise<IgdbGame | null> {
    if (!this.isConfigured()) {
      logger.warn('IGDB not configured, skipping lookup');
      return null;
    }

    const query = `fields name,summary,storyline,first_release_date,cover.url,screenshots.url,genres.name,involved_companies.company.name,involved_companies.publisher,involved_companies.developer,platforms.name,platforms.abbreviation; where id = ${igdbId};`;

    try {
      const results = await this.request<IgdbGame>('/games', query);
      return results[0] ?? null;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('IGDB game details fetch failed', { error: message, igdbId });
      return null;
    }
  }

  /**
   * Destroy the client and clean up resources
   */
  destroy(): void {
    this.rateLimiter.destroy();
  }
}
