package schedpolicy

// Purpose: the spend-enabling policy keys of Epic D10-D13 (economics.*,
// burst.*), registered with Spend: true so P7-TRUST-27's loader takes them from
// operator scope only (a value in the protected branch or the revision is
// ignored and recorded, never applied) and P7-TRUST-26's write path gates every
// write with roles.Authorize(admin). Also the typed view Spend and PaidAllowed.
// Inputs: raw JSON of one key; a policy.Resolved. Outputs: SpendKeys,
// ValidateSpendKey, Spend (SpendPolicy), PaidAllowed.
// Constraints: no I/O, no secret value (token_file is a path, never opened);
// every refusal is E691 with a JSON pointer; money is int64 micro-units and the
// products checked here go through sched.Mul/sched.Add. Everything paid is off
// by default. The family keys (`prefix.*`) match one path segment, so a
// per-provider value is one JSON object; provider-id rules (no paid GitHub
// class, arch names) run in ValidateSpendKey and Spend because Decode sees no key.

import (
	"encoding/json"
	"math"
	"regexp"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/policy"
)

// The spend key names.
const (
	KeyCurrency        = "economics.currency"
	KeyRates           = "economics.rates.*"
	KeyCeilings        = "economics.ceilings.*"
	KeyBudgetJob       = "economics.budgets.job"
	KeyBudgetPipeline  = "economics.budgets.pipeline"
	KeyBudgetDay       = "economics.budgets.day"
	KeyBudgetMonth     = "economics.budgets.month"
	KeyBudgetProvider  = "economics.budgets.provider.*"
	KeyApproval        = "economics.approval_threshold_micro"
	KeyPaidEnabled     = "economics.paid_enabled"
	KeyMaxPaidTimeout  = "economics.max_paid_timeout"
	KeyMargin          = "economics.margin_micro"
	KeyEconProviders   = "economics.providers.*"
	KeyBurstProviders  = "burst.providers.*"
	KeyBurstConcurrent = "burst.max_concurrent"
	KeyBurstMargin     = "burst.margin"
	KeyBurstTrust      = "burst.trust"
	KeyCloudType       = "burst.cloudvm.type"
	KeyCloudLocation   = "burst.cloudvm.location"
	KeyCloudImage      = "burst.cloudvm.image"
	KeyCloudAgent      = "burst.cloudvm.agent.*"
	KeyCloudEnrollAddr = "burst.cloudvm.enroll_addr"
	KeyCloudEnrollURL  = "burst.cloudvm.enroll_url"
	KeyCloudTokenFile  = "burst.cloudvm.token_file"
)

// Registered defaults and bounds. MaxMoneyMicro (a hundredth of int64) leaves
// headroom for sums, yet a rate near it still saturates over MaxPaidTimeoutCap
// (the largest timeout a rate is multiplied by), so the product check is live.
const (
	DefaultCurrency         = "USD"
	DefaultMaxPaidTimeoutMs = int64(2 * 3600 * 1000)
	DefaultBurstConcurrent  = int64(2)
	DefaultBurstMarginMs    = int64(60 * 1000)
	MaxMoneyMicro           = int64(math.MaxInt64 / 100)
	MaxPaidTimeoutCap       = int64(168 * 3600 * 1000)
	maxIncrementMs          = int64(24 * 3600 * 1000)
	maxBurstConcurrent      = 64
	paidMinutesAtCap        = 168 * 60
	paidHoursAtCap          = 168
	costClassMetered        = "metered"
	costClassEphemeral      = "ephemeral"
	defaultBurstIsolation   = "ephemeral-vm"
	defaultBurstPrivacyZone = "provider-allowlist"
	cloudArchAmd64          = "amd64"
	cloudArm                = "arm64"
)

var (
	idRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
	nameRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)
	shaRe    = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
	currency = regexp.MustCompile(`^[A-Z]{3}$`)
	// burstTrustDefaults is Epic D13: accepts owner/collaborator/internal, no secrets.
	burstAccepts = []string{"owner", "collaborator", "internal"}
	costClasses  = []string{"owned", "included", costClassMetered, costClassEphemeral}
)

