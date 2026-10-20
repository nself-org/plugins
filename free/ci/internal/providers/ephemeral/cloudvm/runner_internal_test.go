//go:build unix

package cloudvm

// Purpose: TestTokenEnvFunnel*: the funnel runner gives the child exactly the environment it was handed
// (plus its own attempt marker) and masks the token even if the child echoes it.
// Inputs:  /bin/sh as the child. Outputs: pass/fail. Constraints: synthetic token only.

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTokenEnvFunnelMasksAnEchoedToken(t *testing.T) {
	r := funnelRunner{home: t.TempDir(), coordinator: "masktest"}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, code, err := r.Run(ctx, []string{"/bin/sh", "-c", `echo "token=$NSELF_CI_BURST_TOKEN"; echo "home=${HOME:-unset}"; echo "path=${PATH:-unset}"; echo "other=${SOME_CLOUD_TOKEN:-unset}"`},
		[]string{"NSELF_CI_BURST_TOKEN=" + syntheticSecret}, []string{syntheticSecret})
	if err != nil || code != 0 {
		t.Fatalf("run: %d %v", code, err)
	}
	text := string(out)
	if strings.Contains(text, syntheticSecret) {
		t.Fatalf("the funnel let the token through:\n%s", text)
	}
	if !strings.Contains(text, "home=unset") || !strings.Contains(text, "other=unset") {
		t.Fatalf("the child inherited something:\n%s", text)
	}
}

func TestTokenEnvFunnelEndsAtItsDeadline(t *testing.T) {
	r := funnelRunner{home: t.TempDir(), coordinator: "deadline"}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, _ = r.Run(ctx, []string{"/bin/sh", "-c", "exec sleep 60"}, nil, nil)
	if time.Since(start) > 20*time.Second {
		t.Fatalf("a hung child outlived its deadline by %v", time.Since(start))
	}
}
