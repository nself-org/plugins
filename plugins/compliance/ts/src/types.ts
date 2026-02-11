/**
 * Compliance Plugin Types
 * Complete type definitions for GDPR/CCPA compliance operations
 */

// =============================================================================
// Enum Types
// =============================================================================

export type DsarType = 'access' | 'rectification' | 'erasure' | 'portability' | 'restriction' | 'objection' | 'ccpa_disclosure' | 'ccpa_deletion' | 'ccpa_opt_out';

export type DsarStatus = 'pending' | 'in_progress' | 'verification_required' | 'approved' | 'completed' | 'rejected' | 'cancelled';

export type ConsentStatus = 'granted' | 'denied' | 'withdrawn' | 'expired';

export type DataCategory = 'account' | 'profile' | 'messages' | 'files' | 'metadata' | 'analytics' | 'communications' | 'location' | 'device' | 'usage' | 'preferences';

export type RetentionAction = 'delete' | 'anonymize' | 'archive' | 'notify';

export type BreachSeverity = 'low' | 'medium' | 'high' | 'critical';

// =============================================================================
// DSAR Types
// =============================================================================

export interface DsarRecord {
  id: string;
  source_account_id: string;
  request_type: DsarType;
  request_number: string;
  user_id: string | null;
  requester_email: string;
  requester_name: string | null;
  verification_token: string | null;
  verification_sent_at: Date | null;
  verification_completed_at: Date | null;
  verified_by: string | null;
  description: string | null;
  data_categories: DataCategory[];
  specific_data_requested: string | null;
  status: DsarStatus;
  assigned_to: string | null;
  started_at: Date | null;
  completed_at: Date | null;
  deadline: Date;
  data_package_url: string | null;
  data_package_size_bytes: number | null;
  data_package_generated_at: Date | null;
  resolution_notes: string | null;
  rejection_reason: string | null;
  regulation: string;
  jurisdiction: string | null;
  legal_basis: string | null;
  ip_address: string | null;
  user_agent: string | null;
  created_at: Date;
  updated_at: Date;
}

export interface CreateDsarRequest {
  request_type: DsarType;
  email: string;
  name?: string;
  user_id?: string;
  description?: string;
  data_categories?: DataCategory[];
  specific_data_requested?: string;
  regulation?: string;
  jurisdiction?: string;
}

export interface ProcessDsarRequest {
  action: 'approve' | 'reject';
  notes?: string;
  rejection_reason?: string;
  assigned_to?: string;
}

// =============================================================================
// DSAR Activity Types
// =============================================================================

export interface DsarActivityRecord {
  id: string;
  source_account_id: string;
  dsar_id: string;
  activity_type: string;
  description: string | null;
  performed_by: string | null;
  performed_by_name: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
}

// =============================================================================
// Consent Types
// =============================================================================

