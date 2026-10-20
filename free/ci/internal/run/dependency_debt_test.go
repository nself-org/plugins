package run

import "testing"

func TestDependencyFailureBlocksEvenWhenAllowed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		outcome JobOutcome
		want    bool
	}{
		{"passed", JobOutcome{State: StatePassed}, true},
		{"failed", JobOutcome{State: StateFailed}, false},
		{"allowed failure", JobOutcome{State: StateFailed, AllowFailure: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := depOK(tc.outcome); got != tc.want {
				t.Fatalf("dependency eligible = %v, want %v", got, tc.want)
			}
		})
	}
}
