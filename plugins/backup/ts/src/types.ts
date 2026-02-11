/**
 * Backup Plugin Types
 * Complete type definitions for backup management
 */

export interface BackupPluginConfig {
  port: number;
  host: string;
  databaseHost: string;
  databasePort: number;
  databaseName: string;
  databaseUser: string;
  databasePassword: string;
  databaseSsl: boolean;
  storagePath: string;
  s3Endpoint?: string;
  s3Bucket?: string;
  s3AccessKey?: string;
  s3SecretKey?: string;
  s3Region?: string;
  encryptionKey?: string;
  defaultRetentionDays: number;
  maxConcurrent: number;
  pgDumpPath: string;
  pgRestorePath: string;
  logLevel: string;
  security: {
    apiKey?: string;
    rateLimitMax: number;
    rateLimitWindowMs: number;
  };
}

// =============================================================================
// Database Records
// =============================================================================

export interface BackupScheduleRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  name: string;
  schedule_cron: string;
  backup_type: 'full' | 'incremental' | 'schema_only' | 'data_only';
  target_provider: 'local' | 's3' | 'r2' | 'gcs';
  target_config: Record<string, unknown>;
  include_tables: string[];
  exclude_tables: string[];
  compression: 'none' | 'gzip' | 'zstd';
  encryption_enabled: boolean;
  encryption_key_id: string | null;
  retention_days: number;
  max_backups: number;
  enabled: boolean;
  last_run_at: Date | null;
  next_run_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface BackupArtifactRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  schedule_id: string | null;
  backup_type: 'full' | 'incremental' | 'schema_only' | 'data_only';
  status: 'pending' | 'running' | 'completed' | 'failed' | 'expired';
  file_path: string | null;
  file_size_bytes: number | null;
  checksum_sha256: string | null;
  tables_included: string[];
  row_counts: Record<string, number>;
  duration_ms: number | null;
  error_message: string | null;
  target_provider: 'local' | 's3' | 'r2' | 'gcs';
  target_location: string | null;
  expires_at: Date | null;
  started_at: Date | null;
  completed_at: Date | null;
  created_at: Date;
}

export interface BackupRestoreJobRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  artifact_id: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
  target_database: string;
  tables_to_restore: string[];
  restore_mode: 'merge' | 'replace' | 'dry_run';
  conflict_strategy: 'skip' | 'overwrite' | 'error';
  rows_restored: number;
  errors: Array<{ table: string; error: string }>;
  started_at: Date | null;
  completed_at: Date | null;
  created_at: Date;
}

export interface BackupWebhookEventRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  created_at: Date;
}

// =============================================================================
// API Types
// =============================================================================

export interface CreateScheduleRequest {
  name: string;
  schedule_cron: string;
  backup_type?: 'full' | 'incremental' | 'schema_only' | 'data_only';
  target_provider?: 'local' | 's3' | 'r2' | 'gcs';
  target_config?: Record<string, unknown>;
  include_tables?: string[];
  exclude_tables?: string[];
  compression?: 'none' | 'gzip' | 'zstd';
  encryption_enabled?: boolean;
  encryption_key_id?: string;
  retention_days?: number;
  max_backups?: number;
  enabled?: boolean;
}

export interface UpdateScheduleRequest {
  name?: string;
  schedule_cron?: string;
  backup_type?: 'full' | 'incremental' | 'schema_only' | 'data_only';
  target_provider?: 'local' | 's3' | 'r2' | 'gcs';
  target_config?: Record<string, unknown>;
  include_tables?: string[];
  exclude_tables?: string[];
  compression?: 'none' | 'gzip' | 'zstd';
  encryption_enabled?: boolean;
  encryption_key_id?: string;
  retention_days?: number;
  max_backups?: number;
  enabled?: boolean;
}

export interface RunBackupRequest {
  schedule_id?: string;
  backup_type?: 'full' | 'incremental' | 'schema_only' | 'data_only';
  include_tables?: string[];
  exclude_tables?: string[];
  compression?: 'none' | 'gzip' | 'zstd';
}

export interface RestoreRequest {
  artifact_id: string;
  target_database?: string;
  tables_to_restore?: string[];
  restore_mode?: 'merge' | 'replace' | 'dry_run';
  conflict_strategy?: 'skip' | 'overwrite' | 'error';
}

export interface BackupStats {
  total_schedules: number;
  active_schedules: number;
  total_artifacts: number;
  completed_artifacts: number;
  failed_artifacts: number;
  total_size_bytes: number;
  oldest_backup: Date | null;
  newest_backup: Date | null;
  active_restore_jobs: number;
}

// =============================================================================
// Backup Execution Types
// =============================================================================

export interface BackupOptions {
  scheduleId?: string;
  backupType: 'full' | 'incremental' | 'schema_only' | 'data_only';
  includeTables?: string[];
  excludeTables?: string[];
  compression: 'none' | 'gzip' | 'zstd';
  encryption?: {
    enabled: boolean;
    keyId?: string;
  };
  targetProvider: 'local' | 's3' | 'r2' | 'gcs';
  targetConfig: Record<string, unknown>;
  retentionDays: number;
}

export interface BackupResult {
  artifactId: string;
  success: boolean;
  filePath: string | null;
  fileSize: number | null;
  checksum: string | null;
  tablesIncluded: string[];
  rowCounts: Record<string, number>;
  duration: number;
  error?: string;
}

export interface RestoreOptions {
  artifactId: string;
  targetDatabase: string;
  tablesToRestore?: string[];
  restoreMode: 'merge' | 'replace' | 'dry_run';
  conflictStrategy: 'skip' | 'overwrite' | 'error';
}

export interface RestoreResult {
  jobId: string;
  success: boolean;
  rowsRestored: number;
  duration: number;
  errors: Array<{ table: string; error: string }>;
}
