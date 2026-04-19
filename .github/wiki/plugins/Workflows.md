# Workflows

Automation engine providing trigger-action workflow chains, conditional logic, scheduled tasks, webhook integrations, and cross-plugin orchestration.

## Table of Contents
- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

## Overview

The Workflows plugin provides a powerful automation engine for nself applications. It enables users to create trigger-action workflows with conditional logic, scheduled execution, approval gates, cross-plugin integrations, and webhook delivery for comprehensive process automation.

This plugin is essential for applications requiring business process automation, notification systems, data synchronization, and complex multi-step operations.

### Key Features

- **Trigger-Action Workflows**: Define workflows with triggers that execute actions
- **Conditional Logic**: Branch workflows based on conditions
- **Scheduled Execution**: Run workflows on cron schedules
- **Approval Gates**: Human approval checkpoints in workflows
- **Cross-Plugin Integration**: Trigger and interact with other plugins
- **Webhook Integration**: Send and receive webhooks in workflows
- **Template Library**: Pre-built workflow templates for common scenarios
- **Variable Storage**: Store and reference data across workflow steps
- **Error Handling**: Retry policies and error recovery
- **Execution History**: Complete audit trail of all workflow runs
- **Parallel Execution**: Run multiple workflow steps concurrently
- **Multi-Account Isolation**: Full support for multi-tenant applications

### Supported Triggers

- **Schedule**: Cron-based time triggers
- **Webhook**: HTTP webhook endpoints
- **Event**: Internal application events
- **Manual**: User-initiated execution
- **Chain**: Triggered by another workflow completion

### Supported Actions

- **HTTP Request**: Call external APIs
- **Database Query**: Query/update database
- **Email**: Send email notifications
- **Plugin Action**: Trigger actions in other plugins
- **Transformation**: Transform and map data
- **Conditional**: If/else branching
- **Loop**: Iterate over collections
- **Delay**: Wait for specified duration
- **Approval**: Request human approval

### Use Cases

1. **Onboarding Automation**: Multi-step user onboarding sequences
2. **Notification Systems**: Complex notification routing and delivery
3. **Data Synchronization**: Keep data in sync across systems
4. **Approval Workflows**: Document review and approval processes
5. **Alert Escalation**: Escalate alerts based on conditions and time

## Quick Start

```bash
# Install the plugin
nself plugin install workflows

# Set environment variables
export DATABASE_URL="postgresql://user:pass@localhost:5432/mydb"
export WORKFLOWS_PLUGIN_PORT=3712

# Initialize database schema
nself plugin workflows init

# Start the workflows plugin server
nself plugin workflows server

# Check status
nself plugin workflows status
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `WORKFLOWS_PLUGIN_PORT` | No | `3712` | HTTP server port |
| `WORKFLOWS_DEFAULT_TIMEOUT` | No | `300` | Default execution timeout (seconds) |
| `WORKFLOWS_MAX_CONCURRENT` | No | `10` | Maximum concurrent executions |
| `WORKFLOWS_WORKER_POOL_SIZE` | No | `20` | Worker pool size |

### Example .env

```bash
# Database Configuration
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself

# Server Configuration
WORKFLOWS_PLUGIN_PORT=3712

