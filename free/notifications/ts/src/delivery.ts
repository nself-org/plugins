/**
 * Notification Delivery Service
 * Handles actual delivery of notifications via email, push, and SMS
 */

import nodemailer, { Transporter } from 'nodemailer';
import twilio from 'twilio';
import { createLogger } from '@nself/plugin-utils';
import { createSign, createHmac } from 'crypto';
import { config } from './config.js';

const logger = createLogger('notifications:delivery');

// =============================================================================
// Types
// =============================================================================

export interface DeliveryResult {
  success: boolean;
  message_id?: string;
  error?: string;
  provider_response?: unknown;
}

export interface EmailMessage {
  to: string;
  subject: string;
  text?: string;
  html?: string;
  from?: string;
  reply_to?: string;
  attachments?: Array<{
    filename: string;
    content: string | Buffer;
    contentType?: string;
  }>;
}

export interface PushMessage {
  token: string;
  title: string;
  body: string;
  data?: Record<string, string>;
  badge?: number;
  sound?: string;
  image?: string;
}

export interface SMSMessage {
  to: string;
  body: string;
  from?: string;
  media_url?: string;
}

// =============================================================================
// Email Delivery (Fully Implemented)
// =============================================================================

export class EmailDelivery {
  private transporter: Transporter | null = null;
  private defaultFrom: string;

  constructor() {
    this.defaultFrom = config.email.from_address || 'noreply@nself.app';
    this.initializeTransport();
  }

  private initializeTransport(): void {
    const provider = config.email.provider || 'smtp';

    try {
      switch (provider) {
        case 'smtp':
          this.transporter = this.createSMTPTransport();
          logger.info('Email delivery initialized with SMTP');
          break;

        case 'sendgrid':
          this.transporter = this.createSendGridTransport();
          logger.info('Email delivery initialized with SendGrid');
          break;

        case 'mailgun':
          this.transporter = this.createMailgunTransport();
          logger.info('Email delivery initialized with Mailgun');
          break;

        case 'ses':
          this.transporter = this.createSESTransport();
          logger.info('Email delivery initialized with AWS SES');
          break;

        case 'resend':
          this.transporter = this.createResendTransport();
          logger.info('Email delivery initialized with Resend');
          break;

        default:
          logger.warn(`Unknown email provider: ${provider}, falling back to SMTP`);
          this.transporter = this.createSMTPTransport();
      }
    } catch (error) {
      logger.error('Failed to initialize email transport', {
        error: error instanceof Error ? error.message : 'Unknown error',
        provider
      });
      // Fall back to SMTP if provider initialization fails
      this.transporter = this.createSMTPTransport();
    }
  }

  private createSMTPTransport(): Transporter {
    return nodemailer.createTransport({
      host: config.email.smtp_host || 'localhost',
      port: config.email.smtp_port || 587,
      secure: config.email.smtp_secure || false,
      auth: config.email.smtp_user && config.email.smtp_password
        ? {
            user: config.email.smtp_user,
            pass: config.email.smtp_password,
          }
        : undefined,
    });
  }

  private createSendGridTransport(): Transporter {
    if (!config.email.sendgrid_api_key) {
      throw new Error('SendGrid API key not configured');
    }

    return nodemailer.createTransport({
      host: 'smtp.sendgrid.net',
      port: 587,
      auth: {
        user: 'apikey',
        pass: config.email.sendgrid_api_key,
      },
    });
  }

  private createMailgunTransport(): Transporter {
    if (!config.email.mailgun_api_key || !config.email.mailgun_domain) {
      throw new Error('Mailgun API key or domain not configured');
    }

    return nodemailer.createTransport({
      host: 'smtp.mailgun.org',
      port: 587,
      auth: {
        user: `postmaster@${config.email.mailgun_domain}`,
        pass: config.email.mailgun_api_key,
      },
    });
  }

  private createSESTransport(): Transporter {
    if (!config.email.ses_region) {
      throw new Error('AWS SES region not configured');
    }

    // Note: Uses AWS credentials from environment (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)
    return nodemailer.createTransport({
      host: `email-smtp.${config.email.ses_region}.amazonaws.com`,
      port: 587,
      auth: {
        user: process.env.AWS_ACCESS_KEY_ID || '',
        pass: process.env.AWS_SECRET_ACCESS_KEY || '',
      },
    });
  }

