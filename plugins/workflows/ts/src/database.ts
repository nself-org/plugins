/**
 * Database client for workflows operations
 * Multi-app aware: all queries are scoped by source_account_id
 */

import { Pool, PoolClient } from 'pg';
import { config } from './config.js';
import {
  Workflow, CreateWorkflowInput, UpdateWorkflowInput,
  Execution, ExecuteWorkflowInput, ExecutionStatus,
  ExecutionStep, CreateStepInput,
  Trigger, CreateTriggerInput, UpdateTriggerInput,
  Action,
  WorkflowTemplate, CreateTemplateInput, UpdateTemplateInput,
  Variable, CreateVariableInput,
  WebhookLog,
  Approval, ApprovalResponseInput,
  ListWorkflowsQuery, ListExecutionsQuery, ListTemplatesQuery,
} from './types.js';
import crypto from 'crypto';

export class DatabaseClient {
  private pool: Pool;
  private sourceAccountId: string;

  constructor(sourceAccountId = 'primary') {
    this.pool = new Pool({
      host: config.database.host,
      port: config.database.port,
      database: config.database.database,
      user: config.database.user,
      password: config.database.password,
      ssl: config.database.ssl ? { rejectUnauthorized: false } : false,
    });
    this.sourceAccountId = sourceAccountId;
  }

  forSourceAccount(accountId: string): DatabaseClient {
    const scoped = Object.create(DatabaseClient.prototype) as DatabaseClient;
    scoped.pool = this.pool;
    scoped.sourceAccountId = accountId;
    return scoped;
  }

  getSourceAccountId(): string {
    return this.sourceAccountId;
  }

  async getClient(): Promise<PoolClient> {
    return this.pool.connect();
  }

  async close(): Promise<void> {
    await this.pool.end();
  }

  // =============================================================================
  // Schema Initialization
  // =============================================================================

