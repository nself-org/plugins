package nginx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReloadWatchModeNeedsNoBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no nginx binary anywhere
	c := NewClientMode(filepath.Join(t.TempDir(), "p.conf"), "watch")
	if err := c.Reload(context.Background()); err != nil {
		t.Fatalf("watch mode hands the reload to the conf.d watcher: %v", err)
	}
}

func TestReloadExecModeFailsWithoutBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	c := NewClientMode(filepath.Join(t.TempDir(), "p.conf"), "exec")
	if err := c.Reload(context.Background()); err == nil {
		t.Fatal("exec mode must report a missing nginx binary")
	}
}

func TestAddVhostRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "p.conf")
	c := NewClientMode(p, "watch")
	if err := c.AddVhost(context.Background(), VhostInput{Slug: "a", Domain: "a.example.test", HasuraPort: 8081}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "server_name a.example.test;") {
		t.Fatalf("vhost not written:\n%s", b)
	}
}
