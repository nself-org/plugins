package onboard

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

const schemaPath = "../../schema/infra-v1.schema.json"

var (
	wantPhases = []string{"preflight", "quoted", "approved", "provisioning",
		"provisioned", "hardened", "installed", "tls", "healthy", "done",
		"failed", "rolled_back", "torn_down"}
	wantReasons = []string{"stale", "invalid", "unbound", "expired", "scope",
		"region", "quota", "budget", "offline", "unsupported", "conflict",
		"leftover"}
	hexDigest = "sha256:" + strings.Repeat("ab", 32)
	fixedAt   = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	// wireTypes maps each object of the schema $defs to its Go type.
	wireTypes = map[string]reflect.Type{
		"Plan": reflect.TypeOf(Plan{}), "Quote": reflect.TypeOf(Quote{}),
		"PriceLine": reflect.TypeOf(PriceLine{}), "TokenReport": reflect.TypeOf(TokenReport{}),
		"RunState": reflect.TypeOf(RunState{}), "PhaseEvent": reflect.TypeOf(PhaseEvent{}),
	}
	// wireFixtures holds one fully populated value per top-level wire type.
	wireFixtures = map[string]any{
		"Plan": Plan{Version: ContractVersion, Provider: "hetzner", ServerType: "cx22",
			Location: "nbg1", Image: "ubuntu-24.04", Name: "web-1",
			Labels: map[string]string{"managed-by": "nself"}, HardeningDigest: hexDigest,
			QuoteTotal: "4.51", Currency: "EUR", PriceListFetchedAt: fixedAt},
		"Quote": Quote{Version: ContractVersion, Provider: "hetzner", Location: "nbg1",
			Currency: "EUR", Total: "4.51", FetchedAt: fixedAt,
			Lines: []PriceLine{{Item: "cx22", Kind: "server", Hourly: "0.0072", Monthly: "4.51"}}},
		"TokenReport": TokenReport{Version: ContractVersion, Provider: "hetzner", Present: true,
			Source: "HCLOUD_TOKEN", Valid: false, Scope: "read", ExpiresAt: &fixedAt, Reason: ReasonScope},
		"RunState": RunState{Version: ContractVersion, RunID: "run-1", PlanHash: PlanHash(hexDigest),
			Phase: PhaseFailed, Reason: ReasonQuota, Adopted: true, ServerID: "12345678",
			IPv4: "203.0.113.7", IPv6: "2001:db8::/64", StartedAt: fixedAt,
			UpdatedAt: fixedAt.Add(time.Minute), History: []PhaseEvent{{Phase: PhasePreflight, At: fixedAt},
				{Phase: PhaseFailed, At: fixedAt.Add(time.Minute), Reason: ReasonQuota}}},
	}
	// delKey as a TestInfraV1SchemaRejects case value deletes the key.
	delKey = new(struct{})
)

func loadSchema(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(schemaPath)
	var s map[string]any
	if err == nil {
		err = json.Unmarshal(raw, &s)
	}
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	return s
}

func def(t *testing.T, root map[string]any, name string) map[string]any {
	t.Helper()
	d, ok := root["$defs"].(map[string]any)[name].(map[string]any)
	if !ok {
		t.Fatalf("schema has no $defs/%s", name)
	}
	return d
}

