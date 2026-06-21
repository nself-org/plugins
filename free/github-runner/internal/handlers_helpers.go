package internal

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func tailLines(r io.ReadSeeker, n int) ([]string, error) {
	// Read all content and return the last n lines. Log files for a
	// self-hosted runner are small enough that a full read is fine.
	scanner := bufio.NewScanner(r)
	var all []string
	for scanner.Scan() {
		all = append(all, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	if len(all) <= n {
		return all, nil
	}
	return all[len(all)-n:], nil
}

// --------------------------------------------------------------------------
// Path helpers
// --------------------------------------------------------------------------

// runnerDir returns the directory where the runner binary and config live.
func runnerDir() string {
	if d := os.Getenv("GITHUB_RUNNER_DIR"); d != "" {
		return d
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = "/root"
	}
	return filepath.Join(home, defaultRunnerDir)
}

// logFilePath returns the path to the runner log file.
func logFilePath() string {
	if p := os.Getenv("GITHUB_RUNNER_LOG_PATH"); p != "" {
		return p
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = "/root"
	}
	return filepath.Join(home, ".nself", "logs", "plugins", "github-runner", "runner.log")
}
