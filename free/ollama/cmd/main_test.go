package main

// main_test.go — tests for the ollama plugin lifecycle entrypoint.
//
// Purpose: Verify that the install action correctly checks for an existing model
//          and calls model pull when needed, using a mock docker exec runner.
//
// Inputs:  Injected execCommand mock that simulates docker exec responses.
// Outputs: Confirm model-pull sequence is called only when model is absent.
// Constraints: No Docker daemon required; no network calls.

import (
	"fmt"
	"testing"
)

// TestInstall_ModelAlreadyPresent verifies that install() skips pulling when the
// model is already listed in the Ollama container.
func TestInstall_ModelAlreadyPresent(t *testing.T) {
	t.Setenv("OLLAMA_MODEL", "gemma-3-4b")
	t.Setenv("OLLAMA_CONTAINER", "mock-ollama")

	pullCalled := false

	// Mock: "ollama list" returns model name; "ollama pull" should NOT be called.
	origExec := execCommand
	defer func() { execCommand = origExec }()

	execCommand = func(name string, args ...string) (string, error) {
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

	if err := install(); err != nil {
		t.Fatalf("install() error: %v", err)
	}

	if pullCalled {
		t.Error("install() should not pull when model is already present")
	}
}

// TestInstall_ModelNotPresent verifies that install() triggers a model pull
// when the model is absent from the Ollama container.
func TestInstall_ModelNotPresent(t *testing.T) {
	t.Setenv("OLLAMA_MODEL", "gemma-3-4b")
	t.Setenv("OLLAMA_CONTAINER", "mock-ollama")

	pullCalled := false

	origExec := execCommand
	defer func() { execCommand = origExec }()

	execCommand = func(name string, args ...string) (string, error) {
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

	if err := install(); err != nil {
		t.Fatalf("install() error: %v", err)
	}

	if !pullCalled {
		t.Error("install() should have pulled model when absent")
	}
}
