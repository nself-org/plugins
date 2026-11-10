package onboard

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
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
)

func loadSchema(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var s map[string]any
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("parse schema: %v", err)
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

func TestInfraV1Enums(t *testing.T) {
	var gotP, gotR []string
	for _, p := range AllPhases() {
		gotP = append(gotP, string(p))
		if !p.Valid() {
			t.Errorf("phase %q not Valid", p)
		}
	}
	for _, r := range AllReasons() {
		gotR = append(gotR, string(r))
		if !r.Valid() {
			t.Errorf("reason %q not Valid", r)
		}
	}
	if !reflect.DeepEqual(gotP, wantPhases) {
		t.Errorf("phases = %v, want %v", gotP, wantPhases)
	}
	if !reflect.DeepEqual(gotR, wantReasons) {
		t.Errorf("reasons = %v, want %v", gotR, wantReasons)
	}
	if Phase("running").Valid() || Reason("other").Valid() {
		t.Error("unknown enum value reported Valid")
	}
	s := loadSchema(t)
	if got := strs(def(t, s, "Phase")["enum"]); !reflect.DeepEqual(got, wantPhases) {
		t.Errorf("schema Phase enum = %v, want %v", got, wantPhases)
	}
	if got := strs(def(t, s, "Reason")["enum"]); !reflect.DeepEqual(got, wantReasons) {
		t.Errorf("schema Reason enum = %v, want %v", got, wantReasons)
	}
}

func TestInfraV1NoSecretFields(t *testing.T) {
	bad := regexp.MustCompile(`(?i)token|key|secret|password|credential`)
	for _, v := range []any{Plan{}, RunState{}, PhaseEvent{}} {
		rt := reflect.TypeOf(v)
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			if bad.MatchString(f.Name) || bad.MatchString(f.Tag.Get("json")) {
				t.Errorf("%s.%s looks like a secret field", rt.Name(), f.Name)
			}
		}
	}
}

func jsonNames(rt reflect.Type) []string {
	var out []string
	for i := 0; i < rt.NumField(); i++ {
		out = append(out, strings.Split(rt.Field(i).Tag.Get("json"), ",")[0])
	}
	sort.Strings(out)
	return out
}

func TestInfraV1SchemaMatchesTypes(t *testing.T) {
	s := loadSchema(t)
	types := map[string]reflect.Type{
		"Plan": reflect.TypeOf(Plan{}), "Quote": reflect.TypeOf(Quote{}),
		"PriceLine": reflect.TypeOf(PriceLine{}), "TokenReport": reflect.TypeOf(TokenReport{}),
		"RunState": reflect.TypeOf(RunState{}), "PhaseEvent": reflect.TypeOf(PhaseEvent{}),
	}
	for name, rt := range types {
		var props []string
		for k := range def(t, s, name)["properties"].(map[string]any) {
			props = append(props, k)
		}
		sort.Strings(props)
		if want := jsonNames(rt); !reflect.DeepEqual(props, want) {
			t.Errorf("%s: schema properties %v, Go fields %v", name, props, want)
		}
	}

	at := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	plan := Plan{Version: ContractVersion, Provider: "hetzner", ServerType: "cx22",
		Location: "nbg1", Image: "ubuntu-24.04", Name: "web-1",
		Labels: map[string]string{"managed-by": "nself"}, HardeningDigest: hexDigest,
		QuoteTotal: "4.51", Currency: "EUR", PriceListFetchedAt: at}
	quote := Quote{Version: ContractVersion, Provider: "hetzner", Location: "nbg1",
		Currency: "EUR", Total: "4.51", FetchedAt: at,
		Lines: []PriceLine{{Item: "cx22", Kind: "server", Hourly: "0.0072", Monthly: "4.51"}}}
	token := TokenReport{Version: ContractVersion, Provider: "hetzner", Present: true,
		Source: "HCLOUD_TOKEN", Valid: false, Scope: "read", Reason: ReasonScope}
	run := RunState{Version: ContractVersion, RunID: "run-1", PlanHash: PlanHash(hexDigest),
		Phase: PhaseFailed, Reason: ReasonQuota, ServerID: "12345678", IPv4: "203.0.113.7",
		IPv6: "2001:db8::/64", StartedAt: at, UpdatedAt: at.Add(time.Minute),
		History: []PhaseEvent{{Phase: PhasePreflight, At: at},
			{Phase: PhaseFailed, At: at.Add(time.Minute), Reason: ReasonQuota}}}

	for name, v := range map[string]any{"Plan": plan, "Quote": quote,
		"TokenReport": token, "RunState": run} {
		if err := validateValue(s, def(t, s, name), v); err != nil {
			t.Errorf("%s does not validate: %v", name, err)
		}
	}

	badRun := run
	badRun.Phase = "running"
	if validateValue(s, def(t, s, "RunState"), badRun) == nil {
		t.Error("RunState with unknown phase validated")
	}
	badPlan := plan
	badPlan.QuoteTotal = "4,51"
	if validateValue(s, def(t, s, "Plan"), badPlan) == nil {
		t.Error("Plan with bad quote_total validated")
	}
}

