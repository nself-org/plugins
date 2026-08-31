package wordpress

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNewClientAppendsXMLRPC verifies that NewClient appends /xmlrpc.php when
// only the site root is supplied.
func TestNewClientAppendsXMLRPC(t *testing.T) {
	c := NewClient("https://example.com", "user", "pass")
	if c.Endpoint != "https://example.com/xmlrpc.php" {
		t.Fatalf("expected endpoint to be appended, got %q", c.Endpoint)
	}

	c2 := NewClient("https://example.com/xmlrpc.php", "user", "pass")
	if c2.Endpoint != "https://example.com/xmlrpc.php" {
		t.Fatalf("endpoint should not be re-appended, got %q", c2.Endpoint)
	}
}

// TestNewPostSuccess runs wp.newPost against a mock server and verifies that
// the post ID + permalink are extracted from the response correctly.
func TestNewPostSuccess(t *testing.T) {
	callCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/xml") {
			t.Errorf("expected text/xml content type, got %q", ct)
		}
		body, _ := io.ReadAll(r.Body)
		bodyStr := string(body)
		// Sanity-check the request contains the expected method name, credentials,
		// and the post data.
		if callCount == 1 && !strings.Contains(bodyStr, "wp.newPost") {
			t.Errorf("first call should be wp.newPost, got body: %s", bodyStr)
		}
		if callCount == 2 && !strings.Contains(bodyStr, "wp.getPost") {
			t.Errorf("second call should be wp.getPost, got body: %s", bodyStr)
		}
		if !strings.Contains(bodyStr, "testuser") {
			t.Errorf("request missing username, got: %s", bodyStr)
		}
		if !strings.Contains(bodyStr, "apppass123") {
			t.Errorf("request missing app password, got: %s", bodyStr)
		}

		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		if callCount == 1 {
			// wp.newPost returns the new post ID as a string.
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<methodResponse>
  <params>
    <param><value><string>4242</string></value></param>
  </params>
</methodResponse>`))
		} else {
			// wp.getPost returns a struct containing the permalink under "link".
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<methodResponse>
  <params>
    <param><value><struct>
      <member><name>post_id</name><value><string>4242</string></value></member>
      <member><name>link</name><value><string>https://example.com/2026/04/16/hello-world/</string></value></member>
    </struct></value></param>
  </params>
</methodResponse>`))
		}
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	c := NewClient(srv.URL, "testuser", "apppass123")
	res, err := c.NewPost(context.Background(), NewPostArgs{
		Title:      "Hello World",
		Content:    "Body <strong>html</strong>",
		Categories: []string{"General"},
		Tags:       []string{"launch", "news"},
	})
	if err != nil {
		t.Fatalf("NewPost error: %v", err)
	}
	if res.PostID != "4242" {
		t.Errorf("PostID: got %q, want %q", res.PostID, "4242")
	}
	if res.Permalink != "https://example.com/2026/04/16/hello-world/" {
		t.Errorf("Permalink: got %q", res.Permalink)
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls (newPost + getPost), got %d", callCount)
	}
}

// TestNewPostFault verifies that an XML-RPC fault is surfaced as an error.
func TestNewPostFault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<methodResponse>
  <fault>
    <value><struct>
      <member><name>faultCode</name><value><int>403</int></value></member>
      <member><name>faultString</name><value><string>Incorrect username or password.</string></value></member>
    </struct></value>
  </fault>
</methodResponse>`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "user", "bad")
	_, err := c.NewPost(context.Background(), NewPostArgs{Title: "t", Content: "c"})
	if err == nil {
		t.Fatal("expected error for fault response, got nil")
	}
	if !strings.Contains(err.Error(), "Incorrect username") {
		t.Errorf("expected fault message in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("expected fault code in error, got: %v", err)
	}
}

// TestNewPostHTTPError verifies handling of non-2xx responses.
func TestNewPostHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "user", "pass")
	_, err := c.NewPost(context.Background(), NewPostArgs{Title: "t", Content: "c"})
	if err == nil {
		t.Fatal("expected HTTP error, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("expected HTTP 500 in error, got: %v", err)
	}
}

// TestNewPostMissingCredentials ensures the client refuses to call out when
// credentials are empty — this protects callers from leaking URLs.
func TestNewPostMissingCredentials(t *testing.T) {
	c := NewClient("https://example.com", "", "")
	_, err := c.NewPost(context.Background(), NewPostArgs{Title: "t", Content: "c"})
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("expected 'required' message, got: %v", err)
	}
}

// TestNewPostEmptyEndpoint checks the empty-endpoint guard.
func TestNewPostEmptyEndpoint(t *testing.T) {
	c := &Client{Username: "u", AppPassword: "p"}
	_, err := c.NewPost(context.Background(), NewPostArgs{Title: "t", Content: "c"})
	if err == nil || !strings.Contains(err.Error(), "endpoint is empty") {
		t.Fatalf("expected endpoint-empty error, got: %v", err)
	}
}

// TestBuildNewPostIncludesTerms verifies the category/tag struct is emitted
// correctly in the request body.
func TestBuildNewPostIncludesTerms(t *testing.T) {
	raw := buildNewPostRequest("u", "p", "Title", "Body", []string{"Cat1"}, []string{"tag1", "tag2"}, "publish")
	body := string(raw)
	if !strings.Contains(body, "terms_names") {
		t.Error("expected terms_names in request")
	}
	if !strings.Contains(body, "<name>category</name>") {
		t.Error("expected category in terms_names")
	}
	if !strings.Contains(body, "<name>post_tag</name>") {
		t.Error("expected post_tag in terms_names")
	}
	if !strings.Contains(body, "<string>tag1</string>") || !strings.Contains(body, "<string>tag2</string>") {
		t.Error("expected both tags in request")
	}
}

// TestXMLEscapingInRequest ensures title/body with special chars don't break the XML.
func TestXMLEscapingInRequest(t *testing.T) {
	raw := buildNewPostRequest("u", "p", "A & B <script>", "</strong>", nil, nil, "publish")
	body := string(raw)
	if strings.Contains(body, "<script>") {
		t.Error("unescaped <script> in request body")
	}
	if !strings.Contains(body, "&amp;") {
		t.Error("expected & to be escaped as &amp;")
	}
}
