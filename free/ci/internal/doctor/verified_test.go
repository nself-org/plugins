package doctor

import (
	"encoding/json"
	"strings"
	"testing"
)

// A check that reports a last-verified date carries "verified_at" (a time or null); every other check is unchanged.
func TestCheckVerifiedAtJSON(t *testing.T) {
	at := "2026-10-03T03:00:00Z"
	for name, tc := range map[string]struct {
		check Check
		want  string // substring that must appear
		not   string // substring that must not
	}{
		"dated":        {Check{ID: "a", Status: Pass, HasVerifiedAt: true, VerifiedAt: &at}, `"verified_at":"2026-10-03T03:00:00Z"`, ""},
		"null":         {Check{ID: "a", Status: Pass, HasVerifiedAt: true}, `"verified_at":null`, ""},
		"not reported": {Check{ID: "a", Status: Pass, VerifiedAt: &at}, `"id":"a"`, "verified_at"},
	} {
		raw, err := json.Marshal(tc.check)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), tc.want) || (tc.not != "" && strings.Contains(string(raw), tc.not)) {
			t.Errorf("%s: %s", name, raw)
		}
	}
	raw, _ := json.Marshal([]Check{{ID: "a", Name: "n", Status: Pass, Message: "m", Subject: "s", HasVerifiedAt: true}})
	var back []map[string]any
	if err := json.Unmarshal(raw, &back); err != nil || back[0]["subject"] != "s" || back[0]["message"] != "m" {
		t.Errorf("the ordinary fields survive: %s %v", raw, err)
	}
}
