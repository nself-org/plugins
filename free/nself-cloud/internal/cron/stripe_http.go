package cron

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// issueStripeRefundHTTP is a lightweight fallback for watchdog compensating refunds.
// The full Stripe client is in internal/stripe; this avoids a circular import.
func issueStripeRefundHTTP(ctx context.Context, chargeID, idempotencyKey string) error {
	secretKey := os.Getenv("STRIPE_PLATFORM_SECRET_KEY")
	if secretKey == "" {
		return fmt.Errorf("stripe_http: STRIPE_PLATFORM_SECRET_KEY not set")
	}

	params := url.Values{}
	params.Set("charge", chargeID)

	req, err := http.NewRequestWithContext(ctx,
		http.MethodPost,
		"https://api.stripe.com/v1/refunds",
		strings.NewReader(params.Encode()),
	)
	if err != nil {
		return fmt.Errorf("stripe_http: build refund request: %w", err)
	}
	req.SetBasicAuth(secretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}

	cli := &http.Client{Timeout: 20 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("stripe_http: refund request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("stripe_http: refund HTTP %d for charge %s", resp.StatusCode, chargeID)
	}
	return nil
}
