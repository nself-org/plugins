/**
 * Donorbox API Client
 * Basic HTTP auth, page-based pagination, rate limiting
 */

import { RateLimiter, createLogger } from '@nself/plugin-utils';
import type {
  DonorboxCampaign,
  DonorboxDonor,
  DonorboxDonation,
  DonorboxPlan,
  DonorboxEvent,
  DonorboxTicket,
} from './types.js';

const logger = createLogger('donorbox:client');

const BASE_URL = 'https://donorbox.org/api/v1';

export class DonorboxClient {
  private rateLimiter: RateLimiter;
  private authHeader: string;

  constructor(email: string, apiKey: string) {
    this.authHeader = `Basic ${Buffer.from(`${email}:${apiKey}`).toString('base64')}`;
    // Donorbox: 60 req/min → 1 per second
    this.rateLimiter = new RateLimiter(1);
  }

  // ─── HTTP ──────────────────────────────────────────────────────────────

  private async request<T>(path: string, params?: Record<string, string>): Promise<T> {
    await this.rateLimiter.acquire();

    let url = `${BASE_URL}${path}`;
    if (params) {
      const searchParams = new URLSearchParams();
      for (const [key, value] of Object.entries(params)) {
        if (value !== undefined && value !== null) {
          searchParams.append(key, value);
        }
      }
      const paramString = searchParams.toString();
      if (paramString) url += `?${paramString}`;
    }

    const response = await fetch(url, {
      method: 'GET',
      headers: {
        'Authorization': this.authHeader,
        'Content-Type': 'application/json',
      },
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`Donorbox API error (${response.status}): ${errorText}`);
    }

    return await response.json() as T;
  }

  // ─── Pagination Helper ─────────────────────────────────────────────────

  private async listAllPaginated<T>(path: string, extraParams?: Record<string, string>): Promise<T[]> {
    const all: T[] = [];
    let page = 1;
    const perPage = 50;

    while (true) {
      const params: Record<string, string> = {
        page: String(page),
        per_page: String(perPage),
        ...extraParams,
      };

      const items = await this.request<T[]>(path, params);

      if (!items || items.length === 0) break;

      all.push(...items);

      if (items.length < perPage) break;
      page++;
    }

    return all;
  }

  // ─── Campaigns ─────────────────────────────────────────────────────────

  async listAllCampaigns(): Promise<DonorboxCampaign[]> {
    logger.debug('Fetching all campaigns');
    return this.listAllPaginated<DonorboxCampaign>('/campaigns');
  }

  // ─── Donors ────────────────────────────────────────────────────────────

  async listAllDonors(): Promise<DonorboxDonor[]> {
    logger.debug('Fetching all donors');
    return this.listAllPaginated<DonorboxDonor>('/donors');
  }

  // ─── Donations ─────────────────────────────────────────────────────────

  async listAllDonations(options?: { dateFrom?: string; dateTo?: string }): Promise<DonorboxDonation[]> {
    logger.debug('Fetching all donations');
    const params: Record<string, string> = {};
    if (options?.dateFrom) params.date_from = options.dateFrom;
    if (options?.dateTo) params.date_to = options.dateTo;
    return this.listAllPaginated<DonorboxDonation>('/donations', params);
  }

  // ─── Plans ─────────────────────────────────────────────────────────────

  async listAllPlans(): Promise<DonorboxPlan[]> {
    logger.debug('Fetching all recurring plans');
    return this.listAllPaginated<DonorboxPlan>('/plans');
  }

  // ─── Events ────────────────────────────────────────────────────────────

  async listAllEvents(): Promise<DonorboxEvent[]> {
    logger.debug('Fetching all events');
    return this.listAllPaginated<DonorboxEvent>('/events');
  }

  // ─── Tickets ───────────────────────────────────────────────────────────

  async listAllTickets(): Promise<DonorboxTicket[]> {
    logger.debug('Fetching all tickets');
    return this.listAllPaginated<DonorboxTicket>('/tickets');
  }
}
