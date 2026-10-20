package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/signing"
)

func TestSignerGatedCLIChild(t *testing.T) {
	if os.Getenv("CI_TEST_GATED_SIGNER") != "1" {
		return
	}
	os.Exit(artifactsSigner([]string{"--public"}))
}

func TestSignerResolvesWithStrippedEnvironment(t *testing.T) {
	for _, inherited := range []string{"", "0", "1"} {
		t.Run("inherited_v15_"+inherited, func(t *testing.T) {
			t.Setenv("NSELF_V15", inherited)
			t.Setenv("SIGNER_OPERATOR_SECRET", "must-stay-out")
			root, home := t.TempDir(), t.TempDir()
			secretsDir := fakeNself(t)
			bin := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0]
			if err := os.Rename(filepath.Join(bin, "nself"), filepath.Join(bin, "nself-ungated")); err != nil {
				t.Fatal(err)
			}
			gate := `#!/bin/sh
[ "$NSELF_V15" = 1 ] || exit 64
[ -z "$SIGNER_OPERATOR_SECRET" ] || exit 65
exec "$(dirname "$0")/nself-ungated" "$@"
`
			if err := os.WriteFile(filepath.Join(bin, "nself"), []byte(gate), 0o755); err != nil {
				t.Fatal(err)
			}
			key, value, err := signing.Generate()
			if err != nil {
				t.Fatal(err)
			}
			defer key.Zero()
			defer clear(value)
			if err := os.WriteFile(filepath.Join(secretsDir, signing.KeyName), value, 0o600); err != nil {
				t.Fatal(err)
			}
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			out, code, err := (&signing.FunnelRunner{Home: home}).RunStdio(context.Background(), signing.StdioCall{
				Argv: []string{exe, "-test.run=^TestSignerGatedCLIChild$"},
				Env:  []string{"CI_TEST_GATED_SIGNER=1", "NSELF_V15=1", "NSELF_CI_REPO=" + root, "NSELF_CI_HOME=" + home}, Timeout: 30 * time.Second,
			})
			var info signing.KeyInfo
			if err != nil || code != 0 || json.Unmarshal(out, &info) != nil || info.KeyID != key.ID || info.Public != key.PublicB64() {
				t.Fatalf("signer public key failed: exit=%d err=%v", code, err)
			}
		})
	}
}
