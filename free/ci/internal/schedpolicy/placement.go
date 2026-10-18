package schedpolicy

// Purpose: the scheduling policy keys of Epic D10, registered into
// P7-TRUST-27's loader from init() (the spend-enabling keys are P7-SCHED-17's).
// Inputs: none. Outputs: Keys, RegistrationErrors and the `sched.*` keys in
// policy.Default.
// Constraints: registration never panics (a refused spec is kept in
// RegistrationErrors and fails TestKeysRegistered); numeric keys use the
// no-limit default math.MaxInt64 so a lower scope can only tighten; the
// tightening rules are trust's, not re-implemented here.

import (
	"math"
	"slices"
	"sort"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// Owner is the owner recorded on every key this package registers.
const Owner = "sched"

// The key names, shared by the specs, the typed views and the flag values.
const (
	KeyPreset          = "sched.preset"
	KeyLocalOnly       = "sched.overrides.local_only"
	KeyLANOnly         = "sched.overrides.lan_only"
	KeyPrivateOnly     = "sched.overrides.private_only"
	KeyProvidersAllow  = "sched.overrides.providers.allow"
	KeyProvidersDeny   = "sched.overrides.providers.deny"
	KeyLabels          = "sched.overrides.labels"
	KeyRunner          = "sched.overrides.runner"
	KeyOS              = "sched.overrides.os"
	KeyArch            = "sched.overrides.arch"
	KeyMaxCostMicro    = "sched.overrides.max_cost_micro"
	KeyMaxQueue        = "sched.overrides.max_queue"
	KeyMinIsolation    = "sched.overrides.min_isolation"
	KeyLimitPipeline   = "sched.limits.pipeline"
	KeyLimitJobMatrix  = "sched.limits.job_matrix"
	KeyLimitProvider   = "sched.limits.provider.*"
	KeyLimitProject    = "sched.limits.project"
	KeyLimitUser       = "sched.limits.user"
	KeyLimitTeam       = "sched.limits.team"
	KeyLimitGlobal     = "sched.limits.global"
	KeyStarvationAfter = "sched.fairness.starvation_after"
	KeyQueueMaxDepth   = "sched.queue.max_depth"
)

// NoLimit is the registered default of every numeric key: nothing is capped.
const NoLimit int64 = math.MaxInt64

// DefaultStarvationAfterMs is the registered default of
// sched.fairness.starvation_after (Epic D9: 30 minutes).
const DefaultStarvationAfterMs = int64(30 * time.Minute / time.Millisecond)

// DefaultQueueMaxDepth is the registered default of sched.queue.max_depth
// (Epic D9: a project with more queued jobs than this is refused, E694).
const DefaultQueueMaxDepth int64 = 500

var registrationErrors []error

// RegistrationErrors returns what init() could not register; empty when every
// key is in the process-wide key set.
func RegistrationErrors() []error { return slices.Clone(registrationErrors) }

func init() {
	for _, spec := range Keys() {
		if err := policy.RegisterKey(spec); err != nil {
			registrationErrors = append(registrationErrors, err)
		}
	}
}

// PresetNames returns the closed set of presets (sched.Presets), sorted.
func PresetNames() []string {
	var names []string
	for name := range sched.Presets() {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// IsolationOrder is the order of sched.overrides.min_isolation, loosest first:
// the isolation values the scheduler ranks (trust.RankIsolation), without
// hosted-disposable, which ranks the same as ephemeral-vm.
func IsolationOrder() []string {
	var out []string
	for _, v := range model.EnumValues("Isolation") {
		if v != "hosted-disposable" {
			out = append(out, v)
		}
	}
	return out
}

// Keys returns the specs of every scheduling key.
func Keys() []policy.KeySpec {
	k := func(key string, kind policy.Tightening, def any, dec decoder) policy.KeySpec {
		return policy.KeySpec{Key: key, Owner: Owner, Kind: kind, Default: def, Decode: dec}
	}
	lower := func(key string) policy.KeySpec {
		return k(key, policy.TightenLower, NoLimit, positiveInt(key))
	}
	isolation := k(KeyMinIsolation, policy.TightenHigher, "process", stringOf(KeyMinIsolation, IsolationOrder()))
	isolation.Order = IsolationOrder()
	return []policy.KeySpec{
		k(KeyPreset, policy.TightenAny, "balanced", stringOf(KeyPreset, PresetNames())),
		k(KeyLocalOnly, policy.TightenOnlyOn, false, boolOf(KeyLocalOnly)),
		k(KeyLANOnly, policy.TightenOnlyOn, false, boolOf(KeyLANOnly)),
		k(KeyPrivateOnly, policy.TightenOnlyOn, false, boolOf(KeyPrivateOnly)),
		k(KeyProvidersAllow, policy.TightenSubset, nil, listOf(KeyProvidersAllow, true)),
		k(KeyProvidersDeny, policy.TightenSuperset, []string{}, listOf(KeyProvidersDeny, false)),
		k(KeyLabels, policy.TightenSuperset, []string{}, listOf(KeyLabels, false)),
		k(KeyRunner, policy.TightenFixed, nil, stringOf(KeyRunner, nil)),
		k(KeyOS, policy.TightenFixed, nil, stringOf(KeyOS, nil)),
		k(KeyArch, policy.TightenFixed, nil, stringOf(KeyArch, nil)),
		lower(KeyMaxCostMicro),
		k(KeyMaxQueue, policy.TightenLower, NoLimit, durationMs(KeyMaxQueue)),
		isolation,
		lower(KeyLimitPipeline),
		lower(KeyLimitJobMatrix),
		// Decode sees no key, so a provider limit error points at the family; the loader
		// prefixes the concrete key (`E672 sched.limits.provider.<id>: ...`).
		k(KeyLimitProvider, policy.TightenLower, NoLimit, positiveInt("sched.limits.provider")),
		lower(KeyLimitProject),
		lower(KeyLimitUser),
		lower(KeyLimitTeam),
		lower(KeyLimitGlobal),
		k(KeyStarvationAfter, policy.TightenLower, DefaultStarvationAfterMs, durationMs(KeyStarvationAfter)),
		k(KeyQueueMaxDepth, policy.TightenLower, DefaultQueueMaxDepth, positiveInt(KeyQueueMaxDepth)),
	}
}
