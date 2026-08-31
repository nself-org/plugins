package internal

import (
	"encoding/json"
	"testing"
	"time"
)

// Purpose: unit tests for domain model JSON marshalling.
// Inputs: model structs with known field values.
// Outputs: valid JSON with correct field names and types.
// Constraints: no DB, no network.

func TestEvidenceJSONRoundtrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	e := Evidence{
		ID:              "ev-123",
		SourceAccountID: "primary",
		Type:            "spam",
		Content:         map[string]any{"text": "test content"},
		Reason:          "automated detection",
		Source:          "nchat",
		WorkspaceID:     "ws-456",
		Priority:        "high",
		IsEncrypted:     false,
		Status:          "pending",
		Metadata:        map[string]any{"score": 0.95},
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal Evidence: %v", err)
	}

	var out Evidence
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal Evidence: %v", err)
	}

	if out.ID != e.ID {
		t.Errorf("ID: got %q, want %q", out.ID, e.ID)
	}
	if out.SourceAccountID != e.SourceAccountID {
		t.Errorf("SourceAccountID: got %q, want %q", out.SourceAccountID, e.SourceAccountID)
	}
	if out.Type != e.Type {
		t.Errorf("Type: got %q, want %q", out.Type, e.Type)
	}
	if out.Priority != e.Priority {
		t.Errorf("Priority: got %q, want %q", out.Priority, e.Priority)
	}
	if out.Status != e.Status {
		t.Errorf("Status: got %q, want %q", out.Status, e.Status)
	}
}

func TestLegalHoldJSONFields(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	lh := LegalHold{
		ID:              "lh-789",
		SourceAccountID: "primary",
		Name:            "Compliance Hold Q3",
		Scope:           map[string]any{"channels": []string{"general"}},
		Criteria:        map[string]any{"start": "2026-01-01"},
		Status:          "active",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	b, err := json.Marshal(lh)
	if err != nil {
		t.Fatalf("marshal LegalHold: %v", err)
	}

	m := make(map[string]any)
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	for _, field := range []string{"id", "source_account_id", "name", "scope", "criteria", "status"} {
		if _, ok := m[field]; !ok {
			t.Errorf("missing JSON field %q", field)
		}
	}
}

func TestSpamRuleDefaults(t *testing.T) {
	sr := SpamRule{
		ID:       "sr-001",
		Name:     "block-links",
		RuleType: "pattern",
		Action:   "block",
		Priority: 1,
	}

	if sr.IsEnabled {
		t.Error("new SpamRule should default IsEnabled=false")
	}
	if sr.Priority != 1 {
		t.Errorf("Priority: got %d, want 1", sr.Priority)
	}
}

func TestAbuseScoreFields(t *testing.T) {
	as := AbuseScore{
		UserID:          "user-abc",
		SourceAccountID: "primary",
		TrustScore:      0.75,
		RiskLevel:       "low",
		TotalEvents:     10,
		PositiveEvents:  8,
		NegativeEvents:  2,
	}

	if as.TrustScore < 0 || as.TrustScore > 1 {
		t.Errorf("TrustScore out of range: %f", as.TrustScore)
	}
	if as.TotalEvents != as.PositiveEvents+as.NegativeEvents {
		t.Errorf("TotalEvents mismatch: %d != %d + %d",
			as.TotalEvents, as.PositiveEvents, as.NegativeEvents)
	}
}
