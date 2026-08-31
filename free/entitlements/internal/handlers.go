package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// Handlers holds the database pool and handles all HTTP requests.
type Handlers struct {
	pool PgxIface
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(pool PgxIface) *Handlers {
	return &Handlers{pool: pool}
}

// sa returns the source account ID from the request header, defaulting to "primary".
func (h *Handlers) sa(r *http.Request) string {
	if sa := r.Header.Get("X-Source-Account-ID"); sa != "" {
		return sa
	}
	return "primary"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ============================================================
// Health / Status
// ============================================================

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "plugin": "entitlements"})
}

func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := h.pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"ready": "false", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ready": "true", "plugin": "entitlements"})
}

func (h *Handlers) PluginStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"plugin": "entitlements", "version": "1.0.0", "status": "running"})
}

// ============================================================
// Plans
// ============================================================

func (h *Handlers) ListPlans(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, name, slug, description, billing_interval, price_cents, currency,
		        trial_days, plan_type, is_public, is_archived, features, quotas, metadata, display_order,
		        created_at, updated_at
		 FROM entitlement_plans WHERE source_account_id=$1 AND is_archived=false ORDER BY display_order, created_at`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var plans []Plan
	for rows.Next() {
		var p Plan
		if err := rows.Scan(
			&p.ID, &p.SourceAccountID, &p.Name, &p.Slug, &p.Description,
			&p.BillingInterval, &p.PriceCents, &p.Currency, &p.TrialDays,
			&p.PlanType, &p.IsPublic, &p.IsArchived, &p.Features, &p.Quotas,
			&p.Metadata, &p.DisplayOrder, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		plans = append(plans, p)
	}
	if plans == nil {
		plans = []Plan{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": plans})
}

func (h *Handlers) CreatePlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	name, _ := body["name"].(string)
	slug, _ := body["slug"].(string)
	billingInterval, _ := body["billing_interval"].(string)
	planType, _ := body["plan_type"].(string)
	if name == "" || slug == "" || billingInterval == "" || planType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, slug, billing_interval, and plan_type are required"})
		return
	}
	priceCents := 0
	if v, ok := body["price_cents"].(float64); ok {
		priceCents = int(v)
	}
	currency := "USD"
	if v, ok := body["currency"].(string); ok && v != "" {
		currency = v
	}
	trialDays := 0
	if v, ok := body["trial_days"].(float64); ok {
		trialDays = int(v)
	}
	isPublic := true
	if v, ok := body["is_public"].(bool); ok {
		isPublic = v
	}
	features := body["features"]
	if features == nil {
		features = map[string]any{}
	}
	quotas := body["quotas"]
	if quotas == nil {
		quotas = map[string]any{}
	}
	displayOrder := 0
	if v, ok := body["display_order"].(float64); ok {
		displayOrder = int(v)
	}
	featuresJSON, _ := json.Marshal(features)
	quotasJSON, _ := json.Marshal(quotas)
	var id string
	err := h.pool.QueryRow(ctx,
		`INSERT INTO entitlement_plans
		 (source_account_id, name, slug, description, billing_interval, price_cents, currency,
		  trial_days, plan_type, is_public, is_archived, features, quotas, display_order)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,false,$11,$12,$13)
		 RETURNING id`,
		sa, name, slug, body["description"], billingInterval, priceCents, currency,
		trialDays, planType, isPublic, featuresJSON, quotasJSON, displayOrder,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"success": true, "id": id})
}

func (h *Handlers) GetPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var p Plan
	err := h.pool.QueryRow(ctx,
		`SELECT id, source_account_id, name, slug, description, billing_interval, price_cents, currency,
		        trial_days, plan_type, is_public, is_archived, features, quotas, metadata, display_order,
		        created_at, updated_at
		 FROM entitlement_plans WHERE id=$1 AND source_account_id=$2`, id, sa,
	).Scan(
		&p.ID, &p.SourceAccountID, &p.Name, &p.Slug, &p.Description,
		&p.BillingInterval, &p.PriceCents, &p.Currency, &p.TrialDays,
		&p.PlanType, &p.IsPublic, &p.IsArchived, &p.Features, &p.Quotas,
		&p.Metadata, &p.DisplayOrder, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "plan not found"})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handlers) UpdatePlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	_, err := h.pool.Exec(ctx,
		`UPDATE entitlement_plans SET
		   name = COALESCE($3, name),
		   description = COALESCE($4, description),
		   price_cents = COALESCE($5, price_cents),
		   trial_days = COALESCE($6, trial_days),
		   is_public = COALESCE($7, is_public),
		   updated_at = NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa,
		nullableString(body, "name"),
		nullableString(body, "description"),
		nullableInt(body, "price_cents"),
		nullableInt(body, "trial_days"),
		nullableBool(body, "is_public"),
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("update: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *Handlers) DeletePlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	_, err := h.pool.Exec(ctx,
		`UPDATE entitlement_plans SET is_archived=true, updated_at=NOW() WHERE id=$1 AND source_account_id=$2`,
		id, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("archive: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// ============================================================
// Subscriptions
// ============================================================

func (h *Handlers) ListSubscriptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, workspace_id, user_id, plan_id, status, billing_interval,
		        price_cents, currency, trial_start, trial_end, current_period_start, current_period_end,
		        cancel_at_period_end, canceled_at, metadata, created_at, updated_at
		 FROM entitlement_subscriptions WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var subs []Subscription
	for rows.Next() {
		var s Subscription
		if err := rows.Scan(
			&s.ID, &s.SourceAccountID, &s.WorkspaceID, &s.UserID, &s.PlanID, &s.Status,
			&s.BillingInterval, &s.PriceCents, &s.Currency, &s.TrialStart, &s.TrialEnd,
			&s.PeriodStart, &s.PeriodEnd, &s.CancelAtEnd, &s.CanceledAt, &s.Metadata,
			&s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		subs = append(subs, s)
	}
	if subs == nil {
		subs = []Subscription{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": subs})
}

func (h *Handlers) CreateSubscription(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	planID, _ := body["plan_id"].(string)
	if planID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plan_id is required"})
		return
	}
	userID, _ := body["user_id"].(string)
	workspaceID, _ := body["workspace_id"].(string)
	if userID == "" && workspaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id or workspace_id is required"})
		return
	}
	billingInterval := "month"
	if v, ok := body["billing_interval"].(string); ok && v != "" {
		billingInterval = v
	}
	now := time.Now().UTC()
	var periodEnd time.Time
	if billingInterval == "year" {
		periodEnd = now.AddDate(1, 0, 0)
	} else {
		periodEnd = now.AddDate(0, 1, 0)
	}
	var id string
	err := h.pool.QueryRow(ctx,
		`INSERT INTO entitlement_subscriptions
		 (source_account_id, workspace_id, user_id, plan_id, status, billing_interval,
		  price_cents, currency, current_period_start, current_period_end)
		 VALUES ($1,$2,$3,$4,'active',$5,0,'USD',$6,$7)
		 RETURNING id`,
		sa, nullStr(workspaceID), nullStr(userID), planID, billingInterval, now, periodEnd,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"success": true, "id": id})
}

func (h *Handlers) GetSubscription(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var s Subscription
	err := h.pool.QueryRow(ctx,
		`SELECT id, source_account_id, workspace_id, user_id, plan_id, status, billing_interval,
		        price_cents, currency, trial_start, trial_end, current_period_start, current_period_end,
		        cancel_at_period_end, canceled_at, metadata, created_at, updated_at
		 FROM entitlement_subscriptions WHERE id=$1 AND source_account_id=$2`, id, sa,
	).Scan(
		&s.ID, &s.SourceAccountID, &s.WorkspaceID, &s.UserID, &s.PlanID, &s.Status,
		&s.BillingInterval, &s.PriceCents, &s.Currency, &s.TrialStart, &s.TrialEnd,
		&s.PeriodStart, &s.PeriodEnd, &s.CancelAtEnd, &s.CanceledAt, &s.Metadata,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "subscription not found"})
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *Handlers) UpdateSubscription(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	_, err := h.pool.Exec(ctx,
		`UPDATE entitlement_subscriptions SET updated_at=NOW() WHERE id=$1 AND source_account_id=$2`,
		id, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("update: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *Handlers) CancelSubscription(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var body map[string]any
	json.NewDecoder(r.Body).Decode(&body)
	now := time.Now().UTC()
	_, err := h.pool.Exec(ctx,
		`UPDATE entitlement_subscriptions SET status='canceled', canceled_at=$3, updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa, now)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("cancel: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// ============================================================
// Features
// ============================================================

func (h *Handlers) ListFeatures(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, key, name, description, feature_type, is_active, category,
		        metadata, created_at, updated_at
		 FROM entitlement_features WHERE source_account_id=$1 ORDER BY name`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var features []Feature
	for rows.Next() {
		var f Feature
		if err := rows.Scan(
			&f.ID, &f.SourceAccountID, &f.Key, &f.Name, &f.Description,
			&f.FeatureType, &f.IsActive, &f.Category, &f.Metadata, &f.CreatedAt, &f.UpdatedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		features = append(features, f)
	}
	if features == nil {
		features = []Feature{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": features})
}

func (h *Handlers) CreateFeature(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	key, _ := body["key"].(string)
	name, _ := body["name"].(string)
	featureType, _ := body["feature_type"].(string)
	if key == "" || name == "" || featureType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key, name, and feature_type are required"})
		return
	}
	var id string
	err := h.pool.QueryRow(ctx,
		`INSERT INTO entitlement_features (source_account_id, key, name, description, feature_type, is_active)
		 VALUES ($1,$2,$3,$4,$5,true) RETURNING id`,
		sa, key, name, body["description"], featureType,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"success": true, "id": id})
}

func (h *Handlers) UpdateFeature(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	_, err := h.pool.Exec(ctx,
		`UPDATE entitlement_features SET
		   name = COALESCE($3, name),
		   description = COALESCE($4, description),
		   updated_at = NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa,
		nullableString(body, "name"),
		nullableString(body, "description"),
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("update: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *Handlers) DeleteFeature(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	_, err := h.pool.Exec(ctx,
		`DELETE FROM entitlement_features WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// ============================================================
// Quotas
// ============================================================

func (h *Handlers) ListQuotas(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, plan_id, feature_id, limit_value, period, created_at
		 FROM entitlement_quotas WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var quotas []Quota
	for rows.Next() {
		var q Quota
		if err := rows.Scan(&q.ID, &q.SourceAccountID, &q.PlanID, &q.FeatureID, &q.LimitValue, &q.Period, &q.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		quotas = append(quotas, q)
	}
	if quotas == nil {
		quotas = []Quota{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": quotas})
}

func (h *Handlers) CreateQuota(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	planID, _ := body["plan_id"].(string)
	featureID, _ := body["feature_id"].(string)
	if planID == "" || featureID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plan_id and feature_id are required"})
		return
	}
	period := "monthly"
	if v, ok := body["period"].(string); ok && v != "" {
		period = v
	}
	var limitValue *int
	if v, ok := body["limit_value"].(float64); ok {
		iv := int(v)
		limitValue = &iv
	}
	var id string
	err := h.pool.QueryRow(ctx,
		`INSERT INTO entitlement_quotas (source_account_id, plan_id, feature_id, limit_value, period)
		 VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		sa, planID, featureID, limitValue, period,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"success": true, "id": id})
}

func (h *Handlers) UpdateQuota(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	_, err := h.pool.Exec(ctx,
		`UPDATE entitlement_quotas SET
		   limit_value = COALESCE($3, limit_value),
		   period = COALESCE($4, period)
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa,
		nullableInt(body, "limit_value"),
		nullableString(body, "period"),
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("update: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *Handlers) DeleteQuota(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	_, err := h.pool.Exec(ctx,
		`DELETE FROM entitlement_quotas WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// ============================================================
// Usage
// ============================================================

func (h *Handlers) TrackUsage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	userID, _ := body["user_id"].(string)
	featureKey, _ := body["feature_key"].(string)
	if featureKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "feature_key is required"})
		return
	}
	amount := 1
	if v, ok := body["amount"].(float64); ok {
		amount = int(v)
	}
	var id string
	err := h.pool.QueryRow(ctx,
		`INSERT INTO entitlement_usage (source_account_id, user_id, quota_key, usage_amount)
		 VALUES ($1,$2,$3,$4) RETURNING id`,
		sa, nullStr(userID), featureKey, amount,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"success": true, "id": id})
}

func (h *Handlers) GetUsage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	q := r.URL.Query()
	userID := q.Get("user_id")
	featureKey := q.Get("feature_key")
	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, user_id, quota_key, usage_amount, created_at
		 FROM entitlement_usage
		 WHERE source_account_id=$1
		   AND ($2='' OR user_id=$2)
		   AND ($3='' OR quota_key=$3)
		 ORDER BY created_at DESC LIMIT 100`,
		sa, userID, featureKey)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var usages []Usage
	for rows.Next() {
		var u Usage
		if err := rows.Scan(&u.ID, &u.SourceAccountID, &u.UserID, &u.QuotaKey, &u.UsageAmount, &u.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		usages = append(usages, u)
	}
	if usages == nil {
		usages = []Usage{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": usages})
}

// CheckEntitlement checks whether a user has access to a feature and how much quota remains.
func (h *Handlers) CheckEntitlement(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	userID, _ := body["user_id"].(string)
	featureKey, _ := body["feature_key"].(string)
	if featureKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "feature_key is required"})
		return
	}

	// Check active subscription
	var subCount int
	h.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM entitlement_subscriptions s
		 JOIN entitlement_plans p ON p.id=s.plan_id
		 WHERE s.source_account_id=$1 AND ($2='' OR s.user_id=$2)
		   AND s.status='active' AND p.features ? $3`,
		sa, userID, featureKey,
	).Scan(&subCount)

	allowed := subCount > 0

	// Calculate remaining quota
	var used int
	h.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(usage_amount),0) FROM entitlement_usage
		 WHERE source_account_id=$1 AND ($2='' OR user_id=$2) AND quota_key=$3`,
		sa, userID, featureKey,
	).Scan(&used)

	var limitValue *int
	h.pool.QueryRow(ctx,
		`SELECT q.limit_value FROM entitlement_quotas q
		 JOIN entitlement_subscriptions s ON s.plan_id=q.plan_id
		 JOIN entitlement_features f ON f.id=q.feature_id
		 WHERE s.source_account_id=$1 AND ($2='' OR s.user_id=$2)
		   AND f.key=$3 AND s.status='active'
		 LIMIT 1`,
		sa, userID, featureKey,
	).Scan(&limitValue)

	remaining := -1 // -1 means unlimited
	if limitValue != nil {
		remaining = *limitValue - used
		if remaining < 0 {
			remaining = 0
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"allowed":   allowed,
		"remaining": remaining,
	})
}

// ============================================================
// Addons
// ============================================================

func (h *Handlers) ListAddons(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, subscription_id, addon_plan_id, quantity, price_cents, currency,
		        status, created_at, updated_at
		 FROM entitlement_addons WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var addons []Addon
	for rows.Next() {
		var a Addon
		if err := rows.Scan(
			&a.ID, &a.SourceAccountID, &a.SubscriptionID, &a.AddonPlanID,
			&a.Quantity, &a.PriceCents, &a.Currency, &a.Status, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		addons = append(addons, a)
	}
	if addons == nil {
		addons = []Addon{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": addons})
}

func (h *Handlers) CreateAddon(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	subscriptionID, _ := body["subscription_id"].(string)
	addonPlanID, _ := body["addon_plan_id"].(string)
	if subscriptionID == "" || addonPlanID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "subscription_id and addon_plan_id are required"})
		return
	}
	quantity := 1
	if v, ok := body["quantity"].(float64); ok {
		quantity = int(v)
	}
	var id string
	err := h.pool.QueryRow(ctx,
		`INSERT INTO entitlement_addons (source_account_id, subscription_id, addon_plan_id, quantity, price_cents, currency, status)
		 VALUES ($1,$2,$3,$4,0,'USD','active') RETURNING id`,
		sa, subscriptionID, addonPlanID, quantity,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"success": true, "id": id})
}

// ============================================================
// Helpers
// ============================================================

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableString(body map[string]any, key string) any {
	v, ok := body[key].(string)
	if !ok || v == "" {
		return nil
	}
	return v
}

func nullableInt(body map[string]any, key string) any {
	v, ok := body[key].(float64)
	if !ok {
		return nil
	}
	return int(v)
}

func nullableBool(body map[string]any, key string) any {
	v, ok := body[key].(bool)
	if !ok {
		return nil
	}
	return v
}
