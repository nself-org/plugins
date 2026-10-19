package sbom_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/sbom"
)

func mutateCDX(t *testing.T, fn func(m map[string]any)) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(goldenDoc(t, "dir.cdx.json"), &m); err != nil {
		t.Fatal(err)
	}
	fn(m)
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestValidateCycloneDX(t *testing.T) {
	meta := func(m map[string]any) map[string]any { return m["metadata"].(map[string]any) }
	cases := []struct {
		name string
		fn   func(m map[string]any)
		want string // "" = valid; otherwise a substring of the E718 message
	}{
		{"golden", func(map[string]any) {}, ""},
		{"no components key", func(m map[string]any) { delete(m, "components") }, ""},
		{"empty components", func(m map[string]any) { m["components"] = []any{} }, ""},
		{"legacy tools list", func(m map[string]any) {
			meta(m)["tools"] = []any{map[string]any{"vendor": "anchore", "name": "syft", "version": pinVersion}}
		}, ""},
		{"wrong format", func(m map[string]any) { m["bomFormat"] = "SPDX" }, "bomFormat"},
		{"wrong spec", func(m map[string]any) { m["specVersion"] = "1.5" }, "specVersion"},
		{"serial not urn", func(m map[string]any) { m["serialNumber"] = "5101ade0-4b5e-44d4-8314-18db0b43e247" }, "serialNumber"},
		{"serial not uuid", func(m map[string]any) { m["serialNumber"] = "urn:uuid:nope" }, "serialNumber"},
		{"no version", func(m map[string]any) { delete(m, "version") }, "version"},
		{"no metadata", func(m map[string]any) { delete(m, "metadata") }, "metadata"},
		{"no tools", func(m map[string]any) { delete(meta(m), "tools") }, "metadata.tools"},
		{"other tool", func(m map[string]any) {
			meta(m)["tools"] = map[string]any{"components": []any{map[string]any{"name": "trivy", "version": pinVersion}}}
		}, "metadata.tools"},
		{"tool version differs", func(m map[string]any) {
			meta(m)["tools"] = map[string]any{"components": []any{map[string]any{"name": "syft", "version": "1.0.0"}}}
		}, "metadata.tools"},
		{"no subject component", func(m map[string]any) { delete(meta(m), "component") }, "metadata.component.name"},
		{"wrong subject name", func(m map[string]any) { meta(m)["component"] = map[string]any{"name": "other"} }, "metadata.component.name"},
		{"components object", func(m map[string]any) { m["components"] = map[string]any{} }, "components"},
		{"components null", func(m map[string]any) { m["components"] = nil }, "components"},
		{"component elements are not objects", func(m map[string]any) { m["components"] = []any{nil, 42, map[string]any{}} }, "components[0]"},
		{"component without a name", func(m map[string]any) { m["components"] = []any{map[string]any{"type": "library"}} }, "components[0]"},
		{"component without a type", func(m map[string]any) { m["components"] = []any{map[string]any{"name": "x"}} }, "components[0]"},
		{"component with a garbage type", func(m map[string]any) {
			m["components"] = []any{map[string]any{"type": "garbage", "name": "x"}}
		}, "components[0]"},
		{"subject with a garbage type", func(m map[string]any) {
			meta(m)["component"] = map[string]any{"type": "garbage", "name": "fx"}
		}, "metadata.component.type"},
		{"syft tool entry without a type", func(m map[string]any) {
			meta(m)["tools"] = map[string]any{"components": []any{map[string]any{"name": "syft", "version": pinVersion}}}
		}, "metadata.tools"},
		{"syft tool entry with a garbage type", func(m map[string]any) {
			meta(m)["tools"] = map[string]any{"components": []any{map[string]any{"type": "garbage", "name": "syft", "version": pinVersion}}}
		}, "metadata.tools"},
		{"subject without a type", func(m map[string]any) { meta(m)["component"] = map[string]any{"name": "fx"} }, "metadata.component.type"},
	}
	for _, c := range cases {
		err := sbom.ValidateCycloneDX(mutateCDX(t, c.fn), "fx", pinVersion)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: unexpected %v", c.name, err)
		case c.want != "" && (!sbom.IsReason(err, sbom.ReasonInvalid) || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: got %v, want E718 invalid mentioning %q", c.name, err, c.want)
		}
	}
}

