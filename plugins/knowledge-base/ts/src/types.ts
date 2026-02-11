/**
 * Knowledge Base Plugin Types
 * Complete type definitions for all knowledge base objects
 */

// =============================================================================
// Document Types
// =============================================================================

export type DocumentStatus = 'draft' | 'published' | 'archived';
export type DocumentType = 'article' | 'guide' | 'tutorial' | 'reference' | 'faq' | 'troubleshooting' | 'changelog';
export type Visibility = 'public' | 'internal' | 'private';
export type ProcessingStatus = 'pending' | 'processing' | 'completed' | 'failed';
export type CommentStatus = 'published' | 'hidden' | 'spam';
export type AnalyticsEventType = 'view' | 'search' | 'helpful' | 'not_helpful' | 'share' | 'download' | 'print';
export type TranslationMethod = 'manual' | 'machine' | 'hybrid';
export type TranslationStatus = 'draft' | 'review' | 'published';
export type ReviewStatus = 'pending' | 'in_progress' | 'approved' | 'rejected';
export type ReviewPriority = 'low' | 'normal' | 'high' | 'urgent';

export interface KBDocumentRecord {
  id: string;
  source_account_id: string;
  workspace_id: string;
  collection_id: string | null;
  created_by: string;
  title: string;
  slug: string;
  content: string;
  content_html: string | null;
  excerpt: string | null;
  status: DocumentStatus;
  document_type: DocumentType;
  language: string;
  meta_title: string | null;
  meta_description: string | null;
  meta_keywords: string[] | null;
  tags: string[] | null;
  category: string | null;
  priority: number;
  version: number;
  parent_version_id: string | null;
  is_latest_version: boolean;
  visibility: Visibility;
  required_role: string | null;
  view_count: number;
  helpful_count: number;
  not_helpful_count: number;
  average_rating: number | null;
  published_at: Date | null;
  last_reviewed_at: Date | null;
  review_reminder_at: Date | null;
  created_at: Date;
  updated_at: Date;
  [key: string]: unknown;
}

export interface CreateDocumentRequest {
  workspace_id: string;
  collection_id?: string;
  created_by: string;
  title: string;
  slug: string;
  content: string;
  content_html?: string;
  excerpt?: string;
  status?: DocumentStatus;
  document_type?: DocumentType;
  language?: string;
  meta_title?: string;
  meta_description?: string;
  meta_keywords?: string[];
  tags?: string[];
  category?: string;
  priority?: number;
  visibility?: Visibility;
}

export interface UpdateDocumentRequest {
  collection_id?: string | null;
  title?: string;
  content?: string;
  content_html?: string;
  excerpt?: string;
  status?: DocumentStatus;
  document_type?: DocumentType;
  meta_title?: string;
  meta_description?: string;
  meta_keywords?: string[];
  tags?: string[];
  category?: string;
  priority?: number;
  visibility?: Visibility;
}

// =============================================================================
// Collection Types
// =============================================================================

export interface KBCollectionRecord {
  id: string;
  source_account_id: string;
  workspace_id: string;
  parent_id: string | null;
  created_by: string;
  name: string;
  slug: string;
  description: string | null;
  icon: string | null;
  color: string | null;
  display_order: number;
  path: string[] | null;
  depth: number;
  visibility: Visibility;
  default_language: string;
  allowed_languages: string[] | null;
  created_at: Date;
  updated_at: Date;
  [key: string]: unknown;
}

export interface CreateCollectionRequest {
  workspace_id: string;
  parent_id?: string;
  created_by: string;
  name: string;
  slug: string;
  description?: string;
  icon?: string;
  color?: string;
  visibility?: Visibility;
  default_language?: string;
  allowed_languages?: string[];
}

export interface UpdateCollectionRequest {
  name?: string;
  description?: string;
  icon?: string;
  color?: string;
  visibility?: Visibility;
  display_order?: number;
  allowed_languages?: string[];
}

// =============================================================================
// FAQ Types
// =============================================================================

export interface KBFaqRecord {
  id: string;
  source_account_id: string;
  workspace_id: string;
  collection_id: string | null;
  created_by: string;
  question: string;
  answer: string;
  answer_html: string | null;
  category: string | null;
  tags: string[] | null;
  display_order: number;
  status: DocumentStatus;
  language: string;
  view_count: number;
  helpful_count: number;
  not_helpful_count: number;
  related_documents: string[] | null;
  related_faqs: string[] | null;
  published_at: Date | null;
  created_at: Date;
  updated_at: Date;
  [key: string]: unknown;
}

export interface CreateFaqRequest {
  workspace_id: string;
  collection_id?: string;
  created_by: string;
  question: string;
  answer: string;
  answer_html?: string;
  category?: string;
  tags?: string[];
  status?: DocumentStatus;
  language?: string;
}

export interface UpdateFaqRequest {
  question?: string;
  answer?: string;
  answer_html?: string;
  category?: string;
  tags?: string[];
  status?: DocumentStatus;
  display_order?: number;
}

