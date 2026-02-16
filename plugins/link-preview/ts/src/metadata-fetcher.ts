/**
 * URL Metadata Fetcher
 * Fetches and parses metadata from URLs using Open Graph, Twitter Cards, and HTML meta tags
 */

import { load as loadHTML } from 'cheerio';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('link-preview:metadata-fetcher');

export interface UrlMetadata {
  title?: string;
  description?: string;
  image?: string;
  siteName?: string;
  author?: string;
  publishedTime?: string;
  type?: string;
  url?: string;
  favicon?: string;
  language?: string;
  videoUrl?: string;
  audioUrl?: string;
  estimatedReadTime?: number;
}

export class MetadataFetcher {
  private readonly timeout: number;
  private readonly userAgent: string;

  constructor(timeout = 10000) {
    this.timeout = timeout;
    this.userAgent = 'Mozilla/5.0 (compatible; nSelfBot/1.0; +https://nself.org)';
  }

  /**
   * Fetch and extract metadata from a URL
   */
  async fetchMetadata(url: string): Promise<UrlMetadata> {
    try {
      logger.debug('Fetching metadata for URL', { url });

      // Fetch HTML
      const html = await this.fetchHTML(url);

      // Parse metadata
      const metadata = this.parseMetadata(html, url);

      logger.info('Metadata fetched successfully', {
        url,
        hasTitle: !!metadata.title,
        hasDescription: !!metadata.description,
        hasImage: !!metadata.image,
      });

      return metadata;
    } catch (error) {
      logger.error('Failed to fetch metadata', { url, error });
      throw error;
    }
  }

  /**
   * Fetch HTML from URL
   */
  private async fetchHTML(url: string): Promise<string> {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), this.timeout);

    try {
      const response = await fetch(url, {
        signal: controller.signal,
        headers: {
          'User-Agent': this.userAgent,
          'Accept': 'text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8',
          'Accept-Language': 'en-US,en;q=0.9',
          'Cache-Control': 'no-cache',
        },
      });

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`);
      }

      const contentType = response.headers.get('content-type') || '';
      if (!contentType.includes('text/html') && !contentType.includes('application/xhtml+xml')) {
        throw new Error(`Invalid content type: ${contentType}`);
      }

      return await response.text();
    } finally {
      clearTimeout(timeoutId);
    }
  }

  /**
   * Parse metadata from HTML
   */
  private parseMetadata(html: string, url: string): UrlMetadata {
    const $ = loadHTML(html);
    const metadata: UrlMetadata = {};

    // Extract Open Graph tags (priority 1)
    const ogTitle = $('meta[property="og:title"]').attr('content');
    const ogDescription = $('meta[property="og:description"]').attr('content');
    const ogImage = $('meta[property="og:image"]').attr('content');
    const ogSiteName = $('meta[property="og:site_name"]').attr('content');
    const ogType = $('meta[property="og:type"]').attr('content');
    const ogUrl = $('meta[property="og:url"]').attr('content');
    const ogVideo = $('meta[property="og:video"]').attr('content') || $('meta[property="og:video:url"]').attr('content');
    const ogAudio = $('meta[property="og:audio"]').attr('content') || $('meta[property="og:audio:url"]').attr('content');

    // Extract Twitter Card tags (priority 2)
    const twitterTitle = $('meta[name="twitter:title"]').attr('content');
    const twitterDescription = $('meta[name="twitter:description"]').attr('content');
    const twitterImage = $('meta[name="twitter:image"]').attr('content');
    const twitterCreator = $('meta[name="twitter:creator"]').attr('content');

    // Extract standard meta tags (priority 3)
    const metaDescription = $('meta[name="description"]').attr('content');
    const metaAuthor = $('meta[name="author"]').attr('content');
    const metaPublished = $('meta[name="article:published_time"]').attr('content') ||
                          $('meta[property="article:published_time"]').attr('content');

    // Extract HTML title (fallback)
    const htmlTitle = $('title').text().trim();

    // Extract language
    const language = $('html').attr('lang') || $('meta[http-equiv="content-language"]').attr('content');

    // Extract favicon
    let favicon = $('link[rel="icon"]').attr('href') ||
                  $('link[rel="shortcut icon"]').attr('href') ||
                  $('link[rel="apple-touch-icon"]').attr('href');

    if (favicon && !favicon.startsWith('http')) {
      const base = new URL(url);
      favicon = new URL(favicon, base.origin).toString();
    }

    // Build metadata object with fallbacks
    metadata.title = ogTitle || twitterTitle || htmlTitle || undefined;
    metadata.description = ogDescription || twitterDescription || metaDescription || undefined;
    metadata.image = this.resolveUrl(ogImage || twitterImage, url);
    metadata.siteName = ogSiteName || undefined;
    metadata.author = metaAuthor || twitterCreator || undefined;
    metadata.publishedTime = metaPublished || undefined;
    metadata.type = ogType || undefined;
    metadata.url = ogUrl || url;
    metadata.favicon = favicon || undefined;
    metadata.language = language || undefined;
    metadata.videoUrl = this.resolveUrl(ogVideo, url);
    metadata.audioUrl = this.resolveUrl(ogAudio, url);

    // Estimate read time based on content
    const textContent = $('p').text();
    if (textContent) {
      const wordCount = textContent.split(/\s+/).length;
      metadata.estimatedReadTime = Math.ceil(wordCount / 200); // 200 words per minute
    }

    return metadata;
  }

  /**
   * Resolve relative URL to absolute URL
   */
  private resolveUrl(relativeUrl: string | undefined, baseUrl: string): string | undefined {
    if (!relativeUrl) return undefined;

    try {
      if (relativeUrl.startsWith('http://') || relativeUrl.startsWith('https://')) {
        return relativeUrl;
      }

      const base = new URL(baseUrl);
      return new URL(relativeUrl, base.origin).toString();
    } catch (error) {
      logger.warn('Failed to resolve URL', { relativeUrl, baseUrl, error });
      return relativeUrl;
    }
  }
}
