package internal

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	apnsProdHost    = "https://api.push.apple.com"
	apnsSandboxHost = "https://api.sandbox.push.apple.com"
)

// APNsClient sends push notifications via the APNs HTTP/2 provider API.
// Credentials are loaded at construction time and never re-read from env
// after that, so credential rotation requires a container restart.
type APNsClient struct {
	teamID   string
	keyID    string
	key      *ecdsa.PrivateKey
	bundleID string
	host     string
	http     *http.Client
}

// NewAPNsClient constructs an APNsClient from the given config.
// Returns an error if the PEM cannot be parsed — so startup fails fast
// rather than silently delivering nothing.
func NewAPNsClient(cfg *Config) (*APNsClient, error) {
	if !cfg.APNsEnabled() {
		return nil, nil // APNs not configured; caller checks for nil
	}

	key, err := parseAPNsKey(cfg.APNsKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("apns: parse key PEM: %w", err)
	}

	host := apnsProdHost
	if cfg.APNsSandbox {
		host = apnsSandboxHost
	}

	// HTTP/2 client with TLS 1.2+ (APNs requires TLS 1.2 minimum).
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		ForceAttemptHTTP2: true,
	}

	return &APNsClient{
		teamID:   cfg.APNsTeamID,
		keyID:    cfg.APNsKeyID,
		key:      key,
		bundleID: cfg.APNsBundleID,
		host:     host,
		http:     &http.Client{Timeout: 10 * time.Second, Transport: transport},
	}, nil
}

// APNsPayload is the notification payload sent to APNs.
type APNsPayload struct {
	APS map[string]interface{} `json:"aps"`
}

// SendResult holds the outcome of a single APNs delivery attempt.
type APNsResult struct {
	Success    bool
	Error      string
	StatusCode int
}

// Send delivers a notification to the given device token.
// The payload must be a valid APNs payload JSON ({"aps": {...}}).
// Returns a result indicating success or failure; never panics.
func (c *APNsClient) Send(ctx context.Context, deviceToken string, payload json.RawMessage) APNsResult {
	token, err := c.signedJWT()
	if err != nil {
		return APNsResult{Error: fmt.Sprintf("apns: sign JWT: %v", err)}
	}

	url := fmt.Sprintf("%s/3/device/%s", c.host, deviceToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return APNsResult{Error: fmt.Sprintf("apns: build request: %v", err)}
	}

	req.Header.Set("Authorization", "bearer "+token)
	req.Header.Set("apns-topic", c.bundleID)
	req.Header.Set("apns-push-type", "alert")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return APNsResult{Error: fmt.Sprintf("apns: http: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return APNsResult{Success: true, StatusCode: resp.StatusCode}
	}

	// Decode APNs error response for a useful last_error message.
	var apnsErr struct {
		Reason    string `json:"reason"`
		Timestamp int64  `json:"timestamp,omitempty"`
	}
	if decErr := json.NewDecoder(resp.Body).Decode(&apnsErr); decErr != nil {
		return APNsResult{StatusCode: resp.StatusCode, Error: fmt.Sprintf("apns: status %d", resp.StatusCode)}
	}

	errMsg := fmt.Sprintf("apns: status %d reason %s", resp.StatusCode, apnsErr.Reason)

	// Specific handling for expired or invalid credentials.
	if apnsErr.Reason == "ExpiredProviderToken" || apnsErr.Reason == "InvalidProviderToken" {
		errMsg = fmt.Sprintf("apns: credential error — %s (rotate PUSH_APNS_KEY_PEM and restart push container)", apnsErr.Reason)
	}

	return APNsResult{StatusCode: resp.StatusCode, Error: errMsg}
}

// signedJWT creates a new JWT signed with the loaded EC key.
// APNs tokens expire after 60 minutes; we generate a fresh one per request
// (cheap operation, avoids stale-token errors with no in-memory cache).
func (c *APNsClient) signedJWT() (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss": c.teamID,
		"iat": now.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = c.keyID

	signed, err := token.SignedString(c.key)
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}
	return signed, nil
}

// parseAPNsKey parses a PEM-encoded EC private key (Apple .p8 format).
func parseAPNsKey(pemStr string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in PUSH_APNS_KEY_PEM")
	}

	// Apple .p8 files are PKCS#8-encoded EC keys.
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS8: %w", err)
	}

	ecKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("PUSH_APNS_KEY_PEM is not an EC private key (got %T)", key)
	}

	// Sanity-check: EC key must have a non-zero D value.
	if ecKey.D == nil || ecKey.D.Cmp(big.NewInt(0)) == 0 {
		return nil, fmt.Errorf("parsed EC key has zero D scalar — key may be corrupt")
	}

	return ecKey, nil
}
