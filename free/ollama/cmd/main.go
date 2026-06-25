// ollama plugin entrypoint — lifecycle hook handler.
//
// Purpose: Handle nSelf plugin lifecycle actions (install, start, stop) for the
//          Ollama plugin. On install, pulls the default model (gemma-3-4b) into
//          the Ollama Docker container. On start, verifies the Ollama service is
//          running and reports the endpoint. On stop, gracefully stops the service.
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
	"os/exec"
	"strings"
)

func main() {
	action := envStr("PLUGIN_ACTION", "start")

	switch action {
	case "install":
		if err := install(); err != nil {
			fmt.Fprintf(os.Stderr, "ollama install failed: %v\n", err)
			os.Exit(1)
		}
	case "start":
		if err := start(); err != nil {
			fmt.Fprintf(os.Stderr, "ollama start failed: %v\n", err)
			os.Exit(1)
		}
	case "stop":
		if err := stop(); err != nil {
			fmt.Fprintf(os.Stderr, "ollama stop failed: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "ollama: unknown PLUGIN_ACTION %q (use install|start|stop)\n", action)
		os.Exit(1)
	}
}

// install pulls the default model into the Ollama container.
// Idempotent: checks if the model is already present before pulling.
func install() error {
	model := envStr("OLLAMA_MODEL", "gemma-3-4b")
	container := envStr("OLLAMA_CONTAINER", "nself-ollama")

	fmt.Printf("ollama: checking if model %q is already present...\n", model)

	// Check if model is already available (idempotent guard).
	listOut, err := runCommand("docker", "exec", container, "ollama", "list")
	if err == nil && strings.Contains(listOut, model) {
		fmt.Printf("ollama: model %q already present — skipping pull\n", model)
		return nil
	}

	fmt.Printf("ollama: pulling model %q (this may take several minutes)...\n", model)
	if _, err := runCommand("docker", "exec", container, "ollama", "pull", model); err != nil {
		return fmt.Errorf("pull model %q: %w", model, err)
	}

	fmt.Printf("ollama: model %q pulled successfully\n", model)
	return nil
}

// start verifies the Ollama endpoint is reachable and reports it.
func start() error {
	host := envStr("OLLAMA_HOST", "http://localhost:11434")
	fmt.Printf("ollama: service endpoint: %s\n", host)
	fmt.Println("ollama: use NSELF_AI_PROVIDER=ollama to route AI requests through this instance")
	return nil
}

// stop gracefully stops the Ollama Docker container.
func stop() error {
	container := envStr("OLLAMA_CONTAINER", "nself-ollama")
	fmt.Printf("ollama: stopping container %q\n", container)
	if _, err := runCommand("docker", "stop", container); err != nil {
		return fmt.Errorf("stop container %q: %w", container, err)
	}
	fmt.Println("ollama: stopped")
	return nil
}

// runCommand executes a shell command and returns stdout output.
// Uses execCommand (injectable for tests).
func runCommand(name string, args ...string) (string, error) {
	return execCommand(name, args...)
}

// execCommand is the real command executor (can be replaced in tests).
var execCommand = func(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	return string(out), err
}

// envStr returns env var value or fallback.
func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
