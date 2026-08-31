package internal

import "time"

type Workflow struct {
    ID              string         `json:"id"`
    SourceAccountID string         `json:"source_account_id"`
    Name            string         `json:"name"`
    Description     string         `json:"description,omitempty"`
    Status          string         `json:"status"`
    Definition      map[string]any `json:"definition,omitempty"`
    Version         int            `json:"version"`
    CreatedAt       time.Time      `json:"created_at"`
    UpdatedAt       time.Time      `json:"updated_at"`
}

type Execution struct {
    ID              string     `json:"id"`
    SourceAccountID string     `json:"source_account_id"`
    WorkflowID      string     `json:"workflow_id"`
    Status          string     `json:"status"`
    TriggeredBy     string     `json:"triggered_by"`
    StartedAt       *time.Time `json:"started_at,omitempty"`
    CompletedAt     *time.Time `json:"completed_at,omitempty"`
    Error           string     `json:"error,omitempty"`
    CreatedAt       time.Time  `json:"created_at"`
}

type ExecutionStep struct {
    ID          string     `json:"id"`
    ExecutionID string     `json:"execution_id"`
    StepName    string     `json:"step_name"`
    Status      string     `json:"status"`
    StartedAt   *time.Time `json:"started_at,omitempty"`
    CompletedAt *time.Time `json:"completed_at,omitempty"`
    Error       string     `json:"error,omitempty"`
    CreatedAt   time.Time  `json:"created_at"`
}

type Trigger struct {
    ID              string         `json:"id"`
    SourceAccountID string         `json:"source_account_id"`
    WorkflowID      string         `json:"workflow_id"`
    TriggerType     string         `json:"trigger_type"`
    Config          map[string]any `json:"config,omitempty"`
    IsActive        bool           `json:"is_active"`
    CreatedAt       time.Time      `json:"created_at"`
    UpdatedAt       time.Time      `json:"updated_at"`
}

type Template struct {
    ID              string         `json:"id"`
    SourceAccountID string         `json:"source_account_id"`
    Name            string         `json:"name"`
    Description     string         `json:"description,omitempty"`
    Category        string         `json:"category,omitempty"`
    Definition      map[string]any `json:"definition,omitempty"`
    IsPublic        bool           `json:"is_public"`
    CreatedAt       time.Time      `json:"created_at"`
    UpdatedAt       time.Time      `json:"updated_at"`
}

type Variable struct {
    ID              string    `json:"id"`
    SourceAccountID string    `json:"source_account_id"`
    WorkflowID      string    `json:"workflow_id,omitempty"`
    Name            string    `json:"name"`
    Value           string    `json:"value"`
    IsSecret        bool      `json:"is_secret"`
    CreatedAt       time.Time `json:"created_at"`
    UpdatedAt       time.Time `json:"updated_at"`
}
