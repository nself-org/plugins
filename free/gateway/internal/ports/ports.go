// Package ports holds the fixed port numbers for the AI service trio this
// plugin talks to.
//
// Purpose: name the three ports instead of hardcoding magic numbers inline.
//
// Constraints: this is a narrow copy of the three constants this plugin
// needs from cli/internal/ports (CLI-R11) — that package is unreachable from
// this plugin module. The values are a stable, documented product contract
// (SPORT F10 port registry), not something expected to drift.
package ports

const (
	// AICCPort is the port for nself-ai-cc (Claude Code PTY session relay).
	AICCPort = 3760

	// AIGatewayPort is the port for nself-ai-gateway (AI provider key vault +
	// routing + quota enforcement).
	AIGatewayPort = 3761

	// AIMCPPort is the port for nself-ai-mcp (MCP tool server).
	AIMCPPort = 3762
)
