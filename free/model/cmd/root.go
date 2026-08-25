// Purpose: the plugin's cobra root. ProxyCommand in the core CLI's
// internal/plugin/router.go execs this binary as `nself-model <args...>`
// with the leading "model" argument already stripped, so modelCmd (model.go)
// takes the place `nself model` used to occupy in the core binary. rootCmd is
// only an alias so main.go has one stable name to Execute() across every
// CLI-R11 plugin.
//
// `ollama` is not a separate top-level plugin: cli/cmd/commands/ollama.go
// registered `ollamaCmd` as a child of modelCmd, not of RootCmd — CLI-R09
// made bare `nself ollama` an argv rewrite onto `nself model ollama`
// (cmd/commands/legacy_spellings.go: "ollama": {canonical: []string{"model",
// "ollama"}}). That rewrite still fires in core (it runs before the plugin
// proxy check), producing argv ["model", "ollama", ...] which core then
// proxies to this binary since "model" is no longer a registered core
// command. ollama.go moved here unchanged, preserving the same subcommand
// relationship, so `nself-model ollama ...` resolves exactly as `nself model
// ollama ...` did pre-extraction.
//
// Constraints: no dependency on any github.com/nself-org/cli/internal/*
// package — model.go/ollama.go had zero such imports before extraction (both
// are pure HTTP clients to the Ollama API), so this is a verbatim move.
package main

var rootCmd = modelCmd
