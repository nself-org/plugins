package main

// redact.go: strips credentials from the values the v1.5 envelope returns.
//
// Purpose: monitor URLs, alert channel targets and status page URLs are values
//   the caller stored, and they can carry secrets (URL userinfo, token query
//   parameters, webhook path segments, bot tokens). The v1.5 envelope reaches
//   AI context and logs over MCP/HTTP, so its copy of these values is redacted
//   (D-0926).
// Inputs: the typed API values (sentryapi.Monitor, StatusPage, AlertChannel,
//   TestChannelResult).
// Outputs: redacted copies. The inputs are never changed, so the v1.4 bytes
//   (printed from the original values) and the stored values stay as they are.
// Constraints: pure functions, no I/O. A string with no credential shape comes
//   back byte-for-byte.

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/nself-org/nself-sentry-cli/internal/sentryapi"
)

// redactedMark replaces every credential-bearing part.
const redactedMark = "REDACTED"

var (
	// textURLRe finds absolute URLs inside free text such as a monitor name
	// (the name defaults to the URL).
	textURLRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.\-]*://[^\s"'<>]+`)
	// bareUserinfoRe matches user:pass@ at the start of a scheme-less target.
	bareUserinfoRe = regexp.MustCompile(`^[^/@?#\s]*:[^/@?#\s]*@`)
	// botTokenRe matches a Telegram bot token (<digits>:<secret>), with or
	// without the "bot" prefix.
	botTokenRe = regexp.MustCompile(`(?i)(bot)?\d{5,}:[A-Za-z0-9_\-]{20,}`)
	// slackTokenRe matches Slack API tokens (xoxb-, xoxp-, xapp-, ...).
	slackTokenRe = regexp.MustCompile(`\bx(?:ox[a-z]|app)-[A-Za-z0-9\-]{8,}`)
	// sensitiveQueryRe matches query parameter names that carry credentials.
	sensitiveQueryRe = regexp.MustCompile(`(?i)(token|secret|passw|pwd|key|auth|sig|cred|session|jwt|bearer|code|otp|hmac|apikey)`)
	// opaqueSegmentRe matches a long path segment that looks like a secret.
	opaqueSegmentRe = regexp.MustCompile(`^[A-Za-z0-9_\-=.~]{20,}$`)
)

// redactValue redacts the credential-bearing parts of one stored URL or target.
func redactValue(s string) string {
	if strings.Contains(s, "://") {
		return textURLRe.ReplaceAllStringFunc(s, redactURL)
	}
	if bareUserinfoRe.MatchString(s) {
		s = bareUserinfoRe.ReplaceAllString(s, redactedMark+"@")
	}
	s = botTokenRe.ReplaceAllString(s, redactedMark)
	return slackTokenRe.ReplaceAllString(s, redactedMark)
}

// redactURL redacts userinfo, sensitive query values, the fragment and secret
// path segments of one absolute URL. An unparsable URL loses its whole tail.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" {
		if i := strings.IndexAny(raw, "?#"); i >= 0 {
			raw = raw[:i] + "?" + redactedMark
		}
		return userinfoInURLRe.ReplaceAllString(raw, "://"+redactedMark+"@")
	}
	var b strings.Builder
	b.WriteString(u.Scheme + "://")
	if u.User != nil {
		b.WriteString(redactedMark + "@")
	}
	b.WriteString(u.Host)
	b.WriteString(redactPath(u.Host, u.EscapedPath()))
	if u.RawQuery != "" {
		b.WriteString("?" + redactQuery(u.RawQuery))
	}
	if u.Fragment != "" {
		b.WriteString("#" + redactedMark)
	}
	return b.String()
}

// userinfoInURLRe finds userinfo in a URL the parser rejected.
var userinfoInURLRe = regexp.MustCompile(`://[^/@?#]*@`)

// redactQuery keeps the parameter names and replaces sensitive values.
func redactQuery(q string) string {
	parts := strings.Split(q, "&")
	for i, p := range parts {
		name, _, hasValue := strings.Cut(p, "=")
		if !hasValue {
			continue
		}
		if dn, err := url.QueryUnescape(name); err == nil {
			name = dn
		}
		if sensitiveQueryRe.MatchString(name) {
			parts[i] = p[:strings.IndexByte(p, '=')+1] + redactedMark
		}
	}
	return strings.Join(parts, "&")
}

// redactPath replaces webhook secrets in the path: the Slack, Discord and
// Telegram shapes by name, and any other long opaque segment.
func redactPath(host, path string) string {
	segs := strings.Split(path, "/")
	host = strings.ToLower(host)
	for i, s := range segs {
		switch {
		case s == "":
		case strings.HasSuffix(host, "slack.com") && i >= 3 && segs[1] == "services":
			segs[i] = redactedMark
		case strings.Contains(host, "discord") && i >= 2 && segs[i-2] == "webhooks":
			segs[i] = redactedMark
		case i > 0 && strings.EqualFold(segs[i-1], "token"):
			segs[i] = redactedMark
		case botTokenRe.MatchString(s):
			segs[i] = botTokenRe.ReplaceAllString(s, redactedMark)
		case isOpaqueSecret(s):
			segs[i] = redactedMark
		}
	}
	return strings.Join(segs, "/")
}

// isOpaqueSecret reports whether a path segment looks like a generated secret:
// 20+ URL-safe characters with both a letter and a digit.
func isOpaqueSecret(s string) bool {
	if !opaqueSegmentRe.MatchString(s) {
		return false
	}
	var letter, digit bool
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digit = true
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			letter = true
		}
	}
	return letter && digit
}

// redactTarget redacts an alert channel target. An email address is not a
// credential and is kept; every other kind goes through redactValue.
func redactTarget(kind, target string) string {
	if strings.EqualFold(kind, "email") {
		return target
	}
	return redactValue(target)
}

// redactMonitor returns a redacted copy of m (nil stays nil).
func redactMonitor(m *sentryapi.Monitor) *sentryapi.Monitor {
	if m == nil {
		return nil
	}
	c := *m
	c.URL = redactValue(c.URL)
	c.Name = redactValue(c.Name)
	return &c
}

// redactMonitors returns a redacted copy of ms (nil stays nil).
func redactMonitors(ms []sentryapi.Monitor) []sentryapi.Monitor {
	if ms == nil {
		return nil
	}
	out := make([]sentryapi.Monitor, len(ms))
	for i, m := range ms {
		out[i] = *redactMonitor(&m)
	}
	return out
}

// redactStatusPage returns a redacted copy of p (nil stays nil).
func redactStatusPage(p *sentryapi.StatusPage) *sentryapi.StatusPage {
	if p == nil {
		return nil
	}
	c := *p
	c.URL = redactValue(c.URL)
	c.Name = redactValue(c.Name)
	return &c
}

// redactStatusPages returns a redacted copy of ps (nil stays nil).
func redactStatusPages(ps []sentryapi.StatusPage) []sentryapi.StatusPage {
	if ps == nil {
		return nil
	}
	out := make([]sentryapi.StatusPage, len(ps))
	for i, p := range ps {
		out[i] = *redactStatusPage(&p)
	}
	return out
}

// redactChannels returns a redacted copy of cs (nil stays nil).
func redactChannels(cs []sentryapi.AlertChannel) []sentryapi.AlertChannel {
	if cs == nil {
		return nil
	}
	out := make([]sentryapi.AlertChannel, len(cs))
	for i, c := range cs {
		c.Target = redactTarget(c.Kind, c.Target)
		out[i] = c
	}
	return out
}
