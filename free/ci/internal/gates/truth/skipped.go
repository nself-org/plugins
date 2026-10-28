package truth

// Purpose: bounded-file skip findings, text splitting and UTF-16 host-token decoding.
// Inputs: raw local bytes and compiled rules. Outputs: lines or a finding.
// Constraints: skipped prose fails closed; dead-host matches never expose the token.

import (
	"bytes"
	"strings"
	"unicode/utf16"
)

// skippedFinding fails closed when applicable rules cannot inspect a file.
func skippedFinding(rel string, c compiled, res *Result) {
	shown := rel
	if len(c.hostHits(rel)) > 0 {
		shown = redacted
	}
	c = c.forFile(rel)
	rules := append(append(append([]*Rule{}, c.claims...), c.literals...), c.terms...)
	for _, rs := range c.hosts {
		rules = append(rules, rs...)
	}
	if len(rules) == 0 {
		return
	}
	res.Findings = append(res.Findings, Finding{RuleID: "truth.unscanned-file", Kind: rules[0].Kind, Path: shown, Line: 1,
		Message: "an in-scope file cannot be fully checked; use UTF-8 text under the scan limit or declare a reasoned path exemption"})
}

// splitLines cuts raw at "\n" and drops one trailing "\r" per line.
func splitLines(raw []byte) []string {
	parts := bytes.Split(raw, []byte{'\n'})
	lines := make([]string, len(parts))
	for i, b := range parts {
		lines[i] = strings.Map(func(r rune) rune {
			if isZeroWidth(r) {
				return -1
			}
			return r
		}, strings.TrimSuffix(string(b), "\r"))
	}
	return lines
}

// skippedLines turns a skipped file into lines for the dead-host scan: UTF-16
// is decoded, a NUL byte ends a token like a newline would.
func skippedLines(raw []byte) []string {
	if isUTF16(raw) {
		raw = []byte(string(decodeUTF16(raw)))
	}
	return splitLines(bytes.ReplaceAll(raw, []byte{0}, []byte{' '}))
}

// isUTF16 reports a UTF-16 byte order mark.
func isUTF16(raw []byte) bool {
	return len(raw) >= 2 && (raw[0] == 0xFF && raw[1] == 0xFE || raw[0] == 0xFE && raw[1] == 0xFF)
}

func decodeUTF16(raw []byte) []rune {
	le := raw[0] == 0xFF
	u := make([]uint16, 0, len(raw)/2)
	for i := 2; i+1 < len(raw); i += 2 {
		if le {
			u = append(u, uint16(raw[i])|uint16(raw[i+1])<<8)
		} else {
			u = append(u, uint16(raw[i+1])|uint16(raw[i])<<8)
		}
	}
	return utf16.Decode(u)
}

// scanSkippedHosts also checks UTF-16 without a BOM in either byte order.
func scanSkippedHosts(rel string, raw []byte, rs RuleSet, c compiled, facts map[string]string, res *Result) {
	hosts := c.hostsOnly().forFile(rel)
	if len(hosts.hosts) == 0 {
		return
	}
	start := len(res.Findings)
	scanLines(rel, skippedLines(raw), rs, hosts, facts, res)
	if !isUTF16(raw) && bytes.IndexByte(raw, 0) >= 0 {
		for _, bom := range [][]byte{{0xff, 0xfe}, {0xfe, 0xff}} {
			wide := append(bom, raw...)
			scanLines(rel, skippedLines(wide), rs, hosts, facts, res)
		}
	}
	seen := map[Finding]bool{}
	out := res.Findings[:start]
	for _, f := range res.Findings[start:] {
		if !seen[f] {
			out = append(out, f)
			seen[f] = true
		}
	}
	res.Findings = out
}
