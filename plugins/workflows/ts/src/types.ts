/**
 * Workflows Plugin Type Definitions
 */

// =============================================================================
// Enums / Union Types
// =============================================================================

export type WorkflowStatus = 'draft' | 'published' | 'archived';
export type TriggerType = 'manual' | 'schedule' | 'webhook' | 'event';
export type ExecutionStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled' | 'timeout';
export type StepStatus = 'pending' | 'running' | 'completed' | 'failed' | 'skipped';
export type VariableType = 'string' | 'number' | 'boolean' | 'object' | 'array';
export type ApprovalStatus = 'pending' | 'approved' | 'rejected' | 'expired';

// =============================================================================
// Workflow Types
// =============================================================================

export interface Workflow {
  id: string;
  source_account_id: string;
  created_at: Date;
  updated_at: Date;

  name: string;
  description: string | null;

  owner_id: string;

  definition: Record<string, unknown>;

  status: WorkflowStatus;
  is_enabled: boolean;

  version: number;

  trigger_type: TriggerType | null;
  trigger_config: Record<string, unknown>;

  timeout_seconds: number;
  max_retries: number;
  retry_delay_seconds: number;

  total_executions: number;
  successful_executions: number;
  failed_executions: number;
  avg_duration_ms: number;

  is_public: boolean;
  is_template: boolean;

  tags: string[];

  metadata: Record<string, unknown>;
}

export interface CreateWorkflowInput {
  name: string;
  description?: string;
  owner_id: string;
  definition: Record<string, unknown>;
  trigger_type?: TriggerType;
  trigger_config?: Record<string, unknown>;
  timeout_seconds?: number;
  max_retries?: number;
  retry_delay_seconds?: number;
  tags?: string[];
  metadata?: Record<string, unknown>;
}

export interface UpdateWorkflowInput {
  name?: string;
  description?: string;
  definition?: Record<string, unknown>;
  trigger_type?: TriggerType;
  trigger_config?: Record<string, unknown>;
  timeout_seconds?: number;
  max_retries?: number;
  retry_delay_seconds?: number;
  is_enabled?: boolean;
  tags?: string[];
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Execution Types
// =============================================================================

export interface Execution {
  id: string;
  source_account_id: string;
  created_at: Date;
  updated_at: Date;

  workflow_id: string;
  workflow_version: number;

  triggered_by: TriggerType;
  triggered_by_user_id: string | null;

  status: ExecutionStatus;

  started_at: Date | null;
  completed_at: Date | null;
  duration_ms: number | null;

  input: Record<string, unknown>;
  output: Record<string, unknown>;

  error_message: string | null;
  error_stack: string | null;
  failed_step_id: string | null;

  retry_count: number;
  parent_execution_id: string | null;

  metadata: Record<string, unknown>;
}

export interface ExecuteWorkflowInput {
  triggered_by?: TriggerType;
  triggered_by_user_id?: string;
  input?: Record<string, unknown>;
}

// =============================================================================
// Execution Step Types
// =============================================================================

export interface ExecutionStep {
  id: string;
  source_account_id: string;
  created_at: Date;

  execution_id: string;

  step_id: string;
  step_type: string;
  step_name: string;

  status: StepStatus;

  started_at: Date | null;
  completed_at: Date | null;
  duration_ms: number | null;

  input: Record<string, unknown>;
  output: Record<string, unknown>;

  error_message: string | null;
  error_stack: string | null;

  metadata: Record<string, unknown>;
}

export interface CreateStepInput {
  execution_id: string;
  step_id: string;
  step_type: string;
  step_name: string;
  input?: Record<string, unknown>;
}

// =============================================================================
// Trigger Types
// =============================================================================

export interface Trigger {
  id: string;
  source_account_id: string;
  created_at: Date;
  updated_at: Date;

  workflow_id: string;

  type: TriggerType;

  schedule_cron: string | null;
  schedule_timezone: string;
  last_triggered_at: Date | null;
  next_trigger_at: Date | null;

  webhook_token: string | null;
  webhook_secret: string | null;

  event_type: string | null;
  event_filters: Record<string, unknown>;

  is_active: boolean;

