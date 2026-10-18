package exec

// Purpose: the default output Redactor: masks secret-looking values before a
// chunk reaches the stream, the tail or the evidence excerpt.
// Inputs:  output chunks in arrival order.
// Outputs: the same text with secret values replaced by [redacted].
// Constraints: line-buffered, so a secret split across chunks is still
// caught; a line with no newline is held until Flush or maxHold bytes, then
// cut at whitespace; a private key block is dropped whole. Pattern-based:
// it cannot know a value that matches no pattern (declared secrets are the
// secret source's job).

import (
	"bytes"
	"regexp"
)

const (
	maskText = "[redacted]"
	// maxHold bounds the partial line kept between chunks.
	maxHold = 4096
	// holdBack stays unemitted when a long line is cut, so a token near the cut is whole.
	holdBack = 256
)

var (
	namedSecret = regexp.MustCompile(`(?i)([A-Za-z0-9_.-]*(?:TOKEN|SECRET|PASSWORD|PASSWD|PASSPHRASE|API[_-]?KEY|PRIVATE[_-]?KEY|ACCESS[_-]?KEY|CREDENTIAL)[A-Za-z0-9_.-]*)(["']?\s*[=:]\s*)("[^"\n]*"|'[^'\n]*'|[^\s"']+)`)
	bearer      = regexp.MustCompile(`(?i)\b(bearer|basic)(\s+)[A-Za-z0-9._~+/=-]{8,}`)
	urlUser     = regexp.MustCompile(`(://[^/\s:@]+:)[^/\s@]+(@)`)
	tokenShape  = regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|\bAKIA[0-9A-Z]{16}\b|xox[abprs]-[A-Za-z0-9-]{10,}|\bsk-[A-Za-z0-9_-]{20,}|eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`)
	pemBegin    = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)
	pemEnd      = regexp.MustCompile(`-----END [A-Z ]*PRIVATE KEY-----`)
)

// NewOutputRedactor returns fresh streaming state for one attempt.
func NewOutputRedactor() Redactor { return &lineRedactor{} }

type lineRedactor struct {
	held  []byte
	inPEM bool
}

func (r *lineRedactor) Redact(p []byte) []byte {
	r.held = append(r.held, p...)
	var out []byte
	for {
		i := bytes.IndexAny(r.held, "\n\r")
		if i < 0 {
			break
		}
		out = append(out, r.line(r.held[:i+1])...)
		r.held = r.held[i+1:]
	}
	if len(r.held) > maxHold {
		cut := bytes.LastIndexAny(r.held[:len(r.held)-holdBack], " \t")
		if cut < 0 {
			cut = len(r.held) - holdBack
		}
		out = append(out, r.line(r.held[:cut+1])...)
		r.held = r.held[cut+1:]
	}
	r.held = append([]byte(nil), r.held...)
	return out
}

func (r *lineRedactor) Flush() []byte {
	out := r.line(r.held)
	r.held, r.inPEM = nil, false
	return out
}

// line masks one line, including its terminator.
func (r *lineRedactor) line(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	if r.inPEM {
		if pemEnd.Match(b) {
			r.inPEM = false
		}
		return nil
	}
	if loc := pemBegin.FindIndex(b); loc != nil {
		r.inPEM = !pemEnd.Match(b[loc[1]:])
		return append(append(append([]byte(nil), b[:loc[0]]...), "[redacted private key]"...), eol(b)...)
	}
	b = namedSecret.ReplaceAll(b, []byte("${1}${2}"+maskText))
	b = bearer.ReplaceAll(b, []byte("${1}${2}"+maskText))
	b = urlUser.ReplaceAll(b, []byte("${1}"+maskText+"${2}"))
	return tokenShape.ReplaceAll(b, []byte(maskText))
}

func eol(b []byte) []byte {
	if n := len(b); n > 0 && (b[n-1] == '\n' || b[n-1] == '\r') {
		return b[n-1:]
	}
	return nil
}
