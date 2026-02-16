/**
 * OAuth Service
 * Full implementation using Passport.js strategies for all providers
 */

import passport from 'passport';
import { Strategy as GoogleStrategy } from 'passport-google-oauth20';
import { Strategy as FacebookStrategy } from 'passport-facebook';
import { Strategy as GitHubStrategy } from 'passport-github2';
// @ts-ignore - passport-azure-ad doesn't have proper types
import { BearerStrategy as MicrosoftStrategy } from 'passport-azure-ad';
// @ts-ignore - passport-apple doesn't have types
import AppleStrategy from 'passport-apple';
import jwt from 'jsonwebtoken';
import { createLogger } from '@nself/plugin-utils';
import { AuthConfig, OAuthProviderRecord } from './types.js';
import { AuthDatabase } from './database.js';
import { encrypt } from './crypto.js';

const logger = createLogger('auth:oauth');

export type OAuthProvider = 'google' | 'apple' | 'facebook' | 'github' | 'microsoft';

export interface OAuthProfile {
  provider: OAuthProvider;
  providerId: string;
  email: string | null;
  name: string | null;
  avatarUrl: string | null;
  raw: Record<string, unknown>;
}

export interface OAuthTokens {
  accessToken: string;
  refreshToken?: string;
  expiresAt?: Date;
  scopes?: string[];
}

export interface OAuthAuthorizationUrl {
  url: string;
  state: string;
}

export class OAuthService {
  private config: AuthConfig;
  private db: AuthDatabase;
  private encryptionKey: string;
  private baseUrl: string;
  private appleClientSecret: string | null = null;
  private appleClientSecretExpiry: number = 0;

  constructor(config: AuthConfig, db: AuthDatabase) {
    this.config = config;
    this.db = db;
    this.encryptionKey = config.security.encryptionKey;
    this.baseUrl = config.magicLink.baseUrl; // Reuse base URL config

    this.initializeStrategies();
  }

  /**
   * Initialize all OAuth strategies
   */
  private initializeStrategies(): void {
    // Google OAuth
    if (this.config.oauth.google) {
      passport.use(
        new GoogleStrategy(
          {
            clientID: this.config.oauth.google.clientId,
            clientSecret: this.config.oauth.google.clientSecret,
            callbackURL: `${this.baseUrl}/api/oauth/google/callback`,
            scope: this.config.oauth.google.scopes || ['profile', 'email'],
          },
          (accessToken, refreshToken, profile, done) => {
            done(null, { accessToken, refreshToken, profile });
          }
        )
      );
      logger.info('Google OAuth strategy initialized');
    }

    // Apple Sign In
    if (this.config.oauth.apple) {
      passport.use(
        new AppleStrategy(
          {
            clientID: this.config.oauth.apple.clientId,
            teamID: this.config.oauth.apple.teamId,
            keyID: this.config.oauth.apple.keyId,
            privateKeyString: this.config.oauth.apple.privateKey,
            callbackURL: `${this.baseUrl}/api/oauth/apple/callback`,
            scope: ['name', 'email'],
          },
          (accessToken: string, refreshToken: string, profile: any, done: Function) => {
            done(null, { accessToken, refreshToken, profile });
          }
        )
      );
      logger.info('Apple OAuth strategy initialized');
    }

    // Facebook OAuth
    if (this.config.oauth.facebook) {
      passport.use(
        new FacebookStrategy(
          {
            clientID: this.config.oauth.facebook.appId,
            clientSecret: this.config.oauth.facebook.appSecret,
            callbackURL: `${this.baseUrl}/api/oauth/facebook/callback`,
            profileFields: ['id', 'displayName', 'email', 'picture.type(large)'],
          },
          (accessToken, refreshToken, profile, done) => {
            done(null, { accessToken, refreshToken, profile });
          }
        )
      );
      logger.info('Facebook OAuth strategy initialized');
    }

    // GitHub OAuth
    if (this.config.oauth.github) {
      passport.use(
        new GitHubStrategy(
          {
            clientID: this.config.oauth.github.clientId,
            clientSecret: this.config.oauth.github.clientSecret,
            callbackURL: `${this.baseUrl}/api/oauth/github/callback`,
            scope: ['user:email'],
          },
          (accessToken: string, refreshToken: string, profile: any, done: Function) => {
            done(null, { accessToken, refreshToken, profile });
          }
        )
      );
      logger.info('GitHub OAuth strategy initialized');
    }

    // Microsoft OAuth
    if (this.config.oauth.microsoft) {
      passport.use(
        new MicrosoftStrategy(
          {
            identityMetadata: 'https://login.microsoftonline.com/common/v2.0/.well-known/openid-configuration',
            clientID: this.config.oauth.microsoft.clientId,
            clientSecret: this.config.oauth.microsoft.clientSecret,
            redirectUrl: `${this.baseUrl}/api/oauth/microsoft/callback`,
            scope: ['openid', 'profile', 'email'],
          },
          (token: string, done: Function) => {
            done(null, { accessToken: token, profile: {} });
          }
        )
      );
      logger.info('Microsoft OAuth strategy initialized');
    }
  }

