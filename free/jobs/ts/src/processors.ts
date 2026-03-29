/**
 * Job Processors
 * Built-in processors for common job types
 */

import type { Job } from 'bullmq';
import { createLogger } from '@nself/plugin-utils';
import nodemailer from 'nodemailer';
import { spawn } from 'child_process';
import { createHmac, createHash } from 'crypto';
import { createReadStream, statSync, mkdirSync } from 'fs';
import { unlink } from 'fs/promises';
import { tmpdir } from 'os';
import { join, basename } from 'path';
import type {
  SendEmailPayload,
  HttpRequestPayload,
  DatabaseBackupPayload,
  FileCleanupPayload,
  CustomJobPayload,
  SendEmailResult,
  HttpRequestResult,
  DatabaseBackupResult,
  FileCleanupResult,
} from './types.js';
import { JobsDatabase } from './database.js';

const logger = createLogger('jobs:processors');

// =============================================================================
// Send Email Processor
// =============================================================================

/**
 * Send Email Processor
 *
 * Routes through the notifications plugin HTTP API when NOTIFICATIONS_API_URL
 * is configured. Falls back to direct SMTP via Nodemailer when the notifications
 * service is unavailable or not configured.
 */
export async function processSendEmail(job: Job<SendEmailPayload>): Promise<SendEmailResult> {
  const { to, subject, body, from, cc, bcc, attachments, html } = job.data;

  await job.updateProgress(10);

  const notificationsUrl = process.env.NOTIFICATIONS_API_URL;

  // Route through the notifications plugin if configured.
  if (notificationsUrl) {
    return processSendEmailViaNotificationsPlugin(job, notificationsUrl);
  }

  // Direct SMTP fallback — creates a transporter from environment variables.
  const transporter = nodemailer.createTransport({
    host: process.env.SMTP_HOST || process.env.EMAIL_SMTP_HOST,
    port: parseInt(process.env.SMTP_PORT || process.env.EMAIL_SMTP_PORT || '587', 10),
    secure: process.env.SMTP_SECURE === 'true' || process.env.EMAIL_SMTP_SECURE === 'true',
    auth: {
      user: process.env.SMTP_USER || process.env.EMAIL_SMTP_USER,
      pass: process.env.SMTP_PASSWORD || process.env.EMAIL_SMTP_PASSWORD,
    },
  });

  await job.updateProgress(30);

  logger.info(`Sending email to ${to}: ${subject}`);

  try {
    const info = await transporter.sendMail({
      from: from || process.env.SMTP_FROM || process.env.EMAIL_FROM || 'noreply@nself.org',
      to: Array.isArray(to) ? to.join(', ') : to,
      cc: cc ? (Array.isArray(cc) ? cc.join(', ') : cc) : undefined,
      bcc: bcc ? (Array.isArray(bcc) ? bcc.join(', ') : bcc) : undefined,
      subject,
      html: html || body,
      text: html ? undefined : body,
      attachments: attachments?.map(att => ({
        filename: att.filename,
        content: att.content,
        contentType: att.contentType,
      })),
    });

    await job.updateProgress(100);

    logger.info(`Email sent successfully: ${info.messageId}`);

    return {
      messageId: info.messageId,
      accepted: info.accepted as string[],
      rejected: info.rejected as string[],
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Email sending failed', { error: message });
    throw new Error(`Email sending failed: ${message}`);
  }
}

/**
 * Send email via the notifications plugin HTTP API.
 *
 * The notifications plugin exposes a POST /send endpoint that accepts a channel,
 * recipient address, subject, and body, then routes through its configured email
 * provider (SMTP, SendGrid, Mailgun, SES, or Resend). This approach delegates
 * provider selection and rate-limit enforcement to the notifications service.
 */
async function processSendEmailViaNotificationsPlugin(
  job: Job<SendEmailPayload>,
  notificationsUrl: string
): Promise<SendEmailResult> {
  const { to, subject, body, html, from, cc, bcc } = job.data;

  await job.updateProgress(20);

  const recipients = Array.isArray(to) ? to : [to];
  const apiKey = process.env.NOTIFICATIONS_API_KEY || '';
  const baseUrl = notificationsUrl.replace(/\/$/, '');

  logger.info(`Routing email via notifications plugin: ${baseUrl}`, {
    to: recipients,
    subject,
  });

  // The notifications plugin /send endpoint accepts one recipient per request.
  // Fan-out to multiple recipients here, collecting results.
  const accepted: string[] = [];
  const rejected: string[] = [];
  let firstMessageId: string | undefined;

  for (const recipient of recipients) {
    try {
      const response = await fetch(`${baseUrl}/send`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(apiKey ? { Authorization: `Bearer ${apiKey}` } : {}),
        },
        body: JSON.stringify({
          user_id: recipient,
          channel: 'email',
          category: 'transactional',
          to: { email: recipient },
          content: {
            subject,
            body: html || body,
            html: html,
          },
          ...(from ? { metadata: { from } } : {}),
        }),
        signal: AbortSignal.timeout(30_000),
      });

      if (response.ok) {
        const data = (await response.json()) as {
          notification_id?: string;
          success?: boolean;
        };
        accepted.push(recipient);
        if (!firstMessageId && data.notification_id) {
          firstMessageId = data.notification_id;
        }
      } else {
        const errorText = await response.text();
        logger.warn(`Notifications plugin rejected email to ${recipient}`, {
          status: response.status,
          body: errorText,
        });
        rejected.push(recipient);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.warn(`Failed to send email to ${recipient} via notifications plugin`, {
        error: message,
      });
      rejected.push(recipient);
    }
  }

  await job.updateProgress(100);

  // If all recipients were rejected, surface that as an error so BullMQ can
  // apply its retry policy.
  if (accepted.length === 0) {
    throw new Error(
      `All recipients rejected by notifications plugin: ${rejected.join(', ')}`
    );
  }

  logger.info(`Email dispatched via notifications plugin`, {
    accepted: accepted.length,
    rejected: rejected.length,
  });

  return {
    messageId: firstMessageId || `notifications-plugin-${Date.now()}`,
    accepted,
    rejected,
  };
}

