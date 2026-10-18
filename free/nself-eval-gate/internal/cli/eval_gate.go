package cli

// Purpose: `nself eval gate` — check the autonomy tier gate via nself-eval-gate
//   (GET /eval/gate/{tier}). Port of core `nself ci eval gate`
//   (cli internal/cmd/ci/eval_gate.go).
// Inputs:  --tier (required), --eval-url, --source-account.
// Outputs: gate status on stdout; hints and errors on stderr.
// Constraints: exit 0=cleared, 1=blocked/regression, 2=invalid tier or infra
//   error, 3=precondition not met (BGE-M3/gateway down, NOT a regression).
//   The supervised tier always exits 0 (enforced=false by design).
// SPORT: PLUGINS-NSELF-EVAL-GATE-CLI-001

import (
	"context"
	"errors"
	"io"
	"strings"
)

// validTiers lists the allowed autonomy tier names.
var validTiers = []string{"supervised", "semi-auto", "full-auto"}

var gateFlags = []flagDef{
	{"tier", kindString, ""},
	{"eval-url", kindString, "http://localhost:3770"},
	{"source-account", kindString, "primary"},
}

func runGate(args []string, s streams) int {
	p, err := parseArgs(gateFlags, args)
	if err != nil {
		s.errf("Error: %v\n", err)
		return exitBelowTreshold
	}
	if p.help {
		_, _ = io.WriteString(s.out, gateHelp)
		return exitPassed
	}
	if !p.set["tier"] {
		s.errf("Error: required flag(s) %q not set\n", "tier")
		return exitBelowTreshold
	}
	tier := p.str("tier")
	if !isValidTier(tier) {
		s.errf("error: invalid tier %q — valid values: %s\n", tier, strings.Join(validTiers, ", "))
		return exitValidation
	}
	client := &Client{BaseURL: p.str("eval-url"), SourceAccountID: p.str("source-account")}
	gs, err := client.GetGateStatus(context.Background(), tier)
	if err != nil {
		// HTTP 503/502 or ErrPreconditionNotMet signal an infrastructure precondition
		// failure (plugin-retrieval / BGE-M3 not wired): not a regression, exit 3.
		if isPreconditionError(err) {
			s.errf("Precondition not met: %v\n", err)
			s.errf("hint: ensure plugin-retrieval and nself-ai-gateway are running.\n")
			return exitPrecondition
		}
		s.errf("error: get gate status: %v\n", err)
		return exitValidation
	}
	WriteGateStatus(s.out, gs, true)
	if !gs.Cleared {
		return exitBelowTreshold
	}
	return exitPassed
}

// isPreconditionError returns true if err indicates a BGE-M3 / gateway precondition failure.
func isPreconditionError(err error) bool {
	if errors.Is(err, ErrPreconditionNotMet) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "HTTP 503") ||
		strings.Contains(msg, "HTTP 502") ||
		strings.Contains(msg, "precondition not met") ||
		strings.Contains(msg, "service unavailable")
}

// isValidTier returns true if tier is one of the known autonomy tier names.
func isValidTier(tier string) bool {
	for _, v := range validTiers {
		if v == tier {
			return true
		}
	}
	return false
}