func validateValue(root, schema map[string]any, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	return validate(root, schema, doc, "$")
}

// validate checks doc against the subset of JSON Schema the infra.v1 schema
// uses: $ref, type, enum, pattern, minLength, format date-time, required,
// properties, additionalProperties and items.
func validate(root, s map[string]any, doc any, path string) error {
	if ref, ok := s["$ref"].(string); ok {
		name := strings.TrimPrefix(ref, "#/$defs/")
		d, ok := root["$defs"].(map[string]any)[name].(map[string]any)
		if !ok {
			return fmt.Errorf("%s: unresolved $ref %s", path, ref)
		}
		return validate(root, d, doc, path)
	}
	if tv, ok := s["type"]; ok && !typeOK(tv, doc) {
		return fmt.Errorf("%s: type %T not %v", path, doc, tv)
	}
	if enum, ok := s["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			found = found || e == doc
		}
		if !found {
			return fmt.Errorf("%s: %v not in enum", path, doc)
		}
	}
	if str, ok := doc.(string); ok {
		if p, ok := s["pattern"].(string); ok && !regexp.MustCompile(p).MatchString(str) {
			return fmt.Errorf("%s: %q does not match %s", path, str, p)
		}
		if n, ok := s["minLength"].(float64); ok && len(str) < int(n) {
			return fmt.Errorf("%s: shorter than %v", path, n)
		}
		if s["format"] == "date-time" {
			if _, err := time.Parse(time.RFC3339, str); err != nil {
				return fmt.Errorf("%s: not date-time: %v", path, err)
			}
		}
	}
	if arr, ok := doc.([]any); ok {
		if items, ok := s["items"].(map[string]any); ok {
			for i, x := range arr {
				if err := validate(root, items, x, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
	}
	obj, ok := doc.(map[string]any)
	if !ok {
		return nil
	}
	if req, ok := s["required"].([]any); ok {
		for _, r := range req {
			if _, ok := obj[r.(string)]; !ok {
				return fmt.Errorf("%s: missing %s", path, r)
			}
		}
	}
	props, _ := s["properties"].(map[string]any)
	for k, val := range obj {
		sub := path + "." + k
		if ps, ok := props[k].(map[string]any); ok {
			if err := validate(root, ps, val, sub); err != nil {
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
			if err := validate(root, ap, val, sub); err != nil {
				return err
			}
		}
	}
	return nil
}

func typeOK(tv, doc any) bool {
	if list, ok := tv.([]any); ok {
		for _, x := range list {
			if typeOK(x, doc) {
				return true
			}
		}
		return false
	}
	switch tv {
	case "object":
		_, ok := doc.(map[string]any)
		return ok
	case "array":
		_, ok := doc.([]any)
		return ok
	case "string":
		_, ok := doc.(string)
		return ok
	case "boolean":
		_, ok := doc.(bool)
		return ok
	case "null":
		return doc == nil
	}
	return false
}

// TestInfraV1RequiredMatchesTags proves the schema's required list is exactly
// the set of Go fields without omitempty, for every wire type.
func TestInfraV1RequiredMatchesTags(t *testing.T) {
	s := loadSchema(t)
	types := map[string]reflect.Type{
		"Plan": reflect.TypeOf(Plan{}), "Quote": reflect.TypeOf(Quote{}),
		"PriceLine": reflect.TypeOf(PriceLine{}), "TokenReport": reflect.TypeOf(TokenReport{}),
		"RunState": reflect.TypeOf(RunState{}), "PhaseEvent": reflect.TypeOf(PhaseEvent{}),
	}
	for name, rt := range types {
		var want []string
		for i := 0; i < rt.NumField(); i++ {
			parts := strings.Split(rt.Field(i).Tag.Get("json"), ",")
			if len(parts) > 1 && parts[1] == "omitempty" {
				continue
			}
			want = append(want, parts[0])
		}
		got := strs(def(t, s, name)["required"])
		sort.Strings(want)
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: schema required %v, Go non-omitempty fields %v", name, got, want)
		}
		if def(t, s, name)["additionalProperties"] != false {
			t.Errorf("%s: additionalProperties must be false", name)
		}
	}
}

// TestInfraV1SchemaRejects proves the validator and schema reject wrong
// types, wrong nested items, extra keys, missing keys and unknown enum values.
func TestInfraV1SchemaRejects(t *testing.T) {
	s := loadSchema(t)
	at := "2026-11-03T12:00:00Z"
	plan := func() map[string]any {
		return map[string]any{"version": "infra.v1", "provider": "hetzner",
			"server_type": "cx22", "location": "nbg1", "image": "ubuntu-24.04",
			"name": "web-1", "labels": map[string]any{"a": "b"},
			"hardening_digest": hexDigest, "quote_total": "4.51", "currency": "EUR",
			"price_list_fetched_at": at}
	}
	quote := func() map[string]any {
		return map[string]any{"version": "infra.v1", "provider": "hetzner",
			"location": "nbg1", "currency": "EUR", "total": "4.51", "fetched_at": at,
			"lines": []any{map[string]any{"item": "cx22", "kind": "server",
				"hourly": "0.0072", "monthly": "4.51"}}}
	}
	run := func() map[string]any {
		return map[string]any{"version": "infra.v1", "run_id": "r1", "plan_hash": hexDigest,
			"phase": "failed", "reason": "quota", "started_at": at, "updated_at": at,
			"history": []any{map[string]any{"phase": "failed", "at": at, "reason": "quota"}}}
	}
	for name, ok := range map[string]map[string]any{"Plan": plan(), "Quote": quote(), "RunState": run()} {
		if err := validate(s, def(t, s, name), ok, "$"); err != nil {
			t.Fatalf("baseline %s must validate: %v", name, err)
		}
	}
	cases := []struct {
		name, typ string
		mutate    func() map[string]any
	}{
		{"provider integer", "Plan", func() map[string]any { m := plan(); m["provider"] = 7; return m }},
		{"labels value not string", "Plan", func() map[string]any { m := plan(); m["labels"] = map[string]any{"a": 1}; return m }},
		{"extra key", "Plan", func() map[string]any { m := plan(); m["token"] = "x"; return m }},
		{"missing key", "Plan", func() map[string]any { m := plan(); delete(m, "image"); return m }},
		{"bad digest", "Plan", func() map[string]any { m := plan(); m["hardening_digest"] = "abc"; return m }},
		{"bad time", "Plan", func() map[string]any { m := plan(); m["price_list_fetched_at"] = "yesterday"; return m }},
		{"bad version", "Plan", func() map[string]any { m := plan(); m["version"] = "infra.v2"; return m }},
		{"price line number", "Quote", func() map[string]any {
			m := quote()
			m["lines"] = []any{map[string]any{"item": "cx22", "kind": "server", "hourly": 0.0072, "monthly": "4.51"}}
			return m
		}},
		{"price line missing key", "Quote", func() map[string]any {
			m := quote()
			m["lines"] = []any{map[string]any{"item": "cx22", "kind": "server", "hourly": "0.1"}}
			return m
		}},
		{"lines not array", "Quote", func() map[string]any { m := quote(); m["lines"] = "none"; return m }},
		{"unknown phase", "RunState", func() map[string]any { m := run(); m["phase"] = "running"; return m }},
		{"unknown reason", "RunState", func() map[string]any { m := run(); m["reason"] = "other"; return m }},
		{"history bad phase", "RunState", func() map[string]any {
			m := run()
			m["history"] = []any{map[string]any{"phase": "nope", "at": at}}
			return m
		}},
		{"history reason in token slot", "RunState", func() map[string]any {
			m := run()
			m["history"] = []any{map[string]any{"phase": "failed", "at": at, "reason": 3}}
			return m
		}},
	}
	for _, c := range cases {
		if err := validate(s, def(t, s, c.typ), c.mutate(), "$"); err == nil {
			t.Errorf("%s: %s validated but must be rejected", c.typ, c.name)
		}
	}
}