# Execution Configuration
WORKFLOWS_DEFAULT_TIMEOUT=300
WORKFLOWS_MAX_CONCURRENT=10
WORKFLOWS_WORKER_POOL_SIZE=20
```

## CLI Commands

### Global Commands

#### `init`
Initialize the workflows plugin database schema.

```bash
nself plugin workflows init
```

#### `server`
Start the workflows plugin HTTP server.

```bash
nself plugin workflows server
```

#### `status`
Display current workflows plugin status.

```bash
nself plugin workflows status
```

### Workflow Management

#### `workflows`
Manage workflows.

```bash
nself plugin workflows create "User Onboarding" --trigger manual
nself plugin workflows list
nself plugin workflows info WORKFLOW_ID
nself plugin workflows publish WORKFLOW_ID
nself plugin workflows archive WORKFLOW_ID
```

### Execution Management

#### `executions`
Manage workflow executions.

```bash
nself plugin workflows executions list
nself plugin workflows executions info EXECUTION_ID
nself plugin workflows executions retry EXECUTION_ID
nself plugin workflows executions cancel EXECUTION_ID
```

### Trigger Management

#### `triggers`
Manage triggers.

```bash
nself plugin workflows triggers list
nself plugin workflows triggers create WORKFLOW_ID --type schedule --cron "0 9 * * *"
```

### Template Management

#### `templates`
Manage workflow templates.

```bash
nself plugin workflows templates list
nself plugin workflows templates create "Email Notification" --category notifications
```

### Variable Management

#### `variables`
Manage workflow variables.

```bash
nself plugin workflows variables list WORKFLOW_ID
nself plugin workflows variables set WORKFLOW_ID API_KEY "secret-key-value"
```

## REST API

### Workflow Management

#### `POST /api/workflows/workflows`
Create a workflow.

**Request:**
```json
{
  "name": "User Onboarding",
  "description": "Automated user onboarding sequence",
  "trigger": {
    "type": "event",
    "event": "user.created"
  },
  "steps": [
    {
      "id": "send_welcome_email",
      "type": "email",
      "config": {
        "to": "{{trigger.user.email}}",
        "subject": "Welcome!",
        "template": "welcome_email"
      }
    },
    {
      "id": "create_workspace",
      "type": "plugin_action",
      "config": {
        "plugin": "workspaces",
        "action": "create",
        "params": {
          "userId": "{{trigger.user.id}}"
        }
      }
    }
  ],
  "isActive": true
}
```

**Response:**
```json
{
  "success": true,
  "workflow": {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "name": "User Onboarding",
    "status": "draft",
    "version": 1
  }
}
```

#### `GET /api/workflows/workflows/:workflowId`
Get workflow details.

#### `PATCH /api/workflows/workflows/:workflowId`
Update workflow.

#### `POST /api/workflows/workflows/:workflowId/publish`
Publish workflow (make active).

#### `POST /api/workflows/workflows/:workflowId/archive`
Archive workflow.

#### `GET /api/workflows/workflows`
List workflows.

**Query Parameters:**
- `status` - Filter by status (draft, published, archived)
- `trigger` - Filter by trigger type
- `limit` - Result limit
- `offset` - Result offset

### Execution Management

#### `POST /api/workflows/workflows/:workflowId/execute`
Manually execute workflow.

**Request:**
```json
{
  "input": {
    "userId": "550e8400-e29b-41d4-a716-446655440000",
    "customData": {"key": "value"}
  }
}
```

**Response:**
```json
{
  "success": true,
  "execution": {
    "id": "550e8400-e29b-41d4-a716-446655440002",
    "workflowId": "550e8400-e29b-41d4-a716-446655440001",
    "status": "running",
    "startedAt": "2024-02-10T10:00:00Z"
  }
}
```

#### `GET /api/workflows/executions/:executionId`
Get execution details.

**Response:**
```json
{
  "success": true,
  "execution": {
    "id": "550e8400-e29b-41d4-a716-446655440002",
    "workflowId": "550e8400-e29b-41d4-a716-446655440001",
    "status": "completed",
    "startedAt": "2024-02-10T10:00:00Z",
    "completedAt": "2024-02-10T10:02:15Z",
    "duration": 135,
    "steps": [
      {
        "stepId": "send_welcome_email",
        "status": "completed",
        "output": {"messageId": "msg_123"}
      }
    ]
  }
}
```

#### `POST /api/workflows/executions/:executionId/retry`
Retry failed execution.

#### `POST /api/workflows/executions/:executionId/cancel`
Cancel running execution.

#### `GET /api/workflows/executions`
List executions.

**Query Parameters:**
- `workflowId` - Filter by workflow
- `status` - Filter by status (running, completed, failed, timeout)
- `startDate` - Filter by start date
- `endDate` - Filter by end date
- `limit` - Result limit
- `offset` - Result offset

### Trigger Management

#### `POST /api/workflows/triggers`
Create trigger.

**Request:**
```json
{
  "workflowId": "550e8400-e29b-41d4-a716-446655440001",
  "type": "schedule",
  "config": {
    "cron": "0 9 * * *",
    "timezone": "America/New_York"
  }
}
```

#### `GET /api/workflows/triggers`
List triggers.

#### `DELETE /api/workflows/triggers/:triggerId`
Delete trigger.

### Template Management

#### `POST /api/workflows/templates`
Create workflow template.

#### `GET /api/workflows/templates`
List templates.

#### `POST /api/workflows/templates/:templateId/instantiate`
Create workflow from template.

### Variable Management

#### `POST /api/workflows/workflows/:workflowId/variables`
Set workflow variable.

**Request:**
```json
{
  "name": "API_KEY",
  "value": "secret-key-value",
  "isSecret": true
}
```

#### `GET /api/workflows/workflows/:workflowId/variables`
List workflow variables.

#### `DELETE /api/workflows/workflows/:workflowId/variables/:variableName`
Delete variable.

### Approval Management

#### `GET /api/workflows/approvals`
List pending approvals.

#### `POST /api/workflows/approvals/:approvalId/respond`
Respond to approval request.

**Request:**
```json
{
  "action": "approve",
  "comment": "Looks good",
  "respondedBy": "550e8400-e29b-41d4-a716-446655440003"
}
```

### Webhook Endpoint

#### `POST /webhook`
Receive webhook events.

#### `POST /webhooks/:workflowId`
Webhook trigger endpoint for specific workflow.

## Webhook Events

### Workflow Events

#### `workflow.created`
A workflow was created.

#### `workflow.published`
A workflow was published.

#### `workflow.archived`
A workflow was archived.

### Execution Events

#### `execution.started`
A workflow execution started.

**Payload:**
```json
{
  "type": "execution.started",
  "execution": {
    "id": "550e8400-e29b-41d4-a716-446655440002",
    "workflowId": "550e8400-e29b-41d4-a716-446655440001",
    "trigger": "manual",
    "startedAt": "2024-02-10T10:00:00Z"
  },
  "timestamp": "2024-02-10T10:00:00Z"
}
```

#### `execution.completed`
A workflow execution completed successfully.

#### `execution.failed`
A workflow execution failed.

#### `execution.timeout`
A workflow execution timed out.

### Approval Events

#### `approval.required`
An approval gate requires attention.

**Payload:**
```json
{
  "type": "approval.required",
  "approval": {
    "id": "550e8400-e29b-41d4-a716-446655440004",
    "executionId": "550e8400-e29b-41d4-a716-446655440002",
    "approvers": ["550e8400-e29b-41d4-a716-446655440003"],
    "expiresAt": "2024-02-11T10:00:00Z"
  },
  "timestamp": "2024-02-10T10:00:00Z"
}
```

#### `approval.responded`
An approval was approved or rejected.

### Trigger Events

#### `trigger.fired`
A trigger was fired.

## Database Schema

### np_workflows_workflows

Workflow definitions.

```sql
CREATE TABLE np_workflows_workflows (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(200) NOT NULL,
  description TEXT,
  trigger_type VARCHAR(50) NOT NULL,
  trigger_config JSONB NOT NULL DEFAULT '{}'::jsonb,
  steps JSONB NOT NULL DEFAULT '[]'::jsonb,
  status VARCHAR(50) NOT NULL DEFAULT 'draft',
  version INTEGER NOT NULL DEFAULT 1,
  is_active BOOLEAN NOT NULL DEFAULT false,
  execution_count INTEGER DEFAULT 0,
  success_count INTEGER DEFAULT 0,
  failure_count INTEGER DEFAULT 0,
  last_executed_at TIMESTAMPTZ,
  timeout_seconds INTEGER DEFAULT 300,
  max_retries INTEGER DEFAULT 0,
  retry_delay_seconds INTEGER DEFAULT 60,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_by UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  published_at TIMESTAMPTZ,
  archived_at TIMESTAMPTZ
);

