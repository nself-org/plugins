/**
 * Structured Logging to Loki
 */

import winston from 'winston';
import type { LogEntry, LogLevel, IngestLogRequest } from './types.js';
import { createLogger as createUtilsLogger } from '@nself/plugin-utils';

const logger = createUtilsLogger('observability:logging');

export class LoggingService {
  private lokiUrl: string;
  private enabled: boolean;
  private winstonLogger: winston.Logger;

  constructor(lokiUrl: string, enabled: boolean) {
    this.lokiUrl = lokiUrl;
    this.enabled = enabled;

    // Create Winston logger with JSON format
    this.winstonLogger = winston.createLogger({
      level: 'debug',
      format: winston.format.combine(
        winston.format.timestamp(),
        winston.format.json()
      ),
      transports: [
        new winston.transports.Console({
          format: winston.format.combine(
            winston.format.colorize(),
            winston.format.simple()
          ),
        }),
      ],
    });
  }

  async ingestLog(request: IngestLogRequest): Promise<void> {
    const entry: LogEntry = {
      timestamp: request.timestamp ?? new Date().toISOString(),
      level: request.level,
      message: request.message,
      trace_id: request.trace_id,
      span_id: request.span_id,
      user_id: request.user_id,
      source_account_id: request.source_account_id ?? 'primary',
      service: request.service,
      metadata: request.metadata,
    };

    // Log locally with Winston
    this.winstonLogger.log(entry.level, entry.message, {
      trace_id: entry.trace_id,
      span_id: entry.span_id,
      user_id: entry.user_id,
      source_account_id: entry.source_account_id,
      service: entry.service,
      metadata: entry.metadata,
    });

    // Send to Loki if enabled
    if (this.enabled) {
      await this.sendToLoki(entry);
    }
  }

  private async sendToLoki(entry: LogEntry): Promise<void> {
    try {
      const labels: Record<string, string> = {
        level: entry.level,
        source_account_id: entry.source_account_id ?? 'primary',
      };

      if (entry.service) {
        labels.service = entry.service;
      }

      if (entry.user_id) {
        labels.user_id = entry.user_id;
      }

      const payload = {
        streams: [
          {
            stream: labels,
            values: [
              [
                String(Date.parse(entry.timestamp) * 1000000),
                JSON.stringify({
                  message: entry.message,
                  trace_id: entry.trace_id,
                  span_id: entry.span_id,
                  metadata: entry.metadata,
                }),
              ],
            ],
          },
        ],
      };

      const response = await fetch(`${this.lokiUrl}/loki/api/v1/push`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(payload),
      });

      if (!response.ok) {
        const errorText = await response.text();
        logger.error('Failed to send log to Loki', {
          status: response.status,
          error: errorText,
        });
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Error sending log to Loki', { error: message });
    }
  }

  async queryLogs(
    query: string,
    startTime?: string,
    endTime?: string,
    limit?: number
  ): Promise<LogEntry[]> {
    if (!this.enabled) {
      return [];
    }

    try {
      const params = new URLSearchParams({
        query,
        limit: String(limit ?? 100),
      });

      if (startTime) {
        params.set('start', String(Date.parse(startTime) * 1000000));
      }
      if (endTime) {
        params.set('end', String(Date.parse(endTime) * 1000000));
      }

      const response = await fetch(
        `${this.lokiUrl}/loki/api/v1/query_range?${params.toString()}`
      );

      if (!response.ok) {
        throw new Error(`Loki query failed: ${response.statusText}`);
      }

      const data = (await response.json()) as {
        data?: {
          result?: Array<{
            stream?: Record<string, string>;
            values?: Array<[string, string]>;
          }>;
        };
      };

      const logs: LogEntry[] = [];

      if (data.data?.result) {
        for (const result of data.data.result) {
          if (result.values) {
            for (const [timestampNs, line] of result.values) {
              try {
                const parsed = JSON.parse(line) as {
                  message: string;
                  trace_id?: string;
                  span_id?: string;
                  metadata?: Record<string, unknown>;
                };

                logs.push({
                  timestamp: new Date(parseInt(timestampNs) / 1000000).toISOString(),
                  level: (result.stream?.level as LogLevel) ?? 'info',
                  message: parsed.message,
                  trace_id: parsed.trace_id,
                  span_id: parsed.span_id,
                  user_id: result.stream?.user_id,
                  source_account_id: result.stream?.source_account_id,
                  service: result.stream?.service,
                  metadata: parsed.metadata,
                });
              } catch (parseError) {
                logger.warn('Failed to parse log entry', { line });
              }
            }
          }
        }
      }

      return logs;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Error querying logs from Loki', { error: message });
      return [];
    }
  }

  log(level: LogLevel, message: string, metadata?: Record<string, unknown>): void {
    this.ingestLog({
      level,
      message,
      metadata,
    }).catch((error) => {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to ingest log', { error: errorMessage });
    });
  }

  debug(message: string, metadata?: Record<string, unknown>): void {
    this.log('debug', message, metadata);
  }

  info(message: string, metadata?: Record<string, unknown>): void {
    this.log('info', message, metadata);
  }

  warn(message: string, metadata?: Record<string, unknown>): void {
    this.log('warn', message, metadata);
  }

  error(message: string, metadata?: Record<string, unknown>): void {
    this.log('error', message, metadata);
  }
}
