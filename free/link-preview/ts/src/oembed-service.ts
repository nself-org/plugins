/**
 * oEmbed Service
 * Handles oEmbed provider discovery and API calls for rich embeds
 */

import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('link-preview:oembed');

export interface OEmbedResponse {
  type: 'photo' | 'video' | 'link' | 'rich';
  version: string;
  title?: string;
  author_name?: string;
  author_url?: string;
  provider_name?: string;
  provider_url?: string;
  cache_age?: number;
  thumbnail_url?: string;
  thumbnail_width?: number;
  thumbnail_height?: number;

  // Photo type
  url?: string;
  width?: number;
  height?: number;

  // Video/Rich type
  html?: string;
}

export interface OEmbedProvider {
  name: string;
  url: string;
  endpoints: {
    schemes?: string[];
    url: string;
    discovery?: boolean;
    formats?: string[];
  }[];
}

export class OEmbedService {
  private readonly timeout: number;
  private readonly providers: Map<string, OEmbedProvider>;

  constructor(timeout = 10000) {
    this.timeout = timeout;
    this.providers = new Map();
    this.initializeProviders();
  }

  /**
   * Initialize known oEmbed providers
   */
  private initializeProviders(): void {
    const knownProviders: OEmbedProvider[] = [
      {
        name: 'YouTube',
        url: 'https://www.youtube.com',
        endpoints: [{
          schemes: [
            'https://*.youtube.com/watch*',
            'https://*.youtube.com/v/*',
            'https://youtu.be/*',
            'https://*.youtube.com/shorts/*',
          ],
          url: 'https://www.youtube.com/oembed',
          discovery: true,
          formats: ['json'],
        }],
      },
      {
        name: 'Vimeo',
        url: 'https://vimeo.com',
        endpoints: [{
          schemes: [
            'https://vimeo.com/*',
            'https://vimeo.com/groups/*/videos/*',
            'https://vimeo.com/album/*/video/*',
            'https://vimeo.com/channels/*/*',
          ],
          url: 'https://vimeo.com/api/oembed.json',
          discovery: true,
          formats: ['json'],
        }],
      },
      {
        name: 'Twitter',
        url: 'https://twitter.com',
        endpoints: [{
          schemes: [
            'https://twitter.com/*/status/*',
            'https://twitter.com/*/status/*?s=*',
            'https://x.com/*/status/*',
            'https://x.com/*/status/*?s=*',
          ],
          url: 'https://publish.twitter.com/oembed',
          formats: ['json'],
        }],
      },
      {
        name: 'Instagram',
        url: 'https://instagram.com',
        endpoints: [{
          schemes: [
            'http://instagram.com/*/p/*',
            'http://www.instagram.com/*/p/*',
            'https://instagram.com/*/p/*',
            'https://www.instagram.com/*/p/*',
            'http://instagram.com/p/*',
            'http://www.instagram.com/p/*',
            'https://instagram.com/p/*',
            'https://www.instagram.com/p/*',
            'http://instagram.com/reel/*',
            'http://www.instagram.com/reel/*',
            'https://instagram.com/reel/*',
            'https://www.instagram.com/reel/*',
          ],
          url: 'https://graph.facebook.com/v16.0/instagram_oembed',
          formats: ['json'],
        }],
      },
      {
        name: 'Spotify',
        url: 'https://spotify.com',
        endpoints: [{
          schemes: [
            'https://open.spotify.com/track/*',
            'https://open.spotify.com/album/*',
            'https://open.spotify.com/playlist/*',
            'https://open.spotify.com/episode/*',
            'https://open.spotify.com/show/*',
          ],
          url: 'https://open.spotify.com/oembed',
          formats: ['json'],
        }],
      },
      {
        name: 'SoundCloud',
        url: 'https://soundcloud.com',
        endpoints: [{
          schemes: [
            'http://soundcloud.com/*',
            'https://soundcloud.com/*',
            'https://soundcloud.app.goo.gl/*',
          ],
          url: 'https://soundcloud.com/oembed',
          formats: ['json'],
        }],
      },
      {
        name: 'TikTok',
        url: 'https://www.tiktok.com',
        endpoints: [{
          schemes: [
            'https://www.tiktok.com/*/video/*',
            'https://www.tiktok.com/@*/video/*',
          ],
          url: 'https://www.tiktok.com/oembed',
          formats: ['json'],
        }],
      },
    ];

    for (const provider of knownProviders) {
      this.providers.set(provider.name.toLowerCase(), provider);
    }

    logger.info('oEmbed providers initialized', { count: this.providers.size });
  }

  /**
   * Find oEmbed provider for a URL
   */
  findProvider(url: string): { provider: OEmbedProvider; endpoint: string } | null {
    for (const provider of this.providers.values()) {
      for (const endpoint of provider.endpoints) {
        if (endpoint.schemes) {
          for (const scheme of endpoint.schemes) {
            if (this.matchesScheme(url, scheme)) {
              return { provider, endpoint: endpoint.url };
            }
          }
        }
      }
    }

    return null;
  }

  /**
   * Match URL against oEmbed scheme pattern
   */
  private matchesScheme(url: string, scheme: string): boolean {
    // Convert scheme pattern to regex
    const pattern = scheme
      .replace(/\./g, '\\.')
      .replace(/\*/g, '.*')
      .replace(/\?/g, '\\?');

    const regex = new RegExp(`^${pattern}$`, 'i');
    return regex.test(url);
  }

  /**
   * Fetch oEmbed data from provider
   */
  async fetchEmbed(url: string, maxWidth?: number, maxHeight?: number): Promise<OEmbedResponse | null> {
    try {
      const match = this.findProvider(url);
      if (!match) {
        logger.debug('No oEmbed provider found for URL', { url });
        return null;
      }

      const { provider, endpoint } = match;
      logger.debug('Found oEmbed provider', { url, provider: provider.name, endpoint });

      // Build oEmbed request URL
      const oembedUrl = new URL(endpoint);
      oembedUrl.searchParams.set('url', url);
      oembedUrl.searchParams.set('format', 'json');

      if (maxWidth) {
        oembedUrl.searchParams.set('maxwidth', maxWidth.toString());
      }
      if (maxHeight) {
        oembedUrl.searchParams.set('maxheight', maxHeight.toString());
      }

      // Fetch oEmbed data
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), this.timeout);

      try {
        const response = await fetch(oembedUrl.toString(), {
          signal: controller.signal,
          headers: {
            'Accept': 'application/json',
            'User-Agent': 'Mozilla/5.0 (compatible; nSelfBot/1.0; +https://nself.org)',
          },
        });

        if (!response.ok) {
          throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }

        const data = await response.json() as OEmbedResponse;

        logger.info('oEmbed data fetched successfully', {
          url,
          provider: provider.name,
          type: data.type,
          hasHtml: !!data.html,
        });

        return data;
      } finally {
        clearTimeout(timeoutId);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to fetch oEmbed data', { url, error: message });
      return null;
    }
  }

  /**
   * Get list of supported providers
   */
  getSupportedProviders(): string[] {
    return Array.from(this.providers.values()).map(p => p.name);
  }

  /**
   * Check if URL is supported by any provider
   */
  isSupported(url: string): boolean {
    return this.findProvider(url) !== null;
  }
}
