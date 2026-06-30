package lifecycle_test

// lifecycle_test.go — tests for the ollama plugin lifecycle package.
//
// Purpose: Verify that the Install action correctly checks for an existing model
//          and calls model pull when needed, using a mock docker exec runner.
//
// Inputs:  Injected ExecCommand mock that simulates docker exec responses.
// Outputs: Confirm model-pull sequence is called only when model is absent.
// Constraints: No Docker daemon required; no network calls.

import (
	"fmt"
	"testing"

	"github.com/nself-org/plugins/free/ollama/internal/lifecycle"
)

// TestInstall_ModelAlreadyPresent verifies that Install() skips pulling when the
// model is already listed in the Ollama container.
func TestInstall_ModelAlreadyPresent(t *testing.T) {
	t.Setenv("OLLAMA_MODEL", "gemma-3-4b")
	t.Setenv("OLLAMA_CONTAINER", "mock-ollama")

	pullCalled := false

	origExec := lifecycle.ExecCommand
	defer func() { lifecycle.ExecCommand = origExec }()

	lifecycle.ExecCommand = func(name string, args ...string) (string, error) {
		// args: ["exec", <container>, "ollama", "list"]
		if name == "docker" && len(args) >= 4 && args[3] == "list" {
			return "gemma-3-4b latest abc123 4.1 GB 2 days ago\n", nil
		}
		// args: ["exec", <container>, "ollama", "pull", <model>]
		if name == "docker" && len(args) >= 4 && args[3] == "pull" {
			pullCalled = true
			return "", nil
		}
		return "", fmt.Errorf("unexpected command: %s %v", name, args)
	}

	if err := lifecycle.Install(); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	if pullCalled {
		t.Error("Install() should not pull when model is already present")
	}
}

// TestInstall_ModelNotPresent verifies that Install() triggers a model pull
// when the model is absent from the Ollama container.
func TestInstall_ModelNotPresent(t *testing.T) {
	t.Setenv("OLLAMA_MODEL", "gemma-3-4b")
	t.Setenv("OLLAMA_CONTAINER", "mock-ollama")

	pullCalled := false

	origExec := lifecycle.ExecCommand
	defer func() { lifecycle.ExecCommand = origExec }()

	lifecycle.ExecCommand = func(name string, args ...string) (string, error) {
		// args: ["exec", <container>, "ollama", "list"]
		if name == "docker" && len(args) >= 4 && args[3] == "list" {
			// Model not in list — return different model only
			return "llama3.2:3b latest xyz456 2.0 GB\n", nil
		}
		// args: ["exec", <container>, "ollama", "pull", <model>]
		if name == "docker" && len(args) >= 4 && args[3] == "pull" {
			pullCalled = true
			return "pulling gemma-3-4b...\n", nil
		}
		return "", fmt.Errorf("unexpected command: %s %v", name, args)
	}

	if err := lifecycle.Install(); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	if !pullCalled {
		t.Error("Install() should have pulled model when absent")
	}
}
