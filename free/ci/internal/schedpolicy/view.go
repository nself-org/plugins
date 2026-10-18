package schedpolicy

// Purpose: the typed views of the effective policy (Epic D10): Placement is
// what the world builder hands to sched.Place; Queue is what the dispatcher
// reads for fairness; ProviderLimits is the per-provider part of the limits.
// Inputs: *policy.Effective from the P7-TRUST-27 resolver.
// Outputs: sched.Policy, QueuePolicy and a provider-limit map.
// Constraints: pure; a value of the wrong type is an E691 error (never a
// silent default); the registered no-limit default becomes the scheduler's 0;
// a nil Effective is an error, because falling back to defaults would drop
// every override.

import (
	"math"
	"slices"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// QueuePolicy is the fairness and queue part of the policy. Zero MaxDepth
// means no cap.
type QueuePolicy struct {
	StarvationAfterMs int64
	MaxDepth          int64
}

type reader struct {
	eff *policy.Effective
	err error
}

// optional lists the keys with no registered default: absent means unset.
var optional = map[string]bool{KeyProvidersAllow: true, KeyRunner: true, KeyOS: true, KeyArch: true}

// value returns the effective value of key. A key that has a registered
// default is always present in a merged policy, so a missing one means the
// keys were never registered; that is an error, never "no limit".
func (r *reader) value(key string) (any, bool) {
	if r.err != nil {
		return nil, false
	}
	v, _, ok := r.eff.Value(key)
	if !ok && !optional[key] {
		r.err = fail(key, "missing from the effective policy (are the sched keys registered?)")
	}
	return v, ok
}

func (r *reader) bad(key, want string, v any) {
	if r.err == nil {
		r.err = fail(key, "effective value %v is not %s", v, want)
	}
}

func (r *reader) boolean(key string) bool {
	v, ok := r.value(key)
	if !ok {
		return false
	}
	b, isBool := v.(bool)
	if !isBool {
		r.bad(key, "a boolean", v)
	}
	return b
}

func (r *reader) text(key string) string {
	v, ok := r.value(key)
	if !ok {
		return ""
	}
	s, isStr := v.(string)
	if !isStr {
		r.bad(key, "a string", v)
	}
	return s
}

func (r *reader) list(key string) []string {
	v, ok := r.value(key)
	if !ok {
		return nil
	}
	switch xs := v.(type) {
	case []string:
		return slices.Clone(xs)
	case []any:
		out := make([]string, 0, len(xs))
		for _, x := range xs {
			s, isStr := x.(string)
			if !isStr {
				r.bad(key, "a list of names", v)
				return nil
			}
			out = append(out, s)
		}
		return out
	}
	r.bad(key, "a list of names", v)
	return nil
}

// number returns the value as int64; the no-limit default is 0.
func (r *reader) number(key string) int64 {
	v, ok := r.value(key)
	if !ok {
		return 0
	}
	n, isInt := trust.IntValue(v)
	if !isInt || n < 1 {
		r.bad(key, "a positive whole number", v)
		return 0
	}
	if n == NoLimit {
		return 0
	}
	return n
}

// limit narrows a number to the scheduler's int limit.
func limit(n int64) int {
	if n > math.MaxInt {
		return math.MaxInt
	}
	return int(n)
}

// Placement is the typed scheduling policy of one effective policy. The
// scheduler reads 0 as "no limit", so the no-limit default maps to 0.
// Per-provider limits do not fit sched.Limits (one number); ProviderLimits
// returns them.
func Placement(eff *policy.Effective) (sched.Policy, error) {
	if eff == nil {
		return sched.Policy{}, fail("sched", "no effective policy")
	}
	r := &reader{eff: eff}
	p := sched.Policy{Preset: r.text(KeyPreset), Trust: *eff}
	p.Overrides = sched.Overrides{
		LocalOnly: r.boolean(KeyLocalOnly), LANOnly: r.boolean(KeyLANOnly), PrivateOnly: r.boolean(KeyPrivateOnly),
		ProvidersAllow: r.list(KeyProvidersAllow), ProvidersDeny: r.list(KeyProvidersDeny), Labels: r.list(KeyLabels),
		Runner: r.text(KeyRunner), OS: r.text(KeyOS), Arch: r.text(KeyArch),
		MaxCostMicro: r.number(KeyMaxCostMicro), MaxQueueMs: r.number(KeyMaxQueue),
		MinIsolation: model.Isolation(r.text(KeyMinIsolation)),
	}
	p.Limits = sched.Limits{
		Pipeline: limit(r.number(KeyLimitPipeline)), Job: limit(r.number(KeyLimitJobMatrix)),
		Project: limit(r.number(KeyLimitProject)), User: limit(r.number(KeyLimitUser)),
		Team: limit(r.number(KeyLimitTeam)), Global: limit(r.number(KeyLimitGlobal)),
	}
	if r.err != nil {
		return sched.Policy{}, r.err
	}
	return p, nil
}

// Queue returns the fairness and queue-depth part of the policy.
func Queue(eff *policy.Effective) (QueuePolicy, error) {
	if eff == nil {
		return QueuePolicy{}, fail("sched", "no effective policy")
	}
	r := &reader{eff: eff}
	q := QueuePolicy{StarvationAfterMs: r.number(KeyStarvationAfter), MaxDepth: r.number(KeyQueueMaxDepth)}
	if r.err != nil {
		return QueuePolicy{}, r.err
	}
	return q, nil
}

// providerPrefix is the part of KeyLimitProvider before the provider id.
var providerPrefix = strings.TrimSuffix(KeyLimitProvider, "*")

// ProviderLimits returns the limit of each provider that has one, by provider
// id (a provider with no key is not capped). It reads the keys the effective
// policy carries, so it needs the full key list from Resolved.Values.
func ProviderLimits(values map[string]any) (map[string]int, error) {
	out := map[string]int{}
	for key, v := range values {
		id, found := strings.CutPrefix(key, providerPrefix)
		if !found || id == "" || strings.Contains(id, ".") {
			continue
		}
		n, ok := trust.IntValue(v)
		if !ok || n < 1 {
			return nil, fail(key, "effective value %v is not a positive whole number", v)
		}
		if n != NoLimit {
			out[id] = limit(n)
		}
	}
	return out, nil
}