  /**
   * Generate OAuth authorization URL
   */
  async getAuthorizationUrl(
    provider: OAuthProvider,
    redirectUri: string,
    state?: string,
    scopes?: string[]
  ): Promise<OAuthAuthorizationUrl> {
    const generatedState = state || this.generateState();

    switch (provider) {
      case 'google': {
        if (!this.config.oauth.google) {
          throw new Error('Google OAuth not configured');
        }
        const scopeList = scopes || this.config.oauth.google.scopes || ['profile', 'email'];
        const url = `https://accounts.google.com/o/oauth2/v2/auth?client_id=${this.config.oauth.google.clientId}&redirect_uri=${encodeURIComponent(redirectUri)}&response_type=code&scope=${encodeURIComponent(scopeList.join(' '))}&state=${generatedState}`;
        return { url, state: generatedState };
      }

      case 'apple': {
        if (!this.config.oauth.apple) {
          throw new Error('Apple OAuth not configured');
        }
        const scopeList = scopes || ['name', 'email'];
        const url = `https://appleid.apple.com/auth/authorize?client_id=${this.config.oauth.apple.clientId}&redirect_uri=${encodeURIComponent(redirectUri)}&response_type=code&scope=${encodeURIComponent(scopeList.join(' '))}&state=${generatedState}&response_mode=form_post`;
        return { url, state: generatedState };
      }

      case 'facebook': {
        if (!this.config.oauth.facebook) {
          throw new Error('Facebook OAuth not configured');
        }
        const scopeList = scopes || ['email', 'public_profile'];
        const url = `https://www.facebook.com/v12.0/dialog/oauth?client_id=${this.config.oauth.facebook.appId}&redirect_uri=${encodeURIComponent(redirectUri)}&scope=${encodeURIComponent(scopeList.join(','))}&state=${generatedState}`;
        return { url, state: generatedState };
      }

      case 'github': {
        if (!this.config.oauth.github) {
          throw new Error('GitHub OAuth not configured');
        }
        const scopeList = scopes || ['user:email'];
        const url = `https://github.com/login/oauth/authorize?client_id=${this.config.oauth.github.clientId}&redirect_uri=${encodeURIComponent(redirectUri)}&scope=${encodeURIComponent(scopeList.join(' '))}&state=${generatedState}`;
        return { url, state: generatedState };
      }

      case 'microsoft': {
        if (!this.config.oauth.microsoft) {
          throw new Error('Microsoft OAuth not configured');
        }
        const scopeList = scopes || ['openid', 'profile', 'email'];
        const url = `https://login.microsoftonline.com/common/oauth2/v2.0/authorize?client_id=${this.config.oauth.microsoft.clientId}&redirect_uri=${encodeURIComponent(redirectUri)}&response_type=code&scope=${encodeURIComponent(scopeList.join(' '))}&state=${generatedState}`;
        return { url, state: generatedState };
      }

      default:
        throw new Error(`Unsupported OAuth provider: ${provider}`);
    }
  }

