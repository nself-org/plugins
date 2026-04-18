package sdk

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

const maxBodyBytes = 1 << 20 // 1 MB

// reUUID matches a canonical UUID v4 string (case-insensitive).
var reUUID = regexp.MustCompile(
	`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`,
)

// reEmail is a pragmatic email pattern: local@domain.tld.
// It does not cover every RFC 5321 edge case intentionally — overly strict
// patterns reject valid addresses and overly loose ones pass garbage. This
// rejects obvious junk while remaining practical for form inputs.
var reEmail = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]{2,}$`)

// ValidationError carries a field name and a human-readable message.
// It encodes to {"error":"validation_error","field":"...","message":"..."}.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("validation error on field %q: %s", e.Field, e.Message)
	}
	return fmt.Sprintf("validation error: %s", e.Message)
}

// WriteValidationError writes a 422 JSON response with the ValidationError shape.
func WriteValidationError(w http.ResponseWriter, field, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	json.NewEncoder(w).Encode(map[string]string{
		"error":   "validation_error",
		"field":   field,
		"message": msg,
	})
}

// ValidateJSON reads the request body (limited to 1 MB), decodes it as JSON
// into dst, and returns a typed error on failure.
//
// On success the body has been fully consumed. On failure the body may be
// partially read; callers should not attempt to re-read it.
func ValidateJSON(r *http.Request, dst any) error {
	limited := io.LimitReader(r.Body, maxBodyBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return &ValidationError{Message: "failed to read request body"}
	}
	if int64(len(data)) > maxBodyBytes {
		return &ValidationError{Message: "request body exceeds 1 MB limit"}
	}
	if err := json.Unmarshal(data, dst); err != nil {
		var syntaxErr *json.SyntaxError
		var unmarshalErr *json.UnmarshalTypeError
		switch {
		case errors.As(err, &syntaxErr):
			return &ValidationError{
				Message: fmt.Sprintf("invalid JSON at offset %d", syntaxErr.Offset),
			}
		case errors.As(err, &unmarshalErr):
			return &ValidationError{
				Field:   unmarshalErr.Field,
				Message: fmt.Sprintf("expected %s, got %s", unmarshalErr.Type, unmarshalErr.Value),
			}
		default:
			return &ValidationError{Message: "invalid JSON body"}
		}
	}
	return nil
}

// Required returns a validator that checks each named key is present and
// non-empty (non-zero) in the decoded map.
func Required(fields ...string) func(map[string]any) error {
	return func(m map[string]any) error {
		for _, f := range fields {
			v, ok := m[f]
			if !ok {
				return &ValidationError{Field: f, Message: "field is required"}
			}
			switch val := v.(type) {
			case string:
				if strings.TrimSpace(val) == "" {
					return &ValidationError{Field: f, Message: "field must not be empty"}
				}
			case nil:
				return &ValidationError{Field: f, Message: "field must not be null"}
			}
		}
		return nil
	}
}

// MaxLength returns a validator that checks the string value of field does not
// exceed max characters. Missing or non-string fields are skipped.
func MaxLength(field string, max int) func(map[string]any) error {
	return func(m map[string]any) error {
		v, ok := m[field]
		if !ok {
			return nil
		}
		s, ok := v.(string)
		if !ok {
			return nil
		}
		if len([]rune(s)) > max {
			return &ValidationError{
				Field:   field,
				Message: fmt.Sprintf("must be at most %d characters", max),
			}
		}
		return nil
	}
}

// IsUUID returns a validator that checks the string value of field is a
// canonical UUID (8-4-4-4-12 hex digits). Missing fields are skipped.
func IsUUID(field string) func(map[string]any) error {
	return func(m map[string]any) error {
		v, ok := m[field]
		if !ok {
			return nil
		}
		s, ok := v.(string)
		if !ok {
			return &ValidationError{Field: field, Message: "must be a string UUID"}
		}
		if !reUUID.MatchString(s) {
			return &ValidationError{Field: field, Message: "must be a valid UUID"}
		}
		return nil
	}
}

// IsEmail returns a validator that checks the string value of field looks like
// a valid email address. Missing fields are skipped.
func IsEmail(field string) func(map[string]any) error {
	return func(m map[string]any) error {
		v, ok := m[field]
		if !ok {
			return nil
		}
		s, ok := v.(string)
		if !ok {
			return &ValidationError{Field: field, Message: "must be a string email address"}
		}
		if !reEmail.MatchString(s) {
			return &ValidationError{Field: field, Message: "must be a valid email address"}
		}
		return nil
	}
}

// ValidateRequest is an HTTP middleware that rejects POST, PUT, and PATCH
// requests whose Content-Type header is not application/json. Requests with
// other methods, or with an empty body (Content-Length: 0), pass through
// unchanged.
func ValidateRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			// Skip the check when there is explicitly no body to avoid rejecting
			// action endpoints (e.g. POST /jobs/{id}/retry) that carry no payload.
			if r.ContentLength == 0 {
				break
			}
			ct := r.Header.Get("Content-Type")
			// Strip parameters like "; charset=utf-8".
			if idx := strings.Index(ct, ";"); idx != -1 {
				ct = ct[:idx]
			}
			ct = strings.TrimSpace(ct)
			if ct != "application/json" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnsupportedMediaType)
				json.NewEncoder(w).Encode(map[string]string{
					"error":   "unsupported_media_type",
					"message": "Content-Type must be application/json",
				})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
