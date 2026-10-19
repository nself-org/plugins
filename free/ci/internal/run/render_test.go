package run

// Purpose: the human renderer's failure text (D-1017).
// Constraints: a code-carrying failure prints its code once.

import "testing"

func TestFailTextPrintsCodeOnce(t *testing.T) {
	cases := []struct {
		name string
		o    JobOutcome
		want string
	}{
		{"detail repeats code", JobOutcome{Code: "E712", Detail: "E712 missing: out/a.txt"}, "E712: missing: out/a.txt"},
		{"detail repeats code with colon", JobOutcome{Code: "E712", Detail: "E712: missing: out/a.txt"}, "E712: missing: out/a.txt"},
		{"detail without code", JobOutcome{Code: "E712", Detail: "missing: out/a.txt"}, "E712: missing: out/a.txt"},
		{"other code in detail kept", JobOutcome{Code: "E712", Detail: "E711 refused"}, "E712: E711 refused"},
		{"code only", JobOutcome{Code: "E712", Detail: "E712"}, "E712"},
		{"no code", JobOutcome{ExitCode: 3}, "exit 3"},
	}
	for _, c := range cases {
		if got := failText(c.o); got != c.want {
			t.Errorf("%s: failText = %q, want %q", c.name, got, c.want)
		}
	}
}