  /**
   * Exchange authorization code for tokens and profile
   */
  async handleCallback(
    provider: OAuthProvider,
    code: string,
    redirectUri: string
  ): Promise<{ profile: OAuthProfile; tokens: OAuthTokens }> {
    try {
      switch (provider) {
        case 'google':
          return await this.handleGoogleCallback(code, redirectUri);
        case 'apple':
          return await this.handleAppleCallback(code, redirectUri);
        case 'facebook':
          return await this.handleFacebookCallback(code, redirectUri);
        case 'github':
          return await this.handleGitHubCallback(code, redirectUri);
        case 'microsoft':
          return await this.handleMicrosoftCallback(code, redirectUri);
        default:
          throw new Error(`Unsupported OAuth provider: ${provider}`);
      }
    } catch (error) {
      logger.error(`OAuth callback error for ${provider}`, { error });
      throw error;
    }
  }

  /**
   * Link OAuth provider to existing user
   */
  async linkProvider(
    userId: string,
    provider: OAuthProvider,
    profile: OAuthProfile,
    tokens: OAuthTokens
  ): Promise<void> {
    // Encrypt tokens
    const accessTokenEncrypted = encrypt(tokens.accessToken, this.encryptionKey);
    const refreshTokenEncrypted = tokens.refreshToken
      ? encrypt(tokens.refreshToken, this.encryptionKey)
      : null;

    // Check if provider already linked
    const existing = await this.db.getOAuthProvider(userId, provider);
    if (existing) {
      // Update existing link
      await this.db.updateOAuthProvider(userId, provider, {
        provider_user_id: profile.providerId,
        provider_email: profile.email,
        provider_name: profile.name,
        provider_avatar_url: profile.avatarUrl,
        access_token_encrypted: accessTokenEncrypted,
        refresh_token_encrypted: refreshTokenEncrypted,
        token_expires_at: tokens.expiresAt || null,
        scopes: tokens.scopes || [],
        raw_profile: profile.raw,
        last_used_at: new Date(),
      });
      logger.info('OAuth provider updated', { userId, provider });
    } else {
      // Create new link
      await this.db.insertOAuthProvider({
        user_id: userId,
        provider,
        provider_user_id: profile.providerId,
        provider_email: profile.email,
        provider_name: profile.name,
        provider_avatar_url: profile.avatarUrl,
        access_token_encrypted: accessTokenEncrypted,
        refresh_token_encrypted: refreshTokenEncrypted,
        token_expires_at: tokens.expiresAt || null,
        scopes: tokens.scopes || [],
        raw_profile: profile.raw,
      });
      logger.info('OAuth provider linked', { userId, provider });
    }
  }

  /**
   * Unlink OAuth provider from user
   */
  async unlinkProvider(userId: string, provider: OAuthProvider): Promise<void> {
    await this.db.deleteOAuthProvider(userId, provider);
    logger.info('OAuth provider unlinked', { userId, provider });
  }

  /**
   * Get user's OAuth connections
   */
  async getConnections(userId: string): Promise<OAuthProviderRecord[]> {
    return await this.db.getOAuthProvidersByUser(userId);
  }

  // =========================================================================
  // Provider-specific callback handlers
  // =========================================================================

  private async handleGoogleCallback(
    code: string,
    redirectUri: string
  ): Promise<{ profile: OAuthProfile; tokens: OAuthTokens }> {
    if (!this.config.oauth.google) {
      throw new Error('Google OAuth not configured');
    }

    // Exchange code for tokens
    const tokenResponse = await fetch('https://oauth2.googleapis.com/token', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({
        code,
        client_id: this.config.oauth.google.clientId,
        client_secret: this.config.oauth.google.clientSecret,
        redirect_uri: redirectUri,
        grant_type: 'authorization_code',
      }),
    });

