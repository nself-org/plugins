// Package auth provides a minimal, read-only view of the CLI's credential
// store for plugins that only need to authenticate against a backend
// service, never to log in/out themselves.
//
// Purpose: read ~/.nself/auth.json to get the access token `nself login`
// already wrote there.
//
// Inputs: none (reads a fixed path under the user's home directory).
//
// Outputs: an *AuthFile with the access token, or ErrNotLoggedIn.
//
// Constraints: this is a narrow copy of cli/internal/auth's ReadAuthFile
// (CLI-R11) — that package is unreachable from this plugin module (it lives
// inside github.com/nself-org/cli, a separate Go module) and is otherwise a
// large, actively-changing package (login/OAuth/device-code flows) that this
// plugin has no business depending on. Only the read path is copied; this
// plugin never writes ~/.nself/auth.json.
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// authFileName is the credential file name within ~/.nself/
const authFileName = "auth.json"

// ErrNotLoggedIn is returned when no auth.json exists or token is missing.
var ErrNotLoggedIn = errors.New("not logged in — run 'nself login' to authenticate")

// AuthFile holds the persisted auth credentials this plugin cares about.
// Mirrors cli/internal/auth.AuthFile's JSON shape; unused fields are omitted.
type AuthFile struct {
	AccessToken string `json:"access_token"`
}

// authFilePath returns the absolute path to ~/.nself/auth.json.
func authFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".nself", authFileName), nil
}

// ReadAuthFile reads and parses ~/.nself/auth.json.
// Returns ErrNotLoggedIn if the file does not exist or has no access token.
func ReadAuthFile() (*AuthFile, error) {
	path, err := authFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotLoggedIn
		}
		return nil, fmt.Errorf("reading auth file: %w", err)
	}

	var af AuthFile
	if err := json.Unmarshal(data, &af); err != nil {
		return nil, fmt.Errorf("parsing auth file: %w", err)
	}

	if af.AccessToken == "" {
		return nil, ErrNotLoggedIn
	}

	return &af, nil
}
