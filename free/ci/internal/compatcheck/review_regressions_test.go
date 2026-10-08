package compatcheck

import "testing"

func TestPrereleaseBelowStableMinimum(t *testing.T) {
	if err := Check(func(string) string { return "1.4.12-rc.1" }, RequiresNself); err == nil {
		t.Fatal("prerelease accepted")
	}
}

func TestIncompleteVersionRejected(t *testing.T) {
	if err := Check(func(string) string { return "1.5" }, RequiresNself); err == nil {
		t.Fatal("incomplete version accepted")
	}
}
