// Package internal provides plugin-sms configuration and handlers.
//
// Purpose: SMS messaging via Twilio. Send, track opt-outs, enforce E.164.
// Inputs: HTTP requests with source_account_id from JWT header.
// Outputs: JSON responses with message data.
// Constraints:
//   - TWILIO_ACCOUNT_SID/AUTH_TOKEN/FROM_NUMBER are config-only; never user-overridable per request.
//   - Phone numbers must be validated as E.164 before sending.
//   - Opt-out list is checked before every send.
//   - source_account_id from X-Hasura-Source-Account-Id scopes all DB operations.
//   - requires_license: true — ctx.License.Valid() must return true.
package internal

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds plugin-sms runtime configuration.
type Config struct {
	DBUrl           string
	Port            int
	TwilioSID       string
	TwilioToken     string
	TwilioFrom      string
	RateLimitPerMin int
}

// LoadConfig reads configuration from environment variables.
func LoadConfig() (*Config, error) {
	dbURL := os.Getenv("NSELF_DB_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("NSELF_DB_URL is required")
	}
	port := 9009
	if p := os.Getenv("SMS_PLUGIN_PORT"); p != "" {
		var err error
		port, err = strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid SMS_PLUGIN_PORT: %w", err)
		}
	}
	rateLimit := 10
	if r := os.Getenv("SMS_RATE_LIMIT_PER_MIN"); r != "" {
		var err error
		rateLimit, err = strconv.Atoi(r)
		if err != nil {
			return nil, fmt.Errorf("invalid SMS_RATE_LIMIT_PER_MIN: %w", err)
		}
	}
	return &Config{
		DBUrl:           dbURL,
		Port:            port,
		TwilioSID:       os.Getenv("TWILIO_ACCOUNT_SID"),
		TwilioToken:     os.Getenv("TWILIO_AUTH_TOKEN"),
		TwilioFrom:      os.Getenv("TWILIO_FROM_NUMBER"),
		RateLimitPerMin: rateLimit,
	}, nil
}
