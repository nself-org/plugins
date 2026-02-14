/**
 * Media Scanner - MeiliSearch Integration
 * Index media items and search via MeiliSearch
 */

import { MeiliSearch } from 'meilisearch';
import { createLogger } from '@nself/plugin-utils';
import type { IndexRequest, SearchQuery, SearchResult } from './types.js';

const logger = createLogger('media-scanner:search');

const INDEX_NAME = 'np_mscan_media';

export class MediaSearchService {
  private client: MeiliSearch;
  private initialized = false;

  constructor(url: string, apiKey: string) {
    this.client = new MeiliSearch({
      host: url,
      apiKey: apiKey || undefined,
    });
  }

  /**
   * Initialize the MeiliSearch index with proper settings.
   */
  async initialize(): Promise<void> {
    if (this.initialized) return;

    try {
      // Create or get the index
      await this.client.createIndex(INDEX_NAME, { primaryKey: 'id' });
      logger.debug('Index created or already exists', { index: INDEX_NAME });
    } catch (error) {
      // Index may already exist, which is fine
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.debug('Index creation note', { message });
    }

    const index = this.client.index(INDEX_NAME);

    // Configure searchable attributes
    await index.updateSearchableAttributes([
      'title',
      'description',
      'cast',
      'genre',
    ]);

    // Configure filterable attributes
    await index.updateFilterableAttributes([
      'type',
      'genre',
      'year',
      'rating',
    ]);

    // Configure sortable attributes
    await index.updateSortableAttributes([
      'title',
      'year',
      'rating',
      'added_at',
    ]);

    // Configure typo tolerance
    await index.updateTypoTolerance({
      enabled: true,
      minWordSizeForTypos: {
        oneTypo: 4,
        twoTypos: 8,
      },
    });

    // Configure synonyms
    await index.updateSynonyms({
      'tv': ['television', 'series', 'show'],
      'movie': ['film', 'motion picture'],
      'sci-fi': ['science fiction', 'scifi'],
      'romcom': ['romantic comedy'],
      'doc': ['documentary'],
      'anime': ['animation', 'animated'],
    });

    this.initialized = true;
    logger.info('MeiliSearch index configured', { index: INDEX_NAME });
  }

  /**
   * Index a single media item.
   */
  async indexItem(item: IndexRequest): Promise<void> {
    await this.ensureInitialized();
    const index = this.client.index(INDEX_NAME);

    const document = {
      id: item.id,
      title: item.title,
      type: item.type,
      genre: item.genre ?? [],
      year: item.year ?? null,
      rating: item.rating ?? null,
      description: item.description ?? '',
      cast: item.cast ?? [],
      poster_path: item.poster_path ?? null,
      backdrop_path: item.backdrop_path ?? null,
      file_path: item.file_path ?? null,
      duration_seconds: item.duration_seconds ?? null,
      resolution: item.resolution ?? null,
      codec: item.codec ?? null,
      added_at: new Date().toISOString(),
    };

    const task = await index.addDocuments([document]);
    logger.debug('Document indexed', { id: item.id, taskUid: task.taskUid });
  }

  /**
   * Index multiple media items in bulk.
   */
  async indexBulk(items: IndexRequest[]): Promise<number> {
    if (items.length === 0) return 0;
    await this.ensureInitialized();
    const index = this.client.index(INDEX_NAME);

    const documents = items.map(item => ({
      id: item.id,
      title: item.title,
      type: item.type,
      genre: item.genre ?? [],
      year: item.year ?? null,
      rating: item.rating ?? null,
      description: item.description ?? '',
      cast: item.cast ?? [],
      poster_path: item.poster_path ?? null,
      backdrop_path: item.backdrop_path ?? null,
      file_path: item.file_path ?? null,
      duration_seconds: item.duration_seconds ?? null,
      resolution: item.resolution ?? null,
      codec: item.codec ?? null,
      added_at: new Date().toISOString(),
    }));

    const task = await index.addDocuments(documents);
    logger.debug('Bulk indexed', { count: items.length, taskUid: task.taskUid });
    return items.length;
  }

  /**
   * Search for media items.
   */
  async search(query: SearchQuery): Promise<SearchResult[]> {
    await this.ensureInitialized();
    const index = this.client.index(INDEX_NAME);

    // Build filter array
    const filters: string[] = [];
    if (query.type) {
      filters.push(`type = "${query.type}"`);
    }
    if (query.genre) {
      filters.push(`genre = "${query.genre}"`);
    }
    if (query.year) {
      filters.push(`year = ${query.year}`);
    }

    const result = await index.search(query.q, {
      limit: query.limit ?? 20,
      offset: query.offset ?? 0,
      filter: filters.length > 0 ? filters : undefined,
      sort: ['_relevancy:desc'],
      attributesToRetrieve: [
        'id',
        'title',
        'type',
        'year',
        'rating',
        'genre',
        'description',
        'poster_path',
      ],
    });

    return result.hits.map(hit => ({
      id: String(hit.id),
      title: String(hit.title),
      type: String(hit.type),
      year: hit.year as number | null,
      rating: hit.rating as number | null,
      genre: hit.genre as string[] | undefined,
      description: hit.description as string | undefined,
      poster_path: hit.poster_path as string | undefined,
    }));
  }

  /**
   * Delete a document from the index.
   */
  async deleteItem(id: string): Promise<void> {
    await this.ensureInitialized();
    const index = this.client.index(INDEX_NAME);
    await index.deleteDocument(id);
    logger.debug('Document deleted', { id });
  }

  /**
   * Get index statistics.
   */
  async getStats(): Promise<{ numberOfDocuments: number; isIndexing: boolean }> {
    await this.ensureInitialized();
    const index = this.client.index(INDEX_NAME);
    const stats = await index.getStats();
    return {
      numberOfDocuments: stats.numberOfDocuments,
      isIndexing: stats.isIndexing,
    };
  }

  /**
   * Check if MeiliSearch is reachable.
   */
  async healthCheck(): Promise<boolean> {
    try {
      const health = await this.client.health();
      return health.status === 'available';
    } catch {
      return false;
    }
  }

  private async ensureInitialized(): Promise<void> {
    if (!this.initialized) {
      await this.initialize();
    }
  }
}