  private createResendTransport(): Transporter {
    if (!config.email.resend_api_key) {
      throw new Error('Resend API key not configured');
    }

    return nodemailer.createTransport({
      host: 'smtp.resend.com',
      port: 587,
      auth: {
        user: 'resend',
        pass: config.email.resend_api_key,
      },
    });
  }

  async send(message: EmailMessage): Promise<DeliveryResult> {
    if (!this.transporter) {
      return {
        success: false,
        error: 'Email transport not initialized',
      };
    }

    try {
      const result = await this.transporter.sendMail({
        from: message.from || this.defaultFrom,
        to: message.to,
        subject: message.subject,
        text: message.text,
        html: message.html,
        replyTo: message.reply_to,
        attachments: message.attachments,
      });

      logger.info('Email sent successfully', {
        to: message.to,
        subject: message.subject,
        messageId: result.messageId,
      });

      return {
        success: true,
        message_id: result.messageId,
        provider_response: result,
      };
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to send email', {
        to: message.to,
        subject: message.subject,
        error: errorMessage,
      });

      return {
        success: false,
        error: errorMessage,
      };
    }
  }

  async verify(): Promise<boolean> {
    if (!this.transporter) {
      return false;
    }

    try {
      await this.transporter.verify();
      logger.info('Email transport verification successful');
      return true;
    } catch (error) {
      logger.error('Email transport verification failed', {
        error: error instanceof Error ? error.message : 'Unknown error',
      });
      return false;
    }
  }
}

// =============================================================================
// FCM (Firebase Cloud Messaging) helpers
// =============================================================================

interface FCMServiceAccount {
  project_id: string;
  client_email: string;
  private_key: string;
}

/**
 * Build a short-lived JWT for Firebase service-account OAuth and exchange it
 * for an access token via the Google token endpoint.
 * This avoids the firebase-admin SDK dependency.
 */
async function getFCMAccessToken(serviceAccount: FCMServiceAccount): Promise<string> {
  const now = Math.floor(Date.now() / 1000);
  const header = Buffer.from(JSON.stringify({ alg: 'RS256', typ: 'JWT' })).toString('base64url');
  const payload = Buffer.from(
    JSON.stringify({
      iss: serviceAccount.client_email,
      sub: serviceAccount.client_email,
      aud: 'https://oauth2.googleapis.com/token',
      iat: now,
      exp: now + 3600,
      scope: 'https://www.googleapis.com/auth/firebase.messaging',
    })
  ).toString('base64url');

  const signingInput = `${header}.${payload}`;
  const privateKey = serviceAccount.private_key.replace(/\\n/g, '\n');
  const signer = createSign('RSA-SHA256');
  signer.update(signingInput);
  const signature = signer.sign(privateKey, 'base64url');

  const jwt = `${signingInput}.${signature}`;

  const tokenResponse = await fetch('https://oauth2.googleapis.com/token', {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({
      grant_type: 'urn:ietf:params:oauth:grant-type:jwt-bearer',
      assertion: jwt,
    }),
  });

  if (!tokenResponse.ok) {
    const body = await tokenResponse.text();
    throw new Error(`Failed to obtain FCM access token: ${tokenResponse.status} ${body}`);
  }

  const tokenData = (await tokenResponse.json()) as { access_token: string };
  return tokenData.access_token;
}

/**
 * Send via FCM HTTP v1 API (service-account auth).
 */
