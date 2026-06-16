package plugin

import (
	"context"
	"testing"
)

// Tests for Ready and Shutdown (default no-op implementations).

func TestBase_Ready_DefaultNil(t *testing.T) {
	b := Base{
		PluginInfo: Info{
			Name:    "test-plugin",
			Version: "1.0.0",
			MinCLI:  "1.0.0",
		},
	}
	if err := b.Ready(context.Background()); err != nil {
		t.Errorf("Base.Ready() returned error %v, want nil", err)
	}
}

func TestBase_Shutdown_DefaultNil(t *testing.T) {
	b := Base{
		PluginInfo: Info{
			Name:    "test-plugin",
			Version: "1.0.0",
			MinCLI:  "1.0.0",
		},
	}
	if err := b.Shutdown(context.Background()); err != nil {
		t.Errorf("Base.Shutdown() returned error %v, want nil", err)
	}
}
