package rest

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
)

const (
	connectTimeout        = 5 * time.Second
	requestTimeout        = 30 * time.Second
	downloadTimeout       = 120 * time.Second
	apiCap          int64 = 1 << 20
	downloadCap     int64 = 16 << 20
)

var errPrivate = errors.New("ssrf.private")
var errTooLarge = errors.New("response too large")

// Client sends provider requests through one guarded transport.
type Client struct {
	BaseURL          string
	Auth             AuthFunc
	RedirectHosts    []string
	UserAgent        string
	APIVersion       string
	Provider         string
	AllowPrivateBase bool
	Resolver         *net.Resolver
	TLSConfig        *tls.Config
	Logger           *slog.Logger
	observer         func(Outcome)
}

// Response contains only safe response metadata.
type Response struct {
	Status  int
	RetryAt time.Time
}

func (c *Client) baseHost() string {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func (c *Client) httpClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = c.TLSConfig
	transport.DialContext = c.dialContext
	transport.TLSHandshakeTimeout = connectTimeout
	transport.DisableKeepAlives = true
	return &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: c.checkRedirect}
}

// Do sends one JSON API request. pathTemplate is logged without expanded parameters.
func (c *Client) Do(ctx context.Context, method, pathTemplate string, params map[string]string, body, out any) (Response, error) {
	start := time.Now()
	var response Response
	var finalErr error
	defer func() { c.observe(method, pathTemplate, response, finalErr, start) }()
	base, err := url.Parse(c.BaseURL)
	if err != nil || base.Scheme != "https" || base.Hostname() == "" || base.User != nil || base.RawQuery != "" {
		finalErr = config("base.url")
		return response, finalErr
	}
	path := pathTemplate
	for key, value := range params {
		if value == "." || value == ".." {
			finalErr = config("request.path")
			return response, finalErr
		}
		path = strings.ReplaceAll(path, "{"+key+"}", url.PathEscape(value))
	}
	if strings.ContainsAny(path, "?#") || strings.Contains(path, "{") || !strings.HasPrefix(path, "/") {
		finalErr = config("request.path")
		return response, finalErr
	}
	u := *base
	u.RawPath = strings.TrimRight(base.EscapedPath(), "/") + path
	u.Path, err = url.PathUnescape(u.RawPath)
	if err != nil {
		finalErr = config("request.path")
		return response, finalErr
	}
	var payload io.Reader
	if body != nil {
		encoded, encodeErr := json.Marshal(body)
		if encodeErr != nil {
			finalErr = config("request.body")
			return response, finalErr
		}
		payload = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(context.WithValue(ctx, privateBaseKey{}, true), method, u.String(), payload)
	if err != nil {
		finalErr = config("request.url")
		return response, finalErr
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent())
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.APIVersion != "" {
		req.Header.Set("X-GitHub-Api-Version", c.APIVersion)
	}
	if c.Auth != nil {
		token, authErr := c.Auth(ctx)
		if authErr != nil {
			finalErr = authErr
			return response, finalErr
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	resp, err := c.httpClient(requestTimeout).Do(req)
	if err != nil {
		finalErr = classifyNetwork(err)
		return response, finalErr
	}
	defer func() { _ = resp.Body.Close() }()
	response.Status = resp.StatusCode
	limited, err := readLimited(resp.Body, apiCap)
	if err != nil {
		finalErr = classifyRead(err, "response.too_large")
		return response, finalErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		finalErr = classify(resp.StatusCode, resp.Header, limited, time.Now())
		if pe, ok := finalErr.(*providers.Error); ok {
			response.RetryAt = pe.RetryAt
		}
		return response, finalErr
	}
	if out != nil && len(limited) > 0 && json.Unmarshal(limited, out) != nil {
		finalErr = config("response.json")
	}
	return response, finalErr
}

// Download copies at most maxBytes, never exceeding the global artifact cap.
func (c *Client) Download(ctx context.Context, rawURL string, maxBytes int64, w io.Writer) (Response, error) {
	return c.download(ctx, rawURL, maxBytes, w, c.httpClient(downloadTimeout))
}

func (c *Client) download(ctx context.Context, rawURL string, maxBytes int64, w io.Writer, httpClient *http.Client) (Response, error) {
	start := time.Now()
	var response Response
	var finalErr error
	defer func() { c.observe("GET", "<download>", response, finalErr, start) }()
	u, err := url.Parse(rawURL)
	if err != nil || !c.allowedURL(u, true) {
		finalErr = config("download.host")
		return response, finalErr
	}
	if maxBytes <= 0 || maxBytes > downloadCap {
		maxBytes = downloadCap
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		finalErr = config("download.url")
		return response, finalErr
	}
	req.Header.Set("User-Agent", c.userAgent())
	if sameAuthority(u, c.BaseURL) && c.Auth != nil {
		token, authErr := c.Auth(ctx)
		if authErr != nil {
			finalErr = authErr
			return response, finalErr
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		finalErr = classifyNetwork(err)
		return response, finalErr
	}
	defer func() { _ = resp.Body.Close() }()
	response.Status = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limited, readErr := readLimited(resp.Body, apiCap)
		if readErr != nil {
			finalErr = classifyRead(readErr, "response.too_large")
		} else {
			finalErr = classify(resp.StatusCode, resp.Header, limited, time.Now())
			if pe, ok := finalErr.(*providers.Error); ok {
				response.RetryAt = pe.RetryAt
			}
		}
		return response, finalErr
	}
	// Buffer before writing, so an oversized artifact leaves no partial output.
	data, readErr := readLimited(resp.Body, maxBytes)
	if readErr != nil {
		finalErr = classifyRead(readErr, "download.too_large")
		return response, finalErr
	}
	if _, err = w.Write(data); err != nil {
		finalErr = providers.NewError(providers.Transient, "E704", "download.write", 0, time.Time{})
	}
	return response, finalErr
}

func (c *Client) userAgent() string {
	if c.UserAgent != "" {
		return "nself-ci/" + c.UserAgent
	}
	return "nself-ci/unknown"
}

func readLimited(reader io.Reader, cap int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, cap+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > cap {
		return nil, errTooLarge
	}
	return data, nil
}

func classifyRead(err error, sizeReason string) *providers.Error {
	if errors.Is(err, errTooLarge) {
		return config(sizeReason)
	}
	return providers.NewError(providers.Transient, "E704", "network", 0, time.Time{})
}
