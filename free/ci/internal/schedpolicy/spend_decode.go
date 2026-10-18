package schedpolicy

// Purpose: strict decoders for the spend keys (spend.go registers them):
// bounded money and durations, https URLs, host:port, absolute paths, sha256,
// per-provider objects, the declared rate and burst.trust.
// Inputs: the raw JSON of one key. Outputs: the normalized value (int64,
// string, []string or map[string]any) or an E691 error with a JSON pointer.
// Constraints: pure; unknown fields, floats, negative money and values that
// would saturate over the largest paid timeout are refused; nothing is opened.

import (
	"encoding/json"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// boundedInt accepts a whole number in [lo, hi].
func boundedInt(key string, lo, hi int64) decoder {
	return func(raw json.RawMessage) (any, error) {
		v, err := decodeJSON(key, raw)
		if err != nil {
			return nil, err
		}
		n, ok := v.(json.Number)
		if !ok {
			return nil, fail(key, "expected a whole number")
		}
		i, err := n.Int64()
		if err != nil || i < lo || i > hi {
			return nil, fail(key, "%s is outside %d..%d", n, lo, hi)
		}
		return i, nil
	}
}

func money(key string, lo int64) decoder { return boundedInt(key, lo, MaxMoneyMicro) }

func boundedDuration(key string, hi int64) decoder {
	return func(raw json.RawMessage) (any, error) {
		v, err := durationMs(key)(raw)
		if err == nil && v.(int64) > hi {
			return nil, fail(key, "duration is above the %dms limit", hi)
		}
		return v, err
	}
}

// textOf accepts a string matching re.
func textOf(key string, re *regexp.Regexp, what string) decoder {
	return func(raw json.RawMessage) (any, error) {
		v, err := stringOf(key, nil)(raw)
		if err != nil {
			return nil, err
		}
		if !re.MatchString(v.(string)) {
			return nil, fail(key, "%q is not %s", v, what)
		}
		return v, nil
	}
}

// httpsURL accepts an https URL with a host, no userinfo, no query, no fragment.
func httpsURL(key string) decoder {
	return func(raw json.RawMessage) (any, error) {
		v, err := stringOf(key, nil)(raw)
		if err != nil {
			return nil, err
		}
		u, perr := url.Parse(v.(string))
		if perr != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return nil, fail(key, "must be an https URL with a host and no userinfo, query string or fragment")
		}
		return v, nil
	}
}

func hostPort(key string) decoder {
	return func(raw json.RawMessage) (any, error) {
		v, err := stringOf(key, nil)(raw)
		if err != nil {
			return nil, err
		}
		host, port, serr := net.SplitHostPort(v.(string))
		p, perr := strconv.Atoi(port)
		if serr != nil || host == "" || perr != nil || p < 1 || p > 65535 {
			return nil, fail(key, "must be host:port with a port from 1 to 65535")
		}
		return v, nil
	}
}

// absPath accepts an absolute, clean path; the file is never opened here.
func absPath(key string) decoder {
	return func(raw json.RawMessage) (any, error) {
		v, err := stringOf(key, nil)(raw)
		if err != nil {
			return nil, err
		}
		if s := v.(string); !filepath.IsAbs(s) || filepath.Clean(s) != s || strings.ContainsRune(s, 0) {
			return nil, fail(key, "must be an absolute, clean file path")
		}
		return v, nil
	}
}

func sha256Hex(key string) decoder {
	return func(raw json.RawMessage) (any, error) {
		v, err := textOf(key, shaRe, "64 hex characters")(raw)
		if err != nil {
			return nil, err
		}
		return strings.ToLower(v.(string)), nil
	}
}

// objectOf accepts a JSON object whose fields all have a decoder (unknown
// fields are refused) and names the required ones.
func objectOf(key string, fields map[string]decoder, required ...string) decoder {
	return func(raw json.RawMessage) (any, error) {
		if v, err := decodeJSON(key, raw); err != nil {
			return nil, err
		} else if _, ok := v.(map[string]any); !ok {
			return nil, fail(key, "expected an object")
		}
		var parts map[string]json.RawMessage
		if err := json.Unmarshal(raw, &parts); err != nil {
			return nil, fail(key, "%v", err)
		}
		out := map[string]any{}
		for name, part := range parts {
			dec, ok := fields[name]
			if !ok {
				return nil, fail(key+"."+name, "unknown field")
			}
			v, err := dec(part)
			if err != nil {
				return nil, err
			}
			out[name] = v
		}
		for _, name := range required {
			if _, ok := out[name]; !ok {
				return nil, fail(key+"."+name, "required")
			}
		}
		return out, nil
	}
}

