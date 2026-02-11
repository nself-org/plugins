/**
 * Export/Import Plugin Types
 * Complete type definitions for data export, import, migration, backup, and restore
 */

// =============================================================================
// Configuration
// =============================================================================

export interface ExportImportConfig {
  // Server
  port: number;
  host: string;

  // Database
  databaseHost: string;
  databasePort: number;
  databaseName: string;
  databaseUser: string;
  databasePassword: string;
  databaseSsl: boolean;

  // Export settings
  exportStoragePath: string;
  exportMaxFileSizeGb: number;
  exportRetentionDays: number;

  // Import settings
  importMaxFileSizeGb: number;
  importTempPath: string;

  // Backup settings
  backupStorageBackend: StorageBackend;
  backupRetentionDays: number;

  // Migration settings
  migrationBatchSize: number;
  migrationRateLimitMs: number;

  // Queue settings
  queueConcurrency: number;
  queueTimeoutMinutes: number;

  // Compression
  compressionLevel: number;
  compressionAlgorithm: CompressionType;

  // Security
  apiKey?: string;
  rateLimitMax: number;
  rateLimitWindowMs: number;

  logLevel: string;
}

// =============================================================================
// Enum/Union Types
// =============================================================================

export type ExportType = 'full' | 'partial' | 'incremental';

export type ExportFormat = 'json' | 'csv' | 'xml' | 'yaml' | 'sql' | 'parquet';

export type ImportType = 'merge' | 'replace' | 'append';

export type ImportFormat = 'json' | 'csv' | 'xml' | 'yaml' | 'sql' | 'slack' | 'discord' | 'teams';

export type MigrationPlatform = 'slack' | 'discord' | 'teams' | 'mattermost' | 'rocket_chat' | 'telegram';

export type BackupType = 'full' | 'incremental' | 'differential';

export type RestoreType = 'full' | 'partial' | 'point_in_time';

export type CompressionType = 'none' | 'gzip' | 'bzip2' | 'xz';

export type StorageBackend = 'local' | 's3' | 'gcs' | 'azure' | 'minio';

export type ExportJobStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';

export type ImportJobStatus = 'pending' | 'validating' | 'running' | 'completed' | 'failed' | 'cancelled';

export type MigrationJobStatus = 'pending' | 'analyzing' | 'running' | 'completed' | 'failed' | 'cancelled';

export type RestoreJobStatus = 'pending' | 'validating' | 'running' | 'completed' | 'failed' | 'cancelled';

export type ConflictResolution = 'skip' | 'overwrite' | 'merge' | 'fail';

export type ValidationMode = 'strict' | 'lenient' | 'skip';

export type VerificationStatus = 'pending' | 'verified' | 'failed';

export type JobType = 'export' | 'import' | 'migration' | 'backup' | 'restore';

// =============================================================================
// Database Records
// =============================================================================

