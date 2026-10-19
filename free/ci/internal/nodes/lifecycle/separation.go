package lifecycle

// Purpose: decide whether a user-mode Linux node separates a job from the agent (Epic D8): uid_separation
// is true only when an unprivileged user namespace works on the node. The probe itself is P7-TRUST-09's
// hygiene.ProbeUserns, run on the node by the agent; this file never repeats it.
// Inputs:  a runner bound to the node and the node's platform.
// Outputs: SeparationFacts; every failure to learn the answer is "not separated" (fail closed).
// Constraints: the agent subcommand `userns-probe` prints hygiene.Probe as JSON
//          (`{"available":true,"reason":"userns.ok"}`); an agent without it, a macOS node, any ssh error
//          or unreadable output all mean uid_separation false, accepts [owner] only, signing_eligible false.

import (
	"context"
	"encoding/json"
	"strings"
)

// SeparationFacts is what `nodes add` records in the node's separation block.
type SeparationFacts struct {
	UIDSeparation, UserNS bool
	// Reason is a stable key for the human output ("userns.ok", "not_linux", "probe_unavailable").
	Reason string
}

// SeparationProber learns the separation facts of a node. run executes one fixed argv on the node.
type SeparationProber interface {
	Detect(ctx context.Context, run func(ctx context.Context, argv ...string) (string, error), p Platform) SeparationFacts
}

// AgentSeparation asks the installed agent.
type AgentSeparation struct{}

// Detect implements SeparationProber.
func (AgentSeparation) Detect(ctx context.Context, run func(context.Context, ...string) (string, error), p Platform) SeparationFacts {
	if p.OS != "linux" {
		return SeparationFacts{Reason: "not_linux"}
	}
	out, err := run(ctx, agentPath, "userns-probe")
	if err != nil {
		return SeparationFacts{Reason: "probe_unavailable"}
	}
	var r struct {
		Available bool   `json:"available"`
		Reason    string `json:"reason"`
	}
	if json.Unmarshal([]byte(lastLine(out)), &r) != nil {
		return SeparationFacts{Reason: "probe_unreadable"}
	}
	if !r.Available {
		return SeparationFacts{Reason: keyOr(r.Reason, "userns.unavailable")}
	}
	return SeparationFacts{UIDSeparation: true, UserNS: true, Reason: keyOr(r.Reason, "userns.ok")}
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// keyOr returns v when it is a short reason key (letters, digits, dot, underscore), else def.
func keyOr(v, def string) string {
	if v == "" || len(v) > 48 {
		return def
	}
	for _, c := range v {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '_') {
			return def
		}
	}
	return v
}
