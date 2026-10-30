package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestMachineDefaults covers CI D29: a machine-invoked run posts no status unless --post-status is given.
func TestMachineDefaults(t *testing.T) {
	cases := []struct {
		name     string
		by       string
		args     []string
		noStatus bool
		notice   bool
	}{
		{"unset env keeps the status", "", nil, false, false},
		{"machine caller suppresses it", "mcp", nil, true, true},
		{"post-status restores it", "mcp", []string{"--post-status"}, false, false},
		{"explicit no-status stays quiet", "http", []string{"--no-status"}, true, false},
		{"post-status without a machine caller", "", []string{"--post-status"}, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("NSELF_INVOKED_BY", c.by)
			t.Setenv("NSELF_CI_REPO", t.TempDir())
			var buf bytes.Buffer
			old := machineNotice
			machineNotice = &buf
			t.Cleanup(func() { machineNotice = old })
			o, err := parseBare(c.args)
			if err != nil {
				t.Fatal(err)
			}
			if o.NoStatus != c.noStatus {
				t.Errorf("NoStatus = %v, want %v", o.NoStatus, c.noStatus)
			}
			if got := strings.Count(buf.String(), "\n"); (got == 1) != c.notice || got > 1 {
				t.Errorf("notice lines = %d, want notice=%v: %q", got, c.notice, buf.String())
			}
			if c.notice && !strings.Contains(buf.String(), "--post-status") {
				t.Errorf("notice does not name --post-status: %q", buf.String())
			}
		})
	}
}
