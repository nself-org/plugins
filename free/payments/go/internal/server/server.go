// Package server implements the HTTP handlers for the payments plugin.
package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-payments/internal/config"
	"github.com/nself-org/nself-payments/internal/db"
	"github.com/nself-org/nself-payments/internal/metrics"
	"github.com/nself-org/nself-payments/internal/provider"
)

// Handler holds dependencies for the HTTP handler layer.
type Handler struct {
	pool            *pgxpool.Pool
	providers       map[string]provider.Provider
	defaultProvider provider.Provider
	cfg             *config.Config
	metrics         *metrics.Metrics
}

// New creates a new Handler.
func New(
	pool *pgxpool.Pool,
	providers map[string]provider.Provider,
	defaultProvider provider.Provider,
	cfg *config.Config,
	m *metrics.Metrics,
) *Handler {
	return &Handler{
		pool:            pool,
		providers:       providers,
		defaultProvider: defaultProvider,
		cfg:             cfg,
		metrics:         m,
	}
}

// Register wires all routes into the chi router.
func (h *Handler) Register(r chi.Router) {
	r.Post("/payments/checkout", h.handleCheckout)
	r.Get("/payments/subscription/{uid}", h.handleGetSubscription)
	r.Post("/payments/portal", h.handlePortal)
	r.Post("/payments/cancel", h.handleCancel)
	r.Post("/payments/pause", h.handlePause)
	r.Get("/payments/plans", h.handleListPlans)
	r.Post("/payments/meter/{metric_id}", h.handleRecordUsage)
	r.Get("/payments/meter/{metric_id}/summary", h.handleUsageSummary)

	// Webhook routes — one per provider.
	r.Post("/payments/webhook/stripe", h.handleWebhook("stripe"))
	r.Post("/payments/webhook/lemonsqueezy", h.handleWebhook("lemonsqueezy"))
	r.Post("/payments/webhook/paddle", h.handleWebhook("paddle"))
}

// checkoutRequest is the request body for POST /payments/checkout.
type checkoutRequest struct {
	Provider   string            `json:"provider"`   // optional; defaults to configured default
	UserID     string            `json:"user_id"`
	PlanID     string            `json:"plan_id"`
	Email      string            `json:"email"`
	SuccessURL string            `json:"success_url"`
	CancelURL  string            `json:"cancel_url"`
	Metadata   map[string]string `json:"metadata"`
}

