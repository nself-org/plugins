/**
 * Encryption/Decryption Utilities
 * Using AES-256-GCM for authenticated encryption
 */

import crypto from 'node:crypto';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('auth:crypto');

const ALGORITHM = 'aes-256-gcm';
const IV_LENGTH = 16;
const AUTH_TAG_LENGTH = 16;
const KEY_LENGTH = 32; // 256 bits

/**
 * Derive a 32-byte encryption key from the provided key
 */
function deriveKey(key: string): Buffer {
  // If key is already 32 bytes (64 hex chars), use it directly
  if (key.length === 64 && /^[0-9a-f]+$/i.test(key)) {
    return Buffer.from(key, 'hex');
  }

  // Otherwise, hash it to get 32 bytes
  return crypto.createHash('sha256').update(key).digest();
}

/**
 * Encrypt data using AES-256-GCM
 */
export function encrypt(text: string, encryptionKey: string): string {
  try {
    const key = deriveKey(encryptionKey);
    const iv = crypto.randomBytes(IV_LENGTH);
    const cipher = crypto.createCipheriv(ALGORITHM, key, iv);

    let encrypted = cipher.update(text, 'utf8', 'hex');
    encrypted += cipher.final('hex');

    const authTag = cipher.getAuthTag();

    // Return: iv:authTag:encrypted
    return `${iv.toString('hex')}:${authTag.toString('hex')}:${encrypted}`;
  } catch (error) {
    logger.error('Encryption failed', { error });
    throw new Error('Encryption failed');
  }
}

/**
 * Decrypt data using AES-256-GCM
 */
export function decrypt(encryptedData: string, encryptionKey: string): string {
  try {
    const key = deriveKey(encryptionKey);
    const parts = encryptedData.split(':');

    if (parts.length !== 3) {
      throw new Error('Invalid encrypted data format');
    }

    const [ivHex, authTagHex, encrypted] = parts;
    const iv = Buffer.from(ivHex, 'hex');
    const authTag = Buffer.from(authTagHex, 'hex');

    const decipher = crypto.createDecipheriv(ALGORITHM, key, iv);
    decipher.setAuthTag(authTag);

    let decrypted = decipher.update(encrypted, 'hex', 'utf8');
    decrypted += decipher.final('utf8');

    return decrypted;
  } catch (error) {
    logger.error('Decryption failed', { error });
    throw new Error('Decryption failed');
  }
}

/**
 * Encrypt array of strings (for backup codes)
 */
export function encryptArray(arr: string[], encryptionKey: string): string {
  return encrypt(JSON.stringify(arr), encryptionKey);
}

/**
 * Decrypt array of strings (for backup codes)
 */
export function decryptArray(encryptedData: string, encryptionKey: string): string[] {
  const decrypted = decrypt(encryptedData, encryptionKey);
  return JSON.parse(decrypted);
}
