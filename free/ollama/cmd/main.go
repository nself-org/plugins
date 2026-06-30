// ollama plugin entrypoint — lifecycle hook handler.
//
// Purpose: Handle nSelf plugin lifecycle actions (install, start, stop) for the
//          Ollama plugin. Dispatches to the lifecycle package for all logic.
//
// Inputs:  PLUGIN_ACTION env (install | start | stop)
//          OLLAMA_MODEL   env (default: gemma-3-4b)
//          OLLAMA_HOST    env (default: http://localhost:11434)
// Outputs: Exit 0 on success, non-zero on failure. Logs to stdout.
// Constraints: Requires Docker daemon accessible; Ollama container must be
//              running before "start" action is called.
// SPORT: PLUGINS-OLLAMA-000
package main

import (
	"fmt"
	"os"

	"github.com/nself-org/plugins/free/ollama/internal/lifecycle"
)

func main() {
	action := lifecycle.EnvStr("PLUGIN_ACTION", "start")

	switch action {
	case "install":
		if err := lifecycle.Install(); err != nil {
			fmt.Fprintf(os.Stderr, "ollama install failed: %v\n", err)
			os.Exit(1)
		}
	case "start":
		if err := lifecycle.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "ollama start failed: %v\n", err)
			os.Exit(1)
		}
	case "stop":
		if err := lifecycle.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "ollama stop failed: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "ollama: unknown PLUGIN_ACTION %q (use install|start|stop)\n", action)
		os.Exit(1)
	}
}
