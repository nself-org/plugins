/**
 * JWT Token Generation and Verification
 * Provides access and refresh token management
 */

import jwt from 'jsonwebtoken';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('auth:tokens');

export interface TokenPayload {
  userId: string;
  email?: string;
  role?: string;
  sessionId?: string;
}

export interface TokenPair {
  accessToken: string;
  refreshToken: string;
  expiresIn: number;
}

export interface TokenConfig {
  accessTokenSecret: string;
  refreshTokenSecret: string;
  accessTokenExpiresIn: string; // e.g., '15m'
  refreshTokenExpiresIn: string; // e.g., '7d'
}

export class TokenService {
  private config: TokenConfig;

  constructor(config: TokenConfig) {
    this.config = config;
  }

  /**
   * Generate access and refresh token pair
   */
  generateTokenPair(payload: TokenPayload): TokenPair {
    const accessToken = this.generateAccessToken(payload);
    const refreshToken = this.generateRefreshToken(payload);

    // Calculate expiry in seconds
    const expiresIn = this.getExpirySeconds(this.config.accessTokenExpiresIn);

    logger.debug('Token pair generated', { userId: payload.userId });

    return {
      accessToken,
      refreshToken,
      expiresIn,
    };
  }

  /**
   * Generate access token (short-lived)
   */
  generateAccessToken(payload: TokenPayload): string {
    const tokenData = {
      userId: payload.userId,
      email: payload.email,
      role: payload.role,
      sessionId: payload.sessionId,
      type: 'access',
    };

    // Type assertion needed due to StringValue type from ms package
    const token = jwt.sign(tokenData, this.config.accessTokenSecret, {
      expiresIn: this.config.accessTokenExpiresIn,
    } as any);

    return token;
  }

  /**
   * Generate refresh token (long-lived)
   */
  generateRefreshToken(payload: TokenPayload): string {
    const tokenData = {
      userId: payload.userId,
      sessionId: payload.sessionId,
      type: 'refresh',
    };

    // Type assertion needed due to StringValue type from ms package
    const token = jwt.sign(tokenData, this.config.refreshTokenSecret, {
      expiresIn: this.config.refreshTokenExpiresIn,
    } as any);

    return token;
  }

  /**
   * Verify and decode access token
   */
  verifyAccessToken(token: string): TokenPayload {
    try {
      const decoded = jwt.verify(token, this.config.accessTokenSecret, {
        issuer: 'nself-auth',
        audience: 'nself',
      }) as any;

      if (decoded.type !== 'access') {
        throw new Error('Invalid token type');
      }

      return {
        userId: decoded.userId,
        email: decoded.email,
        role: decoded.role,
        sessionId: decoded.sessionId,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.warn('Access token verification failed', { error: message });
      throw new Error(`Invalid access token: ${message}`);
    }
  }

  /**
   * Verify and decode refresh token
   */
  verifyRefreshToken(token: string): TokenPayload {
    try {
      const decoded = jwt.verify(token, this.config.refreshTokenSecret, {
        issuer: 'nself-auth',
        audience: 'nself',
      }) as any;

      if (decoded.type !== 'refresh') {
        throw new Error('Invalid token type');
      }

      return {
        userId: decoded.userId,
        sessionId: decoded.sessionId,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.warn('Refresh token verification failed', { error: message });
      throw new Error(`Invalid refresh token: ${message}`);
    }
  }

  /**
   * Decode token without verification (for debugging)
   */
  decodeToken(token: string): any {
    return jwt.decode(token);
  }

  /**
   * Convert expiry string to seconds
   */
  private getExpirySeconds(expiry: string): number {
    // Parse expiry string like '15m', '7d', '1h'
    const match = expiry.match(/^(\d+)([smhd])$/);
    if (!match) {
      throw new Error(`Invalid expiry format: ${expiry}`);
    }

    const value = parseInt(match[1], 10);
    const unit = match[2];

    switch (unit) {
      case 's':
        return value;
      case 'm':
        return value * 60;
      case 'h':
        return value * 60 * 60;
      case 'd':
        return value * 60 * 60 * 24;
      default:
        throw new Error(`Unknown time unit: ${unit}`);
    }
  }
}
