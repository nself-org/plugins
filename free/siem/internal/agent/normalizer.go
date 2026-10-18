// Package agent implements the SIEM event pipeline: read, normalize, batch, ship.
package agent

import (
	"encoding/json"
	"fmt"
	"time"
)

// OCSFEvent is a minimal OCSF (Open Cybersecurity Schema Framework) envelope
// for class_uid 3002 (Authentication Activity).
type OCSFEvent struct {
	ClassUID    int            `json:"class_uid"`
	CategoryUID int            `json:"category_uid"`
	Time        int64          `json:"time"` // Unix ms
	SeverityID  int            `json:"severity_id"`
	TypeUID     int            `json:"type_uid"`
	Actor       OCSFActor      `json:"actor"`
	DstEndpoint OCSFEndpoint   `json:"dst_endpoint,omitempty"`
	Metadata    OCSFMetadata   `json:"metadata"`
	RawData     string         `json:"raw_data,omitempty"`
	Unmapped    map[string]any `json:"unmapped,omitempty"`
}

// OCSFActor describes the entity that performed the action.
type OCSFActor struct {
	User OCSFUser `json:"user"`
}

// OCSFUser identifies a user.
type OCSFUser struct {
	UID  string `json:"uid,omitempty"`
	Name string `json:"name,omitempty"`
}

// OCSFEndpoint describes a network endpoint.
type OCSFEndpoint struct {
	IP   string `json:"ip,omitempty"`
	Port int    `json:"port,omitempty"`
}

// OCSFMetadata carries product metadata.
type OCSFMetadata struct {
	Product OCSFProduct `json:"product"`
	Version string      `json:"version"`
}

// OCSFProduct identifies the originating product.
type OCSFProduct struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ECSEvent is a minimal Elastic Common Schema envelope.
type ECSEvent struct {
	Timestamp string         `json:"@timestamp"`
	Event     ECSEventField  `json:"event"`
	User      ECSUser        `json:"user,omitempty"`
	Source    ECSSource      `json:"source,omitempty"`
	Message   string         `json:"message,omitempty"`
	Labels    map[string]any `json:"labels,omitempty"`
}

// ECSEventField holds ECS event categorization.
type ECSEventField struct {
	Category string `json:"category,omitempty"`
	Type     string `json:"type,omitempty"`
	Outcome  string `json:"outcome,omitempty"`
	Severity int    `json:"severity,omitempty"`
}

// ECSUser holds ECS user fields.
type ECSUser struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// ECSSource holds ECS source fields.
type ECSSource struct {
	IP string `json:"ip,omitempty"`
}

// AuditRow represents a row from np_audit_log.
type AuditRow struct {
	ID        string
	TenantID  *string
	Action    string
	TableName string
	RecordID  string
	UserID    string
	UserName  string
	IPAddress string
	OldData   map[string]any
	NewData   map[string]any
	CreatedAt time.Time
}

// Schema constants.
const (
	SchemaOCSF = "ocsf"
	SchemaECS  = "ecs"
	SchemaRaw  = "raw"
)

// OCSFClassAuth is the OCSF class_uid for Authentication Activity.
const OCSFClassAuth = 3002

// OCSFCategoryIdentity is the OCSF category_uid for Identity & Access Management.
const OCSFCategoryIdentity = 3

// NormalizeSeverity maps nSelf action verbs to OCSF severity_id.
// 1=Info, 2=Low, 3=Medium, 4=High, 5=Critical, 6=Fatal, 0=Unknown.
func NormalizeSeverity(action string) int {
	switch action {
	case "auth_failure", "mfa_failure", "token_revoke":
		return 3 // Medium
	case "plugin_remove", "config_change", "admin_login":
		return 2 // Low
	case "permission_escalation", "suspicious_query":
		return 4 // High
	default:
		return 1 // Info
	}
}

// ToOCSF converts an AuditRow to an OCSFEvent envelope.
func ToOCSF(row AuditRow, productVersion string) OCSFEvent {
	return OCSFEvent{
		ClassUID:    OCSFClassAuth,
		CategoryUID: OCSFCategoryIdentity,
		Time:        row.CreatedAt.UnixMilli(),
		SeverityID:  NormalizeSeverity(row.Action),
		TypeUID:     OCSFClassAuth*100 + 1,
		Actor: OCSFActor{
			User: OCSFUser{UID: row.UserID, Name: row.UserName},
		},
		DstEndpoint: OCSFEndpoint{IP: row.IPAddress},
		Metadata: OCSFMetadata{
			Product: OCSFProduct{Name: "nSelf", Version: productVersion},
			Version: "1.0.0",
		},
		RawData: fmt.Sprintf("action=%s table=%s record=%s", row.Action, row.TableName, row.RecordID),
	}
}

// ToECS converts an AuditRow to an ECSEvent envelope.
func ToECS(row AuditRow) ECSEvent {
	outcome := "unknown"
	if row.Action == "auth_failure" || row.Action == "mfa_failure" {
		outcome = "failure"
	} else {
		outcome = "success"
	}
	return ECSEvent{
		Timestamp: row.CreatedAt.UTC().Format(time.RFC3339Nano),
		Event: ECSEventField{
			Category: "authentication",
			Type:     row.Action,
			Outcome:  outcome,
			Severity: NormalizeSeverity(row.Action),
		},
		User:    ECSUser{ID: row.UserID, Name: row.UserName},
		Source:  ECSSource{IP: row.IPAddress},
		Message: fmt.Sprintf("%s on %s (%s)", row.Action, row.TableName, row.RecordID),
	}
}

// ToRaw converts an AuditRow to a raw JSON byte slice.
func ToRaw(row AuditRow) ([]byte, error) {
	return json.Marshal(row)
}

// NormalizeEvent converts an AuditRow to the requested schema bytes.
func NormalizeEvent(row AuditRow, schema string, productVersion string) ([]byte, error) {
	switch schema {
	case SchemaECS:
		return json.Marshal(ToECS(row))
	case SchemaRaw:
		return ToRaw(row)
	default: // ocsf
		return json.Marshal(ToOCSF(row, productVersion))
	}
}
