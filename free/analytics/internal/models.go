package internal

import "time"

// ─── Events ──────────────────────────────────────────────────────────────────

// Event represents a row in analytics_events.
type Event struct {
	ID              string          `json:"id"`
	SourceAccountID string          `json:"source_account_id"`
	EventName       string          `json:"event_name"`
	EventCategory   *string         `json:"event_category"`
	UserID          *string         `json:"user_id"`
	SessionID       *string         `json:"session_id"`
	Properties      JSONMap         `json:"properties"`
	Context         JSONMap         `json:"context"`
	SourcePlugin    *string         `json:"source_plugin"`
	Timestamp       time.Time       `json:"timestamp"`
	CreatedAt       time.Time       `json:"created_at"`
}

// TrackEventRequest is the body for POST /api/v1/events.
type TrackEventRequest struct {
	EventName     string          `json:"event_name"`
	EventCategory string          `json:"event_category,omitempty"`
	UserID        string          `json:"user_id,omitempty"`
	SessionID     string          `json:"session_id,omitempty"`
	Properties    JSONMap         `json:"properties,omitempty"`
	Context       JSONMap         `json:"context,omitempty"`
	SourcePlugin  string          `json:"source_plugin,omitempty"`
	Timestamp     string          `json:"timestamp,omitempty"`
}

// TrackEventBatchRequest is the body for POST /api/v1/events/batch.
type TrackEventBatchRequest struct {
	Events []TrackEventRequest `json:"events"`
}

// ─── Counters ─────────────────────────────────────────────────────────────────

// Counter represents an aggregated row in analytics_counters.
type Counter struct {
	CounterName string    `json:"counter_name"`
	Dimension   string    `json:"dimension"`
	Period      string    `json:"period"`
	PeriodStart time.Time `json:"period_start"`
	Value       int64     `json:"value"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// IncrementCounterRequest is the body for POST /api/v1/counters/increment.
type IncrementCounterRequest struct {
	CounterName string  `json:"counter_name"`
	Dimension   string  `json:"dimension,omitempty"`
	Amount      int64   `json:"amount,omitempty"`
}

// ─── Funnels ──────────────────────────────────────────────────────────────────

// FunnelStep is a single step definition inside a funnel's steps JSON.
type FunnelStep struct {
	Name      string  `json:"name"`
	EventName string  `json:"event_name"`
}

// Funnel represents a row in analytics_funnels.
type Funnel struct {
	ID              string       `json:"id"`
	SourceAccountID string       `json:"source_account_id"`
	Name            string       `json:"name"`
	Description     *string      `json:"description"`
	Steps           []FunnelStep `json:"steps"`
	WindowHours     int          `json:"window_hours"`
	Enabled         bool         `json:"enabled"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

// CreateFunnelRequest is the body for POST /api/v1/funnels.
type CreateFunnelRequest struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Steps       []FunnelStep `json:"steps"`
	WindowHours int          `json:"window_hours,omitempty"`
	Enabled     *bool        `json:"enabled,omitempty"`
}

// FunnelStepResult is a step result inside FunnelAnalysis.
type FunnelStepResult struct {
	StepNumber     int     `json:"step_number"`
	StepName       string  `json:"step_name"`
	EventName      string  `json:"event_name"`
	Users          int64   `json:"users"`
	ConversionRate float64 `json:"conversion_rate"`
	DropOffRate    float64 `json:"drop_off_rate"`
}

// FunnelAnalysis is the response for GET /api/v1/funnels/{id}/analyze.
type FunnelAnalysis struct {
	FunnelID               string             `json:"funnel_id"`
	FunnelName             string             `json:"funnel_name"`
	Steps                  []FunnelStepResult `json:"steps"`
	TotalEntered           int64              `json:"total_entered"`
	TotalCompleted         int64              `json:"total_completed"`
	OverallConversionRate  float64            `json:"overall_conversion_rate"`
	AnalysisTimestamp      time.Time          `json:"analysis_timestamp"`
}

// ─── Quotas ───────────────────────────────────────────────────────────────────

// Quota represents a row in analytics_quotas.
type Quota struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	Name            string    `json:"name"`
	Scope           string    `json:"scope"`
	ScopeID         *string   `json:"scope_id"`
	CounterName     string    `json:"counter_name"`
	MaxValue        int64     `json:"max_value"`
	Period          string    `json:"period"`
	ActionOnExceed  string    `json:"action_on_exceed"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// CreateQuotaRequest is the body for POST /api/v1/quotas.
type CreateQuotaRequest struct {
	Name           string  `json:"name"`
	Scope          string  `json:"scope,omitempty"`
	ScopeID        string  `json:"scope_id,omitempty"`
	CounterName    string  `json:"counter_name"`
	MaxValue       int64   `json:"max_value"`
	Period         string  `json:"period"`
	ActionOnExceed string  `json:"action_on_exceed,omitempty"`
	Enabled        *bool   `json:"enabled,omitempty"`
}

// QuotaViolation represents a row in analytics_quota_violations.
type QuotaViolation struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	QuotaID         string    `json:"quota_id"`
	ScopeID         *string   `json:"scope_id"`
	CurrentValue    int64     `json:"current_value"`
	MaxValue        int64     `json:"max_value"`
	ActionTaken     string    `json:"action_taken"`
	Notified        bool      `json:"notified"`
	CreatedAt       time.Time `json:"created_at"`
}

// ─── Stats ────────────────────────────────────────────────────────────────────

// Stats is returned by GET /api/v1/stats.
type Stats struct {
	Events      int64      `json:"events"`
	UniqueUsers int64      `json:"unique_users"`
	TopEvents   []TopEvent `json:"top_events"`
}

// TopEvent is a single entry in Stats.TopEvents.
type TopEvent struct {
	EventName string `json:"event_name"`
	Count     int64  `json:"count"`
}

// TimeseriesPoint is one data point in GET /api/v1/stats/timeseries.
type TimeseriesPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Count     int64     `json:"count"`
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// JSONMap is a generic JSON object type for JSONB columns.
type JSONMap map[string]interface{}