async function sendFCMv1(
  serviceAccount: FCMServiceAccount,
  message: PushMessage
): Promise<DeliveryResult> {
  const accessToken = await getFCMAccessToken(serviceAccount);

  const fcmMessage = {
    message: {
      token: message.token,
      notification: {
        title: message.title,
        body: message.body,
        ...(message.image ? { image: message.image } : {}),
      },
      ...(message.data ? { data: message.data } : {}),
      android: {
        notification: {
          sound: message.sound || 'default',
          ...(message.image ? { image: message.image } : {}),
        },
      },
      apns: {
        payload: {
          aps: {
            alert: { title: message.title, body: message.body },
            sound: message.sound || 'default',
            ...(message.badge !== undefined ? { badge: message.badge } : {}),
          },
        },
      },
    },
  };

  const url = `https://fcm.googleapis.com/v1/projects/${serviceAccount.project_id}/messages:send`;

  const response = await fetch(url, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${accessToken}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(fcmMessage),
  });

  const responseBody = (await response.json()) as Record<string, unknown>;

  if (!response.ok) {
    const errDetail =
      (responseBody?.error as { message?: string } | undefined)?.message ||
      `HTTP ${response.status}`;
    return { success: false, error: `FCM v1 error: ${errDetail}`, provider_response: responseBody };
  }

  return {
    success: true,
    message_id: responseBody.name as string | undefined,
    provider_response: responseBody,
  };
}

/**
 * Send via FCM Legacy HTTP API (server-key auth).
 * Used when only FCM_SERVER_KEY is set (no service account).
 */