export interface ConsentRecord {
  id: string;
  source_account_id: string;
  user_id: string;
  purpose: string;
  purpose_description: string | null;
  status: ConsentStatus;
  granted_at: Date | null;
  denied_at: Date | null;
  withdrawn_at: Date | null;
  expires_at: Date | null;
  consent_method: string | null;
  consent_text: string | null;
  privacy_policy_version: string | null;
  ip_address: string | null;
  user_agent: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface CreateConsentRequest {
  user_id: string;
  purpose: string;
  status: 'granted' | 'denied';
  purpose_description?: string;
  consent_text?: string;
  consent_method?: string;
  privacy_policy_version?: string;
  expires_at?: string;
}

export interface ConsentHistoryRecord {
  id: string;
  source_account_id: string;
  consent_id: string;
  previous_status: ConsentStatus | null;
  new_status: ConsentStatus;
  change_reason: string | null;
  changed_by: string | null;
  ip_address: string | null;
  user_agent: string | null;
  created_at: Date;
}

// =============================================================================
// Privacy Policy Types
// =============================================================================

export interface PrivacyPolicyRecord {
  id: string;
  source_account_id: string;
  version: string;
  version_number: number;
  title: string;
  content: string;
  summary: string | null;
  changes_summary: string | null;
  is_active: boolean;
  requires_reacceptance: boolean;
  effective_from: Date;
  effective_until: Date | null;
  language: string;
  jurisdiction: string | null;
  created_by: string | null;
  created_at: Date;
}

export interface CreatePrivacyPolicyRequest {
  version: string;
  version_number: number;
  title: string;
  content: string;
  summary?: string;
  changes_summary?: string;
  requires_reacceptance?: boolean;
  effective_from: string;
  language?: string;
  jurisdiction?: string;
}

export interface PolicyAcceptanceRecord {
  id: string;
  source_account_id: string;
  user_id: string;
  policy_id: string;
  accepted_at: Date;
  ip_address: string | null;
  user_agent: string | null;
  metadata: Record<string, unknown>;
}

// =============================================================================
// Retention Types
// =============================================================================

export interface RetentionPolicyRecord {
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  data_category: DataCategory;
  table_name: string | null;
  retention_days: number;
  retention_action: RetentionAction;
  conditions: Record<string, unknown>;
  is_enabled: boolean;
  priority: number;
  legal_basis: string | null;
  regulation: string | null;
  created_by: string | null;
  created_at: Date;
  updated_at: Date;
}

export interface CreateRetentionPolicyRequest {
  name: string;
  description?: string;
  data_category: DataCategory;
  table_name?: string;
  retention_days: number;
  retention_action: RetentionAction;
  conditions?: Record<string, unknown>;
  legal_basis?: string;
  regulation?: string;
}

export interface RetentionExecutionRecord {
  id: string;
  source_account_id: string;
  policy_id: string;
  executed_at: Date;
  records_processed: number;
  records_deleted: number;
  records_anonymized: number;
  records_archived: number;
  status: string;
  error_message: string | null;
  execution_time_ms: number | null;
  metadata: Record<string, unknown>;
}

// =============================================================================
// Processing Records Types
// =============================================================================

export interface ProcessingRecordEntry {
  id: string;
  source_account_id: string;
  activity_name: string;
  activity_description: string | null;
  processing_purpose: string;
  legal_basis: string;
  data_categories: DataCategory[];
  data_subjects: string[];
  recipient_categories: string[];
  third_party_transfers: boolean;
  third_party_countries: string[];
  safeguards: string | null;
  retention_period: string | null;
  security_measures: string | null;
  is_active: boolean;
  created_by: string | null;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// Data Processor Types
// =============================================================================

export interface DataProcessorRecord {
  id: string;
  source_account_id: string;
  processor_name: string;
  processor_type: string | null;
  contact_name: string | null;
  contact_email: string | null;
  contact_phone: string | null;
  country: string | null;
  is_eu_based: boolean;
  dpa_signed: boolean;
  dpa_signed_date: string | null;
  dpa_expiry_date: string | null;
  dpa_document_url: string | null;
  processing_purposes: string[];
  data_categories: DataCategory[];
  has_privacy_shield: boolean;
  has_scc: boolean;
  has_bcr: boolean;
  security_certifications: string[];
  last_security_audit: string | null;
  is_active: boolean;
  notes: string | null;
  metadata: Record<string, unknown>;
  created_by: string | null;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// Breach Types
// =============================================================================

export interface DataBreachRecord {
  id: string;
  source_account_id: string;
  breach_number: string;
  title: string;
  description: string;
  discovered_at: Date;
  discovered_by: string | null;
  severity: BreachSeverity;
  affected_users_count: number | null;
  data_categories: DataCategory[];
  data_description: string | null;
  risk_assessment: string | null;
  mitigation_steps: string | null;
  notification_required: boolean;
  authority_notified_at: Date | null;
  users_notified_at: Date | null;
  notification_deadline: Date | null;
  resolved_at: Date | null;
  resolution_summary: string | null;
  root_cause: string | null;
  preventive_measures: string | null;
  status: string;
  assigned_to: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface CreateBreachRequest {
  title: string;
  description: string;
  severity: BreachSeverity;
  discovered_at?: string;
  discovered_by?: string;
  affected_users_count?: number;
  data_categories: DataCategory[];
  data_description?: string;
  notification_required?: boolean;
}

export interface BreachNotificationRecord {
  id: string;
  source_account_id: string;
  breach_id: string;
  notification_type: string;
  recipient_type: string;
  recipient_email: string | null;
  subject: string | null;
  message_body: string | null;
  sent_at: Date | null;
  delivery_status: string | null;
  created_at: Date;
}

// =============================================================================
// Audit Log Types
// =============================================================================

export interface ComplianceAuditLogRecord {
  id: string;
  source_account_id: string;
  event_type: string;
  event_category: string;
  actor_id: string | null;
  actor_type: string;
  target_type: string | null;
  target_id: string | null;
  accessed_data_categories: DataCategory[];
  data_subject_id: string | null;
  details: Record<string, unknown>;
  ip_address: string | null;
  user_agent: string | null;
  legal_basis: string | null;
  created_at: Date;
}

export interface CreateAuditLogRequest {
  event_type: string;
  event_category: string;
  actor_id?: string;
  actor_type?: string;
  target_type?: string;
  target_id?: string;
  accessed_data_categories?: DataCategory[];
  data_subject_id?: string;
  details?: Record<string, unknown>;
  ip_address?: string;
  user_agent?: string;
  legal_basis?: string;
}

// =============================================================================
// Data Export Types
// =============================================================================

export interface ExportDataRequest {
  user_id: string;
  data_categories?: DataCategory[];
  format?: 'json' | 'csv';
}

export interface ExportDataResponse {
  export_url: string;
  expires_at: string;
}
