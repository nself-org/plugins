/**
 * Notification Delivery Service
 * Handles actual delivery of notifications via email, push, and SMS
 */

import nodemailer, { Transporter } from 'nodemailer';
import twilio from 'twilio';
import { createLogger } from '@nself/plugin-utils';
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
// Push Notification Delivery (Implementation-Ready Stub)
// =============================================================================

export class PushDelivery {
  private fcmEnabled: boolean = false;
  private apnsEnabled: boolean = false;

  constructor() {
    this.initializePushProviders();
  }

  private initializePushProviders(): void {
    // FCM (Firebase Cloud Messaging) initialization
    if (config.push.fcm_server_key) {
      this.fcmEnabled = true;
      logger.info('FCM push notifications enabled');
      // TODO: Initialize Firebase Admin SDK
      // import { initializeApp, credential } from 'firebase-admin/app';
      // import { getMessaging } from 'firebase-admin/messaging';
      //
      // const app = initializeApp({
      //   credential: credential.cert(config.push.fcm_service_account),
      // });
      // this.fcmMessaging = getMessaging(app);
    }

    // APNs (Apple Push Notification service) initialization
    if (config.push.apns_key_id) {
      this.apnsEnabled = true;
      logger.info('APNs push notifications enabled');
      // TODO: Initialize APNs provider
      // import apn from 'apn';
      //
      // this.apnsProvider = new apn.Provider({
      //   token: {
      //     key: config.push.apns_key,
      //     keyId: config.push.apns_key_id,
      //     teamId: config.push.apns_team_id,
      //   },
      //   production: config.push.apns_production,
      // });
    }

    if (!this.fcmEnabled && !this.apnsEnabled) {
      logger.warn('No push notification providers configured');
    }
  }

  async send(message: PushMessage): Promise<DeliveryResult> {
    // Determine platform from token format
    // FCM tokens are typically longer (152+ chars)
    // APNs tokens are 64 hex characters
    const isAPNs = /^[0-9a-f]{64}$/i.test(message.token);

    if (isAPNs && this.apnsEnabled) {
      return this.sendAPNs(message);
    } else if (this.fcmEnabled) {
      return this.sendFCM(message);
    }

    return {
      success: false,
      error: 'No push notification provider available',
    };
  }

  private async sendFCM(message: PushMessage): Promise<DeliveryResult> {
    logger.warn('FCM delivery not yet implemented', { token: message.token });

    // TODO: Implement FCM delivery
    // const fcmMessage = {
    //   token: message.token,
    //   notification: {
    //     title: message.title,
    //     body: message.body,
    //     imageUrl: message.image,
    //   },
    //   data: message.data,
    //   android: {
    //     notification: {
    //       sound: message.sound || 'default',
    //     },
    //   },
    // };
    //
    // try {
    //   const response = await this.fcmMessaging.send(fcmMessage);
    //   return {
    //     success: true,
    //     message_id: response,
    //   };
    // } catch (error) {
    //   return {
    //     success: false,
    //     error: error.message,
    //   };
    // }

    return {
      success: false,
      error: 'FCM delivery not implemented - install firebase-admin and uncomment code',
    };
  }

  private async sendAPNs(message: PushMessage): Promise<DeliveryResult> {
    logger.warn('APNs delivery not yet implemented', { token: message.token });

    // TODO: Implement APNs delivery
    // const notification = new apn.Notification({
    //   alert: {
    //     title: message.title,
    //     body: message.body,
    //   },
    //   badge: message.badge,
    //   sound: message.sound || 'default',
    //   payload: message.data,
    // });
    //
    // try {
    //   const result = await this.apnsProvider.send(notification, message.token);
    //   if (result.failed.length > 0) {
    //     return {
    //       success: false,
    //       error: result.failed[0].response.reason,
    //     };
    //   }
    //   return {
    //     success: true,
    //     message_id: result.sent[0].device,
    //   };
    // } catch (error) {
    //   return {
    //     success: false,
    //     error: error.message,
    //   };
    // }

    return {
      success: false,
      error: 'APNs delivery not implemented - install apn and uncomment code',
    };
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
