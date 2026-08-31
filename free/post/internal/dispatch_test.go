package internal

import (
	"context"
	"errors"
	"testing"
)

// TestDispatchInternalNoop verifies that the "internal" platform returns
// success without hitting any network.
func TestDispatchInternalNoop(t *testing.T) {
	acc := &PostAccount{Platform: "internal"}
	res, err := dispatchToPlatform(context.Background(), acc, "t", "c", nil)
	if err != nil {
		t.Fatalf("internal dispatch: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if res.URL != nil {
		t.Errorf("expected nil URL for internal, got %v", *res.URL)
	}
}

// TestDispatchUnimplementedPlatforms verifies each planned-but-unshipped
// platform returns platformNotImplementedError so the HTTP layer can return 501.
// twitter, x, linkedin, telegram, devto, hashnode are now implemented — they
// return a credentials error when no credentials are supplied, not 501.
func TestDispatchUnimplementedPlatforms(t *testing.T) {
	platforms := []string{"mastodon", "bluesky", "facebook", "instagram", "threads"}
	for _, p := range platforms {
		t.Run(p, func(t *testing.T) {
			acc := &PostAccount{Platform: p}
			_, err := dispatchToPlatform(context.Background(), acc, "t", "c", nil)
			if err == nil {
				t.Fatal("expected error")
			}
			var notImpl *platformNotImplementedError
			if !errors.As(err, &notImpl) {
				t.Errorf("expected platformNotImplementedError, got %T: %v", err, err)
			}
			// The error message should name the platform for the API response.
			if notImpl != nil && notImpl.Platform == "" {
				t.Errorf("platformNotImplementedError missing platform name")
			}
		})
	}
}

// TestDispatchUnknownPlatform verifies that truly unknown platforms return a
// plain error (not platformNotImplementedError) — these map to 502, not 501.
func TestDispatchUnknownPlatform(t *testing.T) {
	acc := &PostAccount{Platform: "myspace"}
	_, err := dispatchToPlatform(context.Background(), acc, "t", "c", nil)
	if err == nil {
		t.Fatal("expected error for unknown platform")
	}
	var notImpl *platformNotImplementedError
	if errors.As(err, &notImpl) {
		t.Error("unknown platforms should not be typed as platformNotImplementedError")
	}
}

// TestDispatchWordPressMissingCreds verifies the WP dispatcher refuses to
// proceed when required credential fields are missing.
func TestDispatchWordPressMissingCreds(t *testing.T) {
	cases := []struct {
		name  string
		creds map[string]interface{}
	}{
		{"no endpoint", map[string]interface{}{"username": "u", "app_password": "p"}},
		{"no username", map[string]interface{}{"endpoint": "https://x.com", "app_password": "p"}},
		{"no app_password", map[string]interface{}{"endpoint": "https://x.com", "username": "u"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			acc := &PostAccount{Platform: "wordpress", Credentials: c.creds}
			_, err := dispatchToPlatform(context.Background(), acc, "t", "c", nil)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// TestDispatchGhostMissingCreds verifies the Ghost dispatcher refuses to
// proceed when required credential fields are missing.
func TestDispatchGhostMissingCreds(t *testing.T) {
	cases := []struct {
		name  string
		creds map[string]interface{}
	}{
		{"no endpoint", map[string]interface{}{"admin_api_key": "k:hex"}},
		{"no key", map[string]interface{}{"endpoint": "https://x.com"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			acc := &PostAccount{Platform: "ghost", Credentials: c.creds}
			_, err := dispatchToPlatform(context.Background(), acc, "t", "c", nil)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// TestStringsCred verifies extraction of []string from interface{} map.
func TestStringsCred(t *testing.T) {
	creds := map[string]interface{}{
		"a": []interface{}{"x", "y", 1, "z"},
		"b": "notalist",
		"c": []string{"direct"},
	}
	if got := stringsCred(creds, "a"); len(got) != 3 || got[0] != "x" || got[1] != "y" || got[2] != "z" {
		t.Errorf("stringsCred a: got %v", got)
	}
	if got := stringsCred(creds, "b"); got != nil {
		t.Errorf("stringsCred b (wrong type): got %v", got)
	}
	if got := stringsCred(creds, "c"); len(got) != 1 || got[0] != "direct" {
		t.Errorf("stringsCred c: got %v", got)
	}
	if got := stringsCred(creds, "missing"); got != nil {
		t.Errorf("stringsCred missing: got %v", got)
	}
	if got := stringsCred(nil, "a"); got != nil {
		t.Errorf("stringsCred nil map: got %v", got)
	}
}

// TestStringCred verifies extraction of a string field from the map.
func TestStringCred(t *testing.T) {
	creds := map[string]interface{}{"a": "value", "b": 42}
	if got := stringCred(creds, "a"); got != "value" {
		t.Errorf("stringCred a: got %q", got)
	}
	if got := stringCred(creds, "b"); got != "" {
		t.Errorf("stringCred b (wrong type): got %q", got)
	}
	if got := stringCred(creds, "missing"); got != "" {
		t.Errorf("stringCred missing: got %q", got)
	}
	if got := stringCred(nil, "a"); got != "" {
		t.Errorf("stringCred nil map: got %q", got)
	}
}
