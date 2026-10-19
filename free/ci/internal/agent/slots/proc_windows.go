//go:build windows

package slots

// alive is conservative on windows: the stub lock never lets a table form.
func alive(int) bool { return true }