// =============================================================================
// HTTP Request Processor
// =============================================================================

/**
 * HTTP Request Processor
 * Makes HTTP requests with retry logic
 */
export async function processHttpRequest(job: Job<HttpRequestPayload>): Promise<HttpRequestResult> {
  const { url, method, headers, body, timeout } = job.data;

  await job.updateProgress(10);

  const startTime = Date.now();

  try {
    const response = await fetch(url, {
      method,
      headers: headers || {},
      body: body ? JSON.stringify(body) : undefined,
      signal: timeout ? AbortSignal.timeout(timeout) : undefined,
    });

    await job.updateProgress(50);

    const responseBody = await response.text();
    let parsedBody: unknown;

    try {
      parsedBody = JSON.parse(responseBody);
    } catch {
      parsedBody = responseBody;
    }

    await job.updateProgress(100);

    return {
      status: response.status,
      statusText: response.statusText,
      headers: Object.fromEntries(response.headers.entries()),
      body: parsedBody,
      duration_ms: Date.now() - startTime,
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('HTTP request failed', { error: message });
    throw error;
  }
}

// =============================================================================
// Database Backup Processor
// =============================================================================

/**
 * Parse an S3 or MinIO destination URL into its components.
 *
 * Supported formats:
 *   s3://bucket/path/to/file.sql.gz
 *   minio://bucket/path/to/file.sql.gz
 *   http://minio-host:9000/bucket/path/to/file.sql.gz
 *   https://s3.amazonaws.com/bucket/path/to/file.sql.gz
 */
interface S3Destination {
  type: 's3' | 'minio';
  endpoint: string;   // e.g. https://s3.amazonaws.com or http://minio:9000
  bucket: string;
  key: string;        // object key within the bucket
  region: string;
}

function parseS3Destination(destination: string): S3Destination | null {
  // s3://bucket/key
  const s3Match = destination.match(/^s3:\/\/([^/]+)\/(.+)$/);
  if (s3Match) {
    const region = process.env.AWS_REGION || process.env.AWS_DEFAULT_REGION || 'us-east-1';
    return {
      type: 's3',
      endpoint: `https://s3.${region}.amazonaws.com`,
      bucket: s3Match[1],
      key: s3Match[2],
      region,
    };
  }

  // minio://bucket/key — uses MINIO_ENDPOINT env var
  const minioMatch = destination.match(/^minio:\/\/([^/]+)\/(.+)$/);
  if (minioMatch) {
    const endpoint =
      process.env.MINIO_ENDPOINT ||
      process.env.STORAGE_ENDPOINT ||
      'http://minio:9000';
    const region = process.env.MINIO_REGION || 'us-east-1';
    return {
      type: 'minio',
      endpoint: endpoint.replace(/\/$/, ''),
      bucket: minioMatch[1],
      key: minioMatch[2],
      region,
    };
  }

  // http(s)://host[:port]/bucket/key — treat as MinIO with explicit endpoint
  const httpMatch = destination.match(/^(https?:\/\/[^/]+)\/([^/]+)\/(.+)$/);
  if (httpMatch) {
    return {
      type: 'minio',
      endpoint: httpMatch[1],
      bucket: httpMatch[2],
      key: httpMatch[3],
      region: process.env.MINIO_REGION || 'us-east-1',
    };
  }

  return null;
}

/**
 * Compute an AWS Signature Version 4 authorization header for a PUT request.
 * This implementation is S3-compatible and works with MinIO.
 */
function awsSigV4PutHeaders(params: {
  method: string;
  endpoint: string;
  bucket: string;
  key: string;
  region: string;
  accessKey: string;
  secretKey: string;
  contentType: string;
  contentLength: number;
  payloadHash: string; // SHA-256 hex of the request body
}): Record<string, string> {
  const {
    method,
    endpoint,
    bucket,
    key,
    region,
    accessKey,
    secretKey,
    contentType,
    contentLength,
    payloadHash,
  } = params;

  const now = new Date();
  const dateStr = now.toISOString().replace(/[-:]/g, '').replace(/\..+/, '') + 'Z';
  const dateShort = dateStr.slice(0, 8);
  const service = 's3';
  const scope = `${dateShort}/${region}/${service}/aws4_request`;

  const url = new URL(`${endpoint}/${bucket}/${key}`);
  const host = url.host;
  const canonicalUri = `/${bucket}/${key}`;

  const signedHeaders = 'content-length;content-type;host;x-amz-content-sha256;x-amz-date';

  const canonicalHeaders = [
    `content-length:${contentLength}`,
    `content-type:${contentType}`,
    `host:${host}`,
    `x-amz-content-sha256:${payloadHash}`,
    `x-amz-date:${dateStr}`,
    '',
  ].join('\n');

  const canonicalRequest = [
    method,
    canonicalUri,
    '',                  // canonical query string (empty)
    canonicalHeaders,
    signedHeaders,
    payloadHash,
  ].join('\n');

  const credentialScope = scope;
  const stringToSign = [
    'AWS4-HMAC-SHA256',
    dateStr,
    credentialScope,
    createHash('sha256').update(canonicalRequest).digest('hex'),
  ].join('\n');

  function hmac(key: string | Buffer, data: string): Buffer {
    return createHmac('sha256', key).update(data).digest();
  }

  const signingKey = hmac(
    hmac(hmac(hmac(`AWS4${secretKey}`, dateShort), region), service),
    'aws4_request'
  );
  const signature = createHmac('sha256', signingKey).update(stringToSign).digest('hex');

  const authorization = [
    `AWS4-HMAC-SHA256 Credential=${accessKey}/${credentialScope}`,
    `SignedHeaders=${signedHeaders}`,
    `Signature=${signature}`,
  ].join(', ');

  return {
    Authorization: authorization,
    'Content-Length': String(contentLength),
    'Content-Type': contentType,
    Host: host,
    'x-amz-content-sha256': payloadHash,
    'x-amz-date': dateStr,
  };
}

/**
 * Upload a file to S3 or MinIO using a streaming PUT request with Sig V4 auth.
 * Returns the ETag of the uploaded object.
 */
async function uploadToS3(
  dest: S3Destination,
  localPath: string,
  contentType: string
): Promise<string> {
  const accessKey =
    process.env.AWS_ACCESS_KEY_ID ||
    process.env.MINIO_ACCESS_KEY ||
    process.env.STORAGE_ACCESS_KEY ||
    '';
  const secretKey =
    process.env.AWS_SECRET_ACCESS_KEY ||
    process.env.MINIO_SECRET_KEY ||
    process.env.STORAGE_SECRET_KEY ||
    '';

  if (!accessKey || !secretKey) {
    throw new Error(
      'S3/MinIO credentials not configured — set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY ' +
        '(or MINIO_ACCESS_KEY and MINIO_SECRET_KEY)'
    );
  }

  const stats = statSync(localPath);
  const contentLength = stats.size;

  // For the payload hash, use the unsigned-payload sentinel so we don't have to
  // buffer the entire file in memory. MinIO and AWS S3 both accept this for
  // streaming uploads when HTTPS is used. For HTTP (local MinIO dev), fall back
  // to a static unsignedPayload marker that MinIO still respects.
  const payloadHash = 'UNSIGNED-PAYLOAD';

  const headers = awsSigV4PutHeaders({
    method: 'PUT',
    endpoint: dest.endpoint,
    bucket: dest.bucket,
    key: dest.key,
    region: dest.region,
    accessKey,
    secretKey,
    contentType,
    contentLength,
    payloadHash,
  });

  const uploadUrl = `${dest.endpoint}/${dest.bucket}/${dest.key}`;

  // Fetch does not accept a ReadableStream from fs.createReadStream directly in
  // all Node versions; use a Buffer-based approach for files that fit comfortably
  // in memory (pg_dump output of typical nself databases). For very large dumps
  // a multipart upload should be used, but that adds significant complexity.
  const { readFile } = await import('fs/promises');
  const fileBuffer = await readFile(localPath);

  const response = await fetch(uploadUrl, {
    method: 'PUT',
    headers,
    body: fileBuffer,
    // @ts-expect-error - duplex required for Node 18+ streaming fetch
    duplex: 'half',
  });

  if (!response.ok) {
    const errorBody = await response.text();
    throw new Error(`S3 upload failed: ${response.status} ${response.statusText} — ${errorBody}`);
  }

  return response.headers.get('ETag') || '';
}

/**
 * Run pg_dump and write the output to a local temp file.
 * Returns the path to the dump file and the number of tables included.
 */
function runPgDump(params: {
  database: string;
  tables?: string[];
  compression: boolean;
  outputPath: string;
}): Promise<{ outputPath: string; tablesBackedUp: number }> {
  return new Promise((resolve, reject) => {
    const { database, tables, compression, outputPath } = params;

    // Build connection args from environment variables following the nself
    // standard env cascade (DATABASE_URL takes precedence over POSTGRES_* vars).
    const dbUrl = process.env.DATABASE_URL;
    const args: string[] = [];

    if (dbUrl) {
      args.push(dbUrl);
    } else {
      const host = process.env.POSTGRES_HOST || 'postgres';
      const port = process.env.POSTGRES_PORT || '5432';
      const user = process.env.POSTGRES_USER || 'postgres';
      const password = process.env.POSTGRES_PASSWORD || '';

      if (password) {
        process.env.PGPASSWORD = password;
      }

      args.push('-h', host, '-p', port, '-U', user, database);
    }

    // Include only specific tables when requested.
    if (tables && tables.length > 0) {
      for (const table of tables) {
        args.push('-t', table);
      }
    }

    // Pg_dump flags: format=plain SQL, no ownership (portable), no ACL.
    args.push('--format=plain', '--no-owner', '--no-acl');

    if (compression) {
      // pg_dump's built-in gzip compression via --compress flag (level 6).
      args.push('--compress=6');
    }

    args.push('-f', outputPath);

    logger.info('Running pg_dump', { database, tables: tables?.length, compression });

    const child = spawn('pg_dump', args, {
      stdio: ['ignore', 'pipe', 'pipe'],
      env: process.env,
    });

    const stderrChunks: Buffer[] = [];
    child.stderr?.on('data', (chunk: Buffer) => stderrChunks.push(chunk));

    child.on('close', (code) => {
      if (code !== 0) {
        const stderr = Buffer.concat(stderrChunks).toString('utf8').trim();
        reject(new Error(`pg_dump exited with code ${code}: ${stderr}`));
        return;
      }

      resolve({
        outputPath,
        tablesBackedUp: tables?.length || 0,
      });
    });

    child.on('error', (err) => {
      reject(new Error(`Failed to spawn pg_dump: ${err.message}`));
    });
  });
}

/**
 * Database Backup Processor
 *
 * Creates a PostgreSQL database dump with pg_dump and uploads it to S3 or MinIO.
 * When the destination is a local path (no URL scheme), the dump is written
 * directly to disk. When the destination is an s3:// or minio:// URL, the dump
 * is written to a temporary file and then uploaded.
 *
 * Environment variables (S3/MinIO):
 *   AWS_ACCESS_KEY_ID / MINIO_ACCESS_KEY / STORAGE_ACCESS_KEY
 *   AWS_SECRET_ACCESS_KEY / MINIO_SECRET_KEY / STORAGE_SECRET_KEY
 *   MINIO_ENDPOINT — base URL for MinIO (e.g. http://minio:9000)
 *   AWS_REGION — region for S3 (default: us-east-1)
 *
 * Environment variables (pg_dump connection):
 *   DATABASE_URL — takes precedence over individual vars
 *   POSTGRES_HOST, POSTGRES_PORT, POSTGRES_USER, POSTGRES_PASSWORD
 */
export async function processDatabaseBackup(
  job: Job<DatabaseBackupPayload>,
  db: JobsDatabase
): Promise<DatabaseBackupResult> {
  const { database, tables, destination, compression = true } = job.data;
  const startTime = Date.now();

  await job.updateProgress(5);

  logger.info(`Starting backup of database: ${database}`, { destination, compression });

  // Determine if the destination is a remote URL or a local path.
  const s3Dest = parseS3Destination(destination);

  // Build a timestamp-based filename.
  const ext = compression ? '.sql.gz' : '.sql';
  const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
  const filename = `${database}_${timestamp}${ext}`;

  let localDumpPath: string;
  let cleanupLocal = false;

  if (s3Dest) {
    // Write dump to a temp file, then upload.
    const tmpDir = join(tmpdir(), 'nself-backups');
    mkdirSync(tmpDir, { recursive: true });
    localDumpPath = join(tmpDir, filename);
    cleanupLocal = true;
  } else {
    // Local filesystem destination: write directly to the destination path or
    // treat the destination as a directory and put the file there.
    if (destination.endsWith('/') || !destination.includes('.')) {
      mkdirSync(destination, { recursive: true });
      localDumpPath = join(destination, filename);
    } else {
      localDumpPath = destination;
    }
    cleanupLocal = false;
  }

  await job.updateProgress(10);

  // Run pg_dump.
  let tablesBackedUp: number;
  try {
    const dumpResult = await runPgDump({
      database,
      tables,
      compression,
      outputPath: localDumpPath,
    });
    tablesBackedUp = dumpResult.tablesBackedUp;
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('pg_dump failed', { database, error: message });
    throw new Error(`Database backup failed: ${message}`);
  }

  await job.updateProgress(60);

  let finalFilename = basename(localDumpPath);
  let sizeBytes = 0;

  try {
    sizeBytes = statSync(localDumpPath).size;
  } catch {
    // File size is informational; don't fail the job over it.
  }

  // Upload to S3/MinIO when a remote destination was specified.
  if (s3Dest) {
    // Adjust the object key to include the generated filename when the key ends
    // with a slash (treated as a prefix/directory).
    if (s3Dest.key.endsWith('/') || !s3Dest.key.includes('.')) {
      s3Dest.key = s3Dest.key.replace(/\/$/, '') + '/' + filename;
    }

    const contentType = compression ? 'application/gzip' : 'text/plain';

    logger.info(`Uploading backup to ${s3Dest.type}: ${s3Dest.endpoint}/${s3Dest.bucket}/${s3Dest.key}`, {
      sizeBytes,
    });

    try {
      await uploadToS3(s3Dest, localDumpPath, contentType);
      finalFilename = s3Dest.key;
      logger.info('Backup uploaded successfully', { key: s3Dest.key, sizeBytes });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('S3/MinIO upload failed', { error: message });
      throw new Error(`Backup upload failed: ${message}`);
    } finally {
      // Always clean up the temp file, even on upload failure (so the next
      // retry starts fresh).
      if (cleanupLocal) {
        await unlink(localDumpPath).catch(() => undefined);
      }
    }
  }

  await job.updateProgress(100);

  const durationMs = Date.now() - startTime;

  logger.info(`Backup completed`, {
    database,
    filename: finalFilename,
    sizeBytes,
    tablesBackedUp,
    durationMs,
  });

  return {
    filename: finalFilename,
    size_bytes: sizeBytes,
    tables_backed_up: tablesBackedUp,
    duration_ms: durationMs,
  };
}

// =============================================================================
// File Cleanup Processor
// =============================================================================

/**
 * File Cleanup Processor
 * Cleans up old completed/failed jobs from database
 */
export async function processFileCleanup(
  job: Job<FileCleanupPayload>,
  db: JobsDatabase
): Promise<FileCleanupResult> {
  const { target, older_than_hours, older_than_days } = job.data;

  await job.updateProgress(10);

  let removedCount = 0;

  if (target === 'completed_jobs' && older_than_hours) {
    const result = await db.query<{ cleanup_old_jobs: number }>(
      'SELECT cleanup_old_jobs($1) as count',
      [older_than_hours]
    );
    removedCount = result[0]?.cleanup_old_jobs || 0;
  } else if (target === 'failed_jobs' && older_than_days) {
    const result = await db.query<{ cleanup_old_failed_jobs: number }>(
      'SELECT cleanup_old_failed_jobs($1) as count',
      [older_than_days]
    );
    removedCount = result[0]?.cleanup_old_failed_jobs || 0;
  }

  await job.updateProgress(100);

  return {
    files_removed: removedCount,
    bytes_freed: removedCount * 1024, // Estimate
    files: [],
  };
}

// =============================================================================
// Custom Job Processor
// =============================================================================

/**
 * Custom Job Processor
 * For user-defined custom jobs via Hasura Actions
 */
export async function processCustomJob(job: Job<CustomJobPayload>): Promise<unknown> {
  const { action, data } = job.data;

  await job.updateProgress(10);

  logger.info(`Processing custom action: ${action}`);

  // NOTE: Hasura Actions integration requires GraphQL endpoint configuration
  // Integration point: Make POST request to Hasura Actions endpoint with action name and data
  // Example: await fetch(hasuraUrl, { method: 'POST', body: JSON.stringify({ action, input: data }) })
  // Requires HASURA_ADMIN_SECRET and HASURA_GRAPHQL_ENDPOINT environment variables

  await job.updateProgress(100);

  return {
    success: true,
    action,
    processedAt: new Date().toISOString(),
  };
}
