/**
 * ID.me Plugin Types
 */

export interface IDmeConfig {
  clientId: string;
  clientSecret: string;
  redirectUri: string;
  scopes: string[];
  sandbox?: boolean;
  webhookSecret?: string;
}

export interface IDmeTokens {
  accessToken: string;
  refreshToken?: string;
  expiresIn: number;
  expiresAt: Date;
}

export interface IDmeUserProfile {
  firstName?: string;
  lastName?: string;
  email?: string;
  birthDate?: string;
  zip?: string;
  phone?: string;
}

export interface IDmeGroup {
  id: string;
  name: string;
  type: 'military' | 'veteran' | 'first_responder' | 'government' | 'teacher' | 'student' | 'nurse';
  verified: boolean;
  verifiedAt?: string;
}

export interface IDmeAttributes {
  firstName?: string;
  lastName?: string;
  email?: string;
  birthDate?: string;
  zip?: string;
  affiliation?: string;
  branch?: string;
  serviceEra?: string;
  rank?: string;
  status?: string;
}

export interface IDmeVerification {
  verified: boolean;
  groups: IDmeGroup[];
  attributes: IDmeAttributes;
}

export interface IDmeBadge {
  type: string;
  name: string;
  icon?: string;
  color?: string;
  verifiedAt?: string;
}

// Database record types
export interface IDmeVerificationRecord {
  id: string;
  source_account_id: string;
  user_id: string;
  idme_user_id?: string;
  email: string;
  verified: boolean;
  verification_level?: string;
  first_name?: string;
  last_name?: string;
  birth_date?: Date;
  zip?: string;
  phone?: string;
  access_token?: string;
  refresh_token?: string;
  token_expires_at?: Date;
  verified_at?: Date;
  last_synced_at: Date;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface IDmeGroupRecord {
  id: string;
  source_account_id: string;
  verification_id: string;
  user_id: string;
  group_type: string;
  group_name: string;
  verified: boolean;
  verified_at?: Date;
  expires_at?: Date;
  affiliation?: string;
  rank?: string;
  status?: string;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface IDmeBadgeRecord {
  id: string;
  source_account_id: string;
  verification_id: string;
  user_id: string;
  badge_type: string;
  badge_name: string;
  badge_icon?: string;
  badge_color?: string;
  verified_at?: Date;
  expires_at?: Date;
  active: boolean;
  display_order: number;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface IDmeAttributeRecord {
  id: string;
  source_account_id: string;
  verification_id: string;
  user_id: string;
  attribute_key: string;
  attribute_value?: string;
  attribute_type?: string;
  verified: boolean;
  verified_at?: Date;
  source: string;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface IDmeWebhookEvent {
  id: string;
  source_account_id: string;
  event_id?: string;
  event_type: string;
  user_id?: string;
  verification_id?: string;
  payload: Record<string, unknown>;
  processed: boolean;
  processed_at?: Date;
  error?: string;
  retry_count: number;
  created_at: Date;
  received_at: Date;
}

// Badge configuration
export const BADGE_CONFIG: Record<string, { name: string; icon: string; color: string }> = {
  military: { name: 'Military', icon: '🪖', color: '#2E7D32' },
  veteran: { name: 'Veteran', icon: '🎖️', color: '#1565C0' },
  first_responder: { name: 'First Responder', icon: '🚨', color: '#C62828' },
  government: { name: 'Government', icon: '🏛️', color: '#6A1B9A' },
  teacher: { name: 'Teacher', icon: '📚', color: '#F57C00' },
  student: { name: 'Student', icon: '🎓', color: '#00897B' },
  nurse: { name: 'Nurse', icon: '⚕️', color: '#AD1457' },
};

// Scope constants
export const IDME_SCOPES = {
  openid: 'openid',
  email: 'email',
  profile: 'profile',
  military: 'military',
  veteran: 'veteran',
  first_responder: 'first_responder',
  government: 'government',
  teacher: 'teacher',
  student: 'student',
  nurse: 'nurse',
} as const;