  metadata: Record<string, unknown>;
}

export interface CreateTriggerInput {
  workflow_id: string;
  type: TriggerType;
  schedule_cron?: string;
  schedule_timezone?: string;
  webhook_token?: string;
  webhook_secret?: string;
  event_type?: string;
  event_filters?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

export interface UpdateTriggerInput {
  schedule_cron?: string;
  schedule_timezone?: string;
  event_type?: string;
  event_filters?: Record<string, unknown>;
  is_active?: boolean;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Action Types (Registry)
// =============================================================================

export interface Action {
  id: string;
  source_account_id: string;
  created_at: Date;

  type: string;
  name: string;
  description: string | null;
  category: string | null;

  input_schema: Record<string, unknown>;
  output_schema: Record<string, unknown>;

  icon: string | null;
  color: string | null;

  is_enabled: boolean;
  requires_auth: boolean;

  metadata: Record<string, unknown>;
}

// =============================================================================
// Template Types
// =============================================================================

export interface WorkflowTemplate {
  id: string;
  source_account_id: string;
  created_at: Date;
  updated_at: Date;

  name: string;
  description: string | null;
  category: string | null;

  definition: Record<string, unknown>;

  author_id: string | null;

  install_count: number;
  rating: number | null;
  review_count: number;

  is_public: boolean;
  is_featured: boolean;

  tags: string[];

  thumbnail_url: string | null;

  metadata: Record<string, unknown>;
}

export interface CreateTemplateInput {
  name: string;
  description?: string;
  category?: string;
  definition: Record<string, unknown>;
  author_id?: string;
  is_public?: boolean;
  tags?: string[];
  thumbnail_url?: string;
  metadata?: Record<string, unknown>;
}

export interface UpdateTemplateInput {
  name?: string;
  description?: string;
  category?: string;
  definition?: Record<string, unknown>;
  is_public?: boolean;
  tags?: string[];
  thumbnail_url?: string;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Variable Types
// =============================================================================

export interface Variable {
  id: string;
  source_account_id: string;
  created_at: Date;
  updated_at: Date;

  owner_id: string;
  workflow_id: string | null;

  key: string;
  value: unknown;

  type: VariableType;

  is_secret: boolean;

  metadata: Record<string, unknown>;
}

export interface CreateVariableInput {
  owner_id: string;
  workflow_id?: string;
  key: string;
  value: unknown;
  type: VariableType;
  is_secret?: boolean;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Webhook Log Types
// =============================================================================

export interface WebhookLog {
  id: string;
  source_account_id: string;
  created_at: Date;

  trigger_id: string;
  execution_id: string | null;

  method: string;
  path: string;
  headers: Record<string, unknown>;
  query: Record<string, unknown>;
  body: Record<string, unknown>;

  ip_address: string | null;
  user_agent: string | null;

  status_code: number | null;
  response: Record<string, unknown>;

  metadata: Record<string, unknown>;
}

// =============================================================================
// Approval Types
// =============================================================================

export interface Approval {
  id: string;
  source_account_id: string;
  created_at: Date;
  updated_at: Date;

  execution_id: string;
  step_id: string;

  message: string | null;
  required_approvers: string[];

  status: ApprovalStatus;

  approved_by: string | null;
  approved_at: Date | null;
  rejection_reason: string | null;

  expires_at: Date | null;

  metadata: Record<string, unknown>;
}

export interface ApprovalResponseInput {
  approved: boolean;
  approved_by: string;
  rejection_reason?: string;
}

// =============================================================================
// Configuration Types
// =============================================================================

export interface WorkflowsConfig {
  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl: boolean;
  };
  server: {
    port: number;
    host: string;
  };
  execution: {
    default_timeout_seconds: number;
    max_concurrent_executions: number;
    history_retention_days: number;
    worker_pool_size: number;
  };
  triggers: {
    schedule_check_interval: number;
    max_webhooks_per_workflow: number;
  };
  retries: {
    max_retries: number;
    initial_delay_seconds: number;
    backoff_multiplier: number;
  };
}

// =============================================================================
// API Query/Response Types
// =============================================================================

export interface ListWorkflowsQuery {
  status?: WorkflowStatus;
  is_enabled?: string;
  is_public?: string;
  owner_id?: string;
  limit?: string;
  offset?: string;
}

export interface ListExecutionsQuery {
  workflow_id?: string;
  status?: ExecutionStatus;
  triggered_by?: TriggerType;
  start_date?: string;
  end_date?: string;
  limit?: string;
  offset?: string;
}

export interface ListTemplatesQuery {
  category?: string;
  is_featured?: string;
  tags?: string;
  limit?: string;
  offset?: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  limit: number;
  offset: number;
}