CREATE INDEX idx_workflows_account ON np_workflows_workflows(source_account_id);
CREATE INDEX idx_workflows_status ON np_workflows_workflows(status);
CREATE INDEX idx_workflows_active ON np_workflows_workflows(is_active) WHERE is_active = true;
CREATE INDEX idx_workflows_trigger ON np_workflows_workflows(trigger_type);
```

### np_workflows_executions

Workflow execution records.

```sql
CREATE TABLE np_workflows_executions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  workflow_id UUID NOT NULL REFERENCES np_workflows_workflows(id) ON DELETE CASCADE,
  workflow_version INTEGER NOT NULL,
  trigger_type VARCHAR(50) NOT NULL,
  trigger_data JSONB DEFAULT '{}'::jsonb,
  status VARCHAR(50) NOT NULL DEFAULT 'pending',
  input JSONB DEFAULT '{}'::jsonb,
  output JSONB DEFAULT '{}'::jsonb,
  error_message TEXT,
  error_stack TEXT,
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  duration_seconds INTEGER,
  retry_count INTEGER DEFAULT 0,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_executions_account ON np_workflows_executions(source_account_id);
CREATE INDEX idx_executions_workflow ON np_workflows_executions(workflow_id);
CREATE INDEX idx_executions_status ON np_workflows_executions(status);
CREATE INDEX idx_executions_started ON np_workflows_executions(started_at DESC);
```

### np_workflows_execution_steps

Individual execution steps.

```sql
CREATE TABLE np_workflows_execution_steps (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  execution_id UUID NOT NULL REFERENCES np_workflows_executions(id) ON DELETE CASCADE,
  step_id VARCHAR(100) NOT NULL,
  step_type VARCHAR(50) NOT NULL,
  step_config JSONB NOT NULL DEFAULT '{}'::jsonb,
  status VARCHAR(50) NOT NULL DEFAULT 'pending',
  input JSONB DEFAULT '{}'::jsonb,
  output JSONB DEFAULT '{}'::jsonb,
  error_message TEXT,
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  duration_seconds INTEGER,
  retry_count INTEGER DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_execution_steps_account ON np_workflows_execution_steps(source_account_id);
CREATE INDEX idx_execution_steps_execution ON np_workflows_execution_steps(execution_id);
CREATE INDEX idx_execution_steps_status ON np_workflows_execution_steps(status);
```

### np_workflows_triggers

Configured triggers.

```sql
CREATE TABLE np_workflows_triggers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  workflow_id UUID NOT NULL REFERENCES np_workflows_workflows(id) ON DELETE CASCADE,
  trigger_type VARCHAR(50) NOT NULL,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  is_active BOOLEAN NOT NULL DEFAULT true,
  last_fired_at TIMESTAMPTZ,
  fire_count INTEGER DEFAULT 0,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_triggers_account ON np_workflows_triggers(source_account_id);
CREATE INDEX idx_triggers_workflow ON np_workflows_triggers(workflow_id);
CREATE INDEX idx_triggers_active ON np_workflows_triggers(is_active) WHERE is_active = true;
```

### np_workflows_actions

Action definitions.

```sql
CREATE TABLE np_workflows_actions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(200) NOT NULL,
  description TEXT,
  action_type VARCHAR(50) NOT NULL,
  config_schema JSONB NOT NULL,
  is_builtin BOOLEAN NOT NULL DEFAULT false,
  is_active BOOLEAN NOT NULL DEFAULT true,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_actions_account ON np_workflows_actions(source_account_id);
CREATE INDEX idx_actions_type ON np_workflows_actions(action_type);
CREATE INDEX idx_actions_active ON np_workflows_actions(is_active) WHERE is_active = true;
```

### np_workflows_templates

Workflow templates.

```sql
CREATE TABLE np_workflows_templates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(200) NOT NULL,
  description TEXT,
  category VARCHAR(100),
  template_data JSONB NOT NULL,
  is_public BOOLEAN NOT NULL DEFAULT false,
  usage_count INTEGER DEFAULT 0,
  created_by UUID NOT NULL,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_templates_account ON np_workflows_templates(source_account_id);
CREATE INDEX idx_templates_category ON np_workflows_templates(category);
CREATE INDEX idx_templates_public ON np_workflows_templates(is_public) WHERE is_public = true;
```

### np_workflows_variables

Workflow variables storage.

```sql
CREATE TABLE np_workflows_variables (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  workflow_id UUID NOT NULL REFERENCES np_workflows_workflows(id) ON DELETE CASCADE,
  name VARCHAR(100) NOT NULL,
  value TEXT,
  value_encrypted TEXT,
  is_secret BOOLEAN NOT NULL DEFAULT false,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, workflow_id, name)
);

