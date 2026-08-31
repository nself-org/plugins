import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('file-processing:client');

export class FileProcessingClient {
  private baseUrl: string;
  private apiKey?: string;

  constructor(baseUrl: string, apiKey?: string) {
    this.baseUrl = baseUrl.replace(/\/$/, '');
    this.apiKey = apiKey;
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' };
    if (this.apiKey) headers['Authorization'] = `Bearer ${this.apiKey}`;
    const response = await fetch(`${this.baseUrl}${path}`, {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    });
    if (!response.ok) {
      const text = await response.text();
      throw new Error(`${method} ${path} failed: ${response.status} ${text}`);
    }
    return response.json() as Promise<T>;
  }

  async processFile(
    fileUrl: string,
    operations: Array<{ type: string; params?: Record<string, unknown> }>,
  ): Promise<{ job_id: string }> {
    logger.debug('Submitting file processing job', { fileUrl, operationCount: operations.length });
    return this.request('POST', '/jobs', { file_url: fileUrl, operations });
  }

  async listJobs(options?: {
    status?: string;
    limit?: number;
  }): Promise<Array<{ job_id: string; status: string; created_at: string }>> {
    const params = new URLSearchParams();
    if (options?.status) params.set('status', options.status);
    if (options?.limit !== undefined) params.set('limit', String(options.limit));
    const qs = params.toString();
    logger.debug('Listing jobs', options);
    return this.request('GET', `/jobs${qs ? `?${qs}` : ''}`);
  }

  async getResult(jobId: string): Promise<{
    job_id: string;
    status: string;
    output?: unknown;
    error?: string;
  }> {
    logger.debug('Getting job result', { jobId });
    return this.request('GET', `/jobs/${jobId}/result`);
  }

  async cancelJob(jobId: string): Promise<void> {
    logger.debug('Cancelling job', { jobId });
    await this.request('DELETE', `/jobs/${jobId}`);
  }
}
