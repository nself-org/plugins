package labels

// Purpose: syntax rules for operator-assigned node labels (P7-NODE-30) and the conversion between
// the `key=value` form used by placement and the store rows.
// Inputs:  label strings from the operator (`key` or `key=value`) or store rows.
// Outputs: Parse, Validate, Strings.
// Constraints: keys match ^[a-z][a-z0-9_.-]{0,62}$, values match ^[A-Za-z0-9_.-]{0,63}$, the
//          `nself.` key prefix is reserved for the fabric, and a node carries at most MaxLabels.

import (
	"regexp"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// MaxLabels is the most labels one node may carry.
const MaxLabels = 32

// ReservedPrefix is the key prefix only the fabric may assign.
const ReservedPrefix = "nself."

var (
	keyRE   = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,62}$`)
	valueRE = regexp.MustCompile(`^[A-Za-z0-9_.-]{0,63}$`)
)

// Parse turns `key` or `key=value` strings into a validated label set ordered by key.
func Parse(in []string) ([]store.NodeLabel, error) {
	out := make([]store.NodeLabel, 0, len(in))
	for _, s := range in {
		k, v, _ := strings.Cut(s, "=")
		out = append(out, store.NodeLabel{Key: k, Value: v})
	}
	if err := Validate(out); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// Validate checks every label of a set and the size of the set.
func Validate(set []store.NodeLabel) error {
	if len(set) > MaxLabels {
		return newErr(ReasonTooMany, "%d labels, at most %d allowed", len(set), MaxLabels)
	}
	seen := map[string]bool{}
	for _, l := range set {
		if !keyRE.MatchString(l.Key) {
			return newErr(ReasonSyntax, "label key %q is not valid", l.Key)
		}
		if !valueRE.MatchString(l.Value) {
			return newErr(ReasonSyntax, "label value %q of key %s is not valid", l.Value, l.Key)
		}
		if strings.HasPrefix(l.Key, ReservedPrefix) {
			return newErr(ReasonReserved, "label key %s uses the reserved %s prefix", l.Key, ReservedPrefix)
		}
		if seen[l.Key] {
			return newErr(ReasonSyntax, "label key %s is given twice", l.Key)
		}
		seen[l.Key] = true
	}
	return nil
}

// Strings renders a label set in the placement form: `key` for a bare key, else `key=value`.
func Strings(set []store.NodeLabel) []string {
	out := make([]string, 0, len(set))
	for _, l := range set {
		if l.Value == "" {
			out = append(out, l.Key)
			continue
		}
		out = append(out, l.Key+"="+l.Value)
	}
	return out
}
