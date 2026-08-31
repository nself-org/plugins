import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('media-processing:client');

export class MediaProcessingClient {
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

  async transcode(
    inputUrl: string,
    options: { format: string; quality?: string; resolution?: string },
  ): Promise<{ job_id: string; status: string }> {
    logger.debug('Submitting transcode job', { inputUrl, options });
    return this.request('POST', '/jobs/transcode', { input_url: inputUrl, ...options });
  }

  async extractMetadata(url: string): Promise<{
    duration?: number;
    codec?: string;
    resolution?: string;
    bitrate?: number;
  }> {
    logger.debug('Extracting metadata', { url });
    return this.request('POST', '/metadata', { url });
  }

  async generateThumbnail(
    url: string,
    options?: { time?: number; width?: number },
  ): Promise<{ url: string }> {
    logger.debug('Generating thumbnail', { url, options });
    return this.request('POST', '/thumbnail', { url, ...options });
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

  async getJobStatus(jobId: string): Promise<{
    job_id: string;
    status: string;
    output_url?: string;
    error?: string;
  }> {
    logger.debug('Getting job status', { jobId });
    return this.request('GET', `/jobs/${jobId}`);
  }
}
