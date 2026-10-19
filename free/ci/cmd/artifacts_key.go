package main

// Purpose: `nself ci artifacts key [--json]`, `key init [--json]` and
// `key rotate [--yes] [--json]` (P7-CACHE-07, Epic CACHE D11): print the
// trust line of the release key (`<key-id> <base64 public key>`, the line
// `.nself/trust/ci-release.keys` takes), create the key, or replace it.
// Inputs:  argv after `key`; env NSELF_CI_HOME, NSELF_CI_REPO.
// Outputs: the trust line or one v1 JSON envelope; exit 0, 1 (usage, no
// confirmation) or 2 (E717).
// Constraints: every key operation runs in the signer process (the key is
// never in this one); init refuses when the secret exists (secret_exists);
// rotate asks for confirmation unless --yes and prints the new trust line and
// the old key id for `.revoked`; the printed values are public.

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/signing"
)

// keyClient builds the signer client; tests replace it with an in-process runner.
var keyClient = func() (*signing.Client, error) {
	root, err := evidenceRoot("")
	if err != nil {
		return nil, err
	}
	env := []string{"NSELF_CI_REPO=" + root}
	if h := os.Getenv("NSELF_CI_HOME"); h != "" {
		env = append(env, "NSELF_CI_HOME="+h)
	}
	return &signing.Client{Runner: &signing.FunnelRunner{Home: os.Getenv("NSELF_CI_HOME")}, Env: env, Team: teamCoordinator()}, nil
}

// keyConfirmIn is where rotate reads the confirmation from.
var keyConfirmIn = os.Stdin

func init() {
	registerArtifactsSub("key", artifactsKey)
}

const keyUsage = "usage: nself ci artifacts key [init | rotate [--yes]] [--json]"

func artifactsKey(args []string) int {
	var yes *bool
	fs, pos, err := parseArtifacts("key", args, func(fs *flag.FlagSet) { yes = fs.Bool("yes", false, "rotate without asking") })
	if err != nil || len(pos) > 1 || (len(pos) == 1 && pos[0] != "init" && pos[0] != "rotate") || (*yes && (len(pos) == 0 || pos[0] != "rotate")) {
		fmt.Fprintln(os.Stderr, keyUsage)
		return 1
	}
	asJSON := fs.Lookup("json").Value.String() == "true"
	sub := "key"
	if len(pos) == 1 {
		sub = "key " + pos[0]
	}
	command := "ci artifacts " + sub
	if sub == "key rotate" && !*yes && !confirmRotate() {
		fmt.Fprintln(os.Stderr, "nself ci artifacts key rotate: not confirmed (answer y, or pass --yes)")
		return 1
	}
	c, err := keyClient()
	if err != nil {
		return artifactsExit(signing.Failf(signing.ReasonSignerFailed, "no project root"), command, asJSON)
	}
	ctx := context.Background()
	var info signing.KeyInfo
	switch sub {
	case "key init":
		info, err = c.Init(ctx)
	case "key rotate":
		info, err = c.Rotate(ctx)
	default:
		info, err = c.PublicKey(ctx)
	}
	if err != nil {
		return artifactsExit(err, command, asJSON)
	}
	return printKey(command, sub, info, asJSON)
}

func confirmRotate() bool {
	fmt.Fprint(os.Stderr, "Replace the release key? Releases signed with the old key fail once its id is in .revoked. [y/N] ")
	line, _ := bufio.NewReader(keyConfirmIn).ReadString('\n')
	return strings.EqualFold(strings.TrimSpace(line), "y") || strings.EqualFold(strings.TrimSpace(line), "yes")
}

func printKey(command, sub string, info signing.KeyInfo, asJSON bool) int {
	if asJSON {
		return writeArtifactsJSON(command, map[string]any{"key_id": info.KeyID, "public_key": info.Public, "old_key_id": nilIfEmpty(info.OldKeyID), "trust_line": info.TrustLine()})
	}
	fmt.Println(info.TrustLine())
	switch sub {
	case "key init":
		fmt.Fprintln(os.Stderr, "Release key created. Commit the line above to .nself/trust/ci-release.keys on the protected default branch.")
	case "key rotate":
		fmt.Fprintf(os.Stderr, "Release key replaced. Add the line above to .nself/trust/ci-release.keys and the old id to .nself/trust/ci-release.revoked:\n%s\n", info.OldKeyID)
	}
	return 0
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
