package main

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func TestTrustReasonsTableCoversEveryReason(t *testing.T) {
	got := trustReasonsTable()
	if !strings.HasPrefix(got, "| Reason | Meaning |\n|---|---|\n") {
		t.Fatalf("header missing: %q", got)
	}
	for r, m := range trust.ReasonMeaning {
		if !strings.Contains(got, "| `"+string(r)+"` | "+m+" |") {
			t.Errorf("row for %s missing", r)
		}
	}
	if strings.Count(got, "\n") != len(trust.ReasonMeaning)+2 {
		t.Errorf("unexpected row count")
	}
}
