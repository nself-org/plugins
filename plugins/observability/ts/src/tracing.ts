/**
 * Distributed Tracing with OpenTelemetry
 */

import { trace, context, SpanStatusCode } from '@opentelemetry/api';
import { BasicTracerProvider } from '@opentelemetry/sdk-trace-base';
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-http';
import type { TraceSpan, IngestTraceRequest } from './types.js';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('observability:tracing');

export class TracingService {
  private tempoUrl: string;
  private enabled: boolean;
  private provider: BasicTracerProvider;

  constructor(tempoUrl: string, enabled: boolean) {
    this.tempoUrl = tempoUrl;
    this.enabled = enabled;

    // Initialize OpenTelemetry
    const exporter = new OTLPTraceExporter({
      url: `${tempoUrl}/v1/traces`,
    });

    this.provider = new BasicTracerProvider();
    this.provider.register();

    // Store exporter for later use if needed
    void exporter;
  }

  async ingestTrace(request: IngestTraceRequest): Promise<void> {
    const span: TraceSpan = {
      trace_id: request.trace_id,
      span_id: request.span_id,
      parent_span_id: request.parent_span_id ?? null,
      operation_name: request.operation_name,
      start_time: request.start_time,
      end_time: request.end_time,
      duration_ms: request.duration_ms,
      tags: request.tags,
      logs: request.logs,
    };

    if (this.enabled) {
      await this.sendToTempo(span);
    }
  }

  private async sendToTempo(span: TraceSpan): Promise<void> {
    try {
      // Convert to Zipkin format (Tempo supports Zipkin format)
      const zipkinSpan = {
        traceId: span.trace_id,
        id: span.span_id,
        parentId: span.parent_span_id ?? undefined,
        name: span.operation_name,
        timestamp: Date.parse(span.start_time) * 1000,
        duration: span.duration_ms * 1000,
        tags: span.tags ?? {},
        annotations: span.logs?.map((log) => ({
          timestamp: Date.parse(log.timestamp) * 1000,
          value: log.message,
        })) ?? [],
      };

      const response = await fetch(`${this.tempoUrl}/api/v2/spans`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify([zipkinSpan]),
      });

      if (!response.ok) {
        const errorText = await response.text();
        logger.error('Failed to send trace to Tempo', {
          status: response.status,
          error: errorText,
        });
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Error sending trace to Tempo', { error: message });
    }
  }

  async queryTraces(
    traceId?: string,
    serviceName?: string,
    operationName?: string,
    startTime?: string,
    endTime?: string,
    limit?: number
  ): Promise<TraceSpan[]> {
    if (!this.enabled) {
      return [];
    }

    try {
      const params = new URLSearchParams({
        limit: String(limit ?? 20),
      });

      if (traceId) {
        params.set('traceId', traceId);
      }
      if (serviceName) {
        params.set('service', serviceName);
      }
      if (operationName) {
        params.set('operation', operationName);
      }
      if (startTime) {
        params.set('start', String(Date.parse(startTime) * 1000000));
      }
      if (endTime) {
        params.set('end', String(Date.parse(endTime) * 1000000));
      }

      const response = await fetch(`${this.tempoUrl}/api/search?${params.toString()}`);

      if (!response.ok) {
        throw new Error(`Tempo query failed: ${response.statusText}`);
      }

      const data = (await response.json()) as {
        traces?: Array<{
          traceID?: string;
          rootServiceName?: string;
          rootTraceName?: string;
          startTimeUnixNano?: string;
          durationMs?: number;
        }>;
      };

      const traces: TraceSpan[] = [];

      if (data.traces) {
        for (const trace of data.traces) {
          if (trace.traceID) {
            traces.push({
              trace_id: trace.traceID,
              span_id: trace.traceID,
              parent_span_id: null,
              operation_name: trace.rootTraceName ?? 'unknown',
              start_time: trace.startTimeUnixNano
                ? new Date(parseInt(trace.startTimeUnixNano) / 1000000).toISOString()
                : new Date().toISOString(),
              end_time: trace.startTimeUnixNano && trace.durationMs
                ? new Date(
                    parseInt(trace.startTimeUnixNano) / 1000000 + trace.durationMs
                  ).toISOString()
                : new Date().toISOString(),
              duration_ms: trace.durationMs ?? 0,
            });
          }
        }
      }

      return traces;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Error querying traces from Tempo', { error: message });
      return [];
    }
  }

  createSpan(
    operationName: string,
    traceId?: string,
    parentSpanId?: string
  ): {
    traceId: string;
    spanId: string;
    end: (tags?: Record<string, string | number | boolean>) => Promise<void>;
  } {
    const tracer = trace.getTracer('observability-plugin');
    const spanContext = context.active();
    const otelSpan = tracer.startSpan(operationName, undefined, spanContext);

    const generatedTraceId = traceId ?? this.generateTraceId();
    const generatedSpanId = this.generateSpanId();

    const startTime = new Date().toISOString();

    return {
      traceId: generatedTraceId,
      spanId: generatedSpanId,
      end: async (tags?: Record<string, string | number | boolean>) => {
        const endTime = new Date().toISOString();
        const durationMs = Date.parse(endTime) - Date.parse(startTime);

        otelSpan.setStatus({ code: SpanStatusCode.OK });
        otelSpan.end();

        await this.ingestTrace({
          trace_id: generatedTraceId,
          span_id: generatedSpanId,
          parent_span_id: parentSpanId ?? null,
          operation_name: operationName,
          start_time: startTime,
          end_time: endTime,
          duration_ms: durationMs,
          tags,
        });
      },
    };
  }

  private generateTraceId(): string {
    return Array.from({ length: 32 }, () =>
      Math.floor(Math.random() * 16).toString(16)
    ).join('');
  }

  private generateSpanId(): string {
    return Array.from({ length: 16 }, () =>
      Math.floor(Math.random() * 16).toString(16)
    ).join('');
  }
}
