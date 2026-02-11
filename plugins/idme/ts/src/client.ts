/**
 * ID.me API Client
 * Complete OAuth 2.0 implementation with verification groups
 */

import crypto from 'crypto';
import { createLogger } from '@nself/plugin-utils';
import type {
  IDmeConfig,
  IDmeTokens,
  IDmeUserProfile,
  IDmeVerification,
  IDmeGroup,
  IDmeAttributes,
} from './types.js';

const logger = createLogger('idme:client');

const IDME_BASE_URL = 'https://api.id.me';
const IDME_SANDBOX_URL = 'https://api.idmelabs.com';

export class IDmeClient {
  private config: IDmeConfig;
  private baseUrl: string;

  constructor(config: IDmeConfig) {
    this.config = config;
    this.baseUrl = config.sandbox ? IDME_SANDBOX_URL : IDME_BASE_URL;
    logger.info('ID.me client initialized', {
      mode: config.sandbox ? 'sandbox' : 'production',
    });
  }

  /**
   * Get the authorization URL for OAuth flow
   * @param state - CSRF protection state parameter
   */
  getAuthorizationUrl(state: string): string {
    const params = new URLSearchParams({
      client_id: this.config.clientId,
      redirect_uri: this.config.redirectUri,
      response_type: 'code',
      scope: this.config.scopes.join(' '),
      state,
    });

    const url = `${this.baseUrl}/oauth/authorize?${params.toString()}`;
    logger.debug('Generated authorization URL', { state });
    return url;
  }

