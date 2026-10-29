//go:build !linux

package run

import "testing"

// privateSlots is a no-op off Linux: slots there are locks under the test's own home.
func privateSlots(t *testing.T) { t.Helper() }
