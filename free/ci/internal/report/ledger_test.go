package report

import "testing"

func TestClaimStatusShape(t *testing.T) {
	s := ClaimStatus("o", "r", sha40, "mm6", "i1")
	if err := s.Validate(); err != nil || s.State != Pending || s.ContextName() != ClaimContext {
		t.Fatalf("%v %+v", err, s)
	}
	if len(s.Description) > MaxDescription {
		t.Fatal("too long")
	}
}
