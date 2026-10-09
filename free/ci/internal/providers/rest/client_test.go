package rest

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
)

func testClient(server *httptest.Server) *Client {
	return &Client{BaseURL: server.URL, AllowPrivateBase: true, TLSConfig: server.Client().Transport.(*http.Transport).TLSClientConfig}
}

func assertError(t *testing.T, err error, code, reason string) {
	t.Helper()
	var pe *providers.Error
	if !errors.As(err, &pe) || pe.Code != code || reason != "" && !strings.Contains(pe.Reason, reason) {
		t.Fatalf("want %s/%s, got %v", code, reason, err)
	}
}

// TestNoRetryOnPost proves a 500 POST is sent once and classified transient.
func TestNoRetryOnPost(t *testing.T) {
	var count atomic.Int32
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { count.Add(1); w.WriteHeader(500) }))
	defer s.Close()
	c := testClient(s)
	_, err := c.Do(context.Background(), "POST", "/dispatch", nil, map[string]string{"x": "y"}, nil)
	assertError(t, err, "E704", "server")
	if count.Load() != 1 {
		t.Fatalf("POST count=%d", count.Load())
	}
}

// TestRedirectRules proves allowlisting, host changes, HTTPS, IP and hop limits.
func TestRedirectRules(t *testing.T) {
	c := &Client{BaseURL: "https://origin.example", RedirectHosts: []string{"target.example.com"}}
	for _, raw := range []string{"http://target.example.com/x", "https://127.0.0.1/x", "https://other.example/x"} {
		req, _ := http.NewRequest("GET", raw, nil)
		if err := c.checkRedirect(req, []*http.Request{{}}); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	req, _ := http.NewRequest("GET", "https://target.example.com/x", nil)
	req.Header.Set("Authorization", "Bearer secret")
	original, _ := http.NewRequest("GET", "https://origin.example/x", nil)
	if err := c.checkRedirect(req, []*http.Request{original}); err != nil || req.Header.Get("Authorization") != "" {
		t.Fatalf("host change: %v", err)
	}
	if err := c.checkRedirect(req, []*http.Request{original, original, original, original}); err == nil {
		t.Fatal("accepted fourth redirect")
	}
	if c.allowedURL(mustURL(t, "https://other.example/artifact"), true) {
		t.Fatal("accepted foreign download")
	}
	if !hostMatch("child.target.example.com", "target.example.com") || hostMatch("eviltarget.example.com", "target.example.com") {
		t.Fatal("host suffix boundary failed")
	}
	targetSeen := false
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetSeen = true
		if r.Header.Get("Authorization") != "" {
			t.Error("authorization reached redirect host")
		}
		w.WriteHeader(204)
	}))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://target.example.com/final", http.StatusFound)
	}))
	defer source.Close()
	c.BaseURL = source.URL
	transport := source.Client().Transport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address == "target.example.com:443" {
			address = target.Listener.Addr().String()
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	httpClient := &http.Client{Transport: transport, CheckRedirect: c.checkRedirect}
	initial, _ := http.NewRequest("GET", source.URL, nil)
	initial.Header.Set("Authorization", "Bearer secret")
	response, err := httpClient.Do(initial)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if !targetSeen {
		t.Fatal("allowlisted target not reached")
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// TestSizeCaps proves oversized API and artifact bodies fail before reading past the cap.
func TestSizeCaps(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/artifact" {
			_, _ = io.CopyN(w, strings.NewReader(strings.Repeat("x", 17<<20)), 17<<20)
			return
		}
		_, _ = io.CopyN(w, strings.NewReader(strings.Repeat("x", 2<<20)), 2<<20)
	}))
	defer s.Close()
	c := testClient(s)
	_, err := c.Do(context.Background(), "GET", "/api", nil, nil, nil)
	assertError(t, err, "E702", "too_large")
	_, err = readLimited(strings.NewReader(strings.Repeat("x", 17<<20)), downloadCap)
	if err == nil {
		t.Fatal("17 MiB artifact passed 16 MiB cap")
	}
	var output bytes.Buffer
	_, err = c.Download(context.Background(), s.URL+"/artifact", 17<<20, &output)
	assertError(t, err, "E702", "download.host")
	if output.Len() != 0 {
		t.Fatal("untrusted artifact was written")
	}
}

