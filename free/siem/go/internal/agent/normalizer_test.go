package agent

import (
	"encoding/json"
	"testing"
	"time"
)

func TestToOCSF_AuthFailure(t *testing.T) {
	row := AuditRow{
		ID:        "abc123",
		Action:    "auth_failure",
		TableName: "np_users",
		RecordID:  "user-1",
		UserID:    "user-1",
		UserName:  "alice",
		IPAddress: "10.0.0.1",
		CreatedAt: time.Unix(1700000000, 0),
	}

	ev := ToOCSF(row, "1.0.9")

	if ev.ClassUID != OCSFClassAuth {
		t.Errorf("class_uid: got %d, want %d", ev.ClassUID, OCSFClassAuth)
	}
	if ev.CategoryUID != OCSFCategoryIdentity {
		t.Errorf("category_uid: got %d, want %d", ev.CategoryUID, OCSFCategoryIdentity)
	}
	if ev.SeverityID != 3 {
		t.Errorf("severity_id for auth_failure: got %d, want 3 (Medium)", ev.SeverityID)
	}
	if ev.Actor.User.UID != "user-1" {
		t.Errorf("actor.user.uid: got %q, want %q", ev.Actor.User.UID, "user-1")
	}
	if ev.Time != row.CreatedAt.UnixMilli() {
		t.Errorf("time: got %d, want %d", ev.Time, row.CreatedAt.UnixMilli())
	}
	if ev.Metadata.Product.Name != "nSelf" {
		t.Errorf("metadata.product.name: got %q", ev.Metadata.Product.Name)
	}
}

func TestToOCSF_ConfigChange(t *testing.T) {
	row := AuditRow{
		Action:    "config_change",
		CreatedAt: time.Now(),
	}
	ev := ToOCSF(row, "1.0.9")
	if ev.SeverityID != 2 {
		t.Errorf("severity for config_change: got %d, want 2 (Low)", ev.SeverityID)
	}
}

func TestToOCSF_PluginInstall(t *testing.T) {
	row := AuditRow{
		Action:    "plugin_install",
		CreatedAt: time.Now(),
	}
	ev := ToOCSF(row, "1.0.9")
	if ev.SeverityID != 1 {
		t.Errorf("severity for plugin_install: got %d, want 1 (Info)", ev.SeverityID)
	}
}

func TestNormalizeEvent_OCSF(t *testing.T) {
	row := AuditRow{
		ID:        "ev1",
		Action:    "auth_failure",
		TableName: "np_users",
		CreatedAt: time.Now(),
	}
	data, err := NormalizeEvent(row, SchemaOCSF, "1.0.9")
	if err != nil {
		t.Fatalf("NormalizeEvent OCSF: %v", err)
	}

	var ev OCSFEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		t.Fatalf("unmarshal OCSF: %v", err)
	}
	if ev.ClassUID != OCSFClassAuth {
		t.Errorf("class_uid: got %d, want %d", ev.ClassUID, OCSFClassAuth)
	}
}

func TestNormalizeEvent_ECS(t *testing.T) {
	row := AuditRow{
		ID:        "ev2",
		Action:    "auth_failure",
		TableName: "np_users",
		UserID:    "u1",
		CreatedAt: time.Now(),
	}
	data, err := NormalizeEvent(row, SchemaECS, "1.0.9")
	if err != nil {
		t.Fatalf("NormalizeEvent ECS: %v", err)
	}

	var ev ECSEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		t.Fatalf("unmarshal ECS: %v", err)
	}
	if ev.Event.Outcome != "failure" {
		t.Errorf("ECS outcome for auth_failure: got %q, want failure", ev.Event.Outcome)
	}
}

func TestNormalizeEvent_Raw(t *testing.T) {
	row := AuditRow{
		ID:        "ev3",
		Action:    "plugin_remove",
		CreatedAt: time.Now(),
	}
	data, err := NormalizeEvent(row, SchemaRaw, "1.0.9")
	if err != nil {
		t.Fatalf("NormalizeEvent Raw: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty raw JSON")
	}
}