// rateOf is the declared price of one provider (D11): one of
// rate_per_hour_micro / rate_per_minute_micro, billing_increment, minimum
// charge and extras. A paid class needs a positive rate and an increment, and
// no product over the largest paid timeout may saturate.
func rateOf(key string) decoder {
	base := strings.TrimSuffix(key, ".*")
	obj := objectOf(base, map[string]decoder{
		"class":                 stringOf(base+".class", costClasses),
		"rate_per_hour_micro":   money(base+".rate_per_hour_micro", 0),
		"rate_per_minute_micro": money(base+".rate_per_minute_micro", 0),
		"billing_increment":     boundedDuration(base+".billing_increment", maxIncrementMs),
		"minimum_charge_micro":  money(base+".minimum_charge_micro", 0),
		"extras_micro":          money(base+".extras_micro", 0),
	})
	return func(raw json.RawMessage) (any, error) {
		v, err := obj(raw)
		if err != nil {
			return nil, err
		}
		r := v.(map[string]any)
		hour, hasHour := r["rate_per_hour_micro"].(int64)
		minute, hasMinute := r["rate_per_minute_micro"].(int64)
		if hasHour && hasMinute {
			return nil, fail(base, "declare rate_per_hour_micro or rate_per_minute_micro, not both")
		}
		if r["class"] == nil {
			r["class"] = costClassMetered // a declared price is a paid class unless the owner says otherwise
		}
		if isPaidClass(r["class"].(string)) && (hour+minute < 1 || r["billing_increment"] == nil) {
			return nil, fail(base, "a paid class needs a positive rate and a billing_increment")
		}
		worst, sat1 := sched.Mul(hour, paidHoursAtCap)
		worstMin, sat2 := sched.Mul(minute, paidMinutesAtCap)
		worst, sat3 := sched.Add(worst, worstMin)
		for _, name := range []string{"minimum_charge_micro", "extras_micro"} {
			n, _ := r[name].(int64)
			var sat bool
			if worst, sat = sched.Add(worst, n); sat {
				sat3 = true
			}
		}
		if sat1 || sat2 || sat3 {
			return nil, fail(base, "the rate over the largest paid timeout overflows")
		}
		return r, nil
	}
}

func isPaidClass(class string) bool { return class == costClassMetered || class == costClassEphemeral }

// burstTrustOf decodes burst.trust. Missing fields take the D13 defaults; a
// value wider than the defaults must name all four fields (E691 otherwise).
func burstTrustOf(key string) decoder {
	trustClasses, secrets, zones := model.EnumValues("TrustClass"), model.EnumValues("SecretClass"), model.EnumValues("PrivacyZone")
	obj := objectOf(key, map[string]decoder{
		"accepts":      listOf(key+".accepts", true),
		"secrets":      listOf(key+".secrets", true),
		"isolation":    stringOf(key+".isolation", IsolationOrder()),
		"privacy_zone": stringOf(key+".privacy_zone", zones),
	})
	return func(raw json.RawMessage) (any, error) {
		v, err := obj(raw)
		if err != nil {
			return nil, err
		}
		t := v.(map[string]any)
		explicit := len(t)
		for name, allowed := range map[string][]string{"accepts": trustClasses, "secrets": secrets} {
			for _, item := range asStrings(t[name]) {
				if !slices.Contains(allowed, item) {
					return nil, fail(key+"."+name, "unknown value %q (one of %v)", item, allowed)
				}
			}
		}
		set := func(name string, def any) { t[name] = firstNonNil(t[name], def) }
		set("accepts", slices.Clone(burstAccepts))
		set("secrets", []string{"none"})
		set("isolation", defaultBurstIsolation)
		set("privacy_zone", defaultBurstPrivacyZone)
		if explicit < 4 && burstTrustWider(t) {
			return nil, fail(key, "wider than the defaults (accepts %v, secrets none, %s, %s): name all four fields", burstAccepts, defaultBurstIsolation, defaultBurstPrivacyZone)
		}
		return t, nil
	}
}

func firstNonNil(a, b any) any {
	if a != nil {
		return a
	}
	return b
}

func burstTrustWider(t map[string]any) bool {
	for _, c := range asStrings(t["accepts"]) {
		if !slices.Contains(burstAccepts, c) {
			return true
		}
	}
	return !slices.Equal(asStrings(t["secrets"]), []string{"none"}) || t["isolation"] != defaultBurstIsolation || t["privacy_zone"] == "hosted-allowed"
}

func asStrings(v any) []string {
	s, _ := v.([]string)
	return s
}