export interface ExportJobRecord {
  id: string;
  source_account_id: string;
  user_id: string;
  name: string;
  description: string | null;
  export_type: ExportType;
  format: ExportFormat;
  scope: Record<string, unknown>;
  filters: Record<string, unknown>;
  compression: CompressionType | null;
  encryption_enabled: boolean;
  encryption_key_id: string | null;
  status: ExportJobStatus;
  progress_percentage: number;
  total_records: number;
  exported_records: number;
  output_path: string | null;
  output_size_bytes: number | null;
  checksum: string | null;
  error_message: string | null;
  metadata: Record<string, unknown>;
  started_at: Date | null;
  completed_at: Date | null;
  expires_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface ImportJobRecord {
  id: string;
  source_account_id: string;
  user_id: string;
  name: string;
  description: string | null;
  import_type: ImportType;
  source_format: ImportFormat;
  source_path: string;
  source_size_bytes: number | null;
  mapping_rules: Record<string, unknown>;
  conflict_resolution: ConflictResolution;
  validation_mode: ValidationMode;
  dry_run: boolean;
  status: ImportJobStatus;
  progress_percentage: number;
  total_records: number;
  imported_records: number;
  skipped_records: number;
  failed_records: number;
  validation_errors: unknown[];
  error_message: string | null;
  metadata: Record<string, unknown>;
  started_at: Date | null;
  completed_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface MigrationJobRecord {
  id: string;
  source_account_id: string;
  user_id: string;
  name: string;
  source_platform: MigrationPlatform;
  source_credentials: Record<string, unknown>;
  destination_scope: Record<string, unknown>;
  migration_plan: Record<string, unknown>;
  status: MigrationJobStatus;
  phase: string | null;
  progress_percentage: number;
  estimated_duration_minutes: number | null;
  total_items: number;
  migrated_items: number;
  failed_items: number;
  warnings: unknown[];
  error_message: string | null;
  metadata: Record<string, unknown>;
  started_at: Date | null;
  completed_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface BackupSnapshotRecord {
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  backup_type: BackupType;
  base_snapshot_id: string | null;
  scope: Record<string, unknown>;
  compression: CompressionType;
  encryption_enabled: boolean;
  encryption_key_id: string | null;
  storage_backend: StorageBackend;
  storage_path: string;
  total_size_bytes: number | null;
  compressed_size_bytes: number | null;
  checksum: string | null;
  verification_status: VerificationStatus | null;
  verified_at: Date | null;
  retention_days: number;
  expires_at: Date | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface RestoreJobRecord {
  id: string;
  source_account_id: string;
  user_id: string;
  snapshot_id: string;
  restore_type: RestoreType;
  target_scope: Record<string, unknown>;
  restore_point: Date | null;
  conflict_resolution: ConflictResolution;
  status: RestoreJobStatus;
  progress_percentage: number;
  total_items: number;
  restored_items: number;
  failed_items: number;
  error_message: string | null;
  metadata: Record<string, unknown>;
  started_at: Date | null;
  completed_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface TransformTemplateRecord {
  id: string;
  source_account_id: string;
  user_id: string | null;
  name: string;
  description: string | null;
  source_format: string;
  target_format: string;
  transformations: Record<string, unknown>;
  is_public: boolean;
  usage_count: number;
  created_at: Date;
  updated_at: Date;
}

export interface DataTransferAuditRecord {
  id: string;
  source_account_id: string;
  job_type: JobType;
  job_id: string;
  user_id: string;
  action: string;
  records_affected: number | null;
  data_size_bytes: number | null;
  ip_address: string | null;
  user_agent: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
}

// =============================================================================
// API Request Types
// =============================================================================

export interface CreateExportJobRequest {
  user_id: string;
  name: string;
  description?: string;
  export_type: ExportType;
  format: ExportFormat;
  scope: Record<string, unknown>;
  filters?: Record<string, unknown>;
  compression?: CompressionType;
  encryption_enabled?: boolean;
}

export interface CreateImportJobRequest {
  user_id: string;
  name: string;
  description?: string;
  import_type: ImportType;
  source_format: ImportFormat;
  source_path: string;
  source_size_bytes?: number;
  mapping_rules?: Record<string, unknown>;
  conflict_resolution?: ConflictResolution;
  validation_mode?: ValidationMode;
  dry_run?: boolean;
}

export interface CreateMigrationJobRequest {
  user_id: string;
  name: string;
  source_platform: MigrationPlatform;
  source_credentials: Record<string, unknown>;
  destination_scope: Record<string, unknown>;
  migration_plan: Record<string, unknown>;
}

export interface CreateBackupSnapshotRequest {
  name: string;
  description?: string;
  backup_type: BackupType;
  scope: Record<string, unknown>;
  compression: CompressionType;
  encryption_enabled?: boolean;
  storage_backend: StorageBackend;
  storage_path?: string;
  retention_days?: number;
}

export interface CreateRestoreJobRequest {
  user_id: string;
  snapshot_id: string;
  restore_type: RestoreType;
  target_scope: Record<string, unknown>;
  restore_point?: string;
  conflict_resolution?: ConflictResolution;
}

export interface CreateTransformTemplateRequest {
  user_id?: string;
  name: string;
  description?: string;
  source_format: string;
  target_format: string;
  transformations: Record<string, unknown>;
  is_public?: boolean;
}

export interface UpdateTransformTemplateRequest {
  name?: string;
  description?: string;
  transformations?: Record<string, unknown>;
  is_public?: boolean;
}

export interface UpdateJobProgressRequest {
  progress_percentage: number;
  metadata?: Record<string, unknown>;
}

export interface EstimateExportRequest {
  scope: Record<string, unknown>;
  filters?: Record<string, unknown>;
}

export interface CreateAuditEntryRequest {
  job_type: JobType;
  job_id: string;
  user_id: string;
  action: string;
  records_affected?: number;
  data_size_bytes?: number;
  ip_address?: string;
  user_agent?: string;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Statistics
// =============================================================================

export interface ExportImportStats {
  export_jobs: { total: number; pending: number; running: number; completed: number; failed: number };
  import_jobs: { total: number; pending: number; running: number; completed: number; failed: number };
  migration_jobs: { total: number; pending: number; running: number; completed: number; failed: number };
  backup_snapshots: { total: number; verified: number; expired: number };
  restore_jobs: { total: number; pending: number; running: number; completed: number; failed: number };
  transform_templates: { total: number; public_count: number };
  audit_entries: number;
}

export interface BackupScheduleConfig {
  enabled: boolean;
  frequency: 'hourly' | 'daily' | 'weekly' | 'monthly';
  time: string;
  backup_type: BackupType;
  retention_days: number;
  storage_backend: StorageBackend;
}
