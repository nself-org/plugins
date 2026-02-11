/**
 * Data Export Plugin Types
 * Complete type definitions for export, deletion, and import operations
 */

export interface ExportPluginConfig {
  port: number;
  host: string;
  storagePath: string;
  downloadExpiryHours: number;
  deletionCooldownHours: number;
  maxExportSizeMB: number;
  verificationCodeLength: number;
  apiKey?: string;
  rateLimitMax: number;
  rateLimitWindowMs: number;
  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl?: boolean;
  };
}

// =============================================================================
// Export Request Types
// =============================================================================

export type ExportRequestType = 'user_data' | 'plugin_data' | 'full_backup' | 'custom';
export type ExportFormat = 'json' | 'csv' | 'zip';
export type ExportStatus = 'pending' | 'processing' | 'completed' | 'failed' | 'expired';

export interface ExportRequestRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  request_type: ExportRequestType;
  requester_id: string;
  target_user_id: string | null;
  target_plugins: string[] | null;
  format: ExportFormat;
  status: ExportStatus;
  file_path: string | null;
  file_size_bytes: number | null;
  download_token: string | null;
  download_expires_at: Date | null;
  error_message: string | null;
  tables_exported: string[] | null;
  row_counts: Record<string, number>;
  started_at: Date | null;
  completed_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface CreateExportRequest {
  requestType: ExportRequestType;
  requesterId: string;
  targetUserId?: string;
  targetPlugins?: string[];
  format?: ExportFormat;
  customQuery?: string;
}

// =============================================================================
// Deletion Request Types
// =============================================================================

export type DeletionStatus = 'pending' | 'verifying' | 'processing' | 'completed' | 'failed' | 'cancelled';

export interface DeletionRequestRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  requester_id: string;
  target_user_id: string;
  reason: string | null;
  status: DeletionStatus;
  verification_code: string | null;
  verified_at: Date | null;
  cooldown_until: Date | null;
  tables_processed: string[] | null;
  rows_deleted: Record<string, number>;
  error_message: string | null;
  started_at: Date | null;
  completed_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface CreateDeletionRequest {
  requesterId: string;
  targetUserId: string;
  reason?: string;
}

// =============================================================================
// Plugin Registry Types
// =============================================================================

export interface PluginRegistryRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  plugin_name: string;
  tables: string[];
  user_id_column: string;
  export_query: string | null;
  deletion_query: string | null;
  enabled: boolean;
  created_at: Date;
  updated_at: Date;
}

export interface RegisterPlugin {
  pluginName: string;
  tables: string[];
  userIdColumn?: string;
  exportQuery?: string;
  deletionQuery?: string;
  enabled?: boolean;
}

// =============================================================================
// Import Job Types
// =============================================================================

export type ImportSourceType = 'file' | 'url';
export type ImportStatus = 'pending' | 'validating' | 'importing' | 'completed' | 'failed';

export interface ImportJobRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  requester_id: string;
  source_type: ImportSourceType;
  source_path: string | null;
  format: ExportFormat;
  status: ImportStatus;
  tables_imported: string[] | null;
  row_counts: Record<string, number>;
  validation_errors: ValidationError[];
  error_message: string | null;
  started_at: Date | null;
  completed_at: Date | null;
  created_at: Date;
}

export interface ValidationError {
  table: string;
  row: number;
  field: string;
  message: string;
}

export interface CreateImportJob {
  requesterId: string;
  sourceType: ImportSourceType;
  sourcePath: string;
  format?: ExportFormat;
}

// =============================================================================
// Webhook Event Types
// =============================================================================

export interface WebhookEventRecord extends Record<string, unknown> {
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
// Export/Import Data Types
// =============================================================================

export interface ExportData {
  metadata: {
    exportId: string;
    requestType: ExportRequestType;
    userId?: string;
    plugins?: string[];
    exportedAt: string;
    format: ExportFormat;
    version: string;
  };
  tables: Record<string, TableExport>;
}

export interface TableExport {
  tableName: string;
  rowCount: number;
  columns: string[];
  rows: Record<string, unknown>[];
}

// =============================================================================
// Statistics Types
// =============================================================================

export interface ExportStats {
  totalExports: number;
  pendingExports: number;
  completedExports: number;
  failedExports: number;
  totalDeletions: number;
  pendingDeletions: number;
  completedDeletions: number;
  failedDeletions: number;
  totalImports: number;
  registeredPlugins: number;
  lastExportAt: Date | null;
  lastDeletionAt: Date | null;
  lastImportAt: Date | null;
}

// =============================================================================
// Query Result Types
// =============================================================================

export interface PaginatedResult<T> {
  data: T[];
  total: number;
  limit: number;
  offset: number;
}

export interface OperationResult {
  success: boolean;
  message: string;
  data?: unknown;
  error?: string;
}
