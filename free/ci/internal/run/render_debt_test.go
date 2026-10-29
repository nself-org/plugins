package run

import "testing"

func TestFailTextSingleCode(t *testing.T) {
	for _, tc := range []struct{ detail, want string }{
		{"E712 missing: tool", "E712: missing: tool"},
		{"E712: missing: tool", "E712: missing: tool"},
		{"missing: tool", "E712: missing: tool"},
		{"E7120: other code", "E712: E7120: other code"},
		{"wrapped E712: missing", "E712: wrapped E712: missing"},
	} {
		if got := failText(JobOutcome{Code: "E712", Detail: tc.detail}); got != tc.want {
			t.Errorf("%q: got %q want %q", tc.detail, got, tc.want)
		}
	}
}