  async initializeSchema(): Promise<void> {
    const client = await this.getClient();
    try {
      await client.query(`
        CREATE TABLE IF NOT EXISTS np_workflows_workflows (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          name TEXT NOT NULL,
          description TEXT,
          owner_id TEXT NOT NULL,
          definition JSONB NOT NULL DEFAULT '{}',
          status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'archived')),
          is_enabled BOOLEAN DEFAULT TRUE,
          version INT NOT NULL DEFAULT 1,
          trigger_type TEXT CHECK (trigger_type IN ('manual', 'schedule', 'webhook', 'event')),
          trigger_config JSONB DEFAULT '{}',
          timeout_seconds INT DEFAULT 300,
          max_retries INT DEFAULT 3,
          retry_delay_seconds INT DEFAULT 60,
          total_executions INT DEFAULT 0,
          successful_executions INT DEFAULT 0,
          failed_executions INT DEFAULT 0,
          avg_duration_ms INT DEFAULT 0,
          is_public BOOLEAN DEFAULT FALSE,
          is_template BOOLEAN DEFAULT FALSE,
          tags JSONB DEFAULT '[]',
          metadata JSONB DEFAULT '{}'
        );
        CREATE INDEX IF NOT EXISTS idx_np_workflows_workflows_owner ON np_workflows_workflows(owner_id);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_workflows_status ON np_workflows_workflows(status);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_workflows_enabled ON np_workflows_workflows(is_enabled);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_workflows_source ON np_workflows_workflows(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS np_workflows_executions (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          workflow_id UUID NOT NULL REFERENCES np_workflows_workflows(id) ON DELETE CASCADE,
          workflow_version INT NOT NULL,
          triggered_by TEXT NOT NULL CHECK (triggered_by IN ('manual', 'schedule', 'webhook', 'event')),
          triggered_by_user_id TEXT,
          status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled', 'timeout')),
          started_at TIMESTAMPTZ,
          completed_at TIMESTAMPTZ,
          duration_ms INT,
          input JSONB DEFAULT '{}',
          output JSONB DEFAULT '{}',
          error_message TEXT,
          error_stack TEXT,
          failed_step_id TEXT,
          retry_count INT DEFAULT 0,
          parent_execution_id UUID REFERENCES np_workflows_executions(id) ON DELETE SET NULL,
          metadata JSONB DEFAULT '{}'
        );
        CREATE INDEX IF NOT EXISTS idx_np_workflows_executions_workflow ON np_workflows_executions(workflow_id);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_executions_status ON np_workflows_executions(status);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_executions_created ON np_workflows_executions(created_at);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_executions_source ON np_workflows_executions(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS np_workflows_execution_steps (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          execution_id UUID NOT NULL REFERENCES np_workflows_executions(id) ON DELETE CASCADE,
          step_id TEXT NOT NULL,
          step_type TEXT NOT NULL,
          step_name TEXT NOT NULL,
          status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed', 'skipped')),
          started_at TIMESTAMPTZ,
          completed_at TIMESTAMPTZ,
          duration_ms INT,
          input JSONB DEFAULT '{}',
          output JSONB DEFAULT '{}',
          error_message TEXT,
          error_stack TEXT,
          metadata JSONB DEFAULT '{}'
        );
        CREATE INDEX IF NOT EXISTS idx_np_workflows_execution_steps_execution ON np_workflows_execution_steps(execution_id);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_execution_steps_status ON np_workflows_execution_steps(status);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_execution_steps_source ON np_workflows_execution_steps(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS np_workflows_triggers (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          workflow_id UUID NOT NULL REFERENCES np_workflows_workflows(id) ON DELETE CASCADE,
          type TEXT NOT NULL CHECK (type IN ('schedule', 'webhook', 'event')),
          schedule_cron TEXT,
          schedule_timezone TEXT DEFAULT 'UTC',
          last_triggered_at TIMESTAMPTZ,
          next_trigger_at TIMESTAMPTZ,
          webhook_token TEXT UNIQUE,
          webhook_secret TEXT,
          event_type TEXT,
          event_filters JSONB DEFAULT '{}',
          is_active BOOLEAN DEFAULT TRUE,
          metadata JSONB DEFAULT '{}'
        );
        CREATE INDEX IF NOT EXISTS idx_np_workflows_triggers_workflow ON np_workflows_triggers(workflow_id);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_triggers_type ON np_workflows_triggers(type);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_triggers_webhook ON np_workflows_triggers(webhook_token);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_triggers_source ON np_workflows_triggers(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS np_workflows_actions (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          type TEXT NOT NULL,
          name TEXT NOT NULL,
          description TEXT,
          category TEXT,
          input_schema JSONB NOT NULL DEFAULT '{}',
          output_schema JSONB NOT NULL DEFAULT '{}',
          icon TEXT,
          color TEXT,
          is_enabled BOOLEAN DEFAULT TRUE,
          requires_auth BOOLEAN DEFAULT FALSE,
          metadata JSONB DEFAULT '{}',
          UNIQUE(source_account_id, type)
        );
        CREATE INDEX IF NOT EXISTS idx_np_workflows_actions_type ON np_workflows_actions(type);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_actions_category ON np_workflows_actions(category);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_actions_source ON np_workflows_actions(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS np_workflows_templates (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          name TEXT NOT NULL,
          description TEXT,
          category TEXT,
          definition JSONB NOT NULL DEFAULT '{}',
          author_id TEXT,
          install_count INT DEFAULT 0,
          rating DECIMAL(3, 2),
          review_count INT DEFAULT 0,
          is_public BOOLEAN DEFAULT TRUE,
          is_featured BOOLEAN DEFAULT FALSE,
          tags JSONB DEFAULT '[]',
          thumbnail_url TEXT,
          metadata JSONB DEFAULT '{}'
        );
        CREATE INDEX IF NOT EXISTS idx_np_workflows_templates_category ON np_workflows_templates(category);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_templates_public ON np_workflows_templates(is_public);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_templates_source ON np_workflows_templates(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS np_workflows_variables (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          owner_id TEXT NOT NULL,
          workflow_id UUID REFERENCES np_workflows_workflows(id) ON DELETE CASCADE,
          key TEXT NOT NULL,
          value JSONB NOT NULL,
          type TEXT NOT NULL DEFAULT 'string' CHECK (type IN ('string', 'number', 'boolean', 'object', 'array')),
          is_secret BOOLEAN DEFAULT FALSE,
          metadata JSONB DEFAULT '{}'
        );
        CREATE INDEX IF NOT EXISTS idx_np_workflows_variables_owner ON np_workflows_variables(owner_id);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_variables_workflow ON np_workflows_variables(workflow_id);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_variables_key ON np_workflows_variables(key);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_variables_source ON np_workflows_variables(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS np_workflows_webhook_logs (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          trigger_id UUID NOT NULL REFERENCES np_workflows_triggers(id) ON DELETE CASCADE,
          execution_id UUID REFERENCES np_workflows_executions(id) ON DELETE SET NULL,
          method TEXT NOT NULL,
          path TEXT NOT NULL,
          headers JSONB DEFAULT '{}',
          query JSONB DEFAULT '{}',
          body JSONB DEFAULT '{}',
          ip_address INET,
          user_agent TEXT,
          status_code INT,
          response JSONB DEFAULT '{}',
          metadata JSONB DEFAULT '{}'
        );
        CREATE INDEX IF NOT EXISTS idx_np_workflows_webhook_logs_trigger ON np_workflows_webhook_logs(trigger_id);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_webhook_logs_created ON np_workflows_webhook_logs(created_at);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_webhook_logs_source ON np_workflows_webhook_logs(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS np_workflows_approvals (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          execution_id UUID NOT NULL REFERENCES np_workflows_executions(id) ON DELETE CASCADE,
          step_id TEXT NOT NULL,
          message TEXT,
          required_approvers JSONB NOT NULL DEFAULT '[]',
          status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'expired')),
          approved_by TEXT,
          approved_at TIMESTAMPTZ,
          rejection_reason TEXT,
          expires_at TIMESTAMPTZ,
          metadata JSONB DEFAULT '{}'
        );
        CREATE INDEX IF NOT EXISTS idx_np_workflows_approvals_execution ON np_workflows_approvals(execution_id);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_approvals_status ON np_workflows_approvals(status);
        CREATE INDEX IF NOT EXISTS idx_np_workflows_approvals_source ON np_workflows_approvals(source_account_id);
      `);
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Workflows CRUD
  // =============================================================================

  async createWorkflow(input: CreateWorkflowInput): Promise<Workflow> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO np_workflows_workflows (
          source_account_id, name, description, owner_id, definition,
          trigger_type, trigger_config, timeout_seconds, max_retries,
          retry_delay_seconds, tags, metadata
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING *`,
        [
          this.sourceAccountId, input.name, input.description ?? null,
          input.owner_id, JSON.stringify(input.definition),
          input.trigger_type ?? null, JSON.stringify(input.trigger_config ?? {}),
          input.timeout_seconds ?? 300, input.max_retries ?? 3,
          input.retry_delay_seconds ?? 60, JSON.stringify(input.tags ?? []),
          JSON.stringify(input.metadata ?? {}),
        ]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async getWorkflow(id: string): Promise<Workflow | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM np_workflows_workflows WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async listWorkflows(query: ListWorkflowsQuery): Promise<{ workflows: Workflow[]; total: number }> {
    const client = await this.getClient();
    try {
      const conditions: string[] = ['source_account_id = $1'];
      const params: unknown[] = [this.sourceAccountId];
      let paramIdx = 2;

      if (query.status) {
        conditions.push(`status = $${paramIdx}`);
        params.push(query.status);
        paramIdx++;
      }
      if (query.is_enabled !== undefined) {
        conditions.push(`is_enabled = $${paramIdx}`);
        params.push(query.is_enabled === 'true');
        paramIdx++;
      }
      if (query.is_public !== undefined) {
        conditions.push(`is_public = $${paramIdx}`);
        params.push(query.is_public === 'true');
        paramIdx++;
      }
      if (query.owner_id) {
        conditions.push(`owner_id = $${paramIdx}`);
        params.push(query.owner_id);
        paramIdx++;
      }

      const where = conditions.join(' AND ');
      const limit = parseInt(query.limit ?? '50', 10);
      const offset = parseInt(query.offset ?? '0', 10);

      const countResult = await client.query(`SELECT COUNT(*) FROM np_workflows_workflows WHERE ${where}`, params);
      const total = parseInt(countResult.rows[0].count, 10);

      params.push(limit, offset);
      const result = await client.query(
        `SELECT * FROM np_workflows_workflows WHERE ${where} ORDER BY updated_at DESC LIMIT $${paramIdx} OFFSET $${paramIdx + 1}`,
        params
      );
      return { workflows: result.rows, total };
    } finally {
      client.release();
    }
  }

  async updateWorkflow(id: string, input: UpdateWorkflowInput): Promise<Workflow | null> {
    const client = await this.getClient();
    try {
      const fields: string[] = ['updated_at = NOW()'];
      const params: unknown[] = [];
      let paramIdx = 1;

      const fieldMap: Record<string, unknown> = {
        name: input.name, description: input.description,
        trigger_type: input.trigger_type, timeout_seconds: input.timeout_seconds,
        max_retries: input.max_retries, retry_delay_seconds: input.retry_delay_seconds,
        is_enabled: input.is_enabled,
      };

      for (const [key, value] of Object.entries(fieldMap)) {
        if (value !== undefined) {
          fields.push(`${key} = $${paramIdx}`);
          params.push(value);
          paramIdx++;
        }
      }
      if (input.definition !== undefined) {
        fields.push(`definition = $${paramIdx}`);
        params.push(JSON.stringify(input.definition));
        paramIdx++;
        // Increment version on definition changes
        fields.push('version = version + 1');
      }
      if (input.trigger_config !== undefined) {
        fields.push(`trigger_config = $${paramIdx}`);
        params.push(JSON.stringify(input.trigger_config));
        paramIdx++;
      }
      if (input.tags !== undefined) {
        fields.push(`tags = $${paramIdx}`);
        params.push(JSON.stringify(input.tags));
        paramIdx++;
      }
      if (input.metadata !== undefined) {
        fields.push(`metadata = $${paramIdx}`);
        params.push(JSON.stringify(input.metadata));
        paramIdx++;
      }

      params.push(id, this.sourceAccountId);
      const result = await client.query(
        `UPDATE np_workflows_workflows SET ${fields.join(', ')} WHERE id = $${paramIdx} AND source_account_id = $${paramIdx + 1} RETURNING *`,
        params
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async deleteWorkflow(id: string): Promise<boolean> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'DELETE FROM np_workflows_workflows WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return (result.rowCount ?? 0) > 0;
    } finally {
      client.release();
    }
  }

  async duplicateWorkflow(id: string, newName: string): Promise<Workflow | null> {
    const client = await this.getClient();
    try {
      const original = await this.getWorkflow(id);
      if (!original) return null;

      const result = await client.query(
        `INSERT INTO np_workflows_workflows (
          source_account_id, name, description, owner_id, definition,
          trigger_type, trigger_config, timeout_seconds, max_retries,
          retry_delay_seconds, tags, metadata
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING *`,
        [
          this.sourceAccountId, newName, original.description,
          original.owner_id, JSON.stringify(original.definition),
          original.trigger_type, JSON.stringify(original.trigger_config),
          original.timeout_seconds, original.max_retries,
          original.retry_delay_seconds, JSON.stringify(original.tags),
          JSON.stringify(original.metadata),
        ]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async publishWorkflow(id: string): Promise<Workflow | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `UPDATE np_workflows_workflows SET status = 'published', updated_at = NOW()
         WHERE id = $1 AND source_account_id = $2 RETURNING *`,
        [id, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async unpublishWorkflow(id: string): Promise<Workflow | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `UPDATE np_workflows_workflows SET status = 'draft', updated_at = NOW()
         WHERE id = $1 AND source_account_id = $2 RETURNING *`,
        [id, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Executions
  // =============================================================================

  async createExecution(workflowId: string, input: ExecuteWorkflowInput): Promise<Execution> {
    const client = await this.getClient();
    try {
      const workflow = await this.getWorkflow(workflowId);
      if (!workflow) throw new Error('Workflow not found');

      const result = await client.query(
        `INSERT INTO np_workflows_executions (
          source_account_id, workflow_id, workflow_version, triggered_by,
          triggered_by_user_id, input, status
        ) VALUES ($1, $2, $3, $4, $5, $6, 'pending') RETURNING *`,
        [
          this.sourceAccountId, workflowId, workflow.version,
          input.triggered_by ?? 'manual', input.triggered_by_user_id ?? null,
          JSON.stringify(input.input ?? {}),
        ]
      );

      await client.query(
        'UPDATE np_workflows_workflows SET total_executions = total_executions + 1 WHERE id = $1 AND source_account_id = $2',
        [workflowId, this.sourceAccountId]
      );

      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async getExecution(id: string): Promise<Execution | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM np_workflows_executions WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async listExecutions(query: ListExecutionsQuery): Promise<{ executions: Execution[]; total: number }> {
    const client = await this.getClient();
    try {
      const conditions: string[] = ['source_account_id = $1'];
      const params: unknown[] = [this.sourceAccountId];
      let paramIdx = 2;

      if (query.workflow_id) {
        conditions.push(`workflow_id = $${paramIdx}`);
        params.push(query.workflow_id);
        paramIdx++;
      }
      if (query.status) {
        conditions.push(`status = $${paramIdx}`);
        params.push(query.status);
        paramIdx++;
      }
      if (query.triggered_by) {
        conditions.push(`triggered_by = $${paramIdx}`);
        params.push(query.triggered_by);
        paramIdx++;
      }
      if (query.start_date) {
        conditions.push(`created_at >= $${paramIdx}`);
        params.push(query.start_date);
        paramIdx++;
      }
      if (query.end_date) {
        conditions.push(`created_at <= $${paramIdx}`);
        params.push(query.end_date);
        paramIdx++;
      }

      const where = conditions.join(' AND ');
      const limit = parseInt(query.limit ?? '50', 10);
      const offset = parseInt(query.offset ?? '0', 10);

      const countResult = await client.query(`SELECT COUNT(*) FROM np_workflows_executions WHERE ${where}`, params);
      const total = parseInt(countResult.rows[0].count, 10);

      params.push(limit, offset);
      const result = await client.query(
        `SELECT * FROM np_workflows_executions WHERE ${where} ORDER BY created_at DESC LIMIT $${paramIdx} OFFSET $${paramIdx + 1}`,
        params
      );
      return { executions: result.rows, total };
    } finally {
      client.release();
    }
  }

  async updateExecutionStatus(id: string, status: ExecutionStatus, updates: Partial<Record<string, unknown>> = {}): Promise<Execution | null> {
    const client = await this.getClient();
    try {
      const fields: string[] = ['status = $1', 'updated_at = NOW()'];
      const params: unknown[] = [status];
      let paramIdx = 2;

      if (status === 'running') {
        fields.push('started_at = NOW()');
      }
      if (status === 'completed' || status === 'failed' || status === 'cancelled' || status === 'timeout') {
        fields.push('completed_at = NOW()');
        fields.push(`duration_ms = EXTRACT(EPOCH FROM (NOW() - started_at))::INT * 1000`);
      }
      if (updates.error_message !== undefined) {
        fields.push(`error_message = $${paramIdx}`);
        params.push(updates.error_message);
        paramIdx++;
      }
      if (updates.error_stack !== undefined) {
        fields.push(`error_stack = $${paramIdx}`);
        params.push(updates.error_stack);
        paramIdx++;
      }
      if (updates.failed_step_id !== undefined) {
        fields.push(`failed_step_id = $${paramIdx}`);
        params.push(updates.failed_step_id);
        paramIdx++;
      }
      if (updates.output !== undefined) {
        fields.push(`output = $${paramIdx}`);
        params.push(JSON.stringify(updates.output));
        paramIdx++;
      }

      params.push(id, this.sourceAccountId);
      const result = await client.query(
        `UPDATE np_workflows_executions SET ${fields.join(', ')} WHERE id = $${paramIdx} AND source_account_id = $${paramIdx + 1} RETURNING *`,
        params
      );

      // Update workflow stats
      const execution = result.rows[0];
      if (execution && (status === 'completed' || status === 'failed')) {
        const statField = status === 'completed' ? 'successful_executions' : 'failed_executions';
        await client.query(
          `UPDATE np_workflows_workflows SET ${statField} = ${statField} + 1 WHERE id = $1 AND source_account_id = $2`,
          [execution.workflow_id, this.sourceAccountId]
        );
      }

      return execution ?? null;
    } finally {
      client.release();
    }
  }

  async retryExecution(id: string): Promise<Execution | null> {
    const client = await this.getClient();
    try {
      const original = await this.getExecution(id);
      if (!original) return null;

      const result = await client.query(
        `INSERT INTO np_workflows_executions (
          source_account_id, workflow_id, workflow_version, triggered_by,
          triggered_by_user_id, input, status, retry_count, parent_execution_id
        ) VALUES ($1, $2, $3, $4, $5, $6, 'pending', $7, $8) RETURNING *`,
        [
          this.sourceAccountId, original.workflow_id, original.workflow_version,
          original.triggered_by, original.triggered_by_user_id,
          JSON.stringify(original.input), original.retry_count + 1, id,
        ]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async cancelExecution(id: string): Promise<boolean> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `UPDATE np_workflows_executions SET status = 'cancelled', completed_at = NOW(), updated_at = NOW()
         WHERE id = $1 AND source_account_id = $2 AND status IN ('pending', 'running') RETURNING id`,
        [id, this.sourceAccountId]
      );
      return (result.rowCount ?? 0) > 0;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Execution Steps
  // =============================================================================

  async createStep(input: CreateStepInput): Promise<ExecutionStep> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO np_workflows_execution_steps (
          source_account_id, execution_id, step_id, step_type, step_name, input
        ) VALUES ($1, $2, $3, $4, $5, $6) RETURNING *`,
        [
          this.sourceAccountId, input.execution_id, input.step_id,
          input.step_type, input.step_name, JSON.stringify(input.input ?? {}),
        ]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async getExecutionSteps(executionId: string): Promise<ExecutionStep[]> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM np_workflows_execution_steps WHERE execution_id = $1 AND source_account_id = $2 ORDER BY created_at',
        [executionId, this.sourceAccountId]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Triggers
  // =============================================================================

  async createTrigger(input: CreateTriggerInput): Promise<Trigger> {
    const client = await this.getClient();
    try {
      const webhookToken = input.type === 'webhook' ? (input.webhook_token ?? crypto.randomUUID()) : null;
      const webhookSecret = input.type === 'webhook' ? (input.webhook_secret ?? crypto.randomBytes(32).toString('hex')) : null;

      const result = await client.query(
        `INSERT INTO np_workflows_triggers (
          source_account_id, workflow_id, type, schedule_cron, schedule_timezone,
          webhook_token, webhook_secret, event_type, event_filters, metadata
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING *`,
        [
          this.sourceAccountId, input.workflow_id, input.type,
          input.schedule_cron ?? null, input.schedule_timezone ?? 'UTC',
          webhookToken, webhookSecret,
          input.event_type ?? null, JSON.stringify(input.event_filters ?? {}),
          JSON.stringify(input.metadata ?? {}),
        ]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async getTrigger(id: string): Promise<Trigger | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM np_workflows_triggers WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async getTriggerByWebhookToken(token: string): Promise<Trigger | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM np_workflows_triggers WHERE webhook_token = $1 AND is_active = TRUE',
        [token]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async listTriggers(workflowId?: string): Promise<Trigger[]> {
    const client = await this.getClient();
    try {
      if (workflowId) {
        const result = await client.query(
          'SELECT * FROM np_workflows_triggers WHERE workflow_id = $1 AND source_account_id = $2 ORDER BY created_at',
          [workflowId, this.sourceAccountId]
        );
        return result.rows;
      }
      const result = await client.query(
        'SELECT * FROM np_workflows_triggers WHERE source_account_id = $1 ORDER BY created_at',
        [this.sourceAccountId]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  async updateTrigger(id: string, input: UpdateTriggerInput): Promise<Trigger | null> {
    const client = await this.getClient();
    try {
      const fields: string[] = ['updated_at = NOW()'];
      const params: unknown[] = [];
      let paramIdx = 1;

      const fieldMap: Record<string, unknown> = {
        schedule_cron: input.schedule_cron, schedule_timezone: input.schedule_timezone,
        event_type: input.event_type, is_active: input.is_active,
      };

      for (const [key, value] of Object.entries(fieldMap)) {
        if (value !== undefined) {
          fields.push(`${key} = $${paramIdx}`);
          params.push(value);
          paramIdx++;
        }
      }
      if (input.event_filters !== undefined) {
        fields.push(`event_filters = $${paramIdx}`);
        params.push(JSON.stringify(input.event_filters));
        paramIdx++;
      }
      if (input.metadata !== undefined) {
        fields.push(`metadata = $${paramIdx}`);
        params.push(JSON.stringify(input.metadata));
        paramIdx++;
      }

      params.push(id, this.sourceAccountId);
      const result = await client.query(
        `UPDATE np_workflows_triggers SET ${fields.join(', ')} WHERE id = $${paramIdx} AND source_account_id = $${paramIdx + 1} RETURNING *`,
        params
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async deleteTrigger(id: string): Promise<boolean> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'DELETE FROM np_workflows_triggers WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return (result.rowCount ?? 0) > 0;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Actions
  // =============================================================================

  async listActions(category?: string): Promise<Action[]> {
    const client = await this.getClient();
    try {
      if (category) {
        const result = await client.query(
          'SELECT * FROM np_workflows_actions WHERE source_account_id = $1 AND category = $2 AND is_enabled = TRUE ORDER BY name',
          [this.sourceAccountId, category]
        );
        return result.rows;
      }
      const result = await client.query(
        'SELECT * FROM np_workflows_actions WHERE source_account_id = $1 AND is_enabled = TRUE ORDER BY category, name',
        [this.sourceAccountId]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  async getAction(type: string): Promise<Action | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM np_workflows_actions WHERE type = $1 AND source_account_id = $2',
        [type, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Templates
  // =============================================================================

  async createTemplate(input: CreateTemplateInput): Promise<WorkflowTemplate> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO np_workflows_templates (
          source_account_id, name, description, category, definition,
          author_id, is_public, tags, thumbnail_url, metadata
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING *`,
        [
          this.sourceAccountId, input.name, input.description ?? null,
          input.category ?? null, JSON.stringify(input.definition),
          input.author_id ?? null, input.is_public ?? true,
          JSON.stringify(input.tags ?? []), input.thumbnail_url ?? null,
          JSON.stringify(input.metadata ?? {}),
        ]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async getTemplate(id: string): Promise<WorkflowTemplate | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM np_workflows_templates WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async listTemplates(query: ListTemplatesQuery): Promise<{ templates: WorkflowTemplate[]; total: number }> {
    const client = await this.getClient();
    try {
      const conditions: string[] = ['source_account_id = $1'];
      const params: unknown[] = [this.sourceAccountId];
      let paramIdx = 2;

      if (query.category) {
        conditions.push(`category = $${paramIdx}`);
        params.push(query.category);
        paramIdx++;
      }
      if (query.is_featured !== undefined) {
        conditions.push(`is_featured = $${paramIdx}`);
        params.push(query.is_featured === 'true');
        paramIdx++;
      }

      const where = conditions.join(' AND ');
      const limit = parseInt(query.limit ?? '50', 10);
      const offset = parseInt(query.offset ?? '0', 10);

      const countResult = await client.query(`SELECT COUNT(*) FROM np_workflows_templates WHERE ${where}`, params);
      const total = parseInt(countResult.rows[0].count, 10);

      params.push(limit, offset);
      const result = await client.query(
        `SELECT * FROM np_workflows_templates WHERE ${where} ORDER BY install_count DESC LIMIT $${paramIdx} OFFSET $${paramIdx + 1}`,
        params
      );
      return { templates: result.rows, total };
    } finally {
      client.release();
    }
  }

  async updateTemplate(id: string, input: UpdateTemplateInput): Promise<WorkflowTemplate | null> {
    const client = await this.getClient();
    try {
      const fields: string[] = ['updated_at = NOW()'];
      const params: unknown[] = [];
      let paramIdx = 1;

      const fieldMap: Record<string, unknown> = {
        name: input.name, description: input.description, category: input.category,
        is_public: input.is_public, thumbnail_url: input.thumbnail_url,
      };

      for (const [key, value] of Object.entries(fieldMap)) {
        if (value !== undefined) {
          fields.push(`${key} = $${paramIdx}`);
          params.push(value);
          paramIdx++;
        }
      }
      if (input.definition !== undefined) {
        fields.push(`definition = $${paramIdx}`);
        params.push(JSON.stringify(input.definition));
        paramIdx++;
      }
      if (input.tags !== undefined) {
        fields.push(`tags = $${paramIdx}`);
        params.push(JSON.stringify(input.tags));
        paramIdx++;
      }
      if (input.metadata !== undefined) {
        fields.push(`metadata = $${paramIdx}`);
        params.push(JSON.stringify(input.metadata));
        paramIdx++;
      }

      params.push(id, this.sourceAccountId);
      const result = await client.query(
        `UPDATE np_workflows_templates SET ${fields.join(', ')} WHERE id = $${paramIdx} AND source_account_id = $${paramIdx + 1} RETURNING *`,
        params
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async deleteTemplate(id: string): Promise<boolean> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'DELETE FROM np_workflows_templates WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return (result.rowCount ?? 0) > 0;
    } finally {
      client.release();
    }
  }

  async installTemplate(templateId: string, name: string, ownerId: string): Promise<Workflow | null> {
    const client = await this.getClient();
    try {
      const template = await this.getTemplate(templateId);
      if (!template) return null;

      // Increment install count
      await client.query(
        'UPDATE np_workflows_templates SET install_count = install_count + 1 WHERE id = $1 AND source_account_id = $2',
        [templateId, this.sourceAccountId]
      );

      const result = await client.query(
        `INSERT INTO np_workflows_workflows (
          source_account_id, name, description, owner_id, definition, tags, metadata
        ) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *`,
        [
          this.sourceAccountId, name, template.description, ownerId,
          JSON.stringify(template.definition), JSON.stringify(template.tags),
          JSON.stringify({ installed_from_template: templateId }),
        ]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Variables
  // =============================================================================

  async createVariable(input: CreateVariableInput): Promise<Variable> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO np_workflows_variables (
          source_account_id, owner_id, workflow_id, key, value, type, is_secret, metadata
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *`,
        [
          this.sourceAccountId, input.owner_id, input.workflow_id ?? null,
          input.key, JSON.stringify(input.value), input.type,
          input.is_secret ?? false, JSON.stringify(input.metadata ?? {}),
        ]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async getVariable(id: string): Promise<Variable | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM np_workflows_variables WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async listVariables(workflowId?: string): Promise<Variable[]> {
    const client = await this.getClient();
    try {
      if (workflowId) {
        const result = await client.query(
          'SELECT * FROM np_workflows_variables WHERE workflow_id = $1 AND source_account_id = $2 ORDER BY key',
          [workflowId, this.sourceAccountId]
        );
        return result.rows;
      }
      const result = await client.query(
        'SELECT * FROM np_workflows_variables WHERE source_account_id = $1 ORDER BY key',
        [this.sourceAccountId]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  async updateVariable(id: string, value: unknown): Promise<Variable | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `UPDATE np_workflows_variables SET value = $1, updated_at = NOW()
         WHERE id = $2 AND source_account_id = $3 RETURNING *`,
        [JSON.stringify(value), id, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async deleteVariable(id: string): Promise<boolean> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'DELETE FROM np_workflows_variables WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return (result.rowCount ?? 0) > 0;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Approvals
  // =============================================================================

  async getPendingApprovals(): Promise<Approval[]> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `SELECT * FROM np_workflows_approvals WHERE source_account_id = $1 AND status = 'pending'
         AND (expires_at IS NULL OR expires_at > NOW()) ORDER BY created_at`,
        [this.sourceAccountId]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  async respondToApproval(approvalId: string, input: ApprovalResponseInput): Promise<Approval | null> {
    const client = await this.getClient();
    try {
      const status = input.approved ? 'approved' : 'rejected';
      const result = await client.query(
        `UPDATE np_workflows_approvals SET status = $1, approved_by = $2, approved_at = NOW(),
         rejection_reason = $3, updated_at = NOW()
         WHERE id = $4 AND source_account_id = $5 AND status = 'pending' RETURNING *`,
        [status, input.approved_by, input.rejection_reason ?? null, approvalId, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Webhook processing endpoint
  // =============================================================================

  async logWebhook(triggerId: string, method: string, path: string, headers: Record<string, unknown>, query: Record<string, unknown>, body: Record<string, unknown>, ipAddress: string | null, userAgent: string | null): Promise<WebhookLog> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO np_workflows_webhook_logs (
          source_account_id, trigger_id, method, path, headers, query, body, ip_address, user_agent
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::INET, $9) RETURNING *`,
        [
          this.sourceAccountId, triggerId, method, path,
          JSON.stringify(headers), JSON.stringify(query),
          JSON.stringify(body), ipAddress, userAgent,
        ]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Statistics
  // =============================================================================

  async getStats(): Promise<Record<string, number>> {
    const client = await this.getClient();
    try {
      const workflows = await client.query('SELECT COUNT(*) FROM np_workflows_workflows WHERE source_account_id = $1', [this.sourceAccountId]);
      const published = await client.query(`SELECT COUNT(*) FROM np_workflows_workflows WHERE source_account_id = $1 AND status = 'published'`, [this.sourceAccountId]);
      const executions = await client.query('SELECT COUNT(*) FROM np_workflows_executions WHERE source_account_id = $1', [this.sourceAccountId]);
      const triggers = await client.query('SELECT COUNT(*) FROM np_workflows_triggers WHERE source_account_id = $1', [this.sourceAccountId]);
      const templates = await client.query('SELECT COUNT(*) FROM np_workflows_templates WHERE source_account_id = $1', [this.sourceAccountId]);
      const pendingApprovals = await client.query(`SELECT COUNT(*) FROM np_workflows_approvals WHERE source_account_id = $1 AND status = 'pending'`, [this.sourceAccountId]);

      return {
        total_workflows: parseInt(workflows.rows[0].count, 10),
        published_workflows: parseInt(published.rows[0].count, 10),
        total_executions: parseInt(executions.rows[0].count, 10),
        total_triggers: parseInt(triggers.rows[0].count, 10),
        total_templates: parseInt(templates.rows[0].count, 10),
        pending_approvals: parseInt(pendingApprovals.rows[0].count, 10),
      };
    } finally {
      client.release();
    }
  }
}

export const db = new DatabaseClient();
