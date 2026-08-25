// Purpose: the plugin's cobra root. ProxyCommand in the core CLI's
// internal/plugin/router.go execs this binary as `nself-ai-studio <args...>`
// with the leading "ai-studio" argument already stripped, so AiStudioCmd
// (bridge.go) takes the place `nself ai-studio` used to occupy in the core
// binary. rootCmd is only an alias so main.go has one stable name to
// Execute() across every CLI-R11 plugin.
//
// Constraints: no dependency on any github.com/nself-org/cli/internal/*
// package — the original cmd/aistudio package had zero such imports (a
// fully self-contained Cloudflare Tunnel bridge + AI Studio proxy), so this
// is a verbatim move, not an adaptation.
package main

var rootCmd = AiStudioCmd
