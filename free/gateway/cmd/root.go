// Purpose: the plugin's cobra root. ProxyCommand in the core CLI's
// internal/plugin/router.go execs this binary as `nself-gateway <args...>`
// with the leading "gateway" argument already stripped, so gatewayCmd
// (gateway.go) takes the place `nself gateway` used to occupy in the core
// binary. rootCmd is only an alias so main.go has one stable name to
// Execute() across every CLI-R11 plugin.
//
// Constraints: no dependency on any github.com/nself-org/cli/internal/*
// package — those are unreachable from outside the cli module. The original
// gateway_cmd*.go files depended on internal/auth (ReadAuthFile only) and
// internal/gateway + internal/ports; all three are carried into this
// module's own internal/ tree, narrowed to exactly what the gateway
// commands use (see internal/auth/storage.go and internal/ports/ports.go
// for what was intentionally left out).
package main

var rootCmd = gatewayCmd