  /**
   * Exchange authorization code for tokens
   */
  async exchangeCode(code: string): Promise<IDmeTokens> {
    logger.debug('Exchanging authorization code');

    const response = await fetch(`${this.baseUrl}/oauth/token`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
      },
      body: new URLSearchParams({
        client_id: this.config.clientId,
        client_secret: this.config.clientSecret,
        code,
        grant_type: 'authorization_code',
        redirect_uri: this.config.redirectUri,
      }),
    });

    if (!response.ok) {
      const error = await response.text();
      logger.error('Failed to exchange code', { status: response.status, error });
      throw new Error(`Failed to exchange authorization code: ${error}`);
    }

    const data = await response.json() as Record<string, unknown>;

    return {
      accessToken: data.access_token as string,
      refreshToken: data.refresh_token as string,
      expiresIn: data.expires_in as number,
      expiresAt: new Date(Date.now() + (data.expires_in as number) * 1000),
    };
  }

  /**
   * Refresh access token
   */
  async refreshToken(refreshToken: string): Promise<IDmeTokens> {
    logger.debug('Refreshing access token');

    const response = await fetch(`${this.baseUrl}/oauth/token`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
      },
      body: new URLSearchParams({
        client_id: this.config.clientId,
        client_secret: this.config.clientSecret,
        refresh_token: refreshToken,
        grant_type: 'refresh_token',
      }),
    });

    if (!response.ok) {
      const error = await response.text();
      logger.error('Failed to refresh token', { status: response.status, error });
      throw new Error(`Failed to refresh token: ${error}`);
    }

    const data = await response.json() as Record<string, unknown>;

    return {
      accessToken: data.access_token as string,
      refreshToken: (data.refresh_token as string) || refreshToken,
      expiresIn: data.expires_in as number,
      expiresAt: new Date(Date.now() + (data.expires_in as number) * 1000),
    };
  }

  /**
   * Get user profile from ID.me
   */
  async getUserProfile(accessToken: string): Promise<IDmeUserProfile> {
    logger.debug('Fetching user profile');

    const response = await fetch(`${this.baseUrl}/api/public/v3/attributes.json`, {
      headers: {
        Authorization: `Bearer ${accessToken}`,
      },
    });

    if (!response.ok) {
      const error = await response.text();
      logger.error('Failed to fetch profile', { status: response.status, error });
      throw new Error(`Failed to fetch user profile: ${error}`);
    }

    const data = await response.json() as Record<string, unknown>;

    return {
      firstName: data.fname as string | undefined,
      lastName: data.lname as string | undefined,
      email: data.email as string | undefined,
      birthDate: data.birth_date as string | undefined,
      zip: data.zip as string | undefined,
      phone: data.phone as string | undefined,
    };
  }

  /**
   * Get verification status for all groups
   */
  async getVerifications(accessToken: string): Promise<IDmeVerification> {
    logger.debug('Fetching verifications');

    const response = await fetch(`${this.baseUrl}/api/public/v3/verified.json`, {
      headers: {
        Authorization: `Bearer ${accessToken}`,
      },
    });

    if (!response.ok) {
      const error = await response.text();
      logger.error('Failed to fetch verifications', { status: response.status, error });
      throw new Error(`Failed to fetch verifications: ${error}`);
    }

    const data = await response.json() as Record<string, unknown>;
    const groups: IDmeGroup[] = [];
    const attributes: IDmeAttributes = {};

    // Parse verification groups
    if (data.military) {
      const military = data.military as Record<string, unknown>;
      groups.push({
        id: 'military',
        name: 'Military',
        type: 'military',
        verified: true,
        verifiedAt: military.verified_at as string | undefined,
      });
    }

    if (data.veteran) {
      const veteran = data.veteran as Record<string, unknown>;
      groups.push({
        id: 'veteran',
        name: 'Veteran',
        type: 'veteran',
        verified: true,
        verifiedAt: veteran.verified_at as string | undefined,
      });

      // Extract veteran-specific attributes
      if (veteran.affiliation) attributes.affiliation = veteran.affiliation as string;
      if (veteran.branch) attributes.branch = veteran.branch as string;
      if (veteran.service_era) attributes.serviceEra = veteran.service_era as string;
      if (veteran.rank) attributes.rank = veteran.rank as string;
      if (veteran.status) attributes.status = veteran.status as string;
    }

    if (data.first_responder) {
      const firstResponder = data.first_responder as Record<string, unknown>;
      groups.push({
        id: 'first_responder',
        name: 'First Responder',
        type: 'first_responder',
        verified: true,
        verifiedAt: firstResponder.verified_at as string | undefined,
      });
    }

    if (data.government) {
      const government = data.government as Record<string, unknown>;
      groups.push({
        id: 'government',
        name: 'Government Employee',
        type: 'government',
        verified: true,
        verifiedAt: government.verified_at as string | undefined,
      });
    }

    if (data.teacher) {
      const teacher = data.teacher as Record<string, unknown>;
      groups.push({
        id: 'teacher',
        name: 'Teacher',
        type: 'teacher',
        verified: true,
        verifiedAt: teacher.verified_at as string | undefined,
      });
    }

    if (data.student) {
      const student = data.student as Record<string, unknown>;
      groups.push({
        id: 'student',
        name: 'Student',
        type: 'student',
        verified: true,
        verifiedAt: student.verified_at as string | undefined,
      });
    }

    if (data.nurse) {
      const nurse = data.nurse as Record<string, unknown>;
      groups.push({
        id: 'nurse',
        name: 'Nurse',
        type: 'nurse',
        verified: true,
        verifiedAt: nurse.verified_at as string | undefined,
      });
    }

    logger.info('Verifications fetched', { groupCount: groups.length });

    return {
      verified: groups.length > 0,
      groups,
      attributes,
    };
  }

  /**
   * Revoke access token
   */
  async revokeToken(accessToken: string): Promise<void> {
    logger.debug('Revoking token');

    await fetch(`${this.baseUrl}/oauth/revoke`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
      },
      body: new URLSearchParams({
        client_id: this.config.clientId,
        client_secret: this.config.clientSecret,
        token: accessToken,
      }),
    });

    logger.info('Token revoked');
  }

  /**
   * Verify webhook signature
   */
  verifyWebhookSignature(payload: string, signature: string): boolean {
    if (!this.config.webhookSecret) {
      logger.warn('Webhook secret not configured');
      return false;
    }

    const expected = crypto
      .createHmac('sha256', this.config.webhookSecret)
      .update(payload)
      .digest('hex');

    return signature === expected;
  }
}

/**
 * Helper to create ID.me client with environment variables
 */
export function createIDmeClient(options?: Partial<IDmeConfig>): IDmeClient {
  const config: IDmeConfig = {
    clientId: process.env.IDME_CLIENT_ID || options?.clientId || '',
    clientSecret: process.env.IDME_CLIENT_SECRET || options?.clientSecret || '',
    redirectUri: process.env.IDME_REDIRECT_URI || options?.redirectUri || '',
    scopes: (process.env.IDME_SCOPES || options?.scopes?.join(',') || 'openid,email,profile').split(','),
    sandbox: process.env.IDME_SANDBOX === 'true' || options?.sandbox || false,
    webhookSecret: process.env.IDME_WEBHOOK_SECRET || options?.webhookSecret,
  };

  if (!config.clientId || !config.clientSecret || !config.redirectUri) {
    throw new Error('Missing required ID.me configuration');
  }

  return new IDmeClient(config);
}
