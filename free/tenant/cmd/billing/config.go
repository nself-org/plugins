package main

import (
	"fmt"
	"os"

	"github.com/nself-org/nself-tenant/internal/config"
)

// loadProjectConfig reads the project settings this command needs.
//
// In the CLI this lived in db_helpers.go and parsed the whole .env cascade.
// Here it reads the environment, which the CLI has already resolved from that
// cascade and passed in for every variable this plugin's manifest declares.
// The signature is unchanged so the moved command code did not have to be.
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