func strs(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

// jsonNames returns rt's sorted JSON names; requiredOnly drops omitempty ones.
func jsonNames(rt reflect.Type, requiredOnly bool) []string {
	var out []string
	for i := 0; i < rt.NumField(); i++ {
		parts := strings.Split(rt.Field(i).Tag.Get("json"), ",")
		if requiredOnly && len(parts) > 1 && parts[1] == "omitempty" {
			continue
		}
		out = append(out, parts[0])
	}
	slices.Sort(out)
	return out
}

// toDoc returns v as the generic JSON object a decoder would see.
func toDoc(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	var doc map[string]any
	if err == nil {
		err = json.Unmarshal(raw, &doc)
	}
	if err != nil {
		t.Fatalf("round-trip %T: %v", v, err)
	}
	return doc
}

// enumStrings returns vals as strings and reports each one valid rejects.
func enumStrings[T ~string](t *testing.T, vals []T, valid func(T) bool) []string {
	var out []string
	for _, v := range vals {
		if out = append(out, string(v)); !valid(v) {
			t.Errorf("%q not Valid", v)
		}
	}
	return out
}

func TestInfraV1Enums(t *testing.T) {
	s := loadSchema(t)
	goVals := map[string][]string{"Phase": enumStrings(t, AllPhases(), Phase.Valid),
		"Reason": enumStrings(t, AllReasons(), Reason.Valid)}
	for name, want := range map[string][]string{"Phase": wantPhases, "Reason": wantReasons} {
		if !reflect.DeepEqual(goVals[name], want) {
			t.Errorf("Go %s values = %v, want %v", name, goVals[name], want)
		}
		if got := strs(def(t, s, name)["enum"]); !reflect.DeepEqual(got, want) {
			t.Errorf("schema %s enum = %v, want %v", name, got, want)
		}
	}
	if Phase("running").Valid() || Reason("other").Valid() {
		t.Error("unknown enum value reported Valid")
	}
}

// TestInfraV1NoSecretFields proves no wire type has a secret-like Go or JSON field name.
func TestInfraV1NoSecretFields(t *testing.T) {
	bad := regexp.MustCompile(`(?i)token|key|secret|password|credential`)
	for name, rt := range wireTypes {
		for i := 0; i < rt.NumField(); i++ {
			if f := rt.Field(i); bad.MatchString(f.Name) || bad.MatchString(f.Tag.Get("json")) {
				t.Errorf("%s.%s looks like a secret field", name, f.Name)
			}
		}
	}
}

// TestInfraV1SchemaMatchesTypes proves per wire type that schema properties are the Go JSON
// names, required is the non-omitempty fields, additionalProperties is false; fixtures validate.
func TestInfraV1SchemaMatchesTypes(t *testing.T) {
	s := loadSchema(t)
	for name, rt := range wireTypes {
		d := def(t, s, name)
		props := slices.Sorted(maps.Keys(d["properties"].(map[string]any)))
		if want := jsonNames(rt, false); !reflect.DeepEqual(props, want) {
			t.Errorf("%s: schema properties %v, Go fields %v", name, props, want)
		}
		req := slices.Sorted(slices.Values(strs(d["required"])))
		if want := jsonNames(rt, true); !reflect.DeepEqual(req, want) {
			t.Errorf("%s: schema required %v, Go non-omitempty fields %v", name, req, want)
		}
		if d["additionalProperties"] != false {
			t.Errorf("%s: additionalProperties must be false", name)
		}
	}
	for name, v := range wireFixtures {
		if err := validateSchema(s, def(t, s, name), toDoc(t, v), "$"); err != nil {
			t.Errorf("%s fixture does not validate: %v", name, err)
		}
	}
}

// TestInfraV1SchemaRejects proves the validator and schema reject wrong
// types, wrong nested items, extra keys, missing keys and unknown enum values.
func TestInfraV1SchemaRejects(t *testing.T) {
	s := loadSchema(t)
	ts := fixedAt.Format(time.RFC3339)
	cases := []struct {
		typ, key string
		val      any
	}{
		{"Plan", "provider", 7},
		{"Plan", "labels", map[string]any{"a": 1}},
		{"Plan", "token", "x"},
		{"Plan", "image", delKey},
		{"Plan", "hardening_digest", "abc"},
		{"Plan", "price_list_fetched_at", "yesterday"},
		{"Plan", "version", "infra.v2"},
		{"Plan", "quote_total", "4,51"},
		{"Quote", "lines", []any{map[string]any{"item": "cx22", "kind": "server", "hourly": 0.0072, "monthly": "4.51"}}},
		{"Quote", "lines", []any{map[string]any{"item": "cx22", "kind": "server", "hourly": "0.1"}}},
		{"Quote", "lines", "none"},
		{"RunState", "phase", "running"},
		{"RunState", "reason", "other"},
		{"RunState", "history", []any{map[string]any{"phase": "nope", "at": ts}}},
		{"RunState", "history", []any{map[string]any{"phase": "failed", "at": ts, "reason": 3}}},
		{"RunState", "adopted", "yes"},
		{"RunState", "adopted", delKey},
		{"TokenReport", "expires_at", "soon"},
	}
	for i, c := range cases {
		m := toDoc(t, wireFixtures[c.typ])
		if err := validateSchema(s, def(t, s, c.typ), m, "$"); err != nil {
			t.Fatalf("baseline %s must validate: %v", c.typ, err)
		}
		if c.val == delKey {
			delete(m, c.key)
		} else {
			m[c.key] = c.val
		}
		if validateSchema(s, def(t, s, c.typ), m, "$") == nil {
			t.Errorf("case %d: %s with bad %s validated but must be rejected", i, c.typ, c.key)
		}
	}
}

// validateSchema checks doc against the JSON Schema subset infra.v1 uses: $ref, type,
// enum, pattern, minLength, date-time, required, properties, additionalProperties, items.
func validateSchema(root, s map[string]any, doc any, path string) error {
	if ref, ok := s["$ref"].(string); ok {
		d, ok := root["$defs"].(map[string]any)[strings.TrimPrefix(ref, "#/$defs/")].(map[string]any)
		if !ok {
			return fmt.Errorf("%s: unresolved $ref %s", path, ref)
		}
		return validateSchema(root, d, doc, path)
	}
	if tv, ok := s["type"]; ok && !typeOK(tv, doc) {
		return fmt.Errorf("%s: type %T not %v", path, doc, tv)
	}
	if enum, ok := s["enum"].([]any); ok && !slices.Contains(enum, doc) {
		return fmt.Errorf("%s: %v not in enum", path, doc)
	}
	if str, ok := doc.(string); ok {
		if p, ok := s["pattern"].(string); ok && !regexp.MustCompile(p).MatchString(str) {
			return fmt.Errorf("%s: %q does not match %s", path, str, p)
		}
		if n, ok := s["minLength"].(float64); ok && len(str) < int(n) {
			return fmt.Errorf("%s: shorter than %v", path, n)
		}
		if _, err := time.Parse(time.RFC3339, str); s["format"] == "date-time" && err != nil {
			return fmt.Errorf("%s: not date-time: %v", path, err)
		}
	}
	arr, _ := doc.([]any)
	items, _ := s["items"].(map[string]any)
	for i, x := range arr {
		if err := validateSchema(root, items, x, fmt.Sprintf("%s[%d]", path, i)); err != nil {
			return err
		}
	}
	obj, ok := doc.(map[string]any)
	if !ok {
		return nil
	}
	req, _ := s["required"].([]any)
	for _, r := range req {
		if _, ok := obj[r.(string)]; !ok {
			return fmt.Errorf("%s: missing %s", path, r)
		}
	}
	props, _ := s["properties"].(map[string]any)
	for k, val := range obj {
		sub := path + "." + k
		if ps, ok := props[k].(map[string]any); ok {
			if err := validateSchema(root, ps, val, sub); err != nil {
				return err
			}
			continue
		}
		switch ap := s["additionalProperties"].(type) {
		case bool:
			if !ap {
				return fmt.Errorf("%s: property not allowed", sub)
			}
		case map[string]any:
			if err := validateSchema(root, ap, val, sub); err != nil {
				return err
			}
		}
	}
	return nil
}

// typeOK reports whether doc has the JSON type tv, or one of the list tv.
func typeOK(tv, doc any) bool {
	if list, ok := tv.([]any); ok {
		return slices.ContainsFunc(list, func(x any) bool { return typeOK(x, doc) })
	}
	_, isObject := doc.(map[string]any)
	_, isArray := doc.([]any)
	_, isString := doc.(string)
	_, isBool := doc.(bool)
	return map[any]bool{"object": isObject, "array": isArray, "string": isString,
		"boolean": isBool, "null": doc == nil}[tv]
}