    const tokenData = (await tokenResponse.json()) as any;
    if (!tokenResponse.ok) {
      throw new Error(`Google token exchange failed: ${JSON.stringify(tokenData)}`);
    }

    // Get user profile
    const profileResponse = await fetch('https://www.googleapis.com/oauth2/v2/userinfo', {
      headers: { Authorization: `Bearer ${tokenData.access_token}` },
    });

    const profileData = (await profileResponse.json()) as any;
    if (!profileResponse.ok) {
      throw new Error(`Google profile fetch failed: ${JSON.stringify(profileData)}`);
    }

    const profile: OAuthProfile = {
      provider: 'google',
      providerId: profileData.id,
      email: profileData.email || null,
      name: profileData.name || null,
      avatarUrl: profileData.picture || null,
      raw: profileData as Record<string, unknown>,
    };

    const tokens: OAuthTokens = {
      accessToken: tokenData.access_token,
      refreshToken: tokenData.refresh_token,
      expiresAt: tokenData.expires_in
        ? new Date(Date.now() + tokenData.expires_in * 1000)
        : undefined,
      scopes: tokenData.scope?.split(' '),
    };

    return { profile, tokens };
  }

  private async handleAppleCallback(
    code: string,
    redirectUri: string
  ): Promise<{ profile: OAuthProfile; tokens: OAuthTokens }> {
    if (!this.config.oauth.apple) {
      throw new Error('Apple OAuth not configured');
    }

    // Exchange code for tokens
    const tokenResponse = await fetch('https://appleid.apple.com/auth/token', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({
        code,
        client_id: this.config.oauth.apple.clientId,
        client_secret: this.generateAppleClientSecret(),
        redirect_uri: redirectUri,
        grant_type: 'authorization_code',
      }),
    });

    const tokenData = (await tokenResponse.json()) as any;
    if (!tokenResponse.ok) {
      throw new Error(`Apple token exchange failed: ${JSON.stringify(tokenData)}`);
    }

    // Decode ID token to get profile (Apple doesn't have a separate profile endpoint)
    const idToken = tokenData.id_token;
    const payload = this.decodeJWT(idToken);

    const profile: OAuthProfile = {
      provider: 'apple',
      providerId: payload.sub,
      email: payload.email || null,
      name: null, // Apple doesn't always provide name in token
      avatarUrl: null,
      raw: payload,
    };

    const tokens: OAuthTokens = {
      accessToken: tokenData.access_token,
      refreshToken: tokenData.refresh_token,
      expiresAt: tokenData.expires_in
        ? new Date(Date.now() + tokenData.expires_in * 1000)
        : undefined,
    };

    return { profile, tokens };
  }

  private async handleFacebookCallback(
    code: string,
    redirectUri: string
  ): Promise<{ profile: OAuthProfile; tokens: OAuthTokens }> {
    if (!this.config.oauth.facebook) {
      throw new Error('Facebook OAuth not configured');
    }

    // Exchange code for tokens
    const tokenUrl = `https://graph.facebook.com/v12.0/oauth/access_token?client_id=${this.config.oauth.facebook.appId}&client_secret=${this.config.oauth.facebook.appSecret}&redirect_uri=${encodeURIComponent(redirectUri)}&code=${code}`;
    const tokenResp = await fetch(tokenUrl);
    const tokenData = (await tokenResp.json()) as any;

    if (!tokenResp.ok) {
      throw new Error(`Facebook token exchange failed: ${JSON.stringify(tokenData)}`);
    }

    // Get user profile
    const profileResponse = await fetch(
      `https://graph.facebook.com/me?fields=id,name,email,picture.type(large)&access_token=${tokenData.access_token}`
    );

    const profileData = (await profileResponse.json()) as any;
    if (!profileResponse.ok) {
      throw new Error(`Facebook profile fetch failed: ${JSON.stringify(profileData)}`);
    }

    const profile: OAuthProfile = {
      provider: 'facebook',
      providerId: profileData.id,
      email: profileData.email || null,
      name: profileData.name || null,
      avatarUrl: profileData.picture?.data?.url || null,
      raw: profileData as Record<string, unknown>,
    };

    const tokens: OAuthTokens = {
      accessToken: tokenData.access_token,
      expiresAt: tokenData.expires_in
        ? new Date(Date.now() + tokenData.expires_in * 1000)
        : undefined,
    };

    return { profile, tokens };
  }

  private async handleGitHubCallback(
    code: string,
    redirectUri: string
  ): Promise<{ profile: OAuthProfile; tokens: OAuthTokens }> {
    if (!this.config.oauth.github) {
      throw new Error('GitHub OAuth not configured');
    }

    // Exchange code for tokens
    const tokenResponse = await fetch('https://github.com/login/oauth/access_token', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'application/json',
      },
      body: JSON.stringify({
        client_id: this.config.oauth.github.clientId,
        client_secret: this.config.oauth.github.clientSecret,
        code,
        redirect_uri: redirectUri,
      }),
    });

    const tokenData = (await tokenResponse.json()) as any;
    if (!tokenResponse.ok || tokenData.error) {
      throw new Error(`GitHub token exchange failed: ${JSON.stringify(tokenData)}`);
    }

    // Get user profile
    const profileResponse = await fetch('https://api.github.com/user', {
      headers: {
        Authorization: `Bearer ${tokenData.access_token}`,
        Accept: 'application/json',
      },
    });

    const profileData = (await profileResponse.json()) as any;
    if (!profileResponse.ok) {
      throw new Error(`GitHub profile fetch failed: ${JSON.stringify(profileData)}`);
    }

    // Get primary email
    const emailResponse = await fetch('https://api.github.com/user/emails', {
      headers: {
        Authorization: `Bearer ${tokenData.access_token}`,
        Accept: 'application/json',
      },
    });

    const emails = (await emailResponse.json()) as any[];
    const primaryEmail = emails.find((e: any) => e.primary)?.email || null;

    const profile: OAuthProfile = {
      provider: 'github',
      providerId: String(profileData.id),
      email: primaryEmail,
      name: profileData.name || profileData.login || null,
      avatarUrl: profileData.avatar_url || null,
      raw: profileData as Record<string, unknown>,
    };

    const tokens: OAuthTokens = {
      accessToken: tokenData.access_token,
      scopes: tokenData.scope?.split(','),
    };

    return { profile, tokens };
  }

  private async handleMicrosoftCallback(
    code: string,
    redirectUri: string
  ): Promise<{ profile: OAuthProfile; tokens: OAuthTokens }> {
    if (!this.config.oauth.microsoft) {
      throw new Error('Microsoft OAuth not configured');
    }

    // Exchange code for tokens
    const tokenResponse = await fetch(
      'https://login.microsoftonline.com/common/oauth2/v2.0/token',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: new URLSearchParams({
          client_id: this.config.oauth.microsoft.clientId,
          client_secret: this.config.oauth.microsoft.clientSecret,
          code,
          redirect_uri: redirectUri,
          grant_type: 'authorization_code',
          scope: 'openid profile email',
        }),
      }
    );

    const tokenData = (await tokenResponse.json()) as any;
    if (!tokenResponse.ok) {
      throw new Error(`Microsoft token exchange failed: ${JSON.stringify(tokenData)}`);
    }

    // Get user profile
    const profileResponse = await fetch('https://graph.microsoft.com/v1.0/me', {
      headers: { Authorization: `Bearer ${tokenData.access_token}` },
    });

    const profileData = (await profileResponse.json()) as any;
    if (!profileResponse.ok) {
      throw new Error(`Microsoft profile fetch failed: ${JSON.stringify(profileData)}`);
    }

    const profile: OAuthProfile = {
      provider: 'microsoft',
      providerId: profileData.id,
      email: profileData.userPrincipalName || profileData.mail || null,
      name: profileData.displayName || null,
      avatarUrl: null, // Microsoft Graph requires additional permissions for photo
      raw: profileData as Record<string, unknown>,
    };

    const tokens: OAuthTokens = {
      accessToken: tokenData.access_token,
      refreshToken: tokenData.refresh_token,
      expiresAt: tokenData.expires_in
        ? new Date(Date.now() + tokenData.expires_in * 1000)
        : undefined,
      scopes: tokenData.scope?.split(' '),
    };

    return { profile, tokens };
  }

  // =========================================================================
  // Helper methods
  // =========================================================================

  /**
   * Generate random state parameter for OAuth flow
   */
  private generateState(): string {
    return Array.from({ length: 32 }, () =>
      Math.floor(Math.random() * 16).toString(16)
    ).join('');
  }

  /**
   * Generate Apple client secret JWT
   * Required by Apple Sign In
   * Cached for 10 minutes to avoid regenerating on every request
   */
  private generateAppleClientSecret(): string {
    // Check cache
    if (this.appleClientSecret && Date.now() < this.appleClientSecretExpiry) {
      return this.appleClientSecret;
    }

    if (!this.config.oauth.apple) {
      throw new Error('Apple OAuth not configured');
    }

    const { teamId, keyId, privateKey, clientId } = this.config.oauth.apple;

    if (!teamId || !keyId || !privateKey) {
      throw new Error('Apple OAuth missing required fields: teamId, keyId, or privateKey');
    }

    // Generate JWT client secret
    const now = Math.floor(Date.now() / 1000);
    const expiresIn = 600; // 10 minutes (Apple allows up to 6 months, but shorter is more secure)

    const claims = {
      iss: teamId,
      iat: now,
      exp: now + expiresIn,
      aud: 'https://appleid.apple.com',
      sub: clientId,
    };

    // Sign JWT with ES256 algorithm and Apple private key
    const secret = jwt.sign(claims, privateKey, {
      algorithm: 'ES256',
      header: {
        alg: 'ES256',
        kid: keyId,
      },
    } as any);

    // Cache the secret for 9 minutes (slightly less than expiry to ensure validity)
    this.appleClientSecret = secret;
    this.appleClientSecretExpiry = Date.now() + (9 * 60 * 1000);

    logger.debug('Generated Apple client secret', { expiresIn });

    return secret;
  }

  /**
   * Decode JWT token (simplified)
   */
  private decodeJWT(token: string): Record<string, any> {
    try {
      const parts = token.split('.');
      if (parts.length !== 3) {
        throw new Error('Invalid JWT format');
      }
      const payload = Buffer.from(parts[1], 'base64').toString('utf-8');
      return JSON.parse(payload);
    } catch (error) {
      logger.error('JWT decode error', { error });
      throw new Error('Failed to decode JWT');
    }
  }

  /**
   * Check if provider is configured
   */
  isProviderEnabled(provider: OAuthProvider): boolean {
    switch (provider) {
      case 'google':
        return !!this.config.oauth.google;
      case 'apple':
        return !!this.config.oauth.apple;
      case 'facebook':
        return !!this.config.oauth.facebook;
      case 'github':
        return !!this.config.oauth.github;
      case 'microsoft':
        return !!this.config.oauth.microsoft;
      default:
        return false;
    }
  }

  /**
   * Get list of enabled providers
   */
  getEnabledProviders(): Array<{ name: OAuthProvider; displayName: string }> {
    const providers: Array<{ name: OAuthProvider; displayName: string }> = [];

    if (this.config.oauth.google) {
      providers.push({ name: 'google', displayName: 'Google' });
    }
    if (this.config.oauth.apple) {
      providers.push({ name: 'apple', displayName: 'Apple' });
    }
    if (this.config.oauth.facebook) {
      providers.push({ name: 'facebook', displayName: 'Facebook' });
    }
    if (this.config.oauth.github) {
      providers.push({ name: 'github', displayName: 'GitHub' });
    }
    if (this.config.oauth.microsoft) {
      providers.push({ name: 'microsoft', displayName: 'Microsoft' });
    }

    return providers;
  }
}
