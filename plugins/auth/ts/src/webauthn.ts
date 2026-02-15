/**
 * WebAuthn/Passkeys Service
 * Uses @simplewebauthn/server for WebAuthn operations
 */

import {
  generateRegistrationOptions,
  verifyRegistrationResponse,
  generateAuthenticationOptions,
  verifyAuthenticationResponse,
  type VerifiedRegistrationResponse,
  type VerifiedAuthenticationResponse,
  type RegistrationResponseJSON,
  type AuthenticationResponseJSON,
  type PublicKeyCredentialDescriptorFuture,
} from '@simplewebauthn/server';
import { createLogger } from '@nself/plugin-utils';
import type { AuthConfig, PasskeyRow } from './types.js';

const logger = createLogger('auth:webauthn');

export interface WebAuthnRegistrationOptions {
  userId: string;
  userName: string;
  userDisplayName: string;
  excludeCredentials?: PasskeyRow[];
}

export interface WebAuthnAuthenticationOptions {
  userId?: string;
  allowedCredentials?: PasskeyRow[];
}

export interface WebAuthnRegistrationResult {
  verified: boolean;
  registrationInfo?: VerifiedRegistrationResponse['registrationInfo'];
}

export interface WebAuthnAuthenticationResult {
  verified: boolean;
  authenticationInfo?: VerifiedAuthenticationResponse['authenticationInfo'];
}

/**
 * WebAuthn Service
 *
 * Provides passkey registration and authentication using WebAuthn standard.
 */
export class WebAuthnService {
  private config: AuthConfig;
  private rpID: string;
  private rpName: string;
  private origin: string;

  constructor(config: AuthConfig) {
    this.config = config;
    this.rpID = config.webauthn.rpId;
    this.rpName = config.webauthn.rpName;
    this.origin = config.webauthn.origin;

    logger.debug('WebAuthn service initialized', {
      rpID: this.rpID,
      rpName: this.rpName,
      origin: this.origin,
    });
  }

  /**
   * Generate registration options for a new passkey
   *
   * This creates the challenge and options that the browser will use
   * to prompt the user to create a new passkey.
   *
   * @param options - Registration options including user info
   * @returns WebAuthn registration options to send to client
   */
  async generateRegistrationOptions(options: WebAuthnRegistrationOptions) {
    const { userId, userName, userDisplayName, excludeCredentials = [] } = options;

    // Convert existing passkeys to exclude list (prevent re-registering same device)
    const excludeCredentialDescriptors = excludeCredentials.map((passkey) => ({
      id: passkey.credential_id, // Base64 string is acceptable
      transports: passkey.transports ? JSON.parse(passkey.transports) : undefined,
    }));

    const registrationOptions = await generateRegistrationOptions({
      rpName: this.rpName,
      rpID: this.rpID,
      userID: Buffer.from(userId),
      userName,
      userDisplayName,
      timeout: this.config.webauthn.timeout,
      attestationType: 'none', // 'none' for simplicity, 'direct' for device attestation
      excludeCredentials: excludeCredentialDescriptors,
      authenticatorSelection: {
        residentKey: 'preferred', // Prefer discoverable credentials
        userVerification: 'preferred', // Prefer biometrics/PIN
      },
      supportedAlgorithmIDs: [-7, -257], // ES256, RS256
    });

    logger.debug('Generated registration options', {
      userId,
      userName,
      challenge: registrationOptions.challenge,
    });

    return registrationOptions;
  }

  /**
   * Verify registration response from client
   *
   * After the user creates a passkey, this verifies the response
   * and extracts the credential data for storage.
   *
   * @param response - WebAuthn registration response from client
   * @param expectedChallenge - The challenge that was sent to client
   * @returns Verification result with credential data
   */
  async verifyRegistrationResponse(
    response: RegistrationResponseJSON,
    expectedChallenge: string
  ): Promise<WebAuthnRegistrationResult> {
    try {
      const verification = await verifyRegistrationResponse({
        response,
        expectedChallenge,
        expectedOrigin: this.origin,
        expectedRPID: this.rpID,
        requireUserVerification: false, // Allow both UV and non-UV
      });

      if (verification.verified && verification.registrationInfo) {
        logger.info('Registration verified successfully', {
          credentialID: Buffer.from(verification.registrationInfo.credential.id).toString('base64'),
          deviceType: verification.registrationInfo.credentialDeviceType,
          backedUp: verification.registrationInfo.credentialBackedUp,
        });

        return {
          verified: true,
          registrationInfo: verification.registrationInfo,
        };
      }

      logger.warn('Registration verification failed');
      return { verified: false };
    } catch (error) {
      logger.error('Registration verification error', { error });
      return { verified: false };
    }
  }