func (h *Handler) handleCheckout(w http.ResponseWriter, r *http.Request) {
	var req checkoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	prov := h.defaultProvider
	if req.Provider != "" {
		p, ok := h.providers[req.Provider]
		if !ok {
			writeError(w, http.StatusBadRequest, "unknown provider: "+req.Provider)
			return
		}
		prov = p
	}

	// Resolve idempotency key: prefer X-Request-Id header, fall back to a
	// freshly generated UUID v4. This value is forwarded to the provider as the
	// Idempotency-Key so that network retries cannot cause a double-charge.
	requestUUID := r.Header.Get("X-Request-Id")
	if requestUUID == "" {
		requestUUID = uuid.New().String()
	}

	// Deduplication check: if we already created a session for this request UUID
	// within the last 24 h, return the cached result without hitting Stripe again.
	if h.pool != nil {
		if entry, err := db.GetCheckoutIdempotency(r.Context(), h.pool, requestUUID); err == nil && entry != nil {
			slog.Info("checkout: returning cached session", "request_uuid", requestUUID, "session_id", entry.SessionID)
			writeJSON(w, http.StatusOK, &provider.CheckoutResult{
				URL:       entry.SessionURL,
				SessionID: entry.SessionID,
			})
			return
		}
	}

	start := time.Now()
	result, err := prov.CreateCheckout(r.Context(), provider.CheckoutParams{
		UserID:      req.UserID,
		PlanID:      req.PlanID,
		SuccessURL:  req.SuccessURL,
		CancelURL:   req.CancelURL,
		Email:       req.Email,
		Metadata:    req.Metadata,
		RequestUUID: requestUUID,
	})
	h.metrics.CheckoutDuration.WithLabelValues(prov.Name()).Observe(time.Since(start).Seconds())

	if err != nil {
		slog.Error("checkout failed", "provider", prov.Name(), "error", err)
		writeError(w, http.StatusBadGateway, "provider error")
		return
	}

	// Persist (request_uuid, session_id, session_url) for 24 h TTL so that client
	// retries with the same X-Request-Id receive the same session without a
	// duplicate Stripe charge.
	if h.pool != nil {
		if storeErr := db.SetCheckoutIdempotency(r.Context(), h.pool, requestUUID, result.SessionID, result.URL); storeErr != nil {
			// Non-fatal: log and continue. The Stripe-side idempotency key still
			// protects against double-charges; the DB entry is belt-and-suspenders.
			slog.Warn("checkout: failed to persist idempotency entry", "request_uuid", requestUUID, "error", storeErr)
		}
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) handleGetSubscription(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	sub, err := db.GetSubscriptionByUser(r.Context(), h.pool, uid)
	if err != nil {
		slog.Error("get subscription failed", "user_id", uid, "error", err)
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	writeJSON(w, http.StatusOK, sub)
}

type portalRequest struct {
	Provider           string `json:"provider"`
	ProviderCustomerID string `json:"provider_customer_id"`
	ReturnURL          string `json:"return_url"`
}

func (h *Handler) handlePortal(w http.ResponseWriter, r *http.Request) {
	var req portalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	prov := h.defaultProvider
	if req.Provider != "" {
		p, ok := h.providers[req.Provider]
		if !ok {
			writeError(w, http.StatusBadRequest, "unknown provider: "+req.Provider)
			return
		}
		prov = p
	}

	result, err := prov.CreatePortalURL(r.Context(), req.ProviderCustomerID, req.ReturnURL)
	if err != nil {
		slog.Error("portal failed", "provider", prov.Name(), "error", err)
		writeError(w, http.StatusBadGateway, "provider error")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type cancelRequest struct {
	Provider      string `json:"provider"`
	ProviderSubID string `json:"provider_sub_id"`
}

func (h *Handler) handleCancel(w http.ResponseWriter, r *http.Request) {
	var req cancelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	prov := h.resolveProvider(req.Provider)
	if err := prov.CancelSubscription(r.Context(), req.ProviderSubID); err != nil {
		slog.Error("cancel failed", "provider", prov.Name(), "error", err)
		writeError(w, http.StatusBadGateway, "provider error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type pauseRequest struct {
	Provider      string `json:"provider"`
	ProviderSubID string `json:"provider_sub_id"`
}

func (h *Handler) handlePause(w http.ResponseWriter, r *http.Request) {
	var req pauseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	prov := h.resolveProvider(req.Provider)
	if err := prov.PauseSubscription(r.Context(), req.ProviderSubID); err != nil {
		slog.Error("pause failed", "provider", prov.Name(), "error", err)
		writeError(w, http.StatusBadGateway, "provider error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleListPlans(w http.ResponseWriter, r *http.Request) {
	provName := r.URL.Query().Get("provider")
	prov := h.resolveProvider(provName)

	plans, err := prov.ListPlans(r.Context())
	if err != nil {
		slog.Error("list plans failed", "provider", prov.Name(), "error", err)
		writeError(w, http.StatusBadGateway, "provider error")
		return
	}
	writeJSON(w, http.StatusOK, plans)
}

type usageRequest struct {
	SubscriptionID string `json:"subscription_id"`
	Quantity       int    `json:"quantity"`
}

func (h *Handler) handleRecordUsage(w http.ResponseWriter, r *http.Request) {
	metricID := chi.URLParam(r, "metric_id")

	var req usageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	subID, err := uuid.Parse(req.SubscriptionID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid subscription_id")
		return
	}

	if err := db.RecordUsage(r.Context(), h.pool, subID, metricID, req.Quantity); err != nil {
		slog.Error("record usage failed", "metric_id", metricID, "error", err)
		writeError(w, http.StatusInternalServerError, "storage error")
		return
	}
	h.metrics.UsageRecords.WithLabelValues(metricID).Add(float64(req.Quantity))
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleUsageSummary(w http.ResponseWriter, r *http.Request) {
	metricID := chi.URLParam(r, "metric_id")
	subIDStr := r.URL.Query().Get("subscription_id")

	subID, err := uuid.Parse(subIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid subscription_id query param")
		return
	}

	total, err := db.GetUsageSummary(r.Context(), h.pool, subID, metricID)
	if err != nil {
		slog.Error("usage summary failed", "metric_id", metricID, "error", err)
		writeError(w, http.StatusInternalServerError, "storage error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"total": total, "metric_id": 0, "quantity": total})
}

// handleWebhook returns a handler closure for the given provider name.
func (h *Handler) handleWebhook(providerName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		prov, ok := h.providers[providerName]
		if !ok {
			http.Error(w, "provider not configured", http.StatusServiceUnavailable)
			return
		}

		// Read raw body for HMAC verification — must happen before any JSON parsing.
		rawBody, err := io.ReadAll(io.LimitReader(r.Body, 2<<20)) // 2 MiB limit
		if err != nil {
			slog.Error("webhook: read body", "provider", providerName, "error", err)
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}

		// Select the correct signature header per provider.
		var sigHeader string
		var webhookSecret string
		switch providerName {
		case "stripe":
			sigHeader = r.Header.Get("Stripe-Signature")
			webhookSecret = h.cfg.StripeWebhookSecret
		case "lemonsqueezy":
			sigHeader = r.Header.Get("X-Signature")
			webhookSecret = h.cfg.LemonSqueezyWebhookSecret
		case "paddle":
			sigHeader = r.Header.Get("Paddle-Signature")
			webhookSecret = h.cfg.PaddleWebhookSecret
		}

		// Signature verification — reject unverified requests with 401.
		eventType, err := prov.VerifyWebhookSignature(sigHeader, rawBody, webhookSecret)
		if err != nil {
			slog.Warn("webhook: signature rejected", "provider", providerName, "error", err)
			h.metrics.WebhookTotal.WithLabelValues(providerName, "unknown", "rejected").Inc()
			http.Error(w, "signature verification failed", http.StatusUnauthorized)
			return
		}

		// Persist raw event for audit before any processing.
		if h.pool != nil {
			if err := db.InsertPaymentEvent(r.Context(), h.pool, providerName, eventType, rawBody); err != nil {
				slog.Error("webhook: persist event", "provider", providerName, "error", err)
				// Continue processing — don't fail the webhook on audit log write error.
			}
		}

		// Parse into canonical subscription (nil = non-subscription event, ack and return).
		sub, err := prov.ParseWebhookEvent(eventType, rawBody)
		if err != nil {
			slog.Error("webhook: parse event", "provider", providerName, "event_type", eventType, "error", err)
			h.metrics.WebhookTotal.WithLabelValues(providerName, eventType, "error").Inc()
			// Return 200 to prevent provider retries on our parse errors.
			w.WriteHeader(http.StatusOK)
			return
		}

		if sub != nil && h.pool != nil {
			// Upsert subscription with ON CONFLICT idempotency.
			userID := ""
			if sub.Metadata != nil {
				userID = sub.Metadata["user_id"]
			}
			if err := db.UpsertSubscription(r.Context(), h.pool, userID, providerName, sub); err != nil {
				slog.Error("webhook: upsert subscription", "provider", providerName, "sub_id", sub.ProviderSubID, "error", err)
				h.metrics.WebhookTotal.WithLabelValues(providerName, eventType, "error").Inc()
				// Still return 200; event is persisted. Will be re-synced by dunning sweep.
				w.WriteHeader(http.StatusOK)
				return
			}
			h.metrics.SubscriptionStatus.WithLabelValues(providerName, string(sub.Status)).Inc()
		}

		elapsed := time.Since(start)
		h.metrics.WebhookDuration.WithLabelValues(providerName, eventType).Observe(elapsed.Seconds())
		h.metrics.WebhookTotal.WithLabelValues(providerName, eventType, "ok").Inc()

		slog.Info("webhook processed",
			"provider", providerName,
			"event_type", eventType,
			"latency_ms", elapsed.Milliseconds(),
		)

		// Update queue depth gauge.
		if h.pool != nil {
			if depth, err := db.QueueDepth(r.Context(), h.pool); err == nil {
				h.metrics.QueueDepth.Set(float64(depth))
			}
		}

		w.WriteHeader(http.StatusOK)
	}
}

func (h *Handler) resolveProvider(name string) provider.Provider {
	if name == "" {
		return h.defaultProvider
	}
	if p, ok := h.providers[name]; ok {
		return p
	}
	return h.defaultProvider
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
