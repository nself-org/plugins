package schedpolicy

// Purpose: strict decoders for the spend keys (spend.go registers them):
// bounded money and durations, https URLs, host:port, absolute paths, sha256,
// per-provider objects (no repeated field) and the declared rate.
// Inputs: the raw JSON of one key. Outputs: the normalized value (int64,
// string, []string or map[string]any) or an E691 error with a JSON pointer.
// Constraints: pure; unknown fields, floats, negative money and values that
// would saturate over the largest paid timeout are refused; nothing is opened.

import (
	"bytes"
	"encoding/json"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

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

// money accepts a whole number of micro-units from lo up to MaxMoneyMicro.
func money(key string, lo int64) decoder { return boundedInt(key, lo, MaxMoneyMicro) }

// boundedDuration accepts a duration string of 1ms up to hi milliseconds; the value is whole ms.
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
		s := v.(string)
		u, perr := url.Parse(s)
		if perr != nil || strings.ContainsAny(s, "# \t\r\n\\") { // an empty fragment or a space still parses
			return nil, fail(key, "must be an https URL with a host and no userinfo, query string or fragment")
		}
		if port := u.Port(); port != "" {
			if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
				return nil, fail(key, "the port must be from 1 to 65535")
			}
		}
		if u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return nil, fail(key, "must be an https URL with a host and no userinfo, query string or fragment")
		}
		return v, nil
	}
}

// hostPort accepts host:port with a port from 1 to 65535 (the enrollment listener address).
func hostPort(key string) decoder {
	return func(raw json.RawMessage) (any, error) {
		v, err := stringOf(key, nil)(raw)
		if err != nil {
			return nil, err
		}
		host, port, serr := net.SplitHostPort(v.(string))
		p, perr := strconv.Atoi(port)
		if serr != nil || host == "" || strings.ContainsAny(host, " \t\r\n") || perr != nil || p < 1 || p > 65535 {
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

// sha256Hex accepts 64 hex characters and normalizes to lower case.
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
		parts, err := objectParts(key, raw)
		if err != nil {
			return nil, err
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

// objectParts splits a JSON object into its members and refuses a repeated
// member: a map decode keeps only the last one, which would hide the first.
func objectParts(key string, raw json.RawMessage) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	if _, err := d.Token(); err != nil {
		return nil, fail(key, "%v", err)
	}
	parts := map[string]json.RawMessage{}
	for d.More() {
		tok, err := d.Token()
		name, isName := tok.(string)
		if err != nil || !isName {
			return nil, fail(key, "malformed object")
		}
		var part json.RawMessage
		if err := d.Decode(&part); err != nil {
			return nil, fail(key+"."+name, "%v", err)
		}
		if _, dup := parts[name]; dup {
			return nil, fail(key+"."+name, "repeated field")
		}
		parts[name] = part
	}
	return parts, nil
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
		// Worst case: the largest paid timeout rounded up to whole billing increments,
		// then up to whole hours (minutes) so the bound never undershoots.
		billable := MaxPaidTimeoutCap
		if inc, _ := r["billing_increment"].(int64); inc > 0 {
			billable = (MaxPaidTimeoutCap + inc - 1) / inc * inc
		}
		worst, sat1 := sched.Mul(hour, (billable+3599999)/3600000)
		worstMin, sat2 := sched.Mul(minute, (billable+59999)/60000)
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
