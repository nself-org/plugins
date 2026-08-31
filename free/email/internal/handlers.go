package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server holds plugin-email handler dependencies.
type Server struct {
	db  *pgxpool.Pool
	cfg *Config
}

// NewServer creates a new Server.
func NewServer(db *pgxpool.Pool, cfg *Config) *Server {
	return &Server{db: db, cfg: cfg}
}

// Routes registers all email plugin routes on r.
func (s *Server) Routes(r chi.Router) {
	r.Get("/health", s.handleHealth)
	r.Route("/email", func(r chi.Router) {
		r.Post("/send", s.handleSend)
		r.Get("/messages", s.handleListMessages)
		r.Post("/templates", s.handleCreateTemplate)
		r.Get("/templates", s.handleListTemplates)
		r.Delete("/templates/{name}", s.handleDeleteTemplate)
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

type sendRequest struct {
	To       string `json:"to"`
	Subject  string `json:"subject"`
	BodyHTML string `json:"body_html"`
	BodyText string `json:"body_text"`
	Template string `json:"template"` // optional template name
}

type messageRow struct {
	ID          string  `json:"id"`
	ToAddress   string  `json:"to"`
	FromAddress string  `json:"from"`
	Subject     string  `json:"subject"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"created_at"`
}

type templateRow struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Subject   string `json:"subject"`
	BodyHTML  string `json:"body_html"`
	CreatedAt string `json:"created_at"`
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	var req sendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.To == "" {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	subject := req.Subject
	bodyHTML := req.BodyHTML
	bodyText := req.BodyText

	// Load template if specified
	if req.Template != "" {
		var tSubject, tHTML string
		err := s.db.QueryRow(r.Context(),
			`SELECT subject, body_html FROM np_email_templates WHERE source_account_id = $1 AND name = $2`,
			sai, req.Template).Scan(&tSubject, &tHTML)
		if err != nil {
			http.Error(w, `{"error":"template not found"}`, http.StatusNotFound)
			return
		}
		if subject == "" {
			subject = tSubject
		}
		if bodyHTML == "" {
			bodyHTML = tHTML
		}
	}

	from := s.cfg.ElasticFrom
	if from == "" {
		from = "noreply@nself.org"
	}

	// Record message
	var msgID string
	err := s.db.QueryRow(r.Context(),
		`INSERT INTO np_email_messages (source_account_id, to_address, from_address, subject, status)
		 VALUES ($1, $2, $3, $4, 'queued') RETURNING id`,
		sai, req.To, from, subject,
	).Scan(&msgID)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}

	// Dispatch via Elastic Email (best-effort; record result)
	providerID, sendErr := s.sendViaElastic(r.Context(), req.To, from, subject, bodyHTML, bodyText)
	if sendErr != nil {
		_, _ = s.db.Exec(r.Context(),
			`UPDATE np_email_messages SET status = 'failed', error_message = $1 WHERE id = $2`,
			sendErr.Error(), msgID)
		writeJSON(w, http.StatusAccepted, map[string]string{"id": msgID, "status": "failed", "error": sendErr.Error()})
		return
	}

	_, _ = s.db.Exec(r.Context(),
		`UPDATE np_email_messages SET status = 'sent', provider_message_id = $1, sent_at = NOW() WHERE id = $2`,
		providerID, msgID)
	writeJSON(w, http.StatusAccepted, map[string]string{"id": msgID, "status": "sent", "provider_id": providerID})
}

// sendViaElastic posts to the Elastic Email v2 API.
// API key comes from config only — never from request parameters.
func (s *Server) sendViaElastic(ctx context.Context, to, from, subject, bodyHTML, bodyText string) (string, error) {
	if s.cfg.ElasticAPIKey == "" {
		return "", fmt.Errorf("ELASTIC_EMAIL_API_KEY not configured")
	}
	form := url.Values{
		"apikey":  {s.cfg.ElasticAPIKey},
		"to":      {to},
		"from":    {from},
		"subject": {subject},
	}
	if bodyHTML != "" {
		form.Set("bodyHtml", bodyHTML)
	}
	if bodyText != "" {
		form.Set("bodyText", bodyText)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.elasticemail.com/v2/email/send",
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
		Data    struct {
			MessageID string `json:"messageid"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("unexpected response from Elastic Email: %s", bytes.TrimSpace(body))
	}
	if !result.Success {
		return "", fmt.Errorf("elastic email error: %s", result.Error)
	}
	return result.Data.MessageID, nil
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	rows, err := s.db.Query(r.Context(),
		`SELECT id, to_address, from_address, subject, status, created_at
		 FROM np_email_messages WHERE source_account_id = $1 ORDER BY created_at DESC LIMIT 100`, sai)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var msgs []messageRow
	for rows.Next() {
		var m messageRow
		var createdAt time.Time
		if err := rows.Scan(&m.ID, &m.ToAddress, &m.FromAddress, &m.Subject, &m.Status, &createdAt); err != nil {
			continue
		}
		m.CreatedAt = createdAt.Format(time.RFC3339)
		msgs = append(msgs, m)
	}
	if msgs == nil {
		msgs = []messageRow{}
	}
	writeJSON(w, http.StatusOK, msgs)
}

func (s *Server) handleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	var body struct {
		Name     string `json:"name"`
		Subject  string `json:"subject"`
		BodyHTML string `json:"body_html"`
		BodyText string `json:"body_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	var id string
	err := s.db.QueryRow(r.Context(),
		`INSERT INTO np_email_templates (source_account_id, name, subject, body_html, body_text)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (source_account_id, name) DO UPDATE SET subject=EXCLUDED.subject, body_html=EXCLUDED.body_html, updated_at=NOW()
		 RETURNING id`,
		sai, body.Name, body.Subject, body.BodyHTML, body.BodyText,
	).Scan(&id)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	rows, err := s.db.Query(r.Context(),
		`SELECT id, name, subject, body_html, created_at FROM np_email_templates WHERE source_account_id = $1 ORDER BY name`, sai)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var templates []templateRow
	for rows.Next() {
		var t templateRow
		var createdAt time.Time
		if err := rows.Scan(&t.ID, &t.Name, &t.Subject, &t.BodyHTML, &createdAt); err != nil {
			continue
		}
		t.CreatedAt = createdAt.Format(time.RFC3339)
		templates = append(templates, t)
	}
	if templates == nil {
		templates = []templateRow{}
	}
	writeJSON(w, http.StatusOK, templates)
}

func (s *Server) handleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	name := chi.URLParam(r, "name")
	_, err := s.db.Exec(r.Context(),
		`DELETE FROM np_email_templates WHERE source_account_id = $1 AND name = $2`, sai, name)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
