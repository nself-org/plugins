// Package wordpress implements a WordPress publishing client via XML-RPC.
//
// The WordPress XML-RPC interface (`xmlrpc.php`) is enabled by default on all
// self-hosted WordPress installs and on WordPress.com. Authentication uses an
// Application Password (wp-admin/authorize-application.php) rather than the
// account password — this is the official WP-supported way to grant scoped
// API access without exposing the real credential.
//
// This package intentionally uses only net/http and encoding/xml so the plugin
// does not take on an additional dependency just for XML-RPC.
package wordpress

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/shared/httpclient"
)

// wordpressFallbackClient is the default HTTP client used when
// Client.HTTPClient is nil. 30s budget per spec.
var wordpressFallbackClient = httpclient.New(httpclient.Options{Timeout: 30 * time.Second})

// Client publishes posts to a WordPress site via XML-RPC.
type Client struct {
	// Endpoint is the full URL to the WordPress xmlrpc.php, e.g.
	// https://example.com/xmlrpc.php. If the caller supplies only the site
	// root ("https://example.com") NewClient will append /xmlrpc.php.
	Endpoint string

	// Username is the WordPress account username.
	Username string

	// AppPassword is a WordPress Application Password (NOT the login password).
	// Generated under Users → Profile → Application Passwords in wp-admin.
	AppPassword string

	// HTTPClient is the transport used for XML-RPC calls. If nil, a default
	// client with a 30 second timeout is used.
	HTTPClient *http.Client
}