  /**
   * Generate authentication options for passkey login
   *
   * This creates the challenge and options that the browser will use
   * to prompt the user to authenticate with their passkey.
   *
   * @param options - Authentication options
   * @returns WebAuthn authentication options to send to client
   */
  async generateAuthenticationOptions(options: WebAuthnAuthenticationOptions = {}) {
    const { userId, allowedCredentials = [] } = options;

    // Convert allowed passkeys to credential descriptors
    const allowCredentialDescriptors = allowedCredentials.map((passkey) => ({
      id: passkey.credential_id, // Base64 string is acceptable
      transports: passkey.transports ? JSON.parse(passkey.transports) : undefined,
    }));

    const authenticationOptions = await generateAuthenticationOptions({
      rpID: this.rpID,
      timeout: this.config.webauthn.timeout,
      allowCredentials: allowCredentialDescriptors.length > 0 ? allowCredentialDescriptors : undefined,
      userVerification: 'preferred',
    });

    logger.debug('Generated authentication options', {
      userId,
      challenge: authenticationOptions.challenge,
      allowCredentialsCount: allowCredentialDescriptors.length,
    });

    return authenticationOptions;
  }

  /**
   * Verify authentication response from client
   *
   * After the user authenticates with their passkey, this verifies
   * the response and updates the credential counter.
   *
   * @param response - WebAuthn authentication response from client
   * @param expectedChallenge - The challenge that was sent to client
   * @param passkey - The stored passkey data for verification
   * @returns Verification result with authentication data
   */
  async verifyAuthenticationResponse(
    response: AuthenticationResponseJSON,
    expectedChallenge: string,
    passkey: PasskeyRow
  ): Promise<WebAuthnAuthenticationResult> {
    try {
      // Parse public key and transports
      const credentialPublicKey = Buffer.from(passkey.public_key, 'base64');
      const transports = passkey.transports ? JSON.parse(passkey.transports) : undefined;

      const verification = await verifyAuthenticationResponse({
        response,
        expectedChallenge,
        expectedOrigin: this.origin,
        expectedRPID: this.rpID,
        credential: {
          id: passkey.credential_id, // Base64 string is acceptable
          publicKey: credentialPublicKey,
          counter: passkey.counter,
          transports,
        },
        requireUserVerification: false, // Allow both UV and non-UV
      });

      if (verification.verified && verification.authenticationInfo) {
        logger.info('Authentication verified successfully', {
          credentialID: passkey.credential_id,
          newCounter: verification.authenticationInfo.newCounter,
        });

        return {
          verified: true,
          authenticationInfo: verification.authenticationInfo,
        };
      }

      logger.warn('Authentication verification failed', {
        credentialID: passkey.credential_id,
      });
      return { verified: false };
    } catch (error) {
      logger.error('Authentication verification error', { error });
      return { verified: false };
    }
  }

  /**
   * Helper: Convert credential ID from response to base64
   */
  credentialIdToBase64(credentialId: Uint8Array | string): string {
    if (typeof credentialId === 'string') {
      return credentialId;
    }
    return Buffer.from(credentialId).toString('base64');
  }

  /**
   * Helper: Convert public key to base64
   */
  publicKeyToBase64(publicKey: Uint8Array | string): string {
    if (typeof publicKey === 'string') {
      return publicKey;
    }
    return Buffer.from(publicKey).toString('base64');
  }

  /**
   * Helper: Get human-readable device type
   */
  getDeviceTypeName(deviceType: 'singleDevice' | 'multiDevice'): string {
    return deviceType === 'multiDevice' ? 'Synced Passkey' : 'Device-Bound Passkey';
  }

  /**
   * Helper: Get recommended friendly name based on device type and backup status
   */
  suggestFriendlyName(
    deviceType: 'singleDevice' | 'multiDevice',
    backedUp: boolean,
    userAgent?: string
  ): string {
    // Parse user agent for device info (basic detection)
    let deviceName = 'Unknown Device';
    if (userAgent) {
      if (userAgent.includes('iPhone')) deviceName = 'iPhone';
      else if (userAgent.includes('iPad')) deviceName = 'iPad';
      else if (userAgent.includes('Mac')) deviceName = 'Mac';
      else if (userAgent.includes('Windows')) deviceName = 'Windows PC';
      else if (userAgent.includes('Android')) deviceName = 'Android';
      else if (userAgent.includes('Linux')) deviceName = 'Linux';
    }

    if (deviceType === 'multiDevice') {
      return backedUp ? `${deviceName} (Synced)` : `${deviceName} (Cloud)`;
    }
    return `${deviceName} (Security Key)`;
  }
}