// TestClassification proves status, scope and retry header fixtures map to D3 classes.
func TestClassification(t *testing.T) {
	cases := []struct {
		status       int
		header       http.Header
		body         string
		code, reason string
	}{
		{401, nil, "", "E701", "credential"},
		{403, http.Header{"X-Accepted-Oauth-Scopes": []string{"admin:org"}}, "", "E701", "admin:org"},
		{403, http.Header{"X-Ratelimit-Remaining": []string{"0"}}, "", "E703", "rate_limit"},
		{403, nil, "secondary rate limit", "E703", "rate_limit"},
		{404, nil, "", "E702", "request"}, {409, nil, "", "E702", "request"},
		{410, nil, "", "E702", "request"}, {422, nil, "", "E702", "request"},
		{429, http.Header{"Retry-After": []string{"12"}}, "", "E703", "rate_limit"},
		{500, nil, "", "E704", "server"},
	}
	for _, tc := range cases {
		e := classify(tc.status, tc.header, []byte(tc.body), time.Now())
		if e.Code != tc.code || !strings.Contains(e.Reason, tc.reason) {
			t.Fatalf("%d: %#v", tc.status, e)
		}
		if tc.status == 429 && e.RetryAt.IsZero() {
			t.Fatal("missing retry time")
		}
	}
}

// TestNoTokenLeak proves logs, errors and observer outcomes contain no credential.
func TestNoTokenLeak(t *testing.T) {
	const secret = "test-secret-token-123"
	t.Setenv("NSELF_TEST_PROVIDER_TOKEN", secret)
	var log bytes.Buffer
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			t.Error("authorization missing")
		}
		w.WriteHeader(403)
		_, _ = w.Write([]byte(secret))
	}))
	defer s.Close()
	c := testClient(s)
	c.Auth = EnvToken("NSELF_TEST_PROVIDER_TOKEN")
	c.Logger = slog.New(slog.NewTextHandler(&log, nil))
	var outcome Outcome
	c.SetObserver(func(o Outcome) { outcome = o })
	_, err := c.Do(context.Background(), "GET", "/private", nil, nil, nil)
	assertError(t, err, "E701", "permission")
	if strings.Contains(log.String()+err.Error()+outcome.Provider+string(outcome.Class), secret) {
		t.Fatal("token leaked")
	}
	if outcome.Class != providers.Auth || outcome.Duration <= 0 {
		t.Fatalf("bad outcome: %+v", outcome)
	}
}

// TestGHTokenViaExec proves gh argv is fixed and stdout stays only in memory.
func TestGHTokenViaExec(t *testing.T) {
	dir := t.TempDir()
	args := filepath.Join(dir, "argv")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > \"" + args + "\"\nprintf 'fake-gh-token\\n'\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	get := GHToken("github.com")
	first, err := get(context.Background())
	if err != nil || first != "fake-gh-token" {
		t.Fatalf("token: %v", err)
	}
	second, err := get(context.Background())
	if err != nil || second != first {
		t.Fatalf("cached token: %v", err)
	}
	got, err := os.ReadFile(args)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "auth token --hostname github.com\n" {
		t.Fatalf("argv: %s", got)
	}
}