// NewClient returns a Client with sensible defaults.
func NewClient(endpoint, username, appPassword string) *Client {
	if endpoint != "" && !strings.Contains(endpoint, "xmlrpc.php") {
		endpoint = strings.TrimRight(endpoint, "/") + "/xmlrpc.php"
	}
	return &Client{
		Endpoint:    endpoint,
		Username:    username,
		AppPassword: appPassword,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewPostArgs is the body of a wp.newPost call.
type NewPostArgs struct {
	Title      string
	Content    string
	Categories []string
	Tags       []string
	Status     string // "publish" (default), "draft", "pending", "private"
}

// NewPostResult is the outcome of a successful wp.newPost call.
type NewPostResult struct {
	PostID    string // WordPress post ID (returned as a string by the API)
	Permalink string // Resolved permalink via wp.getPost
}

// NewPost publishes a new post via wp.newPost and resolves its permalink via
// wp.getPost. Both calls happen inside the supplied context.
func (c *Client) NewPost(ctx context.Context, args NewPostArgs) (*NewPostResult, error) {
	if c.Endpoint == "" {
		return nil, fmt.Errorf("wordpress: endpoint is empty")
	}
	if c.Username == "" || c.AppPassword == "" {
		return nil, fmt.Errorf("wordpress: username and application password are required")
	}
	status := args.Status
	if status == "" {
		status = "publish"
	}

	// wp.newPost signature: blog_id, username, password, content_struct
	// We use blog_id=1 because single-site WP always answers to 1 and
	// MU routing is handled by the URL itself.
	payload := buildNewPostRequest(c.Username, c.AppPassword, args.Title, args.Content, args.Categories, args.Tags, status)
	rawResp, err := c.call(ctx, payload)
	if err != nil {
		return nil, fmt.Errorf("wordpress wp.newPost: %w", err)
	}
	postID, err := parseStringResponse(rawResp)
	if err != nil {
		return nil, fmt.Errorf("wordpress wp.newPost response: %w", err)
	}

	// Resolve the permalink. If this fails we still return the ID — the post
	// was created successfully; permalink lookup is informational only.
	permalink, _ := c.fetchPermalink(ctx, postID)

	return &NewPostResult{PostID: postID, Permalink: permalink}, nil
}

// fetchPermalink calls wp.getPost to retrieve the final URL.
func (c *Client) fetchPermalink(ctx context.Context, postID string) (string, error) {
	payload := buildGetPostRequest(c.Username, c.AppPassword, postID)
	rawResp, err := c.call(ctx, payload)
	if err != nil {
		return "", err
	}
	return parseLinkFromPostStruct(rawResp)
}

// call POSTs an XML-RPC payload to the configured endpoint and returns the
// response body. Non-2xx responses are returned as errors with the status.
func (c *Client) call(ctx context.Context, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("User-Agent", "nself-post/1.0 (+https://nself.org)")

	client := c.HTTPClient
	if client == nil {
		client = wordpressFallbackClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}

// =============================================================================
// XML-RPC request building
// =============================================================================

// buildNewPostRequest constructs an XML-RPC payload for wp.newPost.
//
// Layout:
//
//	<methodCall>
//	  <methodName>wp.newPost</methodName>
//	  <params>
//	    <param><value><int>1</int></value></param>           <!-- blog_id -->
//	    <param><value><string>user</string></value></param>  <!-- username -->
//	    <param><value><string>pass</string></value></param>  <!-- app password -->
//	    <param><value><struct>                                 <!-- content -->
//	       post_title, post_content, post_status, terms_names
//	    </struct></value></param>
//	  </params>
//	</methodCall>
func buildNewPostRequest(username, appPass, title, content string, categories, tags []string, status string) []byte {
	var b strings.Builder
	b.WriteString(xml.Header)
	b.WriteString("<methodCall>")
	b.WriteString("<methodName>wp.newPost</methodName>")
	b.WriteString("<params>")

	// param 1: blog_id = 1
	b.WriteString("<param><value><int>1</int></value></param>")
	// param 2: username
	b.WriteString("<param><value><string>")
	xmlEscape(&b, username)
	b.WriteString("</string></value></param>")
	// param 3: password (application password)
	b.WriteString("<param><value><string>")
	xmlEscape(&b, appPass)
	b.WriteString("</string></value></param>")
	// param 4: content struct
	b.WriteString("<param><value><struct>")

	writeStructMember(&b, "post_title", stringValue(title))
	writeStructMember(&b, "post_content", stringValue(content))
	writeStructMember(&b, "post_status", stringValue(status))

	if len(categories) > 0 || len(tags) > 0 {
		// terms_names is a struct with taxonomy → array of names
		var inner strings.Builder
		inner.WriteString("<struct>")
		if len(categories) > 0 {
			writeStructMember(&inner, "category", arrayOfStrings(categories))
		}
		if len(tags) > 0 {
			writeStructMember(&inner, "post_tag", arrayOfStrings(tags))
		}
		inner.WriteString("</struct>")
		writeStructMember(&b, "terms_names", inner.String())
	}

	b.WriteString("</struct></value></param>")
	b.WriteString("</params>")
	b.WriteString("</methodCall>")
	return []byte(b.String())
}

// buildGetPostRequest constructs an XML-RPC payload for wp.getPost.
func buildGetPostRequest(username, appPass, postID string) []byte {
	var b strings.Builder
	b.WriteString(xml.Header)
	b.WriteString("<methodCall>")
	b.WriteString("<methodName>wp.getPost</methodName>")
	b.WriteString("<params>")
	b.WriteString("<param><value><int>1</int></value></param>")
	b.WriteString("<param><value><string>")
	xmlEscape(&b, username)
	b.WriteString("</string></value></param>")
	b.WriteString("<param><value><string>")
	xmlEscape(&b, appPass)
	b.WriteString("</string></value></param>")
	b.WriteString("<param><value><int>")
	xmlEscape(&b, postID)
	b.WriteString("</int></value></param>")
	b.WriteString("</params>")
	b.WriteString("</methodCall>")
	return []byte(b.String())
}

func stringValue(s string) string {
	var b strings.Builder
	b.WriteString("<string>")
	xmlEscape(&b, s)
	b.WriteString("</string>")
	return b.String()
}

func arrayOfStrings(items []string) string {
	var b strings.Builder
	b.WriteString("<array><data>")
	for _, v := range items {
		b.WriteString("<value><string>")
		xmlEscape(&b, v)
		b.WriteString("</string></value>")
	}
	b.WriteString("</data></array>")
	return b.String()
}

func writeStructMember(b *strings.Builder, name, valueXML string) {
	b.WriteString("<member><name>")
	xmlEscape(b, name)
	b.WriteString("</name><value>")
	b.WriteString(valueXML)
	b.WriteString("</value></member>")
}

// xmlEscape writes s into b with XML special characters escaped.
func xmlEscape(b *strings.Builder, s string) {
	_ = xml.EscapeText(stringWriter{b}, []byte(s))
}

// stringWriter adapts strings.Builder to io.Writer for xml.EscapeText.
type stringWriter struct{ b *strings.Builder }

func (w stringWriter) Write(p []byte) (int, error) { return w.b.Write(p) }

// =============================================================================
// XML-RPC response parsing
// =============================================================================

// methodResponse is the top-level XML-RPC response document.
type methodResponse struct {
	XMLName xml.Name      `xml:"methodResponse"`
	Params  *paramsBlock  `xml:"params"`
	Fault   *faultElement `xml:"fault"`
}

type paramsBlock struct {
	Param paramElement `xml:"param"`
}

type paramElement struct {
	Value valueElement `xml:"value"`
}

// valueElement holds all possible XML-RPC scalar/compound types. XML-RPC is
// untyped at the top — a member can be any of these.
type valueElement struct {
	String  string         `xml:"string"`
	Int     string         `xml:"int"`
	I4      string         `xml:"i4"`
	Boolean string         `xml:"boolean"`
	Double  string         `xml:"double"`
	Array   *arrayElement  `xml:"array"`
	Struct  *structElement `xml:"struct"`
	Raw     string         `xml:",chardata"` // fallback when there is no inner tag
}

type arrayElement struct {
	Data struct {
		Value []valueElement `xml:"value"`
	} `xml:"data"`
}

type structElement struct {
	Member []memberElement `xml:"member"`
}

type memberElement struct {
	Name  string       `xml:"name"`
	Value valueElement `xml:"value"`
}

type faultElement struct {
	Value valueElement `xml:"value"`
}

// parseStringResponse extracts a single string/int param from a methodResponse.
// Returns an error if the response is a fault.
func parseStringResponse(raw []byte) (string, error) {
	var mr methodResponse
	if err := xml.Unmarshal(raw, &mr); err != nil {
		return "", fmt.Errorf("unmarshal: %w", err)
	}
	if mr.Fault != nil {
		msg, code := faultDetails(mr.Fault)
		return "", fmt.Errorf("XML-RPC fault %d: %s", code, msg)
	}
	if mr.Params == nil {
		return "", fmt.Errorf("no params in response")
	}
	v := mr.Params.Param.Value
	switch {
	case v.String != "":
		return v.String, nil
	case v.Int != "":
		return v.Int, nil
	case v.I4 != "":
		return v.I4, nil
	default:
		trimmed := strings.TrimSpace(v.Raw)
		if trimmed != "" {
			return trimmed, nil
		}
		return "", fmt.Errorf("empty response value")
	}
}

// parseLinkFromPostStruct extracts the "link" member from a wp.getPost struct.
// Returns the empty string (no error) when no link is present.
func parseLinkFromPostStruct(raw []byte) (string, error) {
	var mr methodResponse
	if err := xml.Unmarshal(raw, &mr); err != nil {
		return "", fmt.Errorf("unmarshal: %w", err)
	}
	if mr.Fault != nil {
		msg, code := faultDetails(mr.Fault)
		return "", fmt.Errorf("XML-RPC fault %d: %s", code, msg)
	}
	if mr.Params == nil || mr.Params.Param.Value.Struct == nil {
		return "", nil
	}
	for _, m := range mr.Params.Param.Value.Struct.Member {
		if m.Name == "link" && m.Value.String != "" {
			return m.Value.String, nil
		}
	}
	return "", nil
}

// faultDetails pulls the faultCode / faultString from a fault struct.
func faultDetails(f *faultElement) (string, int) {
	if f == nil || f.Value.Struct == nil {
		return "unknown fault", 0
	}
	msg := ""
	code := 0
	for _, m := range f.Value.Struct.Member {
		switch m.Name {
		case "faultString":
			msg = m.Value.String
		case "faultCode":
			if m.Value.Int != "" {
				fmt.Sscanf(m.Value.Int, "%d", &code)
			} else if m.Value.I4 != "" {
				fmt.Sscanf(m.Value.I4, "%d", &code)
			}
		}
	}
	if msg == "" {
		msg = "unknown fault"
	}
	return msg, code
}
