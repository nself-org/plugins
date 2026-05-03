package internal

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestParseHTML_OGTags verifies Open Graph tag extraction.
func TestParseHTML_OGTags(t *testing.T) {
	html := `<!DOCTYPE html><html><head>
		<meta property="og:title" content="OG Title">
		<meta property="og:description" content="OG Description">
		<meta property="og:image" content="https://example.com/img.png">
		<meta property="og:site_name" content="Example Site">
		<meta property="og:type" content="article">
	</head><body></body></html>`

	m, err := parseHTML(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parseHTML error: %v", err)
	}
	if m.Title != "OG Title" {
		t.Errorf("Title: got %q, want %q", m.Title, "OG Title")
	}
	if m.Description != "OG Description" {
		t.Errorf("Description: got %q, want %q", m.Description, "OG Description")
	}
	if m.Image != "https://example.com/img.png" {
		t.Errorf("Image: got %q, want %q", m.Image, "https://example.com/img.png")
	}
	if m.SiteName != "Example Site" {
		t.Errorf("SiteName: got %q, want %q", m.SiteName, "Example Site")
	}
	if m.Type != "article" {
		t.Errorf("Type: got %q, want %q", m.Type, "article")
	}
}

// TestParseHTML_TwitterCardFallback verifies Twitter Card tags used as fallback.
func TestParseHTML_TwitterCardFallback(t *testing.T) {
	html := `<!DOCTYPE html><html><head>
		<meta name="twitter:title" content="Twitter Title">
		<meta name="twitter:description" content="Twitter Desc">
		<meta name="twitter:image" content="https://example.com/tw.png">
	</head><body></body></html>`

	m, err := parseHTML(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parseHTML error: %v", err)
	}
	if m.Title != "Twitter Title" {
		t.Errorf("Title: got %q, want %q", m.Title, "Twitter Title")
	}
	if m.Description != "Twitter Desc" {
		t.Errorf("Description: got %q, want %q", m.Description, "Twitter Desc")
	}
	if m.Image != "https://example.com/tw.png" {
		t.Errorf("Image: got %q, want %q", m.Image, "https://example.com/tw.png")
	}
}

// TestParseHTML_HTMLFallback verifies that <title> and <meta name="description">
// are used when OG/Twitter tags are absent.
func TestParseHTML_HTMLFallback(t *testing.T) {
	html := `<!DOCTYPE html><html><head>
		<title>HTML Title</title>
		<meta name="description" content="Meta description text">
	</head><body></body></html>`

	m, err := parseHTML(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parseHTML error: %v", err)
	}
	if m.Title != "HTML Title" {
		t.Errorf("Title: got %q, want %q", m.Title, "HTML Title")
	}
	if m.Description != "Meta description text" {
		t.Errorf("Description: got %q, want %q", m.Description, "Meta description text")
	}
}

// TestParseHTML_OGPrecedenceOverTwitter verifies that OG tags take precedence
// when both OG and Twitter tags are present.
func TestParseHTML_OGPrecedenceOverTwitter(t *testing.T) {
	html := `<!DOCTYPE html><html><head>
		<meta property="og:title" content="OG Wins">
		<meta name="twitter:title" content="Twitter Loses">
		<meta property="og:description" content="OG desc">
		<meta name="twitter:description" content="Twitter desc">
	</head><body></body></html>`

	m, err := parseHTML(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parseHTML error: %v", err)
	}
	if m.Title != "OG Wins" {
		t.Errorf("Title: got %q, want %q", m.Title, "OG Wins")
	}
	if m.Description != "OG desc" {
		t.Errorf("Description: got %q, want %q", m.Description, "OG desc")
	}
}

// TestParseHTML_EmptyDocument verifies that an empty or minimal HTML document
// returns an empty Metadata struct without error.
func TestParseHTML_EmptyDocument(t *testing.T) {
	m, err := parseHTML(strings.NewReader(`<html><head></head><body></body></html>`))
	if err != nil {
		t.Fatalf("parseHTML error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil Metadata")
	}
	if m.Title != "" || m.Description != "" || m.Image != "" {
		t.Errorf("expected empty fields, got Title=%q Desc=%q Image=%q", m.Title, m.Description, m.Image)
	}
}

// TestExtractMetadata_HappyPath verifies that ExtractMetadata fetches a URL
// served by a test HTTP server and returns the expected OG metadata.
func TestExtractMetadata_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<!DOCTYPE html><html><head>
			<meta property="og:title" content="Test Page">
			<meta property="og:description" content="A test page">
		</head><body>Body content</body></html>`))
	}))
	defer srv.Close()

	m, err := ExtractMetadata(srv.URL)
	if err != nil {
		t.Fatalf("ExtractMetadata error: %v", err)
	}
	if m.Title != "Test Page" {
		t.Errorf("Title: got %q, want %q", m.Title, "Test Page")
	}
	if m.Description != "A test page" {
		t.Errorf("Description: got %q, want %q", m.Description, "A test page")
	}
}

// TestExtractMetadata_HTTPError verifies that a 4xx response returns an error.
func TestExtractMetadata_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := ExtractMetadata(srv.URL)
	if err == nil {
		t.Error("expected error for 404 response, got nil")
	}
}

// TestExtractMetadata_NonHTMLContentType verifies that non-HTML responses are rejected.
func TestExtractMetadata_NonHTMLContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"key":"value"}`))
	}))
	defer srv.Close()

	_, err := ExtractMetadata(srv.URL)
	if err == nil {
		t.Error("expected error for non-HTML content type, got nil")
	}
}

// TestExtractMetadata_InvalidURL verifies that an invalid URL returns an error.
func TestExtractMetadata_InvalidURL(t *testing.T) {
	_, err := ExtractMetadata("not-a-valid-url")
	if err == nil {
		t.Error("expected error for invalid URL, got nil")
	}
}

// TestExtractMetadata_XHTMLContentType verifies that application/xhtml+xml is accepted.
func TestExtractMetadata_XHTMLContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xhtml+xml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml">
			<head><title>XHTML Page</title></head><body></body></html>`))
	}))
	defer srv.Close()

	m, err := ExtractMetadata(srv.URL)
	if err != nil {
		t.Fatalf("ExtractMetadata xhtml error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil metadata for xhtml page")
	}
}
