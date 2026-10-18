// Package deploy handles type-checking of TypeScript source via Deno before activation.
package deploy

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// TypeCheckError is returned when Deno reports a type error in the source.
type TypeCheckError struct {
	Message string
}

func (e *TypeCheckError) Error() string {
	return fmt.Sprintf("type-check error: %s", e.Message)
}

// ErrTypeCheck is a sentinel for errors.As matching on *TypeCheckError.
// Usage: errors.As(err, &deploy.ErrTypeCheck)
var ErrTypeCheck *TypeCheckError

// TypeChecker runs deno check on TypeScript source before deployment.
type TypeChecker struct {
	denoBin string
}

// NewTypeChecker creates a TypeChecker using the given Deno binary path.
func NewTypeChecker(denoBin string) *TypeChecker {
	return &TypeChecker{denoBin: denoBin}
}

// Check writes source to a temp file and runs `deno check` against it.
// Returns a TypeCheckError if Deno reports type errors; nil on success.
// Skipped (returns nil) if the Deno binary is not present — allows local dev
// without Deno installed.
func (tc *TypeChecker) Check(ctx context.Context, source string) error {
	if _, err := os.Stat(tc.denoBin); os.IsNotExist(err) {
		// Deno not available; skip type-check (CI/integration env will catch it).
		return nil
	}

	tmp, err := os.CreateTemp("", "nself-typecheck-*.ts")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.WriteString(source); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	tmp.Close()

	// 30-second ceiling for type-check — generous but bounded.
	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(checkCtx, tc.denoBin,
		"check",
		"--no-config",
		"--no-npm",
		tmp.Name(),
	)
	// Blank env — no host vars needed for type-check.
	cmd.Env = []string{"DENO_NO_UPDATE_CHECK=1", "NO_COLOR=1"}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &bytes.Buffer{}

	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		return &TypeCheckError{Message: errMsg}
	}
	return nil
}