async function sendFCMLegacy(serverKey: string, message: PushMessage): Promise<DeliveryResult> {
  const payload = {
    to: message.token,
    notification: {
      title: message.title,
      body: message.body,
      sound: message.sound || 'default',
      ...(message.badge !== undefined ? { badge: message.badge } : {}),
      ...(message.image ? { image: message.image } : {}),
    },
    ...(message.data ? { data: message.data } : {}),
  };

  const response = await fetch('https://fcm.googleapis.com/fcm/send', {
    method: 'POST',
    headers: {
      Authorization: `key=${serverKey}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(payload),
  });

  const responseBody = (await response.json()) as Record<string, unknown>;

  if (!response.ok) {
    return {
      success: false,
      error: `FCM legacy error: HTTP ${response.status}`,
      provider_response: responseBody,
    };
  }

  const success = (responseBody.success as number | undefined) === 1;
  const failure = (responseBody.failure as number | undefined) === 1;

  if (failure || !success) {
    const results = responseBody.results as Array<{ error?: string }> | undefined;
    const fcmError = results?.[0]?.error || 'Unknown FCM error';
    return { success: false, error: fcmError, provider_response: responseBody };
  }

  const results = responseBody.results as Array<{ message_id?: string }> | undefined;
  return {
    success: true,
    message_id: results?.[0]?.message_id,
    provider_response: responseBody,
  };
}

// =============================================================================
// APNs (Apple Push Notification service) helpers
// =============================================================================

/**
 * Build a JWT for APNs provider authentication.
 * Uses ES256 (ECDSA with P-256 and SHA-256) — the algorithm required by Apple.
 */
function buildAPNsJWT(keyId: string, teamId: string, privateKeyPem: string): string {
  const now = Math.floor(Date.now() / 1000);
  const header = Buffer.from(JSON.stringify({ alg: 'ES256', kid: keyId })).toString('base64url');
  const payload = Buffer.from(JSON.stringify({ iss: teamId, iat: now })).toString('base64url');

  const signingInput = `${header}.${payload}`;
  const key = privateKeyPem.replace(/\\n/g, '\n');
  const signer = createSign('SHA256');
  signer.update(signingInput);
  // APNs expects the raw 64-byte IEEE P-1363 signature, not DER.
  // Node's createSign with EC key returns DER; convert it.
  const derSig = signer.sign({ key, dsaEncoding: 'ieee-p1363' }, 'base64url');
  return `${signingInput}.${derSig}`;
}

/**
 * Send a push notification via Apple's APNs HTTP/2 provider API.
 * Uses JWT auth (provider token authentication) — no certificates needed.
 */
async function sendAPNsHTTP(
  keyId: string,
  teamId: string,
  privateKeyPem: string,
  bundleId: string,
  deviceToken: string,
  message: PushMessage,
  production: boolean
): Promise<DeliveryResult> {
  const jwt = buildAPNsJWT(keyId, teamId, privateKeyPem);

  const host = production ? 'api.push.apple.com' : 'api.sandbox.push.apple.com';
  const url = `https://${host}/3/device/${deviceToken}`;

  const apsPayload: Record<string, unknown> = {
    alert: {
      title: message.title,
      body: message.body,
    },
    sound: message.sound || 'default',
  };
  if (message.badge !== undefined) {
    apsPayload.badge = message.badge;
  }

  const body: Record<string, unknown> = { aps: apsPayload };
  if (message.data) {
    Object.assign(body, message.data);
  }

  const response = await fetch(url, {
    method: 'POST',
    headers: {
      authorization: `bearer ${jwt}`,
      'apns-push-type': 'alert',
      'apns-topic': bundleId,
      'content-type': 'application/json',
    },
    body: JSON.stringify(body),
  });

  // APNs returns 200 on success; on error the body contains a JSON reason.
  if (response.status === 200) {
    const apnsId = response.headers.get('apns-id') || undefined;
    return { success: true, message_id: apnsId };
  }

  let reason = `HTTP ${response.status}`;
  try {
    const errBody = (await response.json()) as { reason?: string };
    reason = errBody.reason || reason;
  } catch {
    // ignore parse error
  }

  return { success: false, error: `APNs error: ${reason}` };
}

// =============================================================================
// Push Notification Delivery
// =============================================================================

export class PushDelivery {
  private fcmEnabled: boolean = false;
  private apnsEnabled: boolean = false;

  // FCM
  private fcmServiceAccount: FCMServiceAccount | null = null;
  private fcmServerKey: string | null = null;

  // APNs
  private apnsKeyId: string | null = null;
  private apnsPrivateKey: string | null = null;
  private apnsTeamId: string | null = null;
  private apnsBundleId: string | null = null;
  private apnsProduction: boolean = false;

  constructor() {
    this.initializePushProviders();
  }

  private initializePushProviders(): void {
    // FCM (Firebase Cloud Messaging) initialization
    if (config.push.fcm_service_account) {
      try {
        const sa = JSON.parse(config.push.fcm_service_account) as FCMServiceAccount;
        if (sa.project_id && sa.client_email && sa.private_key) {
          this.fcmServiceAccount = sa;
          this.fcmEnabled = true;
          logger.info('FCM push notifications enabled (v1 API / service account)');
        } else {
          throw new Error('Service account JSON is missing required fields');
        }
      } catch (error) {
        logger.error('Failed to parse FCM service account JSON', {
          error: error instanceof Error ? error.message : 'Unknown error',
        });
      }
    }

    if (!this.fcmEnabled && config.push.fcm_server_key) {
      this.fcmServerKey = config.push.fcm_server_key;
      this.fcmEnabled = true;
      logger.info('FCM push notifications enabled (legacy API / server key)');
    }

    // APNs (Apple Push Notification service) initialization
    if (config.push.apns_key_id && config.push.apns_key && config.push.apns_team_id) {
      this.apnsKeyId = config.push.apns_key_id;
      this.apnsPrivateKey = config.push.apns_key;
      this.apnsTeamId = config.push.apns_team_id;
      this.apnsProduction = config.push.apns_production || false;
      // Bundle ID can be set via APNS_BUNDLE_ID env var or falls back to a default
      this.apnsBundleId = process.env.APNS_BUNDLE_ID || process.env.NOTIFICATIONS_APNS_BUNDLE_ID || '';
      this.apnsEnabled = true;
      logger.info('APNs push notifications enabled', {
        keyId: this.apnsKeyId,
        teamId: this.apnsTeamId,
        production: this.apnsProduction,
      });
    }

    if (!this.fcmEnabled && !this.apnsEnabled) {
      logger.warn('No push notification providers configured');
    }
  }

  async send(message: PushMessage): Promise<DeliveryResult> {
    // Determine platform from token format.
    // APNs device tokens are exactly 64 lowercase hex characters.
    // FCM registration tokens are much longer and contain non-hex characters.
    const isAPNs = /^[0-9a-f]{64}$/i.test(message.token);

    if (isAPNs && this.apnsEnabled) {
      return this.sendAPNs(message);
    } else if (this.fcmEnabled) {
      return this.sendFCM(message);
    } else if (isAPNs) {
      return { success: false, error: 'APNs not configured (set APNS_KEY_ID, APNS_KEY, APNS_TEAM_ID)' };
    }

    return {
      success: false,
      error: 'No push notification provider available for this token',
    };
  }

  private async sendFCM(message: PushMessage): Promise<DeliveryResult> {
    try {
      if (this.fcmServiceAccount) {
        logger.info('Sending push via FCM v1 API', { token: message.token.slice(0, 10) + '...' });
        return await sendFCMv1(this.fcmServiceAccount, message);
      }

      if (this.fcmServerKey) {
        logger.info('Sending push via FCM legacy API', { token: message.token.slice(0, 10) + '...' });
        return await sendFCMLegacy(this.fcmServerKey, message);
      }

      return { success: false, error: 'FCM not configured' };
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      logger.error('FCM delivery failed', { error: errorMessage });
      return { success: false, error: errorMessage };
    }
  }

  private async sendAPNs(message: PushMessage): Promise<DeliveryResult> {
    if (!this.apnsKeyId || !this.apnsPrivateKey || !this.apnsTeamId) {
      return { success: false, error: 'APNs not fully configured' };
    }

    if (!this.apnsBundleId) {
      return {
        success: false,
        error: 'APNs bundle ID not configured — set APNS_BUNDLE_ID environment variable',
      };
    }

    try {
      logger.info('Sending push via APNs', { token: message.token.slice(0, 10) + '...' });
      return await sendAPNsHTTP(
        this.apnsKeyId,
        this.apnsTeamId,
        this.apnsPrivateKey,
        this.apnsBundleId,
        message.token,
        message,
        this.apnsProduction
      );
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      logger.error('APNs delivery failed', { error: errorMessage });
      return { success: false, error: errorMessage };
    }
  }
}

// =============================================================================
// SMS Delivery (Implementation-Ready Stub)
// =============================================================================

export class SMSDelivery {
  private twilioEnabled: boolean = false;
  private twilioClient?: twilio.Twilio;

  constructor() {
    this.initializeSMSProvider();
  }

  private initializeSMSProvider(): void {
    if (config.sms.twilio_account_sid && config.sms.twilio_auth_token) {
      this.twilioEnabled = true;
      this.twilioClient = twilio(
        config.sms.twilio_account_sid,
        config.sms.twilio_auth_token
      );
      logger.info('Twilio SMS enabled');
    } else {
      logger.warn('No SMS provider configured');
    }
  }

  async send(message: SMSMessage): Promise<DeliveryResult> {
    if (!this.twilioEnabled || !this.twilioClient) {
      return {
        success: false,
        error: 'SMS provider not configured',
      };
    }

    try {
      logger.info('Sending SMS via Twilio', { to: message.to });

      const result = await this.twilioClient.messages.create({
        body: message.body,
        from: message.from || config.sms.twilio_from_number,
        to: message.to,
        mediaUrl: message.media_url ? [message.media_url] : undefined,
      });

      logger.info('SMS sent successfully', {
        to: message.to,
        sid: result.sid,
        status: result.status,
      });

      return {
        success: true,
        message_id: result.sid,
        provider_response: result,
      };
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to send SMS', {
        to: message.to,
        error: errorMessage,
      });

      return {
        success: false,
        error: errorMessage,
      };
    }
  }
}

// =============================================================================
// Unified Delivery Manager
// =============================================================================

export class DeliveryManager {
  private emailDelivery: EmailDelivery;
  private pushDelivery: PushDelivery;
  private smsDelivery: SMSDelivery;

  constructor() {
    this.emailDelivery = new EmailDelivery();
    this.pushDelivery = new PushDelivery();
    this.smsDelivery = new SMSDelivery();
  }

  async sendEmail(message: EmailMessage): Promise<DeliveryResult> {
    return this.emailDelivery.send(message);
  }

  async sendPush(message: PushMessage): Promise<DeliveryResult> {
    return this.pushDelivery.send(message);
  }

  async sendSMS(message: SMSMessage): Promise<DeliveryResult> {
    return this.smsDelivery.send(message);
  }

  async verifyEmail(): Promise<boolean> {
    return this.emailDelivery.verify();
  }
}

// Export singleton instance
export const deliveryManager = new DeliveryManager();
