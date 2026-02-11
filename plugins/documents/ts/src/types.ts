/**
 * Documents Plugin Types
 * Complete type definitions for documents, templates, versions, and shares
 */

// =============================================================================
// Database Record Types
// =============================================================================

export interface DocumentRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  owner_id: string;
  title: string;
  description: string | null;
  doc_type: string;
  category: string | null;
  tags: string[];
  template_id: string | null;
  file_url: string | null;
  file_size_bytes: number | null;
  mime_type: string | null;
  version: number;
  status: 'draft' | 'final' | 'archived';
  generated_from: Record<string, unknown> | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface TemplateRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  doc_type: string;
  output_format: 'pdf' | 'html';
  template_engine: 'handlebars' | 'ejs' | 'pug';
  template_content: string;
  css_content: string | null;
  header_content: string | null;
  footer_content: string | null;
  variables: Record<string, unknown>;
  sample_data: Record<string, unknown>;
  is_default: boolean;
  version: number;
  created_at: Date;
  updated_at: Date;
}

export interface VersionRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  document_id: string;
  version: number;
  file_url: string;
  file_size_bytes: number | null;
  change_summary: string | null;
  created_by: string | null;
  created_at: Date;
}

export interface ShareRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  document_id: string;
  shared_with_user_id: string | null;
  shared_with_email: string | null;
  share_token: string | null;
  permission: 'view' | 'download';
  expires_at: Date | null;
  accessed_at: Date | null;
  created_at: Date;
}

export interface WebhookEventRecord {
  id: string;
  source_account_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  retry_count: number;
  created_at: Date;
}

// =============================================================================
// API Request Types
// =============================================================================

export interface CreateDocumentRequest {
  owner_id: string;
  title: string;
  description?: string;
  doc_type: string;
  category?: string;
  tags?: string[];
  file_url?: string;
  file_size_bytes?: number;
  mime_type?: string;
  metadata?: Record<string, unknown>;
}

export interface UpdateDocumentRequest {
  title?: string;
  description?: string;
  category?: string;
  tags?: string[];
  status?: 'draft' | 'final' | 'archived';
  metadata?: Record<string, unknown>;
}

export interface GenerateDocumentRequest {
  template_id?: string;
  template_name?: string;
  data: Record<string, unknown>;
  output_format?: 'pdf' | 'html';
  title?: string;
  category?: string;
  owner_id: string;
}

export interface GeneratePreviewRequest {
  template_id: string;
  data: Record<string, unknown>;
}

export interface CreateTemplateRequest {
  name: string;
  description?: string;
  doc_type: string;
  output_format?: 'pdf' | 'html';
  template_engine?: 'handlebars' | 'ejs' | 'pug';
  template_content: string;
  css_content?: string;
  header_content?: string;
  footer_content?: string;
  variables?: Record<string, unknown>;
  sample_data?: Record<string, unknown>;
}

export interface UpdateTemplateRequest {
  name?: string;
  description?: string;
  doc_type?: string;
  output_format?: 'pdf' | 'html';
  template_engine?: 'handlebars' | 'ejs' | 'pug';
  template_content?: string;
  css_content?: string;
  header_content?: string;
  footer_content?: string;
  variables?: Record<string, unknown>;
  sample_data?: Record<string, unknown>;
}

export interface CreateShareRequest {
  shared_with_user_id?: string;
  shared_with_email?: string;
  permission?: 'view' | 'download';
  expires_at?: string;
}

export interface SearchDocumentsRequest {
  query: string;
  doc_type?: string;
  category?: string;
  owner_id?: string;
  date_from?: string;
  date_to?: string;
  limit?: number;
}

export interface ListDocumentsQuery {
  owner_id?: string;
  doc_type?: string;
  category?: string;
  status?: string;
  limit?: number;
  offset?: number;
}

export interface ListTemplatesQuery {
  doc_type?: string;
  limit?: number;
  offset?: number;
}

// =============================================================================
// API Response Types
// =============================================================================

export interface GenerateDocumentResponse {
  document_id: string;
  file_url: string;
  mime_type: string;
  file_size_bytes: number;
}

export interface ShareResponse {
  share_id: string;
  share_token: string;
  share_url: string;
}

// =============================================================================
// Stats Types
// =============================================================================

export interface DocumentStats {
  total_documents: number;
  by_type: Record<string, number>;
  by_category: Record<string, number>;
  total_templates: number;
  total_shares: number;
  total_versions: number;
  recent_documents: number;
}
