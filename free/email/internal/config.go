// Package internal provides plugin-email configuration and handlers.
//
// Purpose: Transactional email via Elastic Email API. Send, template, track.
// Inputs: HTTP requests authenticated via X-Hasura-Source-Account-Id header.
// Outputs: JSON responses with message/template data.
// Constraints:
//   - ELASTIC_EMAIL_API_KEY is config-only; never user-overridable per request (SSRF N/A).
//   - source_account_id from X-Hasura-Source-Account-Id scopes all DB operations.
//   - requires_license: true — ctx.License.Valid() must return true.
package internal

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds plugin-email runtime configuration.
type Config struct {
	DBUrl           string
	Port            int
	ElasticAPIKey   string
	ElasticFrom     string
}

// LoadConfig reads configuration from environment variables.
func LoadConfig() (*Config, error) {
	dbURL := os.Getenv("NSELF_DB_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("NSELF_DB_URL is required")
	}
	port := 9008
	if p := os.Getenv("EMAIL_PLUGIN_PORT"); p != "" {
		var err error
		port, err = strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid EMAIL_PLUGIN_PORT: %w", err)
		}
	}
	return &Config{
		DBUrl:         dbURL,
		Port:          port,
		ElasticAPIKey: os.Getenv("ELASTIC_EMAIL_API_KEY"),
		ElasticFrom:   os.Getenv("ELASTIC_EMAIL_FROM"),
	}, nil
}
