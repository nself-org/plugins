//go:build darwin && macosleg

package darwin

import (
	"github.com/nself-org/cli/sdk/go/v2/remote"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Use the same SSH and sudo ancestry as bootstrap so macOS privacy restrictions match.
func TestHostedOwnershipInventory(t *testing.T) {
	config := os.Getenv("NSELF_MACOS_LEG_SSH_CONFIG")
	if config == "" {
		t.Skip("hosted macOS leg only")
	}
	script := strings.ReplaceAll(heldIDsScan(IDLow, IDHigh), "2>/dev/null", "")
	out, err := exec.Command("ssh", "-F", config, "127.0.0.1", "sudo", "-n", "sh", "-c", remote.ShellQuote(script)).CombinedOutput()
	if err != nil || strings.Contains(string(out), "scan-incomplete") {
		t.Fatalf("ownership inventory: %v\n%s", err, out)
	}
}
