/**
 * Email Integration Service
 * Sends emails via the notifications plugin
 */

import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('auth:email');

export interface EmailConfig {
  notificationsUrl: string;
  fromEmail: string;
  fromName: string;
}

export interface SendEmailOptions {
  to: string;
  subject: string;
  bodyText: string;
  bodyHtml: string;
  userId?: string;
  category?: string;
  metadata?: Record<string, unknown>;
}

export class EmailService {
  private config: EmailConfig;

  constructor(config: EmailConfig) {
    this.config = config;
  }

  /**
   * Send an email via notifications plugin
   */
  async sendEmail(options: SendEmailOptions): Promise<{ success: boolean; notificationId?: string; error?: string }> {
    try {
      const response = await fetch(`${this.config.notificationsUrl}/api/notifications/send`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          user_id: options.userId || 'system',
          channel: 'email',
          category: options.category || 'transactional',
          to: {
            email: options.to,
          },
          content: {
            subject: options.subject,
            body: options.bodyText,
            html: options.bodyHtml,
          },
          metadata: {
            ...options.metadata,
            from_service: 'auth',
            from_email: this.config.fromEmail,
            from_name: this.config.fromName,
          },
        }),
      });

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`HTTP ${response.status}: ${errorText}`);
      }

      const result = await response.json() as any;

      if (result.success) {
        logger.info('Email sent successfully', {
          to: options.to,
          subject: options.subject,
          notificationId: result.notification_id,
        });

        return {
          success: true,
          notificationId: result.notification_id,
        };
      } else {
        throw new Error(result.error || 'Unknown error');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Email sending failed', {
        error: message,
        to: options.to,
        subject: options.subject,
      });

      return {
        success: false,
        error: message,
      };
    }
  }

  /**
   * Send password reset email
   */
  async sendPasswordResetEmail(email: string, resetUrl: string, userId?: string): Promise<boolean> {
    const result = await this.sendEmail({
      to: email,
      subject: 'Password Reset Request',
      bodyText: `You requested a password reset. Click here to reset: ${resetUrl}\n\nIf you didn't request this, please ignore this email.\n\nThis link expires in 1 hour.`,
      bodyHtml: `
        <div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
          <h2>Password Reset Request</h2>
          <p>You requested a password reset for your account.</p>
          <p>Click the button below to reset your password:</p>
          <p style="margin: 30px 0;">
            <a href="${resetUrl}" style="background-color: #007bff; color: white; padding: 12px 24px; text-decoration: none; border-radius: 4px; display: inline-block;">
              Reset Password
            </a>
          </p>
          <p>Or copy and paste this link:</p>
          <p><a href="${resetUrl}">${resetUrl}</a></p>
          <hr style="margin: 30px 0; border: none; border-top: 1px solid #ddd;">
          <p style="color: #666; font-size: 12px;">
            If you didn't request this password reset, please ignore this email.
            This link will expire in 1 hour.
          </p>
        </div>
      `,
      userId,
      category: 'security',
      metadata: {
        type: 'password_reset',
      },
    });

    return result.success;
  }

  /**
   * Send magic link email
   */
  async sendMagicLinkEmail(email: string, magicLinkUrl: string, userId?: string, purpose?: string): Promise<boolean> {
    const purposeText = purpose === 'login' ? 'Sign In' : 'Verify Email';

    const result = await this.sendEmail({
      to: email,
      subject: `${purposeText} - Magic Link`,
      bodyText: `Click here to ${purpose === 'login' ? 'sign in' : 'verify your email'}: ${magicLinkUrl}\n\nIf you didn't request this, please ignore this email.\n\nThis link expires in 10 minutes.`,
      bodyHtml: `
        <div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
          <h2>${purposeText}</h2>
          <p>Click the button below to ${purpose === 'login' ? 'sign in to your account' : 'verify your email'}:</p>
          <p style="margin: 30px 0;">
            <a href="${magicLinkUrl}" style="background-color: #28a745; color: white; padding: 12px 24px; text-decoration: none; border-radius: 4px; display: inline-block;">
              ${purposeText}
            </a>
          </p>
          <p>Or copy and paste this link:</p>
          <p><a href="${magicLinkUrl}">${magicLinkUrl}</a></p>
          <hr style="margin: 30px 0; border: none; border-top: 1px solid #ddd;">
          <p style="color: #666; font-size: 12px;">
            If you didn't request this, please ignore this email.
            This link will expire in 10 minutes.
          </p>
        </div>
      `,
      userId,
      category: 'auth',
      metadata: {
        type: 'magic_link',
        purpose,
      },
    });

    return result.success;
  }

  /**
   * Send verification email
   */
  async sendVerificationEmail(email: string, verificationUrl: string, userId?: string): Promise<boolean> {
    const result = await this.sendEmail({
      to: email,
      subject: 'Verify Your Email Address',
      bodyText: `Please verify your email address by clicking: ${verificationUrl}\n\nThis link expires in 24 hours.`,
      bodyHtml: `
        <div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
          <h2>Verify Your Email</h2>
          <p>Please verify your email address to complete your registration.</p>
          <p style="margin: 30px 0;">
            <a href="${verificationUrl}" style="background-color: #007bff; color: white; padding: 12px 24px; text-decoration: none; border-radius: 4px; display: inline-block;">
              Verify Email
            </a>
          </p>
          <p>Or copy and paste this link:</p>
          <p><a href="${verificationUrl}">${verificationUrl}</a></p>
          <hr style="margin: 30px 0; border: none; border-top: 1px solid #ddd;">
          <p style="color: #666; font-size: 12px;">
            This link will expire in 24 hours.
          </p>
        </div>
      `,
      userId,
      category: 'auth',
      metadata: {
        type: 'email_verification',
      },
    });

    return result.success;
  }
}