func TestValidateCycloneDXRefusesNonJSON(t *testing.T) {
	for _, doc := range []string{"", "not json", "[]", `{"bomFormat":"CycloneDX"} {}`, `{"bomFormat":"CycloneDX"}garbage`} {
		if err := sbom.ValidateCycloneDX([]byte(doc), "fx", pinVersion); !sbom.IsReason(err, sbom.ReasonInvalid) {
			t.Errorf("%q: got %v", doc, err)
		}
	}
}

func TestValidatorsRefuseTrailingDelimiters(t *testing.T) {
	cdx, spdx := goldenDoc(t, "dir.cdx.json"), goldenDoc(t, "dir.spdx.json")
	for _, tail := range []string{"}", "]", "}}", "\n}\n", " garbage", "{}"} {
		if err := sbom.ValidateCycloneDX(append(append([]byte(nil), cdx...), tail...), "fx", pinVersion); !sbom.IsReason(err, sbom.ReasonInvalid) {
			t.Errorf("CycloneDX + %q: %v", tail, err)
		}
		if err := sbom.ValidateSPDX(append(append([]byte(nil), spdx...), tail...), "fx", pinVersion); !sbom.IsReason(err, sbom.ReasonInvalid) {
			t.Errorf("SPDX + %q: %v", tail, err)
		}
	}
	if err := sbom.ValidateCycloneDX(append(append([]byte(nil), cdx...), "\n\n"...), "fx", pinVersion); err != nil {
		t.Errorf("trailing whitespace is fine: %v", err)
	}
}

func TestValidateSPDX(t *testing.T) {
	good := goldenDoc(t, "dir.spdx.json")
	if err := sbom.ValidateSPDX(good, "fx", pinVersion); err != nil {
		t.Fatal(err)
	}
	for field, val := range map[string]any{"spdxVersion": "SPDX-2.2", "dataLicense": "MIT", "SPDXID": "x", "name": "", "documentNamespace": "", "creationInfo": nil} {
		var m map[string]any
		if err := json.Unmarshal(good, &m); err != nil {
			t.Fatal(err)
		}
		m[field] = val
		b, _ := json.Marshal(m)
		if err := sbom.ValidateSPDX(b, "fx", pinVersion); !sbom.IsReason(err, sbom.ReasonInvalid) {
			t.Errorf("%s=%v: got %v", field, val, err)
		}
	}
	if err := sbom.ValidateSPDX([]byte("{"), "fx", pinVersion); !sbom.IsReason(err, sbom.ReasonInvalid) {
		t.Fatalf("truncated: %v", err)
	}
	if err := sbom.ValidateSPDX(good, "other", pinVersion); !sbom.IsReason(err, sbom.ReasonInvalid) {
		t.Fatalf("SPDX of another artifact: %v", err)
	}
	if err := sbom.ValidateSPDX(good, "fx", "9.9.9"); !sbom.IsReason(err, sbom.ReasonInvalid) {
		t.Fatalf("SPDX from another syft: %v", err)
	}
}

func TestComponentTypeEnum(t *testing.T) {
	for _, ok := range []string{"application", "framework", "library", "container", "platform", "operating-system", "device", "device-driver", "firmware", "file", "machine-learning-model", "data", "cryptographic-asset"} {
		doc := mutateCDX(t, func(m map[string]any) { m["components"] = []any{map[string]any{"type": ok, "name": "x"}} })
		if err := sbom.ValidateCycloneDX(doc, "fx", pinVersion); err != nil {
			t.Errorf("type %q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"bogus", "Library", "service", ""} {
		doc := mutateCDX(t, func(m map[string]any) { m["components"] = []any{map[string]any{"type": bad, "name": "x"}} })
		if err := sbom.ValidateCycloneDX(doc, "fx", pinVersion); !sbom.IsReason(err, sbom.ReasonInvalid) {
			t.Errorf("type %q accepted: %v", bad, err)
		}
	}
}