CREATE INDEX idx_variables_account ON np_workflows_variables(source_account_id);
CREATE INDEX idx_variables_workflow ON np_workflows_variables(workflow_id);
```

### np_workflows_webhook_logs

Webhook delivery logs.

```sql
CREATE TABLE np_workflows_webhook_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  workflow_id UUID NOT NULL REFERENCES np_workflows_workflows(id) ON DELETE CASCADE,
  execution_id UUID REFERENCES np_workflows_executions(id) ON DELETE CASCADE,
  url TEXT NOT NULL,
  method VARCHAR(10) NOT NULL,
  request_headers JSONB,
  request_body JSONB,
  response_status INTEGER,
  response_headers JSONB,
  response_body TEXT,
  error_message TEXT,
  duration_ms INTEGER,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_webhook_logs_account ON np_workflows_webhook_logs(source_account_id);
CREATE INDEX idx_webhook_logs_workflow ON np_workflows_webhook_logs(workflow_id);
CREATE INDEX idx_webhook_logs_execution ON np_workflows_webhook_logs(execution_id);
```

### np_workflows_approvals

Approval gate tracking.

```sql
CREATE TABLE np_workflows_approvals (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  execution_id UUID NOT NULL REFERENCES np_workflows_executions(id) ON DELETE CASCADE,
  step_id VARCHAR(100) NOT NULL,
  approvers UUID[] NOT NULL,
  required_approvals INTEGER DEFAULT 1,
  status VARCHAR(50) NOT NULL DEFAULT 'pending',
  responded_by UUID,
  response_action VARCHAR(50),
  response_comment TEXT,
  responded_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_approvals_account ON np_workflows_approvals(source_account_id);
CREATE INDEX idx_approvals_execution ON np_workflows_approvals(execution_id);
CREATE INDEX idx_approvals_status ON np_workflows_approvals(status);
CREATE INDEX idx_approvals_pending ON np_workflows_approvals(status, expires_at) WHERE status = 'pending';
```

## Examples

### Example 1: Create Simple Notification Workflow

```bash
# Create workflow
curl -X POST http://localhost:3712/api/workflows/workflows \
  -H "Content-Type: application/json" \
  -d '{
    "name": "New User Welcome",
    "trigger": {"type": "event", "event": "user.created"},
    "steps": [
      {
        "id": "send_email",
        "type": "email",
        "config": {
          "to": "{{trigger.user.email}}",
          "subject": "Welcome!",
          "template": "welcome_email"
        }
      }
    ]
  }'