// SpendKeys returns the specs of every spend-enabling key, all Spend: true and
// TightenAny: money has no "stricter" order, the protection is the scope
// (operator only) and the admin-authorized write.
func SpendKeys() []policy.KeySpec {
	k := func(key string, def any, dec decoder) policy.KeySpec {
		return policy.KeySpec{Key: key, Owner: Owner, Kind: policy.TightenAny, Spend: true, Default: def, Decode: dec}
	}
	fam := func(key string) string { return strings.TrimSuffix(key, ".*") }
	agent := fam(KeyCloudAgent)
	return []policy.KeySpec{
		k(KeyCurrency, DefaultCurrency, textOf(KeyCurrency, currency, "a three-letter upper-case currency code")),
		k(KeyRates, nil, rateOf(KeyRates)),
		k(KeyCeilings, nil, money(fam(KeyCeilings), 0)),
		k(KeyBudgetJob, nil, money(KeyBudgetJob, 0)),
		k(KeyBudgetPipeline, nil, money(KeyBudgetPipeline, 0)),
		k(KeyBudgetDay, nil, money(KeyBudgetDay, 0)),
		k(KeyBudgetMonth, nil, money(KeyBudgetMonth, 0)),
		k(KeyBudgetProvider, nil, money(fam(KeyBudgetProvider), 0)),
		k(KeyApproval, nil, money(KeyApproval, 1)),
		k(KeyPaidEnabled, false, boolOf(KeyPaidEnabled)),
		k(KeyMaxPaidTimeout, DefaultMaxPaidTimeoutMs, boundedDuration(KeyMaxPaidTimeout, MaxPaidTimeoutCap)),
		k(KeyMargin, int64(0), money(KeyMargin, 0)),
		k(KeyEconProviders, nil, objectOf(fam(KeyEconProviders), map[string]decoder{"owner_coordinator": textOf(fam(KeyEconProviders)+".owner_coordinator", nameRe, "a coordinator id")}, "owner_coordinator")),
		k(KeyBurstProviders, nil, objectOf(fam(KeyBurstProviders), map[string]decoder{"enabled": boolOf(fam(KeyBurstProviders) + ".enabled")}, "enabled")),
		k(KeyBurstConcurrent, DefaultBurstConcurrent, boundedInt(KeyBurstConcurrent, 1, maxBurstConcurrent)),
		k(KeyBurstMargin, DefaultBurstMarginMs, boundedDuration(KeyBurstMargin, MaxPaidTimeoutCap)),
		k(KeyBurstTrust, nil, burstTrustOf(KeyBurstTrust)),
		k(KeyCloudType, nil, textOf(KeyCloudType, nameRe, "a server type name")),
		k(KeyCloudLocation, nil, textOf(KeyCloudLocation, nameRe, "a location name")),
		k(KeyCloudImage, nil, textOf(KeyCloudImage, nameRe, "an image name")),
		k(KeyCloudAgent, nil, objectOf(agent, map[string]decoder{"url": httpsURL(agent + ".url"), "sha256": sha256Hex(agent + ".sha256")}, "url", "sha256")),
		k(KeyCloudEnrollAddr, nil, hostPort(KeyCloudEnrollAddr)),
		k(KeyCloudEnrollURL, nil, httpsURL(KeyCloudEnrollURL)),
		k(KeyCloudTokenFile, nil, absPath(KeyCloudTokenFile)),
	}
}

func init() {
	for _, spec := range SpendKeys() {
		if err := policy.RegisterKey(spec); err != nil {
			registrationErrors = append(registrationErrors, err)
		}
	}
}

// ValidateSpendKey is the key-aware check for a write of one spend key: the
// registered Decode plus the provider-id rules Decode cannot see (a GitHub
// provider is never a paid class, a burst provider is never GitHub, agent
// arch names). P7-TRUST-26's write path calls it before storing a value.
func ValidateSpendKey(key string, raw json.RawMessage) error {
	spec, ok := policy.Default.Find(key)
	if !ok || !spec.Spend {
		return fail(key, "not a registered spend key")
	}
	v, err := spec.Decode(raw)
	if err != nil {
		return err
	}
	return checkFamily(key, v)
}

func checkFamily(key string, v any) error {
	i := strings.LastIndex(key, ".")
	prefix, id := key[:i+1], key[i+1:]
	if prefix+"*" != KeyRates && prefix+"*" != KeyEconProviders && prefix+"*" != KeyBurstProviders && prefix+"*" != KeyCloudAgent &&
		prefix+"*" != KeyCeilings && prefix+"*" != KeyBudgetProvider {
		return nil
	}
	if !idRe.MatchString(id) {
		return fail(key, "provider id %q is not a lower-case name", id)
	}
	github := strings.HasPrefix(id, "github")
	switch prefix + "*" {
	case KeyRates:
		if class, _ := v.(map[string]any)["class"].(string); github && isPaidClass(class) {
			return fail(key, "a GitHub provider is never class %s (APC P8: GitHub-hosted is included only)", class)
		}
	case KeyBurstProviders:
		if github {
			return fail(key, "a GitHub provider is never a burst provider")
		}
	case KeyCloudAgent:
		if id != cloudArchAmd64 && id != cloudArm {
			return fail(key, "arch %q is not %s or %s", id, cloudArchAmd64, cloudArm)
		}
	}
	return nil
}
