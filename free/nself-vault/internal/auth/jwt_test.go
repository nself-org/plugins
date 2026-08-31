package auth

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-key-do-not-use-in-prod"

// signClaims builds a signed JWT from arbitrary Claims for test use.
func signClaims(t *testing.T, c Claims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	signed, err := tok.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

// validClaims returns a baseline Claims struct that should successfully validate.
func validClaims() Claims {
	return Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Audience:  jwt.ClaimStrings{ExpectedAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		UserID: "11111111-1111-1111-1111-111111111111",
	}
}

func TestParse_ValidTokenAccepted(t *testing.T) {
	ex := NewExtractor(testSecret)
	tok := signClaims(t, validClaims())
	info, err := ex.Parse(tok)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if info.UserID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("unexpected user id: %q", info.UserID)
	}
}

// V05-F3a: tokens without an `exp` claim must be rejected.
func TestParse_MissingExpRejected(t *testing.T) {
	ex := NewExtractor(testSecret)
	c := validClaims()
	c.ExpiresAt = nil
	tok := signClaims(t, c)
	_, err := ex.Parse(tok)
	if err == nil {
		t.Fatal("expected rejection for missing exp, got nil")
	}
	if !strings.Contains(err.Error(), "exp") {
		t.Fatalf("error should mention exp: %v", err)
	}
}

// V05-F3a: expired tokens must be rejected.
func TestParse_ExpiredTokenRejected(t *testing.T) {
	ex := NewExtractor(testSecret)
	c := validClaims()
	c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-1 * time.Hour))
	tok := signClaims(t, c)
	_, err := ex.Parse(tok)
	if err == nil {
		t.Fatal("expected rejection for expired token, got nil")
	}
}

// V05-F3b: tokens with wrong audience must be rejected.
func TestParse_WrongAudRejected(t *testing.T) {
	ex := NewExtractor(testSecret)
	c := validClaims()
	c.Audience = jwt.ClaimStrings{"nself-sync"} // wrong service
	tok := signClaims(t, c)
	_, err := ex.Parse(tok)
	if err == nil {
		t.Fatal("expected rejection for wrong aud, got nil")
	}
	if !strings.Contains(err.Error(), "aud") {
		t.Fatalf("error should mention aud: %v", err)
	}
}

// V05-F3b: tokens with no audience claim must be rejected.
func TestParse_MissingAudRejected(t *testing.T) {
	ex := NewExtractor(testSecret)
	c := validClaims()
	c.Audience = nil
	tok := signClaims(t, c)
	_, err := ex.Parse(tok)
	if err == nil {
		t.Fatal("expected rejection for missing aud, got nil")
	}
}

// Tokens with aud as array containing ExpectedAudience among other audiences accepted.
func TestParse_AudArrayWithExpectedAccepted(t *testing.T) {
	ex := NewExtractor(testSecret)
	c := validClaims()
	c.Audience = jwt.ClaimStrings{"hasura", ExpectedAudience, "other"}
	tok := signClaims(t, c)
	if _, err := ex.Parse(tok); err != nil {
		t.Fatalf("expected success with aud array, got %v", err)
	}
}

func TestParse_BadSignatureRejected(t *testing.T) {
	ex := NewExtractor("different-secret")
	tok := signClaims(t, validClaims())
	if _, err := ex.Parse(tok); err == nil {
		t.Fatal("expected rejection for bad signature")
	}
}

func TestParse_MissingUserIDRejected(t *testing.T) {
	ex := NewExtractor(testSecret)
	c := validClaims()
	c.UserID = ""
	tok := signClaims(t, c)
	if _, err := ex.Parse(tok); err == nil {
		t.Fatal("expected rejection for missing user_id")
	}
}

func TestFromRequest_BearerExtraction(t *testing.T) {
	ex := NewExtractor(testSecret)
	tok := signClaims(t, validClaims())
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	if _, err := ex.FromRequest(r); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestFromRequest_MissingAuthHeader(t *testing.T) {
	ex := NewExtractor(testSecret)
	r := httptest.NewRequest("GET", "/", nil)
	if _, err := ex.FromRequest(r); err == nil {
		t.Fatal("expected rejection for missing Authorization")
	}
}

// HasuraClaims path still requires top-level aud + exp for V05-F3 compliance.
func TestParse_HasuraClaimsHonoredButAudStillEnforced(t *testing.T) {
	ex := NewExtractor(testSecret)
	c := validClaims()
	c.HasuraClaims = &HasuraClaims{
		UserID:   "22222222-2222-2222-2222-222222222222",
		TenantID: "33333333-3333-3333-3333-333333333333",
		Role:     "user",
	}
	tok := signClaims(t, c)
	info, err := ex.Parse(tok)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	// Hasura namespace overrides top-level user_id.
	if info.UserID != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("expected hasura user_id, got %q", info.UserID)
	}

	// Hasura claims do NOT bypass aud enforcement.
	c2 := validClaims()
	c2.Audience = jwt.ClaimStrings{"nself-sync"}
	c2.HasuraClaims = &HasuraClaims{UserID: "44444444-4444-4444-4444-444444444444"}
	tok2 := signClaims(t, c2)
	if _, err := ex.Parse(tok2); err == nil {
		t.Fatal("expected rejection: hasura claims must not bypass aud check")
	}
}