// =============================================================================
// Attachment Types
// =============================================================================

export interface KBAttachmentRecord {
  id: string;
  source_account_id: string;
  workspace_id: string;
  document_id: string | null;
  uploaded_by: string;
  filename: string;
  original_filename: string;
  mime_type: string;
  file_size: number;
  storage_path: string;
  title: string | null;
  description: string | null;
  alt_text: string | null;
  processing_status: ProcessingStatus;
  thumbnail_url: string | null;
  created_at: Date;
  updated_at: Date;
  [key: string]: unknown;
}

export interface CreateAttachmentRequest {
  workspace_id: string;
  document_id?: string;
  uploaded_by: string;
  filename: string;
  original_filename: string;
  mime_type: string;
  file_size: number;
  storage_path: string;
  title?: string;
  description?: string;
  alt_text?: string;
}

// =============================================================================
// Comment Types
// =============================================================================

export interface KBCommentRecord {
  id: string;
  source_account_id: string;
  workspace_id: string;
  document_id: string;
  user_id: string;
  parent_id: string | null;
  content: string;
  content_html: string | null;
  status: CommentStatus;
  is_staff_reply: boolean;
  helpful_count: number;
  created_at: Date;
  updated_at: Date;
  [key: string]: unknown;
}

export interface CreateCommentRequest {
  workspace_id: string;
  document_id: string;
  user_id: string;
  parent_id?: string;
  content: string;
  content_html?: string;
  is_staff_reply?: boolean;
}

export interface UpdateCommentRequest {
  content?: string;
  content_html?: string;
  status?: CommentStatus;
}

// =============================================================================
// Analytics Types
// =============================================================================

export interface KBAnalyticsRecord {
  id: string;
  source_account_id: string;
  workspace_id: string;
  document_id: string | null;
  faq_id: string | null;
  event_type: AnalyticsEventType;
  user_id: string | null;
  session_id: string | null;
  search_query: string | null;
  referrer: string | null;
  user_agent: string | null;
  ip_address: string | null;
  metadata: Record<string, unknown> | null;
  created_at: Date;
  [key: string]: unknown;
}

export interface TrackAnalyticsEventRequest {
  workspace_id: string;
  document_id?: string;
  faq_id?: string;
  event_type: AnalyticsEventType;
  user_id?: string;
  session_id?: string;
  search_query?: string;
  referrer?: string;
  user_agent?: string;
  ip_address?: string;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Translation Types
// =============================================================================

export interface KBTranslationRecord {
  id: string;
  source_account_id: string;
  workspace_id: string;
  source_document_id: string | null;
  source_faq_id: string | null;
  language: string;
  translated_by: string | null;
  translation_method: TranslationMethod | null;
  title: string | null;
  content: string | null;
  content_html: string | null;
  answer: string | null;
  answer_html: string | null;
  status: TranslationStatus;
  quality_score: number | null;
  created_at: Date;
  updated_at: Date;
  [key: string]: unknown;
}

export interface CreateTranslationRequest {
  workspace_id: string;
  source_document_id?: string;
  source_faq_id?: string;
  language: string;
  translated_by?: string;
  translation_method?: TranslationMethod;
  title?: string;
  content?: string;
  content_html?: string;
  answer?: string;
  answer_html?: string;
  status?: TranslationStatus;
}

export interface UpdateTranslationRequest {
  title?: string;
  content?: string;
  content_html?: string;
  answer?: string;
  answer_html?: string;
  status?: TranslationStatus;
  quality_score?: number;
}

// =============================================================================
// Review Request Types
// =============================================================================

export interface KBReviewRequestRecord {
  id: string;
  source_account_id: string;
  workspace_id: string;
  document_id: string;
  requested_by: string;
  assigned_to: string | null;
  status: ReviewStatus;
  priority: ReviewPriority;
  review_notes: string | null;
  changes_requested: string[] | null;
  due_date: Date | null;
  completed_at: Date | null;
  created_at: Date;
  updated_at: Date;
  [key: string]: unknown;
}

export interface CreateReviewRequestRequest {
  workspace_id: string;
  document_id: string;
  requested_by: string;
  assigned_to?: string;
  priority?: ReviewPriority;
  due_date?: string;
}

// =============================================================================
// Search Types
// =============================================================================

export interface KBSearchResult {
  id: string;
  title: string;
  excerpt: string | null;
  slug: string;
  document_type: DocumentType;
  rank: number;
  [key: string]: unknown;
}

// =============================================================================
// Stats Types
// =============================================================================

export interface KBStats {
  total_documents: number;
  published_documents: number;
  draft_documents: number;
  total_faqs: number;
  total_collections: number;
  total_comments: number;
  total_views: number;
  total_searches: number;
  total_translations: number;
  pending_reviews: number;
}

export interface PopularSearch {
  search_query: string;
  search_count: number;
  unique_users: number;
  [key: string]: unknown;
}
