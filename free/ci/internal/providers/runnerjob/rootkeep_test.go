//go:build integration

package runnerjob_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers/github/demand"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// wellFormedJITConfig is the shape the runner decodes: three base-64 documents (.runner, .credentials,
// .credentials_rsaparams) that it writes into its root. The RSA key is a real (throw-away) key so the
// runner gets as far as a session attempt; the markers are the OAuth client id and the key's private exponent.
func wellFormedJITConfig(t *testing.T) (cfg string, markers []string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	key.Precompute()
	n := func(v *big.Int) string { return base64.StdEncoding.EncodeToString(v.Bytes()) }
	d := n(key.D)
	params := `{"d":"` + d + `","dp":"` + n(key.Precomputed.Dp) + `","dq":"` + n(key.Precomputed.Dq) + `","exponent":"` + n(big.NewInt(int64(key.E))) +
		`","inverseQ":"` + n(key.Precomputed.Qinv) + `","modulus":"` + n(key.N) + `","p":"` + n(key.Primes[0]) + `","q":"` + n(key.Primes[1]) + `"}`
	runner := b64(`{"agentId":"7","agentName":"nself-x","poolId":"1","serverUrl":"https://pipelines.actions.githubusercontent.com/x","gitHubUrl":"https://github.com/o/r","workFolder":"_work","ephemeral":true}`)
	creds := b64(`{"scheme":"OAuth","data":{"clientId":"MARKCLIENTID7","authorizationUrl":"https://vstoken.actions.githubusercontent.com/x","requireFipsCryptography":"False"}}`)
	return b64(`{".runner":"` + runner + `",".credentials":"` + creds + `",".credentials_rsaparams":"` + b64(params) + `"}`), []string{"MARKCLIENTID7", d}
}

// TestRunnerRootKeepsTheDecodedCredentialOffTheNodesDisk proves the Opus M1 fix. The real runner decodes the
// JIT config and writes .runner, .credentials and .credentials_rsaparams (the runner's private key) into its
// root. With the root on the workspace those files sit in the agent's CI home for the life of the job; with
// the runner tmpfs they exist only in the container's memory. The first leg is the positive control: the
// old layout (root in /repo) must be caught by the scan, so a clean second leg means something.
func TestRunnerRootKeepsTheDecodedCredentialOffTheNodesDisk(t *testing.T) {
	needDocker(t, realImage)
	cfg, markers := wellFormedJITConfig(t)
	oldLayout := []string{"sh", "-c", `set -e; r=/repo/runner; mkdir -p "$r/_work"; cp -a /home/runner/bin "$r/bin"; ln -s /home/runner/externals "$r/externals"; timeout -s KILL 15 "$r/bin/Runner.Listener" run || true; sleep 5`}
	run, ran := runLegWith(t, "host19-rootold", realImage, demand.CredentialKind, cfg, oldLayout, false, 90*time.Second, false, markers)
	if !ran {
		t.Fatal("the container leg is required")
	}
	if len(run.NeedleHits) == 0 {
		t.Fatalf("positive control failed: the old layout should leave the decoded credential in the state dir, the scan saw nothing (result %+v, home entries %d)\n%s", run.Res, run.HomeRead, run.Out)
	}
	t.Logf("positive control: the old layout left %d file(s) with the decoded credential on the node", len(run.NeedleHits))

	for _, sandboxed := range []bool{false, true} {
		id := map[bool]string{false: "host19-rootnew-c", true: "host19-rootnew-g"}[sandboxed]
		script := []string{"sh", "-c", strings.Replace(demand.RunnerCommand[2], `exec "$r/bin/Runner.Listener" run`, `timeout -s KILL 15 "$r/bin/Runner.Listener" run || true; ls -A "$r"; sleep 5`, 1)}
		run, ran := runLegWith(t, id, realImage, demand.CredentialKind, cfg, script, sandboxed, 90*time.Second, true, markers)
		if !ran {
			continue
		}
		t.Logf("level %s: %q", run.Level, strings.TrimSpace(run.Out))
		if len(run.NeedleHits) != 0 {
			t.Errorf("level %s: the decoded credential reached the node's disk: %v", run.Level, run.NeedleHits)
		}
		if run.HomeRead == 0 {
			t.Errorf("level %s: the scan read nothing", run.Level)
		}
		for _, m := range append(markers, cfg) {
			for name, text := range map[string]string{"output": run.Out, "ps list": run.Ps, "container config": run.Inspect} {
				if strings.Contains(text, m) {
					t.Errorf("level %s: %.20q is in the %s", run.Level, m, name)
				}
			}
		}
		if !strings.Contains(run.Out, ".credentials_rsaparams") {
			t.Errorf("level %s: the runner did not write its credential files into /runner (so there is nothing to keep off the disk): %+v\n%s", run.Level, run.Res, run.Out)
		}
		if strings.Contains(run.Out, "Base-64") || strings.Contains(run.Out, "decode error") {
			t.Errorf("level %s: the runner did not accept the well-formed config, so this leg proves nothing:\n%s", run.Level, run.Out)
		}
	}
}
