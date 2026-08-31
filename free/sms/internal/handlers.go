package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// e164Regex validates E.164 phone numbers: +<country><number>, 7-15 digits total.
var e164Regex = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)

// ValidateE164 returns an error if number is not valid E.164.
func ValidateE164(number string) error {
	if !e164Regex.MatchString(number) {
		return fmt.Errorf("invalid phone number %q: must be E.164 format (e.g. +14155552671)", number)
	}
	return nil
}

// Server holds plugin-sms handler dependencies.
type Server struct {
	db  *pgxpool.Pool
	cfg *Config
}

// NewServer creates a new Server.
func NewServer(db *pgxpool.Pool, cfg *Config) *Server {
	return &Server{db: db, cfg: cfg}
}

// Routes registers all sms plugin routes on r.
func (s *Server) Routes(r chi.Router) {
	r.Get("/health", s.handleHealth)
	r.Route("/sms", func(r chi.Router) {
		r.Post("/send", s.handleSend)
		r.Get("/messages", s.handleListMessages)
		r.Post("/opt-out", s.handleOptOut)
		r.Delete("/opt-out/{number}", s.handleOptIn)
		r.Get("/opt-outs", s.handleListOptOuts)
	})
}

func sourceAccountID(r *http.Request) string {
	id := r.Header.Get("X-Hasura-Source-Account-Id")
	if id == "" {
		return "primary"
	}
	return id
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		http.Error(w, `{"status":"unhealthy"}`, http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	var req struct {
		To   string `json:"to"`
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.To == "" || req.Body == "" {
		http.Error(w, `{"error":"to and body are required"}`, http.StatusBadRequest)
		return
	}

	// E.164 validation
	if err := ValidateE164(req.To); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	// Check opt-out list
	var optOutCount int
	_ = s.db.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM np_sms_opt_outs WHERE source_account_id = $1 AND phone_number = $2`,
		sai, req.To).Scan(&optOutCount)
	if optOutCount > 0 {
		http.Error(w, `{"error":"recipient has opted out"}`, http.StatusForbidden)
		return
	}

	// Rate limit check: count sends in past minute
	var recentCount int
	_ = s.db.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM np_sms_messages WHERE source_account_id = $1 AND created_at > NOW() - INTERVAL '1 minute'`,
		sai).Scan(&recentCount)
	if recentCount >= s.cfg.RateLimitPerMin {
		http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
		return
	}

	from := s.cfg.TwilioFrom

	// Record message
	var msgID string
	err := s.db.QueryRow(r.Context(),
		`INSERT INTO np_sms_messages (source_account_id, to_number, from_number, body, status)
		 VALUES ($1, $2, $3, $4, 'queued') RETURNING id`,
		sai, req.To, from, req.Body,
	).Scan(&msgID)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}

	// Send via Twilio
	providerSID, sendErr := s.sendViaTwilio(r.Context(), req.To, from, req.Body)
	if sendErr != nil {
		_, _ = s.db.Exec(r.Context(),
			`UPDATE np_sms_messages SET status = 'failed', error_message = $1 WHERE id = $2`,
			sendErr.Error(), msgID)
		writeJSON(w, http.StatusAccepted, map[string]string{"id": msgID, "status": "failed", "error": sendErr.Error()})
		return
	}

	_, _ = s.db.Exec(r.Context(),
		`UPDATE np_sms_messages SET status = 'sent', provider_sid = $1, sent_at = NOW() WHERE id = $2`,
		providerSID, msgID)
	writeJSON(w, http.StatusAccepted, map[string]string{"id": msgID, "status": "sent", "provider_sid": providerSID})
}

// sendViaTwilio posts to the Twilio Messages API.
// Account SID, Auth Token, and From number come from config only — never from request parameters.
func (s *Server) sendViaTwilio(ctx context.Context, to, from, body string) (string, error) {
	if s.cfg.TwilioSID == "" || s.cfg.TwilioToken == "" {
		return "", fmt.Errorf("Twilio credentials not configured")
	}

	// Twilio REST API endpoint — config-only, not user-overridable
	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", s.cfg.TwilioSID)

	form := url.Values{
		"To":   {to},
		"From": {from},
		"Body": {body},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(s.cfg.TwilioSID, s.cfg.TwilioToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		SID          string `json:"sid"`
		Status       string `json:"status"`
		ErrorCode    *int   `json:"error_code"`
		ErrorMessage string `json:"error_message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("unexpected Twilio response")
	}
	if result.ErrorCode != nil {
		return "", fmt.Errorf("twilio error %d: %s", *result.ErrorCode, result.ErrorMessage)
	}
	return result.SID, nil
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	rows, err := s.db.Query(r.Context(),
		`SELECT id, to_number, from_number, body, status, created_at
		 FROM np_sms_messages WHERE source_account_id = $1 ORDER BY created_at DESC LIMIT 100`, sai)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type msgRow struct {
		ID         string `json:"id"`
		ToNumber   string `json:"to"`
		FromNumber string `json:"from"`
		Body       string `json:"body"`
		Status     string `json:"status"`
		CreatedAt  string `json:"created_at"`
	}
	var msgs []msgRow
	for rows.Next() {
		var m msgRow
		var createdAt time.Time
		if err := rows.Scan(&m.ID, &m.ToNumber, &m.FromNumber, &m.Body, &m.Status, &createdAt); err != nil {
			continue
		}
		m.CreatedAt = createdAt.Format(time.RFC3339)
		msgs = append(msgs, m)
	}
	if msgs == nil {
		msgs = []msgRow{}
	}
	writeJSON(w, http.StatusOK, msgs)
}

func (s *Server) handleOptOut(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	var req struct {
		Number string `json:"number"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Number == "" {
		http.Error(w, `{"error":"number required"}`, http.StatusBadRequest)
		return
	}
	if err := ValidateE164(req.Number); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	_, err := s.db.Exec(r.Context(),
		`INSERT INTO np_sms_opt_outs (source_account_id, phone_number)
		 VALUES ($1, $2) ON CONFLICT (source_account_id, phone_number) DO NOTHING`,
		sai, req.Number)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleOptIn(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	number := chi.URLParam(r, "number")
	if err := ValidateE164(number); err != nil {
		http.Error(w, `{"error":"invalid number"}`, http.StatusBadRequest)
		return
	}
	_, err := s.db.Exec(r.Context(),
		`DELETE FROM np_sms_opt_outs WHERE source_account_id = $1 AND phone_number = $2`, sai, number)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListOptOuts(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	rows, err := s.db.Query(r.Context(),
		`SELECT phone_number, opted_out_at FROM np_sms_opt_outs WHERE source_account_id = $1 ORDER BY opted_out_at DESC`, sai)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type row struct {
		Number    string `json:"number"`
		OptedOutAt string `json:"opted_out_at"`
	}
	var list []row
	for rows.Next() {
		var item row
		var t time.Time
		if err := rows.Scan(&item.Number, &t); err != nil {
			continue
		}
		item.OptedOutAt = t.Format(time.RFC3339)
		list = append(list, item)
	}
	if list == nil {
		list = []row{}
	}
	writeJSON(w, http.StatusOK, list)
}
