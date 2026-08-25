// Package ports holds the fixed port numbers this plugin needs for the AI
// services it talks to.
//
// Constraints: this is a narrow copy of two constants from cli/internal/ports
// (CLI-R11) — that package is unreachable from this plugin module. The
// values are a stable, documented product contract (SPORT F10 port
// registry), not something expected to drift.
package ports

const (
	// AICCPort is the port for nself-ai-cc (Claude Code PTY session relay).
	AICCPort = 3760

	// AIGatewayPort is the port for nself-ai-gateway (AI provider key vault +
	// routing + quota enforcement). The claw proxy (cmd/claw_proxy.go) routes
	// exclusively here post-E6.
	AIGatewayPort = 3761
)
