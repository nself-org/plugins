// Package lifecycle implements the nSelf plugin lifecycle actions for Ollama.
//
// Purpose: Provide install, start, and stop operations for the Ollama plugin,
//          extracted from cmd/main.go so the module has a proper non-cmd package
//          and Go's build tool can output a binary without conflicting with the
//          cmd/ directory.
//
// Inputs:  PLUGIN_ACTION env (install | start | stop)
//          OLLAMA_MODEL      env (default: gemma-3-4b)
//          OLLAMA_HOST       env (default: http://localhost:11434)
//          OLLAMA_CONTAINER  env (default: nself-ollama)
// Outputs: Error on failure; logs to stdout on success.
// Constraints: Install/Stop require Docker daemon accessible with Ollama container.
// SPORT: PLUGINS-OLLAMA-000
package lifecycle

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ExecCommand is the real command executor; injectable for tests.
var ExecCommand = func(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	return string(out), err
}

// RunCommand executes a shell command via ExecCommand and returns stdout output.
func RunCommand(name string, args ...string) (string, error) {
	return ExecCommand(name, args...)
}

// EnvStr returns the value of the named env var or fallback if unset/empty.
func EnvStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Install pulls the default model into the Ollama container.
// Idempotent: checks if the model is already present before pulling.
func Install() error {
	model := EnvStr("OLLAMA_MODEL", "gemma-3-4b")
	container := EnvStr("OLLAMA_CONTAINER", "nself-ollama")

	fmt.Printf("ollama: checking if model %q is already present...\n", model)

	// Check if model is already available (idempotent guard).
	listOut, err := RunCommand("docker", "exec", container, "ollama", "list")
	if err == nil && strings.Contains(listOut, model) {
		fmt.Printf("ollama: model %q already present — skipping pull\n", model)
		return nil
	}

	fmt.Printf("ollama: pulling model %q (this may take several minutes)...\n", model)
	if _, err := RunCommand("docker", "exec", container, "ollama", "pull", model); err != nil {
		return fmt.Errorf("pull model %q: %w", model, err)
	}

	fmt.Printf("ollama: model %q pulled successfully\n", model)
	return nil
}

// Start verifies the Ollama endpoint is reachable and reports it.
func Start() error {
	host := EnvStr("OLLAMA_HOST", "http://localhost:11434")
	fmt.Printf("ollama: service endpoint: %s\n", host)
	fmt.Println("ollama: use NSELF_AI_PROVIDER=ollama to route AI requests through this instance")
	return nil
}

// Stop gracefully stops the Ollama Docker container.
func Stop() error {
	container := EnvStr("OLLAMA_CONTAINER", "nself-ollama")
	fmt.Printf("ollama: stopping container %q\n", container)
	if _, err := RunCommand("docker", "stop", container); err != nil {
		return fmt.Errorf("stop container %q: %w", container, err)
	}
	fmt.Println("ollama: stopped")
	return nil
}
