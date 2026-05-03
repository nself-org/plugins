package internal

import (
	"testing"
	"time"
)

// TestEvaluateAttributeOp covers all supported operators.
func TestEvaluateAttributeOp(t *testing.T) {
	ctx := map[string]interface{}{
		"role":    "admin",
		"age":     float64(30),
		"country": "US",
		"email":   "user@example.com",
	}

	cases := []struct {
		attr   string
		op     string
		target interface{}
		want   bool
		desc   string
	}{
		// eq
		{"role", "eq", "admin", true, "eq match"},
		{"role", "eq", "user", false, "eq no match"},
		// neq
		{"role", "neq", "user", true, "neq match"},
		{"role", "neq", "admin", false, "neq no match"},
		// numeric comparisons
		{"age", "gt", float64(25), true, "gt match"},
		{"age", "gt", float64(35), false, "gt no match"},
		{"age", "lt", float64(35), true, "lt match"},
		{"age", "lt", float64(25), false, "lt no match"},
		{"age", "gte", float64(30), true, "gte equal"},
		{"age", "gte", float64(31), false, "gte no match"},
		{"age", "lte", float64(30), true, "lte equal"},
		{"age", "lte", float64(29), false, "lte no match"},
		// contains
		{"email", "contains", "@example", true, "contains match"},
		{"email", "contains", "@other", false, "contains no match"},
		// regex
		{"email", "regex", `^user@.*\.com$`, true, "regex match"},
		{"email", "regex", `^admin@`, false, "regex no match"},
		// missing attribute
		{"missing_key", "eq", "x", false, "missing attribute"},
		// unknown operator
		{"role", "unknown_op", "admin", false, "unknown operator"},
	}

	for _, tc := range cases {
		got := evaluateAttributeOp(tc.attr, tc.op, tc.target, ctx)
		if got != tc.want {
			t.Errorf("[%s] evaluateAttributeOp(%q, %q, %v) = %v, want %v",
				tc.desc, tc.attr, tc.op, tc.target, got, tc.want)
		}
	}
}

// TestEvaluateAttributeOp_InvalidRegex verifies that an invalid regex pattern
// returns false (not a panic).
func TestEvaluateAttributeOp_InvalidRegex(t *testing.T) {
	ctx := map[string]interface{}{"email": "user@example.com"}
	// "[invalid" is not a valid regex.
	got := evaluateAttributeOp("email", "regex", "[invalid", ctx)
	if got {
		t.Error("expected false for invalid regex, got true")
	}
}

// TestHashPercentage verifies that the function is deterministic and returns
// values in [0, 99].
func TestHashPercentage(t *testing.T) {
	seen := make(map[int]bool)
	for i := 0; i < 100; i++ {
		h := hashPercentage("my-flag", "user-"+string(rune('A'+i%26)))
		if h < 0 || h > 99 {
			t.Errorf("hashPercentage out of range: %d", h)
		}
		seen[h] = true
	}

	// Same inputs must produce the same output.
	a := hashPercentage("flag-key", "user-123")
	b := hashPercentage("flag-key", "user-123")
	if a != b {
		t.Errorf("hashPercentage is not deterministic: %d != %d", a, b)
	}

	// Different flag keys should generally produce different hashes.
	c := hashPercentage("other-flag", "user-123")
	// Not strictly required to differ, but with different keys it almost always will.
	_ = c
}

// TestMatchSchedule_PastStart verifies that a schedule with a past start_at
// and future end_at returns true.
func TestMatchSchedule_PastStart(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	future := time.Now().Add(1 * time.Hour).Format(time.RFC3339)

	e := &Evaluator{}
	rule := Rule{
		Type:    "schedule",
		StartAt: &past,
		EndAt:   &future,
	}
	if !e.matchSchedule(rule) {
		t.Error("expected match for past start_at and future end_at")
	}
}

// TestMatchSchedule_FutureStart verifies that a schedule with a future start_at
// returns false.
func TestMatchSchedule_FutureStart(t *testing.T) {
	future := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	e := &Evaluator{}
	rule := Rule{
		Type:    "schedule",
		StartAt: &future,
	}
	if e.matchSchedule(rule) {
		t.Error("expected no match for future start_at")
	}
}

// TestMatchSchedule_PastEnd verifies that a schedule with a past end_at
// returns false.
func TestMatchSchedule_PastEnd(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	e := &Evaluator{}
	rule := Rule{
		Type:  "schedule",
		EndAt: &past,
	}
	if e.matchSchedule(rule) {
		t.Error("expected no match for past end_at")
	}
}

// TestMatchSchedule_NilBounds verifies that a schedule with no bounds returns false.
func TestMatchSchedule_NilBounds(t *testing.T) {
	e := &Evaluator{}
	rule := Rule{Type: "schedule"}
	if e.matchSchedule(rule) {
		t.Error("expected no match for schedule with no bounds")
	}
}

// TestMatchUserList verifies user list targeting.
func TestMatchUserList(t *testing.T) {
	e := &Evaluator{}
	rule := Rule{
		Type:  "user_list",
		Users: []string{"user-a", "user-b", "user-c"},
	}

	if !e.matchUserList(rule, "user-b") {
		t.Error("expected match for user-b")
	}
	if e.matchUserList(rule, "user-z") {
		t.Error("expected no match for user-z")
	}
	if e.matchUserList(rule, "") {
		t.Error("expected no match for empty user ID")
	}
}

// TestMatchUserList_Empty verifies that an empty user list never matches.
func TestMatchUserList_Empty(t *testing.T) {
	e := &Evaluator{}
	rule := Rule{Type: "user_list", Users: nil}
	if e.matchUserList(rule, "any-user") {
		t.Error("expected no match for empty user list")
	}
}

// TestParseJSONValue verifies JSON value decoding.
func TestParseJSONValue(t *testing.T) {
	cases := []struct {
		raw     string
		wantNil bool
	}{
		{`true`, false},
		{`false`, false},
		{`"hello"`, false},
		{`42`, false},
		{`null`, true}, // JSON null decodes to nil
		{``, false}, // empty → false default
	}
	for _, tc := range cases {
		import_json_raw := []byte(tc.raw)
		v := parseJSONValue(import_json_raw)
		if v == nil && !tc.wantNil {
			t.Errorf("parseJSONValue(%q) returned nil", tc.raw)
		}
	}
}

// TestToFloat covers all type cases of the toFloat helper.
func TestToFloat(t *testing.T) {
	cases := []struct {
		input interface{}
		want  float64
	}{
		{float64(3.14), 3.14},
		{float32(2.5), 2.5},
		{int(7), 7},
		{int64(100), 100},
	}
	for _, tc := range cases {
		got := toFloat(tc.input)
		if got != tc.want {
			t.Errorf("toFloat(%T %v) = %v, want %v", tc.input, tc.input, got, tc.want)
		}
	}
}

// TestContainsStr verifies substring search helper.
func TestContainsStr(t *testing.T) {
	cases := []struct {
		s, sub string
		want   bool
	}{
		{"hello world", "world", true},
		{"hello world", "xyz", false},
		{"hello", "hello", true},
		{"hi", "hello", false},
		{"", "a", false},
		{"a", "", true},
	}
	for _, tc := range cases {
		got := containsStr(tc.s, tc.sub)
		if got != tc.want {
			t.Errorf("containsStr(%q, %q) = %v, want %v", tc.s, tc.sub, got, tc.want)
		}
	}
}
