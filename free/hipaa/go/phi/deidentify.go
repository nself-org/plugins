package phi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// maskPatterns maps phi_category to a masking function.
var maskPatterns = map[string]func(string) string{
	"ssn":     maskSSN,
	"dob":     maskDOB,
	"name":    maskName,
	"mrn":     maskMRN,
	"phone":   maskPhone,
	"email":   maskEmail,
	"address": maskAddress,
	"other":   maskGeneric,
}

// tokenStore is a simple in-process token store for non-Vault environments.
// In production with NSELF_HIPAA_VAULT=true, tokens are stored in Vault Transit.
var tokenStore = &inMemoryTokenStore{m: make(map[string]string)}

type inMemoryTokenStore struct {
	mu sync.RWMutex
	m  map[string]string // token → original
	r  map[string]string // original → token
}

func (ts *inMemoryTokenStore) Tokenize(plain string) string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.r == nil {
		ts.r = make(map[string]string)
	}
	if tok, ok := ts.r[plain]; ok {
		return tok
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	tok := "tok_" + hex.EncodeToString(b)
	ts.m[tok] = plain
	ts.r[plain] = tok
	return tok
}

func (ts *inMemoryTokenStore) Detokenize(tok string) (string, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	v, ok := ts.m[tok]
	return v, ok
}

// DeidentifyRequest is the body for POST /hipaa/deidentify.
type DeidentifyRequest struct {
	TableName string            `json:"table_name"`
	Rows      []map[string]any  `json:"rows"`
}

// DeidentifyHandler masks or tokenizes all PHI fields in a dataset.
func DeidentifyHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sourceAccount(r)
		var req DeidentifyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.TableName == "" || len(req.Rows) == 0 {
			writeError(w, http.StatusBadRequest, "table_name and rows are required")
			return
		}

		// Look up registered PHI columns for the table.
		cols, err := LookupPHIColumns(r.Context(), pool, acc, req.TableName)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Build a column→method map.
		colMap := make(map[string]struct {
			Category string
			Method   string
		})
		for _, c := range cols {
			colMap[c.ColumnName] = struct {
				Category string
				Method   string
			}{c.PHICategory, c.DeIDMethod}
		}

		// Process each row.
		out := make([]map[string]any, 0, len(req.Rows))
		for _, row := range req.Rows {
			deID := make(map[string]any, len(row))
			for k, v := range row {
				info, isPHI := colMap[k]
				if !isPHI {
					deID[k] = v
					continue
				}
				str := fmt.Sprintf("%v", v)
				switch info.Method {
				case "redact":
					deID[k] = "[REDACTED]"
				case "tokenize":
					deID[k] = tokenStore.Tokenize(str)
				default: // mask
					if fn, ok := maskPatterns[info.Category]; ok {
						deID[k] = fn(str)
					} else {
						deID[k] = maskGeneric(str)
					}
				}
			}
			out = append(out, deID)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"table_name":    req.TableName,
			"rows_processed": len(out),
			"rows":          out,
		})
	}
}

// TokenizeHandler tokenizes a single PHI value.
func TokenizeHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = pool // future: check license gate via pool
		var req struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Value == "" {
			writeError(w, http.StatusBadRequest, "value is required")
			return
		}
		tok := tokenStore.Tokenize(req.Value)
		writeJSON(w, http.StatusOK, map[string]string{"token": tok})
	}
}

// DetokenizeHandler reverses a token to its original PHI value.
func DetokenizeHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !hasRole(r, "phi:detokenize", "phi:admin", "hipaa:admin") {
			writeError(w, http.StatusForbidden, "phi:detokenize role required")
			return
		}
		tok := chi.URLParam(r, "token")
		plain, ok := tokenStore.Detokenize(tok)
		if !ok {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"token": tok, "value": plain})
	}
}

// -------------------------------------------------------------------------
// Masking functions per PHI category
// -------------------------------------------------------------------------

var (
	rSSN   = regexp.MustCompile(`\d`)
	rPhone = regexp.MustCompile(`\d`)
)

func maskSSN(v string) string {
	// Keep last 4 digits: XXX-XX-1234
	if len(v) >= 4 {
		return "XXX-XX-" + v[len(v)-4:]
	}
	return "XXX-XX-XXXX"
}

func maskDOB(v string) string {
	// Generalise to year only: e.g. "1985-03-14" → "1985-XX-XX"
	parts := strings.SplitN(v, "-", 3)
	if len(parts) == 3 {
		return parts[0] + "-XX-XX"
	}
	return "XXXX-XX-XX"
}

func maskName(v string) string {
	parts := strings.Fields(v)
	if len(parts) == 0 {
		return "[NAME]"
	}
	out := make([]string, len(parts))
	for i, p := range parts {
		if len(p) == 0 {
			continue
		}
		out[i] = string(p[0]) + strings.Repeat("*", len(p)-1)
	}
	return strings.Join(out, " ")
}

func maskMRN(v string) string {
	if len(v) > 4 {
		return strings.Repeat("X", len(v)-4) + v[len(v)-4:]
	}
	return strings.Repeat("X", len(v))
}

func maskPhone(v string) string {
	digits := rPhone.FindAllString(v, -1)
	if len(digits) >= 4 {
		return "XXX-XXX-" + strings.Join(digits[len(digits)-4:], "")
	}
	return "XXX-XXX-XXXX"
}

func maskEmail(v string) string {
	at := strings.Index(v, "@")
	if at <= 0 {
		return "[EMAIL]"
	}
	local := v[:at]
	domain := v[at:]
	if len(local) <= 2 {
		return string(local[0]) + "*" + domain
	}
	return string(local[0]) + strings.Repeat("*", len(local)-2) + string(local[len(local)-1]) + domain
}

func maskAddress(v string) string {
	// Keep only the first word (house number) and mask the rest.
	parts := strings.Fields(v)
	if len(parts) == 0 {
		return "[ADDRESS]"
	}
	return parts[0] + " [STREET MASKED]"
}

func maskGeneric(v string) string {
	if len(v) == 0 {
		return "[PHI]"
	}
	if len(v) <= 4 {
		return strings.Repeat("*", len(v))
	}
	return string(v[0]) + strings.Repeat("*", len(v)-2) + string(v[len(v)-1])
}

// -------------------------------------------------------------------------
// Package-level shared helpers (used by audit.go + registry.go)
// -------------------------------------------------------------------------

func sourceAccount(r *http.Request) string {
	v := r.Header.Get("X-Source-Account-ID")
	if v == "" {
		v = r.URL.Query().Get("source_account_id")
	}
	if v == "" {
		v = "primary"
	}
	return v
}

func hasRole(r *http.Request, roles ...string) bool {
	hdr := r.Header.Get("X-HIPAA-Role")
	for _, role := range roles {
		if hdr == role {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
