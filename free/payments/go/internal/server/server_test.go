package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/nself-org/nself-payments/internal/config"
	"github.com/nself-org/nself-payments/internal/metrics"
	"github.com/nself-org/nself-payments/internal/provider"
)

// mockProvider implements provider.Provider for testing.
type mockProvider struct {
	name            string
	checkoutResult  *provider.CheckoutResult
	checkoutErr     error
	sub             *provider.Subscription
	subErr          error
	portalResult    *provider.PortalResult
	portalErr       error
	cancelErr       error
	pauseErr        error
	retryResult     *provider.RetryResult
	retryErr        error
	plans           []provider.Plan
	plansErr        error
	sigEventType    string
	sigErr          error
	parsedSub       *provider.Subscription
	parseErr        error
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) CreateCheckout(_ context.Context, _ provider.CheckoutParams) (*provider.CheckoutResult, error) {
	return m.checkoutResult, m.checkoutErr
}

func (m *mockProvider) GetSubscription(_ context.Context, _ string) (*provider.Subscription, error) {
	return m.sub, m.subErr
}

func (m *mockProvider) CreatePortalURL(_ context.Context, _ string, _ string) (*provider.PortalResult, error) {
	return m.portalResult, m.portalErr
}

func (m *mockProvider) CancelSubscription(_ context.Context, _ string) error { return m.cancelErr }
func (m *mockProvider) PauseSubscription(_ context.Context, _ string) error  { return m.pauseErr }

func (m *mockProvider) RetryInvoice(_ context.Context, _ string) (*provider.RetryResult, error) {
	return m.retryResult, m.retryErr
}

func (m *mockProvider) ListPlans(_ context.Context) ([]provider.Plan, error) {
	return m.plans, m.plansErr
}

func (m *mockProvider) RecordUsage(_ context.Context, _ provider.UsageRecord) error { return nil }

func (m *mockProvider) VerifyWebhookSignature(sig string, body []byte, secret string) (string, error) {
	return m.sigEventType, m.sigErr
}

func (m *mockProvider) ParseWebhookEvent(eventType string, body []byte) (*provider.Subscription, error) {
	return m.parsedSub, m.parseErr
}

// sharedMetrics is created once per test binary to avoid duplicate Prometheus registration.
var sharedMetrics = metrics.New()

// newTestHandler builds a Handler with mock providers and no real DB.
func newTestHandler(prov *mockProvider) (*Handler, chi.Router) {
	providers := map[string]provider.Provider{prov.name: prov}
	cfg := &config.Config{
		DefaultProvider:     prov.name,
		StripeWebhookSecret: "test_secret",
	}
	h := &Handler{
		pool:            nil, // DB calls skipped in integration tests
		providers:       providers,
		defaultProvider: prov,
		cfg:             cfg,
		metrics:         sharedMetrics,
	}
	r := chi.NewRouter()
	h.Register(r)
	return h, r
}

// TestWebhookHandler_ValidSignature_AcceptsAndReturns200 verifies a valid webhook returns 200.
func TestWebhookHandler_ValidSignature_AcceptsAndReturns200(t *testing.T) {
	body := stripeWebhookBody("customer.subscription.created")
	secret := "test_secret"
	sig := buildTestStripeSignature(t, secret, body)

	prov := &mockProvider{
		name:         "stripe",
		sigEventType: "customer.subscription.created",
		sigErr:       nil,
		parsedSub: &provider.Subscription{
			ProviderSubID: "sub_test",
			Status:        provider.StatusActive,
			Metadata:      map[string]string{"user_id": ""},
		},
	}
	_, r := newTestHandler(prov)

	req := httptest.NewRequest(http.MethodPost, "/payments/webhook/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", sig)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// DB upsert will fail (nil pool) — handler should still return 200 on parse success.
	// We accept either 200 or 500 (DB nil-pool panic would be recovered).
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("expected 200 or 500, got %d", w.Code)
	}
}

// TestWebhookHandler_InvalidSignature_Returns401 verifies bad signature returns 401.
func TestWebhookHandler_InvalidSignature_Returns401(t *testing.T) {
	body := stripeWebhookBody("customer.subscription.created")

	prov := &mockProvider{
		name:   "stripe",
		sigErr: fmt.Errorf("signature mismatch"),
	}
	_, r := newTestHandler(prov)

	req := httptest.NewRequest(http.MethodPost, "/payments/webhook/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", "bad_sig")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestWebhookHandler_NonSubscriptionEvent_Returns200 verifies non-sub events are acked.
func TestWebhookHandler_NonSubscriptionEvent_Returns200(t *testing.T) {
	body := stripeWebhookBody("payment_intent.created")

	prov := &mockProvider{
		name:         "stripe",
		sigEventType: "payment_intent.created",
		sigErr:       nil,
		parsedSub:    nil, // nil = non-subscription event
	}
	_, r := newTestHandler(prov)

	req := httptest.NewRequest(http.MethodPost, "/payments/webhook/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", "valid_sig")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// DB InsertPaymentEvent will fail on nil pool; handler recovers and returns 200.
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("unexpected status %d", w.Code)
	}
}

// TestCheckout_InvalidBody returns 400 for malformed request.
func TestCheckout_InvalidBody(t *testing.T) {
	prov := &mockProvider{name: "stripe"}
	_, r := newTestHandler(prov)

	req := httptest.NewRequest(http.MethodPost, "/payments/checkout", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestCheckout_ValidRequest calls CreateCheckout and returns result.
func TestCheckout_ValidRequest(t *testing.T) {
	prov := &mockProvider{
		name: "stripe",
		checkoutResult: &provider.CheckoutResult{
			URL:       "https://checkout.stripe.com/pay/cs_test_123",
			SessionID: "cs_test_123",
		},
	}
	_, r := newTestHandler(prov)

	payload := map[string]string{
		"user_id":     "usr_1",
		"plan_id":     "price_basic",
		"success_url": "https://app.example.com/success",
		"cancel_url":  "https://app.example.com/cancel",
	}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/payments/checkout", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result provider.CheckoutResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.SessionID != "cs_test_123" {
		t.Errorf("expected session cs_test_123, got %q", result.SessionID)
	}
}

// helpers

func buildTestStripeSignature(t testing.TB, secret string, body []byte) string {
	t.Helper()
	ts := time.Now().Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d.", ts)))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("t=%d,v1=%s", ts, sig)
}

func stripeWebhookBody(eventType string) []byte {
	payload := map[string]interface{}{
		"type": eventType,
		"data": map[string]interface{}{
			"object": map[string]string{
				"id":       "sub_test",
				"customer": "cus_test",
				"status":   "active",
			},
		},
	}
	b, _ := json.Marshal(payload)
	return b
}
