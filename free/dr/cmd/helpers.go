package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/nself-dr/internal/config"
)

// Helpers the dr command shared with the CLI's db commands.
//
// Purpose: both lived in cmd/commands and were reachable from anywhere in that
// package. Once dr moved to its own binary they had to come with it. Neither
// has behaviour worth changing, so both are the CLI's, verbatim in substance.
//
// Inputs: the working directory, and the project name to confirm against.
//
// Outputs: the project configuration; or an error when the typed confirmation
// does not match.
//
// Constraints: requireProductionConfirmation reads from stdin, which the plugin
// inherits from nself, so an interactive confirmation still reaches the user
// running `nself dr ...`.

// loadProjectConfig reads the project settings this command needs.
//
// In the CLI this parsed the whole .env cascade. Here it reads the environment,
// which the CLI has already resolved from that cascade and passed in for every
// variable this plugin's manifest declares.
func loadProjectConfig() (*config.Config, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	return cfg, nil
}

// requireProductionConfirmation makes the operator type the project name before
// a destructive action proceeds against production.
func requireProductionConfirmation(projectName string) error {
	fmt.Printf("WARNING: PRODUCTION: This will DESTROY the database %s.\n", projectName)
	fmt.Print("   Type the project name to confirm: ")
	var confirm string
	if _, err := fmt.Scanln(&confirm); err != nil {
		return fmt.Errorf("reading confirmation: %w", err)
	}
	if strings.TrimSpace(confirm) != projectName {
		return fmt.Errorf("confirmation failed: got %q, expected %q", confirm, projectName)
	}
	return nil
}
