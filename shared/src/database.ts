/**
 * Database utilities for nself plugins
 */

import pg from 'pg';
import type { DatabaseConfig } from './types.js';
import { createLogger } from './logger.js';

const { Pool } = pg;
const logger = createLogger('database');

export class Database {
  private pool: pg.Pool;
  private config: DatabaseConfig;
  private connected = false;

  constructor(config: DatabaseConfig) {
    this.config = config;
    this.pool = new Pool({
      host: config.host,
      port: config.port,
      database: config.database,
      user: config.user,
      password: config.password,
      ssl: config.ssl ? { rejectUnauthorized: false } : undefined,
      max: config.maxConnections ?? 10,
      idleTimeoutMillis: 30000,
      connectionTimeoutMillis: 5000,
    });

    this.pool.on('error', (err) => {
      logger.error('Unexpected database pool error', { error: err.message });
    });
  }

  async connect(): Promise<void> {
    if (this.connected) return;

    try {
      const client = await this.pool.connect();
      client.release();
      this.connected = true;
      logger.info('Database connected', {
        host: this.config.host,
        database: this.config.database,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to connect to database', { error: message });
      throw error;
    }
  }

  async disconnect(): Promise<void> {
    await this.pool.end();
    this.connected = false;
    logger.info('Database disconnected');
  }

  async query<T extends pg.QueryResultRow = pg.QueryResultRow>(
    text: string,
    params?: unknown[]
  ): Promise<pg.QueryResult<T>> {
    const start = Date.now();
    try {
      const result = await this.pool.query<T>(text, params);
      const duration = Date.now() - start;
      logger.debug('Query executed', { duration, rows: result.rowCount });
      return result;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Query failed', { error: message, query: text.substring(0, 100) });
      throw error;
    }
  }

  async queryOne<T extends pg.QueryResultRow = pg.QueryResultRow>(
    text: string,
    params?: unknown[]
  ): Promise<T | null> {
    const result = await this.query<T>(text, params);
    return result.rows[0] ?? null;
  }

  async execute(text: string, params?: unknown[]): Promise<number> {
    const result = await this.query(text, params);
    return result.rowCount ?? 0;
  }

  async transaction<T>(
    callback: (client: pg.PoolClient) => Promise<T>
  ): Promise<T> {
    const client = await this.pool.connect();
    try {
      await client.query('BEGIN');
      const result = await callback(client);
      await client.query('COMMIT');
      return result;
    } catch (error) {
      await client.query('ROLLBACK');
      throw error;
    } finally {
      client.release();
    }
  }

  async tableExists(tableName: string): Promise<boolean> {
    const result = await this.queryOne<{ exists: boolean }>(
      `SELECT EXISTS (
        SELECT FROM information_schema.tables
        WHERE table_schema = 'public'
        AND table_name = $1
      )`,
      [tableName]
    );
    return result?.exists ?? false;
  }

  async executeSqlFile(sql: string): Promise<void> {
    await this.query(sql);
    logger.info('SQL file executed successfully');
  }

  async upsert<T extends Record<string, unknown>>(
    table: string,
    data: T,
    conflictColumns: string[],
    updateColumns?: string[]
  ): Promise<void> {
    const columns = Object.keys(data);
    const values = Object.values(data);
    const placeholders = columns.map((_, i) => `$${i + 1}`).join(', ');

    const updates = (updateColumns ?? columns.filter(c => !conflictColumns.includes(c)))
      .map(col => `${col} = EXCLUDED.${col}`)
      .join(', ');

    const sql = `
      INSERT INTO ${table} (${columns.join(', ')})
      VALUES (${placeholders})
      ON CONFLICT (${conflictColumns.join(', ')})
      DO UPDATE SET ${updates}, synced_at = NOW()
    `;

    await this.execute(sql, values);
  }

  async bulkUpsert<T extends Record<string, unknown>>(
    table: string,
    records: T[],
    conflictColumns: string[],
    updateColumns?: string[]
  ): Promise<number> {
    if (records.length === 0) return 0;

    const columns = Object.keys(records[0]);
    const updates = (updateColumns ?? columns.filter(c => !conflictColumns.includes(c)))
      .map(col => `${col} = EXCLUDED.${col}`)
      .join(', ');

    let paramIndex = 1;
    const valueGroups: string[] = [];
    const allValues: unknown[] = [];

    for (const record of records) {
      const placeholders = columns.map(() => `$${paramIndex++}`).join(', ');
      valueGroups.push(`(${placeholders})`);
      allValues.push(...Object.values(record));
    }

    const sql = `
      INSERT INTO ${table} (${columns.join(', ')})
      VALUES ${valueGroups.join(', ')}
      ON CONFLICT (${conflictColumns.join(', ')})
      DO UPDATE SET ${updates}, synced_at = NOW()
    `;

    return await this.execute(sql, allValues);
  }

  async getLastSyncTime(table: string): Promise<Date | null> {
    const result = await this.queryOne<{ max: Date | null }>(
      `SELECT MAX(synced_at) as max FROM ${table}`
    );
    return result?.max ?? null;
  }

  async count(table: string, where?: string, params?: unknown[]): Promise<number> {
    const sql = `SELECT COUNT(*) as count FROM ${table}${where ? ` WHERE ${where}` : ''}`;
    const result = await this.queryOne<{ count: string }>(sql, params);
    return parseInt(result?.count ?? '0', 10);
  }

  /**
   * Count rows scoped to a specific source_account_id.
   * Optionally add extra WHERE conditions.
   */
  async countScoped(
    table: string,
    sourceAccountId: string,
    where?: string,
    params?: unknown[]
  ): Promise<number> {
    const baseParams = params ? [...params] : [];
    const accountParamIndex = baseParams.length + 1;
    const accountFilter = `source_account_id = $${accountParamIndex}`;
    const fullWhere = where ? `${where} AND ${accountFilter}` : accountFilter;
    const sql = `SELECT COUNT(*) as count FROM ${table} WHERE ${fullWhere}`;
    const result = await this.queryOne<{ count: string }>(sql, [...baseParams, sourceAccountId]);
    return parseInt(result?.count ?? '0', 10);
  }

  /**
   * Get the last sync time for a table scoped to a specific source_account_id.
   */
  async getLastSyncTimeScoped(table: string, sourceAccountId: string): Promise<Date | null> {
    const result = await this.queryOne<{ max: Date | null }>(
      `SELECT MAX(synced_at) as max FROM ${table} WHERE source_account_id = $1`,
      [sourceAccountId]
    );
    return result?.max ?? null;
  }

  /**
   * Delete all rows for a given source_account_id across multiple tables.
   * Tables should be ordered so that child tables come before parent tables.
   * Returns total number of deleted rows.
   */
  async cleanupForAccount(tables: string[], sourceAccountId: string): Promise<number> {
    let total = 0;
    for (const table of tables) {
      const deleted = await this.execute(
        `DELETE FROM ${table} WHERE source_account_id = $1`,
        [sourceAccountId]
      );
      total += deleted;
    }
    return total;
  }
}

/**
 * Parse DATABASE_URL into connection parameters
 */
function parseDatabaseUrl(url: string | undefined): {
  host: string;
  port: number;
  database: string;
  user: string;
  password: string;
  ssl: boolean;
} | null {
  if (!url) {
    return null;
  }

  try {
    const match = url.match(/^postgres(?:ql)?:\/\/([^:]+):([^@]+)@([^:]+):(\d+)\/([^?]+)(\?.*)?$/);
    if (!match) {
      return null;
    }

    const [, user, password, host, port, database, queryString] = match;
    const ssl = queryString?.includes('sslmode=require') || queryString?.includes('ssl=true') || false;

    return {
      host,
      port: parseInt(port, 10),
      database,
      user,
      password,
      ssl,
    };
  } catch {
    return null;
  }
}

export function createDatabase(config?: Partial<DatabaseConfig>): Database {
  // Try to parse DATABASE_URL first, fall back to individual POSTGRES_* vars
  const dbFromUrl = parseDatabaseUrl(process.env.DATABASE_URL);

  const fullConfig: DatabaseConfig = {
    host: config?.host ?? dbFromUrl?.host ?? process.env.POSTGRES_HOST ?? 'localhost',
    port: config?.port ?? dbFromUrl?.port ?? parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    database: config?.database ?? dbFromUrl?.database ?? process.env.POSTGRES_DB ?? 'nself',
    user: config?.user ?? dbFromUrl?.user ?? process.env.POSTGRES_USER ?? 'postgres',
    password: config?.password ?? dbFromUrl?.password ?? process.env.POSTGRES_PASSWORD ?? '',
    ssl: config?.ssl ?? dbFromUrl?.ssl ?? process.env.POSTGRES_SSL === 'true',
    maxConnections: config?.maxConnections ?? parseInt(process.env.POSTGRES_MAX_CONNECTIONS ?? '10', 10),
  };

  // Validate that we have a password (empty string will cause SCRAM auth errors)
  if (!fullConfig.password) {
    const source = config?.password ? 'config' :
                   dbFromUrl?.password ? 'DATABASE_URL' :
                   process.env.POSTGRES_PASSWORD ? 'POSTGRES_PASSWORD' : 'none';
    logger.error('Database password is empty or undefined', {
      source,
      hasDatabaseUrl: !!process.env.DATABASE_URL,
      hasPostgresPassword: !!process.env.POSTGRES_PASSWORD,
    });
  }

  return new Database(fullConfig);
}
