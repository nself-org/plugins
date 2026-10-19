package main

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/transport/ssh"
)

// D-1101: the driver sends secret values only under a PERSONAL coordinator, and only through a source.
func TestConfigureSSHSecretsFollowCoordinatorMode(t *testing.T) {
	for mode, want := range map[string]bool{"": true, "personal": true, "team": false} {
		t.Setenv("NSELF_CI_COORDINATOR_MODE", mode)
		d := &ssh.Driver{}
		configureSSH(d)
		if d.Personal != want {
			t.Errorf("mode %q: Personal = %v, want %v", mode, d.Personal, want)
		}
		if d.Secrets == nil {
			t.Errorf("mode %q: no per-lease secret source is wired", mode)
		}
	}
}