// TestSSRF proves private addresses are rejected at dial and base opt-in is narrow.
func TestSSRF(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.5", "169.254.169.254", "::ffff:10.0.0.1", "64:ff9b::a00:1"} {
		ip := mustAddr(t, raw)
		if !privateAddress(ip) {
			t.Fatalf("accepted %s", raw)
		}
	}
	c := &Client{BaseURL: "https://localhost"}
	_, err := c.dialContext(context.Background(), "tcp", "127.0.0.1:443")
	if !errors.Is(err, errPrivate) {
		t.Fatalf("private dial: %v", err)
	}
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	defer s.Close()
	allowed := testClient(s)
	_, err = allowed.Do(context.Background(), "GET", "/", nil, nil, nil)
	if err != nil {
		t.Fatalf("private base opt-in: %v", err)
	}
	_, err = allowed.Download(context.Background(), s.URL, 1, io.Discard)
	assertError(t, err, "E702", "download.host")
	redirectURL := strings.Replace(s.URL, "127.0.0.1", "localhost", 1)
	redirected, _ := http.NewRequest("GET", redirectURL+"/other", nil)
	original, _ := http.NewRequest("GET", s.URL+"/", nil)
	allowed.RedirectHosts = []string{"localhost"}
	if err := allowed.checkRedirect(redirected, []*http.Request{original}); err != nil {
		t.Fatal(err)
	}
	_, err = allowed.dialContext(redirected.Context(), "tcp", strings.TrimPrefix(redirectURL, "https://"))
	if !errors.Is(err, errPrivate) {
		t.Fatalf("private redirect connected: %v", err)
	}
	allowed.AllowPrivateBase = false
	_, err = allowed.Do(context.Background(), "GET", "/", nil, nil, nil)
	assertError(t, err, "E702", "ssrf.private")
	t.Run("rebinding", func(t *testing.T) {
		resolver := rebindingResolver()
		first, lookupErr := resolver.LookupIPAddr(context.Background(), "rebind.test")
		if lookupErr != nil || len(first) != 1 || first[0].IP.String() != "8.8.8.8" {
			t.Fatalf("public first answer: %v %v", first, lookupErr)
		}
		client := &Client{BaseURL: "https://rebind.test", Resolver: resolver}
		_, dialErr := client.dialContext(context.Background(), "tcp", "rebind.test:443")
		if !errors.Is(dialErr, errPrivate) {
			t.Fatalf("second private answer reached dial: %v", dialErr)
		}
	})
}

func mustAddr(t *testing.T, raw string) netip.Addr {
	t.Helper()
	ip, err := netip.ParseAddr(raw)
	if err != nil {
		t.Fatal(err)
	}
	return ip
}

func rebindingResolver() *net.Resolver {
	var aQueries atomic.Int32
	return &net.Resolver{PreferGo: true, Dial: func(_ context.Context, _, _ string) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer func() { _ = server.Close() }()
			buf := make([]byte, 512)
			n, err := server.Read(buf)
			if err != nil || n < 17 {
				return
			}
			query := buf[:n]
			framed := n >= 2 && int(binary.BigEndian.Uint16(query[:2])) == n-2
			if framed {
				query = query[2:]
			}
			end := 12
			for end < len(query) && query[end] != 0 {
				end += int(query[end]) + 1
			}
			if end+5 > len(query) {
				return
			}
			end += 5
			answer := append([]byte(nil), query[:end]...)
			answer[2], answer[3] = 0x81, 0x80
			for i := 6; i < 12; i++ {
				answer[i] = 0
			}
			if binary.BigEndian.Uint16(query[end-4:end-2]) == 1 {
				answer[7] = 1
				ip := []byte{8, 8, 8, 8}
				if aQueries.Add(1) > 1 {
					ip = []byte{127, 0, 0, 1}
				}
				answer = append(answer, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 0, 0, 4)
				answer = append(answer, ip...)
			}
			if framed {
				prefix := []byte{0, 0}
				binary.BigEndian.PutUint16(prefix, uint16(len(answer)))
				answer = append(prefix, answer...)
			}
			_, _ = server.Write(answer)
		}()
		return client, nil
	}}
}
