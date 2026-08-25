// Purpose: the plugin's cobra root. ProxyCommand in the core CLI's
// internal/plugin/router.go execs this binary as `nself-claw <args...>`
// with the leading "claw" argument already stripped, so clawCmd (claw.go)
// takes the place `nself claw` used to occupy in the core binary. rootCmd
// is only an alias so main.go has one stable name to Execute() across every
// CLI-R11 plugin.
//
// Constraints: no dependency on any github.com/nself-org/cli/internal/*
// package — those are unreachable from outside the cli module. The
// original claw*.go files depended on internal/auth (ReadAuthFile only, for
// claw_session.go), internal/ports (2 constants), internal/httptimeout (the
// Default client only), internal/errs (the Exit(code) shape only), and
// internal/claw + internal/config (claw_migrate.go / claw_pair.go) — all
// carried into this module's own internal/ tree, each narrowed to exactly
// what the claw commands use. See internal/projectenv's doc comment for the
// internal/config replacement rationale specifically; it is the deepest of
// these narrowings.
package main

var rootCmd = clawCmd
