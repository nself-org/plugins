/**
 * Subsonic Authentication
 * Handles Subsonic API authentication via query parameters.
 * Supports plaintext passwords (p=password) and hex-encoded passwords (p=enc:hex).
 * Also supports token+salt authentication (t=token, s=salt) per Subsonic API 1.13.0+.
 */

import crypto from 'node:crypto';
import { createLogger } from '@nself/plugin-utils';
import type { SubsonicQueryParams } from './types.js';

const logger = createLogger('subsonic:auth');

export interface AuthResult {
  authenticated: boolean;
  username: string;
  error?: string;
  errorCode?: number;
}

/**
 * Decode a hex-encoded password (enc:HEXSTRING)
 */
function decodeHexPassword(encoded: string): string {
  const hex = encoded.slice(4); // Remove "enc:" prefix
  return Buffer.from(hex, 'hex').toString('utf-8');
}

/**
 * Validate Subsonic API credentials.
 *
 * Authentication methods:
 * 1. Plaintext: p=cleartext_password
 * 2. Hex-encoded: p=enc:hexadecimal_encoded_password
 * 3. Token/Salt (API 1.13.0+): t=md5(password+salt), s=salt
 */
export function authenticate(
  params: SubsonicQueryParams & { t?: string; s?: string },
  adminPassword: string
): AuthResult {
  const username = params.u;
  const password = params.p;
  const token = params.t;
  const salt = params.s;

  if (!username) {
    return {
      authenticated: false,
      username: '',
      error: 'Required parameter is missing: u',
      errorCode: 10,
    };
  }

  // Token-based authentication (preferred in newer clients)
  if (token && salt) {
    const expectedToken = crypto
      .createHash('md5')
      .update(adminPassword + salt)
      .digest('hex');

    if (token.toLowerCase() === expectedToken.toLowerCase()) {
      logger.debug('Token authentication successful', { username });
      return { authenticated: true, username };
    }

    logger.warn('Token authentication failed', { username });
    return {
      authenticated: false,
      username,
      error: 'Wrong username or password',
      errorCode: 40,
    };
  }

  // Password-based authentication
  if (!password) {
    return {
      authenticated: false,
      username,
      error: 'Required parameter is missing: p (or t+s)',
      errorCode: 10,
    };
  }

  let clearPassword: string;
  if (password.startsWith('enc:')) {
    try {
      clearPassword = decodeHexPassword(password);
    } catch {
      return {
        authenticated: false,
        username,
        error: 'Invalid hex-encoded password',
        errorCode: 40,
      };
    }
  } else {
    clearPassword = password;
  }

  if (clearPassword === adminPassword) {
    logger.debug('Password authentication successful', { username });
    return { authenticated: true, username };
  }

  logger.warn('Password authentication failed', { username });
  return {
    authenticated: false,
    username,
    error: 'Wrong username or password',
    errorCode: 40,
  };
}

/**
 * Validate the Subsonic API version in the request.
 * Returns null if valid, or an error message if incompatible.
 */
export function validateApiVersion(clientVersion: string | undefined): string | null {
  if (!clientVersion) {
    return null; // Many clients omit version; be lenient
  }

  const parts = clientVersion.split('.').map(Number);
  if (parts.length < 2 || isNaN(parts[0]) || isNaN(parts[1])) {
    return null; // Don't reject malformed versions; just ignore
  }

  // We support Subsonic API 1.x
  if (parts[0] !== 1) {
    return `Incompatible Subsonic REST protocol version. Server: 1.16.1, Client: ${clientVersion}`;
  }

  return null;
}
