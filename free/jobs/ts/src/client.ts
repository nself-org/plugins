import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('jobs:client');

export interface EnqueueOptions {
  delay?: number;
  retries?: number;
}

export interface EnqueueJob {
  type: string;
  data: Record<string, unknown>;
  options?: EnqueueOptions;
}

export interface Job {
  id: string;
  type: string;
  status: string;
  data: Record<string, unknown>;
  created_at: string;
  completed_at?: string;
}

export interface JobSummary {
  id: string;
  type: string;
  status: string;
  created_at: string;
}

export interface QueueStats {
  waiting: number;
  active: number;
  completed: number;
  failed: number;
}

export interface ListJobsOptions {
  status?: string;
  limit?: number;
}

export class JobsClient {
  private baseUrl: string;
  private apiKey?: string;

  constructor(baseUrl: string, apiKey?: string) {
    this.baseUrl = baseUrl.replace(/\/$/, '');
    this.apiKey = apiKey;
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' };
    if (this.apiKey) headers['Authorization'] = `Bearer ${this.apiKey}`;
    logger.debug(`${method} ${path}`);
    const res = await fetch(`${this.baseUrl}${path}`, {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    });
    if (!res.ok) throw new Error(`${method} ${path} failed: ${res.status}`);
    return res.json() as Promise<T>;
  }

  async enqueue(queue: string, job: EnqueueJob): Promise<{ job_id: string }> {
    return this.request<{ job_id: string }>('POST', `/queues/${queue}/jobs`, job);
  }

  async getJob(jobId: string): Promise<Job> {
    return this.request<Job>('GET', `/jobs/${jobId}`);
  }

  async listJobs(queue: string, options?: ListJobsOptions): Promise<JobSummary[]> {
    const params = new URLSearchParams();
    if (options?.status) params.set('status', options.status);
    if (options?.limit !== undefined) params.set('limit', String(options.limit));
    const qs = params.toString() ? `?${params}` : '';
    return this.request<JobSummary[]>('GET', `/queues/${queue}/jobs${qs}`);
  }

  async cancelJob(jobId: string): Promise<void> {
    await this.request<void>('DELETE', `/jobs/${jobId}`);
  }

  async getQueueStats(queue: string): Promise<QueueStats> {
    return this.request<QueueStats>('GET', `/queues/${queue}/stats`);
  }
}
