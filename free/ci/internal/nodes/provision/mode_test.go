package provision

// Purpose: pin the compat mode of this package's tests. The ported P7-CANON-10 tests assert v1.4 output
// byte for byte, so they run in v1.4 mode whatever NSELF_V15 the caller exported; every V15 test sets
// NSELF_V15=1 itself (v15_test.go), and both modes are covered that way.
// Inputs:  the process environment. Constraints: affects this package's test binary only.

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	_ = os.Unsetenv("NSELF_V15")
	os.Exit(m.Run())
}
