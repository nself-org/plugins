package verify

// Purpose: the content check of the two SBOMs next to an agent binary (D-1243). The provenance binds each SBOM by digest; this proves the
//          bytes behind the digest are an SBOM document of the right format that names the binary, so a signed release cannot carry
//          arbitrary bytes under the SBOM names.
// Inputs:  the bytes of <binary>.cdx.json and <binary>.spdx.json and the binary name.
// Outputs: nil, or an *Error with code E665 and reason "provenance" naming the SBOM that failed.
// Constraints: structural only and independent of the syft version (clients do not know which syft built a release): CycloneDX 1.6 with
//          metadata.component.name equal to the binary name, SPDX 2.3 with name equal to the binary name; provenance.sh passes
//          `--source-name <binary>` to syft, which sets exactly those two fields. internal/sbom has fuller validators, but they need the syft pin
//          and pull the installer and the store into this package, which the node-side client must stay free of.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// jsonObject decodes b as exactly one JSON object; keys are matched exactly (encoding/json would fold case).
func jsonObject(b []byte) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(&m); err != nil || m == nil {
		return nil, errors.New("not one JSON object")
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("data after the JSON object")
	}
	return m, nil
}

// stringField returns the JSON string at key of m, and whether it is one.
func stringField(m map[string]json.RawMessage, key string) (string, bool) {
	var s string
	raw, ok := m[key]
	if !ok || json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	return s, true
}

// checkCycloneDX requires a CycloneDX 1.6 document whose metadata.component is named name.
func checkCycloneDX(doc []byte, name string) error {
	top, err := jsonObject(doc)
	if err != nil {
		return provFail("the CycloneDX SBOM is %v", err)
	}
	if f, _ := stringField(top, "bomFormat"); f != "CycloneDX" {
		return provFail("the CycloneDX SBOM has bomFormat %q, not CycloneDX", clip(f))
	}
	if v, _ := stringField(top, "specVersion"); v != "1.6" {
		return provFail("the CycloneDX SBOM has specVersion %q, not 1.6", clip(v))
	}
	var comp map[string]json.RawMessage
	if meta, merr := jsonObject(top["metadata"]); merr == nil {
		comp, _ = jsonObject(meta["component"])
	}
	if got, _ := stringField(comp, "name"); got != name {
		return provFail("the CycloneDX SBOM names the component %q, not %s", clip(got), name)
	}
	return nil
}

// checkSPDX requires an SPDX 2.3 document named name.
func checkSPDX(doc []byte, name string) error {
	top, err := jsonObject(doc)
	if err != nil {
		return provFail("the SPDX SBOM is %v", err)
	}
	if v, _ := stringField(top, "spdxVersion"); v != "SPDX-2.3" {
		return provFail("the SPDX SBOM has spdxVersion %q, not SPDX-2.3", clip(v))
	}
	if got, _ := stringField(top, "name"); got != name {
		return provFail("the SPDX SBOM is named %q, not %s", clip(got), name)
	}
	return nil
}