# Publish workflow
curl -X POST http://localhost:3712/api/workflows/workflows/WORKFLOW_ID/publish
```

### Example 2: Scheduled Data Sync

```bash
# Create scheduled workflow
curl -X POST http://localhost:3712/api/workflows/workflows \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Daily Data Sync",
    "trigger": {
      "type": "schedule",
      "cron": "0 2 * * *"
    },
    "steps": [
      {
        "id": "fetch_data",
        "type": "http_request",
        "config": {
          "url": "https://api.example.com/data",
          "method": "GET"
        }
      },
      {
        "id": "save_data",
        "type": "database_query",
        "config": {
          "query": "INSERT INTO sync_data...",
          "params": ["{{steps.fetch_data.output}}"]
        }
      }
    ]
  }'
```

### Example 3: Approval Workflow

```bash
# Create approval workflow
curl -X POST http://localhost:3712/api/workflows/workflows \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Document Approval",
    "trigger": {"type": "manual"},
    "steps": [
      {
        "id": "request_approval",
        "type": "approval",
        "config": {
          "approvers": ["USER_ID_1", "USER_ID_2"],
          "required": 1,
          "timeout": 86400
        }
      },
      {
        "id": "publish_document",
        "type": "plugin_action",
        "config": {
          "plugin": "documents",
          "action": "publish"
        },
        "condition": "{{steps.request_approval.approved}}"
      }
    ]
  }'
```

### Example 4: Conditional Branching

```bash
# Workflow with conditions
curl -X POST http://localhost:3712/api/workflows/workflows \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Order Processing",
    "trigger": {"type": "event", "event": "order.created"},
    "steps": [
      {
        "id": "check_amount",
        "type": "conditional",
        "config": {
          "condition": "{{trigger.order.amount}} > 1000",
          "then": [
            {
              "id": "high_value_process",
              "type": "email",
              "config": {"to": "sales@example.com"}
            }
          ],
          "else": [
            {
              "id": "standard_process",
              "type": "email",
              "config": {"to": "orders@example.com"}
            }
          ]
        }
      }
    ]
  }'
```

### Example 5: Monitor Executions

```bash
# List recent executions
curl "http://localhost:3712/api/workflows/executions?workflowId=WORKFLOW_ID&limit=10"

# Get execution details
curl http://localhost:3712/api/workflows/executions/EXECUTION_ID

# Retry failed execution
curl -X POST http://localhost:3712/api/workflows/executions/EXECUTION_ID/retry
```

## Troubleshooting

### Execution Failures

**Problem:** Workflows failing unexpectedly

**Solutions:**
1. Check execution logs for error messages
2. Verify step configurations are correct
3. Test individual steps in isolation
4. Review variable values and data transformations
5. Check timeout settings

### Trigger Issues

**Problem:** Triggers not firing

**Solutions:**
1. Verify trigger is active: `SELECT * FROM np_workflows_triggers WHERE is_active = true`
2. Check cron syntax for scheduled triggers
3. Verify webhook URLs are accessible
4. Review event subscriptions for event triggers
5. Check trigger configuration

### Performance Issues

**Problem:** Slow workflow execution

**Solutions:**
1. Review execution steps for bottlenecks
2. Increase worker pool size
3. Add parallel execution where possible
4. Optimize database queries
5. Check external API response times

---

**Version:** 1.0.0
**Last Updated:** February 2024
**Support:** https://github.com/acamarata/nself-plugins/issues
