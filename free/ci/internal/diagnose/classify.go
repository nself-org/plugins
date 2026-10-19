package diagnose

// Purpose: classify one failed attempt as code | infra | timeout | cancelled |
// lost, name the signature that decided it, and say what to do next
// (P7-CI-40, cap:ci.diagnostics). Also cut the bounded evidence excerpt.
// Inputs:  an Input: exit status, the executor's coarse class and error, the
// admission or prepare reason and the redacted output tail.
// Outputs: a Diagnosis: class, signature id, retryable, remediation.
// Constraints: pure (no os, exec, net or database); order is cancelled, lost,
// timeout, signature, executor error, then code; an unknown failure is code;
// only infra with a known signature is retryable (an executor error nobody
// recognises may be a deterministic refusal), and the caller still applies the job's own limits
// (idempotent, not release, retry.infra_max).

import (
	"strings"
	"unicode/utf8"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// ExcerptMax is the largest excerpt evidence carries (contract:ci.evidence).
const ExcerptMax = 4096

// Input is everything the classifier reads about a failed attempt.
type Input struct {
	ExecClass model.FailureClass // the executor's coarse class: code, infra, timeout, cancelled; "" when unknown
	ExitCode  int
	Err       string // the executor, admission or prepare error text, if any
	Reason    string // "resource_limit" when a container hit its memory or process limit
	Tail      string // the end of the redacted output
	Lost      bool   // the lease expired or the runner was revoked
	Cancelled bool
	TimedOut  bool
}

// Diagnosis is the verdict on one failed attempt.
type Diagnosis struct {
	Class       model.FailureClass
	Signature   string
	Retryable   bool // infra only
	Remediation string
}

// Classify decides the failure class of a failed attempt.
func Classify(in Input) Diagnosis {
	switch {
	case in.Cancelled || in.ExecClass == "cancelled":
		return named(SigCancelled, "cancelled")
	case in.Lost || in.ExecClass == "lost":
		return named(SigLost, "lost")
	case in.TimedOut || in.ExecClass == "timeout":
		return named(SigTimeout, "timeout") // a timeout kill is SIGKILL too: judged before any signature
	}
	if in.Reason == "resource_limit" {
		return infra(signatures[0])
	}
	tail := in.Tail
	if len(tail) > TailWindow {
		tail = tail[len(tail)-TailWindow:]
	}
	for _, s := range signatures {
		if s.matches(in, tail) {
			return infra(s)
		}
	}
	if in.ExecClass == "infra" {
		return Diagnosis{Class: "infra", Signature: SigExecutor, Remediation: classRemediation[SigExecutor]} // not proven transient: never retried
	}
	return named(SigCode, "code")
}

// ForClass is the diagnosis of a failure the coordinator already classified
// and no signature explains (a store error, an admission refusal).
func ForClass(c model.FailureClass) Diagnosis {
	switch c {
	case "infra":
		return named(SigExecutor, "infra")
	case "timeout", "cancelled", "lost":
		return named(string(c), c)
	}
	return named(SigCode, "code")
}

func (s signature) matches(in Input, tail string) bool {
	for _, x := range s.exits {
		if in.ExitCode == x {
			return true
		}
	}
	if in.Err != "" && s.errText != nil && s.errText.MatchString(in.Err) {
		return true
	}
	return in.ExitCode != 0 && s.output != nil && s.output.MatchString(tail)
}

func infra(s signature) Diagnosis {
	return Diagnosis{Class: "infra", Signature: s.id, Retryable: true, Remediation: s.remediation}
}

func named(sig string, class model.FailureClass) Diagnosis {
	return Diagnosis{Class: class, Signature: sig, Remediation: classRemediation[sig]}
}

// Excerpt returns the last max bytes of redacted output, starting at a line
// and a rune boundary, never longer than max bytes. max < 1 means ExcerptMax.
func Excerpt(text string, max int) string {
	if max < 1 || max > ExcerptMax {
		max = ExcerptMax
	}
	text = strings.TrimRight(text, "\r\n ")
	if len(text) <= max {
		return text
	}
	cut := text[len(text)-max:]
	for len(cut) > 0 && !utf8.RuneStart(cut[0]) {
		cut = cut[1:]
	}
	if i := strings.IndexByte(cut, '\n'); i >= 0 && i < len(cut)/4 {
		cut = cut[i+1:] // start at a whole line when that costs under a quarter
	}
	return cut
}
