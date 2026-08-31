// Package controller implements the multi-tenant project lifecycle:
// create, delete, list, status, suspend, resume, migrate, rotate-credentials.
// All destructive operations run as the nself_controller Postgres role.
// No user-facing GraphQL mutation may call these functions directly.
package controller

import "time"

// Project mirrors nself_controller.projects.
type Project struct {
	ID           string     `json:"id"`
	Slug         string     `json:"slug"`
	Domain       string     `json:"domain"`
	SchemaName   string     `json:"schema_name"`
	RoleName     string     `json:"role_name"`
	HasuraSource string     `json:"hasura_source"`
	HasuraPort   int        `json:"hasura_port"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}

// CreateInput holds parameters for project creation.
type CreateInput struct {
	Slug   string
	Domain string
}

// StatusReport is the per-project status summary returned by ProjectStatus.
type StatusReport struct {
	Project        Project   `json:"project"`
	ConnectionCount int      `json:"connection_count"`
	SchemaBytes    int64     `json:"schema_bytes_approx"`
	LastActiveAt   time.Time `json:"last_active_at"`
	Healthy        bool      `json:"healthy"`
}

// ControllerStatus is the summary returned by ControllerStatus (all projects).
type ControllerStatus struct {
	Projects     []StatusReport `json:"projects"`
	TotalActive  int            `json:"total_active"`
	MaxProjects  int            `json:"max_projects"`
	UtilizationPct float64     `json:"utilization_pct"`
}

// AuditEventType enumerates known event types for project_audit_log.
type AuditEventType = string

const (
	EventProjectCreate         AuditEventType = "project_create"
	EventProjectDelete         AuditEventType = "project_delete"
	EventProjectMigrate        AuditEventType = "project_migrate"
	EventProjectSuspend        AuditEventType = "project_suspend"
	EventProjectResume         AuditEventType = "project_resume"
	EventRotateCredentials     AuditEventType = "project_rotate_credentials"
	EventSourceRegister        AuditEventType = "source_register"
	EventSourceDeregister      AuditEventType = "source_deregister"
	EventNginxReload           AuditEventType = "nginx_reload"
)

// Initiator prefix constants used in audit log initiated_by field.
const (
	InitiatorCLI   = "cli:nself_controller"
	InitiatorAdmin = "admin:user_id:"
)
